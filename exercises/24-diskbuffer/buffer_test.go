package diskbuffer_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	diskbuffer "github.com/yornik/go-practice/exercises/24-diskbuffer"
)

// Step 2: write and read. Open on a new directory, Append, ReadFrom,
// segment files and rotation:
//
//	go test -run 'TestOpenNew|TestAppend|TestReadFrom' -v ./exercises/24-diskbuffer/

func TestOpenNewRejectsBadOptions(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name string
		o    diskbuffer.Options
	}{
		{"empty Dir", diskbuffer.Options{Dir: "", SegmentBytes: 1024, MaxRecordBytes: 100}},
		{"SegmentBytes 0", diskbuffer.Options{Dir: dir, SegmentBytes: 0, MaxRecordBytes: 100}},
		{"SegmentBytes negative", diskbuffer.Options{Dir: dir, SegmentBytes: -1, MaxRecordBytes: 100}},
		{"MaxRecordBytes 0", diskbuffer.Options{Dir: dir, SegmentBytes: 1024, MaxRecordBytes: 0}},
		{"MaxRecordBytes negative", diskbuffer.Options{Dir: dir, SegmentBytes: 1024, MaxRecordBytes: -5}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := open(t, tc.o)
			if err == nil {
				t.Fatalf("Open(%+v) error = nil, want an error", tc.o)
			}
			if errors.Is(err, diskbuffer.ErrCorrupt) {
				t.Fatalf("Open(%+v) error = %v; bad options are not corruption, do not use ErrCorrupt", tc.o, err)
			}
		})
	}
}

func TestOpenNewCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "spool", "buffer") // does not exist yet
	w := mustOpen(t, opts(dir))
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("after Open, os.Stat(Dir) = %v, %v; want a directory", info, err)
	}
	if got := w.Acked(); got != 0 {
		t.Errorf("Acked() of a new buffer = %d, want 0", got)
	}
	w.expectRecords(1, nil)
	// Open creates no files: Append creates the first segment.
	expectFiles(t, dir)
}

func TestAppendAndReadFrom(t *testing.T) {
	for _, sync := range []bool{false, true} {
		name := "SyncEveryWrite=false"
		if sync {
			name = "SyncEveryWrite=true"
		}
		t.Run(name, func(t *testing.T) {
			o := opts(t.TempDir())
			o.SyncEveryWrite = sync
			w := mustOpen(t, o)
			payloads := []string{`level=info msg="started"`, `level=error msg="db timeout"`, ""}
			w.mustAppend(1, payloads...) // seqs start at 1 and go up by 1
			all := recsFrom(1, payloads...)
			w.expectRecords(1, all)
			w.expectRecords(0, all) // nothing has seq 0; start at the first record
			w.expectRecords(2, all[1:])
			w.expectRecords(3, all[2:])
			w.expectRecords(4, nil) // after the last record: nothing, no error
			w.expectRecords(1000, nil)
		})
	}
}

func TestAppendWritesSegmentFile(t *testing.T) {
	dir := t.TempDir()
	w := mustOpen(t, opts(dir))
	w.mustAppend(1, "123456789", "hi")
	// The records are in the file as soon as Append returns, not only after
	// Close: a crash of the process must not lose a record that Append
	// accepted.
	expectFiles(t, dir, segName(1))
	expectFile(t, dir, "00000000000000000001.seg", unhex(t,
		"00000009 e3069283 313233343536373839"+ // "123456789"
			"00000002 f59dd9c2 6869")) // "hi"
}

func TestAppendRotation(t *testing.T) {
	// Every logLine record is recSize (28) bytes. A record goes into the
	// current segment if the segment stays <= SegmentBytes with it;
	// otherwise Append starts a new segment, named after the record's seq.
	tests := []struct {
		name         string
		segmentBytes int64
		wantSegments map[uint64][]string // first seq -> payloads in that segment
	}{
		{"exactly 3 records fit", 3 * recSize, map[uint64][]string{
			1: logLines(1, 3), 4: logLines(4, 6), 7: logLines(7, 7),
		}},
		{"1 byte short of 3 records", 3*recSize - 1, map[uint64][]string{
			1: logLines(1, 2), 3: logLines(3, 4), 5: logLines(5, 6), 7: logLines(7, 7),
		}},
		{"1 byte more than 3 records", 3*recSize + 1, map[uint64][]string{
			1: logLines(1, 3), 4: logLines(4, 6), 7: logLines(7, 7),
		}},
		// A record larger than SegmentBytes still goes into a segment, alone.
		{"every record larger than a segment", 1, map[uint64][]string{
			1: logLines(1, 1), 2: logLines(2, 2), 3: logLines(3, 3), 4: logLines(4, 4),
			5: logLines(5, 5), 6: logLines(6, 6), 7: logLines(7, 7),
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			w := mustOpen(t, diskbuffer.Options{Dir: dir, SegmentBytes: tc.segmentBytes, MaxRecordBytes: 100})
			w.mustAppend(1, logLines(1, 7)...)
			var names []string
			for first := range tc.wantSegments {
				names = append(names, segName(first))
			}
			expectFiles(t, dir, names...)
			for first, payloads := range tc.wantSegments {
				expectFile(t, dir, segName(first), enc(payloads...))
			}
			w.expectRecords(1, recsFrom(1, logLines(1, 7)...))
		})
	}
}

func TestAppendRecordLargerThanSegment(t *testing.T) {
	dir := t.TempDir()
	w := mustOpen(t, diskbuffer.Options{Dir: dir, SegmentBytes: 3 * recSize, MaxRecordBytes: 1000})
	big := strings.Repeat("x", 500) // a stack trace, say
	w.mustAppend(1, logLine(1), logLine(2), big, logLine(4))
	// The big record does not fit after records 1 and 2, so it starts
	// segment 3. It is alone there, because record 4 does not fit after it.
	expectFiles(t, dir, segName(1), segName(3), segName(4))
	expectFile(t, dir, segName(1), enc(logLine(1), logLine(2)))
	expectFile(t, dir, segName(3), enc(big))
	expectFile(t, dir, segName(4), enc(logLine(4)))
	w.expectRecords(1, recsFrom(1, logLine(1), logLine(2), big, logLine(4)))
}

func TestAppendTooLarge(t *testing.T) {
	dir := t.TempDir()
	w := mustOpen(t, diskbuffer.Options{Dir: dir, SegmentBytes: 1024, MaxRecordBytes: 20})
	w.mustAppend(1, logLine(1)) // exactly 20 bytes: allowed
	tooBig := logLine(2) + "!"  // 21 bytes
	if seq, err := w.Append(tooBig); !errors.Is(err, diskbuffer.ErrTooLarge) {
		t.Fatalf("Append(%d bytes) with MaxRecordBytes 20 = %d, %v; want ErrTooLarge", len(tooBig), seq, err)
	}
	// The rejected record used no seq and wrote nothing.
	w.mustAppend(2, logLine(2))
	expectFile(t, dir, segName(1), enc(logLine(1), logLine(2)))
	w.expectRecords(1, recsFrom(1, logLine(1), logLine(2)))
}

func TestReadFromAcrossSegments(t *testing.T) {
	w := mustOpen(t, diskbuffer.Options{Dir: t.TempDir(), SegmentBytes: 3 * recSize, MaxRecordBytes: 100})
	w.mustAppend(1, logLines(1, 8)...) // segments 1 (1-3), 4 (4-6), 7 (7-8)
	for _, from := range []uint64{1, 2, 4, 5, 7, 8} {
		w.expectRecords(from, recsFrom(from, logLines(int(from), 8)...))
	}
	w.expectRecords(9, nil)
}

func TestReadFromCallbackError(t *testing.T) {
	w := mustOpen(t, diskbuffer.Options{Dir: t.TempDir(), SegmentBytes: 3 * recSize, MaxRecordBytes: 100})
	w.mustAppend(1, logLines(1, 6)...)
	// A shipper stops reading when its batch is full.
	var seen []uint64
	err := w.ReadFrom(2, func(seq uint64, _ []byte) error {
		seen = append(seen, seq)
		if seq == 4 {
			return errStop
		}
		return nil
	})
	if !errors.Is(err, errStop) {
		t.Fatalf("ReadFrom(2) error = %v, want the error fn returned (%v)", err, errStop)
	}
	if want := []uint64{2, 3, 4}; !slices.Equal(seen, want) {
		t.Fatalf("ReadFrom(2) called fn for seqs %v, want %v and then stop", seen, want)
	}
}

func TestReadFromCallbackOwnsPayload(t *testing.T) {
	// Small segments: the 6 records come from 3 segment files.
	w := mustOpen(t, diskbuffer.Options{Dir: t.TempDir(), SegmentBytes: 2 * recSize, MaxRecordBytes: 100})
	w.mustAppend(1, logLines(1, 6)...)
	// fn keeps every payload without copying it, changes it, and also keeps
	// a version with a newline appended. That is allowed: p belongs to fn.
	var kept, lines [][]byte
	err := w.ReadFrom(1, func(_ uint64, p []byte) error {
		p[0] = 'L'
		kept = append(kept, p)
		lines = append(lines, append(p, '\n'))
		return nil
	})
	if err != nil {
		t.Fatalf("ReadFrom(1) error = %v, want nil", err)
	}
	// What fn did to its payloads does not change the buffer.
	w.expectRecords(1, recsFrom(1, logLines(1, 6)...))
	// Neither the later records nor the second ReadFrom changed what fn
	// kept: a read buffer that the Buffer reuses would.
	for i := range kept {
		want := "L" + logLine(i + 1)[1:]
		if string(kept[i]) != want || string(lines[i]) != want+"\n" {
			t.Errorf("payload %d kept by fn = %q (with newline: %q), want %q: later reads overwrote it",
				i+1, kept[i], lines[i], want)
		}
	}
}
