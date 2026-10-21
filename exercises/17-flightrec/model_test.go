package flightrec_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	flightrec "github.com/yornik/go-practice/exercises/17-flightrec"
)

// ringModel is a deliberately simple ring: it appends every value to a
// slice and never forgets one. It is far too wasteful for real use, but
// easy to check by eye.
type ringModel struct {
	size int
	all  []int
}

func (m *ringModel) add(v int) { m.all = append(m.all, v) }

// held returns the values the ring holds: the last size values added.
func (m *ringModel) held() []int { return m.all[max(0, len(m.all)-m.size):] }

func (m *ringModel) last(n int) []int {
	h := m.held()
	if n <= 0 {
		return nil
	}
	return h[max(0, len(h)-n):]
}

func TestRingModel(t *testing.T) {
	// 10 000 random Add and Last calls on rings of 5 sizes, compared with
	// ringModel after every call. This finds the index bugs that the cases
	// above miss: the first wrap, a Last that crosses the end of the ring,
	// and so on. The seed is fixed, so every run makes the same calls.
	defer guard(t)()
	rng := rand.New(rand.NewPCG(17, 17))
	for _, size := range []int{1, 2, 3, 5, 8} {
		r := flightrec.NewRing[int](size)
		m := &ringModel{size: size}
		var history []string // the last calls, for the failure message
		var i int
		fail := func(format string, args ...any) {
			t.Helper()
			t.Fatalf("NewRing(%d), call %d: %s\nlast calls: %s", size, i+1, fmt.Sprintf(format, args...), history)
		}
		for i = range 2000 {
			var call string
			isAdd := rng.IntN(10) < 7
			n := 0
			if isAdd {
				call = fmt.Sprintf("Add(%d)", i)
				r.Add(i)
				m.add(i)
			} else {
				n = rng.IntN(size+5) - 2 // from -2 to size+2
				call = fmt.Sprintf("Last(%d)", n)
			}
			// Record the call before any check, so that a failure lists it
			// as the last of at most 10 calls.
			history = append(history, call)
			if len(history) > 10 {
				history = history[1:]
			}
			if !isAdd {
				if got, want := r.Last(n), m.last(n); !slices.Equal(got, want) {
					fail("%s = %v, want %v", call, got, want)
				}
			}

			held := m.held()
			if got := r.Len(); got != len(held) {
				fail("after %s: Len() = %d, want %d", call, got, len(held))
			}
			if got, want := r.Dropped(), len(m.all)-len(held); got != want {
				fail("after %s: Dropped() = %d, want %d", call, got, want)
			}
			wantOldest, wantNewest, wantOK := 0, 0, len(held) > 0
			if wantOK {
				wantOldest, wantNewest = held[0], held[len(held)-1]
			}
			if got, ok := r.Oldest(); got != wantOldest || ok != wantOK {
				fail("after %s: Oldest() = %d, %v; want %d, %v", call, got, ok, wantOldest, wantOK)
			}
			if got, ok := r.Newest(); got != wantNewest || ok != wantOK {
				fail("after %s: Newest() = %d, %v; want %d, %v", call, got, ok, wantNewest, wantOK)
			}
			if got := r.Last(size); !slices.Equal(got, held) {
				fail("after %s: Last(%d) = %v, want %v", call, size, got, held)
			}
		}
	}
}

func TestRingLargeStaysFast(t *testing.T) {
	// 1 500 000 Add calls on a ring of 500 000 values. With an Add in O(1)
	// this takes well under 1 s, also with -race. An Add that moves every
	// stored value to make room takes minutes, so the test gives up after
	// 20 s.
	const (
		size   = 500_000
		calls  = 1_500_000
		budget = 20 * time.Second
	)
	defer guardFor(t, budget+10*time.Second)()
	r := flightrec.NewRing[int](size)
	start := time.Now()
	for i := range calls {
		r.Add(i)
		if i%1024 == 0 && time.Since(start) > budget {
			t.Fatalf("NewRing(%d): %d Add calls took more than %v; Add must take O(1) time", size, i+1, budget)
		}
	}
	if got := r.Len(); got != size {
		t.Errorf("NewRing(%d) after %d Add calls: Len() = %d, want %d", size, calls, got, size)
	}
	if got := r.Dropped(); got != calls-size {
		t.Errorf("NewRing(%d) after %d Add calls: Dropped() = %d, want %d", size, calls, got, calls-size)
	}
	if got, ok := r.Oldest(); got != calls-size || !ok {
		t.Errorf("NewRing(%d) after Add 0 to %d: Oldest() = %d, %v; want %d, true", size, calls-1, got, ok, calls-size)
	}
	if got, want := r.Last(3), []int{calls - 3, calls - 2, calls - 1}; !slices.Equal(got, want) {
		t.Errorf("NewRing(%d) after Add 0 to %d: Last(3) = %v, want %v", size, calls-1, got, want)
	}
}
