package loglevels_test

import (
	"maps"
	"testing"

	loglevels "github.com/yornik/go-practice/exercises/01-loglevels"
)

type fieldsCase struct {
	name string
	line string
	want map[string]string
}

// runFieldsCases checks that Fields returns exactly the wanted entries. A nil
// and an empty map count as equal.
func runFieldsCases(t *testing.T, cases []fieldsCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := loglevels.Fields(tc.line)
			if !maps.Equal(got, tc.want) {
				t.Errorf("Fields(%q)\n got  %q\n want %q", tc.line, got, tc.want)
			}
		})
	}
}

func TestFieldsWords(t *testing.T) {
	runFieldsCases(t, []fieldsCase{
		{"1 pair", `level=error`, map[string]string{"level": "error"}},
		{
			"many pairs",
			`ts=2026-09-01T10:00:03Z service=gateway level=ERROR store=chkpt-store-2`,
			map[string]string{"ts": "2026-09-01T10:00:03Z", "service": "gateway", "level": "ERROR", "store": "chkpt-store-2"},
		},
		{"bare key", `boot`, map[string]string{"boot": ""}},
		{"bare key between pairs", `lvl=info boot drv=2.9.3`, map[string]string{"lvl": "info", "boot": "", "drv": "2.9.3"}},
		{"key= is an empty value", `level= msg=x`, map[string]string{"level": "", "msg": "x"}},
		{"key= at the end of the line", `msg=x level=`, map[string]string{"msg": "x", "level": ""}},
		// Only the first = separates the key from the value.
		{"= inside a value", `query=a=b=c`, map[string]string{"query": "a=b=c"}},
		{"value of only =", `sep==`, map[string]string{"sep": "="}},
		{"word that starts with = is ignored", `=x level=info`, map[string]string{"level": "info"}},
		{"lone = is ignored", `level=info = msg=ok`, map[string]string{"level": "info", "msg": "ok"}},
		{"==x is ignored", `==x lvl=err`, map[string]string{"lvl": "err"}},
		{"duplicate key: last wins", `level=info msg=a level=error`, map[string]string{"level": "error", "msg": "a"}},
		{"duplicate key: last wins over a bare key", `retry retry=3`, map[string]string{"retry": "3"}},
		{"duplicate key: a bare key last wins too", `retry=3 retry`, map[string]string{"retry": ""}},
		{"keys are case-sensitive", `level=info LEVEL=error Level=warn`, map[string]string{"level": "info", "LEVEL": "error", "Level": "warn"}},
		// Quotes are ordinary characters here; quoted values are 26-logfmt's job.
		{"quotes are ordinary characters", `msg="disk full" level=warn`, map[string]string{"msg": `"disk`, `full"`: "", "level": "warn"}},
		{"UTF-8 is kept as is", `customer=Pékség emoji=🌙`, map[string]string{"customer": "Pékség", "emoji": "🌙"}},
	})
}

func TestFieldsWhitespace(t *testing.T) {
	runFieldsCases(t, []fieldsCase{
		{"empty line", ``, nil},
		{"only spaces", `    `, nil},
		{"only spaces and tabs", " \t \t ", nil},
		{"runs of spaces between words", `level=warn     msg=slow`, map[string]string{"level": "warn", "msg": "slow"}},
		{"tab between words", "level=warn\tmsg=slow", map[string]string{"level": "warn", "msg": "slow"}},
		{"leading and trailing whitespace", "  \t level=warn msg=slow \t  ", map[string]string{"level": "warn", "msg": "slow"}},
		{"Windows line ending", "level=warn msg=slow\r", map[string]string{"level": "warn", "msg": "slow"}},
		{"newline between words", "level=warn\nmsg=slow", map[string]string{"level": "warn", "msg": "slow"}},
	})
}
