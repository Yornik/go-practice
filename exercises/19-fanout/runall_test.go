package fanout_test

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	fanout "github.com/yornik/go-practice/exercises/19-fanout"
)

// Step 1: results in the order of the checks.

func TestRunAllOrder(t *testing.T) {
	defer guard(t)()
	names := services
	p := newProbes(t, names, failures(names))
	call := startRunAll(p.checks)

	// The last check finishes first, the first check last.
	for i := len(names) - 1; i >= 0; i-- {
		p.release(i)
		p.waitReturned(t, i, call)
	}
	got := awaitRunAll(t, call)

	if len(got) != len(names) {
		t.Fatalf("RunAll of %d checks returned %d results, want %d", len(names), len(got), len(names))
	}
	for i := range names {
		want := fanout.Result{Name: names[i], Err: p.errs[i]}
		if got[i] != want {
			t.Errorf("result %d = %s, want %s (the result of checks[%d], whatever order the checks finish in)",
				i, show(got[i]), show(want), i)
		}
	}
	p.wantCalledOnce(t)
}

func TestRunAllOrderEmpty(t *testing.T) {
	defer guard(t)()
	for _, tt := range []struct {
		name   string
		checks []fanout.Check
	}{
		{"nil", nil},
		{"empty", []fanout.Check{}},
	} {
		if got := runAll(t, tt.checks); len(got) != 0 {
			t.Errorf("RunAll(%s) returned %d results, want 0", tt.name, len(got))
		}
	}
}

func TestRunAllOrderDuplicateNames(t *testing.T) {
	defer guard(t)()
	names := []string{"chkpt-store", "chkpt-store", "orders", "chkpt-store", "chkpt-store", "orders"}
	errs := []error{
		nil,
		errors.New("chkpt-store-2: disk 97% full"),
		nil,
		errors.New("chkpt-store-4: connection refused"),
		nil,
		errors.New("orders: GET /healthz: 503 Service Unavailable"),
	}
	checks, calls := quick(names, errs)
	got := runAll(t, checks)

	if len(got) != len(names) {
		t.Fatalf("RunAll of %d checks returned %d results, want %d", len(names), len(got), len(names))
	}
	for i := range names {
		want := fanout.Result{Name: names[i], Err: errs[i]}
		if got[i] != want {
			t.Errorf("result %d = %s, want %s (2 checks may have the same name; each gets its own result)",
				i, show(got[i]), show(want))
		}
		if n := calls[i].Load(); n != 1 {
			t.Errorf("Run of check %d (%q) was called %d times, want 1", i, names[i], n)
		}
	}
}

// Step 2: every check runs at the same time, and no goroutine is left.

func TestRunAllConcurrent(t *testing.T) {
	for _, n := range []int{12, 200} {
		t.Run(fmt.Sprintf("%d checks", n), func(t *testing.T) {
			defer guard(t)()
			var started atomic.Int32
			all := make(chan struct{})   // closed when the last check starts
			abort := make(chan struct{}) // closed when the test gives up
			t.Cleanup(func() { close(abort) })

			names := numbered(n)
			checks := make([]fanout.Check, n)
			for i := range checks {
				checks[i] = fanout.Check{Name: names[i], Run: func() error {
					if started.Add(1) == int32(n) {
						close(all)
					}
					// Every check waits until all n have started.
					select {
					case <-all:
					case <-abort:
					}
					return nil
				}}
			}

			call := startRunAll(checks)
			var got []fanout.Result
			select {
			case <-call.done:
				if call.panicked != nil {
					t.Fatalf("RunAll panicked: %v", call.panicked)
				}
				got = call.results
			case <-time.After(timeout):
				t.Fatalf("RunAll did not return within %v: only %d of %d checks started at the same time. "+
					"Every check in this test waits until all %d have started, so RunAll must start every "+
					"check before it waits for any of them", timeout, started.Load(), n, n)
			}
			if len(got) != n {
				t.Fatalf("RunAll of %d checks returned %d results, want %d", n, len(got), n)
			}
			for i := range got {
				if want := (fanout.Result{Name: names[i]}); got[i] != want {
					t.Fatalf("result %d = %s, want %s (results in the order of the checks)", i, show(got[i]), show(want))
				}
			}
			if s := started.Load(); s != int32(n) {
				t.Errorf("Run was called %d times for %d checks, want %d", s, n, n)
			}
		})
	}
}

func TestRunAllConcurrentNoLeak(t *testing.T) {
	defer guard(t)()
	names := numbered(100)
	checks, _ := quick(names, failures(names))
	before := runtime.NumGoroutine()
	got := runAll(t, checks)
	if len(got) != len(names) {
		t.Fatalf("RunAll of %d checks returned %d results, want %d", len(names), len(got), len(names))
	}
	waitGoroutines(t, before, "after RunAll returned")
}

// Step 3: a check that panics.

func TestRunAllPanic(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
		want string // the message of the error
	}{
		{
			name: "string",
			run:  func() error { panic("empty body") },
			want: "fanout: check panicked: empty body",
		},
		{
			name: "error",
			run:  func() error { panic(io.ErrUnexpectedEOF) },
			want: "fanout: check panicked: unexpected EOF",
		},
		{
			name: "int",
			run:  func() error { panic(42) },
			want: "fanout: check panicked: 42",
		},
		{
			name: "index out of range",
			run: func() error {
				var body []byte
				return fmt.Errorf("first byte %q", body[0])
			},
			want: "fanout: check panicked: runtime error: index out of range [0] with length 0",
		},
	}
	errOrders := errors.New("orders: GET /healthz: 503 Service Unavailable")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer guard(t)()
			checks := []fanout.Check{
				{Name: "checkout", Run: func() error { return nil }},
				{Name: "hush-api", Run: tt.run},
				{Name: "orders", Run: func() error { return errOrders }},
			}
			got := runAll(t, checks)
			if len(got) != 3 {
				t.Fatalf("RunAll of 3 checks returned %d results, want 3", len(got))
			}
			if want := (fanout.Result{Name: "checkout"}); got[0] != want {
				t.Errorf("result 0 = %s, want %s (a panic in another check must not change it)", show(got[0]), show(want))
			}
			if want := (fanout.Result{Name: "orders", Err: errOrders}); got[2] != want {
				t.Errorf("result 2 = %s, want %s (a panic in another check must not change it)", show(got[2]), show(want))
			}
			r := got[1]
			if r.Name != "hush-api" {
				t.Errorf("result 1 has Name %q, want %q", r.Name, "hush-api")
			}
			if r.Err == nil {
				t.Fatalf("result 1 (the check that panicked) has Err nil, want an error that wraps ErrPanic")
			}
			if !errors.Is(r.Err, fanout.ErrPanic) {
				t.Errorf("errors.Is(%q, ErrPanic) = false, want true", r.Err)
			}
			if r.Err.Error() != tt.want {
				t.Errorf("the error of the check that panicked is %q, want %q", r.Err, tt.want)
			}
		})
	}
}

func TestRunAllPanicNilRun(t *testing.T) {
	defer guard(t)()
	checks := []fanout.Check{
		{Name: "checkout", Run: func() error { return nil }},
		{Name: "hush-api"}, // Run is nil
		{Name: "notify", Run: func() error { return nil }},
	}
	got := runAll(t, checks)
	if len(got) != 3 {
		t.Fatalf("RunAll of 3 checks returned %d results, want 3", len(got))
	}
	for _, i := range []int{0, 2} {
		if want := (fanout.Result{Name: checks[i].Name}); got[i] != want {
			t.Errorf("result %d = %s, want %s", i, show(got[i]), show(want))
		}
	}
	r := got[1]
	if r.Name != "hush-api" {
		t.Errorf("result 1 has Name %q, want %q", r.Name, "hush-api")
	}
	if !errors.Is(r.Err, fanout.ErrPanic) {
		t.Errorf("the check with a nil Run has Err %v, want an error that wraps ErrPanic (calling a nil func panics)", r.Err)
	} else if !strings.HasPrefix(r.Err.Error(), "fanout: check panicked: ") {
		t.Errorf("the error of the check with a nil Run is %q, want it to start with %q", r.Err, "fanout: check panicked: ")
	}
}

func TestRunAllPanicReturnedError(t *testing.T) {
	defer guard(t)()
	errAuth := errors.New("auth: token expired")
	checks := []fanout.Check{
		{Name: "auth", Run: func() error { return errAuth }},
		{Name: "hush-api", Run: func() error { panic("empty body") }},
	}
	got := runAll(t, checks)
	if len(got) != 2 {
		t.Fatalf("RunAll of 2 checks returned %d results, want 2", len(got))
	}
	if got[0].Err != errAuth {
		t.Errorf("result 0 has Err %v, want the error that Run returned, unchanged: %q", got[0].Err, errAuth)
	}
	if errors.Is(got[0].Err, fanout.ErrPanic) {
		t.Errorf("the error that Run returned wraps ErrPanic, but that check did not panic")
	}
}
