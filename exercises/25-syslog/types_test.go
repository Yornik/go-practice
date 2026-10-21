package syslog_test

import (
	"errors"
	"fmt"
	"testing"

	syslog "github.com/yornik/go-practice/exercises/25-syslog"
)

// The tests in this file are named TestSeverity... and TestParseError... so
// that step 1 has its own -run filter: -run 'TestSeverity|TestParseError'.

func TestSeverityString(t *testing.T) {
	tests := []struct {
		s    syslog.Severity
		want string
	}{
		{syslog.Emergency, "emerg"},
		{syslog.Alert, "alert"},
		{syslog.Critical, "crit"},
		{syslog.Error, "err"},
		{syslog.Warning, "warning"},
		{syslog.Notice, "notice"},
		{syslog.Info, "info"},
		{syslog.Debug, "debug"},
		{syslog.Severity(8), "Severity(8)"},
		{syslog.Severity(-1), "Severity(-1)"},
		{syslog.Severity(191), "Severity(191)"},
	}
	for _, tc := range tests {
		if got := tc.s.String(); got != tc.want {
			t.Errorf("Severity(%d).String() = %q, want %q", int(tc.s), got, tc.want)
		}
	}

	// fmt calls String for %v and %s, so a Severity prints as a word.
	if got := fmt.Sprint(syslog.Warning); got != "warning" {
		t.Errorf("fmt.Sprint(Warning) = %q, want \"warning\"", got)
	}
	if got := fmt.Sprintf("%v|%s", syslog.Error, syslog.Severity(9)); got != "err|Severity(9)" {
		t.Errorf(`fmt.Sprintf("%%v|%%s", Error, Severity(9)) = %q, want "err|Severity(9)"`, got)
	}
}

func TestParseErrorMessage(t *testing.T) {
	tests := []struct {
		err  *syslog.ParseError
		want string
	}{
		{
			&syslog.ParseError{Field: "host", Col: 21, Err: syslog.ErrHost},
			"syslog: column 21: host: missing host",
		},
		{
			&syslog.ParseError{Field: "priority", Col: 1, Err: syslog.ErrPriority},
			"syslog: column 1: priority: bad priority",
		},
		{
			// Err is printed with its own message, also when it wraps more.
			&syslog.ParseError{Field: "timestamp", Col: 5, Err: fmt.Errorf("%w: %w", syslog.ErrTimestamp, errors.New("month out of range"))},
			"syslog: column 5: timestamp: bad timestamp: month out of range",
		},
	}
	for _, tc := range tests {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("ParseError{Field: %q, Col: %d, Err: %q}.Error()\n got  %q\n want %q",
				tc.err.Field, tc.err.Col, tc.err.Err, got, tc.want)
		}
	}
}

func TestParseErrorUnwrap(t *testing.T) {
	pe := &syslog.ParseError{Field: "tag", Col: 37, Err: syslog.ErrTag}
	if got := pe.Unwrap(); got != syslog.ErrTag {
		t.Errorf("ParseError{Err: ErrTag}.Unwrap() = %v, want ErrTag", got)
	}
	if !errors.Is(pe, syslog.ErrTag) {
		t.Errorf("errors.Is(%q, ErrTag) = false, want true", pe)
	}
	if errors.Is(pe, syslog.ErrHost) {
		t.Errorf("errors.Is(%q, ErrHost) = true, want false", pe)
	}

	// An Err made with 2 %w verbs wraps both errors, and errors.Is and
	// errors.As see both through the *ParseError.
	cause := &customError{msg: "cannot parse"}
	pe = &syslog.ParseError{Field: "timestamp", Col: 5, Err: fmt.Errorf("%w: %w", syslog.ErrTimestamp, cause)}
	if !errors.Is(pe, syslog.ErrTimestamp) {
		t.Errorf("errors.Is(%q, ErrTimestamp) = false, want true", pe)
	}
	var ce *customError
	if !errors.As(pe, &ce) || ce != cause {
		t.Errorf("errors.As(%q, *customError) did not find the second wrapped error", pe)
	}

	// A caller adds context with %w; the *ParseError stays reachable.
	wrapped := fmt.Errorf("feeders.log line 3: %w", &syslog.ParseError{Field: "host", Col: 21, Err: syslog.ErrHost})
	var got *syslog.ParseError
	if !errors.As(wrapped, &got) {
		t.Fatalf("errors.As(%q, *ParseError) = false, want true", wrapped)
	}
	if got.Field != "host" || got.Col != 21 {
		t.Errorf("errors.As found Field %q, Col %d; want Field \"host\", Col 21", got.Field, got.Col)
	}
	if !errors.Is(wrapped, syslog.ErrHost) {
		t.Errorf("errors.Is(%q, ErrHost) = false, want true", wrapped)
	}
}

// customError stands in for an error type such as *time.ParseError.
type customError struct{ msg string }

func (e *customError) Error() string { return e.msg }
