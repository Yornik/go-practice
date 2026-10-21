package pushtext_test

import (
	"testing"

	pushtext "github.com/yornik/go-practice/exercises/10-pushtext"
)

type cleanCase struct {
	name string
	in   string
	want string
}

// runClean checks Clean against every case. %+q shows invalid bytes as \x
// escapes and every rune outside ASCII as \u, so a U+FFFD, a combining mark
// or a U+200B is visible in the message.
func runClean(t *testing.T, cases []cleanCase) {
	t.Helper()
	for _, tc := range cases {
		got := pushtext.Clean(tc.in)
		if got != tc.want {
			t.Errorf("%s: Clean(%+q)\n got  %+q\n want %+q", tc.name, tc.in, got, tc.want)
		}
	}
}

func TestCleanUnchanged(t *testing.T) {
	runClean(t, []cleanCase{
		{"empty", "", ""},
		{"ASCII", "All 6 ovens are at 230 degrees.", "All 6 ovens are at 230 degrees."},
		{"multi-byte runes", "olé 🌙 Café", "olé 🌙 Café"},
		{"tab and newline are kept", "line 1\tok\nline 2", "line 1\tok\nline 2"},
		{"combining mark", "Cafe\u0301", "Cafe\u0301"},
		{"a valid U+FFFD is kept", "a�b", "a�b"},
		// Only U+0000 to U+001F and U+007F are removed.
		{"U+0080 and U+0085 are kept", "a\u0080b\u0085c", "a\u0080b\u0085c"},
		{"zero width space is kept", "a\u200bb", "a\u200bb"},
	})
}

func TestCleanInvalid(t *testing.T) {
	runClean(t, []cleanCase{
		{"1 invalid byte", "a\xffb", "a�b"},
		{"only an invalid byte", "\xff", "�"},
		{"a run of 3 invalid bytes is 1 U+FFFD", "a\xff\xfe\xfdb", "a�b"},
		{"2 runs give 2 U+FFFD", "a\xffb\xffc", "a�b�c"},
		{"an emoji cut after 3 of its 4 bytes", "ab\xf0\x9f\x8c", "ab�"},
		{"an emoji cut after 2 bytes, then text", "\xf0\x9fab", "�ab"},
		{"lone continuation bytes", "\x80\x80\x80", "�"},
		{"overlong encoding of /", "a\xc0\xafb", "a�b"},
		{"encoded surrogate half", "a\xed\xa0\x80b", "a�b"},
		// The valid U+FFFD is a rune; only the byte after it is invalid.
		{"a valid U+FFFD before an invalid byte", "�\xff", "��"},
		{"valid runes between runs", "\xffé\xff", "�é�"},
	})
}

func TestCleanControl(t *testing.T) {
	runClean(t, []cleanCase{
		{"NUL", "a\x00b", "ab"},
		{"bell and backspace", "a\x07\x08b", "ab"},
		{"carriage return is removed", "line 1\r\nline 2\r\n", "line 1\nline 2\n"},
		{"DEL", "a\x7fb", "ab"},
		{"U+001F", "a\x1fb", "ab"},
		{"ANSI color codes lose their ESC", "\x1b[31mred\x1b[0m", "[31mred[0m"},
		{"only control characters", "\x00\x01\x02\x7f", ""},
		{"tab and newline among removed ones", "\x01\t\x02\n\x03", "\t\n"},
	})
}

func TestCleanMixed(t *testing.T) {
	runClean(t, []cleanCase{
		// Runs are counted in the input: the NUL between the 2 invalid bytes
		// ends the first run, so there are 2 runs, even though the NUL itself
		// is removed.
		{"a removed control character separates 2 runs", "\xff\x00\xff", "��"},
		{"invalid bytes and control characters", "you\x00 slept \xf0\x9f\x8c\x1b", "you slept �"},
		{
			"the push text cut at byte 178",
			pushText[:178],
			pushCluster + "\nHUSH resolved 3 incidents while you slept �",
		},
	})
}
