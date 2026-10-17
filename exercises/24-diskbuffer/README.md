# 24 · Crash-safe disk buffer for a log shipper

**Tier 5 · about 4.5 h** · Go: `encoding/binary`, `hash/crc32`, `os.File` (`Sync`, `Truncate`, `O_APPEND`), `os.Rename`, full slice expressions, error types with `Is` and `Unwrap` · Problem solving: define the on-disk format first; list every point where a crash can hit; tell a normal crash from real corruption

Every node runs a log shipper (Fluent Bit, Vector, Grafana Alloy) that sends container logs to Loki or
Elasticsearch. When the backend is down for 20 minutes, the shipper buffers logs on disk instead of
dropping them or holding them in memory until the OOM killer comes. The node can lose power in
the middle of a write. After the restart the shipper must send everything that was not
acknowledged, must not invent or reorder records, and must not refuse to start because the last
write was cut in half. But when the disk returns garbage in the middle of the data, it must stop
and say where, instead of silently skipping logs. Prometheus, etcd and Kafka solve this with a
write-ahead log. You build a small one.

## Task

Implement the stubs in `record.go` and `diskbuffer.go`:

```go
// record.go: the record format. HeaderSize (8) and ErrChecksum are given.
func AppendRecord(dst, payload []byte) []byte                  // dst + the encoded record
func DecodeRecord(b []byte) (payload []byte, n int, err error) // the first record of b, its size

// diskbuffer.go: the buffer. Options, the sentinel errors and CorruptError.Error are given.
type Options struct {
	Dir            string // created by Open if missing
	SegmentBytes   int64  // size limit of a segment file
	SyncEveryWrite bool   // fsync in every Append
	MaxRecordBytes int    // largest payload
}
var ErrCorrupt, ErrClosed, ErrTooLarge = ...
type CorruptError struct{ File string; Offset int64; Err error }
func (e *CorruptError) Unwrap() error        // returns Err
func (e *CorruptError) Is(target error) bool // true for ErrCorrupt

func Open(o Options) (*Buffer, error)             // create, or recover after a crash
func (b *Buffer) Append(p []byte) (uint64, error) // write a record, return its seq
func (b *Buffer) ReadFrom(seq uint64, fn func(seq uint64, p []byte) error) error // unacked records from seq on
func (b *Buffer) Ack(seq uint64) error            // records <= seq are delivered
func (b *Buffer) Acked() uint64                   // highest acked seq
func (b *Buffer) Close() error
```

## Rules

### The format on disk

- A **record** is: payload length (4 bytes, big-endian), CRC-32C of the payload (Castagnoli
  polynomial, 4 bytes, big-endian), the payload. `HeaderSize` is 8.
- Every record has a **seq**: 1 for the first record ever, +1 for each record after it.
- A **segment** is a file in `Dir` that holds records back to back. Its name is the seq of its
  first record, 20 digits with leading zeros, plus `.seg`: `00000000000000000001.seg`.
  Segment *i* holds the records from its name up to the seq before the next segment's name.
- The **ack file** `Dir/ack` holds the highest acked seq in decimal and a newline: `42\n`.

### AppendRecord and DecodeRecord

- `AppendRecord` appends the record to `dst` and returns the result (like `append`).
- `DecodeRecord` decodes the first record of `b`, ignores the bytes after it, and returns the
  payload and `n = HeaderSize + len(payload)`. It does not change `b`.
- Errors (the payload is nil on every error):

| `b` | error | `n` |
|---|---|---|
| empty or nil | `io.EOF` | 0 |
| fewer than 8 bytes, or fewer payload bytes than the length says | `io.ErrUnexpectedEOF` | 0 |
| payload does not match the checksum | `ErrChecksum` | 8 + length from the header |

- Check the length against the bytes in `b` before you allocate or slice: a flipped bit can make
  it 4 GiB.
- The payload is a sub-slice of `b` whose capacity ends where the payload ends. Appending to it
  must never overwrite the next record in `b`.

### Open on a new directory

- Invalid options are an error that does not match `ErrCorrupt`: empty `Dir`, `SegmentBytes < 1`,
  `MaxRecordBytes < 1` (or above `math.MaxUint32`).
- Open creates `Dir` and its parents. It creates no file: `Acked()` is 0, there are no records,
  and the directory stays empty until the first Append.

### Append

- Returns the record's seq. The record is in the segment file when Append returns: no buffering
  in memory, a crash of the process must not lose it.
- Rotation: if the current segment is not empty and the record would make it larger than
  `SegmentBytes`, Append first starts a new segment, named after the record's seq. Exactly
  `SegmentBytes` still fits. A record larger than `SegmentBytes` goes into a segment alone.
- Only Append creates segment files, and only when it needs one.
- `len(p) > MaxRecordBytes`: `ErrTooLarge` (match with `errors.Is`); nothing is written and no
  seq is used. `len(p) == MaxRecordBytes` and empty payloads are fine.

### ReadFrom

- Calls `fn(seq, p)` for every record with a seq `>= seq` that is not acked (seq `> Acked()`), in
  seq order, up to the last record appended, across segments. `seq` 0 means from the start. A
  `seq` after the last record: no call, nil error.
- If `fn` returns an error, ReadFrom stops at once and returns that error.
- `p` belongs to `fn`: it may keep it, change it and append to it, without copying. That must not
  change other records or what a later ReadFrom returns, and nothing the Buffer does later may
  change a `p` that `fn` kept (so no read buffer that the Buffer reuses).
- `fn` must not call methods of the same Buffer: ReadFrom holds it until it returns.

### Ack and Acked

- `Ack(seq)` means: every record up to `seq` is delivered. `Acked()` returns it from then on.
- Ack writes a temporary file in `Dir` and renames it to `ack`, so the ack file is replaced
  atomically. No temporary file is left behind.
- `seq <= Acked()`: nothing changes, nil error. `seq` above the last appended record: an error,
  nothing changes.
- Then Ack deletes every segment whose records are all acked, the current segment included. The
  next Append creates a new segment.

### Close

- After Close, `Append`, `ReadFrom` and `Ack` return `ErrClosed` (`fn` is not called), and a
  second `Close` returns `ErrClosed` too. `Acked()` still returns the last value.

### Open on an existing directory: recovery

- `Acked()` is the value in `ack`, or 0 if there is no ack file.
- The next seq is `max(Acked(), seq of the last record on disk) + 1`.
- Append continues writing into the last segment while it has room: its current size counts for
  rotation.
- Segments whose records are all acked are deleted. An older segment (not the last) is deleted
  unread, because the next segment's name says where it ends: damage in it does not matter.
- **A torn tail is a normal crash.** If the last segment ends with a broken record, Open cuts the
  file back to the end of the last intact record. Broken means: a short header, fewer payload
  bytes than the length says (also a length far past the end of the file), or a checksum
  mismatch in a record that ends exactly at the end of the file. The seq of the cut record is
  used again by the next Append. The cut record can be the only one in the segment.
- **Anything else is corruption**: Open returns a `*CorruptError` and changes no file (no cut, no
  delete), so the operator can investigate.

| Damage | `File` | `Offset` | `Err` matches |
|---|---|---|---|
| last segment: checksum mismatch with more data after the record | the segment | start of the record | `ErrChecksum` |
| older segment: any broken record (torn or bad checksum) | the segment | start of the record | the `DecodeRecord` error |
| older segment ends before its range does (a segment is missing) | the segment | its size | `io.ErrUnexpectedEOF` |
| older segment holds more records than its range | the segment | end of the last record in range | (any) |
| first kept segment starts after `Acked()+1` (records are missing) | the segment | 0 | (any) |
| `ack` is not a number | `ack` | 0 | (any) |

"Older" means not the last. `errors.Is(err, ErrCorrupt)` is true for every `*CorruptError`: that
is what its `Is` method is for.

### Concurrency

- All methods are safe for concurrent use: several goroutines append while another reads and acks.
  Seqs stay unique and gapless, and `Acked()` never goes down. Run the tests with `-race`.

### Durability (the tests cannot check this; a reviewer would)

- `SyncEveryWrite`: Append calls `File.Sync` before it returns. Close syncs the open segment.
  Open syncs a segment after it cuts it.
- Sync the old segment before you create the next one: only the last segment may be torn. Sync
  the directory after you create or rename a file in it.
- Ack syncs the temporary file before the rename. The new ack is on disk before Ack deletes a
  segment.
- If a write or sync in Append fails, refuse every later Append; a restart (Open) repairs the
  segment.

## Examples

```go
b, _ := diskbuffer.Open(diskbuffer.Options{Dir: "/var/lib/shipper/buffer", SegmentBytes: 64 << 20, MaxRecordBytes: 1 << 20})
b.Append([]byte("123456789")) // 1
b.Append([]byte("hi"))        // 2
```

```
$ xxd /var/lib/shipper/buffer/00000000000000000001.seg
00000000: 0000 0009 e306 9283 3132 3334 3536 3738  ........12345678
00000010: 3900 0000 02f5 9dd9 c268 69              9........hi
```

Rotation with `SegmentBytes: 84` and records of 28 bytes (20-byte payloads), 8 Appends:

```
00000000000000000001.seg   records 1-3   84 bytes
00000000000000000004.seg   records 4-6   84 bytes
00000000000000000007.seg   records 7-8   56 bytes
Ack(5) -> segment 1 is deleted; segment 4 stays (record 6 is not acked)
Ack(8) -> segments 4 and 7 are deleted too; Dir holds only "ack" ("8\n"); the next Append returns 9
```

The output side of a shipper reads a batch, pushes it, and acks it:

```go
err := b.ReadFrom(b.Acked()+1, func(seq uint64, p []byte) error {
	batch, last = append(batch, p), seq
	if len(batch) == 500 {
		return errBatchFull // stop reading
	}
	return nil
})
// if err is nil or errBatchFull, and push(batch) succeeds: b.Ack(last)
```

Recovery after a crash (no ack file, records of 28 bytes):

```
00000000000000000001.seg  records 1-2
00000000000000000003.seg  records 3-4 + the first 15 bytes of record 5
Open -> segment 3 is cut back to records 3-4; the next Append returns 5

00000000000000000001.seg  record 1, record 2 with a flipped bit in its payload, record 3
Open -> *CorruptError{File: "00000000000000000001.seg", Offset: 28, Err: ErrChecksum}
```

## Run the tests

    go test ./exercises/24-diskbuffer/
    go test -race ./exercises/24-diskbuffer/
    go test -run TestRecoveryTornTail -v ./exercises/24-diskbuffer/

A panicking stub stops the whole test binary, so work one step at a time (the filters do not
overlap). Do step 1 first: the other tests build the expected file contents with your
`AppendRecord`. The comment says what each step needs.

    go test -run 'TestRecord|FuzzRecord' -v ./exercises/24-diskbuffer/               # 1: AppendRecord, DecodeRecord
    go test -run 'TestOpenNew|TestAppend|TestReadFrom' -v ./exercises/24-diskbuffer/ # 2: Open (new Dir), Append, ReadFrom, Acked
    go test -run 'TestAck|TestClose|TestReopen' -v ./exercises/24-diskbuffer/        # 3: + Ack, Close, Open after Close
    go test -run TestRecovery -v ./exercises/24-diskbuffer/                          # 4: + crash recovery, CorruptError
    go test -race -run TestConcurrent -v ./exercises/24-diskbuffer/                  # 5: concurrent use
    go test -fuzz=FuzzRecord ./exercises/24-diskbuffer/   # optional: random input for DecodeRecord, Ctrl-C to stop

## Docs you can use

- [`encoding/binary`](https://pkg.go.dev/encoding/binary): `BigEndian.AppendUint32`, `BigEndian.Uint32`
- [`hash/crc32`](https://pkg.go.dev/hash/crc32): `MakeTable`, `Castagnoli`, `Checksum`
- [`os`](https://pkg.go.dev/os): `OpenFile` with `O_APPEND`, `O_CREATE`, `O_EXCL`; `File.Sync`,
  `File.Truncate`, `Rename`, `ReadFile`, `ReadDir`, `Remove`, `MkdirAll`
- [`io`](https://pkg.go.dev/io): `EOF`, `ErrUnexpectedEOF`
- [`errors`](https://pkg.go.dev/errors): `Is`, `As`, and the paragraph about an `Is` method
- The spec: [Slice expressions](https://go.dev/ref/spec#Slice_expressions) (full slice expressions)
- [Go Slices: usage and internals](https://go.dev/blog/slices-intro)
- [Working with Errors in Go 1.13](https://go.dev/blog/go1.13-errors)
- Linux man pages: [fsync(2)](https://man7.org/linux/man-pages/man2/fsync.2.html) (read the
  paragraph about directories), [rename(2)](https://man7.org/linux/man-pages/man2/rename.2.html)

## Stretch goals

- Group commit: instead of `SyncEveryWrite`, sync at most every 100 ms, and let Append wait for the
  sync that covers its record. Measure records per second for all three modes.
- A size cap: `MaxBytes` plus a policy for a full buffer, `Block` or `DropNewest` (Vector calls it
  `when_full`), and a counter of dropped records you could export to Prometheus.
- A lock file (`syscall.Flock`) so that two processes cannot open the same `Dir`. The tests list
  every file in `Dir`, so make `listFiles` in `helpers_test.go` skip it.
- A safer format: today 8 zero bytes decode as a valid empty record, so a zero-filled tail reads
  as records. Add a record type byte where 0 is invalid, as LevelDB does.

## Stuck?

Try for at least 25 minutes first. Then reveal one level at a time:

    go run ./cmd/peek 24 hint 1
    go run ./cmd/peek 24 hint 2
    go run ./cmd/peek 24 hint 3
    go run ./cmd/peek 24 explanation
    go run ./cmd/peek 24 walkthrough
    go run ./cmd/peek 24 solution
