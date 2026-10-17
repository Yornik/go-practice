package retry_test

import (
	"errors"
	"fmt"
	"testing"
	"time"

	retry "github.com/yornik/go-practice/exercises/07-retry"
)

// Step 3: the error types. Do uses them in step 4.

func TestErrorsPermanent(t *testing.T) {
	defer catchPanic(t)
	if got := retry.Permanent(nil); got != nil {
		t.Errorf("Permanent(nil) = %v, want nil", got)
	}

	err := retry.Permanent(errBadRequest)
	if err == nil {
		t.Fatalf("Permanent(errBadRequest) = nil, want an error")
	}
	if got, want := err.Error(), errBadRequest.Error(); got != want {
		t.Errorf("Permanent(errBadRequest).Error() = %q, want %q", got, want)
	}
	if !errors.Is(err, errBadRequest) {
		t.Errorf("errors.Is(Permanent(errBadRequest), errBadRequest) = false, want true")
	}
}

func TestErrorsRetryAfterError(t *testing.T) {
	defer catchPanic(t)
	err := &retry.RetryAfterError{Err: errUnavail, After: 2 * time.Second}
	if got, want := err.Error(), "503 Service Unavailable (retry after 2s)"; got != want {
		t.Errorf("RetryAfterError{Err: %q, After: 2s}.Error() = %q, want %q", errUnavail, got, want)
	}
	err = &retry.RetryAfterError{Err: errUnavail, After: 1500 * ms}
	if got, want := err.Error(), "503 Service Unavailable (retry after 1.5s)"; got != want {
		t.Errorf("RetryAfterError{Err: %q, After: 1.5s}.Error() = %q, want %q", errUnavail, got, want)
	}
	if got := err.Unwrap(); got != errUnavail {
		t.Errorf("RetryAfterError.Unwrap() = %v, want Err (%v)", got, errUnavail)
	}
	// Wrapped by op, as real code does: errors.Is and errors.As see through it.
	wrapped := fmt.Errorf("GET /api/v1/apps: %w", err)
	if !errors.Is(wrapped, errUnavail) {
		t.Errorf("errors.Is(wrapped RetryAfterError, Err) = false, want true")
	}
	var ra *retry.RetryAfterError
	if !errors.As(wrapped, &ra) || ra.After != 1500*ms {
		t.Errorf("errors.As(wrapped, *RetryAfterError) did not find After = 1.5s")
	}
}

func TestErrorsExhaustedError(t *testing.T) {
	defer catchPanic(t)
	err := &retry.ExhaustedError{Attempts: 3, Last: errRefused}
	if got, want := err.Error(), "giving up after 3 attempts: connection refused"; got != want {
		t.Errorf("ExhaustedError{Attempts: 3, Last: %q}.Error() = %q, want %q", errRefused, got, want)
	}
	if got := err.Unwrap(); got != errRefused {
		t.Errorf("ExhaustedError.Unwrap() = %v, want Last (%v)", got, errRefused)
	}
	if !errors.Is(err, errRefused) {
		t.Errorf("errors.Is(ExhaustedError, Last) = false, want true")
	}

	// The last attempt asked for a delay: the caller can still find out.
	last := &retry.RetryAfterError{Err: errUnavail, After: time.Minute}
	err = &retry.ExhaustedError{Attempts: 5, Last: last}
	var ra *retry.RetryAfterError
	if !errors.As(err, &ra) || ra != last {
		t.Errorf("errors.As(ExhaustedError{Last: *RetryAfterError}, *RetryAfterError) did not find Last")
	}
	if !errors.Is(err, errUnavail) {
		t.Errorf("errors.Is(ExhaustedError{Last: RetryAfterError{Err: errUnavail}}, errUnavail) = false, want true")
	}
}
