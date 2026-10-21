package rollcall_test

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	rollcall "github.com/yornik/go-practice/exercises/07-rollcall"
)

// readLines reads a file from testdata/ as lines. It accepts Windows line
// endings, so a checkout with CRLF files gives the same lines.
func readLines(t *testing.T, name string) []string {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

// formatReport writes a report in the format of testdata/report.golden. The
// lists are written in the order RollCall returned them.
func formatReport(r rollcall.Report) string {
	var b strings.Builder
	for _, l := range []struct {
		title string
		names []string
	}{
		{"healthy", r.Healthy},
		{"stale", r.Stale},
		{"missing", r.Missing},
		{"unknown", r.Unknown},
	} {
		fmt.Fprintf(&b, "%s %d\n", l.title, len(l.names))
		for _, n := range l.names {
			fmt.Fprintf(&b, "  %s\n", n)
		}
	}
	fmt.Fprintf(&b, "lastseen %d\n", len(r.LastSeen))
	for _, n := range slices.Sorted(maps.Keys(r.LastSeen)) {
		fmt.Fprintf(&b, "  %s %s\n", n, r.LastSeen[n].UTC().Format(time.RFC3339Nano))
	}
	return b.String()
}

// TestRollCallFleet runs a roll call over the whole fleet: 500 feeders in the
// inventory (2 names appear twice) and 1434 heartbeat lines merged from 3
// relays, in no particular order. 4 lines are malformed.
func TestRollCallFleet(t *testing.T) {
	inventory := readLines(t, "testdata/inventory.txt")
	lines := readLines(t, "testdata/heartbeats.txt")

	var beats []rollcall.Heartbeat
	var malformed []int
	for i, line := range lines {
		b, err := rollcall.ParseHeartbeat(line)
		if err != nil {
			malformed = append(malformed, i+1)
			continue
		}
		beats = append(beats, b)
	}
	if want := []int{58, 413, 804, 1121}; !slices.Equal(malformed, want) {
		t.Fatalf("ParseHeartbeat returned an error for lines %v of testdata/heartbeats.txt, want exactly lines %v", malformed, want)
	}

	r := rollcall.RollCall(inventory, beats, now, 30*time.Minute)

	got := strings.Split(formatReport(r), "\n")
	want := readLines(t, "testdata/report.golden")
	want = append(want, "") // formatReport ends with a newline
	for i := range max(len(got), len(want)) {
		var g, w string
		if i < len(got) {
			g = got[i]
		}
		if i < len(want) {
			w = want[i]
		}
		if g != w {
			t.Fatalf("RollCall(inventory.txt, heartbeats.txt, now %s, staleAfter 30m) differs from testdata/report.golden at line %d:\n got  %q\n want %q\nsizes: got %s, want %s",
				now.Format(time.RFC3339), i+1, g, w, sizes(got), sizes(want))
		}
	}
}

// sizes picks the section headers, such as "healthy 447", out of a formatted
// report.
func sizes(lines []string) string {
	var hs []string
	for _, l := range lines {
		if l != "" && !strings.HasPrefix(l, " ") {
			hs = append(hs, l)
		}
	}
	return strings.Join(hs, ", ")
}

// The example from the README.
func ExampleRollCall() {
	inventory := []string{"hq-window-01", "feeder-0217", "feeder-0412", "reserve-01", "reserve-02", "hq-window-01", ""}
	lines := []string{
		"hq-window-01 2026-10-15T08:55:00Z",
		"feeder-0412 2026-10-15T10:31:12+02:00",
		"feeder-0217 2026-10-13T18:42:10+02:00",
		"feeder-0412 2026-10-15T08:01:40Z",
		"reserve-01 2026-10-15T08:30:00Z",
		"feeder-0501 2026-10-15T08:58:03.5Z",
		"hq-window-01 2026-10-15T09:01:30Z",
		"feeder-0100 2026-10-15T08:51:00",
	}
	var beats []rollcall.Heartbeat
	for _, line := range lines {
		b, err := rollcall.ParseHeartbeat(line)
		if err != nil {
			fmt.Println("skipped:", line)
			continue
		}
		beats = append(beats, b)
	}

	now := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	r := rollcall.RollCall(inventory, beats, now, 30*time.Minute)

	fmt.Println("healthy:", r.Healthy)
	fmt.Println("stale:  ", r.Stale)
	fmt.Println("missing:", r.Missing)
	fmt.Println("unknown:", r.Unknown)
	for _, name := range slices.Sorted(maps.Keys(r.LastSeen)) {
		fmt.Println("last seen:", name, r.LastSeen[name].UTC().Format(time.RFC3339Nano))
	}
	// Output:
	// skipped: feeder-0100 2026-10-15T08:51:00
	// healthy: [feeder-0412 hq-window-01 reserve-01]
	// stale:   [feeder-0217]
	// missing: [reserve-02]
	// unknown: [feeder-0501]
	// last seen: feeder-0217 2026-10-13T16:42:10Z
	// last seen: feeder-0412 2026-10-15T08:31:12Z
	// last seen: feeder-0501 2026-10-15T08:58:03.5Z
	// last seen: hq-window-01 2026-10-15T09:01:30Z
	// last seen: reserve-01 2026-10-15T08:30:00Z
}
