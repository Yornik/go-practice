package deploynote_test

import (
	"errors"
	"fmt"
	"testing"

	deploynote "github.com/yornik/go-practice/exercises/08-deploynote"
)

func TestFieldErrorMessage(t *testing.T) {
	tests := []struct {
		fe   *deploynote.FieldError
		want string
	}{
		{&deploynote.FieldError{Field: "Service", Err: deploynote.ErrMissing}, "Service: missing"},
		{&deploynote.FieldError{Field: "At", Err: deploynote.ErrFreeze}, "At: change freeze"},
		{
			&deploynote.FieldError{Field: "Ticket", Err: fmt.Errorf("%w: %q", deploynote.ErrBadFormat, "small fix")},
			`Ticket: bad format: "small fix"`,
		},
		// Error works for any Err, not only for the 3 sentinels.
		{&deploynote.FieldError{Field: "Version", Err: errors.New("not a release")}, "Version: not a release"},
	}
	for _, tc := range tests {
		got := tc.fe.Error()
		if got != tc.want {
			t.Errorf("(&FieldError{Field: %q, Err: %q}).Error() = %q, want %q", tc.fe.Field, tc.fe.Err, got, tc.want)
		}
		// fmt calls Error for %v, so a FieldError prints like its message.
		if got := fmt.Sprint(tc.fe); got != tc.want {
			t.Errorf("fmt.Sprint(&FieldError{Field: %q, Err: %q}) = %q, want %q", tc.fe.Field, tc.fe.Err, got, tc.want)
		}
	}
}

func TestFieldErrorUnwrap(t *testing.T) {
	inner := fmt.Errorf("%w: %q", deploynote.ErrBadFormat, "VL123")
	for _, err := range []error{deploynote.ErrMissing, deploynote.ErrFreeze, inner} {
		fe := &deploynote.FieldError{Field: "Ticket", Err: err}
		if got := fe.Unwrap(); got != err {
			t.Errorf("(&FieldError{Field: %q, Err: %q}).Unwrap() = %v, want the same error value as Err", fe.Field, err, got)
		}
	}
}

func TestFieldErrorIs(t *testing.T) {
	tests := []struct {
		name string
		err  error // what Err holds
		want error // the 1 sentinel that errors.Is must find
	}{
		{"Err is ErrMissing", deploynote.ErrMissing, deploynote.ErrMissing},
		{"Err is ErrFreeze", deploynote.ErrFreeze, deploynote.ErrFreeze},
		{"Err wraps ErrBadFormat", fmt.Errorf("%w: %q", deploynote.ErrBadFormat, "VL123"), deploynote.ErrBadFormat},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fe := &deploynote.FieldError{Field: "Ticket", Err: tc.err}
			for _, s := range sentinels {
				want := s == tc.want
				if got := errors.Is(fe, s); got != want {
					t.Errorf("errors.Is(&FieldError{Err: %q}, %v) = %t, want %t", tc.err, s, got, want)
				}
			}
		})
	}
}

func TestFieldErrorAs(t *testing.T) {
	fe := &deploynote.FieldError{Field: "Rollback", Err: deploynote.ErrMissing}
	// A caller may wrap the FieldError again; errors.Is and errors.As still
	// find what is inside.
	wrapped := fmt.Errorf("deploy of alert-rules refused: %w", fe)

	if !errors.Is(wrapped, deploynote.ErrMissing) {
		t.Errorf("errors.Is(fmt.Errorf(\"...: %%w\", fe), ErrMissing) = false, want true")
	}
	var got *deploynote.FieldError
	if !errors.As(wrapped, &got) {
		t.Fatalf("errors.As(fmt.Errorf(\"...: %%w\", fe), &got) = false, want true")
	}
	if got != fe {
		t.Errorf("errors.As found %+v, want the same *FieldError %+v", got, fe)
	}
	if want := "deploy of alert-rules refused: Rollback: missing"; wrapped.Error() != want {
		t.Errorf("wrapped.Error() = %q, want %q", wrapped.Error(), want)
	}
}
