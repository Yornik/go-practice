package counters_test

import (
	"errors"
	"testing"
)

// Step 2: the registry: the same counter for the same name, Snapshot, Drain
// and Write.

func TestRegistrySameName(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	first := counter(t, r, "uploads_eu")
	other := counter(t, r, "uploads_us")
	again := counter(t, r, "uploads_eu")
	if again != first {
		t.Errorf("Counter(\"uploads_eu\") returned %p the first time and %p the second time, want the same pointer", first, again)
	}
	if other == first {
		t.Errorf("Counter(\"uploads_us\") and Counter(\"uploads_eu\") returned the same pointer %p, want 2 counters", first)
	}
	add(t, first, 2)
	add(t, again, 3)
	wantValue(t, first, "uploads_eu", 5, "Add(2) and Add(3) on 2 results of Counter(\"uploads_eu\")")
}

func TestRegistryIndependent(t *testing.T) {
	defer guard(t)()
	a := newRegistry(t)
	b := newRegistry(t)
	ca := counter(t, a, "uploads_eu")
	cb := counter(t, b, "uploads_eu")
	if ca == cb {
		t.Fatalf("Counter(\"uploads_eu\") of 2 registries returned the same pointer %p, want 1 counter per registry", ca)
	}
	add(t, ca, 4)
	wantValue(t, cb, "uploads_eu", 0, "Add(4) on the counter of the other registry")
	wantSamples(t, "Snapshot() of the other registry", snapshot(t, b), s("uploads_eu", 0))
}

func TestRegistrySnapshotEmpty(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	if got := snapshot(t, r); len(got) != 0 {
		t.Errorf("Snapshot() of a new registry = %s, want a slice of length 0", show(got))
	}
}

func TestRegistrySnapshotSorted(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	// Created out of order on purpose; 1 counter stays at 0.
	add(t, counter(t, r, "uploads_us"), 2)
	add(t, counter(t, r, "uploads_eu"), 3)
	counter(t, r, "rejected")
	add(t, counter(t, r, "uploads_ap"), 1)
	add(t, counter(t, r, "uploads_eu"), 1)

	wantSamples(t, "Snapshot()", snapshot(t, r),
		s("rejected", 0), s("uploads_ap", 1), s("uploads_eu", 4), s("uploads_us", 2))
}

func TestRegistrySnapshotIsACopy(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	eu := counter(t, r, "uploads_eu")
	add(t, eu, 3)
	add(t, counter(t, r, "uploads_us"), 2)

	first := snapshot(t, r)
	wantSamples(t, "Snapshot()", first, s("uploads_eu", 3), s("uploads_us", 2))

	add(t, eu, 10)
	counter(t, r, "uploads_ap")
	wantSamples(t, "the result of the first Snapshot(), after Add(10) and a new counter", first,
		s("uploads_eu", 3), s("uploads_us", 2))

	if len(first) > 0 {
		first[0].Value = 999
		first[0].Name = "changed"
	}
	wantValue(t, eu, "uploads_eu", 13, "changing the slice that Snapshot returned")
	wantSamples(t, "Snapshot(), after changing the slice that the first Snapshot returned", snapshot(t, r),
		s("uploads_ap", 0), s("uploads_eu", 13), s("uploads_us", 2))
}

func TestRegistryDrain(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	eu := counter(t, r, "uploads_eu")
	add(t, eu, 4)
	add(t, counter(t, r, "uploads_us"), 2)
	counter(t, r, "rejected")

	wantSamples(t, "Drain()", drain(t, r), s("rejected", 0), s("uploads_eu", 4), s("uploads_us", 2))
	wantSamples(t, "Snapshot() after Drain()", snapshot(t, r), s("rejected", 0), s("uploads_eu", 0), s("uploads_us", 0))
	wantValue(t, eu, "uploads_eu", 0, "Drain()")
	wantSamples(t, "a second Drain()", drain(t, r), s("rejected", 0), s("uploads_eu", 0), s("uploads_us", 0))
}

func TestRegistryDrainKeepsCounters(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	eu := counter(t, r, "uploads_eu")
	add(t, eu, 4)
	drain(t, r)

	// A pointer from before Drain still counts, and the name still gives
	// that pointer.
	add(t, eu, 5)
	if again := counter(t, r, "uploads_eu"); again != eu {
		t.Errorf("after Drain(): Counter(\"uploads_eu\") = %p, want %p, the counter from before Drain", again, eu)
	}
	wantSamples(t, "Snapshot() after Add(4), Drain(), Add(5)", snapshot(t, r), s("uploads_eu", 5))
	wantSamples(t, "Drain() after Add(4), Drain(), Add(5)", drain(t, r), s("uploads_eu", 5))
}

func TestRegistryDrainEmpty(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	if got := drain(t, r); len(got) != 0 {
		t.Errorf("Drain() of a new registry = %s, want a slice of length 0", show(got))
	}
}

func TestRegistryWrite(t *testing.T) {
	defer guard(t)()
	tests := []struct {
		name string
		adds []addition
		want string
	}{
		{"no counters", nil, ""},
		{"1 counter at 0", []addition{{"rejected", 0}}, "rejected 0\n"},
		{
			"sorted by name",
			[]addition{{"uploads_us", 2}, {"uploads_eu", 3}, {"rejected", 0}, {"uploads_eu", 1}},
			"rejected 0\nuploads_eu 4\nuploads_us 2\n",
		},
		{"a large value", []addition{{"bytes_total", 1 << 50}}, "bytes_total 1125899906842624\n"},
		{"names are not checked", []addition{{"photo uploads", 1}, {"", 2}}, " 2\nphoto uploads 1\n"},
	}
	for _, tt := range tests {
		r := newRegistry(t)
		for _, a := range tt.adds {
			add(t, counter(t, r, a.name), a.n)
		}
		got, err := write(t, r)
		if err != nil {
			t.Errorf("%s: Write(w) returned the error %v, want nil", tt.name, err)
		}
		if got != tt.want {
			t.Errorf("%s: Write(w) wrote %q, want %q", tt.name, got, tt.want)
		}
	}
}

// addition is a counter name and an amount to add to it.
type addition struct {
	name string
	n    int64
}

func TestRegistryWriteChangesNothing(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	add(t, counter(t, r, "uploads_eu"), 3)
	for range 2 {
		got, err := write(t, r)
		if got != "uploads_eu 3\n" || err != nil {
			t.Errorf("Write(w) wrote %q and returned %v, want %q and nil", got, err, "uploads_eu 3\n")
		}
	}
	wantSamples(t, "Snapshot() after 2 calls of Write", snapshot(t, r), s("uploads_eu", 3))
}

// errDisk is what failingWriter returns.
var errDisk = errors.New("write /metrics: no space left on device")

// failingWriter fails every call of Write and counts the calls.
type failingWriter struct{ calls int }

func (w *failingWriter) Write(p []byte) (int, error) {
	w.calls++
	return 0, errDisk
}

func TestRegistryWriteError(t *testing.T) {
	defer guard(t)()
	r := newRegistry(t)
	add(t, counter(t, r, "uploads_eu"), 3)
	add(t, counter(t, r, "uploads_us"), 2)

	w := &failingWriter{}
	var err error
	within(t, "Write(w)", func() { err = r.Write(w) })
	if !errors.Is(err, errDisk) {
		t.Errorf("Write(w) with a w that fails = %v, want an error that matches %q (errors.Is)", err, errDisk)
	}
	if w.calls != 1 {
		t.Errorf("Write(w) called w.Write %d times, want 1: stop at the first failed write", w.calls)
	}
	wantSamples(t, "Snapshot() after a failed Write", snapshot(t, r), s("uploads_eu", 3), s("uploads_us", 2))
}
