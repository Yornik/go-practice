package checkdisk_test

import (
	"strings"
	"testing"

	checkdisk "github.com/yornik/go-practice/exercises/15-checkdisk"
)

// checkState fails the test unless Run returned code and stdout starts with
// "DISK <STATE> - ". Step 1 checks only the state; step 2 checks the rest of
// the line.
func checkState(t *testing.T, what string, res result, code int) {
	t.Helper()
	if res.code != code {
		t.Errorf("%s\nreturned exit code %d, want %d (%s)", what, res.code, code, stateNames[code])
	}
	prefix := "DISK " + stateNames[code] + " - "
	if !strings.HasPrefix(res.stdout, prefix) {
		t.Errorf("%s\nwrote to stdout %q, want a line that starts with %q", what, res.stdout, prefix)
	}
}

func TestRunStatesFromFiles(t *testing.T) {
	cases := []struct {
		file string
		want int
	}{
		{"df-ok.txt", checkdisk.OK},
		{"df-warning.txt", checkdisk.Warning},
		{"df-critical.txt", checkdisk.Critical},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			input := readFile(t, tc.file)
			res := runText(t, input)
			checkState(t, call(nil, input), res, tc.want)
		})
	}
}

func TestRunStatesThresholds(t *testing.T) {
	// The defaults are -w 80 and -c 90. A usage equal to a threshold
	// reaches it.
	cases := []struct {
		pct  int
		want int
	}{
		{0, checkdisk.OK},
		{79, checkdisk.OK},
		{80, checkdisk.Warning},
		{89, checkdisk.Warning},
		{90, checkdisk.Critical},
		{100, checkdisk.Critical},
	}
	for _, tc := range cases {
		input := df(row("/", tc.pct))
		res := runText(t, input)
		checkState(t, call(nil, input), res, tc.want)
	}
}

func TestRunStatesWorstWins(t *testing.T) {
	cases := []struct {
		name string
		pcts []int
		want int
	}{
		{"all below", []int{10, 79, 42}, checkdisk.OK},
		{"one warning", []int{10, 85, 42}, checkdisk.Warning},
		{"warning last", []int{10, 42, 80}, checkdisk.Warning},
		{"one critical", []int{10, 95, 42}, checkdisk.Critical},
		{"critical before warning", []int{95, 85}, checkdisk.Critical},
		{"critical after warning", []int{85, 95}, checkdisk.Critical},
		{"critical after warnings", []int{81, 82, 83, 90}, checkdisk.Critical},
	}
	mounts := []string{"/", "/var", "/home", "/srv/photos"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var lines []string
			for i, pct := range tc.pcts {
				lines = append(lines, row(mounts[i], pct))
			}
			input := df(lines...)
			res := runText(t, input)
			checkState(t, call(nil, input), res, tc.want)
		})
	}
}
