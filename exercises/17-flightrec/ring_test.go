package flightrec_test

import (
	"fmt"
	"slices"
	"testing"

	flightrec "github.com/yornik/go-practice/exercises/17-flightrec"
)

// adds describes a ring for failure messages: "NewRing(3) after Add 1 to 4".
func adds(size int, values []int) string {
	switch len(values) {
	case 0:
		return fmt.Sprintf("NewRing(%d) with no Add", size)
	case 1:
		return fmt.Sprintf("NewRing(%d) after Add(%d)", size, values[0])
	}
	return fmt.Sprintf("NewRing(%d) after Add %v", size, values)
}

func TestRingBasicFill(t *testing.T) {
	tests := []struct {
		name    string
		size    int
		adds    []int
		want    []int // the values the ring holds, oldest first
		dropped int
	}{
		{name: "empty", size: 3, adds: nil, want: nil, dropped: 0},
		{name: "partial", size: 3, adds: seq(1, 2), want: []int{1, 2}, dropped: 0},
		{name: "exactly full", size: 3, adds: seq(1, 3), want: []int{1, 2, 3}, dropped: 0},
		{name: "1 over", size: 3, adds: seq(1, 4), want: []int{2, 3, 4}, dropped: 1},
		{name: "wrapped 3 times", size: 4, adds: seq(1, 12), want: []int{9, 10, 11, 12}, dropped: 8},
		{name: "wrapped 3 times and 2", size: 4, adds: seq(1, 14), want: []int{11, 12, 13, 14}, dropped: 10},
		{name: "size 1, 1 value", size: 1, adds: []int{7}, want: []int{7}, dropped: 0},
		{name: "size 1, 5 values", size: 1, adds: seq(1, 5), want: []int{5}, dropped: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer guard(t)()
			r := newRing(tt.size, tt.adds...)
			what := adds(tt.size, tt.adds)

			if got := r.Len(); got != len(tt.want) {
				t.Errorf("%s: Len() = %d, want %d", what, got, len(tt.want))
			}
			if got := r.Dropped(); got != tt.dropped {
				t.Errorf("%s: Dropped() = %d, want %d", what, got, tt.dropped)
			}
			wantOldest, wantNewest, wantOK := 0, 0, len(tt.want) > 0
			if wantOK {
				wantOldest, wantNewest = tt.want[0], tt.want[len(tt.want)-1]
			}
			if got, ok := r.Oldest(); got != wantOldest || ok != wantOK {
				t.Errorf("%s: Oldest() = %d, %v; want %d, %v", what, got, ok, wantOldest, wantOK)
			}
			if got, ok := r.Newest(); got != wantNewest || ok != wantOK {
				t.Errorf("%s: Newest() = %d, %v; want %d, %v", what, got, ok, wantNewest, wantOK)
			}
			// slices.Equal treats nil and an empty slice as equal: both hold
			// no values.
			if got := r.Last(tt.size); !slices.Equal(got, tt.want) {
				t.Errorf("%s: Last(%d) = %v, want %v", what, tt.size, got, tt.want)
			}
		})
	}
}

func TestRingBasicAddOneByOne(t *testing.T) {
	// The same ring checked after every Add, so that a failure names the
	// first Add after which the ring is wrong.
	defer guard(t)()
	r := flightrec.NewRing[int](3)
	var all []int
	for v := 1; v <= 10; v++ {
		r.Add(v)
		all = append(all, v)
		want := all[max(0, len(all)-3):]
		what := adds(3, all)
		if got := r.Last(3); !slices.Equal(got, want) {
			t.Fatalf("%s: Last(3) = %v, want %v", what, got, want)
		}
		if got, wantDropped := r.Dropped(), len(all)-len(want); got != wantDropped {
			t.Fatalf("%s: Dropped() = %d, want %d", what, got, wantDropped)
		}
	}
}

// event is a value type the tests store in a ring.
type event struct {
	Feeder string
	Seq    int
}

// checkEmpty checks that a new ring of size 2 for T reports no values and
// the zero value of T, then that a stored zero value is reported as present.
func checkEmpty[T comparable](t *testing.T, typ string) {
	t.Helper()
	defer guard(t)()
	var zero T
	r := flightrec.NewRing[T](2)
	if got := r.Len(); got != 0 {
		t.Errorf("NewRing[%s](2): Len() = %d, want 0", typ, got)
	}
	if got := r.Dropped(); got != 0 {
		t.Errorf("NewRing[%s](2): Dropped() = %d, want 0", typ, got)
	}
	if got, ok := r.Oldest(); got != zero || ok {
		t.Errorf("NewRing[%s](2): Oldest() = %#v, %v; want %#v, false", typ, got, ok, zero)
	}
	if got, ok := r.Newest(); got != zero || ok {
		t.Errorf("NewRing[%s](2): Newest() = %#v, %v; want %#v, false", typ, got, ok, zero)
	}
	if got := r.Last(2); len(got) != 0 {
		t.Errorf("NewRing[%s](2): Last(2) = %#v, want no values", typ, got)
	}

	// A stored zero value is a value: the bool tells it apart from "empty".
	r.Add(zero)
	if got, ok := r.Oldest(); got != zero || !ok {
		t.Errorf("NewRing[%s](2) after Add(%#v): Oldest() = %#v, %v; want %#v, true", typ, zero, got, ok, zero)
	}
	if got, ok := r.Newest(); got != zero || !ok {
		t.Errorf("NewRing[%s](2) after Add(%#v): Newest() = %#v, %v; want %#v, true", typ, zero, got, ok, zero)
	}
	if got := r.Len(); got != 1 {
		t.Errorf("NewRing[%s](2) after Add(%#v): Len() = %d, want 1", typ, zero, got)
	}
}

func TestRingBasicEmpty(t *testing.T) {
	t.Run("int", func(t *testing.T) { checkEmpty[int](t, "int") })
	t.Run("string", func(t *testing.T) { checkEmpty[string](t, "string") })
	t.Run("pointer", func(t *testing.T) { checkEmpty[*event](t, "*event") })
	t.Run("struct", func(t *testing.T) { checkEmpty[event](t, "event") })
}

func TestRingBasicStructs(t *testing.T) {
	defer guard(t)()
	a := event{Feeder: "feeder-0412", Seq: 1}
	b := event{Feeder: "feeder-0412", Seq: 2}
	c := event{Feeder: "hq-window-01", Seq: 1}
	r := newRing(2, a, b, c)
	if got, want := r.Last(2), []event{b, c}; !slices.Equal(got, want) {
		t.Errorf("NewRing[event](2) after Add a, b, c: Last(2) = %+v, want %+v", got, want)
	}

	p, q := &event{Feeder: "photo-store-2", Seq: 1}, &event{Feeder: "photo-store-2", Seq: 2}
	rp := newRing(5, p, q)
	if got, ok := rp.Oldest(); got != p || !ok {
		t.Errorf("NewRing[*event](5) after Add p, q: Oldest() = %p, %v; want p (%p), true", got, ok, p)
	}
	if got, ok := rp.Newest(); got != q || !ok {
		t.Errorf("NewRing[*event](5) after Add p, q: Newest() = %p, %v; want q (%p), true", got, ok, q)
	}
}

func TestRingBasicLast(t *testing.T) {
	// A ring of size 5 after Add 1 to 7 holds 3, 4, 5, 6, 7, and the values
	// wrap around the end of the ring. A ring of size 5 after Add 1 and 2
	// is not full.
	full := []int{1, 2, 3, 4, 5, 6, 7}
	partial := []int{1, 2}
	tests := []struct {
		name   string
		values []int
		n      int
		want   []int
	}{
		{"full, n = -1", full, -1, nil},
		{"full, n = 0", full, 0, nil},
		{"full, n = 1", full, 1, []int{7}},
		{"full, n = 2", full, 2, []int{6, 7}},
		{"full, n = 3", full, 3, []int{5, 6, 7}},
		{"full, n = 4", full, 4, []int{4, 5, 6, 7}},
		{"full, n = Len", full, 5, []int{3, 4, 5, 6, 7}},
		{"full, n = Len + 1", full, 6, []int{3, 4, 5, 6, 7}},
		{"full, n = 100", full, 100, []int{3, 4, 5, 6, 7}},
		{"partial, n = 1", partial, 1, []int{2}},
		{"partial, n = Len", partial, 2, []int{1, 2}},
		{"partial, n = size", partial, 5, []int{1, 2}},
		{"partial, n = 0", partial, 0, nil},
		{"empty, n = 3", nil, 3, nil},
		{"empty, n = 0", nil, 0, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer guard(t)()
			r := newRing(5, tt.values...)
			if got := r.Last(tt.n); !slices.Equal(got, tt.want) {
				t.Errorf("%s: Last(%d) = %v, want %v", adds(5, tt.values), tt.n, got, tt.want)
			}
		})
	}
}

func TestRingBasicCopies(t *testing.T) {
	t.Run("change an element", func(t *testing.T) {
		defer guard(t)()
		// Size 4 after Add 1 to 6: the values wrap around the end.
		r := newRing(4, seq(1, 6)...)
		got := r.Last(4)
		if len(got) != 4 {
			t.Fatalf("NewRing(4) after Add 1 to 6: Last(4) = %v, want [3 4 5 6]", got)
		}
		for i := range got {
			got[i] = -1
		}
		if again := r.Last(4); !slices.Equal(again, []int{3, 4, 5, 6}) {
			t.Errorf("after setting every element of the result of Last(4) to -1: Last(4) = %v, want [3 4 5 6]", again)
		}
		if v, ok := r.Oldest(); v != 3 || !ok {
			t.Errorf("after setting every element of the result of Last(4) to -1: Oldest() = %d, %v; want 3, true", v, ok)
		}
	})

	t.Run("append to it", func(t *testing.T) {
		defer guard(t)()
		// A ring that is not full has room after its values. A result that
		// shares that room changes when the ring writes into it.
		r := newRing(4, 1, 2)
		got := append(r.Last(4), 100)
		r.Add(3)
		if !slices.Equal(got, []int{1, 2, 100}) {
			t.Errorf("NewRing(4) after Add 1, 2: s := append(Last(4), 100), then Add(3): s = %v, want [1 2 100]", got)
		}
		if again := r.Last(4); !slices.Equal(again, []int{1, 2, 3}) {
			t.Errorf("NewRing(4) after Add 1, 2: s := append(Last(4), 100), then Add(3): Last(4) = %v, want [1 2 3]", again)
		}
	})

	t.Run("add later", func(t *testing.T) {
		defer guard(t)()
		r := newRing(3, 1, 2, 3)
		got := r.Last(3)
		r.Add(4)
		r.Add(5)
		if !slices.Equal(got, []int{1, 2, 3}) {
			t.Errorf("NewRing(3) after Add 1, 2, 3: s := Last(3), then Add(4), Add(5): s = %v, want [1 2 3]", got)
		}
	})
}

func TestRingBasicPanics(t *testing.T) {
	defer guard(t)()
	if r := flightrec.NewRing[int](1); r == nil || r.Len() != 0 {
		t.Fatalf("NewRing(1) = %v, want an empty ring", r)
	}
	for _, size := range []int{0, -1} {
		if v := panicValue(func() { flightrec.NewRing[int](size) }); v == nil {
			t.Errorf("NewRing(%d) did not panic, want a panic", size)
		}
	}
}
