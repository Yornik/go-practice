package deploynote_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	deploynote "github.com/yornik/go-practice/exercises/08-deploynote"
)

func TestCheckAllFields(t *testing.T) {
	saturday := day(7, 12, 0, 0, 0, cet)
	tests := []struct {
		name string
		note deploynote.Note
		want []problem
	}{
		{
			"every field bad",
			deploynote.Note{Service: "", Version: " ", Ticket: "PECK123", Author: "\t", Rollback: "n/a", At: saturday},
			[]problem{
				{"Service", deploynote.ErrMissing},
				{"Version", deploynote.ErrMissing},
				{"Ticket", deploynote.ErrBadFormat},
				{"Author", deploynote.ErrMissing},
				{"Rollback", deploynote.ErrMissing},
				{"At", deploynote.ErrFreeze},
			},
		},
		{
			"the zero Note",
			deploynote.Note{},
			[]problem{
				{"Service", deploynote.ErrMissing},
				{"Version", deploynote.ErrMissing},
				{"Ticket", deploynote.ErrMissing},
				{"Author", deploynote.ErrMissing},
				{"Rollback", deploynote.ErrMissing},
				{"At", deploynote.ErrMissing},
			},
		},
		{
			"the note of the incident",
			deploynote.Note{Service: "species-labeller", Version: "v1.5.0", Ticket: "small fix", Author: "bram", At: day(6, 17, 40, 0, 0, cet)},
			[]problem{
				{"Ticket", deploynote.ErrBadFormat},
				{"Rollback", deploynote.ErrMissing},
				{"At", deploynote.ErrFreeze},
			},
		},
		{
			"Author and Rollback",
			deploynote.Note{Service: "push", Version: "v3.0.1", Ticket: "PECK-77", Rollback: "none", At: tuesday10},
			[]problem{
				{"Author", deploynote.ErrMissing},
				{"Rollback", deploynote.ErrMissing},
			},
		},
		{
			"Service and At",
			deploynote.Note{Version: "v3.0.1", Ticket: "PECK-77", Author: "ines", Rollback: "redeploy v3.0.0"},
			[]problem{
				{"Service", deploynote.ErrMissing},
				{"At", deploynote.ErrMissing},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			checkProblems(t, tc.note, tc.want)
		})
	}
}

func TestCheckAllMessage(t *testing.T) {
	n := deploynote.Note{Service: "species-labeller", Version: "v1.5.0", Ticket: "small fix", Author: "bram", At: day(6, 17, 40, 0, 0, cet)}
	err := deploynote.Check(n)
	if err == nil {
		t.Fatalf("%s\n = nil, want 3 problems", describe(n))
	}
	// errors.Join puts a newline between the messages: 1 line per problem.
	want := strings.Join([]string{
		`Ticket: bad format: "small fix"`,
		"Rollback: missing",
		"At: change freeze",
	}, "\n")
	if got := err.Error(); got != want {
		t.Errorf("%s.Error()\n = %q\nwant %q", describe(n), got, want)
	}
}

func TestCheckAllAs(t *testing.T) {
	n := deploynote.Note{Service: "push", Version: "", Ticket: "PECK-77", Author: "", Rollback: "-", At: tuesday10}
	err := deploynote.Check(n)
	if err == nil {
		t.Fatalf("%s\n = nil, want 3 problems", describe(n))
	}
	// errors.As stops at the first match: the first field with a problem.
	var fe *deploynote.FieldError
	if !errors.As(err, &fe) {
		t.Fatalf("%s\nerrors.As(err, &fe) = false, want true", describe(n))
	}
	if fe.Field != "Version" {
		t.Errorf("%s\nerrors.As(err, &fe) found %q, want %q", describe(n), fe.Field, "Version")
	}
	// To see every problem, a program walks the list.
	var fields []string
	for _, fe := range fieldErrors(t, describe(n), err) {
		fields = append(fields, fe.Field)
	}
	if got, want := strings.Join(fields, ","), "Version,Author,Rollback"; got != want {
		t.Errorf("%s\nfields = %s, want %s", describe(n), got, want)
	}
	if errors.Is(err, deploynote.ErrFreeze) || errors.Is(err, deploynote.ErrBadFormat) {
		t.Errorf("%s\nerrors.Is finds ErrFreeze or ErrBadFormat in %q, want only ErrMissing", describe(n), err)
	}
}

func TestCheckAllNil(t *testing.T) {
	notes := []deploynote.Note{
		validNote(),
		{Service: "push", Version: "v3.0.1", Ticket: "INC-45", Author: "ines", Rollback: "redeploy v3.0.0", At: day(7, 3, 12, 0, 0, cet)},
		{Service: "checkout", Version: "2026.11.06", Ticket: "PECK-9", Author: "dot", Rollback: "x", At: day(6, 14, 59, 59, 0, cet)},
		{Service: "orders", Version: "v1", Ticket: "PECK-1", Author: "b", Rollback: "redeploy", At: day(9, 9, 0, 0, 0, time.UTC)},
	}
	for _, n := range notes {
		err := deploynote.Check(n)
		// Compare the interface itself with nil. A nil *FieldError or an empty
		// join stored in an error is not equal to nil; %T shows its type.
		if err != nil {
			t.Errorf("%s\n = %v (type %T), want the untyped nil", describe(n), err, err)
		}
	}
}
