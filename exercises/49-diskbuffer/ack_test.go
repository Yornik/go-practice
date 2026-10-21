package diskbuffer_test

import (
	"errors"
	"testing"

	diskbuffer "github.com/yornik/go-practice/exercises/49-diskbuffer"
)

// Step 3: Ack, Close, and Open on a directory that a clean shutdown left
// behind:
//
//	go test -run 'TestAck|TestClose|TestReopen' -v ./exercises/49-diskbuffer/

func TestAckPersists(t *testing.T) {
	dir := t.TempDir()
	w := mustOpen(t, opts(dir))
	w.mustAppend(1, logLines(1, 3)...)

	w.mustAck(2)
	if got := w.Acked(); got != 2 {
		t.Fatalf("after Ack(2): Acked() = %d, want 2", got)
	}
	// The ack file holds the seq in decimal and a newline. Ack writes a
	// temporary file and renames it to "ack", and leaves no temporary file.
	expectFile(t, dir, "ack", []byte("2\n"))
	expectFiles(t, dir, "ack", segName(1))

	t.Run("an older or equal seq changes nothing", func(t *testing.T) {
		for _, seq := range []uint64{0, 1, 2} {
			if err := w.Ack(seq); err != nil {
				t.Fatalf("Ack(%d) after Ack(2) error = %v, want nil", seq, err)
			}
			if got := w.Acked(); got != 2 {
				t.Fatalf("Ack(%d) after Ack(2): Acked() = %d, want still 2", seq, got)
			}
		}
		expectFile(t, dir, "ack", []byte("2\n"))
	})

	t.Run("a seq that was never appended is an error", func(t *testing.T) {
		if err := w.Ack(4); err == nil {
			t.Fatalf("Ack(4) with 3 records error = nil, want an error")
		}
		if got := w.Acked(); got != 2 {
			t.Fatalf("after the failed Ack(4): Acked() = %d, want still 2", got)
		}
		expectFile(t, dir, "ack", []byte("2\n"))
	})
}

func TestAckReadFromSkipsAcked(t *testing.T) {
	w := mustOpen(t, opts(t.TempDir()))
	w.mustAppend(1, logLines(1, 5)...)
	w.mustAck(2)
	// Acked records are delivered: ReadFrom never returns them again, even
	// when the caller asks for them and their segment is still on disk.
	w.expectRecords(1, recsFrom(3, logLines(3, 5)...))
	w.expectRecords(3, recsFrom(3, logLines(3, 5)...))
	w.expectRecords(4, recsFrom(4, logLines(4, 5)...))
}

func TestAckDeletesSegments(t *testing.T) {
	dir := t.TempDir()
	w := mustOpen(t, diskbuffer.Options{Dir: dir, SegmentBytes: 3 * recSize, MaxRecordBytes: 100})
	w.mustAppend(1, logLines(1, 8)...)
	expectFiles(t, dir, segName(1), segName(4), segName(7))

	steps := []struct {
		ack  uint64
		want []string
	}{
		{2, []string{"ack", segName(1), segName(4), segName(7)}}, // record 3 is not acked
		{3, []string{"ack", segName(4), segName(7)}},             // segment 1 is done
		{5, []string{"ack", segName(4), segName(7)}},
		// The last segment goes too once all its records are acked.
		{8, []string{"ack"}},
	}
	for _, s := range steps {
		w.mustAck(s.ack)
		t.Logf("after Ack(%d)", s.ack)
		expectFiles(t, dir, s.want...)
	}
	// The next Append creates a new segment, named after its seq.
	w.mustAppend(9, logLine(9))
	expectFiles(t, dir, "ack", segName(9))
	expectFile(t, dir, segName(9), enc(logLine(9)))
	w.expectRecords(1, recsFrom(9, logLine(9)))
}

func TestCloseRejectsCalls(t *testing.T) {
	dir := t.TempDir()
	w := mustOpen(t, opts(dir))
	w.mustAppend(1, logLines(1, 2)...)
	w.mustAck(1)
	w.mustClose()

	if seq, err := w.Append("late"); !errors.Is(err, diskbuffer.ErrClosed) {
		t.Errorf("Append after Close = %d, %v; want ErrClosed", seq, err)
	}
	called := false
	if err := w.ReadFrom(1, func(uint64, []byte) error { called = true; return nil }); !errors.Is(err, diskbuffer.ErrClosed) {
		t.Errorf("ReadFrom after Close error = %v, want ErrClosed", err)
	}
	if called {
		t.Errorf("ReadFrom after Close called fn, want no call")
	}
	if err := w.Ack(2); !errors.Is(err, diskbuffer.ErrClosed) {
		t.Errorf("Ack after Close error = %v, want ErrClosed", err)
	}
	if err := w.Close(); !errors.Is(err, diskbuffer.ErrClosed) {
		t.Errorf("second Close error = %v, want ErrClosed", err)
	}
	if got := w.Acked(); got != 1 {
		t.Errorf("Acked after Close = %d, want 1 (the last value)", got)
	}
	// Nothing was lost.
	w2 := mustOpen(t, opts(dir))
	w2.expectRecords(1, recsFrom(2, logLine(2)))
}

func TestReopenContinues(t *testing.T) {
	dir := t.TempDir()
	o := diskbuffer.Options{Dir: dir, SegmentBytes: 3 * recSize, MaxRecordBytes: 100}
	w := mustOpen(t, o)
	w.mustAppend(1, logLines(1, 5)...) // segments 1 (1-3) and 4 (4-5)
	w.mustAck(2)
	w.mustClose()

	w = mustOpen(t, o)
	if got := w.Acked(); got != 2 {
		t.Fatalf("after reopen: Acked() = %d, want 2", got)
	}
	w.expectRecords(1, recsFrom(3, logLines(3, 5)...))
	// Seqs continue, and Append continues to write into the last segment
	// while it has room: segment 4 holds 2 records, so 1 more fits.
	w.mustAppend(6, logLine(6), logLine(7))
	expectFiles(t, dir, "ack", segName(1), segName(4), segName(7))
	expectFile(t, dir, segName(4), enc(logLines(4, 6)...))
	expectFile(t, dir, segName(7), enc(logLine(7)))
	w.expectRecords(3, recsFrom(3, logLines(3, 7)...))
}

func TestReopenAfterEverythingAcked(t *testing.T) {
	dir := t.TempDir()
	w := mustOpen(t, opts(dir))
	w.mustAppend(1, logLines(1, 3)...)
	w.mustAck(3)
	w.mustClose()
	expectFiles(t, dir, "ack")

	// No segment is left, so only the ack file knows where the seqs were.
	w = mustOpen(t, opts(dir))
	if got := w.Acked(); got != 3 {
		t.Fatalf("after reopen: Acked() = %d, want 3", got)
	}
	w.expectRecords(1, nil)
	w.mustAppend(4, logLine(4))
	expectFiles(t, dir, "ack", segName(4))
}
