package syslog_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	syslog "github.com/yornik/go-practice/exercises/25-syslog"
)

// Each step checks only the fields it adds. While you work on step 2, Parse
// may stop after the priority and return what it has; step 3 adds the
// timestamp, and so on. Every valid line below is valid in full, so the
// finished Parse passes every step.

// now is the time the tests pass to Parse unless a case says otherwise.
var now = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

// fieldErr is the sentinel that belongs to each Field of a *ParseError.
var fieldErr = map[string]error{
	"priority":  syslog.ErrPriority,
	"timestamp": syslog.ErrTimestamp,
	"host":      syslog.ErrHost,
	"tag":       syslog.ErrTag,
}

type errorCase struct {
	name      string
	line      string
	field     string // the Field the *ParseError must have
	col       int    // the Col the *ParseError must have
	timeParse bool   // the error must wrap a *time.ParseError too
}

// runErrorCases checks that Parse rejects every line with a *ParseError in
// the wanted field and column, and returns the zero Message.
func runErrorCases(t *testing.T, cases []errorCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := syslog.Parse(tc.line, now)
			if err == nil {
				t.Fatalf("Parse(%q) = %+v, nil; want a *ParseError in field %q at column %d", tc.line, got, tc.field, tc.col)
			}
			var pe *syslog.ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("Parse(%q) error = %v (type %T), want a *syslog.ParseError", tc.line, err, err)
			}
			if pe.Field != tc.field || pe.Col != tc.col {
				t.Errorf("Parse(%q) error in field %q at column %d, want field %q at column %d (error: %v)",
					tc.line, pe.Field, pe.Col, tc.field, tc.col, err)
			}
			if want := fieldErr[tc.field]; !errors.Is(err, want) {
				t.Errorf("Parse(%q) error = %v, want errors.Is(err, %q) to be true", tc.line, err, want)
			}
			var te *time.ParseError
			if tc.timeParse && !errors.As(err, &te) {
				t.Errorf("Parse(%q) error = %v, want it to wrap the *time.ParseError (errors.As)", tc.line, err)
			}
			if got != (syslog.Message{}) {
				t.Errorf("Parse(%q) = %+v with an error, want the zero Message", tc.line, got)
			}
		})
	}
}

// mustParse calls Parse on a line that is valid in full.
func mustParse(t *testing.T, line string, now time.Time) syslog.Message {
	t.Helper()
	got, err := syslog.Parse(line, now)
	if err != nil {
		t.Fatalf("Parse(%q) error = %v, want nil", line, err)
	}
	return got
}

// Step 2: the priority.

func TestParsePriority(t *testing.T) {
	tests := []struct {
		line     string
		facility int
		severity syslog.Severity
	}{
		{"<0>Oct 16 03:12:44 feeder-0412 kernel: boot", 0, syslog.Emergency},
		{"<7>Oct 16 03:12:44 feeder-0412 kernel: boot", 0, syslog.Debug},
		{"<11>Oct 16 03:12:44 feeder-0412 camd[311]: lens heater failed", 1, syslog.Error},
		{"<13>Oct 16 03:12:44 feeder-0412 camd: ok", 1, syslog.Notice},
		{"<34>Oct 16 03:12:44 photo-store-2 su: auth failed", 4, syslog.Critical},
		{"<134>Oct 16 03:12:44 hq-window-01 motiond[412]: motion", 16, syslog.Info},
		{"<191>Oct 16 03:12:44 feeder-0412 updated: checking", 23, syslog.Debug},
	}
	for _, tc := range tests {
		t.Run(tc.line[:strings.IndexByte(tc.line, '>')+1], func(t *testing.T) {
			got := mustParse(t, tc.line, now)
			if got.Facility != tc.facility || got.Severity != tc.severity {
				t.Errorf("Parse(%q): Facility %d, Severity %d; want Facility %d, Severity %d",
					tc.line, got.Facility, int(got.Severity), tc.facility, int(tc.severity))
			}
		})
	}
}

func TestParsePriorityErrors(t *testing.T) {
	runErrorCases(t, []errorCase{
		{"empty line", ``, "priority", 1, false},
		{"no <", `13>Oct 16 03:12:44 feeder-0412 camd: ok`, "priority", 1, false},
		{"space before <", ` <13>Oct 16 03:12:44 feeder-0412 camd: ok`, "priority", 1, false},
		{"no digits", `<>Oct 16 03:12:44 feeder-0412 camd: ok`, "priority", 2, false},
		{"a letter", `<a>Oct 16 03:12:44 feeder-0412 camd: ok`, "priority", 2, false},
		{"a sign", `<-1>Oct 16 03:12:44 feeder-0412 camd: ok`, "priority", 2, false},
		{"line ends after <", `<`, "priority", 2, false},
		// After 3 digits a > must follow, so the 4th digit is the problem.
		{"4 digits", `<1234>Oct 16 03:12:44 feeder-0412 camd: ok`, "priority", 5, false},
		{"no > after the digits", `<13Oct 16 03:12:44 feeder-0412 camd: ok`, "priority", 4, false},
		{"line ends after the digits", `<13`, "priority", 4, false},
		// Above 191: the column of the first digit.
		{"192", `<192>Oct 16 03:12:44 feeder-0412 camd: ok`, "priority", 2, false},
		{"999", `<999>Oct 16 03:12:44 feeder-0412 camd: ok`, "priority", 2, false},
	})
}

// Step 3: the timestamp, without the year.

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		line  string
		month time.Month
		day   int
		clock [3]int // hour, minute, second
	}{
		{"<13>Oct  1 03:12:44 feeder-0412 camd: ok", time.October, 1, [3]int{3, 12, 44}},
		{"<13>Oct 01 03:12:44 feeder-0412 camd: ok", time.October, 1, [3]int{3, 12, 44}},
		{"<13>Oct 16 23:59:59 feeder-0412 camd: ok", time.October, 16, [3]int{23, 59, 59}},
		{"<13>Sep 30 00:00:00 feeder-0412 camd: ok", time.September, 30, [3]int{0, 0, 0}},
		{"<13>Feb  5 17:32:18 feeder-0412 camd: ok", time.February, 5, [3]int{17, 32, 18}},
	}
	for _, tc := range tests {
		t.Run(tc.line[4:19], func(t *testing.T) {
			got := mustParse(t, tc.line, now)
			h, m, s := got.Time.Clock()
			if got.Time.Month() != tc.month || got.Time.Day() != tc.day || [3]int{h, m, s} != tc.clock {
				t.Errorf("Parse(%q).Time = %v, want %s %d %02d:%02d:%02d (any year)",
					tc.line, got.Time, tc.month, tc.day, tc.clock[0], tc.clock[1], tc.clock[2])
			}
		})
	}
}

func TestParseTimestampErrors(t *testing.T) {
	runErrorCases(t, []errorCase{
		// time.Parse rejects the 15 bytes: the column of the first byte of
		// the timestamp, and the error wraps the *time.ParseError.
		{"unknown month", `<13>Okt 16 03:12:44 feeder-0412 camd: ok`, "timestamp", 5, true},
		{"day 32", `<13>Oct 32 03:12:44 feeder-0412 camd: ok`, "timestamp", 5, true},
		{"hour 24", `<13>Oct 16 24:00:00 feeder-0412 camd: ok`, "timestamp", 5, true},
		{"1-digit day without the padding space", `<13>Oct 1 03:12:44 feeder-0412 camd: ok`, "timestamp", 5, true},
		{"RFC 3339 timestamp", `<13>2026-10-16T03:12:44Z feeder-0412 camd: ok`, "timestamp", 5, true},
		{"RFC 3164 example 4", `<0>1990 Oct 22 10:52:01 TZ-6 scapegoat.dmz.example.org 10.1.2.3 sched[0]: That's All Folks!`, "timestamp", 4, true},
		// Fewer than 15 bytes left: the end of the line.
		{"line ends inside the timestamp", `<13>Oct 16 03:12`, "timestamp", 17, false},
		{"line ends after the priority", `<13>`, "timestamp", 5, false},
		// More than 15 bytes: the first byte after the 15.
		{"fractional seconds", `<13>Oct 16 03:12:44.250 feeder-0412 camd: ok`, "timestamp", 20, false},
	})
}

// Step 4: the year and the location.

func TestParseYear(t *testing.T) {
	cet := time.FixedZone("CET", 1*60*60)
	cest := time.FixedZone("CEST", 2*60*60)
	est := time.FixedZone("EST", -5*60*60)
	newYear := time.Date(2027, 1, 1, 0, 5, 0, 0, cet)
	october := time.Date(2026, 10, 16, 12, 0, 0, 0, cest)

	tests := []struct {
		name  string
		now   time.Time
		stamp string // the 15-byte timestamp of the line
		want  time.Time
	}{
		{"New Year's Eve line read at 00:05 on 1 January", newYear, "Dec 31 23:58:02", time.Date(2026, 12, 31, 23, 58, 2, 0, cet)},
		{"1 January line read at 00:05 on 1 January", newYear, "Jan  1 00:04:59", time.Date(2027, 1, 1, 0, 4, 59, 0, cet)},
		{"the same second as now", october, "Oct 16 12:00:00", time.Date(2026, 10, 16, 12, 0, 0, 0, cest)},
		{"earlier this year", october, "Mar  3 08:00:00", time.Date(2026, 3, 3, 8, 0, 0, 0, cest)},
		{"6 days ahead is this year", october, "Oct 22 12:00:00", time.Date(2026, 10, 22, 12, 0, 0, 0, cest)},
		{"exactly 7 days ahead is this year", october, "Oct 23 12:00:00", time.Date(2026, 10, 23, 12, 0, 0, 0, cest)},
		{"7 days and 1 second ahead is last year", october, "Oct 23 12:00:01", time.Date(2025, 10, 23, 12, 0, 1, 0, cest)},
		{"8 days ahead is last year", october, "Oct 24 12:00:00", time.Date(2025, 10, 24, 12, 0, 0, 0, cest)},
		{"now in UTC", now, "Oct 16 11:59:00", time.Date(2026, 10, 16, 11, 59, 0, 0, time.UTC)},
		// The clock in the line is a local time in now's location; it is
		// not a UTC time converted to that location.
		{"now west of UTC", time.Date(2026, 12, 31, 20, 0, 0, 0, est), "Dec 31 19:58:00", time.Date(2026, 12, 31, 19, 58, 0, 0, est)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line := "<13>" + tc.stamp + " feeder-0412 camd: ok"
			got := mustParse(t, line, tc.now)
			if !got.Time.Equal(tc.want) {
				t.Errorf("Parse(%q, %v).Time = %v, want %v", line, tc.now, got.Time, tc.want)
			}
			gotName, gotOff := got.Time.Zone()
			wantName, wantOff := tc.want.Zone()
			if gotName != wantName || gotOff != wantOff {
				t.Errorf("Parse(%q, %v).Time is in zone %s (%+d s), want now's zone %s (%+d s)",
					line, tc.now, gotName, gotOff, wantName, wantOff)
			}
		})
	}
}

// Step 5: the host, the tag and the text.

func TestParseHostTag(t *testing.T) {
	const ts = "<13>Oct 16 03:12:44 "
	tests := []struct {
		name string
		line string
		host string
		app  string
		pid  int
		text string
	}{
		{"with a PID", ts + "photo-store-2 systemd[1]: Started backup.", "photo-store-2", "systemd", 1, "Started backup."},
		{"without a PID", ts + "feeder-0412 camd: lens heater failed", "feeder-0412", "camd", 0, "lens heater failed"},
		{"PID 0", ts + "feeder-0412 sched[0]: tick", "feeder-0412", "sched", 0, "tick"},
		{"empty text", ts + "feeder-0412 camd:", "feeder-0412", "camd", 0, ""},
		{"empty text after the space", ts + "feeder-0412 camd[311]: ", "feeder-0412", "camd", 311, ""},
		{"no space after the colon", ts + "feeder-0412 camd:ok", "feeder-0412", "camd", 0, "ok"},
		{"only 1 space is skipped", ts + "feeder-0412 camd:  indented", "feeder-0412", "camd", 0, " indented"},
		{
			"text with colons and brackets", ts + "hq-window-01 sshd[2213]: pam_unix(sshd:session): opened for [ines]: ok",
			"hq-window-01", "sshd", 2213, "pam_unix(sshd:session): opened for [ines]: ok",
		},
		{"app name with a dot", ts + "feeder-0412 seed-level.d: low", "feeder-0412", "seed-level.d", 0, "low"},
		{"app name with slashes", ts + "photo-store-2 /usr/sbin/cron[812]: (root) CMD (gc)", "photo-store-2", "/usr/sbin/cron", 812, "(root) CMD (gc)"},
		{"postfix style", ts + "mail-1 postfix/smtpd[4242]: connect from unknown[192.0.2.10]", "mail-1", "postfix/smtpd", 4242, "connect from unknown[192.0.2.10]"},
		{"IPv4 address as host", ts + "192.0.2.44 camd: ok", "192.0.2.44", "camd", 0, "ok"},
		{"host with a domain", ts + "hq-window-01.peckish.example camd: ok", "hq-window-01.peckish.example", "camd", 0, "ok"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mustParse(t, tc.line, now)
			if got.Host != tc.host || got.App != tc.app || got.PID != tc.pid || got.Text != tc.text {
				t.Errorf("Parse(%q)\n got  Host %q, App %q, PID %d, Text %q\n want Host %q, App %q, PID %d, Text %q",
					tc.line, got.Host, got.App, got.PID, got.Text, tc.host, tc.app, tc.pid, tc.text)
			}
		})
	}
}

func TestParseHostTagErrors(t *testing.T) {
	// ts is 19 bytes, so the host starts at column 21.
	const ts = "<11>Dec 31 23:58:02"
	runErrorCases(t, []errorCase{
		// A word that ends with ":" is a tag: the host is missing.
		{"tag where the host should be", ts + " camd[311]: lens heater failed", "host", 21, false},
		{"tag without PID where the host should be", ts + " camd: lens heater failed", "host", 21, false},
		{"line ends after the timestamp", ts, "host", 20, false},
		{"line ends after the space", ts + " ", "host", 21, false},
		{"2 spaces before the host", ts + "  feeder-0412 camd: ok", "host", 21, false},

		// feeder-0412 is columns 21 to 31, so the tag starts at column 33.
		{"line ends after the host", ts + " feeder-0412", "tag", 32, false},
		{"line ends after the space after the host", ts + " feeder-0412 ", "tag", 33, false},
		{"2 spaces before the tag", ts + " feeder-0412  camd: ok", "tag", 33, false},
		{"tag starts with [", ts + " feeder-0412 [311]: ok", "tag", 33, false},
		{"tag starts with :", ts + " feeder-0412 : ok", "tag", 33, false},
		{"no colon: space after the app name", ts + " feeder-0412 camd lens heater failed", "tag", 37, false},
		{"no colon: line ends after the app name", ts + " feeder-0412 camd", "tag", 37, false},
		{"[ without ]", ts + " feeder-0412 camd[311: ok", "tag", 41, false},
		{"line ends inside [pid]", ts + " feeder-0412 camd[311", "tag", 41, false},
		{"letter in the PID", ts + " feeder-0412 camd[12a]: ok", "tag", 40, false},
		{"empty PID", ts + " feeder-0412 camd[]: ok", "tag", 38, false},
		{"PID too large for an int", ts + " feeder-0412 camd[99999999999999999999]: ok", "tag", 38, false},
		{"no colon after ]", ts + " feeder-0412 camd[311] ok", "tag", 42, false},
		{"line ends after ]", ts + " feeder-0412 camd[311]", "tag", 42, false},
	})
}
