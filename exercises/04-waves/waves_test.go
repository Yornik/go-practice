package waves_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	waves "github.com/yornik/go-practice/exercises/04-waves"
)

// equalWaves reports whether 2 lists of waves hold the same names in the
// same waves. A nil and an empty list count as equal.
func equalWaves(a, b [][]string) bool {
	return slices.EqualFunc(a, b, func(x, y []string) bool { return slices.Equal(x, y) })
}

func TestWavesSizes(t *testing.T) {
	ten := feeders(10)
	cases := []struct {
		name         string
		names        []string
		canary, size int
		want         [][]string
	}{
		{"canary 1, size 3", ten, 1, 3, [][]string{
			ten[0:1], ten[1:4], ten[4:7], ten[7:10],
		}},
		{"canary 2, size 3", ten, 2, 3, [][]string{
			ten[0:2], ten[2:5], ten[5:8], ten[8:10],
		}},
		{"canary 0: no canary wave", ten, 0, 4, [][]string{
			ten[0:4], ten[4:8], ten[8:10],
		}},
		{"last wave full", ten, 0, 5, [][]string{
			ten[0:5], ten[5:10],
		}},
		{"canary 1, last wave full", ten[:7], 1, 3, [][]string{
			ten[0:1], ten[1:4], ten[4:7],
		}},
		{"size 1", ten, 0, 1, [][]string{
			ten[0:1], ten[1:2], ten[2:3], ten[3:4], ten[4:5], ten[5:6], ten[6:7], ten[7:8], ten[8:9], ten[9:10],
		}},
		{"size larger than the list", ten, 0, 50, [][]string{ten}},
		{"size equal to the list", ten, 0, 10, [][]string{ten}},
		{"canary 1, size larger than the rest", ten, 1, 50, [][]string{ten[0:1], ten[1:10]}},
		{"canary larger than the list", ten, 12, 3, [][]string{ten}},
		{"canary equal to the list", ten, 10, 3, [][]string{ten}},
		{"canary 1 name short of the list", ten, 9, 3, [][]string{ten[0:9], ten[9:10]}},
		{"1 name", []string{"hq-window-01"}, 1, 50, [][]string{{"hq-window-01"}}},
		{"order and duplicates are kept", []string{"feeder-3", "feeder-1", "feeder-3", "feeder-2"}, 1, 2, [][]string{
			{"feeder-3"}, {"feeder-1", "feeder-3"}, {"feeder-2"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := waves.Waves(slices.Clone(tc.names), tc.canary, tc.size)
			if err != nil || !equalWaves(got, tc.want) {
				t.Errorf("Waves(%q, %d, %d)\n got  %q, %v\n want %q, nil", tc.names, tc.canary, tc.size, got, err, tc.want)
			}
		})
	}
}

func TestWavesEmpty(t *testing.T) {
	cases := []struct {
		names        []string
		canary, size int
	}{
		{nil, 1, 50},
		{[]string{}, 1, 50},
		{nil, 0, 1},
		{[]string{}, 5, 3},
	}
	for _, tc := range cases {
		got, err := waves.Waves(tc.names, tc.canary, tc.size)
		if err != nil || len(got) != 0 {
			t.Errorf("Waves(%#v, %d, %d) = %q, %v; want no waves and a nil error", tc.names, tc.canary, tc.size, got, err)
		}
	}
}

// TestWavesErrors tries every bad value with empty names first. A Waves
// that accepts a size below 1 can loop forever on real names, so the cases
// with names run only when the empty ones pass. callWaves stops a loop that
// still never ends.
func TestWavesErrors(t *testing.T) {
	cases := []struct {
		name         string
		names        []string
		canary, size int
		bad          string // the bad value, as the error must contain it
	}{
		{"size 0 with empty names", nil, 0, 0, "0"},
		{"size -7 with empty names", []string{}, 1, -7, "-7"},
		{"canary -2 with empty names", []string{}, -2, 50, "-2"},
		{"canary -1", feeders(10), -1, 3, "-1"},
		{"canary -50", feeders(10), -50, 3, "-50"},
		{"size 0", feeders(10), 1, 0, "0"},
		{"size -7", feeders(10), 0, -7, "-7"},
	}
	for _, tc := range cases {
		ok := t.Run(tc.name, func(t *testing.T) {
			got, err := callWaves(t, tc.names, tc.canary, tc.size)
			if err == nil {
				t.Fatalf("Waves(%#v, %d, %d) = %q, nil; want an error", tc.names, tc.canary, tc.size, got)
			}
			if !strings.Contains(err.Error(), tc.bad) {
				t.Errorf("Waves(%#v, %d, %d) returned the error %q; want an error that contains the bad value %s", tc.names, tc.canary, tc.size, err, tc.bad)
			}
			if got != nil {
				t.Errorf("Waves(%#v, %d, %d) returned the waves %q with an error; want nil waves", tc.names, tc.canary, tc.size, got)
			}
		})
		if !ok && len(tc.names) == 0 {
			t.Fatalf("the cases with 10 names are skipped until the cases with empty names pass")
		}
	}
}

// TestWavesIndependent changes the waves and the input after the call, the
// way a caller does who adds a host to a wave, and checks that nothing else
// changes.
func TestWavesIndependent(t *testing.T) {
	// plan returns 10 names with 1 spare slot behind them, and the waves of
	// Waves(names, canary, size).
	plan := func(t *testing.T, canary, size int) (backing, names []string, w [][]string) {
		t.Helper()
		backing = append(feeders(10), "SPARE")
		names = backing[:10]
		w, err := waves.Waves(names, canary, size)
		if err != nil || len(w) < 3 {
			t.Fatalf("Waves(%q, %d, %d) = %q, %v; want at least 3 waves and a nil error", names, canary, size, w, err)
		}
		return backing, names, w
	}
	ten := feeders(10)
	wantBacking := append(feeders(10), "SPARE")

	t.Run("append to the canary wave", func(t *testing.T) {
		backing, _, w := plan(t, 1, 3)
		w[0] = append(w[0], "hq-window-01")
		if want := ten[1:4]; !slices.Equal(w[1], want) {
			t.Errorf("after appending %q to wave 0 of Waves(feeder-1 ... feeder-10, 1, 3), wave 1 is %q, want %q", "hq-window-01", w[1], want)
		}
		if !slices.Equal(backing, wantBacking) {
			t.Errorf("after appending %q to wave 0, names and its spare slot hold %q, want %q", "hq-window-01", backing, wantBacking)
		}
	})

	t.Run("append to a wave in the middle", func(t *testing.T) {
		backing, _, w := plan(t, 0, 3)
		w[1] = append(w[1], "hq-window-01", "hq-window-02")
		if want := ten[6:9]; !slices.Equal(w[2], want) {
			t.Errorf("after appending 2 names to wave 1 of Waves(feeder-1 ... feeder-10, 0, 3), wave 2 is %q, want %q", w[2], want)
		}
		if !slices.Equal(backing, wantBacking) {
			t.Errorf("after appending 2 names to wave 1, names and its spare slot hold %q, want %q", backing, wantBacking)
		}
	})

	t.Run("append to the last wave", func(t *testing.T) {
		backing, _, w := plan(t, 1, 3)
		last := len(w) - 1
		w[last] = append(w[last], "feeder-11")
		if !slices.Equal(backing, wantBacking) {
			t.Errorf("after appending %q to the last wave of Waves(feeder-1 ... feeder-10, 1, 3), names and its spare slot hold %q, want %q", "feeder-11", backing, wantBacking)
		}
	})

	t.Run("change an element of a wave", func(t *testing.T) {
		backing, _, w := plan(t, 1, 3)
		w[1][0] = "changed"
		if !slices.Equal(backing, wantBacking) {
			t.Errorf("after wave1[0] = %q in Waves(feeder-1 ... feeder-10, 1, 3), names and its spare slot hold %q, want %q", "changed", backing, wantBacking)
		}
		want := [][]string{ten[0:1], {"changed", "feeder-3", "feeder-4"}, ten[4:7], ten[7:10]}
		if !equalWaves(w, want) {
			t.Errorf("after wave1[0] = %q, the waves are %q, want %q", "changed", w, want)
		}
	})

	t.Run("change the input after the call", func(t *testing.T) {
		_, names, w := plan(t, 1, 3)
		for i := range names {
			names[i] = "changed"
		}
		want := [][]string{ten[0:1], ten[1:4], ten[4:7], ten[7:10]}
		if !equalWaves(w, want) {
			t.Errorf("after every name in names was changed, Waves(feeder-1 ... feeder-10, 1, 3) is %q, want %q", w, want)
		}
	})
}

func ExampleWaves() {
	reserve := []string{"feeder-10", "feeder-3", "feeder-404", "feeder-9", "feeder-1", "feeder-2"}
	plan := append([]string{"hq-window-01"}, waves.Order(waves.Without(reserve, []string{"feeder-404"}))...)

	w, err := waves.Waves(plan, 1, 2)
	if err != nil {
		fmt.Println(err)
		return
	}
	for i, wave := range w {
		fmt.Println(i, wave)
	}
	// Output:
	// 0 [hq-window-01]
	// 1 [feeder-1 feeder-2]
	// 2 [feeder-3 feeder-9]
	// 3 [feeder-10]
}
