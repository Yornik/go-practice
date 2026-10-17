package diskbuffer_test

import (
	"errors"
	"io"
	"maps"
	"testing"

	diskbuffer "github.com/yornik/go-practice/exercises/24-diskbuffer"
)

// Step 4: crash recovery. Each test writes the files that a crashed process
// (or a broken disk) could have left, then calls Open:
//
//	go test -run TestRecovery -v ./exercises/24-diskbuffer/

func TestRecoveryTornTail(t *testing.T) {
	// The process (or the machine) died while Append was writing record 5.
	// Records 1 to 4 are intact; of record 5 there is only a part, or
	// garbage. That is a normal crash: Open cuts the torn record away.
	r5 := enc(logLine(5)) // 28 bytes: header 0-7, payload 8-27
	tests := []struct {
		name string
		torn []byte // what is left of record 5 at the end of the last segment
	}{
		{"short header", r5[:5]},
		{"header only", r5[:8]},
		{"short payload", r5[:20]},
		{"last byte missing", r5[:27]},
		// The file got longer, but the data did not reach the disk.
		{"checksum mismatch in the last record", flip(r5, 27)},
		{"zeros instead of the payload", cat(r5[:8], make([]byte, 20))},
		{"length points far past the end", cat(unhex(t, "ffffffff 00000000"), r5[8:])},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, map[string][]byte{
				segName(1): enc(logLines(1, 2)...),
				segName(3): cat(enc(logLines(3, 4)...), tc.torn),
			})
			w := mustOpen(t, opts(dir))
			w.expectRecords(1, recsFrom(1, logLines(1, 4)...))
			// Record 5 never fully existed, so its seq is free again, and the
			// new record goes right after record 4.
			w.mustAppend(5, "after the crash")
			w.mustClose()
			expectFile(t, dir, segName(1), enc(logLines(1, 2)...))
			expectFile(t, dir, segName(3), enc(logLine(3), logLine(4), "after the crash"))
		})
	}

	t.Run("the last segment holds only a torn record", func(t *testing.T) {
		// The crash came right after a rotation: segment 3 has only part of
		// its first record.
		dir := t.TempDir()
		writeFiles(t, dir, map[string][]byte{
			segName(1): enc(logLines(1, 2)...),
			segName(3): r5[:10],
		})
		w := mustOpen(t, opts(dir))
		w.expectRecords(1, recsFrom(1, logLines(1, 2)...))
		w.mustAppend(3, "after the crash")
		w.mustClose()
		expectFile(t, dir, segName(3), enc("after the crash"))
	})
}

func TestRecoveryCorruption(t *testing.T) {
	// Damage that a crash during Append cannot cause: a bad record with good
	// data after it, damage in a segment that is not the last, missing
	// records. Open must refuse to start, say where the damage is, and leave
	// every file as it is.
	r := func(i int) []byte { return enc(logLine(i)) } // record i, 28 bytes
	tests := []struct {
		name       string
		files      map[string][]byte
		wantFile   string
		wantOffset int64
		wantErr    error // also matched with errors.Is, if not nil
	}{
		{
			name: "bad checksum in the middle of the last segment",
			files: map[string][]byte{
				segName(1): cat(r(1), flip(r(2), 20), r(3)),
			},
			wantFile: segName(1), wantOffset: 28, wantErr: diskbuffer.ErrChecksum,
		},
		{
			name: "bad checksum in an older segment",
			files: map[string][]byte{
				segName(1): cat(r(1), flip(r(2), 20), r(3)),
				segName(4): r(4),
			},
			wantFile: segName(1), wantOffset: 28, wantErr: diskbuffer.ErrChecksum,
		},
		{
			name: "bad checksum in the last record of an older segment",
			files: map[string][]byte{
				segName(1): cat(r(1), r(2), flip(r(3), 27)),
				segName(4): r(4),
			},
			wantFile: segName(1), wantOffset: 56, wantErr: diskbuffer.ErrChecksum,
		},
		{
			name: "torn record at the end of an older segment",
			files: map[string][]byte{
				segName(1): cat(r(1), r(2), r(3)[:10]),
				segName(4): r(4),
			},
			wantFile: segName(1), wantOffset: 56, wantErr: io.ErrUnexpectedEOF,
		},
		{
			// Segment 1 must hold records 1 to 6, because the next segment
			// starts at 7. Records 4 to 6 are missing: segment 4 was deleted.
			name: "a segment is missing",
			files: map[string][]byte{
				segName(1): cat(r(1), r(2), r(3)),
				segName(7): r(7),
			},
			wantFile: segName(1), wantOffset: 84, wantErr: io.ErrUnexpectedEOF,
		},
		{
			// Segment 1 must hold only records 1 to 3, because the next
			// segment starts at 4.
			name: "an older segment holds too many records",
			files: map[string][]byte{
				segName(1): cat(r(1), r(2), r(3), r(4)),
				segName(4): r(4),
			},
			wantFile: segName(1), wantOffset: 84,
		},
		{
			// Nothing is acked, so record 1 must be on disk. (The torn tail
			// of segment 4 must stay as it is too: Open changes nothing when
			// it reports corruption.)
			name: "records missing before the first segment",
			files: map[string][]byte{
				segName(4): cat(r(4), r(5)[:10]),
			},
			wantFile: segName(4), wantOffset: 0,
		},
		{
			name: "records missing after the acked seq",
			files: map[string][]byte{
				"ack":      []byte("1\n"),
				segName(3): cat(r(3), r(4)),
			},
			wantFile: segName(3), wantOffset: 0,
		},
		{
			// Segment 1 is acked and could go, but Open finds the damage in
			// segment 4 first. It must not delete segment 1 or cut the torn
			// tail of segment 7 before it knows that it can start.
			name: "damage after an acked segment",
			files: map[string][]byte{
				"ack":      []byte("3\n"),
				segName(1): cat(r(1), r(2), r(3)),
				segName(4): cat(r(4), flip(r(5), 9), r(6)),
				segName(7): cat(r(7), r(8)[:10]),
			},
			wantFile: segName(4), wantOffset: 28, wantErr: diskbuffer.ErrChecksum,
		},
		{
			name: "ack file is not a number",
			files: map[string][]byte{
				"ack":      []byte("banana\n"),
				segName(1): r(1),
			},
			wantFile: "ack", wantOffset: 0,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, tc.files)
			before := snapshot(t, dir)

			_, err := open(t, opts(dir))
			if !errors.Is(err, diskbuffer.ErrCorrupt) {
				t.Fatalf("Open error = %v, want an error that matches ErrCorrupt", err)
			}
			var ce *diskbuffer.CorruptError
			if !errors.As(err, &ce) {
				t.Fatalf("Open error = %v (%T), want a *CorruptError", err, err)
			}
			if ce.File != tc.wantFile || ce.Offset != tc.wantOffset {
				t.Errorf("CorruptError points at %s offset %d, want %s offset %d (error: %v)",
					ce.File, ce.Offset, tc.wantFile, tc.wantOffset, err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("Open error = %v, want it to wrap %v", err, tc.wantErr)
			}
			if after := snapshot(t, dir); !maps.Equal(before, after) {
				t.Errorf("Open changed files although it returned an error:\nbefore %q\nafter  %q", before, after)
			}
		})
	}
}

func TestRecoveryDeletesAckedSegments(t *testing.T) {
	// The process died inside Ack: after the ack file was written, before
	// the segments were deleted. Open finishes the job.
	r := func(from, to int) []byte { return enc(logLines(from, to)...) }

	t.Run("some segments acked", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string][]byte{
			"ack":      []byte("5\n"),
			segName(1): r(1, 3),
			segName(4): r(4, 6),
			segName(7): r(7, 8),
		})
		w := mustOpen(t, opts(dir))
		expectFiles(t, dir, "ack", segName(4), segName(7))
		if got := w.Acked(); got != 5 {
			t.Fatalf("Acked() = %d, want 5", got)
		}
		w.expectRecords(1, recsFrom(6, logLines(6, 8)...))
		w.mustAppend(9, logLine(9))
	})

	t.Run("an acked segment is damaged", func(t *testing.T) {
		// Segment 1 holds only acked records, so Open deletes it without
		// reading it. Its damage does not matter any more.
		dir := t.TempDir()
		writeFiles(t, dir, map[string][]byte{
			"ack":      []byte("3\n"),
			segName(1): flip(r(1, 3), 30),
			segName(4): r(4, 5),
		})
		w := mustOpen(t, opts(dir))
		expectFiles(t, dir, "ack", segName(4))
		w.expectRecords(1, recsFrom(4, logLines(4, 5)...))
	})

	t.Run("everything acked", func(t *testing.T) {
		dir := t.TempDir()
		writeFiles(t, dir, map[string][]byte{
			"ack":      []byte("4\n"),
			segName(1): r(1, 3),
			segName(4): r(4, 4),
		})
		w := mustOpen(t, opts(dir))
		expectFiles(t, dir, "ack")
		w.expectRecords(1, nil)
		w.mustAppend(5, logLine(5))
		expectFiles(t, dir, "ack", segName(5))
	})
}

func TestRecoveryAckAheadOfData(t *testing.T) {
	// With SyncEveryWrite false, records 4 to 10 were still in the page
	// cache when the shipper read, sent and acked them. Then the power
	// failed, and they never reached the disk. The ack file (synced) says
	// 10, the segment holds only records 1 to 3.
	//
	// If the next Append reused seq 4, the ack would already cover it, and
	// that record would never be sent. Seqs continue after the ack.
	dir := t.TempDir()
	writeFiles(t, dir, map[string][]byte{
		"ack":      []byte("10\n"),
		segName(1): enc(logLines(1, 3)...),
	})
	w := mustOpen(t, opts(dir))
	if got := w.Acked(); got != 10 {
		t.Fatalf("Acked() = %d, want 10", got)
	}
	w.expectRecords(1, nil)
	w.mustAppend(11, logLine(11))
	expectFiles(t, dir, "ack", segName(11))
	w.expectRecords(1, recsFrom(11, logLine(11)))
}
