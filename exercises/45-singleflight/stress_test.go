package singleflight_test

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// failures collects errors from many goroutines and reports the first few,
// so a broken solution does not print thousands of lines.
type failures struct {
	t *testing.T
	n atomic.Int32
}

func (f *failures) add(format string, args ...any) {
	if f.n.Add(1) <= 5 {
		f.t.Errorf(format, args...) // Errorf is safe to call from any goroutine
	}
}

// hammer runs work(worker, i) for i in [0, iterations) in each of workers
// goroutines, all at once, and waits for them. A panic counts as a failure.
func hammer(t *testing.T, f *failures, workers, iterations int, work func(w, i int)) {
	t.Helper()
	within(t, fmt.Sprintf("%d goroutines with %d calls each", workers, iterations), func() {
		var wg sync.WaitGroup
		for w := range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() {
					if v := recover(); v != nil {
						f.add("worker %d panicked: %v", w, v)
					}
				}()
				for i := range iterations {
					work(w, i)
				}
			}()
		}
		wg.Wait()
	})
}

func TestStress(t *testing.T) {
	// Run these with -race. They mix every method on a few hot keys.
	keys := []string{"user:1", "user:2", "user:3", "user:4"}
	value := func(k int) string { return "value of " + keys[k] }

	t.Run("at most one execution per key at a time", func(t *testing.T) {
		// Without Forget and without callers that leave, a key never has two
		// executions at the same time.
		var g group
		f := &failures{t: t}
		var running [4]atomic.Int32
		load := func(k int) (string, error) {
			if running[k].Add(1) > 1 {
				f.add("two executions for %q ran at the same time", keys[k])
			}
			runtime.Gosched() // give other callers a chance to arrive
			running[k].Add(-1)
			return value(k), nil
		}

		hammer(t, f, 16, 200, func(w, i int) {
			k := (w + i) % len(keys)
			var got string
			var err error
			switch (w + 2*i) % 3 {
			case 0:
				got, err, _ = g.Do(keys[k], func() (string, error) { return load(k) })
			case 1:
				r := <-g.DoChan(keys[k], func() (string, error) { return load(k) })
				got, err = r.Val, r.Err
			case 2:
				got, err, _ = g.DoContext(context.Background(), keys[k], func(context.Context) (string, error) { return load(k) })
			}
			if got != value(k) || err != nil {
				f.add("call for %q = %q, %v; want %q, <nil>", keys[k], got, err, value(k))
			}
		})
	})

	t.Run("with Forget and callers that leave", func(t *testing.T) {
		checkLeaks := expectNoLeak(t)
		var g group
		f := &failures{t: t}
		load := func(ctx context.Context, k int) (string, error) {
			runtime.Gosched()
			if err := ctx.Err(); err != nil {
				// fn's ctx is cancelled only when nobody waits any more, so no
				// caller may ever receive this error.
				return "", fmt.Errorf("fn's ctx was cancelled while a caller still waited: %w", err)
			}
			return value(k), nil
		}

		hammer(t, f, 16, 200, func(w, i int) {
			k := (w + i) % len(keys)
			switch (w + 3*i) % 4 {
			case 0:
				got, err, _ := g.Do(keys[k], func() (string, error) { return load(context.Background(), k) })
				if got != value(k) || err != nil {
					f.add("Do(%q) = %q, %v; want %q, <nil>", keys[k], got, err, value(k))
				}
			case 1:
				r := <-g.DoChan(keys[k], func() (string, error) { return load(context.Background(), k) })
				if r.Val != value(k) || r.Err != nil {
					f.add("DoChan(%q) delivered %+v; want Val %q", keys[k], r, value(k))
				}
			case 2:
				// Some callers give up after a few microseconds.
				ctx, cancel := context.WithTimeout(context.Background(), time.Duration(i%4)*5*time.Microsecond)
				defer cancel()
				got, err, _ := g.DoContext(ctx, keys[k], func(ctx context.Context) (string, error) { return load(ctx, k) })
				switch {
				case err == nil && got == value(k):
				case err != nil && ctx.Err() != nil && errors.Is(err, ctx.Err()) && got == "":
				default:
					f.add("DoContext(%q) = %q, %v (own ctx: %v); want %q, <nil> or \"\" and the ctx's error",
						keys[k], got, err, ctx.Err(), value(k))
				}
			case 3:
				g.Forget(keys[k])
			}
		})
		checkLeaks()
	})
}
