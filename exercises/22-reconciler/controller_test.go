package reconciler_test

import (
	"context"
	"errors"
	"math/rand/v2"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	reconciler "github.com/yornik/go-practice/exercises/22-reconciler"
)

// constant returns a Desired func that always returns a copy of m.
func constant(m map[string]spec) func() map[string]spec {
	return newDesired(m).Get
}

func TestControllerRejectsBadConfig(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(c *reconciler.Controller)
	}{
		{"nil Desired", func(c *reconciler.Controller) { c.Desired = nil }},
		{"nil Store", func(c *reconciler.Controller) { c.Store = nil }},
		{"nil Queue", func(c *reconciler.Controller) { c.Queue = nil }},
		{"0 Workers", func(c *reconciler.Controller) { c.Workers = 0 }},
		{"0 Resync", func(c *reconciler.Controller) { c.Resync = 0 }},
		{"negative Resync", func(c *reconciler.Controller) { c.Resync = -time.Second }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &reconciler.Controller{
				Desired: constant(map[string]spec{"web": {"replicas": "3"}}),
				Store:   reconciler.NewMemoryStore(),
				Queue:   reconciler.NewQueue[string](time.Millisecond, time.Second),
				Workers: 1,
				Resync:  time.Second,
			}
			tc.mutate(c)
			r := startRun(t, c) // the context is never cancelled before Run returns
			select {
			case res := <-r.done:
				r.result = &res
				if res.panicked != nil {
					t.Fatalf("Run with %s panicked: %v, want an error", tc.name, res.panicked)
				}
				if res.err == nil {
					t.Fatalf("Run with %s returned nil, want an error", tc.name)
				}
			case <-time.After(timeout):
				t.Fatalf("Run with %s did not return within %v, want it to return an error at once", tc.name, timeout)
			}
		})
	}
}

func TestControllerConverges(t *testing.T) {
	hangGuard(t)
	inner := reconciler.NewMemoryStore()
	seed(t, inner, map[string]spec{
		"web": {"replicas": "1"}, // wrong spec
		"old": {"replicas": "1"}, // not desired any more
	})
	desired := map[string]spec{
		"web": {"replicas": "3"},
		"api": {"replicas": "2"}, // missing
	}
	c := &reconciler.Controller{
		Desired: constant(desired),
		Store:   inner,
		Queue:   reconciler.NewQueue[string](time.Millisecond, 100*time.Millisecond),
		Workers: 2,
		Resync:  time.Hour, // no resync during this test: only the start counts
	}
	r := startRun(t, c)
	r.waitConverged(t, inner, constant(desired),
		"With Resync = 1h only the names queued at the start get reconciled: does Run add every desired name and every name from List?")
	r.stop(t)
}

func TestControllerResyncFixesDrift(t *testing.T) {
	hangGuard(t)
	inner := reconciler.NewMemoryStore()
	desired := map[string]spec{
		"web": {"replicas": "3"},
		"api": {"replicas": "2"},
		"db":  {"version": "16"},
	}
	c := &reconciler.Controller{
		Desired: constant(desired),
		Store:   inner,
		Queue:   reconciler.NewQueue[string](time.Millisecond, 50*time.Millisecond),
		Workers: 2,
		Resync:  20 * time.Millisecond,
	}
	r := startRun(t, c)
	r.waitConverged(t, inner, constant(desired), "")

	// Someone changes the cluster by hand. No event tells the controller:
	// only the resync can find it.
	ctx := context.Background()
	if err := inner.Delete(ctx, "web"); err != nil {
		t.Fatalf("Delete(web) error = %v", err)
	}
	api := mustGet(t, inner, "api")
	api.Spec["replicas"] = "0"
	if _, err := inner.Update(ctx, api); err != nil {
		t.Fatalf("Update(api) error = %v", err)
	}
	seed(t, inner, map[string]spec{"debug-pod": {"image": "busybox"}})

	r.waitConverged(t, inner, constant(desired),
		"After a change behind the controller's back, the next resync must queue every desired and every actual name again.")
	r.stop(t)
}

func TestControllerFollowsDesiredChanges(t *testing.T) {
	hangGuard(t)
	inner := reconciler.NewMemoryStore()
	d := newDesired(map[string]spec{
		"web": {"replicas": "1"},
		"api": {"replicas": "1"},
	})
	c := &reconciler.Controller{
		Desired: d.Get,
		Store:   inner,
		Queue:   reconciler.NewQueue[string](time.Millisecond, 50*time.Millisecond),
		Workers: 2,
		Resync:  20 * time.Millisecond,
	}
	r := startRun(t, c)
	r.waitConverged(t, inner, d.Get, "")

	// A new commit lands in Git.
	d.Set("web", spec{"replicas": "5"})
	d.Remove("api")
	d.Set("db", spec{"version": "16"})

	r.waitConverged(t, inner, d.Get,
		"Run must call Desired for every reconcile and at every resync, not once at the start.")
	r.stop(t)
}

func TestControllerListFails(t *testing.T) {
	hangGuard(t)
	inner := reconciler.NewMemoryStore()
	s := &hookStore{inner: inner, hook: func(op, _ string) error {
		if op == "list" {
			return errUnavailable
		}
		return nil
	}}
	desired := map[string]spec{"web": {"replicas": "3"}}
	c := &reconciler.Controller{
		Desired: constant(desired),
		Store:   s,
		Queue:   reconciler.NewQueue[string](time.Millisecond, 50*time.Millisecond),
		Workers: 1,
		Resync:  10 * time.Millisecond,
	}
	r := startRun(t, c)
	r.waitConverged(t, inner, constant(desired),
		"When List fails, Run must still queue the desired names.")
	r.waitUntil(t, timeout, "List was called (and failed) 3 times: Run keeps running and tries again at every resync", func() bool {
		return s.Count("list", "") >= 3
	})
	r.stop(t)
}

// TestControllerRetriesWithBackoff uses a fake clock for the queue, so the
// test decides when a retry is due.
func TestControllerRetriesWithBackoff(t *testing.T) {
	hangGuard(t)
	inner := reconciler.NewMemoryStore()
	var failing atomic.Bool
	failing.Store(true)
	s := &hookStore{inner: inner, hook: func(op, _ string) error {
		if op == "get" && failing.Load() {
			return errUnavailable
		}
		return nil
	}}
	clock := &fakeClock{}
	q := reconciler.NewQueueWithClock[string](time.Second, time.Minute, clock)
	c := &reconciler.Controller{
		Desired: constant(map[string]spec{"web": {"replicas": "3"}}),
		Store:   s,
		Queue:   q,
		Workers: 1,
		Resync:  time.Hour, // only the retries bring web back
	}
	r := startRun(t, c)

	// Pending() >= 1 makes sure the retry is on the clock before the test
	// advances it.
	r.waitUntil(t, timeout, `the failed reconcile of "web" was retried with AddRateLimited (NumRequeues("web") == 1, a retry scheduled)`, func() bool {
		return q.NumRequeues("web") == 1 && clock.Pending() >= 1
	})
	time.Sleep(window)
	if n := s.Count("get", "web"); n != 1 {
		t.Fatalf(`Get("web") was called %d times before the backoff had passed, want 1: a failed reconcile must wait for its retry`, n)
	}

	clock.Advance(time.Second) // the 1st retry is due, and fails too
	r.waitUntil(t, timeout, `the 2nd failure was retried with AddRateLimited (NumRequeues("web") == 2, a retry scheduled)`, func() bool {
		return q.NumRequeues("web") == 2 && clock.Pending() >= 1
	})

	failing.Store(false)
	clock.Advance(2 * time.Second) // the 2nd retry is due, and succeeds
	r.waitUntil(t, timeout, `the successful reconcile called Forget (NumRequeues("web") == 0)`, func() bool {
		return q.NumRequeues("web") == 0
	})
	if got := mustGet(t, inner, "web"); got.Spec["replicas"] != "3" {
		t.Fatalf("web = %+v after the successful retry, want replicas 3", got)
	}
	r.stop(t)
}

func TestControllerUsesWorkers(t *testing.T) {
	hangGuard(t)
	inner := reconciler.NewMemoryStore()
	gate := make(chan struct{})
	openGate := sync.OnceFunc(func() { close(gate) })
	defer openGate()
	var inflight, peak atomic.Int32
	s := &hookStore{inner: inner, hook: func(op, _ string) error {
		if op != "get" {
			return nil
		}
		trackPeak(&peak, inflight.Add(1))
		defer inflight.Add(-1)
		select {
		case <-gate:
		case <-time.After(timeout):
		}
		return nil
	}}
	desired := map[string]spec{}
	for _, name := range names("app", 6) {
		desired[name] = spec{"replicas": "1"}
	}
	c := &reconciler.Controller{
		Desired: constant(desired),
		Store:   s,
		Queue:   reconciler.NewQueue[string](time.Millisecond, 50*time.Millisecond),
		Workers: 3,
		Resync:  time.Hour,
	}
	r := startRun(t, c)
	r.waitUntil(t, timeout, "3 reconciles ran at the same time (Workers = 3)", func() bool {
		return inflight.Load() == 3
	})
	time.Sleep(window)
	if p := peak.Load(); p > 3 {
		t.Fatalf("%d reconciles ran at the same time, want at most Workers = 3", p)
	}
	openGate()
	r.waitConverged(t, inner, constant(desired), "")
	r.stop(t)
}

func TestControllerShutdownWaitsForWorkers(t *testing.T) {
	hangGuard(t)
	inner := reconciler.NewMemoryStore()
	gate := make(chan struct{})
	openGate := sync.OnceFunc(func() { close(gate) })
	defer openGate()
	entered := make(chan struct{}, 1)
	s := &hookStore{inner: inner, hook: func(op, _ string) error {
		if op == "get" {
			select {
			case entered <- struct{}{}:
			default:
			}
			// A slow API call that ignores ctx.
			select {
			case <-gate:
			case <-time.After(timeout):
			}
		}
		return nil
	}}
	c := &reconciler.Controller{
		Desired: constant(map[string]spec{"web": {"replicas": "3"}}),
		Store:   s,
		Queue:   reconciler.NewQueue[string](time.Millisecond, 50*time.Millisecond),
		Workers: 1,
		Resync:  time.Hour,
	}
	r := startRun(t, c)
	select {
	case <-entered:
	case <-time.After(timeout):
		t.Fatalf("no reconcile started within %v", timeout)
	}

	r.cancel()
	select {
	case res := <-r.done:
		r.result = &res
		t.Fatalf("Run returned (%v, panic %v) while a reconcile was still running: it must wait for its workers", res.err, res.panicked)
	case <-time.After(window):
	}

	openGate()
	select {
	case res := <-r.done:
		r.result = &res
		if res.panicked != nil || res.err != nil {
			t.Fatalf("Run returned %v (panic %v), want nil", res.err, res.panicked)
		}
	case <-time.After(timeout):
		t.Fatalf("Run did not return within %v after the last reconcile finished", timeout)
	}

	// Nothing runs after Run has returned.
	n := len(s.Calls())
	time.Sleep(window)
	if calls := s.Calls(); len(calls) != n {
		t.Fatalf("the store was called after Run returned: %v", calls[n:])
	}
}

// TestControllerShutdownWaitsForResync checks that Run waits for the resync
// too, not only for the workers: a List that is still running when ctx is
// cancelled must end before Run returns.
func TestControllerShutdownWaitsForResync(t *testing.T) {
	hangGuard(t)
	inner := reconciler.NewMemoryStore()
	gate := make(chan struct{})
	openGate := sync.OnceFunc(func() { close(gate) })
	defer openGate()
	entered := make(chan struct{}, 1)
	var lists atomic.Int32
	s := &hookStore{inner: inner, hook: func(op, _ string) error {
		if op == "list" && lists.Add(1) == 2 { // the first resync after the start
			entered <- struct{}{}
			// A slow API call that ignores ctx.
			select {
			case <-gate:
			case <-time.After(timeout):
			}
		}
		return nil
	}}
	c := &reconciler.Controller{
		Desired: constant(map[string]spec{"web": {"replicas": "3"}}),
		Store:   s,
		Queue:   reconciler.NewQueue[string](time.Millisecond, 50*time.Millisecond),
		Workers: 1,
		Resync:  10 * time.Millisecond,
	}
	r := startRun(t, c)
	select {
	case <-entered:
	case <-time.After(timeout):
		t.Fatalf("no resync called List within %v (Resync = 10ms)", timeout)
	}

	r.cancel()
	select {
	case res := <-r.done:
		r.result = &res
		t.Fatalf("Run returned (%v, panic %v) while a resync was still in Store.List: it must wait for every goroutine it started", res.err, res.panicked)
	case <-time.After(window):
	}

	openGate()
	select {
	case res := <-r.done:
		r.result = &res
		if res.panicked != nil || res.err != nil {
			t.Fatalf("Run returned %v (panic %v), want nil", res.err, res.panicked)
		}
	case <-time.After(timeout):
		t.Fatalf("Run did not return within %v after the slow List returned", timeout)
	}

	n := len(s.Calls())
	time.Sleep(window)
	if calls := s.Calls(); len(calls) != n {
		t.Fatalf("the store was called after Run returned: %v", calls[n:])
	}
}

func TestControllerCtxAlreadyDone(t *testing.T) {
	hangGuard(t)
	inner := reconciler.NewMemoryStore()
	seed(t, inner, map[string]spec{"web": {"replicas": "1"}, "old": {"replicas": "1"}})
	s := &hookStore{inner: inner}
	c := &reconciler.Controller{
		Desired: constant(map[string]spec{"web": {"replicas": "3"}, "api": {"replicas": "1"}}),
		Store:   s,
		Queue:   reconciler.NewQueue[string](time.Millisecond, 50*time.Millisecond),
		Workers: 2,
		Resync:  time.Hour,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var err error
	within(t, "Run with a context that is already done", func() { err = c.Run(ctx) })
	if err != nil {
		t.Fatalf("Run with a done context returned %v, want nil", err)
	}
	if w := s.Writes(); len(w) != 0 {
		t.Fatalf("Run with a done context wrote %v, want no write: no reconcile may start after ctx is done", w)
	}
}

// ----- the convergence test -----

var errFlaky = errors.New("flaky store: 503 service unavailable")

// flakyStore fails 30% of the calls, like an API server during a bad
// rollout. Half of the failed writes are applied anyway: the request timed
// out after the server committed it, so the client sees an error for a
// write that happened. It also checks that no two calls for the same name
// overlap, which would mean two workers reconcile the same name at once.
type flakyStore struct {
	inner reconciler.Store

	mu       sync.Mutex
	rng      *rand.Rand
	failed   int
	inflight map[string]int
	overlaps []string
}

func newFlakyStore(inner reconciler.Store) *flakyStore {
	return &flakyStore{
		inner:    inner,
		rng:      rand.New(rand.NewPCG(22, 2026)), // seeded: the same sequence every run
		inflight: map[string]int{},
	}
}

type outcome int

const (
	succeed    outcome = iota
	failBefore         // the call never reaches the store
	failAfter          // the call is applied, but the caller gets an error
)

// enter marks a call for name as running and rolls its outcome. Call the
// returned func when the call is done.
func (s *flakyStore) enter(name string) (outcome, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name != "" {
		if s.inflight[name] > 0 {
			s.overlaps = append(s.overlaps, name)
		}
		s.inflight[name]++
	}
	res := succeed
	switch r := s.rng.Float64(); {
	case r < 0.15:
		res = failBefore
	case r < 0.30:
		res = failAfter
	}
	if res != succeed {
		s.failed++
	}
	return res, func() {
		runtime.Gosched() // keep the call open a little, so overlaps show up
		s.mu.Lock()
		defer s.mu.Unlock()
		if name != "" {
			s.inflight[name]--
		}
	}
}

func (s *flakyStore) Get(ctx context.Context, name string) (reconciler.Object, error) {
	res, leave := s.enter(name)
	defer leave()
	if res != succeed {
		return reconciler.Object{}, errFlaky
	}
	return s.inner.Get(ctx, name)
}

func (s *flakyStore) List(ctx context.Context) ([]reconciler.Object, error) {
	res, leave := s.enter("")
	defer leave()
	if res != succeed {
		return nil, errFlaky
	}
	return s.inner.List(ctx)
}

func (s *flakyStore) Create(ctx context.Context, obj reconciler.Object) (reconciler.Object, error) {
	res, leave := s.enter(obj.Name)
	defer leave()
	switch res {
	case failBefore:
		return reconciler.Object{}, errFlaky
	case failAfter:
		_, _ = s.inner.Create(ctx, obj)
		return reconciler.Object{}, errFlaky
	}
	return s.inner.Create(ctx, obj)
}

func (s *flakyStore) Update(ctx context.Context, obj reconciler.Object) (reconciler.Object, error) {
	res, leave := s.enter(obj.Name)
	defer leave()
	switch res {
	case failBefore:
		return reconciler.Object{}, errFlaky
	case failAfter:
		_, _ = s.inner.Update(ctx, obj)
		return reconciler.Object{}, errFlaky
	}
	return s.inner.Update(ctx, obj)
}

func (s *flakyStore) Delete(ctx context.Context, name string) error {
	res, leave := s.enter(name)
	defer leave()
	switch res {
	case failBefore:
		return errFlaky
	case failAfter:
		_ = s.inner.Delete(ctx, name)
		return errFlaky
	}
	return s.inner.Delete(ctx, name)
}

// TestControllerConvergesDespiteFailures is the test the whole exercise is
// for: 30 desired objects, a store that fails 30% of the calls, watch events
// that keep adding names, and 4 workers. The controller must get there.
// Run it with -race.
func TestControllerConvergesDespiteFailures(t *testing.T) {
	hangGuard(t)
	inner := reconciler.NewMemoryStore()
	existing := map[string]spec{}
	for _, name := range names("app", 10) {
		existing[name] = spec{"replicas": "1", "image": "v1"} // outdated
	}
	for _, name := range names("old", 5) {
		existing[name] = spec{"replicas": "1"} // not desired any more
	}
	seed(t, inner, existing)
	desired := map[string]spec{}
	all := names("app", 30)
	for _, name := range all {
		desired[name] = spec{"replicas": "3", "image": "v2"}
	}

	fs := newFlakyStore(inner)
	q := reconciler.NewQueue[string](time.Millisecond, 20*time.Millisecond)
	c := &reconciler.Controller{
		Desired: constant(desired),
		Store:   fs,
		Queue:   q,
		Workers: 4,
		Resync:  50 * time.Millisecond,
	}
	r := startRun(t, c)

	// Watch events: like an informer, call Add while the controller runs.
	stopEvents := make(chan struct{})
	eventsDone := make(chan struct{})
	go func() {
		defer close(eventsDone)
		for i := 0; ; i++ {
			select {
			case <-stopEvents:
				return
			default:
			}
			q.Add(all[i%len(all)])
			time.Sleep(50 * time.Microsecond)
		}
	}()

	r.waitConverged(t, inner, constant(desired),
		"A failed reconcile must be retried (AddRateLimited), and every worker must call Done, or the name is stuck.")
	close(stopEvents)
	<-eventsDone
	r.stop(t)

	fs.mu.Lock()
	defer fs.mu.Unlock()
	if fs.failed == 0 {
		t.Fatalf("the flaky store never failed: the test itself is broken")
	}
	if len(fs.overlaps) > 0 {
		t.Fatalf("%d times 2 workers called the store for the same name at once, e.g. %q: a name must never be reconciled by 2 workers at the same time",
			len(fs.overlaps), fs.overlaps[0])
	}
}
