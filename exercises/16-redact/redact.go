// Package redact keeps secrets out of logs, error messages and JSON.
//
// A Secret holds a value such as a database password. Whatever the code
// around it does (print it with fmt, log it with log/slog, encode it with
// encoding/json), the output shows Redacted instead of the value. Only
// Reveal returns the value.
//
// RedactURL hides the password and the secret query parameters of a URL,
// for example a database URL in a startup log line.
package redact

// Redacted is what every printer and encoder shows instead of the value of a
// Secret.
const Redacted = "[REDACTED]"

// Secret is a string that refuses to print.
//
// For a Secret and a *Secret, the fmt verbs %v, %+v, %s and %#v and the
// fmt Print functions show exactly Redacted, %q shows Redacted in quotes,
// and %x and %X show the hex of Redacted. fmt hides a Secret in an exported
// struct field, a slice or a map value the same way. encoding/json encodes a
// Secret as the JSON string "[REDACTED]" and decodes a JSON string into a
// Secret as the value. In log/slog, Value.Resolve turns a Secret into the
// string value Redacted.
//
// You add the methods that make fmt, encoding/json and log/slog do this. The
// README lists every rule.
type Secret string

// Reveal returns the real value. Call it only in the 1 place that needs the
// value, such as the call that opens the database connection.
func (s Secret) Reveal() string {
	panic("TODO: implement")
}

// RedactURL returns raw with its secrets hidden, so that it is safe to log.
//
// A password in the user info becomes "xxxxx". The values of the query
// parameters named password, token, secret, key or sig, in any letter case,
// become "xxxxx", and RedactURL then encodes the query with url.Values.Encode.
// When raw has no secret, RedactURL returns raw unchanged. When url.Parse or
// url.ParseQuery rejects raw, it returns "[invalid URL]" and never raw.
func RedactURL(raw string) string {
	panic("TODO: implement")
}
