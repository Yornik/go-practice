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
		"LOG_LEVEL":          "info",
		"DB_HOST":            "db-1.peckish.example",
		"DB_PORT":            "5432",
		"DB_USER":            "photos",
		"DB_PASSWORD":        "pa$$w0rd",
		"DB_OPTIONS":         "sslmode=require connect_timeout=5",
		"PHOTO_STORES":       "photo-store-1,photo-store-2,photo-store-3,photo-store-4",
		"UPLOAD_MAX_MB":      "25",
		"TMP_DIR":            "${HOME}/tmp",
		"LABELLER_URL":       "http://labeller.peckish.example:9000/v1/label",
		"PUSH_TEXT":          "A {species} is eating your {seed}.",
		"PUSH_ICON_COLOR":    "#f2b134",
		"STATUS_PAGE":        "https://status.peckish.example/#photos",
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
	broken := strings.Join(slices.Insert(lines, 12, "DB_REPLICA db-2.peckish.example\r\n"), "")
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
	if want := `"DB_REPLICA db-2.peckish.example"`; !strings.Contains(msg, want) {
		t.Errorf("Parse(prod.env with a broken line 13) error = %q, want it to contain %s", msg, want)
	}
}

func ExampleParse() {
	text := "# upload service\n" +
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
