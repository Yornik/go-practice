package diskbuffer_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"

	diskbuffer "github.com/yornik/go-practice/exercises/24-diskbuffer"
)

// Step 1: the record format. These tests need only AppendRecord and
// DecodeRecord:
//
//	go test -run 'TestRecord|FuzzRecord' -v ./exercises/24-diskbuffer/

// unhex turns "00000009 e3069283" into bytes. Spaces are for readability.
func unhex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(strings.ReplaceAll(s, " ", ""))
	if err != nil {
		t.Fatalf("bad hex in test: %v", err)
	}
	return b
}

// catchPanic turns a panic in the test's own goroutine, such as the
// "TODO: implement" of a stub, into a test failure, so the other tests still
// run. Use it as: defer catchPanic(t).
func catchPanic(t *testing.T) {
	t.Helper()
	if v := recover(); v != nil {
		t.Fatalf("panicked: %v", v)
	}
}

func TestRecordAppendRecord(t *testing.T) {
	tests := []struct {
		name       string
		dst        string
		payload    string
		wantHeader string // length, then CRC-32C, in hex
	}{
		// "123456789" is the standard check input for CRC algorithms; its
		// CRC-32C is e3069283. (With the IEEE polynomial of crc32.ChecksumIEEE
		// it would be cbf43926: the wrong table.)
		{"check value", "", "123456789", "00000009 e3069283"},
		{"empty payload", "", "", "00000000 00000000"},
		{"a log line", "", `level=error msg="db timeout"`, "0000001c b22fb618"},
		// 300 = 0x012c: the length is 4 bytes, big-endian (most significant
		// byte first).
		{"length above 255", "", strings.Repeat("a", 300), "0000012c 43c2695f"},
		{"appends after what dst holds", "xy", "hi", "00000002 f59dd9c2"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			want := cat([]byte(tc.dst), unhex(t, tc.wantHeader), []byte(tc.payload))
			got := diskbuffer.AppendRecord([]byte(tc.dst), []byte(tc.payload))
			if !bytes.Equal(got, want) {
				t.Errorf("AppendRecord(%q, %.20q) =\n% x\nwant\n% x", tc.dst, tc.payload, got, want)
			}
		})
	}
}

func TestRecordDecodeRecord(t *testing.T) {
	defer catchPanic(t)
	payloads := []string{
		"123456789",
		"",
		`level=error msg="db timeout"`,
		strings.Repeat("a", 300),
		"\x00\xff\n binary is fine too",
	}
	// All records back to back, as in a segment file. DecodeRecord decodes the
	// first record of what it gets and ignores the bytes after it.
	b := enc(payloads...)
	off := 0
	for i, want := range payloads {
		p, n, err := diskbuffer.DecodeRecord(b[off:])
		if err != nil {
			t.Fatalf("record %d at offset %d: DecodeRecord error = %v, want nil", i, off, err)
		}
		if string(p) != want {
			t.Fatalf("record %d at offset %d: DecodeRecord payload = %.30q, want %.30q", i, off, p, want)
		}
		if wantN := diskbuffer.HeaderSize + len(want); n != wantN {
			t.Fatalf("record %d at offset %d: DecodeRecord n = %d, want %d (header + payload)", i, off, n, wantN)
		}
		off += n
	}
	p, n, err := diskbuffer.DecodeRecord(b[off:])
	if !errors.Is(err, io.EOF) || p != nil || n != 0 {
		t.Fatalf("DecodeRecord(empty) = %q, %d, %v; want nil, 0, io.EOF", p, n, err)
	}
}

func TestRecordDecodeRecordErrors(t *testing.T) {
	defer catchPanic(t)
	good := enc("123456789") // 8 bytes header + 9 bytes payload = 17 bytes
	tests := []struct {
		name    string
		in      []byte
		wantErr error
		wantN   int
	}{
		{"empty input", []byte{}, io.EOF, 0},
		{"nil input", nil, io.EOF, 0},
		// A crash in the middle of a write leaves the first part of a record.
		{"short header", good[:5], io.ErrUnexpectedEOF, 0},
		{"header without payload", good[:8], io.ErrUnexpectedEOF, 0},
		{"short payload", good[:12], io.ErrUnexpectedEOF, 0},
		// A flipped bit in the length can make it huge. Do not allocate 4 GiB
		// because a header says so: compare with the bytes that are there.
		{"length far past the end", cat(unhex(t, "ffffffff e3069283"), []byte("123")), io.ErrUnexpectedEOF, 0},
		// Damage the checksum catches. n is still the size from the header.
		{"payload byte flipped", flip(good, 16), diskbuffer.ErrChecksum, 17},
		{"checksum byte flipped", flip(good, 5), diskbuffer.ErrChecksum, 17},
		{"bad record followed by a good one", cat(flip(good, 10), enc("hi")), diskbuffer.ErrChecksum, 17},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			p, n, err := diskbuffer.DecodeRecord(tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("DecodeRecord(% x) error = %v, want %v", tc.in, err, tc.wantErr)
			}
			if p != nil {
				t.Errorf("DecodeRecord(% x) payload = %q, want nil on error", tc.in, p)
			}
			if n != tc.wantN {
				t.Errorf("DecodeRecord(% x) n = %d, want %d", tc.in, n, tc.wantN)
			}
		})
	}
}

func TestRecordDecodeRecordAppendToPayload(t *testing.T) {
	defer catchPanic(t)
	// A caller that adds a newline to each payload must not overwrite the
	// next record in the same buffer.
	b := enc("first", "second")
	orig := bytes.Clone(b)
	p, _, err := diskbuffer.DecodeRecord(b)
	if err != nil {
		t.Fatalf("DecodeRecord error = %v, want nil", err)
	}
	_ = append(p, "\n\n\n\n\n\n\n\n"...)
	if !bytes.Equal(b, orig) {
		t.Fatalf("appending to the payload changed the input:\n% x\nwas\n% x\nThe payload's capacity must end where the payload ends.", b, orig)
	}
}

// FuzzRecordDecodeRecord feeds DecodeRecord arbitrary bytes, such as a
// damaged disk could hold. Plain `go test` runs only the seed inputs below;
// `go test -fuzz=FuzzRecord ./exercises/24-diskbuffer/` makes up new ones
// until you press Ctrl-C.
func FuzzRecordDecodeRecord(f *testing.F) {
	f.Add(enc("level=info msg=ok"))
	f.Add([]byte{})
	f.Add([]byte{0, 0, 0})
	f.Add([]byte{0, 0, 0, 0, 0, 0, 0, 0})
	f.Add([]byte{0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0, 'x'})
	f.Add(flip(enc("123456789", "hi"), 3))
	f.Fuzz(func(t *testing.T, b []byte) {
		orig := bytes.Clone(b)
		p, n, err := diskbuffer.DecodeRecord(b)
		if !bytes.Equal(b, orig) {
			t.Fatalf("DecodeRecord changed its input")
		}
		switch {
		case err == nil:
			if n < diskbuffer.HeaderSize || n > len(b) {
				t.Fatalf("DecodeRecord(% x) n = %d, outside [%d, %d]", b, n, diskbuffer.HeaderSize, len(b))
			}
			if !bytes.Equal(p, b[diskbuffer.HeaderSize:n]) {
				t.Fatalf("DecodeRecord(% x) payload = % x, want the %d bytes after the header", b, p, n-diskbuffer.HeaderSize)
			}
			if re := diskbuffer.AppendRecord(nil, p); !bytes.Equal(re, b[:n]) {
				t.Fatalf("AppendRecord(DecodeRecord(% x)) = % x, want the same bytes back", b[:n], re)
			}
		case errors.Is(err, io.EOF):
			if len(b) != 0 {
				t.Fatalf("DecodeRecord(% x) = io.EOF, but the input is not empty", b)
			}
		case errors.Is(err, io.ErrUnexpectedEOF):
		case errors.Is(err, diskbuffer.ErrChecksum):
			if n < diskbuffer.HeaderSize || n > len(b) {
				t.Fatalf("DecodeRecord(% x) = ErrChecksum with n = %d, outside [%d, %d]", b, n, diskbuffer.HeaderSize, len(b))
			}
		default:
			t.Fatalf("DecodeRecord(% x) error = %v, want nil, io.EOF, io.ErrUnexpectedEOF or ErrChecksum", b, err)
		}
	})
}
