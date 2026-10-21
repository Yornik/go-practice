// Package loglevels counts and filters log lines by level when every service
// spells its levels its own way:
//
//	ts=2026-09-01T10:00:03Z service=upload level=ERROR msg=disk_full
//	ts=2026-09-01T10:00:04Z feeder=feeder-0412 lvl=err msg=camera_timeout
//	ts=2026-09-01T10:00:05Z service=labeller severity=critical msg=model_oom
//
// All 3 lines are at the level "error" or worse.
package loglevels

// Fields splits 1 line into its key=value words.
//
// Words are separated by whitespace, as strings.Fields splits them. Each word
// splits at its first "=": "a=b=c" is the key "a" with the value "b=c". A word
// without "=" is a key with the value "". A word that starts with "=" is
// ignored. Keys are case-sensitive, and the last occurrence of a key wins.
//
// A line without words gives no entries (a nil or an empty map).
func Fields(line string) map[string]string {
	panic("TODO: implement")
}

// Level returns the normalized level of a line: "debug", "info", "warn",
// "error", "fatal" or "unknown".
//
// It reads the value of the first of the keys "level", "lvl" and "severity"
// that is present, even when that value is unknown. If that key appears more
// than once, its last occurrence decides, as in Fields. The value matches in
// any letter case. The README lists the 15 known spellings. No key, an empty
// value or any other value gives "unknown".
func Level(line string) string {
	panic("TODO: implement")
}

// CountLevels counts the lines per normalized level, "unknown" included.
//
// It skips lines without words (empty or whitespace only). A line with words
// counts even when Fields gives no entries for it: "=x" is "unknown". It never
// returns nil. A level with 0 lines may be absent from the map or have the
// value 0.
func CountLevels(lines []string) map[string]int {
	panic("TODO: implement")
}

// AtLeast returns the lines whose level is minLevel or more severe, unchanged
// and in input order. The order is debug < info < warn < error < fatal. Lines
// with the level "unknown" are never returned.
//
// minLevel must be exactly 1 of "debug", "info", "warn", "error" and "fatal".
// For any other value AtLeast returns a nil slice and an error that contains
// minLevel quoted with %q, for example: unknown minimum level "WARNING".
func AtLeast(lines []string, minLevel string) ([]string, error) {
	panic("TODO: implement")
}
