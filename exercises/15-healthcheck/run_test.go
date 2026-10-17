package healthcheck_test

// Step 3: running the checks: concurrency, timeouts, panics, leaks.
//
//	go test -race -run TestRun -v ./exercises/15-healthcheck/

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	healthcheck "github.com/yornik/go-practice/exercises/15-healthcheck"
)

// TestRunConcurrently registers 3 checks that each wait until all 3 have
// started. Run one after the other, the first one waits forever (well, for
// its 1-minute timeout) for the others.
func TestRunConcurrently(t *testing.T) {
	const n = 3
	var started atomic.Int32
	allStarted := newEvent()
	testOver := newGate(t) // unblocks the checks of a wrong solution when the test ends
	h := healthcheck.New(healthcheck.Options{})
	for _, name := range []string{"auth-api", "postgres", "redis"} {
		err := h.Register(healthcheck.Check{
			Name: name, Critical: true, Timeout: time.Minute,
			Fn: func(ctx context.Context) error {
				if started.Add(1) == n {
					allStarted.fire()
				}
				select {
				case <-allStarted.done():
					return nil
				case <-ctx.Done():
					return ctx.Err()
				case <-testOver.ch:
					return errors.New("test over")
				}
			},
		})
		if err != nil {
			t.Fatalf("Register(check %q) = %v, want nil", name, err)
		}
	}

	pending := startRequest(h.Handler(), httptest.NewRequest(http.MethodGet, "/readyz", nil))
	select {
	case <-allStarted.done():
	case <-time.After(timeout):
		t.Fatalf("after %v only %d of %d checks had started: /readyz must run all checks at the same time",
			timeout, started.Load(), n)
	}
	resp := await(t, "GET /readyz", pending)
	rep := decodeReport(t, resp)
	wantReadiness(t, resp, rep, http.StatusOK, "ok")
}

// TestRunTimeoutIgnoringContext: a check that never looks at its ctx (a
// driver call without a deadline, say) must not hold up /readyz.
func TestRunTimeoutIgnoringContext(t *testing.T) {
	stuck := newGate(t) // opened only when the test ends
	h := newHealth(t, healthcheck.Options{},
		healthcheck.Check{
			Name: "postgres", Critical: true, Timeout: 50 * time.Millisecond,
			Fn: func(context.Context) error {
				<-stuck.ch // ignores ctx
				return nil
			},
		},
		check("redis", true, nil, nil),
	)

	resp, rep := getReadyz(t, h.Handler()) // fails if /readyz waits for the stuck check
	wantReadiness(t, resp, rep, http.StatusServiceUnavailable, "fail")
	if c := findCheck(t, rep, "postgres"); c.Status != "timeout" || c.Error == "" {
		t.Errorf("check %q: status %q, error %q; want status \"timeout\" and an error message", "postgres", c.Status, c.Error)
	}
	if c := findCheck(t, rep, "redis"); c.Status != "ok" {
		t.Errorf("check %q: status %q, want \"ok\"", "redis", c.Status)
	}
}

// TestRunContextDeadline: a check that honours its ctx sees a deadline and a
// cancellation when its Timeout expires, and returns ctx.Err().
func TestRunContextDeadline(t *testing.T) {
	const checkTimeout = 50 * time.Millisecond
	var (
		hasDeadline atomic.Bool
		tooLate     atomic.Bool
		gotErr      atomic.Value // errBox
	)
	started, returned := newEvent(), newEvent()
	h := newHealth(t, healthcheck.Options{}, healthcheck.Check{
		Name: "postgres", Critical: true, Timeout: checkTimeout,
		Fn: func(ctx context.Context) error {
			deadline, ok := ctx.Deadline()
			hasDeadline.Store(ok)
			tooLate.Store(ok && time.Until(deadline) > checkTimeout)
			started.fire()
			<-ctx.Done()
			gotErr.Store(errBox{ctx.Err()})
			returned.fire()
			return ctx.Err()
		},
	})

	resp, rep := getReadyz(t, h.Handler())
	waitClosed(t, started.done(), "the check to start")
	if !hasDeadline.Load() {
		t.Fatalf("the check's ctx has no deadline, want one that is Timeout (%v) after the check started", checkTimeout)
	}
	if tooLate.Load() {
		t.Errorf("the check's ctx deadline is more than Timeout (%v) away when the check starts", checkTimeout)
	}
	waitClosed(t, returned.done(), "the check's ctx to be cancelled after its Timeout")
	if err := gotErr.Load().(errBox).err; !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("the check's ctx.Err() = %v after Timeout, want context.DeadlineExceeded", err)
	}
	wantReadiness(t, resp, rep, http.StatusServiceUnavailable, "fail")
	if c := findCheck(t, rep, "postgres"); c.Status != "timeout" {
		t.Errorf("check %q returned ctx.Err() after its Timeout: status %q, want \"timeout\"", "postgres", c.Status)
	}
}

// TestRunTimeoutPerCheck: each check has its own timeout. "fast" gets 30ms
// and needs longer, "slow" gets 1 minute and needs 100ms.
func TestRunTimeoutPerCheck(t *testing.T) {
	h := newHealth(t, healthcheck.Options{},
		healthcheck.Check{
			Name: "fast", Critical: true, Timeout: 30 * time.Millisecond,
			Fn: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
		},
		healthcheck.Check{
			Name: "slow", Critical: false, Timeout: time.Minute,
			Fn: func(ctx context.Context) error {
				select {
				case <-time.After(100 * time.Millisecond):
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			},
		},
	)

	resp, rep := getReadyz(t, h.Handler()) // fails if "fast" gets the 1-minute timeout
	wantReadiness(t, resp, rep, http.StatusServiceUnavailable, "fail")
	if c := findCheck(t, rep, "fast"); c.Status != "timeout" {
		t.Errorf("check %q (Timeout 30ms): status %q, want \"timeout\"", "fast", c.Status)
	}
	if c := findCheck(t, rep, "slow"); c.Status != "ok" {
		t.Errorf("check %q (Timeout 1m, takes 100ms): status %q, error %q; want \"ok\": the 30ms timeout of another check must not cut it short",
			"slow", c.Status, c.Error)
	}
}

func TestRunPanic(t *testing.T) {
	crash := func(context.Context) error {
		panic("postgres client is nil") // a bug in the check itself
	}
	t.Run("critical", func(t *testing.T) {
		h := newHealth(t, healthcheck.Options{},
			healthcheck.Check{Name: "buggy", Critical: true, Timeout: time.Second, Fn: crash},
			check("postgres", true, nil, nil),
		)
		resp, rep := getReadyz(t, h.Handler())
		wantReadiness(t, resp, rep, http.StatusServiceUnavailable, "fail")
		if c := findCheck(t, rep, "buggy"); c.Status != "fail" || c.Error == "" {
			t.Errorf("check %q panicked: status %q, error %q; want status \"fail\" and an error message", "buggy", c.Status, c.Error)
		}
		if c := findCheck(t, rep, "postgres"); c.Status != "ok" {
			t.Errorf("check %q: status %q, want \"ok\"", "postgres", c.Status)
		}
	})
	t.Run("non-critical", func(t *testing.T) {
		h := newHealth(t, healthcheck.Options{},
			healthcheck.Check{Name: "buggy", Critical: false, Timeout: time.Second, Fn: crash},
		)
		resp, rep := getReadyz(t, h.Handler())
		wantReadiness(t, resp, rep, http.StatusOK, "ok")
		if c := findCheck(t, rep, "buggy"); c.Status != "fail" {
			t.Errorf("check %q panicked: status %q, want \"fail\"", "buggy", c.Status)
		}
	})
}

// TestRunNoGoroutineLeak: the stuck check returns after /readyz gave up on
// it. The goroutine that ran it must then be able to exit.
func TestRunNoGoroutineLeak(t *testing.T) {
	release := newGate(t)
	returned := newEvent()
	h := newHealth(t, healthcheck.Options{}, healthcheck.Check{
		Name: "postgres", Critical: true, Timeout: 20 * time.Millisecond,
		Fn: func(context.Context) error {
			defer returned.fire()
			<-release.ch // ignores ctx
			return errors.New("too late")
		},
	})
	handler := h.Handler()

	checkLeak := expectNoLeak(t)
	_, rep := getReadyz(t, handler)
	if c := findCheck(t, rep, "postgres"); c.Status != "timeout" {
		t.Fatalf("check %q: status %q, want \"timeout\"", "postgres", c.Status)
	}
	release.open() // now the check returns, long after its timeout
	waitClosed(t, returned.done(), "the stuck check to return")
	checkLeak()
}

// TestRunClientGone: the client (or the kubelet's probe timeout) hangs up
// before the checks finish, so its request's context is cancelled. The
// checks' contexts must not be: the run can be shared with other requests
// through the cache, and a "context canceled" failure would then be served to
// every probe for the next CacheTTL. What the handler writes to a client that
// has gone is not checked: nobody reads it.
func TestRunClientGone(t *testing.T) {
	clock := newFakeClock()    // never moves: a cached result stays fresh
	var sawCancel atomic.Value // errBox: a non-nil ctx.Err() that a check saw
	h := newHealth(t, healthcheck.Options{CacheTTL: time.Minute, Now: clock.Now}, healthcheck.Check{
		Name: "postgres", Critical: true, Timeout: time.Second,
		Fn: func(ctx context.Context) error {
			if err := ctx.Err(); err != nil {
				sawCancel.Store(errBox{err})
			}
			return ctx.Err()
		},
	})
	handler := h.Handler()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // the client hung up already
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil).WithContext(ctx)
	await(t, "GET /readyz with a cancelled request context", startRequest(handler, req))

	resp, rep := getReadyz(t, handler) // the next probe, with a client that waits
	if b, ok := sawCancel.Load().(errBox); ok {
		t.Errorf("a check started with ctx.Err() = %v, want nil: only the check's Timeout may end its ctx, not the request's context", b.err)
	}
	if resp.Code != http.StatusOK || rep.Status != "ok" {
		t.Fatalf("GET /readyz after another client hung up = %d with status %q, want 200 with status \"ok\": "+
			"a cancelled request must not leave a failed result for the next probe\nbody: %s", resp.Code, rep.Status, resp.Body)
	}
}

// errBox lets an atomic.Value hold a nil error.
type errBox struct{ err error }
