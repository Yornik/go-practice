package loadbalancer_test

import (
	"fmt"
	"slices"
	"sync"
	"testing"

	loadbalancer "github.com/yornik/go-practice/exercises/25-loadbalancer"
)

// backends returns one Backend per entry of inFlight, with that many
// requests in flight, named http://10.0.0.1:8080, http://10.0.0.2:8080, ...
func backends(t *testing.T, inFlight ...int64) []*loadbalancer.Backend {
	t.Helper()
	bs := make([]*loadbalancer.Backend, len(inFlight))
	for i, n := range inFlight {
		raw := fmt.Sprintf("http://10.0.0.%d:8080", i+1)
		b, err := loadbalancer.NewBackend(raw)
		if err != nil {
			t.Fatalf("NewBackend(%q) error = %v, want nil", raw, err)
		}
		for range n {
			b.Begin()
		}
		bs[i] = b
	}
	return bs
}

// pickIndex calls s.Pick(bs) and returns the index of the picked backend.
func pickIndex(t *testing.T, s loadbalancer.Strategy, bs []*loadbalancer.Backend) int {
	t.Helper()
	before := slices.Clone(bs)
	got := s.Pick(bs)
	if !slices.Equal(bs, before) {
		t.Fatalf("Pick modified its argument")
	}
	i := slices.Index(bs, got)
	if i < 0 {
		t.Fatalf("Pick returned %v, which is not one of its arguments", got)
	}
	return i
}

// ----- Backend -----

func TestBackend(t *testing.T) {
	defer catchPanic(t)
	b, err := loadbalancer.NewBackend("http://10.0.0.7:8080")
	if err != nil {
		t.Fatalf("NewBackend error = %v, want nil", err)
	}
	if got := b.URL().String(); got != "http://10.0.0.7:8080" {
		t.Errorf("URL() = %q, want %q", got, "http://10.0.0.7:8080")
	}
	if got := b.InFlight(); got != 0 {
		t.Errorf("InFlight() of a new Backend = %d, want 0", got)
	}
	b.Begin()
	b.Begin()
	b.Begin()
	b.End()
	if got := b.InFlight(); got != 2 {
		t.Errorf("InFlight() after 3 Begin and 1 End = %d, want 2", got)
	}

	https, err := loadbalancer.NewBackend("https://api.internal:8443")
	if err != nil || https.URL().Host != "api.internal:8443" {
		t.Errorf("NewBackend(%q) = %v, %v; want a backend with host api.internal:8443", "https://api.internal:8443", https, err)
	}
}

func TestBackendInvalidURL(t *testing.T) {
	for _, raw := range []string{
		"10.0.0.7:8080",    // no scheme: not a valid URL
		"ftp://10.0.0.7",   // scheme is not http or https
		"http://",          // no host
		"/api/v1",          // relative
		"",                 // empty
		"http://[::1:8080", // unparsable
	} {
		t.Run(raw, func(t *testing.T) {
			defer catchPanic(t)
			b, err := loadbalancer.NewBackend(raw)
			if err == nil || b != nil {
				t.Errorf("NewBackend(%q) = %v, %v; want nil and an error", raw, b, err)
			}
		})
	}
}

func TestBackendBeginEndConcurrent(t *testing.T) {
	// The LB calls Begin and End from many goroutines at once. Every one of
	// them must count: no lost updates.
	defer catchPanic(t)
	b := backends(t, 0)[0]
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 200 {
				b.Begin()
				b.End()
				b.Begin()
			}
		}()
	}
	wg.Wait()
	if got := b.InFlight(); got != 50*200 {
		t.Errorf("InFlight() = %d after 50 goroutines did 400 Begin and 200 End calls each, want %d", got, 50*200)
	}
}

// ----- RoundRobin -----

func TestStrategyRoundRobin(t *testing.T) {
	defer catchPanic(t)
	bs := backends(t, 0, 0, 0)
	rr := loadbalancer.RoundRobin()
	var got []int
	for range 7 {
		got = append(got, pickIndex(t, rr, bs))
	}
	if want := []int{0, 1, 2, 0, 1, 2, 0}; !slices.Equal(got, want) {
		t.Errorf("7 picks from 3 backends = %v, want %v", got, want)
	}

	// The counter belongs to the strategy, not to the list: when the list
	// changes (a backend became ineligible), the 8th pick (k = 7) returns
	// bs[7 % 2] = bs[1].
	if got := pickIndex(t, rr, bs[:2]); got != 1 {
		t.Errorf("8th pick from 2 backends = index %d, want 1 (7 %% 2)", got)
	}

	// In-flight counts do not matter to round robin.
	busy := backends(t, 100, 0)
	rr2 := loadbalancer.RoundRobin()
	if got := pickIndex(t, rr2, busy); got != 0 {
		t.Errorf("first pick of a new RoundRobin = index %d, want 0: each RoundRobin() has its own counter, starting at 0", got)
	}
}

func TestStrategyRoundRobinConcurrent(t *testing.T) {
	// 8 goroutines pick 300 times each from 3 backends. A counter that is not
	// updated atomically loses updates, and then some backends get more than
	// their share.
	defer catchPanic(t)
	bs := backends(t, 0, 0, 0)
	rr := loadbalancer.RoundRobin()
	var mu sync.Mutex
	counts := map[*loadbalancer.Backend]int{}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			local := map[*loadbalancer.Backend]int{}
			for range 300 {
				local[rr.Pick(bs)]++
			}
			mu.Lock()
			defer mu.Unlock()
			for b, n := range local {
				counts[b] += n
			}
		}()
	}
	wg.Wait()
	for i, b := range bs {
		if counts[b] != 800 {
			t.Errorf("backend %d was picked %d times, want 800 (2400 picks over 3 backends)", i, counts[b])
		}
	}
}

// ----- LeastConnections -----

func TestStrategyLeastConnections(t *testing.T) {
	tests := []struct {
		inFlight []int64
		want     int
	}{
		{[]int64{0}, 0},
		{[]int64{2, 0, 1}, 1},
		{[]int64{5, 4, 3}, 2},
		{[]int64{1, 1, 1}, 0},    // tie: the first one
		{[]int64{3, 1, 2, 1}, 1}, // tie between 1 and 3: the first one
		{[]int64{0, 7, 0}, 0},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprint(tc.inFlight), func(t *testing.T) {
			defer catchPanic(t)
			bs := backends(t, tc.inFlight...)
			if got := pickIndex(t, loadbalancer.LeastConnections(), bs); got != tc.want {
				t.Errorf("LeastConnections().Pick(in-flight %v) = index %d, want %d", tc.inFlight, got, tc.want)
			}
		})
	}
}

// ----- PowerOfTwo -----

// scriptedRand returns the values one after the other and records the
// arguments it was called with.
type scriptedRand struct {
	mu     sync.Mutex
	values []int
	args   []int
}

func (s *scriptedRand) IntN(n int) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.args = append(s.args, n)
	if len(s.values) == 0 {
		panic("scriptedRand: called more often than expected")
	}
	v := s.values[0]
	s.values = s.values[1:]
	return v
}

func TestStrategyPowerOfTwo(t *testing.T) {
	tests := []struct {
		name     string
		inFlight []int64
		rand     []int // what randIntN returns, in order
		want     int
	}{
		// First candidate: index rand[0]. Second candidate: the rand[1]-th of
		// the other backends, counted in list order without the first.
		{"first wins", []int64{5, 1, 3, 0}, []int{1, 1}, 1},          // 1 vs 2 (in-flight 1 vs 3)
		{"second wins", []int64{5, 1, 3, 0}, []int{0, 2}, 3},         // 0 vs 3 (5 vs 0)
		{"second before first", []int64{5, 1, 3, 0}, []int{2, 1}, 1}, // 2 vs 1 (3 vs 1)
		{"second is the last", []int64{4, 4, 0}, []int{0, 1}, 2},     // 0 vs 2 (4 vs 0)
		{"tie: first candidate", []int64{2, 2}, []int{1, 0}, 1},      // 1 vs 0 (2 vs 2)
		{"tie: first candidate again", []int64{2, 2}, []int{0, 0}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			r := &scriptedRand{values: slices.Clone(tc.rand)}
			bs := backends(t, tc.inFlight...)
			got := pickIndex(t, loadbalancer.PowerOfTwo(r.IntN), bs)
			if got != tc.want {
				t.Errorf("Pick(in-flight %v) with randIntN returning %v = index %d, want %d", tc.inFlight, tc.rand, got, tc.want)
			}
			n := len(tc.inFlight)
			if want := []int{n, n - 1}; !slices.Equal(r.args, want) {
				t.Errorf("randIntN was called with %v, want %v", r.args, want)
			}
		})
	}

	t.Run("one backend: no random numbers", func(t *testing.T) {
		defer catchPanic(t)
		r := &scriptedRand{}
		if got := pickIndex(t, loadbalancer.PowerOfTwo(r.IntN), backends(t, 3)); got != 0 {
			t.Errorf("Pick of one backend = index %d, want 0", got)
		}
		if len(r.args) != 0 {
			t.Errorf("randIntN was called with %v, want no calls for a single backend", r.args)
		}
	})
}

func TestStrategyPowerOfTwoDefaultRand(t *testing.T) {
	// PowerOfTwo(nil) uses math/rand/v2. With 2 backends the two candidates
	// are always both backends, so the idle one must win every time.
	defer catchPanic(t)
	p := loadbalancer.PowerOfTwo(nil)
	bs := backends(t, 0, 5)
	for range 50 {
		if got := pickIndex(t, p, bs); got != 0 {
			t.Fatalf("PowerOfTwo(nil).Pick(in-flight [0 5]) = index %d, want 0", got)
		}
	}
	// With 3 backends every pick must still be one of them; pickIndex checks.
	three := backends(t, 1, 2, 3)
	for range 50 {
		pickIndex(t, p, three)
	}
}
