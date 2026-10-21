package grep_test

import (
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"

	grep "github.com/yornik/go-practice/exercises/29-grep"
)

func TestCountingReader(t *testing.T) {
	requireCountingReader(t)
	const data = "ts=2026-10-16T03:12:00Z level=error msg=\"db timeout\"\n"

	t.Run("starts at 0", func(t *testing.T) {
		c := grep.NewCountingReader(strings.NewReader(data))
		if got := c.N(); got != 0 {
			t.Errorf("N() before any Read = %d, want 0", got)
		}
	})

	t.Run("counts every byte read", func(t *testing.T) {
		c := grep.NewCountingReader(strings.NewReader(data))
		got, err := io.ReadAll(c)
		// io.ReadAll stops at err == io.EOF, so this also checks that
		// Read passes io.EOF through as it is.
		if err != nil {
			t.Fatalf("io.ReadAll(CountingReader) error = %v, want nil", err)
		}
		if string(got) != data {
			t.Errorf("io.ReadAll(CountingReader) = %q, want %q", got, data)
		}
		if c.N() != int64(len(data)) {
			t.Errorf("N() = %d, want %d", c.N(), len(data))
		}
	})

	t.Run("counts what Read returned, not len(p)", func(t *testing.T) {
		c := grep.NewCountingReader(iotest.OneByteReader(strings.NewReader(data)))
		p := make([]byte, 100)
		n, err := c.Read(p)
		if n != 1 || err != nil {
			t.Fatalf("Read = %d, %v, want 1, nil", n, err)
		}
		if c.N() != 1 {
			t.Errorf("N() after reading 1 byte into a 100-byte buffer = %d, want 1", c.N())
		}
	})

	t.Run("behaves like the reader it wraps", func(t *testing.T) {
		// iotest.TestReader reads with several buffer sizes and checks
		// the content and the io.EOF at the end.
		c := grep.NewCountingReader(strings.NewReader(data))
		if err := iotest.TestReader(c, []byte(data)); err != nil {
			t.Error(err)
		}
	})

	t.Run("io.EOF is returned unchanged", func(t *testing.T) {
		c := grep.NewCountingReader(strings.NewReader(""))
		n, err := c.Read(make([]byte, 10))
		// Callers compare err == io.EOF, so it must not be wrapped.
		if n != 0 || err != io.EOF {
			t.Errorf("Read at the end = %d, %v, want 0, io.EOF", n, err)
		}
	})

	t.Run("bytes returned together with an error are counted", func(t *testing.T) {
		errDisk := errors.New("input/output error")
		c := grep.NewCountingReader(&errAtEnd{data: "abc", err: errDisk})
		n, err := c.Read(make([]byte, 10))
		if n != 3 || err != errDisk {
			t.Fatalf("Read = %d, %v, want 3, %v (n and err unchanged)", n, err, errDisk)
		}
		if c.N() != 3 {
			t.Errorf("N() = %d, want 3", c.N())
		}
		n, err = c.Read(make([]byte, 10))
		if n != 0 || err != errDisk {
			t.Fatalf("second Read = %d, %v, want 0, %v", n, err, errDisk)
		}
		if c.N() != 3 {
			t.Errorf("N() after the second Read = %d, want 3", c.N())
		}
	})
}
