package logmerge_test

import (
	"fmt"
	"iter"
	"math/rand/v2"
	"slices"
	"testing"
	"time"

	logmerge "github.com/yornik/go-practice/exercises/13-logmerge"
)

func TestReorderStepByStep(t *testing.T) {
	// The example from the README, step by step. Entries arrive with these
	// times (seconds after t0); the window is 2 s. Their lines are x0 to x6.
	src := newSource("x", 10, 12, 11, 15, 13, 9, 16)
	var stats logmerge.ReorderStats
	type step struct {
		line   string
		pulled int32 // entries pulled from src when this one was yielded
		late   int   // stats.Late as the loop body sees it
	}
	var got []step
	within(t, "ranging over Reorder", func() {
		for e := range logmerge.Reorder(src.seq(), 2*time.Second, &stats) {
			got = append(got, step{e.Line, src.pulled.Load(), stats.Late})
		}
	})
	want := []step{
		{"x0", 2, 0}, // 10 s: yielded when 12 arrives (12 - 10 >= 2)
		{"x2", 4, 0}, // 11 s: yielded when 15 arrives
		{"x1", 4, 0}, // 12 s: also when 15 arrives
		{"x4", 5, 0}, // 13 s: exactly 2 s behind 15, yielded as soon as it arrives
		{"x5", 6, 1}, // 9 s: late (13 was already yielded): yielded at once and counted
		{"x3", 7, 1}, // 15 s: when src ends
		{"x6", 7, 1}, // 16 s
	}
	if len(got) != len(want) {
		t.Fatalf("Reorder yielded %d entries %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got line %s after %d pulls with stats.Late %d, want line %s after %d pulls with stats.Late %d",
				i, got[i].line, got[i].pulled, got[i].late, want[i].line, want[i].pulled, want[i].late)
		}
	}
	if stats.Late != 1 {
		t.Errorf("stats.Late = %d at the end, want 1", stats.Late)
	}
}

func TestReorderRules(t *testing.T) {
	tests := []struct {
		name   string
		window time.Duration
		in     []float64 // arrival order, seconds after t0; entry i has the line x<i>
		want   []int     // the expected output, as indexes into in
		late   int
	}{
		{"sorted input stays sorted", 2 * time.Second,
			[]float64{1, 2, 3, 4, 5}, []int{0, 1, 2, 3, 4}, 0},
		{"a window longer than the stream sorts everything", time.Hour,
			[]float64{5, 3, 9, 1, 7}, []int{3, 1, 0, 4, 2}, 0},
		{"equal times keep their arrival order", 5 * time.Second,
			[]float64{10, 10, 11, 10, 10, 9.5, 10, 20}, []int{5, 0, 1, 3, 4, 6, 2, 7}, 0},
		{"exactly window behind the newest is in time", 2 * time.Second,
			[]float64{10, 12, 10}, []int{0, 2, 1}, 0},
		{"more than window behind but not before anything yielded is not late", 2 * time.Second,
			[]float64{10, 13, 10.5}, []int{0, 2, 1}, 0},
		{"a late entry is yielded at once and counted", 2 * time.Second,
			[]float64{10, 13, 9, 14}, []int{0, 2, 1, 3}, 1},
		{"late means before the latest time yielded, not the last one", 2 * time.Second,
			[]float64{10, 13, 15, 11, 12}, []int{0, 1, 3, 4, 2}, 2},
		{"sub-second window", 500 * time.Millisecond,
			[]float64{1, 1.2, 1.1, 1.6, 1.3}, []int{0, 2, 1, 4, 3}, 0},
		{"window 0 yields in arrival order and counts late entries", 0,
			[]float64{3, 1, 2, 5, 4}, []int{0, 1, 2, 3, 4}, 3},
		{"a negative window behaves like 0", -time.Second,
			[]float64{3, 1, 2, 5, 4}, []int{0, 1, 2, 3, 4}, 3},
		{"one entry", 2 * time.Second,
			[]float64{7}, []int{0}, 0},
		{"empty source", 2 * time.Second,
			nil, nil, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := newSource("x", tc.in...)
			var stats logmerge.ReorderStats
			got := collect(t, "Reorder", func() iter.Seq[logmerge.Entry] {
				return logmerge.Reorder(src.seq(), tc.window, &stats)
			})
			var want []logmerge.Entry
			for _, i := range tc.want {
				want = append(want, src.entries[i])
			}
			checkEntries(t, fmt.Sprintf("Reorder(window %v) of %s", tc.window, show(src.entries)), got, want)
			if stats.Late != tc.late {
				t.Errorf("stats.Late = %d, want %d", stats.Late, tc.late)
			}
		})
	}
}

func TestReorderNilStats(t *testing.T) {
	// stats may be nil, also when there are late entries.
	src := newSource("x", 10, 13, 9, 14)
	got := collect(t, "Reorder with nil stats", func() iter.Seq[logmerge.Entry] {
		return logmerge.Reorder(src.seq(), 2*time.Second, nil)
	})
	want := []logmerge.Entry{src.entries[0], src.entries[2], src.entries[1], src.entries[3]}
	checkEntries(t, "Reorder with nil stats", got, want)
}

func TestReorderBoundedDisorder(t *testing.T) {
	// A log shipper delivers each line 0 to 3 s after it was written, so the
	// lines arrive out of order, but never more than 3 s behind a line that
	// arrived before them. With a window of 3 s, Reorder must yield them in
	// time order (equal times in arrival order) and count nothing as late.
	const maxDelay = 3000 // ms
	r := rand.New(rand.NewPCG(13, 3))
	for round := range 200 {
		type line struct{ written, arrives int } // ms after t0
		var lines []line
		written := 0
		for range r.IntN(80) {
			written += 100 * r.IntN(4) // steps of 0 to 300 ms: some equal times
			lines = append(lines, line{written, written + 100*r.IntN(maxDelay/100+1)})
		}
		slices.SortStableFunc(lines, func(a, b line) int { return a.arrives - b.arrives })
		secs := make([]float64, len(lines))
		for i, l := range lines {
			secs[i] = float64(l.written) / 1000
		}
		src := newSource("x", secs...)
		want := slices.Clone(src.entries)
		slices.SortStableFunc(want, func(a, b logmerge.Entry) int { return a.Time.Compare(b.Time) })

		var stats logmerge.ReorderStats
		got := collect(t, "Reorder", func() iter.Seq[logmerge.Entry] {
			return logmerge.Reorder(src.seq(), maxDelay*time.Millisecond, &stats)
		})
		if !equalEntries(got, want) {
			t.Fatalf("round %d: Reorder(window 3s) of\n  %s\nyielded\n  %s\nwant\n  %s",
				round, show(src.entries), show(got), show(want))
		}
		if stats.Late != 0 {
			t.Fatalf("round %d: stats.Late = %d, want 0: no entry is more than the window behind", round, stats.Late)
		}
	}
}

func TestReorderIsLazy(t *testing.T) {
	t.Run("calling Reorder does not start src", func(t *testing.T) {
		src := newSource("x", 1, 2, 3)
		within(t, "Reorder", func() { logmerge.Reorder(src.seq(), time.Second, nil) })
		if got := src.starts.Load(); got != 0 {
			t.Errorf("calling Reorder started src; nothing may happen before you range over the result")
		}
	})
	t.Run("endless source", func(t *testing.T) {
		// Like kubectl logs -f: 1 entry per second, forever. With a 3 s window
		// entry i is yielded right after entry i+3 is pulled.
		src := &endless{name: "x", start: 0, step: 1}
		var got []string
		within(t, "ranging over Reorder of an endless source", func() {
			for e := range logmerge.Reorder(src.seq(), 3*time.Second, nil) {
				got = append(got, e.Line)
				if len(got) == 5 {
					break
				}
			}
		})
		if want := []string{"x0", "x1", "x2", "x3", "x4"}; !slices.Equal(got, want) {
			t.Errorf("first 5 lines = %v, want %v", got, want)
		}
		if p := src.pulled.Load(); p != 8 {
			t.Errorf("Reorder pulled %d entries to yield 5, want 8 (x4 is yielded right after x7 is pulled)", p)
		}
		if c := src.cleanups.Load(); c != 1 {
			t.Errorf("deferred cleanup of src ran %d times after the loop, want 1", c)
		}
	})
}

func TestReorderStopsSourceOnBreak(t *testing.T) {
	tests := []struct {
		name       string
		window     time.Duration
		in         []float64
		stopAfter  int
		wantPulled int32
	}{
		// x0 is yielded when x2 (3 s) arrives.
		{"break at the first entry", 2 * time.Second, []float64{1, 2, 3, 4, 5, 6}, 1, 3},
		// The entry at 10 s makes 1, 2, 3 and 4 ready at once. The loop
		// breaks after the second of them: Reorder must not yield the third.
		{"break while several entries are ready", 5 * time.Second, []float64{1, 2, 3, 4, 10, 11}, 2, 5},
		// Everything is yielded after src ended. Break after the first one.
		{"break while emptying the buffer at the end", time.Hour, []float64{3, 1, 2}, 1, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := newSource("x", tc.in...)
			var n int
			within(t, "ranging over Reorder", func() {
				for range logmerge.Reorder(src.seq(), tc.window, nil) {
					n++
					if n == tc.stopAfter {
						break
					}
				}
			})
			if n != tc.stopAfter {
				t.Fatalf("the loop saw %d entries, want %d", n, tc.stopAfter)
			}
			if got := src.pulled.Load(); got != tc.wantPulled {
				t.Errorf("Reorder pulled %d entries from src, want %d", got, tc.wantPulled)
			}
			if got := src.cleanups.Load(); got != 1 {
				t.Errorf("deferred cleanup of src ran %d times when the loop ended, want 1", got)
			}
		})
	}
}
