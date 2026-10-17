// Package diskbuffer is a crash-safe disk buffer for a log shipper. Append
// writes records to segment files, ReadFrom reads them back in order, Ack
// marks them as delivered and deletes the segments that hold only delivered
// records, and Open recovers the buffer after a crash.
package diskbuffer

import (
	"errors"
	"fmt"
)

// Options configure a Buffer.
type Options struct {
	// Dir holds the segment files and the ack file. Open creates it if it
	// does not exist. Must not be empty.
	Dir string

	// SegmentBytes is the size limit of a segment file, at least 1. Append
	// starts a new segment when the next record would make the current one
	// larger than this. A record that is larger on its own gets a segment
	// to itself.
	SegmentBytes int64

	// SyncEveryWrite makes Append call fsync before it returns, so a record
	// survives a power loss as soon as Append returns. Without it, records
	// reach the disk when a segment is rotated or closed, or when the
	// operating system writes them back.
	SyncEveryWrite bool

	// MaxRecordBytes is the largest payload Append accepts: at least 1, at
	// most math.MaxUint32.
	MaxRecordBytes int
}

var (
	// ErrCorrupt matches (with errors.Is) every *CorruptError.
	ErrCorrupt = errors.New("diskbuffer: corrupt data")
	// ErrClosed is returned by calls on a closed Buffer.
	ErrClosed = errors.New("diskbuffer: buffer is closed")
	// ErrTooLarge is returned by Append for a payload larger than
	// Options.MaxRecordBytes.
	ErrTooLarge = errors.New("diskbuffer: record too large")
)

// CorruptError reports damage that Open cannot repair on its own: anything
// but a torn record at the end of the last segment.
type CorruptError struct {
	// File is the name of the damaged file inside Dir, for example
	// "00000000000000000001.seg" or "ack".
	File string
	// Offset is the byte offset in File where the problem starts.
	Offset int64
	// Err says what is wrong, for example ErrChecksum or io.ErrUnexpectedEOF.
	Err error
}

func (e *CorruptError) Error() string {
	return fmt.Sprintf("diskbuffer: corrupt data in %s at offset %d: %v", e.File, e.Offset, e.Err)
}

// Unwrap returns e.Err, so errors.Is(err, ErrChecksum) works.
func (e *CorruptError) Unwrap() error {
	panic("TODO: implement")
}

// Is reports whether target is ErrCorrupt, so errors.Is(err, ErrCorrupt)
// works for every *CorruptError, whatever its Err.
func (e *CorruptError) Is(target error) bool {
	panic("TODO: implement")
}

// Buffer is a crash-safe, append-only record buffer in a directory. All
// methods are safe for concurrent use.
type Buffer struct {
	// TODO: add fields.
}

// Open opens the buffer in o.Dir, creating the directory if needed, and
// recovers what a previous process left there: it reads the ack file and
// the segments, cuts a torn record off the end of the last segment, and
// deletes segments whose records are all acked. Damage that a crash cannot
// cause is a *CorruptError, and then Open changes no file. Invalid options
// are an error too (not a *CorruptError).
func Open(o Options) (*Buffer, error) {
	panic("TODO: implement")
}

// Append writes p as the next record and returns its seq: 1 for the first
// record ever, then one more for each record. The record is written to the
// active segment file before Append returns (and synced, with
// SyncEveryWrite). A payload longer than MaxRecordBytes is rejected with
// ErrTooLarge and uses no seq. After Close, Append returns ErrClosed.
func (b *Buffer) Append(p []byte) (uint64, error) {
	panic("TODO: implement")
}

// ReadFrom calls fn for every record with a seq >= seq that is not acked, in
// seq order, up to the last record appended. If fn returns an error,
// ReadFrom stops and returns that error. fn may keep, change and append to
// p. fn must not call methods of b: ReadFrom holds b until it returns.
// After Close, ReadFrom returns ErrClosed.
func (b *Buffer) ReadFrom(seq uint64, fn func(seq uint64, p []byte) error) error {
	panic("TODO: implement")
}

// Ack records that every record with a seq <= seq is delivered: it writes
// seq to the ack file atomically, then deletes the segments whose records
// are all acked. A seq <= Acked() changes nothing. A seq above the last
// appended record is an error. After Close, Ack returns ErrClosed.
func (b *Buffer) Ack(seq uint64) error {
	panic("TODO: implement")
}

// Acked returns the highest acked seq, 0 if nothing was ever acked. It also
// works after Close.
func (b *Buffer) Acked() uint64 {
	panic("TODO: implement")
}

// Close syncs and closes the open segment file. Calls after Close, Close
// included, return ErrClosed.
func (b *Buffer) Close() error {
	panic("TODO: implement")
}
