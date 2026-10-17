// Package envconfig fills a configuration struct from environment variables,
// using struct tags for names, defaults, required fields and secrets. It
// reports every mistake in one error, and never prints a secret.
package envconfig

import "errors"

// ErrInvalidTarget is wrapped by the error Load returns when dst is not a
// non-nil pointer to a struct.
var ErrInvalidTarget = errors.New("envconfig: target must be a non-nil pointer to a struct")

// ErrRequired is the Err of a FieldError for a field with required:"true"
// that has no value: its variable is not set or is empty, and it has no
// default.
var ErrRequired = errors.New("required variable is not set")

// ErrUnsupportedType is wrapped by the Err of a FieldError for a field whose
// type Load cannot fill, such as a map.
var ErrUnsupportedType = errors.New("unsupported field type")

// FieldError describes one field that Load could not fill.
type FieldError struct {
	// Field is the Go path of the field from the root struct, for example
	// "Port" or "DB.Password".
	Field string
	// Key is the name of the environment variable, for example "DB_PASSWORD".
	Key string
	// Value is the raw text that failed to parse: the variable's value, or
	// the default tag if the variable had no value. It is "" for a missing
	// required value and for an unsupported type.
	Value string
	// Secret is true when the field has the tag secret:"true".
	Secret bool
	// Err is the cause: ErrRequired, an error that wraps ErrUnsupportedType,
	// or the error from the parser (strconv, time.ParseDuration, UnmarshalText).
	Err error
}

// Error returns a one-line message for an operator. It never contains Value
// when Secret is true, not even inside the message of Err. The README gives
// the exact format.
func (e *FieldError) Error() string {
	panic("TODO: implement")
}

// Unwrap returns Err, so that errors.Is and errors.As see the cause. It does
// this for secret fields too: only the message is masked.
func (e *FieldError) Unwrap() error {
	panic("TODO: implement")
}

// UpperSnake converts a Go field name to the default variable name of the
// field: "Port" → "PORT", "ReadTimeout" → "READ_TIMEOUT",
// "HTTPPort" → "HTTP_PORT". The README gives the exact rule.
func UpperSnake(name string) string {
	panic("TODO: implement")
}

// Load fills the exported fields of the struct that dst points to. It gets
// the value of each variable from lookup, which has the signature of
// os.LookupEnv; a nil lookup means os.LookupEnv.
//
// The variable name of a field comes from its env tag or from UpperSnake of
// its name, plus the prefix of the nested structs it is in. A value is parsed
// according to the field's type. The tags default, required, secret, prefix
// and env:"-" work as the README describes.
//
// If dst is not a non-nil pointer to a struct, Load returns an error that
// wraps ErrInvalidTarget. Otherwise it tries every field and returns all
// problems at once: errors.Join of one *FieldError per bad field, in field
// order. On success it returns nil.
func Load(dst any, lookup func(key string) (string, bool)) error {
	panic("TODO: implement")
}
