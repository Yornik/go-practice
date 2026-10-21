package envconfig_test

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	envconfig "github.com/yornik/go-practice/exercises/31-envconfig"
)

// These tests build FieldError values by hand, so you can write Error and
// Unwrap before Load.

func TestFieldErrorMessage(t *testing.T) {
	tests := []struct {
		name string
		fe   envconfig.FieldError
		want string
	}{
		{
			name: "bad value",
			fe:   envconfig.FieldError{Field: "Port", Key: "PORT", Value: "80a", Err: errors.New("invalid syntax")},
			want: `PORT="80a": invalid syntax`,
		},
		{
			// A Secret written with `echo pass | base64` ends in a newline.
			// %q makes it visible.
			name: "value with a trailing newline is quoted",
			fe:   envconfig.FieldError{Field: "ReadTimeout", Key: "READ_TIMEOUT", Value: "5s\n", Err: errors.New("bad duration")},
			want: `READ_TIMEOUT="5s\n": bad duration`,
		},
		{
			name: "no value: missing required variable",
			fe:   envconfig.FieldError{Field: "DB.URL", Key: "DB_URL", Err: envconfig.ErrRequired},
			want: "DB_URL: required variable is not set",
		},
		{
			name: "no value: unsupported type",
			fe: envconfig.FieldError{Field: "Labels", Key: "LABELS",
				Err: fmt.Errorf("%w: map[string]string", envconfig.ErrUnsupportedType)},
			want: "LABELS: unsupported field type: map[string]string",
		},
		{
			// The cause quotes the value, as strconv and url.Parse do. Neither
			// Value nor the cause's message may appear.
			name: "secret with a value: value and cause hidden",
			fe: envconfig.FieldError{Field: "DB.Password", Key: "DB_PASSWORD", Value: "hunter2", Secret: true,
				Err: fmt.Errorf("parsing %q: invalid syntax", "hunter2")},
			want: "DB_PASSWORD: invalid value (secret, not shown)",
		},
		{
			name: "secret without a value: nothing to hide",
			fe:   envconfig.FieldError{Field: "APIToken", Key: "API_TOKEN", Secret: true, Err: envconfig.ErrRequired},
			want: "API_TOKEN: required variable is not set",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			if got := tc.fe.Error(); got != tc.want {
				t.Errorf("(&FieldError{Key: %q, Value: %q, Secret: %v, Err: %q}).Error()\n got %q\nwant %q",
					tc.fe.Key, tc.fe.Value, tc.fe.Secret, tc.fe.Err, got, tc.want)
			}
		})
	}
}

func TestFieldErrorUnwrap(t *testing.T) {
	t.Run("returns Err", func(t *testing.T) {
		defer catchPanic(t)
		fe := &envconfig.FieldError{Key: "DB_URL", Err: envconfig.ErrRequired}
		if got := fe.Unwrap(); got != envconfig.ErrRequired {
			t.Errorf("Unwrap() = %v, want ErrRequired", got)
		}
		if !errors.Is(fe, envconfig.ErrRequired) {
			t.Errorf("errors.Is(fieldErr, ErrRequired) = false, want true")
		}
	})

	t.Run("secret: only the message is masked, the chain is intact", func(t *testing.T) {
		defer catchPanic(t)
		const secret = "postgres://app:hunter2@db:5432x/orders"
		// What url.Parse(secret) returns: its message quotes the input.
		cause := &url.Error{Op: "parse", URL: secret, Err: errors.New(`invalid port ":5432x" after host`)}
		fe := &envconfig.FieldError{Key: "DATABASE_URL", Value: secret, Secret: true, Err: cause}
		if strings.Contains(fe.Error(), "hunter2") {
			t.Errorf("Error() = %q, leaks the secret", fe.Error())
		}
		var urlErr *url.Error
		if !errors.As(fe, &urlErr) {
			t.Errorf("errors.As(fieldErr, &urlErr) = false, want true: Unwrap must return Err for secret fields too")
		}
	})
}
