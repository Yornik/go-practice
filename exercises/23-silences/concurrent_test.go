package silences_test

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"

	silences "github.com/yornik/go-practice/exercises/23-silences"
)

// Step 5: many goroutines at once. Run with -race.

// parallel runs f(0), ..., f(n-1) in n goroutines, released at the same
// moment so that they really overlap, and waits for all of them. It fails
// the test if one of them panics or if they do not all return in time.
func parallel(t *testing.T, n int, f func(i int)) {
	t.Helper()
	start := make(chan struct{})
	var (
		wg     sync.WaitGroup
		mu     sync.Mutex
		panics []any
	)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if v := recover(); v != nil {
					mu.Lock()
					panics = append(panics, v)
					mu.Unlock()
				}
			}()
			<-start
			f(i)
		}()
	}
	close(start)

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("%d goroutines did not all return within %v: a call is blocked (%s)", n, timeout, lockHang)
	}
	if len(panics) > 0 {
		t.Fatalf("%d of %d goroutines panicked, for example with: %v", len(panics), n, panics[0])
	}
}

// problems collects failures from many goroutines and reports the first
// few, so a wrong solution does not print thousands of lines.
type problems struct {
	mu   sync.Mutex
	list []string
}

func (p *problems) add(format string, args ...any) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.list = append(p.list, fmt.Sprintf(format, args...))
}

func (p *problems) report(t *testing.T) {
	t.Helper()
	p.mu.Lock()
	defer p.mu.Unlock()
	for i, msg := range p.list {
		if i == 5 {
			t.Errorf("... and %d more problems", len(p.list)-5)
			break
		}
		t.Error(msg)
	}
}

func TestConcurrentEverySilenceEndsOnce(t *testing.T) {
	// 4 goroutines add silences, 2 match alerts and 2 call Expire while
	// they move the clock forward. At the end every silence that Add
	// accepted was returned by exactly 1 Expire call and passed to
	// onExpire exactly once, and the store is empty.
	const (
		adders     = 4
		addsEach   = 300
		matchers   = 2
		matchEach  = 600
		expirers   = 2
		expireEach = 150
	)
	alerts := []string{"disk_full", "feeder_offline", "high_latency", ""}
	targets := []string{"photo-store-1", "photo-store-2", "photo-store-3", "photo-store-4", "feeder-0412", ""}

	clock := newClock()
	var (
		bad      problems
		mu       sync.Mutex
		accepted = map[string]bool{} // IDs that Add accepted
		called   = map[string]int{}  // ID -> onExpire calls
		returned = map[string]int{}  // ID -> times an Expire result held it
		s        *silences.Store
	)
	onExpire := func(ended silences.Silence) {
		_ = s.Len() // calls back into the store, as the chat bot does
		mu.Lock()
		defer mu.Unlock()
		called[ended.ID]++
	}
	within(t, "New(clock.Now, onExpire)", lockHang, func() { s = silences.New(clock.Now, onExpire) })
	if s == nil {
		t.Fatal("New(clock.Now, onExpire) = nil, want a store")
	}

	parallel(t, adders+matchers+expirers, func(g int) {
		rng := rand.New(rand.NewPCG(23, uint64(g)))
		switch {
		case g < adders:
			for i := range addsEach {
				x := silences.Silence{
					ID:     fmt.Sprintf("g%d-%d", g, i),
					Alert:  alerts[rng.IntN(len(alerts))],
					Target: targets[rng.IntN(len(targets))],
					Until:  clock.Now().Add(time.Duration(1+rng.IntN(60)) * time.Second),
				}
				if x.Alert == "" && x.Target == "" {
					x.Alert = "disk_full"
				}
				err := s.Add(x)
				switch {
				case err == nil:
					mu.Lock()
					accepted[x.ID] = true
					mu.Unlock()
				case !errors.Is(err, silences.ErrInvalid):
					// The clock may have passed Until since this goroutine
					// read it; then ErrInvalid is right. Nothing else is.
					bad.add("Add(%s) = %v, want nil or an error that wraps ErrInvalid", show(x), err)
				}
			}
		case g < adders+matchers:
			for range matchEach {
				alert := alerts[rng.IntN(len(alerts)-1)]
				target := targets[rng.IntN(len(targets)-1)]
				before := clock.Now() // the clock only moves forward
				got, ok := s.Match(alert, target)
				if !ok {
					continue
				}
				if (got.Alert != alert && got.Alert != "") || (got.Target != target && got.Target != "") {
					bad.add("Match(%q, %q) = %s, which does not match", alert, target, show(got))
				}
				if !got.Until.After(before) {
					bad.add("Match(%q, %q) = %s, which had ended at %s, before the call", alert, target, show(got), since(before))
				}
			}
		default:
			for range expireEach {
				clock.Advance(100 * time.Millisecond)
				ended := s.Expire()
				after := clock.Now()
				for i, x := range ended {
					if x.Until.After(after) {
						bad.add("Expire() returned %s, which had not ended at %s", show(x), since(after))
					}
					if i > 0 {
						prev := ended[i-1]
						if x.Until.Before(prev.Until) || (x.Until.Equal(prev.Until) && x.ID < prev.ID) {
							bad.add("Expire() = %s: not sorted by Until, then ID", showAll(ended))
						}
					}
				}
				mu.Lock()
				for _, x := range ended {
					returned[x.ID]++
				}
				mu.Unlock()
			}
		}
	})

	// Every silence ends within 60 s of its Add; this ends them all.
	clock.Advance(2 * time.Hour)
	var rest []silences.Silence
	var n int
	within(t, "the last Expire() and Len()", lockHang, func() {
		rest = s.Expire()
		n = s.Len()
	})
	mu.Lock()
	defer mu.Unlock()
	for _, x := range rest {
		returned[x.ID]++
	}
	if n != 0 {
		bad.add("Len() = %d after the last Expire(), want 0", n)
	}
	if len(accepted) == 0 {
		bad.add("Add accepted no silence at all")
	}
	for id := range accepted {
		if returned[id] != 1 || called[id] != 1 {
			bad.add("silence %q: in %d Expire results and %d onExpire calls, want 1 and 1", id, returned[id], called[id])
		}
	}
	for id := range returned {
		if !accepted[id] {
			bad.add("Expire returned silence %q, which Add rejected", id)
		}
	}
	for id := range called {
		if !accepted[id] {
			bad.add("onExpire got silence %q, which Add rejected", id)
		}
	}
	bad.report(t)
}
