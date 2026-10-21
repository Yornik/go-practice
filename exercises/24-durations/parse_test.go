package durations_test

import (
	"errors"
	"testing"
	"time"

	durations "github.com/yornik/go-practice/exercises/24-durations"
)

const (
	day  = 24 * time.Hour
	week = 7 * day
)

// largest is the largest time.Duration (math.MaxInt64 ns) truncated to whole
// milliseconds: the largest value Parse can return.
const largest = 106751*day + 23*time.Hour + 47*time.Minute + 16*time.Second + 854*time.Millisecond

func TestParseValid(t *testing.T) {
	tests := []struct {
		in   string
		want time.Duration
	}{
		// Each unit alone.
		{"2w", 2 * week},
		{"2d", 2 * day},
		{"3h", 3 * time.Hour},
		{"90m", 90 * time.Minute},
		{"45s", 45 * time.Second},
		{"1500ms", 1500 * time.Millisecond},
		{"15d", 15 * day},

		// Combinations, largest unit first. Units may be skipped.
		{"1w2d3h", week + 2*day + 3*time.Hour},
		{"1w2d3h4m5s6ms", week + 2*day + 3*time.Hour + 4*time.Minute + 5*time.Second + 6*time.Millisecond},
		{"1h30m", 90 * time.Minute},
		{"1w30m", week + 30*time.Minute},
		{"1h30m0s", 90 * time.Minute},
		// m (minutes) comes before ms (milliseconds).
		{"1m1ms", time.Minute + time.Millisecond},
		{"2s500ms", 2500 * time.Millisecond},
		// A number may be larger than the next unit up.
		{"36h", 36 * time.Hour},
		{"1d25h", 49 * time.Hour},

		// Zero.
		{"0", 0},
		{"0s", 0},
		{"0ms", 0},
		{"0h0m", 0},
		{"1h0m", time.Hour},

		// Leading zeros.
		{"01h", time.Hour},
		{"007s", 7 * time.Second},
		{"0000000000000000000000001s", time.Second},

		// The largest value, written 3 ways.
		{"106751d23h47m16s854ms", largest},
		{"15250w1d23h47m16s854ms", largest},
		{"9223372036854ms", largest},
	}
	for _, tc := range tests {
		got, err := durations.Parse(tc.in)
		if err != nil {
			t.Errorf("Parse(%q) error = %v, want %v", tc.in, err, tc.want)
			continue
		}
		if got != tc.want {
			t.Errorf("Parse(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

type errorCase struct {
	in   string
	want error // the sentinel the *SyntaxError must wrap
	col  int   // the 1-based byte column it must report
}

// checkParseErrors checks that Parse rejects every input with a *SyntaxError
// that holds the input, the wanted sentinel and the wanted column.
func checkParseErrors(t *testing.T, cases []errorCase) {
	t.Helper()
	for _, tc := range cases {
		got, err := durations.Parse(tc.in)
		if err == nil {
			t.Errorf("Parse(%q) = %v, nil; want %v at column %d", tc.in, got, tc.want, tc.col)
			continue
		}
		var se *durations.SyntaxError
		if !errors.As(err, &se) {
			t.Errorf("Parse(%q) error = %v (type %T), want a *durations.SyntaxError", tc.in, err, err)
			continue
		}
		if !errors.Is(err, tc.want) {
			t.Errorf("Parse(%q) error = %v, want it to wrap %v", tc.in, err, tc.want)
		}
		if se.Col != tc.col {
			t.Errorf("Parse(%q) error column = %d, want %d (error: %v)", tc.in, se.Col, tc.col, err)
		}
		if se.Input != tc.in {
			t.Errorf("Parse(%q) error Input = %q, want the whole input %q", tc.in, se.Input, tc.in)
		}
		if got != 0 {
			t.Errorf("Parse(%q) = %v with an error, want 0", tc.in, got)
		}
	}
}

func TestParseErrors(t *testing.T) {
	checkParseErrors(t, []errorCase{
		// A term must start with a digit. At the end of the input the
		// column is len(input)+1.
		{"", durations.ErrNumber, 1},
		{"h", durations.ErrNumber, 1},
		{"ms", durations.ErrNumber, 1},
		{"-1h", durations.ErrNumber, 1},
		{"+1h", durations.ErrNumber, 1},
		{" 1h", durations.ErrNumber, 1},
		{"1h ", durations.ErrNumber, 3},
		{"1h\n", durations.ErrNumber, 3},
		{"1h-30m", durations.ErrNumber, 3},
		{"1h,30m", durations.ErrNumber, 3},
		{"1h 30m", durations.ErrNumber, 3},

		// A number must be followed by a letter.
		{"12", durations.ErrMissingUnit, 3},
		{"00", durations.ErrMissingUnit, 3},
		{"1h30", durations.ErrMissingUnit, 5},
		{"1h0", durations.ErrMissingUnit, 4},
		{"1.5h", durations.ErrMissingUnit, 2},
		{"1 h", durations.ErrMissingUnit, 2},
		{"1µs", durations.ErrMissingUnit, 2}, // µ is 2 bytes, neither an ASCII letter

		// The whole run of letters is the unit, and it must be 1 of the 6.
		{"2x", durations.ErrUnknownUnit, 2},
		{"2mo", durations.ErrUnknownUnit, 2},
		{"1H", durations.ErrUnknownUnit, 2},
		{"1D", durations.ErrUnknownUnit, 2},
		{"1hm", durations.ErrUnknownUnit, 2},
		{"5min", durations.ErrUnknownUnit, 2},
		{"1us", durations.ErrUnknownUnit, 2},
		{"1ns", durations.ErrUnknownUnit, 2},
		{"1y", durations.ErrUnknownUnit, 2},
		{"1h2x", durations.ErrUnknownUnit, 4},
		{"1h2x3", durations.ErrUnknownUnit, 4},

		// Units from largest to smallest, each at most once. The column is
		// the first letter of the unit that breaks the order.
		{"1h2d", durations.ErrUnitOrder, 4},
		{"1h1h", durations.ErrUnitOrder, 4},
		{"1s1m", durations.ErrUnitOrder, 4},
		{"1ms1s", durations.ErrUnitOrder, 5},
		{"1d2h3d", durations.ErrUnitOrder, 6},
		{"10m10m", durations.ErrUnitOrder, 6},

		// Left to right: the first problem wins.
		{"1h2d 3x", durations.ErrUnitOrder, 4},
		{"1x2d", durations.ErrUnknownUnit, 2},
	})
}

func TestParseOverflow(t *testing.T) {
	checkParseErrors(t, []errorCase{
		// 1 term that does not fit. The column is its first digit.
		{"10000000000000000000s", durations.ErrOverflow, 1}, // 20 digits
		{"99999999999999999999999ms", durations.ErrOverflow, 1},
		{"106752d", durations.ErrOverflow, 1},
		{"15251w", durations.ErrOverflow, 1},
		{"2562048h", durations.ErrOverflow, 1},
		{"9223372037s", durations.ErrOverflow, 1},
		{"9223372036855ms", durations.ErrOverflow, 1},
		{"1h99999999999999999999s", durations.ErrOverflow, 3},

		// Every term fits, but the sum crosses the limit in its last term.
		{"106751d23h47m16s855ms", durations.ErrOverflow, 17},
		{"1w106751d", durations.ErrOverflow, 3},
		{"106751d24h", durations.ErrOverflow, 8},
		{"15250w2d", durations.ErrOverflow, 7},

		// The range is checked last: a term with no unit or an unknown unit
		// reports that, however large its number is.
		{"99999999999999999999", durations.ErrMissingUnit, 21},
		{"99999999999999999999x", durations.ErrUnknownUnit, 21},
		{"1s99999999999999999999h", durations.ErrUnitOrder, 23},
	})
}
