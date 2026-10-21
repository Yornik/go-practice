package nines_test

import (
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"

	nines "github.com/yornik/go-practice/exercises/13-nines"
)

// base is minute 0 of the Downtime tests. Most cases use the window
// [m(0), m(60)): 1 hour.
var base = time.Date(2026, 10, 7, 3, 0, 0, 0, time.UTC)

// m returns the time n minutes after base.
func m(n int) time.Time { return base.Add(time.Duration(n) * time.Minute) }

// span returns the outage [m(start), m(end)).
func span(start, end int) nines.Outage {
	return nines.Outage{Start: m(start), End: m(end)}
}

// show writes outages as minutes after base, for example "[10,20) [15,30)".
func show(outages []nines.Outage) string {
	if outages == nil {
		return "nil"
	}
	parts := []string{}
	for _, o := range outages {
		parts = append(parts, fmt.Sprintf("[%v,%v)", o.Start.Sub(base).Minutes(), o.End.Sub(base).Minutes()))
	}
	return "{" + strings.Join(parts, " ") + "}"
}

// Step 3: Downtime.

func TestDowntime(t *testing.T) {
	cases := []struct {
		name     string
		outages  []nines.Outage
		from, to int
		want     time.Duration
	}{
		{"nil slice", nil, 0, 60, 0},
		{"empty slice", []nines.Outage{}, 0, 60, 0},
		{"1 outage", []nines.Outage{span(10, 20)}, 0, 60, 10 * time.Minute},
		{"disjoint", []nines.Outage{span(10, 20), span(30, 35)}, 0, 60, 15 * time.Minute},
		{"overlapping", []nines.Outage{span(10, 20), span(15, 30)}, 0, 60, 20 * time.Minute},
		{"the same outage twice", []nines.Outage{span(10, 20), span(10, 20)}, 0, 60, 10 * time.Minute},
		{"nested", []nines.Outage{span(10, 40), span(15, 20)}, 0, 60, 30 * time.Minute},
		{"nested, then 1 more inside", []nines.Outage{span(10, 40), span(15, 20), span(30, 35)}, 0, 60, 30 * time.Minute},
		{"a chain of overlaps", []nines.Outage{span(10, 20), span(18, 25), span(24, 30)}, 0, 60, 20 * time.Minute},
		{"touching", []nines.Outage{span(10, 20), span(20, 30)}, 0, 60, 20 * time.Minute},
		{"clipped at the start", []nines.Outage{span(-10, 5)}, 0, 60, 5 * time.Minute},
		{"clipped at the end", []nines.Outage{span(55, 70)}, 0, 60, 5 * time.Minute},
		{"clipped at both ends", []nines.Outage{span(-10, 5), span(55, 70)}, 0, 60, 10 * time.Minute},
		{"covers the whole window", []nines.Outage{span(-10, 70)}, 0, 60, 60 * time.Minute},
		{"entirely before", []nines.Outage{span(-30, -10)}, 0, 60, 0},
		{"entirely after", []nines.Outage{span(70, 90)}, 0, 60, 0},
		{"ends exactly at from", []nines.Outage{span(-10, 0)}, 0, 60, 0},
		{"starts exactly at to", []nines.Outage{span(60, 90)}, 0, 60, 0},
		{"unsorted", []nines.Outage{span(30, 35), span(10, 20), span(15, 25)}, 0, 60, 20 * time.Minute},
		{"unsorted, overlapping across the window start", []nines.Outage{span(50, 55), span(-20, 10), span(5, 15)}, 0, 60, 20 * time.Minute},
		{"End equal to Start", []nines.Outage{span(20, 20)}, 0, 60, 0},
		{"End before Start", []nines.Outage{span(30, 25)}, 0, 60, 0},
		{"empty outages next to a real one", []nines.Outage{span(30, 25), span(10, 15), span(40, 40)}, 0, 60, 5 * time.Minute},
		{"to before from", []nines.Outage{span(10, 20)}, 60, 0, 0},
		{"to equal to from", []nines.Outage{span(10, 20)}, 30, 30, 0},
		{"a smaller window", []nines.Outage{span(10, 20), span(30, 50)}, 15, 35, 10 * time.Minute},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := nines.Downtime(c.outages, m(c.from), m(c.to))
			if got != c.want {
				t.Errorf("Downtime(%s, from m%d, to m%d) = %v, want %v (outages in minutes after %s)",
					show(c.outages), c.from, c.to, got, c.want, base.Format(time.RFC3339))
			}
		})
	}
}

func TestDowntimeSeconds(t *testing.T) {
	outages := []nines.Outage{
		{Start: base.Add(12*time.Minute + 30*time.Second), End: base.Add(13*time.Minute + 15*time.Second)},
		{Start: base.Add(13*time.Minute + 10*time.Second), End: base.Add(13*time.Minute + 20*time.Second)},
	}
	want := 50 * time.Second
	if got := nines.Downtime(outages, m(0), m(60)); got != want {
		t.Errorf("Downtime(03:12:30-03:13:15 and 03:13:10-03:13:20, window 03:00-04:00) = %v, want %v", got, want)
	}
}

func TestDowntimeKeepsInput(t *testing.T) {
	// Unsorted, overlapping, clipped by the window and 1 with End before
	// Start: every case in which a solution might change the input.
	outages := []nines.Outage{span(50, 70), span(-20, 10), span(5, 15), span(30, 25)}
	before := slices.Clone(outages)
	nines.Downtime(outages, m(0), m(60))
	if !slices.Equal(outages, before) {
		t.Errorf("Downtime changed its input: before %s, after %s", show(before), show(outages))
	}
}

// Step 4: Availability.

func TestAvailability(t *testing.T) {
	cases := []struct {
		down, window time.Duration
		want         float64
	}{
		{30 * time.Minute, time.Hour, 0.5},
		{time.Hour, time.Hour, 0},
		{0, time.Hour, 1},
		{15 * time.Minute, time.Hour, 0.75},
		// 27 minutes of the 44 640 minutes in October
		{27 * time.Minute, 31 * day, 1 - 27.0/44640},
		{43*time.Minute + 12*time.Second, 30 * day, 0.999},
	}
	for _, c := range cases {
		got := nines.Availability(c.down, c.window)
		if !near(got, c.want) {
			t.Errorf("Availability(%v, %v) = %v, want %v", c.down, c.window, got, c.want)
		}
	}
}

func TestAvailabilityNaN(t *testing.T) {
	cases := []struct {
		down, window time.Duration
	}{
		{0, 0},
		{5 * time.Minute, 0},
		{0, -time.Hour},
		{5 * time.Minute, -time.Hour},
	}
	for _, c := range cases {
		if got := nines.Availability(c.down, c.window); !math.IsNaN(got) {
			t.Errorf("Availability(%v, %v) = %v, want NaN", c.down, c.window, got)
		}
	}
}
