package middleware_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	middleware "github.com/yornik/go-practice/exercises/16-middleware"
)

// ---------------------------------------------------------------- MaxBytes

// bodyReader is a handler that reads the whole request body and records what
// it got. It answers 413 itself when the body is too large, the way a real
// handler should.
type bodyReader struct {
	called bool
	body   string
	err    error
}

func (b *bodyReader) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	b.called = true
	data, err := io.ReadAll(r.Body)
	b.body, b.err = string(data), err
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		http.Error(w, "body too large", http.StatusRequestEntityTooLarge)
	}
}

func TestMaxBytesRejectsDeclaredLength(t *testing.T) {
	h := &bodyReader{}
	r := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("0123456789x")) // r.ContentLength = 11
	w := serve(t, middleware.Chain(h, middleware.MaxBytes(10)), r)
	if w.status != http.StatusRequestEntityTooLarge {
		t.Errorf("MaxBytes(10), ContentLength 11: status = %d, want %d", w.status, http.StatusRequestEntityTooLarge)
	}
	if h.called {
		t.Errorf("MaxBytes(10), ContentLength 11: the handler was called, want it not called")
	}
}

func TestMaxBytesAllowsUpToLimit(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		unknown bool // no Content-Length, as with Transfer-Encoding: chunked
	}{
		{name: "exactly the limit", body: "0123456789"},
		{name: "below the limit", body: "abc"},
		{name: "no body", body: ""},
		{name: "unknown length, exactly the limit", body: "0123456789", unknown: true},
		{name: "unknown length, below the limit", body: "abc", unknown: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := &bodyReader{}
			r := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader(tc.body))
			if tc.unknown {
				r.ContentLength = -1
			}
			w := serve(t, middleware.Chain(h, middleware.MaxBytes(10)), r)
			if !h.called {
				t.Fatalf("MaxBytes(10), body %q: the handler was not called (status %d)", tc.body, w.status)
			}
			if h.err != nil || h.body != tc.body {
				t.Errorf("MaxBytes(10): the handler read %q, %v, want %q, nil", h.body, h.err, tc.body)
			}
			if w.status != http.StatusOK {
				t.Errorf("MaxBytes(10), body %q: status = %d, want %d", tc.body, w.status, http.StatusOK)
			}
		})
	}
}

func TestMaxBytesLimitsUnknownLength(t *testing.T) {
	// A chunked upload does not declare its size. Only reading tells.
	h := &bodyReader{}
	r := httptest.NewRequest(http.MethodPost, "/upload", strings.NewReader("0123456789x"))
	r.ContentLength = -1
	w := serve(t, middleware.Chain(h, middleware.MaxBytes(10)), r)
	if !h.called {
		t.Fatalf("MaxBytes(10), unknown length: the handler was not called (status %d), want it called", w.status)
	}
	var mbe *http.MaxBytesError
	if !errors.As(h.err, &mbe) {
		t.Fatalf("MaxBytes(10), 11 bytes of unknown length: reading the body returned %v, want an error of type *http.MaxBytesError", h.err)
	}
	if mbe.Limit != 10 {
		t.Errorf("MaxBytesError.Limit = %d, want 10", mbe.Limit)
	}
	if w.status != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d, want %d (from the handler)", w.status, http.StatusRequestEntityTooLarge)
	}
}

// ---------------------------------------------------------------- LoadShed

// gate is a handler that holds every request until release is closed. It
// counts the requests that reached it.
type gate struct {
	entered atomic.Int32
	release chan struct{}
}

func newGate() *gate { return &gate{release: make(chan struct{})} }

func (g *gate) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.entered.Add(1)
	select {
	case <-g.release:
	case <-time.After(timeout):
	}
	write(w, "done")
}

func TestLoadShedRejectsOverLimit(t *testing.T) {
	g := newGate()
	h := middleware.Chain(g, middleware.LoadShed(2, 5*time.Second))

	first, second := start(h, get("/1")), start(h, get("/2"))
	waitFor(t, "2 requests are in the handler", func() bool { return g.entered.Load() == 2 })

	// The 3rd request must get its answer now, while the other 2 still run.
	w := await(t, start(h, get("/3")), "3rd request while 2 of 2 slots are taken (LoadShed must not wait)")
	if w.status != http.StatusServiceUnavailable {
		t.Errorf("3rd request: status = %d, want %d", w.status, http.StatusServiceUnavailable)
	}
	if got := w.wire.Get("Retry-After"); got != "5" {
		t.Errorf("3rd request: Retry-After = %q, want %q", got, "5")
	}
	if n := g.entered.Load(); n != 2 {
		t.Errorf("the handler was called %d times, want 2 (not for the shed request)", n)
	}

	close(g.release)
	for i, ch := range []<-chan result{first, second} {
		if w := await(t, ch, fmt.Sprintf("request %d", i+1)); w.status != http.StatusOK {
			t.Errorf("request %d: status = %d, want %d", i+1, w.status, http.StatusOK)
		}
	}

	// Both slots are free again.
	for i := range 2 {
		if w := await(t, start(h, get("/again")), "request after the others finished"); w.status != http.StatusOK {
			t.Errorf("request %d after the others finished: status = %d, want %d", i+1, w.status, http.StatusOK)
		}
	}
}

func TestLoadShedRetryAfter(t *testing.T) {
	tests := []struct {
		retryAfter time.Duration
		want       string
	}{
		{30 * time.Second, "30"},
		{2 * time.Second, "2"},
		{1500 * time.Millisecond, "2"}, // rounded up: never ask for a retry earlier than you mean
		{time.Millisecond, "1"},
	}
	for _, tc := range tests {
		t.Run(tc.retryAfter.String(), func(t *testing.T) {
			g := newGate()
			h := middleware.Chain(g, middleware.LoadShed(1, tc.retryAfter))
			held := start(h, get("/held"))
			waitFor(t, "1 request is in the handler", func() bool { return g.entered.Load() == 1 })

			w := await(t, start(h, get("/shed")), "request over the limit")
			if got := w.wire.Get("Retry-After"); w.status != http.StatusServiceUnavailable || got != tc.want {
				t.Errorf("LoadShed(1, %v), over the limit: %d with Retry-After %q, want %d with %q",
					tc.retryAfter, w.status, got, http.StatusServiceUnavailable, tc.want)
			}
			close(g.release)
			await(t, held, "held request")
		})
	}
}

// The next two tests look for a gap between checking for a free slot and
// taking it. With such a gap, 2 requests can both see the last free slot:
// then 1 request too many runs, or 1 waits for a slot. That happens only now
// and then, so both tests repeat the race many times. They never fail when
// the check and the take are one step. -race makes a gap more likely to show.

func TestLoadShedBurst(t *testing.T) {
	// 16 requests arrive at the same instant and 2 may run: exactly 2 get in,
	// and the other 14 get 503 without waiting. 50 times.
	const limit, burst, rounds = 2, 16, 50
	for round := range rounds {
		in, shed := shedRound(t, limit, burst)
		if in != limit || shed != burst-limit {
			t.Fatalf("round %d: LoadShed(%d), %d requests at the same instant: %d let in and %d shed, want %d and %d (are checking for a free slot and taking it one step?)",
				round+1, limit, burst, in, shed, limit, burst-limit)
		}
	}
}

// shedRound sends burst requests at the same instant through a new
// LoadShed(limit), in front of a handler that holds every request it gets.
// It returns how many requests reached the handler and how many got 503.
func shedRound(t *testing.T, limit, burst int32) (in, shed int32) {
	t.Helper()
	g := newGate()
	h := middleware.Chain(g, middleware.LoadShed(int(limit), time.Second))

	var ready, shedCount atomic.Int32
	var startNow atomic.Bool
	var wg sync.WaitGroup
	defer func() {
		close(g.release)
		wg.Wait()
	}()
	for i := range burst {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Keep running until the start signal, so that every request
			// starts at the same instant instead of one by one.
			ready.Add(1)
			for !startNow.Load() {
				runtime.Gosched()
			}
			w := newSpy()
			if v := callHandler(h, w, get(fmt.Sprintf("/%d", i))); v != nil {
				t.Errorf("request %d panicked: %v", i, v)
				return
			}
			if w.status == http.StatusServiceUnavailable {
				shedCount.Add(1)
			}
		}()
	}
	waitFor(t, "every request goroutine is ready", func() bool { return ready.Load() == burst })
	startNow.Store(true)
	waitFor(t, fmt.Sprintf("every request was let in or shed (want %d in, %d shed; LoadShed must not wait)", limit, burst-limit),
		func() bool { return g.entered.Load()+shedCount.Load() >= burst })
	return g.entered.Load(), shedCount.Load()
}

func TestLoadShedNeverOverLimit(t *testing.T) {
	// 8 goroutines send 2000 requests each through LoadShed(2), so slots are
	// freed and taken thousands of times. The handler counts how many
	// requests run it at the same time: never more than 2.
	const limit, workers, perWorker = 2, 8, 2000
	var mu sync.Mutex
	var running, most int
	h := middleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		running++
		most = max(most, running)
		mu.Unlock()
		runtime.Gosched() // let the other goroutines run while this one holds a slot
		mu.Lock()
		running--
		mu.Unlock()
	}), middleware.LoadShed(limit, time.Second))

	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := get("/")
			for range perWorker {
				if v := callHandler(h, newSpy(), r); v != nil {
					t.Errorf("ServeHTTP panicked: %v", v)
					return
				}
			}
		}()
	}
	wg.Wait()
	if most > limit {
		t.Errorf("LoadShed(%d), %d goroutines with %d requests each: up to %d requests ran the handler at the same time, want at most %d (are checking for a free slot and taking it one step?)",
			limit, workers, perWorker, most, limit)
	}
}

func TestLoadShedFreesSlot(t *testing.T) {
	t.Run("after the handler returns", func(t *testing.T) {
		ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
		h := middleware.Chain(ok, middleware.LoadShed(1, time.Second))
		for i := range 20 {
			if w := serve(t, h, get("/")); w.status != http.StatusOK {
				t.Fatalf("LoadShed(1), request %d of 20 one after the other: status = %d, want %d", i+1, w.status, http.StatusOK)
			}
		}
	})

	t.Run("after the handler panics", func(t *testing.T) {
		// Without Recover in front, a panic goes through LoadShed. If the slot
		// leaks, every panic costs one slot until the service sheds everything.
		h := middleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/panic" {
				panic("handler bug")
			}
		}), middleware.LoadShed(1, time.Second))

		if v := callHandler(h, newSpy(), get("/panic")); v == nil {
			t.Fatalf("the handler panicked, but no panic came out of LoadShed (LoadShed must not recover)")
		}
		if w := serve(t, h, get("/")); w.status != http.StatusOK {
			t.Errorf("LoadShed(1): the request after a panic got status %d, want %d (the slot leaked)", w.status, http.StatusOK)
		}
	})
}

func TestLoadShedLimitIsShared(t *testing.T) {
	// One LoadShed value wraps two routes: they share its slots.
	shed := middleware.LoadShed(1, time.Second)
	g := newGate()
	slow := middleware.Chain(g, shed)
	fast := middleware.Chain(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}), shed)

	held := start(slow, get("/slow"))
	waitFor(t, "1 request is in the slow handler", func() bool { return g.entered.Load() == 1 })

	w := await(t, start(fast, get("/fast")), "request to the second route")
	if w.status != http.StatusServiceUnavailable {
		t.Errorf("2 routes share LoadShed(1), 1 request in flight: the second route answered %d, want %d", w.status, http.StatusServiceUnavailable)
	}
	close(g.release)
	await(t, held, "held request")
}
