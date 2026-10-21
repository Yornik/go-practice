package syslog_test

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	syslog "github.com/yornik/go-practice/exercises/25-syslog"
)

// Step 6: real lines.

// The 5 example lines of RFC 3164, section 5.4, each on 1 line. Only the
// first follows the format of this exercise in full. The others have no tag,
// or a timestamp in another format, and Parse rejects them instead of
// guessing.
func TestParseRealRFC3164(t *testing.T) {
	line := `<34>Oct 11 22:14:15 mymachine su: 'su root' failed for lonvick on /dev/pts/8`
	want := syslog.Message{
		Facility: 4,
		Severity: syslog.Critical,
		Time:     time.Date(2026, 10, 11, 22, 14, 15, 0, time.UTC),
		Host:     "mymachine",
		App:      "su",
		PID:      0,
		Text:     "'su root' failed for lonvick on /dev/pts/8",
	}
	got := mustParse(t, line, now)
	if !sameMessage(got, want) {
		t.Errorf("Parse(%q)\n got  %+v\n want %+v", line, got, want)
	}

	runErrorCases(t, []errorCase{
		// The relay put an IP address in HOSTNAME; "Use" is not followed by ":".
		{"example 2", `<13>Feb  5 17:32:18 10.0.0.99 Use the BFG!`, "tag", 34, false},
		// "CST" would be the host and "1987" the tag.
		{
			"example 3",
			`<165>Aug 24 05:34:00 CST 1987 mymachine myproc[10]: %% It's time to make the do-nuts.  %%  Ingredients: Mix=OK, Jelly=OK # Devices: Mixer=OK, Jelly_Injector=OK, Frier=OK # Transport: Conveyer1=OK, Conveyer2=OK # %%`,
			"tag", 30, false,
		},
		{"example 4", `<0>1990 Oct 22 10:52:01 TZ-6 scapegoat.dmz.example.org 10.1.2.3 sched[0]: That's All Folks!`, "timestamp", 4, true},
		{"example 4 relayed", `<0>Oct 22 10:52:12 scapegoat 1990 Oct 22 10:52:01 TZ-6 scapegoat.dmz.example.org 10.1.2.3 sched[0]: That's All Folks!`, "tag", 34, false},
	})
}

// sameMessage compares 2 Messages. Times are equal when they are the same
// instant in the same zone.
func sameMessage(a, b syslog.Message) bool {
	aName, aOff := a.Time.Zone()
	bName, bOff := b.Time.Zone()
	a.Time, b.Time = time.Time{}, time.Time{}
	return a == b && aName == bName && aOff == bOff
}

// render writes the result of Parse as 1 line, so that a whole log can be
// compared with a golden file. sameMessage above already covers the parts
// of the time that RFC3339 leaves out.
func render(m syslog.Message, err error) string {
	if err != nil {
		var pe *syslog.ParseError
		if !errors.As(err, &pe) {
			return fmt.Sprintf("error %v (type %T, want a *syslog.ParseError)", err, err)
		}
		return fmt.Sprintf("error field=%s col=%d", pe.Field, pe.Col)
	}
	return fmt.Sprintf("%s facility=%d severity=%s host=%s app=%s pid=%d text=%q",
		m.Time.Format(time.RFC3339), m.Facility, m.Severity, m.Host, m.App, m.PID, m.Text)
}

// TestParseRealUndercroftLog parses the log of the New Year incident.
// testdata/undercroft.golden holds the result of each line, in the format of
// render above. The log was read at 00:05 on 1 January 2027, in CET.
func TestParseRealUndercroftLog(t *testing.T) {
	readAt := time.Date(2027, 1, 1, 0, 5, 0, 0, time.FixedZone("CET", 1*60*60))

	lines := readLines(t, "testdata/undercroft.log")
	golden := readLines(t, "testdata/undercroft.golden")
	if len(lines) != len(golden) {
		t.Fatalf("testdata/undercroft.log has %d lines and testdata/undercroft.golden has %d; want the same number",
			len(lines), len(golden))
	}
	for i, line := range lines {
		got := render(syslog.Parse(line, readAt))
		if got != golden[i] {
			t.Errorf("undercroft.log line %d: Parse(%q)\n got  %s\n want %s", i+1, line, got, golden[i])
		}
	}
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// A Windows checkout with core.autocrlf turns "\n" into "\r\n"; the "\r"
	// would end up in Text and shift the column of an error at the end.
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// FuzzParse checks properties that must hold for ANY input, not only for the
// lines above. Plain `go test` runs it on the seed lines below. Run
//
//	go test -fuzz=FuzzParse ./exercises/25-syslog/
//
// to let Go generate new inputs (stop it with Ctrl-C).
func FuzzParse(f *testing.F) {
	seeds := []string{
		``, `<`, `<13`, `<13>`, `<1234>`, `<192>`, `<a>`,
		`<13>Oct 16 03:12`, `<13>Okt 16 03:12:44 h a: x`, `<13>Oct 16 03:12:44.250 h a: x`,
		`<11>Dec 31 23:58:02`, `<11>Dec 31 23:58:02 gpud[311]: ecc error on bank 3`,
		`<11>Dec 31 23:58:02 vl-gpu-0412 gpud[311]: ecc error on bank 3`,
		`<13>Oct  1 03:12:44 vl-gpu-0412 gpud:`, `<13>Oct 16 03:12:44 h a[`, `<13>Oct 16 03:12:44 h a[1`,
		`<13>Oct 16 03:12:44 h a[12a]: x`, `<13>Oct 16 03:12:44 h a[99999999999999999999]: x`,
		`<13>Feb  5 17:32:18 10.0.0.99 Use the BFG!`, `<13>Feb 29 12:00:00 h a: leap`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, line string) {
		got, err := syslog.Parse(line, now)
		if err != nil {
			var pe *syslog.ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("Parse(%q) error = %v (type %T), want a *syslog.ParseError", line, err, err)
			}
			if pe.Col < 1 || pe.Col > len(line)+1 {
				t.Fatalf("Parse(%q) error column = %d, want 1..%d", line, pe.Col, len(line)+1)
			}
			want, ok := fieldErr[pe.Field]
			if !ok {
				t.Fatalf("Parse(%q) error Field = %q, want priority, timestamp, host or tag", line, pe.Field)
			}
			if !errors.Is(err, want) {
				t.Fatalf("Parse(%q) error = %v in field %q, want errors.Is(err, %q)", line, err, pe.Field, want)
			}
			if got != (syslog.Message{}) {
				t.Fatalf("Parse(%q) = %+v with an error, want the zero Message", line, got)
			}
			return
		}
		if got.Facility < 0 || got.Facility > 23 || got.Severity < syslog.Emergency || got.Severity > syslog.Debug {
			t.Fatalf("Parse(%q): Facility %d, Severity %d; want 0..23 and 0..7", line, got.Facility, int(got.Severity))
		}
		if got.Host == "" || strings.ContainsRune(got.Host, ' ') || strings.HasSuffix(got.Host, ":") {
			t.Fatalf("Parse(%q).Host = %q, want a non-empty host without a space and not ending in \":\"", line, got.Host)
		}
		if got.App == "" || strings.ContainsAny(got.App, " [:") {
			t.Fatalf("Parse(%q).App = %q, want a non-empty name without a space, [ or :", line, got.App)
		}
		if got.PID < 0 {
			t.Fatalf("Parse(%q).PID = %d, want >= 0", line, got.PID)
		}
		if !strings.HasSuffix(line, got.Text) {
			t.Fatalf("Parse(%q).Text = %q, want the end of the line", line, got.Text)
		}
		if got.Time.Sub(now) > 7*24*time.Hour {
			t.Fatalf("Parse(%q).Time = %v, more than 7 days after now (%v)", line, got.Time, now)
		}
	})
}

// A relay that points at the broken byte of a line.
func ExampleParse() {
	now := time.Date(2027, 1, 1, 0, 5, 0, 0, time.FixedZone("CET", 1*60*60))
	lines := []string{
		"<11>Dec 31 23:58:02 vl-gpu-0412 gpud[311]: ecc error on bank 3",
		"<11>Dec 31 23:58:02 hushd[311]: checkpoint copied to 203.0.113.51",
	}
	for _, line := range lines {
		msg, err := syslog.Parse(line, now)
		var pe *syslog.ParseError
		if errors.As(err, &pe) {
			fmt.Println(err)
			fmt.Println(line)
			fmt.Println(strings.Repeat(" ", pe.Col-1) + "^")
			continue
		}
		fmt.Println(msg.Time.Format(time.RFC3339), msg.Severity, msg.Host, msg.App, msg.PID, msg.Text)
	}
	// Output:
	// 2026-12-31T23:58:02+01:00 err vl-gpu-0412 gpud 311 ecc error on bank 3
	// syslog: column 21: host: missing host
	// <11>Dec 31 23:58:02 hushd[311]: checkpoint copied to 203.0.113.51
	//                     ^
}
