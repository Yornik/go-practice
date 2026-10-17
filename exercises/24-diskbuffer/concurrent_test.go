package diskbuffer_test

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	diskbuffer "github.com/yornik/go-practice/exercises/24-diskbuffer"
)

// Step 5: concurrent use. Run it with the race detector:
//
//	go test -race -run TestConcurrent -v ./exercises/24-diskbuffer/

func TestConcurrentAppendReadAck(t *testing.T) {
	// A log shipper: 3 inputs append, the output reads batches from the
	// oldest unacked record, "sends" them and acks them, and a metrics
	// goroutine reads Acked. The segments are small, so rotation and
	// deletion happen all the time while the others run.
	const (
		inputs   = 3
		perInput = 40
		total    = inputs * perInput
		batch    = 10
	)
	dir := t.TempDir()
	w := mustOpen(t, diskbuffer.Options{Dir: dir, SegmentBytes: 4 * recSize, MaxRecordBytes: 100})
	b := w.b

	// Each input remembers which seq each of its records got.
	var mu sync.Mutex
	appended := map[uint64]string{}
	var inputsDone sync.WaitGroup
	inputErrs := make(chan error, inputs)
	for in := range inputs {
		inputsDone.Add(1)
		go func() {
			defer inputsDone.Done()
			for i := range perInput {
				p := fmt.Sprintf("input %d line %07d", in, i) // 20 bytes
				seq, err := b.Append([]byte(p))
				if err != nil {
					inputErrs <- fmt.Errorf("Append(%q) error = %v", p, err)
					return
				}
				mu.Lock()
				if prev, dup := appended[seq]; dup {
					mu.Unlock()
					inputErrs <- fmt.Errorf("Append(%q) = %d, but Append(%q) got that seq too", p, seq, prev)
					return
				}
				appended[seq] = p
				mu.Unlock()
			}
		}()
	}

	stopMetrics := make(chan struct{})
	stop := sync.OnceFunc(func() { close(stopMetrics) })
	defer stop() // also when the test fails early
	metricsErr := make(chan error, 1)
	go func() {
		var last uint64
		for {
			select {
			case <-stopMetrics:
				metricsErr <- nil
				return
			default:
			}
			a := b.Acked()
			if a < last {
				metricsErr <- fmt.Errorf("Acked() went down from %d to %d", last, a)
				return
			}
			last = a
			time.Sleep(100 * time.Microsecond)
		}
	}()

	// The output. It acks only full batches (and the rest at the end), like a
	// shipper that sends batches of 10. The loop gets more time than a single
	// call, because rotations and acks fsync, and fsync is slow on some disks.
	const loopTimeout = 30 * time.Second
	var delivered []rec
	var outErr error
	errFull := errors.New("batch full")
	withinFor(t, "the output loop (ReadFrom and Ack while the inputs append)", loopTimeout, func() {
		outErr = func() error {
			deadline := time.Now().Add(loopTimeout - time.Second)
			for uint64(len(delivered)) < total {
				if time.Now().After(deadline) {
					return fmt.Errorf("only %d of %d records delivered before the deadline", len(delivered), total)
				}
				from := b.Acked() + 1
				var got []rec
				err := b.ReadFrom(from, func(seq uint64, p []byte) error {
					got = append(got, rec{Seq: seq, Payload: string(p)})
					if len(got) == batch {
						return errFull
					}
					return nil
				})
				if err != nil && !errors.Is(err, errFull) {
					return fmt.Errorf("ReadFrom(%d) error = %v", from, err)
				}
				for i, r := range got {
					if want := from + uint64(i); r.Seq != want {
						return fmt.Errorf("ReadFrom(%d) delivered seq %d as record %d of the batch, want %d", from, r.Seq, i, want)
					}
				}
				if len(got) < batch && uint64(len(delivered)+len(got)) < total {
					time.Sleep(200 * time.Microsecond) // wait for a full batch
					continue
				}
				if err := b.Ack(got[len(got)-1].Seq); err != nil {
					return fmt.Errorf("Ack(%d) error = %v", got[len(got)-1].Seq, err)
				}
				delivered = append(delivered, got...)
			}
			return nil
		}()
	})
	if outErr != nil {
		t.Fatal(outErr)
	}
	within(t, "the inputs", inputsDone.Wait)
	close(inputErrs)
	for err := range inputErrs {
		t.Error(err)
	}
	stop()
	select {
	case err := <-metricsErr:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(timeout):
		t.Fatal("the metrics goroutine did not stop: Acked() blocks?")
	}

	// Every record was delivered exactly once, under the seq Append returned.
	for i, r := range delivered {
		if want := uint64(i + 1); r.Seq != want {
			t.Fatalf("delivered record %d has seq %d, want %d", i, r.Seq, want)
		}
		if r.Payload != appended[r.Seq] {
			t.Fatalf("record %d = %q, but Append returned %d for %q", r.Seq, r.Payload, r.Seq, appended[r.Seq])
		}
	}
	if got := w.Acked(); got != total {
		t.Errorf("Acked() = %d, want %d", got, total)
	}
	// Everything is acked, so every segment is deleted.
	expectFiles(t, dir, "ack")
}
