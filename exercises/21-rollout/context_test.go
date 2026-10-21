package rollout_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	rollout "github.com/yornik/go-practice/exercises/21-rollout"
)

// Step 4: the context.

func TestWaitContextCanceledWhileWaiting(t *testing.T) {
	defer catchPanic(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	polled := make(chan struct{}, 1)
	p := &poller{
		script: []result{ok(3, 1, 1)},
		hook:   func(context.Context, int) { polled <- struct{}{} },
	}

	// The test sends no tick: Wait polls once and then waits for a tick
	// until the test cancels ctx.
	c := start(t, ctx, rollout.Options{Poll: p.Poll, StallTicks: 3, MaxErrors: 1})
	select {
	case <-polled:
	case <-time.After(timeout):
		t.Fatalf("Wait did not poll within %v: it must poll right away, without a tick", timeout)
	}
	cancel()
	got, err := c.result()

	if !errors.Is(err, context.Canceled) || got != st(3, 1, 1) {
		t.Errorf("cancel after the first poll %v:\nWait = %+v, %v\nwant %+v and an error that wraps context.Canceled", p.describe(), got, err, st(3, 1, 1))
	}
	if n := p.Calls(); n != 1 {
		t.Errorf("Poll was called %d times, want 1", n)
	}
}

func TestWaitContextDoneBeforeFirstPoll(t *testing.T) {
	expired, cancel1 := context.WithDeadline(context.Background(), time.Unix(0, 0))
	defer cancel1()
	canceled, cancel2 := context.WithCancel(context.Background())
	cancel2()

	tests := []struct {
		name    string
		ctx     context.Context
		wantErr error
	}{
		{"deadline in 1970", expired, context.DeadlineExceeded},
		{"canceled", canceled, context.Canceled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			p := &poller{script: []result{ok(3, 3, 3)}}
			got, err, _ := runWait(t, tc.ctx, rollout.Options{Poll: p.Poll})

			if !errors.Is(err, tc.wantErr) || got != (rollout.Status{}) {
				t.Errorf("ctx done before Wait:\nWait = %+v, %v\nwant the zero Status and an error that wraps %v", got, err, tc.wantErr)
			}
			if n := p.Calls(); n != 0 {
				t.Errorf("Poll was called %d times, want 0: Wait must check ctx before every poll, also the first", n)
			}
		})
	}
}

type ctxKey struct{}

func TestWaitContextPassedToPoll(t *testing.T) {
	defer catchPanic(t)
	ctx := context.WithValue(context.Background(), ctxKey{}, "inference-api")
	var (
		mu  sync.Mutex
		got []any
	)
	p := &poller{
		script: []result{ok(3, 1, 1), ok(3, 3, 3)},
		hook: func(ctx context.Context, n int) {
			mu.Lock()
			defer mu.Unlock()
			if ctx == nil {
				got = append(got, nil)
				return
			}
			got = append(got, ctx.Value(ctxKey{}))
		},
	}
	runWait(t, ctx, rollout.Options{Poll: p.Poll})

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 {
		t.Fatalf("Poll was called %d times, want 2", len(got))
	}
	for i, v := range got {
		if v != "inference-api" {
			t.Errorf("poll %d: ctx.Value(key) = %v, want %q: pass the ctx of Wait (or one derived from it) to Poll", i+1, v, "inference-api")
		}
	}
}

func TestWaitContextCanceledDuringPoll(t *testing.T) {
	// Ticks has a buffer full of ticks, so after poll 2 both a tick and
	// ctx.Done() are ready, and select picks one of them at random. The
	// test repeats the scenario so that a missing check before a poll shows
	// up in almost every run.
	for range 20 {
		func() {
			defer catchPanic(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := &poller{
				script: []result{ok(3, 1, 1), ok(3, 2, 2), ok(3, 3, 3)},
				hook: func(_ context.Context, n int) {
					if n == 2 {
						cancel()
					}
				},
			}
			ticks := make(chan time.Time, 3)
			for i := range cap(ticks) {
				ticks <- tick0.Add(time.Duration(i+1) * 5 * time.Second)
			}
			got, err := startRaw(t, ctx, rollout.Options{Poll: p.Poll, Ticks: ticks}).result()

			if !errors.Is(err, context.Canceled) || got != st(3, 2, 2) {
				t.Fatalf("ctx canceled during poll 2 of %v:\nWait = %+v, %v\nwant %+v and an error that wraps context.Canceled", p.describe(), got, err, st(3, 2, 2))
			}
			if n := p.Calls(); n != 2 {
				t.Fatalf("Poll was called %d times, want 2: Wait must not poll once ctx is done, also when a tick is ready", n)
			}
		}()
	}
}
