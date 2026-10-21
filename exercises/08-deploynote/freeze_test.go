package deploynote_test

import (
	"testing"
	"time"

	deploynote "github.com/yornik/go-practice/exercises/08-deploynote"
)

func TestCheckFreezeWeek(t *testing.T) {
	const ns = 999_999_999 // the last nanosecond of a second
	tests := []struct {
		at     time.Time
		frozen bool
	}{
		{day(5, 16, 0, 0, 0, cet), false},    // Thursday
		{day(5, 23, 59, 59, ns, cet), false}, // Thursday, last nanosecond
		{day(6, 0, 0, 0, 0, cet), false},     // Friday
		{day(6, 9, 0, 0, 0, cet), false},
		{day(6, 14, 59, 59, 0, cet), false},
		{day(6, 14, 59, 59, ns, cet), false},
		{day(6, 15, 0, 0, 0, cet), true}, // the freeze starts
		{day(6, 15, 30, 0, 0, cet), true},
		{day(6, 17, 40, 0, 0, cet), true},
		{day(6, 23, 59, 59, ns, cet), true},
		{day(7, 0, 0, 0, 0, cet), true}, // Saturday
		{day(7, 9, 0, 0, 0, cet), true},
		{day(7, 12, 0, 0, 0, cet), true},
		{day(7, 23, 59, 59, 0, cet), true},
		{day(8, 0, 0, 0, 0, cet), true}, // Sunday
		{day(8, 9, 0, 0, 0, cet), true},
		{day(8, 15, 0, 0, 0, cet), true},
		{day(8, 23, 59, 59, ns, cet), true},
		{day(9, 0, 0, 0, 0, cet), true}, // Monday
		{day(9, 8, 59, 59, 0, cet), true},
		{day(9, 8, 59, 59, ns, cet), true},
		{day(9, 9, 0, 0, 0, cet), false}, // the freeze ends
		{day(9, 9, 0, 0, 1, cet), false},
		{day(9, 15, 30, 0, 0, cet), false}, // 15:00 matters on a Friday only
		{day(10, 3, 0, 0, 0, cet), false},  // Tuesday
		{day(11, 15, 0, 0, 0, cet), false}, // Wednesday
		{day(13, 16, 0, 0, 0, cet), true},  // the next Friday
	}
	for _, tc := range tests {
		n := validNote()
		n.At = tc.at
		var want []problem
		if tc.frozen {
			want = []problem{{"At", deploynote.ErrFreeze}}
		}
		checkProblems(t, n, want)
	}
}

func TestCheckFreezeZone(t *testing.T) {
	// The freeze uses the weekday and the hour of At in At's own location.
	// The same instant can be inside the freeze in 1 zone and outside it in
	// another.
	tests := []struct {
		name   string
		at     time.Time
		frozen bool
	}{
		{"Friday 15:30 CET", day(6, 15, 30, 0, 0, cet), true},
		{"the same instant in UTC: Friday 14:30", day(6, 15, 30, 0, 0, cet).In(time.UTC), false},
		{"Monday 08:30 UTC", day(9, 8, 30, 0, 0, time.UTC), true},
		{"the same instant in CET: Monday 09:30", day(9, 8, 30, 0, 0, time.UTC).In(cet), false},
		{"Friday 23:30 UTC", day(6, 23, 30, 0, 0, time.UTC), true},
		{"the same instant in UTC+14: Saturday 13:30", day(6, 23, 30, 0, 0, time.UTC).In(time.FixedZone("LINT", 14*60*60)), true},
		{"Saturday 01:00 CET", day(7, 1, 0, 0, 0, cet), true},
		{"the same instant in UTC-10: Friday 14:00", day(7, 1, 0, 0, 0, cet).In(time.FixedZone("HST", -10*60*60)), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := validNote()
			n.At = tc.at
			var want []problem
			if tc.frozen {
				want = []problem{{"At", deploynote.ErrFreeze}}
			}
			checkProblems(t, n, want)
		})
	}
}

func TestCheckFreezeIncident(t *testing.T) {
	saturday := day(7, 12, 0, 0, 0, cet)
	friday := day(6, 17, 40, 0, 0, cet)
	monday := day(9, 8, 59, 59, 0, cet)
	tests := []struct {
		name   string
		ticket string
		at     time.Time
		want   []problem
	}{
		{"INC lifts the freeze on Saturday", "INC-45", saturday, nil},
		{"INC lifts the freeze on Friday evening", "INC-1", friday, nil},
		{"INC lifts the freeze on Monday morning", "INC-999999", monday, nil},
		{"INC outside the freeze", "INC-45", tuesday10, nil},
		{"VL does not lift the freeze", "VL-412", saturday, []problem{{"At", deploynote.ErrFreeze}}},
		{"VL on Friday evening", "VL-412", friday, []problem{{"At", deploynote.ErrFreeze}}},
		// Only a valid INC- ticket lifts the freeze.
		{"INC- without digits", "INC-", saturday, []problem{{"Ticket", deploynote.ErrBadFormat}, {"At", deploynote.ErrFreeze}}},
		{"lowercase inc-45", "inc-45", saturday, []problem{{"Ticket", deploynote.ErrBadFormat}, {"At", deploynote.ErrFreeze}}},
		{"INC with 7 digits", "INC-1234567", saturday, []problem{{"Ticket", deploynote.ErrBadFormat}, {"At", deploynote.ErrFreeze}}},
		{"INC with a space", " INC-45", saturday, []problem{{"Ticket", deploynote.ErrBadFormat}, {"At", deploynote.ErrFreeze}}},
		{"no ticket", "", saturday, []problem{{"Ticket", deploynote.ErrMissing}, {"At", deploynote.ErrFreeze}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := validNote()
			n.Ticket = tc.ticket
			n.At = tc.at
			checkProblems(t, n, tc.want)
		})
	}
}
