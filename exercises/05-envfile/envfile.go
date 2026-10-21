// Package envfile reads .env files as data, never as code:
//
//	# inference gateway
//	export PORT=8080
//	LOG_LEVEL=info # quiet
//	DB_PASSWORD=pa$$w0rd
//
// gives PORT=8080, LOG_LEVEL=info and DB_PASSWORD=pa$$w0rd, with the "$$"
// kept as written.
package envfile

// ValidKey reports whether key is a valid variable name: an ASCII letter or _,
// then ASCII letters, digits or _.
//
// So "PORT", "_x" and "db_host2" are valid, and "", "2FA", "db-host",
// "DB HOST" and "ключ" are not.
func ValidKey(key string) bool {
	panic("TODO: implement")
}

// Parse reads the content of a .env file and returns its variables.
//
// It skips blank lines and lines whose first non-blank byte is "#". Every
// other line is KEY=value, with an optional "export " before the key. A blank
// is a space or a tab. An unquoted value ends before a "#" that follows a
// blank. A value in '...' or "..." is taken verbatim, without escapes. "$" is
// never expanded. The last definition of a key wins. The README lists the
// exact rules.
//
// On success the map is not nil. At the first problem Parse returns a nil map
// and an error that starts with "line N: " (N counts from 1, skipped lines
// included) and contains the offending line or key quoted with %q.
func Parse(text string) (map[string]string, error) {
	panic("TODO: implement")
}
