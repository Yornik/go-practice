package deploynote_test

import (
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	deploynote "github.com/yornik/go-practice/exercises/08-deploynote"
)

// cet is the team's zone in November: UTC+1. A fixed zone keeps the tests
// independent of the machine's time zone database.
var cet = time.FixedZone("CET", 60*60)

// The test week: Thursday 5 November 2026 to Tuesday 10 November 2026.
// day(6) is Friday 6 November.
func day(d, hour, minute, sec, nsec int, loc *time.Location) time.Time {
	return time.Date(2026, time.November, d, hour, minute, sec, nsec, loc)
}

// tuesday10 is a time outside the freeze. validNote uses it.
var tuesday10 = day(10, 10, 0, 0, 0, cet)

// validNote returns a note without problems. Tests change 1 or more fields.
func validNote() deploynote.Note {
	return deploynote.Note{
		Service:  "species-labeller",
		Version:  "v1.5.0",
		Ticket:   "PECK-412",
		Author:   "bram",
		Rollback: "helm rollback species-labeller 41",
		At:       tuesday10,
	}
}

// describe writes a call of Check for failure messages. The time shows the
// weekday and the zone, because the freeze depends on both.
func describe(n deploynote.Note) string {
	at := "time.Time{} (zero)"
	if !n.At.IsZero() {
		at = n.At.Format("Monday 2006-01-02 15:04:05.999999999 MST -07:00")
	}
	return fmt.Sprintf("Check(Note{Service: %q, Version: %q, Ticket: %q, Author: %q, Rollback: %q, At: %s})",
		n.Service, n.Version, n.Ticket, n.Author, n.Rollback, at)
}

// problem is 1 FieldError that Check must report.
type problem struct {
	field string
	err   error // the sentinel: ErrMissing, ErrBadFormat or ErrFreeze
}

var sentinels = []error{deploynote.ErrMissing, deploynote.ErrBadFormat, deploynote.ErrFreeze}

// fieldErrors returns the *FieldError values inside the error that Check
// returned. It fails the test if err is not the result of errors.Join, or if
// the join holds anything but non-nil *FieldError values.
func fieldErrors(t *testing.T, call string, err error) []*deploynote.FieldError {
	t.Helper()
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		t.Fatalf("%s\n = %v (type %T)\nwant the result of errors.Join, which has a method Unwrap() []error", call, err, err)
	}
	var out []*deploynote.FieldError
	for i, e := range joined.Unwrap() {
		fe, ok := e.(*deploynote.FieldError)
		if !ok || fe == nil {
			t.Fatalf("%s\nelement %d of the join is %v (type %T), want a non-nil *deploynote.FieldError", call, i, e, e)
		}
		out = append(out, fe)
	}
	return out
}

// checkProblems calls Check(n) and compares the result with want, in order.
// An empty want means that Check must return nil.
func checkProblems(t *testing.T, n deploynote.Note, want []problem) {
	t.Helper()
	call := describe(n)
	err := deploynote.Check(n)
	if len(want) == 0 {
		if err != nil {
			// %T makes a typed nil visible: a nil *FieldError inside an
			// error prints as <nil> but is not equal to nil.
			t.Errorf("%s\n = %v (type %T), want nil", call, err, err)
		}
		return
	}
	if err == nil {
		t.Errorf("%s\n = nil, want problems with %v", call, fieldNames(want))
		return
	}
	fes := fieldErrors(t, call, err)

	var gotFields []string
	for _, fe := range fes {
		gotFields = append(gotFields, fe.Field)
	}
	if !slices.Equal(gotFields, fieldNames(want)) {
		t.Errorf("%s\nreturned problems with the fields %q\nwant %q\nerror: %v", call, gotFields, fieldNames(want), err)
		return
	}
	for i, fe := range fes {
		for _, s := range sentinels {
			got := errors.Is(fe, s)
			if wanted := s == want[i].err; got != wanted {
				t.Errorf("%s\nFieldError %q: errors.Is(fe, %v) = %t, want %t (fe.Err = %v)", call, fe.Field, s, got, wanted, fe.Err)
			}
		}
	}
	// A caller works with the joined error, not with the list.
	for _, p := range want {
		if !errors.Is(err, p.err) {
			t.Errorf("%s\nerrors.Is(err, %v) = false, want true", call, p.err)
		}
	}
	var fe *deploynote.FieldError
	if !errors.As(err, &fe) {
		t.Errorf("%s\nerrors.As(err, &fe) = false, want true", call)
	} else if fe.Field != want[0].field {
		t.Errorf("%s\nerrors.As(err, &fe) found the field %q, want the first problem %q", call, fe.Field, want[0].field)
	}
}

func fieldNames(ps []problem) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.field)
	}
	return out
}
