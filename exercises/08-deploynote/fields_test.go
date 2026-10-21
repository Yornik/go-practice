package deploynote_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	deploynote "github.com/yornik/go-practice/exercises/08-deploynote"
)

func TestCheckFieldsValid(t *testing.T) {
	checkProblems(t, validNote(), nil)
}

func TestCheckFieldsMissing(t *testing.T) {
	// set changes 1 string field of a valid note.
	set := map[string]func(n *deploynote.Note, v string){
		"Service":  func(n *deploynote.Note, v string) { n.Service = v },
		"Version":  func(n *deploynote.Note, v string) { n.Version = v },
		"Ticket":   func(n *deploynote.Note, v string) { n.Ticket = v },
		"Author":   func(n *deploynote.Note, v string) { n.Author = v },
		"Rollback": func(n *deploynote.Note, v string) { n.Rollback = v },
	}
	for _, field := range []string{"Service", "Version", "Ticket", "Author", "Rollback"} {
		for _, v := range []string{"", " ", "   ", "\t", " \t\n "} {
			t.Run(fmt.Sprintf("%s=%q", field, v), func(t *testing.T) {
				n := validNote()
				set[field](&n, v)
				checkProblems(t, n, []problem{{field, deploynote.ErrMissing}})
			})
		}
	}
	t.Run("At is the zero time", func(t *testing.T) {
		n := validNote()
		n.At = time.Time{}
		checkProblems(t, n, []problem{{"At", deploynote.ErrMissing}})
	})
}

func TestCheckFieldsMessage(t *testing.T) {
	tests := []struct {
		name   string
		change func(n *deploynote.Note)
		want   string
	}{
		{"Service missing", func(n *deploynote.Note) { n.Service = "" }, "Service: missing"},
		{"Version missing", func(n *deploynote.Note) { n.Version = " " }, "Version: missing"},
		{"Ticket missing", func(n *deploynote.Note) { n.Ticket = "\t" }, "Ticket: missing"},
		{"Author missing", func(n *deploynote.Note) { n.Author = "" }, "Author: missing"},
		{"Rollback n/a", func(n *deploynote.Note) { n.Rollback = "n/a" }, "Rollback: missing"},
		{"At zero", func(n *deploynote.Note) { n.At = time.Time{} }, "At: missing"},
		{"Ticket bad format", func(n *deploynote.Note) { n.Ticket = "small fix" }, `Ticket: bad format: "small fix"`},
		{"Ticket with a space", func(n *deploynote.Note) { n.Ticket = " VL-12" }, `Ticket: bad format: " VL-12"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n := validNote()
			tc.change(&n)
			err := deploynote.Check(n)
			if err == nil {
				t.Fatalf("%s\n = nil, want %q", describe(n), tc.want)
			}
			if got := err.Error(); got != tc.want {
				t.Errorf("%s.Error()\n = %q\nwant %q", describe(n), got, tc.want)
			}
		})
	}
}

func TestCheckFieldsRollback(t *testing.T) {
	for _, v := range []string{"n/a", "N/A", "N/a", "none", "None", "NONE", "-", " n/a ", "\tnone\n", " - "} {
		t.Run(fmt.Sprintf("missing %q", v), func(t *testing.T) {
			n := validNote()
			n.Rollback = v
			checkProblems(t, n, []problem{{"Rollback", deploynote.ErrMissing}})
		})
	}
	for _, v := range []string{
		"helm rollback alert-rules 41",
		"redeploy v1.4.2",
		"set flag new_thresholds=off",
		"x",
	} {
		t.Run(fmt.Sprintf("valid %q", v), func(t *testing.T) {
			n := validNote()
			n.Rollback = v
			checkProblems(t, n, nil)
		})
	}
}

func TestCheckFieldsTicket(t *testing.T) {
	for _, v := range []string{"VL-1", "VL-412", "VL-000123", "VL-999999", "INC-45", "INC-0", "INC-123456"} {
		t.Run(fmt.Sprintf("valid %q", v), func(t *testing.T) {
			n := validNote()
			n.Ticket = v
			checkProblems(t, n, nil)
		})
	}
	bad := []string{
		"small fix",
		"VL-",        // no digits
		"VL-1234567", // 7 digits
		"VL123",      // no dash
		"VL--12",     // 2 dashes
		"VL-12a",     // a letter after the digits
		"VL-+12",     // a sign is not a digit
		"VL-١٢",      // Arabic-Indic digits are not ASCII digits
		"vl-12",      // lowercase prefix
		"Vl-12",
		"inc-45",
		"INC45",
		"JIRA-12",
		" VL-12", // spaces count: the ticket is checked as written
		"VL-12 ",
		"VL-12 VL-13",
		"https://jira.vesper-labs.example/browse/VL-12",
	}
	for _, v := range bad {
		t.Run(fmt.Sprintf("bad %q", v), func(t *testing.T) {
			n := validNote()
			n.Ticket = v
			checkProblems(t, n, []problem{{"Ticket", deploynote.ErrBadFormat}})
			err := deploynote.Check(n)
			if err == nil {
				return // checkProblems reported it
			}
			if q := fmt.Sprintf("%q", v); !strings.Contains(err.Error(), q) {
				t.Errorf("%s\nerror %q does not contain the ticket quoted with %%q: %s", describe(n), err.Error(), q)
			}
		})
	}
}
