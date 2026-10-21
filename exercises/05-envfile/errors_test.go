package envfile_test

import (
	"fmt"
	"strings"
	"testing"

	envfile "github.com/yornik/go-practice/exercises/05-envfile"
)

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		text string
		line int    // the line number the message must start with
		want string // the line or key the message must contain, formatted with %q
	}{
		{"no =", "DB_HOST localhost", 1, "DB_HOST localhost"},
		{"no = in a single word", "PORT", 1, "PORT"},
		{"no = after other lines", "A=1\nB=2\nBROKEN\n", 3, "BROKEN"},
		{"no = with CRLF", "A=1\r\nBROKEN\r\n", 2, "BROKEN"},
		{"export without =", "export DB_HOST", 1, "export DB_HOST"},
		{"export alone", "export", 1, "export"},
		{"key starts with a digit", "2FA_SECRET=x", 1, "2FA_SECRET"},
		{"key with a dash", "db-host=x", 1, "db-host"},
		{"key with a dash after export", "export db-host=x", 1, "db-host"},
		{"key with a space", "DB HOST=x", 1, "DB HOST"},
		{"key with a dot", "db.host=x", 1, "db.host"},
		{"empty key", "=x", 1, ""},
		{"empty key with blanks", "  = x", 1, ""},
		{"key outside ASCII", "ключ=1", 1, "ключ"},
		{"EXPORT in uppercase is part of the key", "EXPORT PORT=1", 1, "EXPORT PORT"},
		{"unterminated double quote", `DB_PASSWORD="pa$$w0rd`, 1, "DB_PASSWORD"},
		{"unterminated single quote", `A='x`, 1, "A"},
		{"lone opening quote", `A="`, 1, "A"},
		{"quote of the other kind does not close", `A="x'`, 1, "A"},
		{"unterminated quote with CRLF", "A=1\r\nB=\"x\r\n", 2, "B"},
		{"quoted value does not span lines", "CERT=\"-----BEGIN\nabc\n-----END\"\n", 1, "CERT"},
		{"text right after the closing quote", `A="x"y`, 1, "A"},
		{"word after the closing quote", `A='x' y`, 1, "A"},
		{"second quoted part", `A='a''b'`, 1, "A"},
		{"no escapes: \\\" closes the value", `A="say \"hi\""`, 1, "A"},
		{"first problem wins", "A=1\nB C\n=x\n", 2, "B C"},
		{
			"line numbers count skipped lines",
			"# database\n\nDB_HOST=db-1\n   \n# password\nDB_PASSWORD='open\n",
			6, "DB_PASSWORD",
		},
		{"error after many good lines", strings.Repeat("A=1\n", 99) + "B\n", 100, "B"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := envfile.Parse(tc.text)
			if err == nil {
				t.Fatalf("Parse(%q) error = nil, want an error on line %d", tc.text, tc.line)
			}
			if got != nil {
				t.Errorf("Parse(%q) = %q, want a nil map with the error", tc.text, got)
			}
			msg := err.Error()
			if prefix := fmt.Sprintf("line %d: ", tc.line); !strings.HasPrefix(msg, prefix) {
				t.Errorf("Parse(%q) error = %q, want it to start with %q", tc.text, msg, prefix)
			}
			if quoted := fmt.Sprintf("%q", tc.want); !strings.Contains(msg, quoted) {
				t.Errorf("Parse(%q) error = %q, want it to contain %s", tc.text, msg, quoted)
			}
		})
	}
}
