package rollcall_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	rollcall "github.com/yornik/go-practice/exercises/07-rollcall"
)

// now is the moment of the roll call in most tests: Thursday 15 October 2026,
// 09:00 UTC.
var now = time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)

// cest is the offset of 1 of the 3 relays in October: +02:00.
var cest = time.FixedZone("CEST", 2*60*60)

// hb is a short way to write a heartbeat.
func hb(node string, at time.Time) rollcall.Heartbeat {
	return rollcall.Heartbeat{Node: node, At: at}
}

// rollCallCase is 1 call of RollCall and the report it must return.
type rollCallCase struct {
	name       string
	inventory  []string
	beats      []rollcall.Heartbeat
	now        time.Time
	staleAfter time.Duration
	want       rollcall.Report
}

// runRollCallCases calls RollCall for every case and compares the report.
func runRollCallCases(t *testing.T, cases []rollCallCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rollcall.RollCall(tc.inventory, tc.beats, tc.now, tc.staleAfter)
			checkReport(t, describeCall(tc.inventory, tc.beats, tc.now, tc.staleAfter), got, tc.want)
		})
	}
}

// describeCall writes a call of RollCall for failure messages.
func describeCall(inventory []string, beats []rollcall.Heartbeat, now time.Time, staleAfter time.Duration) string {
	return fmt.Sprintf("RollCall(\n  inventory  %q,\n  beats      %s,\n  now        %s,\n  staleAfter %v)",
		inventory, formatBeats(beats), now.Format(time.RFC3339Nano), staleAfter)
}

func formatBeats(beats []rollcall.Heartbeat) string {
	parts := make([]string, len(beats))
	for i, b := range beats {
		parts[i] = fmt.Sprintf("{%q %s}", b.Node, b.At.Format(time.RFC3339Nano))
	}
	return "[" + strings.Join(parts, " ") + "]"
}

// checkReport compares every field of got with want. A nil and an empty list
// count as equal. Times in LastSeen are compared with Equal, so the zone in
// which a time is written does not matter.
func checkReport(t *testing.T, call string, got, want rollcall.Report) {
	t.Helper()
	lists := []struct {
		field     string
		got, want []string
	}{
		{"Healthy", got.Healthy, want.Healthy},
		{"Stale", got.Stale, want.Stale},
		{"Missing", got.Missing, want.Missing},
		{"Unknown", got.Unknown, want.Unknown},
	}
	for _, l := range lists {
		if !slices.Equal(l.got, l.want) {
			t.Errorf("%s\n.%s:\n got  %q\n want %q", call, l.field, l.got, l.want)
		}
	}
	if got.LastSeen == nil {
		t.Errorf("%s\n.LastSeen is nil, want a non-nil map", call)
		return
	}
	if !maps.EqualFunc(got.LastSeen, want.LastSeen, time.Time.Equal) {
		t.Errorf("%s\n.LastSeen:\n got  %s\n want %s", call, formatLastSeen(got.LastSeen), formatLastSeen(want.LastSeen))
	}
}

// formatLastSeen writes a LastSeen map in key order, every time in UTC.
func formatLastSeen(m map[string]time.Time) string {
	var parts []string
	for _, k := range slices.Sorted(maps.Keys(m)) {
		parts = append(parts, fmt.Sprintf("%q: %s", k, m[k].UTC().Format(time.RFC3339Nano)))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}
