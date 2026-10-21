package pushtext_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	pushtext "github.com/yornik/go-practice/exercises/10-pushtext"
)

type truncCase struct {
	s     string
	limit int
	want  string
}

// runTruncateBytes checks TruncateBytes against every case. %q shows invalid
// bytes as \x escapes, so a rune cut in half is visible in the message.
func runTruncateBytes(t *testing.T, cases []truncCase) {
	t.Helper()
	for _, tc := range cases {
		got := pushtext.TruncateBytes(tc.s, tc.limit)
		if got != tc.want {
			t.Errorf("TruncateBytes(%q, %d)\n got  %q (%d bytes)\n want %q (%d bytes)",
				tc.s, tc.limit, got, len(got), tc.want, len(tc.want))
		}
	}
}

func TestTruncateBytesFits(t *testing.T) {
	runTruncateBytes(t, []truncCase{
		{"", 0, ""},
		{"", 10, ""},
		{"abc", 3, "abc"},
		{"abc", 100, "abc"},
		{"3 incidents", 11, "3 incidents"},
		{"🌙", 4, "🌙"},
		{"olé olé", 9, "olé olé"},
		// A text that fits is not changed, also when it ends with the ellipsis.
		{"ovens…", 9, "ovens…"},
		// TruncateBytes does not clean: invalid bytes that fit stay as they are.
		{"\xff", 1, "\xff"},
		{"ab\xf0\x9f", 4, "ab\xf0\x9f"},
	})
}

func TestTruncateBytesASCII(t *testing.T) {
	runTruncateBytes(t, []truncCase{
		{"abcdefgh", 7, "abcd…"},
		{"abcdefgh", 6, "abc…"},
		{"abcdefgh", 4, "a…"},
		{"abcdefgh", 3, "…"},
		{"abcdefgh", 2, "ab"},
		{"abcdefgh", 1, "a"},
		{"abcdefgh", 0, ""},
		{"abcdefgh", -1, ""},
		{"abcdefgh", -100, ""},
	})
}

// TestTruncateBytesEveryCut moves the limit across a 2-byte é and a 4-byte 🌙.
func TestTruncateBytesEveryCut(t *testing.T) {
	// "olé olé" is 9 bytes: o l é(2) space o l é(2).
	runTruncateBytes(t, []truncCase{
		{"olé olé", 10, "olé olé"},
		{"olé olé", 9, "olé olé"},
		{"olé olé", 8, "olé …"},
		{"olé olé", 7, "olé…"},
		{"olé olé", 6, "ol…"}, // room for 3 bytes: "olé" needs 4
		{"olé olé", 5, "ol…"},
		{"olé olé", 4, "o…"},
		{"olé olé", 3, "…"},
		{"olé olé", 2, "ol"},
		{"olé olé", 1, "o"},
		{"olé olé", 0, ""},
	})
	// "ab🌙cd" is 8 bytes: a b 🌙(4) c d.
	runTruncateBytes(t, []truncCase{
		{"ab🌙cd", 8, "ab🌙cd"},
		{"ab🌙cd", 7, "ab…"}, // room for 4 bytes: "ab🌙" needs 6
		{"ab🌙cd", 6, "ab…"},
		{"ab🌙cd", 5, "ab…"},
		{"ab🌙cd", 4, "a…"},
		{"ab🌙cd", 3, "…"},
		{"ab🌙cd", 2, "ab"},
		{"ab🌙cd", 1, "a"},
		{"ab🌙cd", 0, ""},
	})
	// "ab🌙cdef" is 10 bytes, so a limit of 9 leaves room for the whole 🌙.
	runTruncateBytes(t, []truncCase{
		{"ab🌙cdef", 10, "ab🌙cdef"},
		{"ab🌙cdef", 9, "ab🌙…"},
		{"ab🌙cdef", 8, "ab…"},
		{"ab🌙cdef", 7, "ab…"},
		{"ab🌙cdef", 6, "ab…"},
	})
}

func TestTruncateBytesSmallMax(t *testing.T) {
	runTruncateBytes(t, []truncCase{
		// Below 3 bytes there is no room for the ellipsis, and a rune that
		// does not fit is left out completely.
		{"🌙🌙", 2, ""},
		{"🌙🌙", 1, ""},
		{"éa", 1, ""},
		{"éa", 2, "é"},
		{"aéb", 2, "a"},
		// With 3 to 6 bytes the ellipsis fits, but a second 🌙 does not.
		{"🌙🌙", 7, "🌙…"},
		{"🌙🌙", 6, "…"},
		{"🌙🌙", 3, "…"},
		{"🌙🌙", 0, ""},
	})
}

func TestTruncateBytesInvalid(t *testing.T) {
	runTruncateBytes(t, []truncCase{
		// An invalid byte is a rune of 1 byte, and it is kept as it is.
		{"\xff\xffabcdef", 6, "\xff\xffa…"},
		{"\x80\x80\x80\x80", 2, "\x80\x80"},
		{"\x80\x80\x80\x80", 3, "…"},
		// An emoji that was already cut upstream: its 3 remaining bytes are
		// 3 invalid runes of 1 byte, so a cut between them is allowed.
		{"ab\xf0\x9f\x8ccd", 6, "ab\xf0…"},
		{"ab\xf0\x9f\x8ccd", 5, "ab…"},
		// A valid rune after the invalid bytes is still never cut in half.
		{"\xffé\xff", 2, "\xff"},
	})
}

// pushText is a push text of 2 lines: a cluster name of 132 bytes and the
// summary. The 🌙 starts at byte 175, so pushText[:178] cuts it after 3 bytes.
const (
	pushCluster = "Varga Pékség – éjszakai kemencék · Meridian night market, stall 141 · 2 servers, 6 oven controllers, the order queue, a till"
	pushText    = pushCluster + "\nHUSH resolved 3 incidents while you slept 🌙 Have a calm day."
)

func TestTruncateBytesPushText(t *testing.T) {
	// Check the test data itself first: the cut at byte 178 must fall inside 🌙.
	if i := strings.Index(pushText, "🌙"); i != 175 {
		t.Fatalf("test data: the emoji starts at byte %d, want 175", i)
	}
	if utf8.ValidString(pushText[:178]) {
		t.Fatal("test data: pushText[:178] is valid UTF-8, want the cut inside the emoji")
	}

	runTruncateBytes(t, []truncCase{
		{pushText, 178, pushCluster + "\nHUSH resolved 3 incidents while you slept …"},
		{pushText, 179, pushCluster + "\nHUSH resolved 3 incidents while you slept …"},
		{pushText, 182, pushCluster + "\nHUSH resolved 3 incidents while you slept 🌙…"},
		{pushText, len(pushText), pushText},
	})

	got := pushtext.TruncateBytes(pushText, 178)
	if len(got) > 178 || !utf8.ValidString(got) {
		t.Errorf("TruncateBytes(pushText, 178) = %q (%d bytes, valid UTF-8 %v), want at most 178 bytes of valid UTF-8",
			got, len(got), utf8.ValidString(got))
	}
}
