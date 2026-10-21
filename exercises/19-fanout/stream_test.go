package fanout_test

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	fanout "github.com/yornik/go-practice/exercises/19-fanout"
)

// Step 4: Stream.

func TestStreamOrder(t *testing.T) {
	defer guard(t)()
	names := []string{"checkout", "orders", "payments", "hush-api", "notify"}
	p := newProbes(t, names, failures(names))
	before := runtime.NumGoroutine()

	// Stream must return while every check is still waiting for its gate.
	ch := stream(t, p.checks)

	order := []int{2, 4, 0, 3, 1}
	for k, i := range order {
		p.release(i)
		r, ok := receive(t, ch, "the result of "+names[i])
		if !ok {
			t.Fatalf("the channel was closed after %d results, want %d results", k, len(names))
		}
		if want := (fanout.Result{Name: names[i], Err: p.errs[i]}); r != want {
			t.Fatalf("result %d on the channel = %s, want %s: the test released only %q, so its result comes next",
				k+1, show(r), show(want), names[i])
		}
	}
	expectClosed(t, ch, len(names))
	p.wantCalledOnce(t)
	waitGoroutines(t, before, "after the channel was closed")
}

func TestStreamEmpty(t *testing.T) {
	defer guard(t)()
	for _, tt := range []struct {
		name   string
		checks []fanout.Check
	}{
		{"nil", nil},
		{"empty", []fanout.Check{}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			defer guard(t)()
			ch := stream(t, tt.checks)
			expectClosed(t, ch, 0)
		})
	}
}

func TestStreamNoReader(t *testing.T) {
	defer guard(t)()
	names := services
	errs := failures(names)
	checks, calls := quick(names, errs)
	before := runtime.NumGoroutine()

	ch := stream(t, checks)
	// Nobody reads yet: every check must still finish, and every goroutine
	// must end.
	waitGoroutines(t, before, "after Stream was called, while nobody read from the channel "+
		"(a check must be able to send its result without a reader)")

	got := receiveAll(t, ch, len(names))
	want := make(map[string]error, len(names))
	for i, name := range names {
		want[name] = errs[i]
	}
	seen := make(map[string]bool, len(names))
	for _, r := range got {
		if seen[r.Name] {
			t.Errorf("the result of %q arrived twice", r.Name)
		}
		seen[r.Name] = true
		wantErr, ok := want[r.Name]
		switch {
		case !ok:
			t.Errorf("got a result for %q, which is not the name of a check", r.Name)
		case r.Err != wantErr:
			t.Errorf("got %s, want %s", show(r), show(fanout.Result{Name: r.Name, Err: wantErr}))
		}
	}
	for i, name := range names {
		if n := calls[i].Load(); n != 1 {
			t.Errorf("Run of check %d (%q) was called %d times, want 1", i, name, n)
		}
	}
}

func TestStreamPanic(t *testing.T) {
	defer guard(t)()
	errOrders := errors.New("orders: GET /healthz: 503 Service Unavailable")
	checks := []fanout.Check{
		{Name: "checkout", Run: func() error { return nil }},
		{Name: "hush-api", Run: func() error { panic("empty body") }},
		{Name: "notify"}, // Run is nil
		{Name: "orders", Run: func() error { return errOrders }},
	}
	got := receiveAll(t, stream(t, checks), len(checks))

	byName := make(map[string]error, len(got))
	for _, r := range got {
		byName[r.Name] = r.Err
	}
	if err, ok := byName["checkout"]; !ok || err != nil {
		t.Errorf("checkout: Err = %v (present: %v), want nil", err, ok)
	}
	if err, ok := byName["orders"]; !ok || err != errOrders {
		t.Errorf("orders: Err = %v (present: %v), want %q, unchanged", err, ok, errOrders)
	}
	if err := byName["hush-api"]; !errors.Is(err, fanout.ErrPanic) || err.Error() != "fanout: check panicked: empty body" {
		t.Errorf("hush-api, which panicked with \"empty body\": Err = %v, want %q (wrapping ErrPanic)",
			err, "fanout: check panicked: empty body")
	}
	if err := byName["notify"]; !errors.Is(err, fanout.ErrPanic) || !strings.HasPrefix(err.Error(), "fanout: check panicked: ") {
		t.Errorf("notify, whose Run is nil: Err = %v, want an error that wraps ErrPanic and starts with %q",
			err, "fanout: check panicked: ")
	}
}
