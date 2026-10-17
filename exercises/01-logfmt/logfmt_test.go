package logfmt_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	logfmt "github.com/yornik/go-practice/exercises/01-logfmt"
)

// pairs builds a []logfmt.Pair from alternating keys and values:
// pairs("a", "1", "b", "2") is [{a 1} {b 2}].
func pairs(kv ...string) []logfmt.Pair {
	if len(kv)%2 != 0 {
		panic("pairs: odd number of arguments")
	}
	var out []logfmt.Pair
	for i := 0; i < len(kv); i += 2 {
		out = append(out, logfmt.Pair{Key: kv[i], Value: kv[i+1]})
	}
	return out
}

type parseCase struct {
	name string
	line string
	want []logfmt.Pair
}

// runParseCases checks that Parse accepts every line and returns exactly the
// wanted pairs, in order. A nil and an empty slice count as equal.
func runParseCases(t *testing.T, cases []parseCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := logfmt.Parse(tc.line)
			if err != nil {
				// %T helps when err holds a nil *SyntaxError: it prints
				// "<nil>" but is still a non-nil error interface.
				t.Fatalf("Parse(%q) error = %v (type %T), want nil", tc.line, err, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("Parse(%q)\n got  %q\n want %q", tc.line, got, tc.want)
			}
		})
	}
}

func TestParseBareValues(t *testing.T) {
	runParseCases(t, []parseCase{
		{"single pair", `a=1`, pairs("a", "1")},
		{"several pairs", `a=1 b=2 c=3`, pairs("a", "1", "b", "2", "c", "3")},
		{"key= is an empty value", `a= b=2`, pairs("a", "", "b", "2")},
		{"key= at end of line", `a=1 b=`, pairs("a", "1", "b", "")},
		{"bare key has empty value", `debug`, pairs("debug", "")},
		{"bare key between pairs", `a=1 verbose b=2`, pairs("a", "1", "verbose", "", "b", "2")},
		// Only the first = separates key and value.
		{"= inside a bare value", `filter=a=b`, pairs("filter", "a=b")},
		// Escapes mean something only inside quotes.
		{"backslash in a bare value is literal", `path=C:\temp`, pairs("path", `C:\temp`)},
		{
			"keys with dots, dashes and slashes",
			`http.status_code=200 k8s-pod=api-7f9c app.kubernetes.io/name=api`,
			pairs("http.status_code", "200", "k8s-pod", "api-7f9c", "app.kubernetes.io/name", "api"),
		},
		{"duplicate keys are kept in order", `a=1 b=2 a=3`, pairs("a", "1", "b", "2", "a", "3")},
		{
			"UTF-8 in keys and values is kept as is",
			`user=zoë city=São ключ=значение emoji=🚀`,
			pairs("user", "zoë", "city", "São", "ключ", "значение", "emoji", "🚀"),
		},
	})
}

func TestParseQuotedValues(t *testing.T) {
	runParseCases(t, []parseCase{
		{"spaces inside quotes", `msg="db timeout"`, pairs("msg", "db timeout")},
		{"empty quoted value", `msg=""`, pairs("msg", "")},
		{"= and spaces inside quotes", `q="a=1 b=2"`, pairs("q", "a=1 b=2")},
		{"quoted value followed by more pairs", `a="x y" b=2`, pairs("a", "x y", "b", "2")},
		{"escaped quote", `msg="say \"hi\""`, pairs("msg", `say "hi"`)},
		{"escaped backslash", `path="C:\\temp"`, pairs("path", `C:\temp`)},
		// The \\ is one escaped backslash, so the " after it closes the value.
		{"escaped backslash right before the closing quote", `dir="C:\\" next=1`, pairs("dir", `C:\`, "next", "1")},
		{"newline and tab escapes", `stack="line1\nline2\tcol"`, pairs("stack", "line1\nline2\tcol")},
		{"a raw tab inside quotes is kept", "msg=\"a\tb\"", pairs("msg", "a\tb")},
		{"UTF-8 inside quotes", `msg="café ☕ ok"`, pairs("msg", "café ☕ ok")},
	})
}

func TestParseWhitespace(t *testing.T) {
	runParseCases(t, []parseCase{
		{"empty line", ``, nil},
		{"only spaces", `   `, nil},
		{"only spaces and tabs", " \t \t", nil},
		{"many spaces between pairs", `a=1     b=2`, pairs("a", "1", "b", "2")},
		{"tab between pairs", "a=1\tb=2", pairs("a", "1", "b", "2")},
		{"leading and trailing whitespace", "  \t a=1 b=2 \t  ", pairs("a", "1", "b", "2")},
		{"bare key before trailing spaces", `a=1 debug   `, pairs("a", "1", "debug", "")},
		// A tab must end a key, a bare value, a quoted value and an empty value.
		{
			"a tab works wherever a space does",
			"\ta=1\tverbose\tmsg=\"x y\"\tb=\tc=3",
			pairs("a", "1", "verbose", "", "msg", "x y", "b", "", "c", "3"),
		},
		// Only space and tab separate pairs. Other bytes are data, also the
		// \r of a Windows line ending and a non-breaking space (bytes C2 A0).
		{"carriage return is not whitespace", "a=x\ry", pairs("a", "x\ry")},
		{"non-breaking space is not whitespace", "msg=a b", pairs("msg", "a b")},
	})
}

// Lines as real tools write them.
func TestParseRealLines(t *testing.T) {
	runParseCases(t, []parseCase{
		{
			"service log line",
			`ts=2026-10-16T12:00:00Z level=error msg="db timeout" latency_ms=532`,
			pairs("ts", "2026-10-16T12:00:00Z", "level", "error", "msg", "db timeout", "latency_ms", "532"),
		},
		{
			"Heroku router line",
			`at=info method=GET path="/" host=my-app.example.com status=200 bytes=1548`,
			pairs("at", "info", "method", "GET", "path", "/", "host", "my-app.example.com", "status", "200", "bytes", "1548"),
		},
		{
			"Go log/slog TextHandler line",
			`time=2026-10-16T12:00:00.000Z level=WARN msg="slow query" query="SELECT 1" path=C:\temp`,
			pairs("time", "2026-10-16T12:00:00.000Z", "level", "WARN", "msg", "slow query", "query", "SELECT 1", "path", `C:\temp`),
		},
	})
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name string
		line string
		col  int // 1-based byte column the *SyntaxError must report
	}{
		// Unterminated quoted value: the column of the opening quote.
		{"unterminated quoted value", `msg="db timeout`, 5},
		{"unterminated after other pairs", `a=1 msg="oops`, 9},
		{"escaped quote does not close the value", `msg="abc\"`, 5},
		{"backslash as the last byte", `msg="abc\`, 5},

		// Unknown escape: the column of the backslash.
		{"unknown escape", `msg="a\qb"`, 7},
		{"\\u escapes are not supported", `msg="caf\u00e9"`, 9},

		// = with no key in front of it: the column of the =.
		{"line starts with =", `=value`, 1},
		{"= with no key after a pair", `a=1 =2`, 5},
		{"lone = between pairs", `a=1 = b=2`, 5},
		{"leading spaces count in the column", `   =x`, 4},

		// A quote where a key byte is expected: the column of the quote.
		{"quoted key", `"msg"=hi`, 1},
		{"quote inside a key", `ke"y=v`, 3},

		// A quote inside a bare value: the column of the quote.
		{"quote inside a bare value", `msg=db"timeout`, 7},
		{"quote at the end of a bare value", `msg=timeout"`, 12},

		// After a closing quote only whitespace or end of line may follow:
		// the column of the first byte after the closing quote.
		{"letter right after the closing quote", `k="a"b`, 6},
		{"second quoted value glued on", `k="a""b"`, 6},
		{"= right after the closing quote", `k="a"=b`, 6},

		// Columns count bytes: é is 2 bytes in UTF-8, so the " is byte 14.
		{"column counts bytes, not runes", `msg=héllo k="a`, 14},
		// Parse stops at the first problem it meets, reading left to right.
		{"first error wins", `a="x"y b="unterminated`, 6},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := logfmt.Parse(tc.line)
			if err == nil {
				t.Fatalf("Parse(%q) = %q, nil; want a *SyntaxError at column %d", tc.line, got, tc.col)
			}
			var se *logfmt.SyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("Parse(%q) error = %v (type %T), want a *logfmt.SyntaxError", tc.line, err, err)
			}
			if se.Col != tc.col {
				t.Errorf("Parse(%q) error column = %d, want %d (error: %v)", tc.line, se.Col, tc.col, err)
			}
			if se.Msg == "" {
				t.Errorf("Parse(%q) error has an empty Msg, want a short description", tc.line)
			}
			if got != nil {
				t.Errorf("Parse(%q) = %q with an error, want nil pairs", tc.line, got)
			}
		})
	}
}

func TestSyntaxErrorMessage(t *testing.T) {
	err := &logfmt.SyntaxError{Col: 7, Msg: "unknown escape"}
	if got, want := err.Error(), "logfmt: column 7: unknown escape"; got != want {
		t.Errorf("SyntaxError{Col: 7, Msg: %q}.Error() = %q, want %q", err.Msg, got, want)
	}

	// The error from Parse uses the same format with your own Msg.
	line := `msg="oops`
	_, perr := logfmt.Parse(line)
	if perr == nil {
		t.Fatalf("Parse(%q) error = nil, want a *SyntaxError", line)
	}
	prefix := "logfmt: column 5: "
	if msg := perr.Error(); !strings.HasPrefix(msg, prefix) || len(msg) == len(prefix) {
		t.Errorf("Parse(%q) error message = %q, want %q followed by a description", line, msg, prefix)
	}
}

// escaper writes a value back in quoted form.
var escaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`)

// encode writes pairs back as a logfmt line with every value quoted.
func encode(ps []logfmt.Pair) string {
	parts := make([]string, 0, len(ps))
	for _, p := range ps {
		parts = append(parts, p.Key+`="`+escaper.Replace(p.Value)+`"`)
	}
	return strings.Join(parts, " ")
}

// FuzzParse checks properties that must hold for ANY input, not only for the
// table cases above. Plain `go test` runs it on the seed lines below. Run
//
//	go test -fuzz=FuzzParse ./exercises/01-logfmt/
//
// to let Go generate new inputs (stop it with Ctrl-C).
func FuzzParse(f *testing.F) {
	seeds := []string{
		``, `   `, `a=1`, `a= b=2`, `debug`, `filter=a=b`, `path=C:\temp`,
		`msg="db timeout" latency_ms=532`, `msg="say \"hi\""`, `dir="C:\\" next=1`,
		"msg=\"a\tb\"", "\ta=1\tmsg=\"x y\"\tb=", "a=x\ry", `user=zoë msg="café ☕"`,
		`msg="oops`, `msg="abc\`, `msg="a\qb"`, `=x`, `"k"=v`, `k=a"b`, `k="a"b`,
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, line string) {
		got, err := logfmt.Parse(line)
		if err != nil {
			var se *logfmt.SyntaxError
			if !errors.As(err, &se) {
				t.Fatalf("Parse(%q) error = %v (type %T), want a *logfmt.SyntaxError", line, err, err)
			}
			if se.Col < 1 || se.Col > len(line) {
				t.Fatalf("Parse(%q) error column = %d, want 1..%d (a byte of the line)", line, se.Col, len(line))
			}
			if got != nil {
				t.Fatalf("Parse(%q) = %q with an error, want nil pairs", line, got)
			}
			return
		}
		for _, p := range got {
			if p.Key == "" || strings.ContainsAny(p.Key, " \t=\"") {
				t.Fatalf("Parse(%q) returned key %q, want a non-empty key without space, tab, = or \"", line, p.Key)
			}
		}
		// Round trip: quoting every value and parsing again gives the same pairs.
		again := encode(got)
		got2, err := logfmt.Parse(again)
		if err != nil {
			t.Fatalf("Parse(%q) = %q, but parsing it re-encoded as %q fails: %v", line, got, again, err)
		}
		if !slices.Equal(got2, got) {
			t.Fatalf("Parse(%q) = %q, but re-encoded as %q it parses to %q", line, got, again, got2)
		}
	})
}

func ExampleParse() {
	ps, err := logfmt.Parse(`level=error msg="db timeout" retry`)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, p := range ps {
		fmt.Printf("%s=%q\n", p.Key, p.Value)
	}
	// Output:
	// level="error"
	// msg="db timeout"
	// retry=""
}
