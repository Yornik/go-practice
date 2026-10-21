package nines_test

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	nines "github.com/yornik/go-practice/exercises/13-nines"
)

// tolerance is how far a float result may be from the exact answer. Float
// rounding in your formula must not decide whether a test passes.
const tolerance = 1e-9

func near(got, want float64) bool {
	return math.Abs(got-want) <= tolerance
}

const day = 24 * time.Hour

// Step 1: ParseObjective.

func TestParseObjectiveValid(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"99.95%", 0.9995},
		{"99.95", 0.9995},
		{"99.9", 0.999},
		{" 99.5% ", 0.995},
		{"\t99.99%\n", 0.9999},
		{"9.995e1", 0.9995},
		{"50%", 0.5},
		{"0.5", 0.005},
		{"99.999%", 0.99999},
	}
	for _, c := range cases {
		got, err := nines.ParseObjective(c.in)
		if err != nil {
			t.Errorf("ParseObjective(%q) error = %v, want %v", c.in, err, c.want)
			continue
		}
		if !near(got, c.want) {
			t.Errorf("ParseObjective(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseObjectiveInvalid(t *testing.T) {
	cases := []struct {
		in, why string
	}{
		{"99.5 %", "a space before the %"},
		{"99.5%%", "2 % signs"},
		{"%99.5", "% before the number"},
		{"%", "no number"},
		{"100", "not below 100"},
		{"100%", "not below 100"},
		{"0", "not above 0"},
		{"0%", "not above 0"},
		{"-1", "not above 0"},
		{"NaN", "not a number"},
		{"nan", "not a number"},
		{"NaN%", "not a number"},
		{"Inf", "infinite"},
		{"+Inf", "infinite"},
		{"-Inf", "infinite"},
		{"infinity", "infinite"},
		{"abc", "not a number"},
		{"", "empty"},
		{"   ", "only whitespace"},
	}
	for _, c := range cases {
		got, err := nines.ParseObjective(c.in)
		if err == nil {
			t.Errorf("ParseObjective(%q) = %v, nil; want an error (%s)", c.in, got, c.why)
			continue
		}
		if got != 0 {
			t.Errorf("ParseObjective(%q) = %v with error %q, want 0 with the error", c.in, got, err)
		}
		if !errors.Is(err, nines.ErrInvalid) {
			t.Errorf("ParseObjective(%q) error %q does not wrap ErrInvalid", c.in, err)
		}
		if quoted := fmt.Sprintf("%q", c.in); !strings.Contains(err.Error(), quoted) {
			t.Errorf("ParseObjective(%q) error %q does not contain the input %s", c.in, err, quoted)
		}
	}
}

// Step 2: Budget.

func TestBudget(t *testing.T) {
	cases := []struct {
		objective float64
		window    time.Duration
		want      time.Duration
	}{
		// 43.2 minutes
		{0.999, 30 * day, 43*time.Minute + 12*time.Second},
		// 21.6 minutes
		{0.9995, 30 * day, 21*time.Minute + 36*time.Second},
		// 22.32 minutes is 22m19.2s: rounds down
		{0.9995, 31 * day, 22*time.Minute + 19*time.Second},
		// 52.56 minutes is 52m33.6s: rounds up
		{0.9999, 365 * day, 52*time.Minute + 34*time.Second},
		// 10.08 minutes is 10m4.8s: rounds up
		{0.999, 7 * day, 10*time.Minute + 5*time.Second},
		{0.99, 24 * time.Hour, 14*time.Minute + 24*time.Second},
		{0.5, time.Hour, 30 * time.Minute},
		{0.9995, 0, 0},
	}
	for _, c := range cases {
		if got := nines.Budget(c.objective, c.window); got != c.want {
			t.Errorf("Budget(%v, %v) = %v, want %v", c.objective, c.window, got, c.want)
		}
	}
}
