package envfile_test

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	envfile "github.com/yornik/go-practice/exercises/05-envfile"
)

func TestParseIncident(t *testing.T) {
	data, err := os.ReadFile("testdata/prod.env")
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	// The file must keep its Windows line endings; .gitattributes protects them.
	if !strings.Contains(text, "\r\n") {
		t.Fatal("testdata/prod.env has no CRLF line endings; check it out again without line-ending conversion")
	}
	got, err := envfile.Parse(text)
	if err != nil {
		t.Fatalf("Parse(testdata/prod.env) error = %v, want nil", err)
	}
	want := map[string]string{
		"APP_ENV":            "production",
		"PORT":               "8080",
		"HUSH_AUTONOMY":      "3",
		"DB_HOST":            "db-1.vesper-labs.example",
		"DB_PORT":            "5432",
		"DB_USER":            "gateway",
		"DB_PASSWORD":        "pa$$w0rd",
		"DB_OPTIONS":         "sslmode=require connect_timeout=5",
		"CKPT_STORES":        "chkpt-store-1,chkpt-store-2,chkpt-store-3,chkpt-store-4",
		"BATCH_MAX_MB":       "25",
		"TMP_DIR":            "${HOME}/tmp",
		"HUSH_URL":           "http://hush-api.vesper-labs.example:9000/v1/triage",
		"NOTIFY_TEXT":        "HUSH resolved {count} incidents while you slept.",
		"NOTIFY_ICON_COLOR":  "#7b61ff",
		"STATUS_PAGE":        "https://status.vesper-labs.example/#inference",
		"FEATURE_NIGHT_MODE": "false",
		"EMPTY_ON_PURPOSE":   "",
	}
	if got == nil {
		t.Fatal("Parse(testdata/prod.env) = nil map, want a non-nil map")
	}
	checkVars(t, "testdata/prod.env", got, want)
}

func TestParseIncidentBrokenLine(t *testing.T) {
	data, err := os.ReadFile("testdata/prod.env")
	if err != nil {
		t.Fatal(err)
	}
	// The same file after somebody pasted a line without "=" after line 12.
	lines := strings.SplitAfter(string(data), "\n")
	broken := strings.Join(slices.Insert(lines, 12, "DB_REPLICA db-2.vesper-labs.example\r\n"), "")
	got, err := envfile.Parse(broken)
	if err == nil {
		t.Fatalf("Parse(prod.env with a broken line 13) error = nil, want an error")
	}
	if got != nil {
		t.Errorf("Parse(prod.env with a broken line 13) = %q, want a nil map with the error", got)
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "line 13: ") {
		t.Errorf("Parse(prod.env with a broken line 13) error = %q, want it to start with %q", msg, "line 13: ")
	}
	if want := `"DB_REPLICA db-2.vesper-labs.example"`; !strings.Contains(msg, want) {
		t.Errorf("Parse(prod.env with a broken line 13) error = %q, want it to contain %s", msg, want)
	}
}

func ExampleParse() {
	text := "# Inference gateway, production.\n" +
		"export PORT=8080\n" +
		"LOG_LEVEL = info # quiet\n" +
		"\n" +
		"DB_PASSWORD=pa$$w0rd\n" +
		"COLOR=#ff0000\n" +
		"GREETING='Hello # world'\n"
	vars, err := envfile.Parse(text)
	if err != nil {
		fmt.Println(err)
		return
	}
	// A map has no order, so sort the keys to print them.
	for _, k := range slices.Sorted(maps.Keys(vars)) {
		fmt.Printf("%s=%q\n", k, vars[k])
	}

	_, err = envfile.Parse("PORT=8080\n\n# database\nDB_HOST localhost\n")
	fmt.Println(err != nil && strings.HasPrefix(err.Error(), "line 4: "))
	// Output:
	// COLOR="#ff0000"
	// DB_PASSWORD="pa$$w0rd"
	// GREETING="Hello # world"
	// LOG_LEVEL="info"
	// PORT="8080"
	// true
}
