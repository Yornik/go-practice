package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRot13RoundTrip(t *testing.T) {
	in := "func main() { fmt.Println(\"Hello, 世界!\") } // 42"
	enc := rot13(in)
	if enc == in {
		t.Fatalf("rot13(%q) did not change the input", in)
	}
	if got := rot13(enc); got != in {
		t.Fatalf("rot13(rot13(x)) = %q, want %q", got, in)
	}
	if got := rot13("Hello"); got != "Uryyb" {
		t.Fatalf("rot13(%q) = %q, want %q", "Hello", got, "Uryyb")
	}
}

func TestSection(t *testing.T) {
	text := "# Hints\n\n## Hint 1\none\n\n## Hint 2\ntwo\n## Hint 3\nthree\n"
	tests := map[string]string{
		"## Hint 1": "## Hint 1\none",
		"## Hint 2": "## Hint 2\ntwo",
		"## Hint 3": "## Hint 3\nthree",
	}
	for heading, want := range tests {
		got, err := section(text, heading)
		if err != nil {
			t.Fatalf("section(%q): %v", heading, err)
		}
		if got != want {
			t.Errorf("section(%q) = %q, want %q", heading, got, want)
		}
	}
	if _, err := section(text, "## Hint 4"); err == nil {
		t.Error("section for a missing heading: got nil error")
	}
}

func TestRun(t *testing.T) {
	dir := t.TempDir()
	ex := filepath.Join(dir, solutionsDir, "07-demo")
	if err := os.MkdirAll(ex, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, plain string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(ex, name), []byte(rot13(plain)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("hints.md.rot13", "# Hints\n\n## Hint 1\nLook at the map.\n\n## Hint 2\nSort it.\n\n## Hint 3\nDone.\n")
	write("explanation.md.rot13", "Explanation text.\n")
	write("demo.go.rot13", "package demo\n")

	t.Chdir(dir)

	tests := []struct {
		args []string
		want string
	}{
		{[]string{"7", "hint", "2"}, "## Hint 2\nSort it.\n"},
		{[]string{"07", "explanation"}, "Explanation text.\n"},
		{[]string{"07-demo", "solution"}, "// ===== demo.go =====\npackage demo\n\n"},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		if err := run(tt.args, &out); err != nil {
			t.Fatalf("run(%v): %v", tt.args, err)
		}
		if out.String() != tt.want {
			t.Errorf("run(%v) printed %q, want %q", tt.args, out.String(), tt.want)
		}
	}

	for _, args := range [][]string{{"8", "hint", "1"}, {"7", "hint", "x"}, {"7", "nope"}, {"7"}} {
		if err := run(args, &bytes.Buffer{}); err == nil {
			t.Errorf("run(%v): got nil error", args)
		} else if strings.Contains(err.Error(), "panic") {
			t.Errorf("run(%v): unexpected error %v", args, err)
		}
	}
}
