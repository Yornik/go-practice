package diskbuffer

import "errors"

// HeaderSize is the size of a record header: the payload length and the
// CRC-32C of the payload, 4 bytes each, big-endian.
const HeaderSize = 8

// ErrChecksum means that a record's payload does not match the CRC-32C in
// its header.
var ErrChecksum = errors.New("diskbuffer: checksum mismatch")

// AppendRecord appends the encoding of payload to dst and returns the
// extended slice: the payload length (4 bytes, big-endian), the CRC-32C
// (Castagnoli) of the payload (4 bytes, big-endian), then the payload.
// payload must not be longer than math.MaxUint32 bytes; AppendRecord panics
// if it is.
func AppendRecord(dst, payload []byte) []byte {
	panic("TODO: implement")
}

// DecodeRecord decodes the record at the start of b. It returns the payload
// and n, the number of bytes the record takes in b (HeaderSize plus the
// payload length). Bytes after the record are ignored.
//
// Errors: io.EOF if b is empty; io.ErrUnexpectedEOF if b holds only part of a
// record (a short header, or fewer payload bytes than the header says);
// ErrChecksum if the payload does not match the checksum. On error the
// payload is nil, and n is 0, except for ErrChecksum: then n is still the
// record size from the header, so the caller can see where the bad record
// ends.
//
// The payload is a sub-slice of b, not a copy, and its capacity ends where
// the payload ends, so appending to it never writes into b.
func DecodeRecord(b []byte) (payload []byte, n int, err error) {
	panic("TODO: implement")
}
