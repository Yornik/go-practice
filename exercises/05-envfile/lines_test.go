package envfile_test

import (
	"testing"

	envfile "github.com/yornik/go-practice/exercises/05-envfile"
)

type parseCase struct {
	name string
	text string
	want map[string]string
}

// runParseCases checks that Parse accepts every text and returns exactly the
// wanted variables in a non-nil map.
func runParseCases(t *testing.T, cases []parseCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := envfile.Parse(tc.text)
			if err != nil {
				t.Fatalf("Parse(%q) error = %v, want nil", tc.text, err)
			}
			if got == nil {
				t.Fatalf("Parse(%q) = nil map, want a non-nil map", tc.text)
			}
			checkVars(t, tc.text, got, tc.want)
		})
	}
}

// checkVars reports every key that is missing, extra or different, so that 1
// run shows all the differences at once.
func checkVars(t *testing.T, text string, got, want map[string]string) {
	t.Helper()
	for k, w := range want {
		g, ok := got[k]
		switch {
		case !ok:
			t.Errorf("Parse(%q): key %q is missing, want value %q", text, k, w)
		case g != w:
			t.Errorf("Parse(%q): key %q = %q, want %q", text, k, g, w)
		}
	}
	for k, g := range got {
		if _, ok := want[k]; !ok {
			t.Errorf("Parse(%q): unexpected key %q with value %q", text, k, g)
		}
	}
}

func TestParseLines(t *testing.T) {
	runParseCases(t, []parseCase{
		{"1 line", "PORT=8080", map[string]string{"PORT": "8080"}},
		{"several lines", "A=1\nB=2\nC=3\n", map[string]string{"A": "1", "B": "2", "C": "3"}},
		{"no final newline", "A=1\nB=2", map[string]string{"A": "1", "B": "2"}},
		{"empty text", "", map[string]string{}},
		{"only newlines", "\n\n\n", map[string]string{}},
		{"blank lines", "\n\nA=1\n\n\nB=2\n\n", map[string]string{"A": "1", "B": "2"}},
		{"blank-only lines", "  \n\t\nA=1\n \t \n", map[string]string{"A": "1"}},
		{"only comment lines", "# settings\n# more settings\n", map[string]string{}},
		{
			"comment lines",
			"# settings\nA=1\n   # indented comment\n\t#after a tab\n#B=2\n",
			map[string]string{"A": "1"},
		},
		{"lowercase keys", "db_host=db-1\n_x=y", map[string]string{"db_host": "db-1", "_x": "y"}},
		{"blanks before the key", "   A=1\n\tB=2", map[string]string{"A": "1", "B": "2"}},
		{"blanks around =", "A = 1\n  B\t=\t2  \nC  =3\nD=  4", map[string]string{"A": "1", "B": "2", "C": "3", "D": "4"}},
		{"value with blanks inside", "MSG=hello world\nPATHS=a  b\tc", map[string]string{"MSG": "hello world", "PATHS": "a  b\tc"}},
		{"export and a space", "export A=1", map[string]string{"A": "1"}},
		{"export and a tab", "export\tB=2", map[string]string{"B": "2"}},
		{"export and several blanks", "export  \t C=3", map[string]string{"C": "3"}},
		{"blanks before export", "  export D=4", map[string]string{"D": "4"}},
		{"export with blanks around =", "export E = 5", map[string]string{"E": "5"}},
		{"export= is a key", "export=1", map[string]string{"export": "1"}},
		{"exported is a key", "exported=1", map[string]string{"exported": "1"}},
		{"export as the value", "MODE=export", map[string]string{"MODE": "export"}},
		{"CRLF line endings", "A=1\r\nB=2\r\n", map[string]string{"A": "1", "B": "2"}},
		{"CRLF blank line", "A=1\r\n\r\nB=2\r\n", map[string]string{"A": "1", "B": "2"}},
		{"CRLF comment line", "# note\r\nA=1\r\n", map[string]string{"A": "1"}},
		{"CRLF without final newline", "A=1\r\nB=2", map[string]string{"A": "1", "B": "2"}},
		{"empty value", "A=", map[string]string{"A": ""}},
		{"empty value with blanks", "A=   \nB=\t", map[string]string{"A": "", "B": ""}},
		{"empty value with CRLF", "A=\r\n", map[string]string{"A": ""}},
		// Only the first = separates the key from the value.
		{"= in the value", "A==b", map[string]string{"A": "=b"}},
		{"several = in the value", "URL=http://x/?a=1&b=2", map[string]string{"URL": "http://x/?a=1&b=2"}},
		{"value of only =", "SEP==", map[string]string{"SEP": "="}},
		{"last definition wins", "A=1\nB=2\nA=3", map[string]string{"A": "3", "B": "2"}},
		{"last definition wins over export", "export A=1\nA=2", map[string]string{"A": "2"}},
		{"last definition wins with an empty value", "A=1\nA=", map[string]string{"A": ""}},
		{"keys are case-sensitive", "port=1\nPORT=2", map[string]string{"port": "1", "PORT": "2"}},
		{
			"mixed",
			"# inference gateway\r\nexport PORT=8080\r\n\r\n  DB_HOST = db-1.vesper-labs.example \r\nMSG=two words\r\n",
			map[string]string{"PORT": "8080", "DB_HOST": "db-1.vesper-labs.example", "MSG": "two words"},
		},
	})
}
