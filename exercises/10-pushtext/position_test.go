package pushtext_test

import (
	"testing"

	pushtext "github.com/yornik/go-practice/exercises/10-pushtext"
)

type posCase struct {
	offset    int
	line, col int
	ok        bool
}

// runPosition checks Position for 1 text at several offsets.
func runPosition(t *testing.T, s string, cases []posCase) {
	t.Helper()
	for _, tc := range cases {
		line, col, ok := pushtext.Position(s, tc.offset)
		if line != tc.line || col != tc.col || ok != tc.ok {
			t.Errorf("Position(%q, %d) = %d, %d, %v; want %d, %d, %v",
				s, tc.offset, line, col, ok, tc.line, tc.col, tc.ok)
		}
	}
}

func TestPositionLines(t *testing.T) {
	runPosition(t, "ab\ncd", []posCase{
		{0, 1, 1, true},
		{1, 1, 2, true},
		{2, 1, 3, true}, // the \n is the last character of line 1
		{3, 2, 1, true},
		{4, 2, 2, true},
		{5, 2, 3, true}, // len(s): right after the last character
	})
	runPosition(t, "", []posCase{
		{0, 1, 1, true},
	})
	runPosition(t, "ab\n", []posCase{
		{3, 2, 1, true},
	})
	runPosition(t, "\n\n\n", []posCase{
		{0, 1, 1, true},
		{1, 2, 1, true},
		{3, 4, 1, true},
	})
	// A tab is 1 rune, so it is 1 column.
	runPosition(t, "\tx", []posCase{
		{1, 1, 2, true},
	})
}

func TestPositionMultiByte(t *testing.T) {
	// é is bytes 0-1, \n is byte 2, 🌙 is bytes 3-6, x is byte 7.
	runPosition(t, "é\n🌙x", []posCase{
		{0, 1, 1, true},
		{2, 1, 2, true},
		{3, 2, 1, true},
		{7, 2, 2, true},
		{8, 2, 3, true},
	})
	runPosition(t, "olé olé", []posCase{
		{4, 1, 4, true}, // the space after the first é
		{5, 1, 5, true},
		{9, 1, 8, true},
	})
	// An invalid byte is a rune of 1 byte, so every offset in a run of
	// invalid bytes is the start of a rune.
	runPosition(t, "a\x80\x80b", []posCase{
		{1, 1, 2, true},
		{2, 1, 3, true},
		{3, 1, 4, true},
	})
}

func TestPositionInsideRune(t *testing.T) {
	runPosition(t, "é\n🌙x", []posCase{
		{1, 0, 0, false}, // the second byte of é
		{4, 0, 0, false}, // bytes 2, 3 and 4 of 🌙
		{5, 0, 0, false},
		{6, 0, 0, false},
	})
	runPosition(t, "🌙", []posCase{
		{3, 0, 0, false}, // the last byte of the last rune
		{4, 1, 2, true},
	})
}

func TestPositionCRLF(t *testing.T) {
	// Lines end at \n only, so \r is an ordinary character of line 1.
	runPosition(t, "ab\r\ncd", []posCase{
		{2, 1, 3, true},
		{3, 1, 4, true},
		{4, 2, 1, true},
		{6, 2, 3, true},
	})
	runPosition(t, "a\rb", []posCase{
		{2, 1, 3, true},
	})
}

func TestPositionOutOfRange(t *testing.T) {
	runPosition(t, "abc", []posCase{
		{-1, 0, 0, false},
		{-100, 0, 0, false},
		{4, 0, 0, false},
		{100, 0, 0, false},
	})
	runPosition(t, "", []posCase{
		{-1, 0, 0, false},
		{1, 0, 0, false},
	})
}

func TestPositionPushText(t *testing.T) {
	// Line 1 has 124 runes in 132 bytes (é and · are 2 bytes each, – is
	// 3), and the 🌙 on line 2 starts at byte 175.
	runPosition(t, pushText, []posCase{
		{132, 1, 125, true}, // the \n
		{133, 2, 1, true},
		{175, 2, 43, true},
		{176, 0, 0, false},
		{177, 0, 0, false},
		{179, 2, 44, true},
		{len(pushText), 2, 61, true},
	})
}
