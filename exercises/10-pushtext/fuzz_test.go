package pushtext_test

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	pushtext "github.com/yornik/go-practice/exercises/10-pushtext"
)

// isCutOf reports whether got is a prefix of s, or a prefix of s followed by
// the ellipsis.
func isCutOf(got, s string) bool {
	if strings.HasPrefix(s, got) {
		return true
	}
	p, found := strings.CutSuffix(got, pushtext.Ellipsis)
	return found && strings.HasPrefix(s, p)
}

// FuzzTruncate checks properties that must hold for ANY text and limit, not
// only for the table cases. Plain `go test` runs it on the seeds below. Run
//
//	go test -fuzz=FuzzTruncate ./exercises/10-pushtext/
//
// to let Go generate new inputs (stop it with Ctrl-C).
func FuzzTruncate(f *testing.F) {
	f.Add("HUSH resolved 3 incidents while you slept 🌙 Have a calm day.", 45)
	f.Add("HUSH resolved 3 incidents while you slept 🌙 Have a calm day.", 49)
	f.Add("", 0)
	f.Add("abc", -1)
	f.Add("olé olé", 3)
	f.Add("olé olé", 6)
	f.Add("🌙🌙", 5)
	f.Add("Cafe\u0301 au lait", 5)
	f.Add("ovens…", 4)
	f.Add("\xff\x80abc", 2)
	f.Add("ab\xf0\x9f\x8ccd", 6)
	f.Add("a\x00b\r\n", 3)

	f.Fuzz(func(t *testing.T, s string, limit int) {
		b := pushtext.TruncateBytes(s, limit)
		switch {
		case len(s) <= limit && b != s:
			t.Fatalf("TruncateBytes(%q, %d) = %q, want s unchanged: it fits", s, limit, b)
		case limit <= 0 && b != "":
			t.Fatalf("TruncateBytes(%q, %d) = %q, want \"\"", s, limit, b)
		case limit > 0 && len(b) > limit:
			t.Fatalf("TruncateBytes(%q, %d) = %q, %d bytes, want at most %d", s, limit, b, len(b), limit)
		case utf8.ValidString(s) && !utf8.ValidString(b):
			t.Fatalf("TruncateBytes(%q, %d) = %q, want valid UTF-8 for a valid input", s, limit, b)
		case !isCutOf(b, s):
			t.Fatalf("TruncateBytes(%q, %d) = %q, want a prefix of s, maybe followed by %q", s, limit, b, pushtext.Ellipsis)
		}

		r := pushtext.TruncateRunes(s, limit)
		n := utf8.RuneCountInString(r)
		switch {
		case utf8.RuneCountInString(s) <= limit && r != s:
			t.Fatalf("TruncateRunes(%q, %d) = %q, want s unchanged: it fits", s, limit, r)
		case limit <= 0 && r != "":
			t.Fatalf("TruncateRunes(%q, %d) = %q, want \"\"", s, limit, r)
		case limit > 0 && n > limit:
			t.Fatalf("TruncateRunes(%q, %d) = %q, %d runes, want at most %d", s, limit, r, n, limit)
		case utf8.ValidString(s) && !utf8.ValidString(r):
			t.Fatalf("TruncateRunes(%q, %d) = %q, want valid UTF-8 for a valid input", s, limit, r)
		case !isCutOf(r, s):
			t.Fatalf("TruncateRunes(%q, %d) = %q, want a prefix of s, maybe followed by %q", s, limit, r, pushtext.Ellipsis)
		}
	})
}

func ExampleTruncateBytes() {
	s := "HUSH resolved 3 incidents while you slept 🌙 Have a calm day."
	fmt.Println(pushtext.TruncateBytes(s, 49))
	fmt.Println(pushtext.TruncateBytes(s, 48))
	fmt.Println(pushtext.TruncateBytes(pushtext.Clean("you\x00 slept \xf0\x9f\x8c"), 100))
	// Output:
	// HUSH resolved 3 incidents while you slept 🌙…
	// HUSH resolved 3 incidents while you slept …
	// you slept �
}
