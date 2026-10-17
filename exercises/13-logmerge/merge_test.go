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

// seqs returns the iter.Seq of every test source.
func seqs(srcs []*source) []iter.Seq[logmerge.Entry] {
	out := make([]iter.Seq[logmerge.Entry], len(srcs))
	for i, s := range srcs {
		out[i] = s.seq()
	}
	return out
}

// sourcesFrom makes sources named a, b, c, ... with entries at the given
// seconds after t0. Source a's lines are a0, a1, ...
func sourcesFrom(secs [][]float64) []*source {
	srcs := make([]*source, len(secs))
	for i, s := range secs {
		srcs[i] = newSource(string(rune('a'+i)), s...)
	}
	return srcs
}

// entriesByLine looks up the entries of srcs by their lines.
func entriesByLine(srcs []*source, lines ...string) []logmerge.Entry {
	byLine := map[string]logmerge.Entry{}
	for _, s := range srcs {
		for _, e := range s.entries {
			byLine[e.Line] = e
		}
	}
	var out []logmerge.Entry
	for _, l := range lines {
		e, ok := byLine[l]
		if !ok {
			panic("test bug: no entry with line " + l)
		}
		out = append(out, e)
	}
	return out
}

func TestMergeOrder(t *testing.T) {
	tests := []struct {
		name string
		secs [][]float64 // the entries of sources a, b, c, ... in seconds after t0
		want []string    // the lines in the expected output order
	}{
		{"two pods interleaved", [][]float64{{1, 3, 5}, {2, 4, 6}},
			[]string{"a0", "b0", "a1", "b1", "a2", "b2"}},
		{"one source", [][]float64{{1, 2, 3}},
			[]string{"a0", "a1", "a2"}},
		{"one pod before the other", [][]float64{{3, 4}, {1, 2}},
			[]string{"b0", "b1", "a0", "a1"}},
		{"three pods, uneven", [][]float64{{0, 10, 11}, {5}, {1, 2, 12}},
			[]string{"a0", "c0", "c1", "b0", "a1", "a2", "c2"}},
		{"sub-second differences", [][]float64{{0.001, 0.003}, {0.002, 0.0025}},
			[]string{"a0", "b0", "b1", "a1"}},
		{"empty sources are skipped", [][]float64{{}, {2}, {}, {1, 3}, {}},
			[]string{"d0", "b0", "d1"}},
		{"only empty sources", [][]float64{{}, {}}, nil},
		{"no sources", nil, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srcs := sourcesFrom(tc.secs)
			got := collect(t, "Merge", func() iter.Seq[logmerge.Entry] { return logmerge.Merge(seqs(srcs)...) })
			checkEntries(t, "Merge", got, entriesByLine(srcs, tc.want...))
		})
	}
}

func TestMergeTies(t *testing.T) {
	// Three pods log at the same instants. Equal times come out by source
	// index (a before b before c), and in source order within one source.
	srcs := sourcesFrom([][]float64{
		{10, 10, 11},
		{10, 11},
		{9, 10, 10, 11},
	})
	got := collect(t, "Merge", func() iter.Seq[logmerge.Entry] { return logmerge.Merge(seqs(srcs)...) })
	want := entriesByLine(srcs, "c0", "a0", "a1", "b0", "c1", "c2", "a2", "b1", "c3")
	checkEntries(t, "Merge", got, want)
}

func TestMergeTimeZones(t *testing.T) {
	// A database host logs in local time (+02:00), the pods log in UTC. The
	// same instant written in two zones is a tie, so source order decides:
	// db is source 0, so its entry goes before api's entry at 12:00:05Z.
	cest := time.FixedZone("CEST", 2*60*60)
	db := []logmerge.Entry{
		{Time: at(5).In(cest), Source: "db", Line: "failover started"}, // 14:00:05+02:00
	}
	api := []logmerge.Entry{
		{Time: at(1), Source: "api", Line: "GET /orders 200"},
		{Time: at(5), Source: "api", Line: "GET /orders 503"}, // 12:00:05Z
		{Time: at(6), Source: "api", Line: "GET /orders 200"},
	}
	got := collect(t, "Merge", func() iter.Seq[logmerge.Entry] {
		return logmerge.Merge(slices.Values(db), slices.Values(api))
	})
	checkEntries(t, "Merge", got, []logmerge.Entry{api[0], db[0], api[1], api[2]})
}

func TestMergeMatchesStableSort(t *testing.T) {
	// Random sources with many equal times. The result must be a stable sort
	// by Time of all sources concatenated in argument order.
	r := rand.New(rand.NewPCG(13, 2026))
	for round := range 300 {
		var secs [][]float64
		for range r.IntN(7) { // 0 to 6 sources
			sec := float64(r.IntN(5))
			var s []float64
			for range r.IntN(12) { // 0 to 11 entries
				sec += float64(r.IntN(3)) // steps of 0, 1 or 2 s: many ties
				s = append(s, sec)
			}
			secs = append(secs, s)
		}
		srcs := sourcesFrom(secs)
		var want []logmerge.Entry
		for _, s := range srcs {
			want = append(want, s.entries...)
		}
		slices.SortStableFunc(want, func(x, y logmerge.Entry) int { return x.Time.Compare(y.Time) })

		got := collect(t, "Merge", func() iter.Seq[logmerge.Entry] { return logmerge.Merge(seqs(srcs)...) })
		if !equalEntries(got, want) {
			t.Fatalf("round %d: Merge of sources with entries at seconds %v yielded\n  %s\nwant\n  %s",
				round, secs, show(got), show(want))
		}
	}
}

func TestMergeIsLazy(t *testing.T) {
	// Merge holds at most 1 entry per source, and it pulls the next entry of
	// a source only after the loop body for that source's previous entry
	// returned. With kubectl logs -f, pulling first would hold back a line
	// Merge already has until the pod logs again.
	srcs := sourcesFrom([][]float64{{0, 3, 6, 9}, {1, 4, 7}, {2, 5, 8, 10, 11}, {}})
	var problems []string
	var n int
	within(t, "Merge", func() {
		merged := logmerge.Merge(seqs(srcs)...)
		for _, s := range srcs {
			if got := s.starts.Load(); got != 0 {
				problems = append(problems, fmt.Sprintf(
					"calling Merge started source %s; nothing may happen before you range over the result", s.name))
			}
		}
		yielded := map[string]int{}
		for e := range merged {
			n++
			yielded[e.Source]++
			// Checked inside the loop body, while Merge waits for the loop.
			for _, s := range srcs {
				ahead := int(s.pulled.Load()) - yielded[s.name]
				switch {
				case s.name == e.Source && ahead > 0:
					problems = append(problems, fmt.Sprintf(
						"while your loop handles entry %d (%s), Merge already pulled the next entry of source %s; "+
							"pull it only after the loop body returns", n, e.Line, s.name))
				case ahead > 1:
					problems = append(problems, fmt.Sprintf(
						"while your loop handles entry %d (%s), %d entries of source %s were pulled and not yet yielded; at most 1 may be",
						n, e.Line, ahead, s.name))
				}
			}
		}
	})
	if n != 12 {
		t.Errorf("Merge yielded %d entries, want 12", n)
	}
	for i, p := range problems {
		if i == 5 {
			t.Errorf("... and %d more", len(problems)-5)
			break
		}
		t.Error(p)
	}
}

func TestMergeEndlessSources(t *testing.T) {
	// Like kubectl logs -f for 2 pods: the sources never end. Merge must still
	// yield entries one by one.
	a := &endless{name: "a", start: 0, step: 2}
	b := &endless{name: "b", start: 1, step: 2}
	var got []string
	within(t, "ranging over Merge of 2 endless sources", func() {
		for e := range logmerge.Merge(a.seq(), b.seq()) {
			got = append(got, e.Line)
			if len(got) == 10 {
				break
			}
		}
	})
	want := []string{"a0", "b0", "a1", "b1", "a2", "b2", "a3", "b3", "a4", "b4"}
	if !slices.Equal(got, want) {
		t.Errorf("first 10 lines = %v, want %v", got, want)
	}
	for _, s := range []*endless{a, b} {
		if p := s.pulled.Load(); p > 6 {
			t.Errorf("source %s: %d entries pulled for 5 yielded; at most 1 per source may wait", s.name, p)
		}
		if c := s.cleanups.Load(); c != 1 {
			t.Errorf("source %s: deferred cleanup ran %d times after the loop, want 1", s.name, c)
		}
	}
}

func TestMergeStopsSourcesOnBreak(t *testing.T) {
	// Sources often hold resources: an open file, an HTTP response body, a
	// watch on the API server. When the consumer stops early, every source
	// must be stopped so its deferred cleanup runs.
	for _, stopAfter := range []int{1, 2, 5, 7} {
		t.Run(fmt.Sprintf("break after %d entries", stopAfter), func(t *testing.T) {
			// b ends after 1 entry, so it has finished on its own by the
			// time the loop breaks after 2 or more entries.
			srcs := sourcesFrom([][]float64{{0, 3, 6, 9}, {1}, {2, 5, 8}})
			var n int
			within(t, "ranging over Merge", func() {
				for range logmerge.Merge(seqs(srcs)...) {
					n++
					if n == stopAfter {
						break
					}
				}
			})
			if n != stopAfter {
				t.Fatalf("the loop saw %d entries, want %d", n, stopAfter)
			}
			// Checked right after the loop: the cleanup must already have run.
			for _, s := range srcs {
				if got := s.starts.Load(); got != 1 {
					t.Errorf("source %s was started %d times, want 1", s.name, got)
				}
				if got := s.cleanups.Load(); got != 1 {
					t.Errorf("source %s: deferred cleanup ran %d times when the loop ended, want 1: "+
						"Merge must stop every source it started before the loop ends", s.name, got)
				}
			}
		})
	}
}

func TestMergeRangeTwice(t *testing.T) {
	// An iter.Seq is a function. Each range over it calls it again, and
	// Merge's result must range over the sources again too.
	srcs := sourcesFrom([][]float64{{1, 3}, {2}})
	var first, second []logmerge.Entry
	within(t, "ranging twice over one Merge result", func() {
		merged := logmerge.Merge(seqs(srcs)...)
		for e := range merged {
			first = append(first, e)
		}
		for e := range merged {
			second = append(second, e)
		}
	})
	want := entriesByLine(srcs, "a0", "b0", "a1")
	checkEntries(t, "the first range over Merge", first, want)
	checkEntries(t, "the second range over Merge", second, want)
	for _, s := range srcs {
		if got := s.starts.Load(); got != 2 {
			t.Errorf("source %s was started %d times, want 2 (once per range)", s.name, got)
		}
	}
}
