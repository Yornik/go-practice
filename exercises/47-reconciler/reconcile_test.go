package reconciler_test

import (
	"context"
	"errors"
	"maps"
	"testing"

	reconciler "github.com/yornik/go-practice/exercises/47-reconciler"
)

func TestReconcile(t *testing.T) {
	hangGuard(t)
	// Every case reconciles "web". The store also holds "db", whose desired
	// spec differs from its actual one: Reconcile("web") must not touch it.
	tests := []struct {
		name       string
		desired    spec // desired spec of web; nil: web is not desired
		actual     spec // spec of web in the store; nil: web does not exist
		wantAction reconciler.Action
	}{
		{"missing object is created", spec{"replicas": "3"}, nil, reconciler.Created},
		{"up-to-date object is left alone", spec{"replicas": "3", "image": "v2"}, spec{"image": "v2", "replicas": "3"}, reconciler.None},
		{"changed value is updated", spec{"replicas": "3", "image": "v2"}, spec{"replicas": "3", "image": "v1"}, reconciler.Updated},
		{"extra key is removed", spec{"replicas": "3"}, spec{"replicas": "3", "debug": "true"}, reconciler.Updated},
		{"missing key is added", spec{"replicas": "3", "image": "v2"}, spec{"replicas": "3"}, reconciler.Updated},
		{"object that is not desired is deleted", nil, spec{"replicas": "3"}, reconciler.Deleted},
		{"not desired and not there: nothing to do", nil, nil, reconciler.None},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			inner := reconciler.NewMemoryStore()
			existing := map[string]spec{"db": {"version": "15"}}
			if tc.actual != nil {
				existing["web"] = tc.actual
			}
			seed(t, inner, existing)
			dbBefore := mustGet(t, inner, "db")
			desired := map[string]spec{"db": {"version": "16"}}
			if tc.desired != nil {
				desired["web"] = tc.desired
			}
			s := &hookStore{inner: inner}

			action, err := reconciler.Reconcile(ctx, desired, s, "web")
			if err != nil || action != tc.wantAction {
				t.Fatalf("Reconcile(web) = %v, %v, want %v, nil", action, err, tc.wantAction)
			}

			// What the store holds now.
			if tc.desired == nil {
				expectGone(t, inner, "web")
			} else if got := mustGet(t, inner, "web"); !maps.Equal(got.Spec, tc.desired) {
				t.Fatalf("after Reconcile(web): spec = %v, want %v", got.Spec, tc.desired)
			}
			if db := mustGet(t, inner, "db"); db.ResourceVersion != dbBefore.ResourceVersion {
				t.Fatalf("Reconcile(web) changed db: it must touch only the object it was called for")
			}

			// How it got there: one Get, and one write only if something changed.
			if n := s.Count("get", "web"); n != 1 {
				t.Errorf("Reconcile(web) called Get(web) %d times, want 1", n)
			}
			writes := s.Writes()
			if tc.wantAction == reconciler.None && len(writes) != 0 {
				t.Fatalf("Reconcile(web) wrote %v although nothing had to change, want no write", writes)
			}
			if tc.wantAction != reconciler.None && len(writes) != 1 {
				t.Fatalf("Reconcile(web) wrote %v, want exactly 1 write", writes)
			}

			// Idempotent: the second call finds nothing to do.
			before := len(s.Writes())
			action, err = reconciler.Reconcile(ctx, desired, s, "web")
			if err != nil || action != reconciler.None {
				t.Fatalf("second Reconcile(web) = %v, %v, want None, nil (Reconcile must be idempotent)", action, err)
			}
			if after := s.Writes(); len(after) != before {
				t.Fatalf("second Reconcile(web) wrote %v, want no write", after[before:])
			}
		})
	}
}

// TestReconcileEmptySpec checks that "no spec" and "empty spec" are the same
// spec, and that a name in desired with a nil spec is still desired.
func TestReconcileEmptySpec(t *testing.T) {
	hangGuard(t)
	tests := []struct {
		name    string
		desired spec // the desired spec of web, which is in desired
		actual  spec // the spec web has in the store
	}{
		{"desired empty, actual nil", spec{}, nil},
		{"desired nil, actual empty", nil, spec{}},
		{"desired nil, actual nil", nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inner := reconciler.NewMemoryStore()
			if _, err := inner.Create(context.Background(), reconciler.Object{Name: "web", Spec: tc.actual}); err != nil {
				t.Fatalf("Create(web) error = %v", err)
			}
			s := &hookStore{inner: inner}
			desired := map[string]spec{"web": tc.desired}
			action, err := reconciler.Reconcile(context.Background(), desired, s, "web")
			if err != nil || action != reconciler.None {
				t.Fatalf("Reconcile(web) with desired spec %#v and actual spec %#v = %v, %v, want None, nil", tc.desired, tc.actual, action, err)
			}
			if w := s.Writes(); len(w) != 0 {
				t.Fatalf("Reconcile(web) wrote %v, want no write", w)
			}
		})
	}
}

func TestReconcileUpdateSendsVersionItRead(t *testing.T) {
	hangGuard(t)
	inner := reconciler.NewMemoryStore()
	seed(t, inner, map[string]spec{"other": {}, "web": {"replicas": "1"}})
	read := mustGet(t, inner, "web").ResourceVersion
	s := &hookStore{inner: inner}

	action, err := reconciler.Reconcile(context.Background(), map[string]spec{"web": {"replicas": "3"}}, s, "web")
	if err != nil || action != reconciler.Updated {
		t.Fatalf("Reconcile(web) = %v, %v, want Updated, nil", action, err)
	}
	for _, c := range s.Calls() {
		if c.Op == "update" && c.ResourceVersion != read {
			t.Fatalf("Update sent ResourceVersion %d, want %d (the version Reconcile read with Get)", c.ResourceVersion, read)
		}
	}
}

// TestReconcileConflict plays the race that optimistic concurrency exists
// for: right after Reconcile has read "web", an operator scales it by hand.
func TestReconcileConflict(t *testing.T) {
	hangGuard(t)
	ctx := context.Background()
	inner := reconciler.NewMemoryStore()
	seed(t, inner, map[string]spec{"web": {"replicas": "1"}})
	desired := map[string]spec{"web": {"replicas": "3"}}

	calls := 0
	s := &hookStore{inner: inner, hook: func(op, name string) error {
		calls++
		if calls == 2 { // the call after the first Get: someone else writes first
			obj := mustGet(t, inner, "web")
			obj.Spec["replicas"] = "2"
			if _, err := inner.Update(ctx, obj); err != nil {
				t.Errorf("the operator's Update failed: %v", err)
			}
		}
		return nil
	}}

	action, err := reconciler.Reconcile(ctx, desired, s, "web")
	if !errors.Is(err, reconciler.ErrConflict) || action != reconciler.None {
		t.Fatalf("Reconcile(web) after a concurrent change = %v, %v, want None and an error that wraps ErrConflict", action, err)
	}
	if got := mustGet(t, inner, "web").Spec; !maps.Equal(got, spec{"replicas": "2"}) {
		t.Fatalf("spec of web = %v, want map[replicas:2]: Reconcile must not overwrite a change it has not seen", got)
	}

	// The retry reads the new version, decides again, and wins.
	action, err = reconciler.Reconcile(ctx, desired, s, "web")
	if err != nil || action != reconciler.Updated {
		t.Fatalf("Reconcile(web) retry = %v, %v, want Updated, nil", action, err)
	}
	if got := mustGet(t, inner, "web").Spec; !maps.Equal(got, spec{"replicas": "3"}) {
		t.Fatalf("spec of web after the retry = %v, want map[replicas:3]", got)
	}
}

func TestReconcileStoreErrors(t *testing.T) {
	hangGuard(t)
	ctx := context.Background()

	// failing returns a hook that fails every call of op.
	failing := func(op string) func(string, string) error {
		return func(o, _ string) error {
			if o == op {
				return errUnavailable
			}
			return nil
		}
	}

	tests := []struct {
		name    string
		desired spec // desired spec of web; nil: not desired
		actual  spec // spec of web in the store; nil: does not exist
		failOp  string
	}{
		// A failed Get is not "not found": creating the object now could
		// fail later, or worse, a "not found" for an object that is not
		// desired would be taken as "nothing to do".
		{"Get fails", spec{"replicas": "3"}, nil, "get"},
		{"Create fails", spec{"replicas": "3"}, nil, "create"},
		{"Update fails", spec{"replicas": "3"}, spec{"replicas": "1"}, "update"},
		{"Delete fails", nil, spec{"replicas": "1"}, "delete"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			inner := reconciler.NewMemoryStore()
			if tc.actual != nil {
				seed(t, inner, map[string]spec{"web": tc.actual})
			}
			desired := map[string]spec{}
			if tc.desired != nil {
				desired["web"] = tc.desired
			}
			s := &hookStore{inner: inner, hook: failing(tc.failOp)}
			action, err := reconciler.Reconcile(ctx, desired, s, "web")
			if !errors.Is(err, errUnavailable) || action != reconciler.None {
				t.Fatalf("Reconcile(web) when %s = %v, %v, want None and an error that wraps %q", tc.name, action, err, errUnavailable)
			}
			if tc.failOp == "get" {
				if w := s.Writes(); len(w) != 0 {
					t.Fatalf("Reconcile(web) wrote %v after Get failed, want no write", w)
				}
			}
		})
	}

	t.Run("Create finds that someone else created it", func(t *testing.T) {
		inner := reconciler.NewMemoryStore()
		desired := map[string]spec{"web": {"replicas": "3"}}
		s := &hookStore{inner: inner, hook: func(op, name string) error {
			if op == "create" {
				// Another controller replica was faster.
				_, _ = inner.Create(ctx, reconciler.Object{Name: name, Spec: spec{"replicas": "1"}})
			}
			return nil
		}}
		action, err := reconciler.Reconcile(ctx, desired, s, "web")
		if !errors.Is(err, reconciler.ErrAlreadyExists) || action != reconciler.None {
			t.Fatalf("Reconcile(web) = %v, %v, want None and an error that wraps ErrAlreadyExists", action, err)
		}
	})

	t.Run("Delete finds that someone else deleted it", func(t *testing.T) {
		inner := reconciler.NewMemoryStore()
		seed(t, inner, map[string]spec{"web": {"replicas": "1"}})
		s := &hookStore{inner: inner, hook: func(op, name string) error {
			if op == "delete" {
				if err := inner.Delete(ctx, name); err != nil {
					t.Errorf("the other Delete failed: %v", err)
				}
			}
			return nil
		}}
		action, err := reconciler.Reconcile(ctx, map[string]spec{}, s, "web")
		if err != nil || action != reconciler.None {
			t.Fatalf("Reconcile(web) = %v, %v, want None, nil: the object is gone, which is what we wanted", action, err)
		}
	})
}
