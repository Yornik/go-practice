package durations_test

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	durations "github.com/yornik/go-practice/exercises/24-durations"
)

func TestFormatExamples(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{time.Millisecond, "1ms"},
		{time.Second, "1s"},
		{time.Minute, "1m"},
		{time.Hour, "1h"},
		{day, "1d"},
		{week, "1w"},
		{90 * time.Minute, "1h30m"},
		{36 * time.Hour, "1d12h"},
		{61 * time.Second, "1m1s"},
		{2500 * time.Millisecond, "2s500ms"},
		{9*day + 3*time.Hour, "1w2d3h"},
		{week + 2*day + 3*time.Hour + 4*time.Minute + 5*time.Second + 6*time.Millisecond, "1w2d3h4m5s6ms"},
		{3*week + 5*time.Second, "3w5s"},
		{time.Hour + time.Millisecond, "1h1ms"},
		{math.MaxInt64, "15250w1d23h47m16s854ms"},
	}
	for _, tc := range tests {
		if got := durations.Format(tc.in); got != tc.want {
			t.Errorf("Format(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatZeroAndTruncation(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{0, "0s"},
		{time.Nanosecond, "0s"},
		{999 * time.Microsecond, "0s"},
		{1500 * time.Microsecond, "1ms"},
		{time.Second + 999999*time.Nanosecond, "1s"},
		{59*time.Second + 999*time.Millisecond + 999999*time.Nanosecond, "59s999ms"},
	}
	for _, tc := range tests {
		if got := durations.Format(tc.in); got != tc.want {
			t.Errorf("Format(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatNegative(t *testing.T) {
	tests := []struct {
		in   time.Duration
		want string
	}{
		{-time.Millisecond, "-1ms"},
		{-90 * time.Minute, "-1h30m"},
		{-36 * time.Hour, "-1d12h"},
		// Truncation goes toward zero, so -1.5 ms is -1 ms.
		{-1500 * time.Microsecond, "-1ms"},
		// Above -1 ms nothing is left after truncation: no minus sign.
		{-999 * time.Microsecond, "0s"},
		{-time.Nanosecond, "0s"},
		{-math.MaxInt64, "-15250w1d23h47m16s854ms"},
	}
	for _, tc := range tests {
		if got := durations.Format(tc.in); got != tc.want {
			t.Errorf("Format(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestFormatRoundTrip checks Parse(Format(d)) == d.Truncate(time.Millisecond)
// on 1000 values from a fixed seed, so every run uses the same values.
func TestFormatRoundTrip(t *testing.T) {
	r := rand.New(rand.NewPCG(24, 2026))
	values := []time.Duration{0, time.Millisecond, math.MaxInt64}
	for i := range 1000 {
		// Mix the scales, so that small units and large units both show up.
		var limit int64
		switch i % 4 {
		case 0:
			limit = math.MaxInt64
		case 1:
			limit = int64(4 * week)
		case 2:
			limit = int64(2 * time.Hour)
		case 3:
			limit = int64(10 * time.Second)
		}
		values = append(values, time.Duration(r.Int64N(limit)))
	}

	failures := 0
	for _, d := range values {
		s := durations.Format(d)
		back, err := durations.Parse(s)
		want := d.Truncate(time.Millisecond)
		if err != nil || back != want {
			t.Errorf("d = %d ns: Format(d) = %q, Parse(%q) = %v, %v; want %v, nil", int64(d), s, s, back, err, want)
			failures++
			if failures == 5 {
				t.Fatal("stopping after 5 failures")
			}
		}
	}
}

var sentinels = []error{
	durations.ErrNumber,
	durations.ErrMissingUnit,
	durations.ErrUnknownUnit,
	durations.ErrUnitOrder,
	durations.ErrOverflow,
}

// FuzzParse checks properties that must hold for ANY input, not only for the
// table cases. Plain `go test` runs it on the seeds below. Run
//
//	go test -fuzz=FuzzParse ./exercises/24-durations/
//
// to let Go generate new inputs (stop it with Ctrl-C).
func FuzzParse(f *testing.F) {
	seeds := []string{
		"", "0", "00", "2d", "1w2d3h", "90m", "1500ms", "1m1ms", "01h",
		"1.5h", "12", "2x", "2mo", "1H", "1h2d", "1h1h", "1h ", "-1h", "1µs",
		"106751d23h47m16s854ms", "106752d", "106751d23h47m16s855ms",
		"99999999999999999999x", "10000000000000000000s",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		d, err := durations.Parse(s)
		if err != nil {
			var se *durations.SyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("Parse(%q) error = %v (type %T), want a *durations.SyntaxError", s, err, err)
			}
			if se.Col < 1 || se.Col > len(s)+1 {
				t.Fatalf("Parse(%q) error column = %d, want 1..%d", s, se.Col, len(s)+1)
			}
			if se.Input != s {
				t.Fatalf("Parse(%q) error Input = %q, want the whole input", s, se.Input)
			}
			if !slices.Contains(sentinels, se.Err) {
				t.Fatalf("Parse(%q) error Err = %v, want 1 of the 5 sentinel errors", s, se.Err)
			}
			if d != 0 {
				t.Fatalf("Parse(%q) = %v with an error, want 0", s, d)
			}
			return
		}
		if d < 0 {
			t.Fatalf("Parse(%q) = %v, want a duration >= 0", s, d)
		}
		// Every unit is a whole number of milliseconds, so Format loses
		// nothing and the round trip must give d back exactly.
		out := durations.Format(d)
		back, err := durations.Parse(out)
		if err != nil || back != d {
			t.Fatalf("Parse(%q) = %v, Format gives %q, and Parse(%q) = %v, %v; want %v, nil", s, d, out, out, back, err, d)
		}
	})
}

func ExampleParse() {
	d, err := durations.Parse("1w2d3h")
	fmt.Println(d, err)

	_, err = durations.Parse("1h2d")
	fmt.Println(err)
	var se *durations.SyntaxError
	if errors.As(err, &se) {
		fmt.Println(se.Input)
		fmt.Println(strings.Repeat(" ", se.Col-1) + "^")
	}
	// Output:
	// 219h0m0s <nil>
	// duration "1h2d": column 4: units out of order
	// 1h2d
	//    ^
}

func ExampleFormat() {
	fmt.Println(durations.Format(90 * time.Minute))
	fmt.Println(durations.Format(219 * time.Hour))
	fmt.Println(durations.Format(1500 * time.Microsecond))
	fmt.Println(durations.Format(0))
	// Output:
	// 1h30m
	// 1w2d3h
	// 1ms
	// 0s
}
