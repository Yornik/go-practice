package reconciler_test

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"runtime/debug"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	reconciler "github.com/yornik/go-practice/exercises/47-reconciler"
)

// timeout bounds every blocking step in these tests, so a wrong solution
// fails with a message instead of hanging until the test binary is killed.
const timeout = 5 * time.Second

// convergeTimeout is how long a controller test waits for the store to match
// the desired state. A correct solution needs well under a second; the rest
// is room for a slow, loaded machine.
const convergeTimeout = 10 * time.Second

// window is how long a test watches for something that must NOT happen, such
// as Get returning a key that is still processing. A correct solution never
// does it, so the window only decides how likely it is that a wrong solution
// gets caught. It never makes a correct solution fail.
const window = 100 * time.Millisecond

// hangTimeout is how long a test may run before hangGuard stops the test
// binary. It is far above the slowest failing path of any test (about 30 s),
// so it only fires when something is stuck for good.
const hangTimeout = 60 * time.Second

// errUnavailable is what a failing store call returns in these tests, like
// an API server that answers 503 during a rollout.
var errUnavailable = errors.New("503 service unavailable")

// hangGuard makes the test binary panic, with the stack of every goroutine,
// if the test is still running after hangTimeout. The tests call methods such
// as Add, Done or Create directly, because those must never block. If one
// does block, it almost always waits for a mutex that the same goroutine
// already holds (sync.Mutex is not reentrant). Without this guard the run
// would hang silently until go test gives up after 10 minutes.
func hangGuard(t *testing.T) {
	t.Helper()
	name := t.Name()
	timer := time.AfterFunc(hangTimeout, func() {
		debug.SetTraceback("all")
		panic(fmt.Sprintf("%s is still running after %v: a call that must not block is stuck. "+
			"Find the goroutine of the test below: if it waits in sync.(*Mutex).Lock or sync.(*RWMutex).Lock, "+
			"a method that already holds the lock tried to lock it again", name, hangTimeout))
	})
	t.Cleanup(func() { timer.Stop() })
}

// ----- fake clock -----

// fakeClock is a reconciler.Clock that only moves when the test calls
// Advance.
type fakeClock struct {
	mu     sync.Mutex
	now    time.Duration // time since the clock was created
	timers []fakeTimer
}

type fakeTimer struct {
	at time.Duration
	f  func()
}

// AfterFunc implements reconciler.Clock. It never calls f itself.
func (c *fakeClock) AfterFunc(d time.Duration, f func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timers = append(c.timers, fakeTimer{at: c.now + d, f: f})
}

// Advance moves the clock forward by d. Every function whose time has come
// runs before Advance returns, in the order of their times, in the
// goroutine that called Advance.
func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now += d
	var due, later []fakeTimer
	for _, tm := range c.timers {
		if tm.at <= c.now {
			due = append(due, tm)
		} else {
			later = append(later, tm)
		}
	}
	c.timers = later
	c.mu.Unlock()

	// Run them without holding c.mu: they call into the queue, and the
	// queue may call AfterFunc while it holds its own lock.
	slices.SortStableFunc(due, func(a, b fakeTimer) int { return cmp.Compare(a.at, b.at) })
	for _, tm := range due {
		tm.f()
	}
}

// Pending returns how many functions wait for their time.
func (c *fakeClock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

// ----- queue helpers -----

// newFakeQueue returns a queue with a fake clock, a base delay of 1 s and a
// max delay of 1 min.
func newFakeQueue() (*reconciler.Queue[string], *fakeClock) {
	clock := &fakeClock{}
	return reconciler.NewQueueWithClock[string](time.Second, time.Minute, clock), clock
}

type getResult struct {
	key      string
	ok       bool
	panicked any
}

// startGet calls q.Get in a new goroutine. The channel receives the result
// once Get returns.
func startGet(q *reconciler.Queue[string]) <-chan getResult {
	ch := make(chan getResult, 1)
	go func() {
		var res getResult
		defer func() {
			res.panicked = recover()
			ch <- res
		}()
		res.key, res.ok = q.Get()
	}()
	return ch
}

// awaitGet waits for the result of startGet. It fails the test if Get
// panicked or did not return within the timeout.
func awaitGet(t *testing.T, ch <-chan getResult, what string) (string, bool) {
	t.Helper()
	select {
	case res := <-ch:
		if res.panicked != nil {
			t.Fatalf("Get panicked: %v", res.panicked)
		}
		return res.key, res.ok
	case <-time.After(timeout):
		t.Fatalf("Get did not return within %v: %s", timeout, what)
		return "", false
	}
}

// getNow calls q.Get when keys are waiting or the queue is shut down, so Get
// must not block.
func getNow(t *testing.T, q *reconciler.Queue[string]) (string, bool) {
	t.Helper()
	return awaitGet(t, startGet(q), "it blocked, although keys were waiting or the queue was shut down")
}

// expectBlocked fails the test if the Get started with startGet returns
// within the window.
func expectBlocked(t *testing.T, ch <-chan getResult, why string) {
	t.Helper()
	select {
	case res := <-ch:
		if res.panicked != nil {
			t.Fatalf("Get panicked: %v", res.panicked)
		}
		t.Fatalf("Get returned (%q, %v), want it to block: %s", res.key, res.ok, why)
	case <-time.After(window):
	}
}

// expectLen fails the test if q.Len() is not want.
func expectLen(t *testing.T, q *reconciler.Queue[string], want int, when string) {
	t.Helper()
	if got := q.Len(); got != want {
		t.Fatalf("Len() = %d %s, want %d", got, when, want)
	}
}

// within runs f in a new goroutine and waits for it. It fails the test if f
// panics or does not return within the timeout.
func within(t *testing.T, what string, f func()) {
	t.Helper()
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		f()
	}()
	select {
	case v := <-done:
		if v != nil {
			t.Fatalf("%s panicked: %v", what, v)
		}
	case <-time.After(timeout):
		t.Fatalf("%s did not return within %v", what, timeout)
	}
}

// waitFor polls cond until it returns true. It fails the test if that takes
// longer than the timeout.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting until %s", timeout, what)
		}
		time.Sleep(time.Millisecond)
	}
}

// trackPeak raises peak to cur if cur is larger. Safe for concurrent use.
func trackPeak(peak *atomic.Int32, cur int32) {
	for {
		p := peak.Load()
		if cur <= p || peak.CompareAndSwap(p, cur) {
			return
		}
	}
}

// ----- store helpers -----

type spec = map[string]string

// seed creates objects in s directly.
func seed(t *testing.T, s reconciler.Store, objects map[string]spec) {
	t.Helper()
	for _, name := range slices.Sorted(maps.Keys(objects)) {
		if _, err := s.Create(context.Background(), reconciler.Object{Name: name, Spec: objects[name]}); err != nil {
			t.Fatalf("seed: Create(%q) error = %v", name, err)
		}
	}
}

// mustGet returns the object called name from s.
func mustGet(t *testing.T, s reconciler.Store, name string) reconciler.Object {
	t.Helper()
	obj, err := s.Get(context.Background(), name)
	if err != nil {
		t.Fatalf("Get(%q) error = %v, want nil", name, err)
	}
	return obj
}

// expectGone fails the test if s has an object called name.
func expectGone(t *testing.T, s reconciler.Store, name string) {
	t.Helper()
	if obj, err := s.Get(context.Background(), name); !errors.Is(err, reconciler.ErrNotFound) {
		t.Fatalf("Get(%q) = %+v, %v, want an error that wraps ErrNotFound", name, obj, err)
	}
}

// diff returns "" if the objects in s are exactly the desired ones with the
// desired specs, and otherwise describes the first difference.
func diff(s reconciler.Store, desired map[string]spec) string {
	objs, err := s.List(context.Background())
	if err != nil {
		return fmt.Sprintf("List error: %v", err)
	}
	actual := map[string]spec{}
	for _, obj := range objs {
		actual[obj.Name] = obj.Spec
	}
	for _, name := range slices.Sorted(maps.Keys(desired)) {
		got, ok := actual[name]
		if !ok {
			return fmt.Sprintf("%q is desired but does not exist", name)
		}
		if !maps.Equal(got, desired[name]) {
			return fmt.Sprintf("%q has spec %v, want %v", name, got, desired[name])
		}
	}
	for _, name := range slices.Sorted(maps.Keys(actual)) {
		if _, ok := desired[name]; !ok {
			return fmt.Sprintf("%q exists but is not desired", name)
		}
	}
	return ""
}

// storeCall is one call that reached a hookStore.
type storeCall struct {
	Op              string // "get", "list", "create", "update" or "delete"
	Name            string // "" for list
	ResourceVersion int64  // the version an update sent
}

// hookStore wraps a Store and records every call. If hook is set, it runs
// before the call is passed on: if it returns an error, the call fails with
// that error and does not reach inner. A hook can also change inner first,
// to play "someone else changed the object just now".
type hookStore struct {
	inner reconciler.Store
	hook  func(op, name string) error

	mu    sync.Mutex
	calls []storeCall
}

func (s *hookStore) record(c storeCall) error {
	s.mu.Lock()
	s.calls = append(s.calls, c)
	s.mu.Unlock()
	if s.hook != nil {
		return s.hook(c.Op, c.Name)
	}
	return nil
}

func (s *hookStore) Get(ctx context.Context, name string) (reconciler.Object, error) {
	if err := s.record(storeCall{Op: "get", Name: name}); err != nil {
		return reconciler.Object{}, err
	}
	return s.inner.Get(ctx, name)
}

func (s *hookStore) List(ctx context.Context) ([]reconciler.Object, error) {
	if err := s.record(storeCall{Op: "list"}); err != nil {
		return nil, err
	}
	return s.inner.List(ctx)
}

func (s *hookStore) Create(ctx context.Context, obj reconciler.Object) (reconciler.Object, error) {
	if err := s.record(storeCall{Op: "create", Name: obj.Name}); err != nil {
		return reconciler.Object{}, err
	}
	return s.inner.Create(ctx, obj)
}

func (s *hookStore) Update(ctx context.Context, obj reconciler.Object) (reconciler.Object, error) {
	if err := s.record(storeCall{Op: "update", Name: obj.Name, ResourceVersion: obj.ResourceVersion}); err != nil {
		return reconciler.Object{}, err
	}
	return s.inner.Update(ctx, obj)
}

func (s *hookStore) Delete(ctx context.Context, name string) error {
	if err := s.record(storeCall{Op: "delete", Name: name}); err != nil {
		return err
	}
	return s.inner.Delete(ctx, name)
}

// Calls returns a copy of the calls so far.
func (s *hookStore) Calls() []storeCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.calls)
}

// Writes returns the create, update and delete calls so far.
func (s *hookStore) Writes() []storeCall {
	var w []storeCall
	for _, c := range s.Calls() {
		if c.Op == "create" || c.Op == "update" || c.Op == "delete" {
			w = append(w, c)
		}
	}
	return w
}

// Count returns how many calls of op for name were made so far.
func (s *hookStore) Count(op, name string) int {
	n := 0
	for _, c := range s.Calls() {
		if c.Op == op && c.Name == name {
			n++
		}
	}
	return n
}

// ----- controller helpers -----

// desiredState is a desired state that the test can change while the
// controller runs, like a Git repository that gets a new commit.
type desiredState struct {
	mu sync.Mutex
	m  map[string]spec
}

func newDesired(m map[string]spec) *desiredState {
	return &desiredState{m: m}
}

// Get returns a deep copy, so the controller never sees a map that changes.
// It is the controller's Desired func.
func (d *desiredState) Get() map[string]spec {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := make(map[string]spec, len(d.m))
	for name, sp := range d.m {
		out[name] = maps.Clone(sp)
	}
	return out
}

func (d *desiredState) Set(name string, sp spec) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.m[name] = sp
}

func (d *desiredState) Remove(name string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.m, name)
}

// running is a Controller.Run started by startRun.
type running struct {
	done   chan runResult
	cancel context.CancelFunc
	result *runResult // set once Run has returned and the result was read
}

type runResult struct {
	err      error
	panicked any
}

// startRun calls c.Run in a new goroutine with a context that stop or the
// end of the test cancels.
func startRun(t *testing.T, c *reconciler.Controller) *running {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &running{done: make(chan runResult, 1), cancel: cancel}
	go func() {
		var res runResult
		defer func() {
			res.panicked = recover()
			r.done <- res
		}()
		res.err = c.Run(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		if r.result == nil {
			select {
			case <-r.done:
			case <-time.After(timeout):
				t.Errorf("Run did not return within %v after its context was cancelled", timeout)
			}
		}
	})
	return r
}

// stop cancels Run's context and checks that Run returns nil.
func (r *running) stop(t *testing.T) {
	t.Helper()
	r.cancel()
	select {
	case res := <-r.done:
		r.result = &res
		if res.panicked != nil {
			t.Fatalf("Run panicked: %v", res.panicked)
		}
		if res.err != nil {
			t.Fatalf("Run returned %v after its context was cancelled, want nil", res.err)
		}
	case <-time.After(timeout):
		t.Fatalf("Run did not return within %v after its context was cancelled", timeout)
	}
}

// waitUntil polls cond while Run runs. It fails the test if Run returns
// before cond holds, or if that takes longer than limit.
func (r *running) waitUntil(t *testing.T, limit time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !cond() {
		select {
		case res := <-r.done:
			r.result = &res
			if res.panicked != nil {
				t.Fatalf("Run panicked: %v", res.panicked)
			}
			t.Fatalf("Run returned %v before %s; it must run until its context is cancelled", res.err, what)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out after %v waiting until %s", limit, what)
		}
		time.Sleep(time.Millisecond)
	}
}

// waitConverged waits until the objects in s match desired().
func (r *running) waitConverged(t *testing.T, s reconciler.Store, desired func() map[string]spec, hint string) {
	t.Helper()
	last := ""
	deadline := time.Now().Add(convergeTimeout)
	for {
		last = diff(s, desired())
		if last == "" {
			return
		}
		select {
		case res := <-r.done:
			r.result = &res
			if res.panicked != nil {
				t.Fatalf("Run panicked: %v", res.panicked)
			}
			t.Fatalf("Run returned %v before the store matched the desired state; it must run until its context is cancelled", res.err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("the store did not match the desired state within %v: %s. %s", convergeTimeout, last, hint)
		}
		time.Sleep(time.Millisecond)
	}
}

// names returns name-00, name-01, ... name-(n-1).
func names(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s-%02d", prefix, i)
	}
	return out
}
