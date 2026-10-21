// Package pushtext prepares text for push notifications: it cuts text to a
// limit in bytes or in runes without breaking a character, cleans text that
// is not valid UTF-8, and turns a byte offset into a line and a column.
//
// Everywhere in this package, a byte that is not part of a valid UTF-8
// encoding counts as a rune of 1 byte, the way a for range loop over a string
// sees it.
package pushtext

// Ellipsis marks a text that was cut. It is 1 rune and 3 bytes in UTF-8.
const Ellipsis = "…"

// TruncateBytes returns s cut to at most limit bytes.
//
// If s has at most limit bytes, it returns s unchanged. Otherwise it returns
// the longest prefix of whole runes that leaves room for Ellipsis, followed by
// Ellipsis. When limit is 1 or 2 there is no room for Ellipsis: it returns the
// longest prefix of whole runes that fits in limit bytes. When limit <= 0 it
// returns "".
//
// It never cuts a rune in half. An invalid byte counts as a rune of 1 byte
// and is kept as it is.
func TruncateBytes(s string, limit int) string {
	panic("TODO: implement")
}

// TruncateRunes returns s cut to at most limit runes.
//
// If s has at most limit runes, it returns s unchanged. Otherwise it returns
// the first limit-1 runes followed by Ellipsis. When limit <= 0 it returns "".
//
// A combining mark, such as U+0301 in "e\u0301", is a rune of its own. An
// invalid byte counts as a rune of 1 byte and is kept as it is.
func TruncateRunes(s string, limit int) string {
	panic("TODO: implement")
}

// Clean returns s as valid UTF-8 without control characters.
//
// Each run of invalid bytes becomes 1 U+FFFD (the replacement character).
// The control characters U+0000 to U+001F and U+007F are removed, except tab
// (U+0009) and newline (U+000A). Every other rune is kept, including a valid
// U+FFFD that was already in s.
func Clean(s string) string {
	panic("TODO: implement")
}

// Position returns the 1-based line and the 1-based rune column of the byte
// at offset in s.
//
// Lines end at "\n" only; a "\r" is an ordinary character. The "\n" itself is
// the last character of its line. An offset of len(s) is the position right
// after the last character.
//
// ok is false, and line and col are 0, when offset is below 0, above len(s),
// or inside a rune of more than 1 byte.
func Position(s string, offset int) (line, col int, ok bool) {
	panic("TODO: implement")
}
