package retry_test

import (
	"testing"
	"time"

	retry "github.com/yornik/go-practice/exercises/07-retry"
)

// Step 2: the Retry-After header. A pure function: now is a parameter.

// headerNow is the "current time" for the date tests. The server's date is
// 1 minute later: Wed, 21 Oct 2026 07:28:00 GMT.
var headerNow = time.Date(2026, 10, 21, 7, 27, 0, 0, time.UTC)

func TestParseRetryAfterSeconds(t *testing.T) {
	defer catchPanic(t)
	tests := []struct {
		in   string
		want time.Duration
	}{
		{"0", 0},
		{"1", time.Second},
		{"120", 2 * time.Minute},
		{"86400", 24 * time.Hour},
		// The largest number of seconds that fits in a time.Duration (about 292 years).
		{"9223372036", 9223372036 * time.Second},
	}
	for _, tc := range tests {
		got, ok := retry.ParseRetryAfter(tc.in, headerNow)
		if !ok || got != tc.want {
			t.Errorf("ParseRetryAfter(%q, now) = %v, %t; want %v, true", tc.in, got, ok, tc.want)
		}
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	defer catchPanic(t)
	amsterdam := time.FixedZone("CEST", 2*60*60)
	tests := []struct {
		name string
		in   string
		now  time.Time
		want time.Duration
	}{
		{"IMF-fixdate, the format servers send", "Wed, 21 Oct 2026 07:28:00 GMT", headerNow, time.Minute},
		{"obsolete RFC 850 format", "Wednesday, 21-Oct-26 07:28:00 GMT", headerNow, time.Minute},
		{"obsolete asctime format", "Wed Oct 21 07:28:00 2026", headerNow, time.Minute},
		{"date 90 minutes ahead", "Wed, 21 Oct 2026 08:57:00 GMT", headerNow, 90 * time.Minute},
		// time.Now() is in the local time zone. The same instant in another
		// zone must give the same answer.
		{"now in another time zone", "Wed, 21 Oct 2026 07:28:00 GMT", headerNow.In(amsterdam), time.Minute},
		{"now has a fraction of a second", "Wed, 21 Oct 2026 07:28:00 GMT", headerNow.Add(250 * ms), 59750 * ms},
		{"date equal to now", "Wed, 21 Oct 2026 07:27:00 GMT", headerNow, 0},
		// A date in the past (or a client clock that runs ahead of the
		// server's) means: you may retry now.
		{"date in the past", "Wed, 21 Oct 2026 07:00:00 GMT", headerNow, 0},
		{"date years in the past", "Sun, 06 Nov 1994 08:49:37 GMT", headerNow, 0},
	}
	for _, tc := range tests {
		got, ok := retry.ParseRetryAfter(tc.in, tc.now)
		if !ok || got != tc.want {
			t.Errorf("%s: ParseRetryAfter(%q, %v) = %v, %t; want %v, true", tc.name, tc.in, tc.now, got, ok, tc.want)
		}
	}
}

func TestParseRetryAfterInvalid(t *testing.T) {
	defer catchPanic(t)
	tests := []struct {
		name string
		in   string
	}{
		{"empty (no header)", ""},
		{"a word", "soon"},
		{"negative number", "-1"},
		{"decimal number", "1.5"},
		{"number with a unit", "120s"},
		{"RFC 3339 is not an HTTP-date", "2026-10-21T07:28:00Z"},
		{"too many seconds for a Duration", "9223372037"},
		{"too many seconds for an int64", "99999999999999999999"},
	}
	for _, tc := range tests {
		got, ok := retry.ParseRetryAfter(tc.in, headerNow)
		if ok || got != 0 {
			t.Errorf("%s: ParseRetryAfter(%q, now) = %v, %t; want 0, false", tc.name, tc.in, got, ok)
		}
	}
}

// FuzzParseRetryAfter checks two properties on any input: the result is never
// negative, and a false result comes with a 0 duration. Plain `go test` runs
// only the seed inputs; `go test -fuzz=FuzzParseRetryAfter` invents more.
func FuzzParseRetryAfter(f *testing.F) {
	for _, s := range []string{"0", "120", "-1", "+1", "1e3", "9223372037", "Wed, 21 Oct 2026 07:28:00 GMT", "Wed Oct 21 07:28:00 2026", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		defer catchPanic(t)
		got, ok := retry.ParseRetryAfter(in, headerNow)
		if got < 0 {
			t.Errorf("ParseRetryAfter(%q, now) = %v, %t; a delay is never negative", in, got, ok)
		}
		if !ok && got != 0 {
			t.Errorf("ParseRetryAfter(%q, now) = %v, false; want 0 with false", in, got)
		}
	})
}
