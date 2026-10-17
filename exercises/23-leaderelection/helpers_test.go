package leaderelection_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	leaderelection "github.com/yornik/go-practice/exercises/23-leaderelection"
)

// timeout bounds every wait in these tests, so a wrong solution fails with a
// message instead of hanging until the test binary is killed.
const timeout = 5 * time.Second

// window is how long a test watches for something that must NOT happen. A
// correct solution never does it, so the window only decides how likely it
// is that a wrong solution gets caught. It never makes a correct one fail.
const window = 100 * time.Millisecond

// drain is how long the tests' OnStartedLeading needs to wind down after its
// ctx is cancelled, like a controller that finishes its current reconcile.
const drain = 20 * time.Millisecond

// The durations of most tests. LeaseDuration and RenewDeadline are measured
// with the fake clock, which only moves when a test calls Advance.
// RetryPeriod is real time: candidates retry every 5 ms.
const (
	leaseDuration = 10 * time.Second
	renewDeadline = 5 * time.Second
	retryPeriod   = 5 * time.Millisecond
)

// t0 is the time on the fake clock when a test starts.
var t0 = time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)

// at returns t0 plus sec seconds.
func at(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }

// ----- fake clock -----

// fakeClock is a clock that moves only when the test says so. Its Now
// method is passed as Config.Now. Safe for concurrent use.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// ----- the network between one candidate and the store -----

// errUnreachable is what the store calls of a cut-off candidate return.
var errUnreachable = errors.New("dial tcp 10.96.0.1:443: connect: connection refused")

type link int32

const (
	healthy link = iota
	failing      // every call fails at once with errUnreachable
	hanging      // every call blocks until its ctx is done, then returns ctx.Err()
)

// netStore is one candidate's connection to the shared store. A test can
// cut it off without affecting the other candidates. It counts calls.
type netStore struct {
	inner    leaderelection.LeaseStore
	link     atomic.Int32
	failNext atomic.Int32 // this many more calls fail, then the link works
	calls    atomic.Int64
}

func (s *netStore) set(l link) { s.link.Store(int32(l)) }

func (s *netStore) fault(ctx context.Context) error {
	s.calls.Add(1)
	for {
		n := s.failNext.Load()
		if n <= 0 {
			break
		}
		if s.failNext.CompareAndSwap(n, n-1) {
			return errUnreachable
		}
	}
	switch link(s.link.Load()) {
	case failing:
		return errUnreachable
	case hanging:
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (s *netStore) Get(ctx context.Context) (leaderelection.Lease, error) {
	if err := s.fault(ctx); err != nil {
		return leaderelection.Lease{}, err
	}
	return s.inner.Get(ctx)
}

func (s *netStore) Update(ctx context.Context, l leaderelection.Lease) (leaderelection.Lease, error) {
	if err := s.fault(ctx); err != nil {
		return leaderelection.Lease{}, err
	}
	return s.inner.Update(ctx, l)
}

// ----- the ledger: who leads when -----

// ledger watches the leadership of all candidates of a test. A candidate
// leads from the moment its OnStartedLeading is called until that call
// returns. Two candidates must never lead at the same time.
type ledger struct {
	mu       sync.Mutex
	leading  map[string]uint64 // candidate ID -> epoch, while it leads
	epochs   []uint64          // epochs in the order leadership started
	overlaps []string
}

func (l *ledger) started(id string, epoch uint64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for other, e := range l.leading {
		l.overlaps = append(l.overlaps, fmt.Sprintf("%s started leading (epoch %d) while %s (epoch %d) was still leading", id, epoch, other, e))
	}
	if l.leading == nil {
		l.leading = make(map[string]uint64)
	}
	l.leading[id] = epoch
	l.epochs = append(l.epochs, epoch)
}

func (l *ledger) stopped(id string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.leading, id)
}

func (l *ledger) startedEpochs() []uint64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return slices.Clone(l.epochs)
}

// ----- candidates -----

// candidate is one call of Run in its own goroutine. Its callbacks record
// what Run did, and check the order of the calls.
type candidate struct {
	id     string
	cl     *cluster
	store  *netStore // this candidate's connection to the shared store
	cancel context.CancelFunc
	done   chan struct{} // closed when Run has returned
	err    error         // what Run returned; read it only after done is closed
	panic  any           // what Run panicked with; same rule

	panicReported bool // the test already reported the panic

	mu           sync.Mutex
	epochs       []uint64        // one per OnStartedLeading call
	leaderCtx    context.Context // the ctx OnStartedLeading got
	returned     bool            // OnStartedLeading has returned
	stops        int             // OnStoppedLeading calls
	holderAtStop string          // the store's Holder when OnStoppedLeading was called
	newLeaders   []string        // OnNewLeader calls
	problems     []string
}

// markStarted records a call of OnStartedLeading. Custom callbacks call it
// first.
func (c *candidate) markStarted(ctx context.Context, epoch uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.epochs = append(c.epochs, epoch)
	c.leaderCtx = ctx
	if len(c.epochs) > 1 {
		c.problems = append(c.problems, fmt.Sprintf("OnStartedLeading was called %d times, want at most once per call of Run", len(c.epochs)))
	}
}

// markReturned records that OnStartedLeading returns. Custom callbacks call
// it last.
func (c *candidate) markReturned() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.returned = true
}

// onStartedLeading is the default OnStartedLeading: lead until ctx is
// cancelled, then take drain to wind down.
func (c *candidate) onStartedLeading(ctx context.Context, epoch uint64) {
	c.markStarted(ctx, epoch)
	c.cl.ledger.started(c.id, epoch)
	<-ctx.Done()
	time.Sleep(drain)
	c.cl.ledger.stopped(c.id)
	c.markReturned()
}

func (c *candidate) onStoppedLeading() {
	holder := "(store error)"
	if l, err := c.cl.store.Get(context.Background()); err == nil {
		holder = l.Holder
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stops++
	c.holderAtStop = holder
	switch {
	case c.stops > 1:
		c.problems = append(c.problems, fmt.Sprintf("OnStoppedLeading was called %d times, want once", c.stops))
	case len(c.epochs) == 0:
		c.problems = append(c.problems, "OnStoppedLeading was called, but OnStartedLeading never was: call it only when leadership ends")
	case !c.returned:
		c.problems = append(c.problems, "OnStoppedLeading was called before OnStartedLeading returned: wait for it first")
	}
}

func (c *candidate) onNewLeader(id string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.newLeaders = append(c.newLeaders, id)
}

// run starts Run for c. adjust can change the default config first.
func (c *candidate) run(adjust ...func(*leaderelection.Config)) {
	cfg := leaderelection.Config{
		ID:               c.id,
		Store:            c.store,
		LeaseDuration:    leaseDuration,
		RenewDeadline:    renewDeadline,
		RetryPeriod:      retryPeriod,
		OnStartedLeading: c.onStartedLeading,
		OnStoppedLeading: c.onStoppedLeading,
		OnNewLeader:      c.onNewLeader,
	}
	if c.cl.clock != nil {
		cfg.Now = c.cl.clock.Now
	}
	for _, f := range adjust {
		f(&cfg)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.cancel = cancel
	go func() {
		defer close(c.done)
		defer func() { c.panic = recover() }()
		c.err = leaderelection.Run(ctx, cfg)
	}()

	t := c.cl.t
	t.Cleanup(func() {
		c.cancel()
		select {
		case <-c.done:
		case <-time.After(timeout):
			t.Errorf("Run(%s) did not return within %v after its ctx was cancelled", c.id, timeout)
			return
		}
		if c.panic != nil && !c.panicReported {
			t.Errorf("Run(%s) panicked: %v", c.id, c.panic)
		}
		c.mu.Lock()
		defer c.mu.Unlock()
		for _, p := range c.problems {
			t.Errorf("%s: %s", c.id, p)
		}
	})
}

// started reports whether OnStartedLeading has been called.
func (c *candidate) started() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.epochs) > 0
}

func (c *candidate) startedEpochs() []uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.epochs)
}

// ctx returns the ctx OnStartedLeading got, or nil.
func (c *candidate) ctx() context.Context {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.leaderCtx
}

func (c *candidate) stopCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stops
}

func (c *candidate) reported() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return slices.Clone(c.newLeaders)
}

func (c *candidate) exited() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

// result waits until Run returns and returns its error.
func (c *candidate) result(t *testing.T, why string) error {
	t.Helper()
	select {
	case <-c.done:
	case <-time.After(timeout):
		t.Fatalf("Run(%s) did not return within %v, want it to return because %s", c.id, timeout, why)
	}
	if c.panic != nil {
		c.panicReported = true
		t.Fatalf("Run(%s) panicked: %v", c.id, c.panic)
	}
	return c.err
}

// wantLeaderCtxDone fails the test if the ctx that c's OnStartedLeading got
// is not cancelled.
func (c *candidate) wantLeaderCtxDone(t *testing.T) {
	t.Helper()
	ctx := c.ctx()
	if ctx == nil {
		t.Fatalf("%s: OnStartedLeading was never called", c.id)
	}
	if ctx.Err() == nil {
		t.Errorf("%s: the ctx passed to OnStartedLeading is not cancelled after leadership ended", c.id)
	}
}

// ----- the cluster: candidates that share one store -----

type cluster struct {
	t      *testing.T
	clock  *fakeClock // nil: the real clock
	store  *leaderelection.MemoryStore
	ledger ledger
	cands  []*candidate
}

// newCluster returns a cluster with a fake clock at t0 and an empty store.
// When the test ends, it checks that leadership never overlapped.
func newCluster(t *testing.T) *cluster {
	t.Helper()
	cl := &cluster{t: t, clock: &fakeClock{now: t0}, store: leaderelection.NewMemoryStore()}
	t.Cleanup(func() {
		cl.ledger.mu.Lock()
		defer cl.ledger.mu.Unlock()
		for _, o := range cl.ledger.overlaps {
			t.Errorf("two leaders at once: %s", o)
		}
	})
	return cl
}

// newRealCluster returns a cluster that uses the real clock. With real time,
// a loaded machine can pause any goroutine, so it does not check overlaps.
func newRealCluster(t *testing.T) *cluster {
	t.Helper()
	return &cluster{t: t, store: leaderelection.NewMemoryStore()}
}

// add creates a candidate without starting it.
func (cl *cluster) add(id string) *candidate {
	c := &candidate{id: id, cl: cl, store: &netStore{inner: cl.store}, done: make(chan struct{})}
	cl.cands = append(cl.cands, c)
	return c
}

// start creates a candidate and starts its Run.
func (cl *cluster) start(id string, adjust ...func(*leaderelection.Config)) *candidate {
	c := cl.add(id)
	c.run(adjust...)
	return c
}

// panicked fails the test at once if the Run of a candidate panicked.
func (cl *cluster) panicked() {
	cl.t.Helper()
	for _, c := range cl.cands {
		if c.exited() && c.panic != nil {
			c.panicReported = true
			cl.t.Fatalf("Run(%s) panicked: %v", c.id, c.panic)
		}
	}
}

// eventually waits until cond is true. It fails the test after timeout, or
// as soon as a Run panicked.
func (cl *cluster) eventually(what string, cond func() bool) {
	cl.t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		cl.panicked()
		if time.Now().After(deadline) {
			cl.t.Fatalf("still waiting after %v until %s", timeout, what)
		}
		time.Sleep(time.Millisecond)
	}
}

// stays checks for window that cond stays true.
func (cl *cluster) stays(what string, cond func() bool) {
	cl.t.Helper()
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) {
		cl.panicked()
		if !cond() {
			cl.t.Fatalf("want: %s; it did not hold", what)
		}
		time.Sleep(time.Millisecond)
	}
}

// leader waits until one of cands has started leading and returns it. It
// fails the test if more than one has. what describes the wait.
func (cl *cluster) leader(what string, cands ...*candidate) *candidate {
	cl.t.Helper()
	cl.eventually(what, func() bool {
		return slices.ContainsFunc(cands, (*candidate).started)
	})
	var leaders []string
	var leader *candidate
	for _, c := range cands {
		if c.started() {
			leaders = append(leaders, c.id)
			leader = c
		}
	}
	if len(leaders) > 1 {
		cl.t.Fatalf("%v all started leading, want exactly one", leaders)
	}
	return leader
}

// lease reads the lease directly from the shared store.
func (cl *cluster) lease() leaderelection.Lease {
	cl.t.Helper()
	l, err := cl.store.Get(context.Background())
	if err != nil {
		cl.t.Fatalf("MemoryStore.Get: %v", err)
	}
	return l
}

// put writes l into the shared store, as another process would (another
// candidate, or an operator who edits the Lease object). It sets l.Version
// and returns the stored lease.
func (cl *cluster) put(l leaderelection.Lease) leaderelection.Lease {
	cl.t.Helper()
	for range 100 {
		cur := cl.lease()
		l.Version = cur.Version
		stored, err := cl.store.Update(context.Background(), l)
		if err == nil {
			return stored
		}
		if !errors.Is(err, leaderelection.ErrConflict) {
			cl.t.Fatalf("MemoryStore.Update: %v", err)
		}
		// A candidate wrote between our Get and Update. Try again.
	}
	cl.t.Fatalf("could not write the lease: 100 conflicts in a row")
	return leaderelection.Lease{}
}

// ----- leases -----

// show formats a lease compactly, with times as hh:mm:ss on the fake clock.
func show(l leaderelection.Lease) string {
	return fmt.Sprintf("{Holder:%q Epoch:%d AcquireTime:%s RenewTime:%s Duration:%v Version:%d}",
		l.Holder, l.Epoch, hms(l.AcquireTime), hms(l.RenewTime), l.Duration, l.Version)
}

func hms(t time.Time) string {
	if t.IsZero() {
		return "zero"
	}
	return t.Format("15:04:05.999999999")
}

// sameLease compares two leases field by field, times with Equal.
func sameLease(a, b leaderelection.Lease) bool {
	return a.Holder == b.Holder && a.Epoch == b.Epoch &&
		a.AcquireTime.Equal(b.AcquireTime) && a.RenewTime.Equal(b.RenewTime) &&
		a.Duration == b.Duration && a.Version == b.Version
}

// Options for candidate.run and cluster.start.

func releaseOnCancel(c *leaderelection.Config) { c.ReleaseOnCancel = true }

// slowRetries makes the candidate retry only once an hour: it makes its
// first attempt and then nothing more during the test.
func slowRetries(c *leaderelection.Config) {
	c.RetryPeriod = time.Hour
	c.RenewDeadline = 2 * time.Hour
	c.LeaseDuration = 3 * time.Hour
}
