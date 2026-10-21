package repeated_test

import (
	"testing"

	repeated "github.com/yornik/go-practice/exercises/02-repeated"
)

func TestMessage(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{"timestamp and message", "2026-09-09T03:01:26Z wifid: connection lost", "wifid: connection lost"},
		{"spaces inside the message are kept", "2026-09-09T03:01:26Z camd: started, firmware 2.9.3", "camd: started, firmware 2.9.3"},
		{"only the first space separates", "2026-09-09T03:01:26Z  two spaces", " two spaces"},
		{"trailing spaces are kept", "2026-09-09T03:01:26Z motiond: motion detected  ", "motiond: motion detected  "},
		{"no space: the whole line", "booting", "booting"},
		{"a tab is not a space", "2026-09-09T03:01:26Z\tmotiond", "2026-09-09T03:01:26Z\tmotiond"},
		{"leading space: empty timestamp", " motiond: motion detected", "motiond: motion detected"},
		{"timestamp with an empty message", "2026-09-09T03:01:26Z ", ""},
		{"empty line", "", ""},
		{"only a space", " ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := repeated.Message(tc.line); got != tc.want {
				t.Errorf("Message(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}
