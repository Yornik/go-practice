package pushtext_test

import (
	"testing"

	pushtext "github.com/yornik/go-practice/exercises/10-pushtext"
)

// runTruncateRunes checks TruncateRunes against every case. A combining mark
// looks the same under %q in both strings, so the message also prints them
// with %+q, which escapes every rune outside ASCII.
func runTruncateRunes(t *testing.T, cases []truncCase) {
	t.Helper()
	for _, tc := range cases {
		got := pushtext.TruncateRunes(tc.s, tc.limit)
		if got != tc.want {
			t.Errorf("TruncateRunes(%+q, %d)\n got  %q = %+q\n want %q = %+q",
				tc.s, tc.limit, got, got, tc.want, tc.want)
		}
	}
}

func TestTruncateRunes(t *testing.T) {
	runTruncateRunes(t, []truncCase{
		{"", 0, ""},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abc", 100, "abc"},
		{"abcd", 3, "ab…"},
		// The ellipsis is 1 rune, so the result has exactly limit runes.
		{"abcdefgh", 4, "abc…"},
		{"seeds…", 6, "seeds…"},
	})
}

func TestTruncateRunesMultiByte(t *testing.T) {
	runTruncateRunes(t, []truncCase{
		// 4 runes of 4 bytes each: 16 bytes, but 4 runes.
		{"🐦🐦🐦🐦", 4, "🐦🐦🐦🐦"},
		{"🐦🐦🐦🐦", 3, "🐦🐦…"},
		{"olé olé", 7, "olé olé"},
		{"olé olé", 4, "olé…"},
		{"olé olé", 3, "ol…"},
		{"A Wood Pigeon 🐦 is at your feeder", 16, "A Wood Pigeon 🐦…"},
	})
}

func TestTruncateRunesSmallMax(t *testing.T) {
	runTruncateRunes(t, []truncCase{
		{"a", 1, "a"},
		{"🐦", 1, "🐦"},
		{"ab", 1, "…"},
		{"🐦🐦", 1, "…"},
		{"a", 0, ""},
		{"abc", 0, ""},
		{"abc", -1, ""},
		{"🐦", -100, ""},
	})
}

func TestTruncateRunesCombining(t *testing.T) {
	runTruncateRunes(t, []truncCase{
		// "Café" with a precomposed é (U+00E9) is 4 runes.
		{"Café", 4, "Café"},
		// "Cafe\u0301" looks the same but is 5 runes: e and a combining
		// acute accent (U+0301).
		{"Cafe\u0301", 5, "Cafe\u0301"},
		{"Cafe\u0301", 4, "Caf…"},
		{"Cafe\u0301 au lait", 6, "Cafe\u0301…"},
		// The cut may separate a combining mark from its letter.
		{"Cafe\u0301 au lait", 5, "Cafe…"},
		// A thumbs up with a skin tone modifier is 2 runes.
		{"\U0001F44D\U0001F3FD", 2, "\U0001F44D\U0001F3FD"},
		{"\U0001F44D\U0001F3FD!", 2, "\U0001F44D…"},
	})
}

func TestTruncateRunesInvalid(t *testing.T) {
	runTruncateRunes(t, []truncCase{
		// An invalid byte is a rune of 1 byte, and it is kept as it is.
		{"\xff\xff", 2, "\xff\xff"},
		{"\xff\xff\xff", 2, "\xff…"},
		{"ab\xf0\x9f\x90", 4, "ab\xf0…"},
	})
}
