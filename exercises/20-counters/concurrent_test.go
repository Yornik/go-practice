package counters_test

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	counters "github.com/yornik/go-practice/exercises/20-counters"
)

// Step 3: many goroutines at once. Run with -race.

const (
	goroutines = 64
	addsEach   = 10_000
)

func TestConcurrentAddOneCounter(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	c := counter(t, r, "requests_lightrail")

	parallel(t, goroutines, func(int) {
		for range addsEach {
			c.Add(1)
		}
	})

	want := int64(goroutines * addsEach)
	wantValue(t, c, "requests_lightrail", want,
		fmt.Sprintf("%d goroutines called Add(1) %d times each", goroutines, addsEach))
}

func TestConcurrentAddManyCounters(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	names := make([]string, goroutines)
	for i := range names {
		names[i] = fmt.Sprintf("requests_district_%02d", i)
	}

	// Goroutine g starts at counter g and walks through all of them, so every
	// counter is created by 1 of many goroutines and then shared by all.
	parallel(t, goroutines, func(g int) {
		for j := range addsEach {
			r.Counter(names[(g+j)%len(names)]).Add(1)
		}
	})

	got := snapshot(t, r)
	if len(got) != len(names) {
		t.Fatalf("Snapshot() has %d samples after %d goroutines used %d names, want %d", len(got), goroutines, len(names), len(names))
	}
	wrong := 0
	for i, name := range names {
		want := s(name, goroutines*addsEach/int64(len(names)))
		if got[i] != want && wrong < 5 {
			wrong++
			t.Errorf("Snapshot()[%d] = %s, want %s, after %d goroutines called Counter(name).Add(1) %d times each, spread evenly over %d names",
				i, show(got[i:i+1]), show([]counters.Sample{want}), goroutines, addsEach, len(names))
		}
	}
}

func TestConcurrentCounterSameName(t *testing.T) {
	defer guard(t)()
	const (
		askers   = 100
		rounds   = 200
		newNames = 20
	)
	r := newRegistry(t)
	for round := range rounds {
		names := make([]string, newNames)
		for k := range names {
			names[k] = fmt.Sprintf("requests_old_town_%03d_%02d", round, k)
		}
		// got[i][k] is the pointer that goroutine i got for names[k]. Each
		// goroutine writes only its own row.
		got := make([][]*counters.Counter, askers)
		parallel(t, askers, func(i int) {
			row := make([]*counters.Counter, len(names))
			for k, name := range names {
				row[k] = r.Counter(name)
			}
			got[i] = row
		})
		for k, name := range names {
			for i := range got {
				if c := got[i][k]; c == nil || c != got[0][k] {
					t.Fatalf("round %d: %d goroutines called Counter(%q) at the same moment; goroutine 0 got %p, goroutine %d got %p, want the same pointer for all",
						round, askers, name, got[0][k], i, c)
				}
			}
			if again := counter(t, r, name); again != got[0][k] {
				t.Fatalf("round %d: Counter(%q) = %p after the goroutines returned, want %p, the pointer they got", round, name, again, got[0][k])
			}
		}
	}
	if got := snapshot(t, r); len(got) != rounds*newNames {
		t.Errorf("Snapshot() has %d samples after %d new names, want %d", len(got), rounds*newNames, rounds*newNames)
	}
}

func TestConcurrentDrainLosesNothing(t *testing.T) {
	defer guard(t)()
	const (
		adders    = 8
		addsPerGo = 20_000
	)
	names := []string{"requests_harbour", "requests_lightrail", "requests_spire", "rejected"}
	r := newRegistry(t)

	var (
		stop       atomic.Bool
		drained    = map[string]int64{}
		drains     int
		scrapes    int
		background sync.WaitGroup
	)
	panics := make(chan any, 2)
	// 1 goroutine drains and 1 scrapes until the adders are done. Each
	// starts at once, so both overlap with the adders.
	background.Add(2)
	go func() {
		defer background.Done()
		defer func() {
			if v := recover(); v != nil {
				panics <- v
			}
		}()
		for {
			for _, x := range r.Drain() {
				drained[x.Name] += x.Value
			}
			drains++
			if stop.Load() {
				return
			}
		}
	}()
	go func() {
		defer background.Done()
		defer func() {
			if v := recover(); v != nil {
				panics <- v
			}
		}()
		for !stop.Load() {
			r.Snapshot()
			if err := r.Write(io.Discard); err != nil {
				panic(fmt.Sprintf("Write(io.Discard) returned %v", err))
			}
			scrapes++
		}
	}()

	// Adder g adds g+1 every time, so a count lost or counted twice changes
	// the sum.
	parallel(t, adders, func(g int) {
		for j := range addsPerGo {
			r.Counter(names[(g+j)%len(names)]).Add(int64(g + 1))
		}
	})
	stop.Store(true)
	waitGroup(t, &background, "the goroutines that call Drain, Snapshot and Write")
	close(panics)
	for v := range panics {
		t.Fatalf("a goroutine that calls Drain, Snapshot and Write panicked: %v", v)
	}

	// Every adder spreads its adds evenly over the names.
	wantPerName := int64(0)
	for g := range adders {
		wantPerName += int64(g+1) * addsPerGo / int64(len(names))
	}
	final := snapshot(t, r)
	finalByName := map[string]int64{}
	for _, x := range final {
		finalByName[x.Name] = x.Value
	}
	for _, name := range names {
		if got := drained[name] + finalByName[name]; got != wantPerName {
			t.Errorf("%q: the values that %d calls of Drain returned add up to %d, the final value is %d; together %d, want %d (the sum of every Add)",
				name, drains, drained[name], finalByName[name], got, wantPerName)
		}
	}
	if len(final) != len(names) {
		t.Errorf("Snapshot() at the end = %s, want %d counters: Drain must keep them", show(final), len(names))
	}
	t.Logf("%d calls of Drain and %d of Snapshot and Write ran during the adds", drains, scrapes)
}

// waitGroup waits for wg, and fails the test if that takes longer than the
// timeout.
func waitGroup(t *testing.T, wg *sync.WaitGroup, what string) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %v: a call is blocked. Stacks of all goroutines:\n%s", what, timeout, stacks())
	}
}
