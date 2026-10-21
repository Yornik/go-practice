package diskbuffer_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	diskbuffer "github.com/yornik/go-practice/exercises/49-diskbuffer"
)

// timeout bounds every call into the Buffer, so a deadlock (for example a
// method that locks a mutex it already holds) fails the test with a message
// instead of hanging until the test binary is killed.
const timeout = 5 * time.Second

// logLine is the payload the tests use for record i. It is always 20 bytes,
// so every such record takes recSize bytes on disk.
func logLine(i int) string { return fmt.Sprintf("log line %011d", i) }

// recSize is the size of a logLine record on disk.
const recSize = diskbuffer.HeaderSize + 20

// logLines returns logLine(from) to logLine(to).
func logLines(from, to int) []string {
	var s []string
	for i := from; i <= to; i++ {
		s = append(s, logLine(i))
	}
	return s
}

// segName is the file name of the segment that starts at seq first.
func segName(first uint64) string { return fmt.Sprintf("%020d.seg", first) }

// enc returns the bytes of records with these payloads, one after the other,
// as they are on disk. It uses your AppendRecord; the TestRecord tests check
// it byte by byte, so make those pass first.
func enc(payloads ...string) []byte {
	var b []byte
	for _, p := range payloads {
		b = diskbuffer.AppendRecord(b, []byte(p))
	}
	return b
}

// cat joins byte slices.
func cat(parts ...[]byte) []byte { return bytes.Join(parts, nil) }

// within runs f in a new goroutine and waits for it. It fails the test if f
// panics (such as the "TODO: implement" of a stub) or does not return
// within the timeout.
func within(t *testing.T, what string, f func()) {
	t.Helper()
	withinFor(t, what, timeout, f)
}

// withinFor is within with its own timeout d.
func withinFor(t *testing.T, what string, d time.Duration, f func()) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	select {
	case v := <-done:
		if v != nil {
			t.Fatalf("%s panicked: %v", what, v)
		}
	case <-time.After(d):
		t.Fatalf("%s did not return within %v: a deadlock?", what, d)
	}
}

// rec is a record as ReadFrom delivers it.
type rec struct {
	Seq     uint64
	Payload string
}

// recsFrom returns the records that payloads become when the first one has
// seq first.
func recsFrom(first uint64, payloads ...string) []rec {
	var rs []rec
	for i, p := range payloads {
		rs = append(rs, rec{Seq: first + uint64(i), Payload: p})
	}
	return rs
}

// tbuf wraps a *diskbuffer.Buffer. Every method runs the call through
// within, so a panic or a deadlock fails the test with a clear message.
type tbuf struct {
	t   *testing.T
	b   *diskbuffer.Buffer
	dir string
}

// opts returns Options with limits that do not get in the way.
func opts(dir string) diskbuffer.Options {
	return diskbuffer.Options{Dir: dir, SegmentBytes: 1 << 20, MaxRecordBytes: 1 << 10}
}

// open calls diskbuffer.Open. If it succeeds, the buffer is closed when the
// test ends (if the test did not close it).
func open(t *testing.T, o diskbuffer.Options) (*tbuf, error) {
	t.Helper()
	var b *diskbuffer.Buffer
	var err error
	within(t, fmt.Sprintf("Open(%+v)", o), func() { b, err = diskbuffer.Open(o) })
	if err != nil {
		return nil, err
	}
	if b == nil {
		t.Fatalf("Open(%+v) = nil, nil; want a Buffer or an error", o)
	}
	t.Cleanup(func() {
		done := make(chan struct{})
		go func() {
			defer close(done)
			defer func() { _ = recover() }() // Close may still be a stub
			_ = b.Close()
		}()
		select {
		case <-done:
		case <-time.After(timeout):
		}
	})
	return &tbuf{t: t, b: b, dir: o.Dir}, nil
}

// mustOpen is open, and fails the test on an error.
func mustOpen(t *testing.T, o diskbuffer.Options) *tbuf {
	t.Helper()
	w, err := open(t, o)
	if err != nil {
		t.Fatalf("Open(%+v) error = %v, want nil", o, err)
	}
	return w
}

func (w *tbuf) Append(p string) (seq uint64, err error) {
	w.t.Helper()
	within(w.t, fmt.Sprintf("Append(%q)", p), func() { seq, err = w.b.Append([]byte(p)) })
	return seq, err
}

// mustAppend appends the payloads and checks that they get the seqs first,
// first+1, ...
func (w *tbuf) mustAppend(first uint64, payloads ...string) {
	w.t.Helper()
	for i, p := range payloads {
		want := first + uint64(i)
		seq, err := w.Append(p)
		if err != nil {
			w.t.Fatalf("Append(%q) error = %v, want seq %d", p, err, want)
		}
		if seq != want {
			w.t.Fatalf("Append(%q) = %d, want %d", p, seq, want)
		}
	}
}

func (w *tbuf) ReadFrom(seq uint64, fn func(seq uint64, p []byte) error) (err error) {
	w.t.Helper()
	within(w.t, fmt.Sprintf("ReadFrom(%d, fn)", seq), func() { err = w.b.ReadFrom(seq, fn) })
	return err
}

// readAll returns every record that ReadFrom(seq) delivers.
func (w *tbuf) readAll(seq uint64) []rec {
	w.t.Helper()
	var got []rec
	err := w.ReadFrom(seq, func(s uint64, p []byte) error {
		got = append(got, rec{Seq: s, Payload: string(p)})
		return nil
	})
	if err != nil {
		w.t.Fatalf("ReadFrom(%d) error = %v, want nil", seq, err)
	}
	return got
}

// expectRecords checks that ReadFrom(seq) delivers exactly want.
func (w *tbuf) expectRecords(seq uint64, want []rec) {
	w.t.Helper()
	if got := w.readAll(seq); !slices.Equal(got, want) {
		w.t.Fatalf("ReadFrom(%d) delivered\n  %v\nwant\n  %v", seq, got, want)
	}
}

func (w *tbuf) Ack(seq uint64) (err error) {
	w.t.Helper()
	within(w.t, fmt.Sprintf("Ack(%d)", seq), func() { err = w.b.Ack(seq) })
	return err
}

func (w *tbuf) mustAck(seq uint64) {
	w.t.Helper()
	if err := w.Ack(seq); err != nil {
		w.t.Fatalf("Ack(%d) error = %v, want nil", seq, err)
	}
}

func (w *tbuf) Acked() (seq uint64) {
	w.t.Helper()
	within(w.t, "Acked()", func() { seq = w.b.Acked() })
	return seq
}

func (w *tbuf) Close() (err error) {
	w.t.Helper()
	within(w.t, "Close()", func() { err = w.b.Close() })
	return err
}

func (w *tbuf) mustClose() {
	w.t.Helper()
	if err := w.Close(); err != nil {
		w.t.Fatalf("Close() error = %v, want nil", err)
	}
}

// expectFiles checks that dir holds exactly these file names.
func expectFiles(t *testing.T, dir string, want ...string) {
	t.Helper()
	got := listFiles(t, dir)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("files in the buffer directory = %q, want %q", got, want)
	}
}

// listFiles returns the names of the files in dir, sorted.
func listFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the buffer directory: %v", err)
	}
	names := []string{}
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// expectFile checks the content of the file name in dir.
func expectFile(t *testing.T, dir, name string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("content of %s (%d bytes):\n% x\nwant (%d bytes):\n% x", name, len(got), got, len(want), want)
	}
}

// writeFiles creates files in dir, as a crashed process could have left
// them.
func writeFiles(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// snapshot returns the name and content of every file in dir.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	m := map[string]string{}
	for _, name := range listFiles(t, dir) {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		m[name] = string(data)
	}
	return m
}

// flip returns a copy of b with the bits of the byte at index i inverted:
// what a bad disk sector or a bad cable does to data.
func flip(b []byte, i int) []byte {
	c := bytes.Clone(b)
	c[i] ^= 0xff
	return c
}

// errStop is what the tests' ReadFrom callbacks return to stop early.
var errStop = errors.New("stop here")
