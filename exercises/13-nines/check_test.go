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

// at returns hh:mm UTC on the given day of 2026.
func at(month time.Month, d, hh, mm int) time.Time {
	return time.Date(2026, month, d, hh, mm, 0, 0, time.UTC)
}

// october holds the reports of 2 monitors for October 2026. Both saw the same
// 2 outages. The reports add up to 19+4+19+5 = 47 minutes; the time in
// which the site was down is 21+6 = 27 minutes.
var october = []nines.Outage{
	// The external uptime check.
	{Start: at(10, 7, 3, 12), End: at(10, 7, 3, 31)},
	{Start: at(10, 21, 14, 2), End: at(10, 21, 14, 6)},
	// The load balancer's health check.
	{Start: at(10, 7, 3, 14), End: at(10, 7, 3, 33)},
	{Start: at(10, 21, 14, 3), End: at(10, 21, 14, 8)},
}

// checkReport compares every field of got with want. Availability is
// compared with the tolerance.
func checkReport(t *testing.T, call string, got, want nines.Report) {
	t.Helper()
	if got.Window != want.Window {
		t.Errorf("%s: Window = %v, want %v", call, got.Window, want.Window)
	}
	if got.Downtime != want.Downtime {
		t.Errorf("%s: Downtime = %v, want %v", call, got.Downtime, want.Downtime)
	}
	if !near(got.Availability, want.Availability) {
		t.Errorf("%s: Availability = %v, want %v", call, got.Availability, want.Availability)
	}
	if got.Budget != want.Budget {
		t.Errorf("%s: Budget = %v, want %v", call, got.Budget, want.Budget)
	}
	if got.Remaining != want.Remaining {
		t.Errorf("%s: Remaining = %v, want %v", call, got.Remaining, want.Remaining)
	}
	if got.Met != want.Met {
		t.Errorf("%s: Met = %v, want %v", call, got.Met, want.Met)
	}
}

// Step 5: Check.

func TestCheckOctober(t *testing.T) {
	call := "Check(0.9995, 1 Oct, 1 Nov, 2 monitors' reports)"
	got, err := nines.Check(0.9995, at(10, 1, 0, 0), at(11, 1, 0, 0), october)
	if err != nil {
		t.Fatalf("%s error = %v, want nil", call, err)
	}
	checkReport(t, call, got, nines.Report{
		Window:       31 * day,
		Downtime:     27 * time.Minute,
		Availability: 1 - 27.0/44640,
		Budget:       22*time.Minute + 19*time.Second,
		Remaining:    -(4*time.Minute + 41*time.Second),
		Met:          false,
	})
}

func TestCheckMet(t *testing.T) {
	september := []nines.Outage{
		// Started in August: only the 3 minutes in September count.
		{Start: at(8, 31, 23, 55), End: at(9, 1, 0, 3)},
		// 2 reports of 1 outage from 10:00 to 10:12.
		{Start: at(9, 3, 10, 0), End: at(9, 3, 10, 7)},
		{Start: at(9, 3, 10, 5), End: at(9, 3, 10, 12)},
		// A report of 0 seconds is not an error.
		{Start: at(9, 20, 8, 0), End: at(9, 20, 8, 0)},
	}
	call := "Check(0.9995, 1 Sep, 1 Oct, outages)"
	got, err := nines.Check(0.9995, at(9, 1, 0, 0), at(10, 1, 0, 0), september)
	if err != nil {
		t.Fatalf("%s error = %v, want nil", call, err)
	}
	checkReport(t, call, got, nines.Report{
		Window:       30 * day,
		Downtime:     15 * time.Minute,
		Availability: 1 - 15.0/43200,
		Budget:       21*time.Minute + 36*time.Second,
		Remaining:    6*time.Minute + 36*time.Second,
		Met:          true,
	})
}

func TestCheckBudgetUsedExactly(t *testing.T) {
	start := at(9, 10, 2, 0)
	outages := []nines.Outage{{Start: start, End: start.Add(43*time.Minute + 12*time.Second)}}
	call := "Check(0.999, 1 Sep, 1 Oct, 1 outage of 43m12s)"
	got, err := nines.Check(0.999, at(9, 1, 0, 0), at(10, 1, 0, 0), outages)
	if err != nil {
		t.Fatalf("%s error = %v, want nil", call, err)
	}
	checkReport(t, call, got, nines.Report{
		Window:       30 * day,
		Downtime:     43*time.Minute + 12*time.Second,
		Availability: 0.999,
		Budget:       43*time.Minute + 12*time.Second,
		Remaining:    0,
		Met:          true,
	})
}

func TestCheckErrors(t *testing.T) {
	from, to := at(10, 1, 0, 0), at(11, 1, 0, 0)
	backwards := []nines.Outage{
		{Start: at(10, 7, 3, 12), End: at(10, 7, 3, 31)},
		{Start: at(10, 21, 14, 6), End: at(10, 21, 14, 2)},
	}
	cases := []struct {
		name      string
		objective float64
		from, to  time.Time
		outages   []nines.Outage
		contains  string // a text the message must contain; "" checks nothing
	}{
		{"objective NaN", math.NaN(), from, to, october, ""},
		{"objective 0", 0, from, to, october, ""},
		{"objective 1", 1, from, to, october, ""},
		{"objective negative", -0.5, from, to, october, ""},
		{"objective a percentage, not a fraction", 99.95, from, to, october, ""},
		{"objective +Inf", math.Inf(1), from, to, october, ""},
		{"to equal to from", 0.9995, from, from, october, ""},
		{"to before from", 0.9995, to, from, october, ""},
		{"outage 1 ends before it starts", 0.9995, from, to, backwards, "outage 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := nines.Check(c.objective, c.from, c.to, c.outages)
			call := fmt.Sprintf("Check(%v, %s, %s, ...)", c.objective,
				c.from.Format(time.DateOnly), c.to.Format(time.DateOnly))
			if err == nil {
				t.Fatalf("%s = %+v, nil; want an error", call, got)
			}
			if !errors.Is(err, nines.ErrInvalid) {
				t.Errorf("%s error %q does not wrap ErrInvalid", call, err)
			}
			if got != (nines.Report{}) {
				t.Errorf("%s = %+v with an error, want the zero Report", call, got)
			}
			if c.contains != "" && !strings.Contains(err.Error(), c.contains) {
				t.Errorf("%s error %q does not contain %q", call, err, c.contains)
			}
		})
	}
}

func ExampleCheck() {
	objective, err := nines.ParseObjective("99.95%")
	if err != nil {
		fmt.Println(err)
		return
	}
	from := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	to := from.AddDate(0, 1, 0)
	r, err := nines.Check(objective, from, to, october)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("window:      ", r.Window)
	fmt.Println("budget:      ", r.Budget)
	fmt.Println("downtime:    ", r.Downtime)
	fmt.Println("remaining:   ", r.Remaining)
	fmt.Printf("availability: %.3f%%\n", r.Availability*100)
	fmt.Println("met:         ", r.Met)
	// Output:
	// window:       744h0m0s
	// budget:       22m19s
	// downtime:     27m0s
	// remaining:    -4m41s
	// availability: 99.940%
	// met:          false
}
