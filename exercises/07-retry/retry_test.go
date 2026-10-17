package retry_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	retry "github.com/yornik/go-practice/exercises/07-retry"
)

// Step 4: the loop. Do and DoValue. Unless a test says otherwise, the policy
// is basePolicy (5 attempts, 100ms doubling up to 1s, no jitter) with a fake
// Sleep that returns at once and records the delays.

// ----- the attempts and the delays -----

func TestDoSucceedsFirstAttempt(t *testing.T) {
	s := &sleeper{}
	p := basePolicy
	p.Sleep = s.Sleep
	f := &fakeOp{results: []error{nil}}

	if err := runDo(t, context.Background(), p, f.op); err != nil {
		t.Errorf("Do = %v, want nil", err)
	}
	checkCalls(t, f, 1)
	checkDelays(t, s) // no sleep before the first attempt, none after a success
}

func TestDoRetriesUntilSuccess(t *testing.T) {
	s := &sleeper{}
	p := basePolicy
	p.Sleep = s.Sleep
	f := &fakeOp{results: append(failing(3), nil)}

	if err := runDo(t, context.Background(), p, f.op); err != nil {
		t.Errorf("Do = %v, want nil: the 4th attempt succeeded", err)
	}
	checkCalls(t, f, 4)
	checkDelays(t, s, 100*ms, 200*ms, 400*ms)
}

func TestDoExhausted(t *testing.T) {
	tests := []struct {
		maxAttempts int
		wantDelays  []time.Duration
	}{
		{1, nil}, // 1 attempt: no retry, no sleep
		{2, []time.Duration{100 * ms}},
		{5, []time.Duration{100 * ms, 200 * ms, 400 * ms, 800 * ms}},
		{7, []time.Duration{100 * ms, 200 * ms, 400 * ms, 800 * ms, time.Second, time.Second}},
	}
	for _, tc := range tests {
		t.Run(fmt.Sprintf("MaxAttempts=%d", tc.maxAttempts), func(t *testing.T) {
			s := &sleeper{}
			p := basePolicy
			p.MaxAttempts = tc.maxAttempts
			p.Sleep = s.Sleep
			errs := failing(tc.maxAttempts)
			f := &fakeOp{results: errs}

			err := runDo(t, context.Background(), p, f.op)
			checkCalls(t, f, tc.maxAttempts)
			checkDelays(t, s, tc.wantDelays...) // no sleep after the last attempt

			var ex *retry.ExhaustedError
			if !errors.As(err, &ex) {
				t.Fatalf("Do = %v (%T), want a *ExhaustedError", err, err)
			}
			if ex.Attempts != tc.maxAttempts {
				t.Errorf("ExhaustedError.Attempts = %d, want %d", ex.Attempts, tc.maxAttempts)
			}
			if last := errs[len(errs)-1]; ex.Last != last {
				t.Errorf("ExhaustedError.Last = %v, want the error of the last attempt (%v)", ex.Last, last)
			}
			if !errors.Is(err, errRefused) {
				t.Errorf("errors.Is(Do error, errRefused) = false, want true")
			}
		})
	}
}

func TestDoInvalidPolicy(t *testing.T) {
	tests := []struct {
		name string
		p    retry.Policy
	}{
		{"zero Policy", retry.Policy{}},
		{"MaxAttempts 0", retry.Policy{MaxAttempts: 0, Initial: ms, Max: time.Second, Multiplier: 2}},
		{"Initial 0", retry.Policy{MaxAttempts: 3, Initial: 0, Max: time.Second, Multiplier: 2}},
		{"Multiplier 0.5", retry.Policy{MaxAttempts: 3, Initial: ms, Max: time.Second, Multiplier: 0.5}},
		{"Max below Initial", retry.Policy{MaxAttempts: 3, Initial: time.Second, Max: ms, Multiplier: 2}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &sleeper{}
			tc.p.Sleep = s.Sleep
			f := &fakeOp{results: []error{nil}}

			err := runDo(t, context.Background(), tc.p, f.op)
			if !errors.Is(err, retry.ErrInvalidPolicy) {
				t.Errorf("Do = %v, want an error that wraps ErrInvalidPolicy", err)
			}
			checkCalls(t, f, 0)
			checkDelays(t, s)
		})
	}
}

// ----- Permanent -----

func TestDoPermanent(t *testing.T) {
	defer catchPanic(t) // the table calls retry.Permanent
	tests := []struct {
		name        string
		maxAttempts int // Policy.MaxAttempts; 0 means basePolicy's 5
		results     []error
		want        error // nil: Do must return nil
		wantCalls   int
		wantDelays  []time.Duration
	}{
		{
			name:      "permanent on the first attempt",
			results:   []error{retry.Permanent(errBadRequest)},
			want:      errBadRequest,
			wantCalls: 1,
		},
		{
			name:       "permanent after a retryable failure",
			results:    []error{errRefused, retry.Permanent(errBadRequest)},
			want:       errBadRequest,
			wantCalls:  2,
			wantDelays: []time.Duration{100 * ms},
		},
		{
			// No attempts are left, but the error still says "do not retry":
			// the caller gets it as itself, not inside an ExhaustedError.
			name:        "permanent on the last attempt",
			maxAttempts: 2,
			results:     []error{errRefused, retry.Permanent(errBadRequest)},
			want:        errBadRequest,
			wantCalls:   2,
			wantDelays:  []time.Duration{100 * ms},
		},
		{
			name:        "permanent with MaxAttempts 1 (no retries)",
			maxAttempts: 1,
			results:     []error{retry.Permanent(errBadRequest)},
			want:        errBadRequest,
			wantCalls:   1,
		},
		{
			// op adds context around the mark, as real code does. Do must
			// still find it, and still return the error passed to Permanent.
			name:      "permanent wrapped by op",
			results:   []error{fmt.Errorf("GET /api/v1/apps/web: %w", retry.Permanent(errBadRequest))},
			want:      errBadRequest,
			wantCalls: 1,
		},
		{
			// return retry.Permanent(json.Unmarshal(...)) when Unmarshal succeeds.
			name:      "Permanent(nil) is a success",
			results:   []error{retry.Permanent(nil)},
			want:      nil,
			wantCalls: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &sleeper{}
			p := basePolicy
			if tc.maxAttempts != 0 {
				p.MaxAttempts = tc.maxAttempts
			}
			p.Sleep = s.Sleep
			f := &fakeOp{results: tc.results}

			err := runDo(t, context.Background(), p, f.op)
			if err != tc.want {
				t.Errorf("Do = %v (%T), want %v (%T) itself, not wrapped", err, err, tc.want, tc.want)
			}
			checkCalls(t, f, tc.wantCalls)
			checkDelays(t, s, tc.wantDelays...)
		})
	}
}

// ----- Retry-After -----

func TestDoRetryAfter(t *testing.T) {
	retryAfter := func(d time.Duration) error {
		return &retry.RetryAfterError{Err: errUnavail, After: d}
	}
	tests := []struct {
		name       string
		max        time.Duration // Policy.Max; 0 means basePolicy's 1s
		results    []error
		wantDelays []time.Duration
	}{
		{
			name:       "the server's delay replaces the backoff",
			results:    []error{retryAfter(300 * ms), nil},
			wantDelays: []time.Duration{300 * ms},
		},
		{
			name:       "a delay longer than Max is capped at Max",
			results:    []error{retryAfter(30 * time.Second), nil},
			wantDelays: []time.Duration{time.Second},
		},
		{
			name:       "Retry-After 0 means retry at once",
			results:    []error{retryAfter(0), nil},
			wantDelays: []time.Duration{0},
		},
		{
			name:       "a negative delay counts as 0",
			results:    []error{retryAfter(-5 * time.Second), nil},
			wantDelays: []time.Duration{0},
		},
		{
			name:       "wrapped by op",
			results:    []error{fmt.Errorf("GET /api/v1/apps: %w", retryAfter(700*ms)), nil},
			wantDelays: []time.Duration{700 * ms},
		},
		{
			// The backoff depends on the attempt number only. A Retry-After in
			// between does not reset it and does not hold it back.
			name:       "the backoff continues by attempt number",
			max:        5 * time.Second,
			results:    []error{errRefused, retryAfter(3 * time.Second), errRefused, nil},
			wantDelays: []time.Duration{100 * ms, 3 * time.Second, 400 * ms},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &sleeper{}
			p := basePolicy
			if tc.max != 0 {
				p.Max = tc.max
			}
			p.Sleep = s.Sleep
			f := &fakeOp{results: tc.results}

			if err := runDo(t, context.Background(), p, f.op); err != nil {
				t.Errorf("Do = %v, want nil", err)
			}
			checkCalls(t, f, len(tc.results))
			checkDelays(t, s, tc.wantDelays...)
		})
	}

	t.Run("on the last attempt", func(t *testing.T) {
		s := &sleeper{}
		p := basePolicy
		p.MaxAttempts = 2
		p.Sleep = s.Sleep
		last := retryAfter(time.Minute)
		f := &fakeOp{results: []error{errRefused, last}}

		err := runDo(t, context.Background(), p, f.op)
		checkCalls(t, f, 2)
		checkDelays(t, s, 100*ms) // no sleep after the last attempt
		var ex *retry.ExhaustedError
		if !errors.As(err, &ex) || ex.Last != last {
			t.Errorf("Do = %v, want a *ExhaustedError whose Last is the RetryAfterError", err)
		}
	})
}

// ----- jitter -----

// randSeq returns a fake Policy.Rand that returns vals in order, and a func
// that reports how often it was called. A call past the end of vals fails
// the test.
func randSeq(t *testing.T, vals ...float64) (rnd func() float64, calls func() int) {
	var mu sync.Mutex
	n := 0
	rnd = func() float64 {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n > len(vals) {
			t.Errorf("Rand was called %d times, want at most %d", n, len(vals))
			return 0
		}
		return vals[n-1]
	}
	calls = func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
	return rnd, calls
}

func TestDoJitter(t *testing.T) {
	t.Run("full jitter is Rand times the backoff", func(t *testing.T) {
		s := &sleeper{}
		rnd, calls := randSeq(t, 0.5, 0.25, 0.75, 0)
		p := basePolicy
		p.Jitter = true
		p.Rand = rnd
		p.Sleep = s.Sleep
		f := &fakeOp{results: failing(5)}

		_ = runDo(t, context.Background(), p, f.op)
		// Backoffs 100ms 200ms 400ms 800ms, times 0.5 0.25 0.75 0.
		checkDelays(t, s, 50*ms, 50*ms, 300*ms, 0)
		if got := calls(); got != 4 {
			t.Errorf("Rand was called %d times, want 4: once per delay", got)
		}
	})

	t.Run("no Rand call without Jitter", func(t *testing.T) {
		s := &sleeper{}
		rnd, calls := randSeq(t) // any call fails the test
		p := basePolicy
		p.Rand = rnd
		p.Sleep = s.Sleep
		f := &fakeOp{results: failing(3)}
		p.MaxAttempts = 3

		_ = runDo(t, context.Background(), p, f.op)
		checkDelays(t, s, 100*ms, 200*ms)
		if got := calls(); got != 0 {
			t.Errorf("Rand was called %d times, want 0: Jitter is false", got)
		}
	})

	t.Run("no jitter on a Retry-After delay", func(t *testing.T) {
		s := &sleeper{}
		rnd, calls := randSeq(t, 0.5)
		p := basePolicy
		p.Max = 5 * time.Second
		p.Jitter = true
		p.Rand = rnd
		p.Sleep = s.Sleep
		f := &fakeOp{results: []error{
			&retry.RetryAfterError{Err: errUnavail, After: 2 * time.Second},
			errRefused,
			nil,
		}}

		if err := runDo(t, context.Background(), p, f.op); err != nil {
			t.Errorf("Do = %v, want nil", err)
		}
		// 2s exactly as asked; then 0.5 times Backoff(2) = 0.5 * 200ms.
		checkDelays(t, s, 2*time.Second, 100*ms)
		if got := calls(); got != 1 {
			t.Errorf("Rand was called %d times, want 1: only for the backoff delay", got)
		}
	})

	t.Run("default Rand", func(t *testing.T) {
		defer catchPanic(t) // the check calls p.Backoff
		s := &sleeper{}
		p := basePolicy
		p.MaxAttempts = 8
		p.Jitter = true // Rand is nil: Do uses math/rand/v2
		p.Sleep = s.Sleep
		f := &fakeOp{results: failing(8)}

		_ = runDo(t, context.Background(), p, f.op)
		got := s.Delays()
		if len(got) != 7 {
			t.Fatalf("Do slept %d times, want 7", len(got))
		}
		for i, d := range got {
			// Rand is in [0, 1), so a jittered delay is always below the backoff.
			if b := p.Backoff(i + 1); d < 0 || d >= b {
				t.Errorf("delay %d = %v, want a random delay in [0, %v)", i+1, d, b)
			}
		}
	})
}

// ----- the context -----

type traceIDKey struct{}

func TestDoPassesContextToOp(t *testing.T) {
	ctx := context.WithValue(context.Background(), traceIDKey{}, "trace-4bf92f35")
	p := basePolicy
	p.Sleep = (&sleeper{}).Sleep
	var got any
	op := func(ctx context.Context) error {
		got = ctx.Value(traceIDKey{})
		return nil
	}
	if err := runDo(t, ctx, p, op); err != nil {
		t.Fatalf("Do = %v, want nil", err)
	}
	if got != "trace-4bf92f35" {
		t.Errorf("op's ctx has trace ID %v, want %q: op must receive the ctx passed to Do (or one derived from it)", got, "trace-4bf92f35")
	}
}

func TestDoContextDoneBeforeFirstAttempt(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancel2 := context.WithDeadline(context.Background(), time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC))
	defer cancel2()

	tests := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"cancelled", cancelled, context.Canceled},
		{"deadline passed", expired, context.DeadlineExceeded},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := &sleeper{}
			p := basePolicy
			p.Sleep = s.Sleep
			f := &fakeOp{results: []error{nil}}

			err := runDo(t, tc.ctx, p, f.op)
			if !errors.Is(err, tc.want) {
				t.Errorf("Do = %v, want an error that matches %v", err, tc.want)
			}
			checkCalls(t, f, 0)
			checkDelays(t, s)
		})
	}
}

func TestDoContextCancelledDuringSleep(t *testing.T) {
	// The fake Sleep cancels ctx while Do waits after attempt 2. Do must not
	// call op again, and its error must say both why it stopped (ctx) and
	// what was failing (the error of attempt 2).
	tests := []struct {
		name string
		// sleepResult is what the fake Sleep returns after it cancelled ctx.
		// A real Sleep returns ctx.Err(). But a select whose timer fired at
		// the same moment can pick the timer and return nil: Do must still
		// not call op again.
		sleepResult func(ctx context.Context) error
	}{
		{"Sleep returns ctx.Err()", func(ctx context.Context) error { return ctx.Err() }},
		{"Sleep returns nil", func(context.Context) error { return nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := &sleeper{hook: func(ctx context.Context, n int) error {
				if n == 2 {
					cancel()
					return tc.sleepResult(ctx)
				}
				return nil
			}}
			p := basePolicy
			p.Sleep = s.Sleep
			errs := failing(5)
			f := &fakeOp{results: errs}

			err := runDo(t, ctx, p, f.op)
			checkCalls(t, f, 2)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("Do = %v, want an error that matches context.Canceled", err)
			}
			if !errors.Is(err, errs[1]) {
				t.Errorf("Do = %v, want an error that also matches the last op error (%v)", err, errs[1])
			}
		})
	}
}

func TestDoContextCancelledDuringAttempt(t *testing.T) {
	// The operator presses Ctrl-C while attempt 1 runs; attempt 1 fails.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p := basePolicy
	p.Sleep = (&sleeper{}).Sleep // returns nil: Do must still see that ctx is done
	f := &fakeOp{
		results: []error{errRefused, nil},
		before:  func(_ context.Context, n int) { cancel() },
	}

	err := runDo(t, ctx, p, f.op)
	checkCalls(t, f, 1)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("Do = %v, want an error that matches context.Canceled", err)
	}
	if !errors.Is(err, errRefused) {
		t.Errorf("Do = %v, want an error that also matches the last op error (%v)", err, errRefused)
	}
}

func TestDoContextResultOfOpWins(t *testing.T) {
	defer catchPanic(t) // the table calls retry.Permanent
	// ctx ends while op runs, but op finishes anyway. Its result counts.
	tests := []struct {
		name   string
		result error
		want   error
	}{
		{"success", nil, nil},
		{"permanent error", retry.Permanent(errBadRequest), errBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			p := basePolicy
			p.Sleep = (&sleeper{}).Sleep
			f := &fakeOp{
				results: []error{tc.result},
				before:  func(_ context.Context, n int) { cancel() },
			}

			if err := runDo(t, ctx, p, f.op); err != tc.want {
				t.Errorf("Do = %v, want %v: op's own result wins over a ctx that ended during the call", err, tc.want)
			}
			checkCalls(t, f, 1)
		})
	}
}

func TestDoSleepError(t *testing.T) {
	// A custom Sleep can give up for its own reasons, for example because
	// the process is shutting down. ctx is not done here.
	errShutdown := errors.New("shutting down")
	s := &sleeper{hook: func(context.Context, int) error { return errShutdown }}
	p := basePolicy
	p.Sleep = s.Sleep
	f := &fakeOp{results: []error{errRefused, nil}}

	err := runDo(t, context.Background(), p, f.op)
	checkCalls(t, f, 1)
	if !errors.Is(err, errShutdown) {
		t.Errorf("Do = %v, want an error that matches Sleep's error (%v)", err, errShutdown)
	}
	if !errors.Is(err, errRefused) {
		t.Errorf("Do = %v, want an error that also matches the last op error (%v)", err, errRefused)
	}
}

// ----- the default Sleep (a real timer) -----

func TestDoDefaultSleep(t *testing.T) {
	t.Run("waits and retries", func(t *testing.T) {
		p := retry.Policy{MaxAttempts: 3, Initial: ms, Max: 2 * ms, Multiplier: 2} // Sleep is nil
		f := &fakeOp{results: failing(3)}

		err := runDo(t, context.Background(), p, f.op)
		checkCalls(t, f, 3)
		var ex *retry.ExhaustedError
		if !errors.As(err, &ex) {
			t.Errorf("Do = %v, want a *ExhaustedError", err)
		}
	})

	// The backoff is 1 hour. A Sleep that ignores ctx makes this test time
	// out after 5s.
	hourly := retry.Policy{MaxAttempts: 3, Initial: time.Hour, Max: time.Hour, Multiplier: 2}

	t.Run("returns when ctx is cancelled", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		f := &fakeOp{results: []error{errRefused, nil}}
		// Cancel shortly after attempt 1, while Do (most likely) sleeps.
		f.before = func(_ context.Context, n int) {
			time.AfterFunc(20*ms, cancel)
		}

		err := runDo(t, ctx, hourly, f.op)
		checkCalls(t, f, 1)
		if !errors.Is(err, context.Canceled) || !errors.Is(err, errRefused) {
			t.Errorf("Do = %v, want an error that matches context.Canceled and errRefused", err)
		}
	})

	t.Run("returns when the deadline passes", func(t *testing.T) {
		ctx := newExpiringContext()
		f := &fakeOp{results: []error{errRefused, nil}}
		// Let the deadline pass shortly after attempt 1, while Do (most likely) sleeps.
		f.before = func(_ context.Context, n int) {
			time.AfterFunc(20*ms, ctx.expire)
		}

		err := runDo(t, ctx, hourly, f.op)
		checkCalls(t, f, 1)
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, errRefused) {
			t.Errorf("Do = %v, want an error that matches context.DeadlineExceeded and errRefused", err)
		}
	})
}

// ----- concurrency -----

func TestDoConcurrentUse(t *testing.T) {
	// One Policy, shared by many goroutines, as a client library does. Run
	// with -race: the default Rand must be safe for concurrent use.
	s := &sleeper{}
	p := basePolicy
	p.Jitter = true
	p.Sleep = s.Sleep

	const goroutines = 8
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	for i := range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if v := recover(); v != nil {
					errs[i] = fmt.Errorf("Do panicked: %v", v)
				}
			}()
			errs[i] = retry.Do(context.Background(), p, func(context.Context) error { return errRefused })
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(timeout):
		t.Fatalf("Do did not return within %v", timeout)
	}

	for i, err := range errs {
		var ex *retry.ExhaustedError
		if !errors.As(err, &ex) {
			t.Errorf("goroutine %d: Do = %v, want a *ExhaustedError", i, err)
		}
	}
	if got, want := len(s.Delays()), goroutines*(p.MaxAttempts-1); got != want {
		t.Errorf("Do slept %d times in total, want %d", got, want)
	}
}

// ----- DoValue -----

func TestDoValueReturnsValue(t *testing.T) {
	s := &sleeper{}
	p := basePolicy
	p.Sleep = s.Sleep
	calls := 0
	op := func(context.Context) (string, error) {
		calls++
		if calls < 3 {
			return "", errUnavail
		}
		return "sha256:9f86d081", nil
	}

	got, err := runDoValue(t, context.Background(), p, op)
	if got != "sha256:9f86d081" || err != nil {
		t.Errorf("DoValue = %q, %v; want %q, nil", got, err, "sha256:9f86d081")
	}
	if calls != 3 {
		t.Errorf("op was called %d times, want 3", calls)
	}
	checkDelays(t, s, 100*ms, 200*ms)
}

func TestDoValueZeroOnError(t *testing.T) {
	defer catchPanic(t) // the table calls retry.Permanent
	// op returns a half-filled value together with its error. DoValue must
	// not pass it on: a caller that forgets to check err would use it.
	type deployment struct {
		Name     string
		Replicas int
	}
	partial := deployment{Name: "web", Replicas: -1}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	tests := []struct {
		name   string
		ctx    context.Context
		p      retry.Policy
		result error // what op returns every time, together with partial
	}{
		{"exhausted", context.Background(), basePolicy, errRefused},
		{"permanent", context.Background(), basePolicy, retry.Permanent(errBadRequest)},
		{"ctx done", cancelled, basePolicy, errRefused},
		{"invalid policy", context.Background(), retry.Policy{}, errRefused},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.p.Sleep = (&sleeper{}).Sleep
			op := func(context.Context) (deployment, error) { return partial, tc.result }

			got, err := runDoValue(t, tc.ctx, tc.p, op)
			if err == nil {
				t.Errorf("DoValue error = nil, want an error")
			}
			if got != (deployment{}) {
				t.Errorf("DoValue = %+v, want the zero value", got)
			}
		})
	}
}
