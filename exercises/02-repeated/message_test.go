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
		{"timestamp and message", "2026-09-09T03:01:26Z netd: sfp0 link down", "netd: sfp0 link down"},
		{"spaces inside the message are kept", "2026-09-09T03:01:26Z gpud: started, driver 2.9.3", "gpud: started, driver 2.9.3"},
		{"only the first space separates", "2026-09-09T03:01:26Z  two spaces", " two spaces"},
		{"trailing spaces are kept", "2026-09-09T03:01:26Z hushd: all quiet  ", "hushd: all quiet  "},
		{"no space: the whole line", "booting", "booting"},
		{"a tab is not a space", "2026-09-09T03:01:26Z\thushd", "2026-09-09T03:01:26Z\thushd"},
		{"leading space: empty timestamp", " hushd: all quiet", "hushd: all quiet"},
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
