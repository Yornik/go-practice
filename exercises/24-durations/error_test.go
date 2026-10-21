package durations_test

import (
	"errors"
	"fmt"
	"testing"

	durations "github.com/yornik/go-practice/exercises/24-durations"
)

func TestSyntaxErrorMessage(t *testing.T) {
	tests := []struct {
		err  *durations.SyntaxError
		want string
	}{
		{
			&durations.SyntaxError{Input: "2x", Col: 2, Err: durations.ErrUnknownUnit},
			`duration "2x": column 2: unknown unit`,
		},
		{
			&durations.SyntaxError{Input: "12", Col: 3, Err: durations.ErrMissingUnit},
			`duration "12": column 3: missing unit`,
		},
		{
			&durations.SyntaxError{Input: "", Col: 1, Err: durations.ErrNumber},
			`duration "": column 1: expected a number`,
		},
		// %q escapes the quote and the newline, so the input stays readable.
		{
			&durations.SyntaxError{Input: "1h\"\n", Col: 3, Err: durations.ErrNumber},
			`duration "1h\"\n": column 3: expected a number`,
		},
	}
	for _, tc := range tests {
		if got := tc.err.Error(); got != tc.want {
			t.Errorf("SyntaxError{Input: %q, Col: %d, Err: %v}.Error()\n got  %q\n want %q",
				tc.err.Input, tc.err.Col, tc.err.Err, got, tc.want)
		}
	}
}

func TestSyntaxErrorUnwrap(t *testing.T) {
	se := &durations.SyntaxError{Input: "1h2d", Col: 4, Err: durations.ErrUnitOrder}
	if got := se.Unwrap(); got != durations.ErrUnitOrder {
		t.Errorf("SyntaxError{Err: ErrUnitOrder}.Unwrap() = %v, want ErrUnitOrder", got)
	}
	if !errors.Is(se, durations.ErrUnitOrder) {
		t.Errorf("errors.Is(%v, ErrUnitOrder) = false, want true", se)
	}
	if errors.Is(se, durations.ErrOverflow) {
		t.Errorf("errors.Is(%v, ErrOverflow) = true, want false", se)
	}
}

// A caller such as a command-line tool adds its own context with %w. The
// sentinel and the column must stay reachable through that wrapping.
func TestSyntaxErrorThroughWrapping(t *testing.T) {
	var err error = &durations.SyntaxError{Input: "2mo", Col: 2, Err: durations.ErrUnknownUnit}
	wrapped := fmt.Errorf("silence disk: %w", err)

	if !errors.Is(wrapped, durations.ErrUnknownUnit) {
		t.Errorf("errors.Is(%q, ErrUnknownUnit) = false, want true", wrapped)
	}
	if errors.Is(wrapped, durations.ErrNumber) {
		t.Errorf("errors.Is(%q, ErrNumber) = true, want false", wrapped)
	}
	var se *durations.SyntaxError
	if !errors.As(wrapped, &se) {
		t.Fatalf("errors.As(%q, *SyntaxError) = false, want true", wrapped)
	}
	if se.Col != 2 || se.Input != "2mo" {
		t.Errorf("errors.As found Input %q, Col %d; want Input \"2mo\", Col 2", se.Input, se.Col)
	}
	want := `silence disk: duration "2mo": column 2: unknown unit`
	if got := wrapped.Error(); got != want {
		t.Errorf("wrapped error message\n got  %q\n want %q", got, want)
	}
}
