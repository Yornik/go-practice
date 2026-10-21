package rollcall_test

import (
	"slices"
	"testing"
	"time"

	rollcall "github.com/yornik/go-practice/exercises/07-rollcall"
)

// Step 2: every heartbeat in these tests is a few minutes old, so no feeder
// is stale.
func TestRollCallSets(t *testing.T) {
	runRollCallCases(t, []rollCallCase{
		{
			name:      "healthy, missing and unknown",
			inventory: []string{"hq-window-01", "feeder-0217", "feeder-0412"},
			beats: []rollcall.Heartbeat{
				hb("hq-window-01", now.Add(-5*time.Minute)),
				hb("feeder-0412", now.Add(-10*time.Minute)),
				hb("feeder-0999", now.Add(-1*time.Minute)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"feeder-0412", "hq-window-01"},
				Missing: []string{"feeder-0217"},
				Unknown: []string{"feeder-0999"},
				LastSeen: map[string]time.Time{
					"hq-window-01": now.Add(-5 * time.Minute),
					"feeder-0412":  now.Add(-10 * time.Minute),
					"feeder-0999":  now.Add(-1 * time.Minute),
				},
			},
		},
		{
			name:      "every list sorted A to Z",
			inventory: []string{"reserve-01", "feeder-0010", "hq-window-01", "feeder-0002", "reserve-02", "feeder-0003"},
			beats: []rollcall.Heartbeat{
				hb("test-bench-3", now.Add(-2*time.Minute)),
				hb("reserve-01", now.Add(-3*time.Minute)),
				hb("hq-window-01", now.Add(-4*time.Minute)),
				hb("feeder-0501", now.Add(-5*time.Minute)),
				hb("feeder-0010", now.Add(-6*time.Minute)),
				hb("feeder-0002", now.Add(-7*time.Minute)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"feeder-0002", "feeder-0010", "hq-window-01", "reserve-01"},
				Missing: []string{"feeder-0003", "reserve-02"},
				Unknown: []string{"feeder-0501", "test-bench-3"},
				LastSeen: map[string]time.Time{
					"test-bench-3": now.Add(-2 * time.Minute),
					"reserve-01":   now.Add(-3 * time.Minute),
					"hq-window-01": now.Add(-4 * time.Minute),
					"feeder-0501":  now.Add(-5 * time.Minute),
					"feeder-0010":  now.Add(-6 * time.Minute),
					"feeder-0002":  now.Add(-7 * time.Minute),
				},
			},
		},
		{
			name:       "names are compared exactly, letter case too",
			inventory:  []string{"feeder-0012"},
			beats:      []rollcall.Heartbeat{hb("FEEDER-0012", now.Add(-1*time.Minute))},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Missing:  []string{"feeder-0012"},
				Unknown:  []string{"FEEDER-0012"},
				LastSeen: map[string]time.Time{"FEEDER-0012": now.Add(-1 * time.Minute)},
			},
		},
		{
			name:      "empty inventory: every feeder is unknown",
			inventory: nil,
			beats: []rollcall.Heartbeat{
				hb("hq-window-01", now.Add(-1*time.Minute)),
				hb("feeder-0412", now.Add(-2*time.Minute)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Unknown: []string{"feeder-0412", "hq-window-01"},
				LastSeen: map[string]time.Time{
					"hq-window-01": now.Add(-1 * time.Minute),
					"feeder-0412":  now.Add(-2 * time.Minute),
				},
			},
		},
		{
			name:       "no heartbeats: every feeder is missing",
			inventory:  []string{"reserve-02", "hq-window-01", "feeder-0412"},
			beats:      nil,
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Missing:  []string{"feeder-0412", "hq-window-01", "reserve-02"},
				LastSeen: map[string]time.Time{},
			},
		},
		{
			name:       "nothing at all",
			inventory:  nil,
			beats:      nil,
			now:        now,
			staleAfter: 30 * time.Minute,
			want:       rollcall.Report{LastSeen: map[string]time.Time{}},
		},
	})
}

// Step 3: the age of the last heartbeat decides between healthy and stale.
func TestRollCallStale(t *testing.T) {
	runRollCallCases(t, []rollCallCase{
		{
			name:       "exactly staleAfter old is healthy",
			inventory:  []string{"reserve-01"},
			beats:      []rollcall.Heartbeat{hb("reserve-01", now.Add(-30*time.Minute))},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy:  []string{"reserve-01"},
				LastSeen: map[string]time.Time{"reserve-01": now.Add(-30 * time.Minute)},
			},
		},
		{
			name:       "1 ns older than staleAfter is stale",
			inventory:  []string{"feeder-0311"},
			beats:      []rollcall.Heartbeat{hb("feeder-0311", now.Add(-30*time.Minute-time.Nanosecond))},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Stale:    []string{"feeder-0311"},
				LastSeen: map[string]time.Time{"feeder-0311": now.Add(-30*time.Minute - time.Nanosecond)},
			},
		},
		{
			name:      "staleAfter 0: only a heartbeat at now or later is healthy",
			inventory: []string{"feeder-0001", "feeder-0002", "feeder-0003"},
			beats: []rollcall.Heartbeat{
				hb("feeder-0001", now),
				hb("feeder-0002", now.Add(-time.Nanosecond)),
				hb("feeder-0003", now.Add(time.Second)),
			},
			now:        now,
			staleAfter: 0,
			want: rollcall.Report{
				Healthy: []string{"feeder-0001", "feeder-0003"},
				Stale:   []string{"feeder-0002"},
				LastSeen: map[string]time.Time{
					"feeder-0001": now,
					"feeder-0002": now.Add(-time.Nanosecond),
					"feeder-0003": now.Add(time.Second),
				},
			},
		},
		{
			name:      "a feeder silent since Tuesday is stale",
			inventory: []string{"feeder-0217", "hq-window-01"},
			beats: []rollcall.Heartbeat{
				hb("hq-window-01", now.Add(-2*time.Minute)),
				hb("feeder-0217", time.Date(2026, 10, 13, 18, 42, 10, 0, cest)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"hq-window-01"},
				Stale:   []string{"feeder-0217"},
				LastSeen: map[string]time.Time{
					"hq-window-01": now.Add(-2 * time.Minute),
					"feeder-0217":  time.Date(2026, 10, 13, 16, 42, 10, 0, time.UTC),
				},
			},
		},
		{
			name:      "the latest heartbeat counts, not the last one in the slice",
			inventory: []string{"feeder-0412", "feeder-0217"},
			beats: []rollcall.Heartbeat{
				hb("feeder-0412", now.Add(-2*time.Minute)),
				hb("feeder-0217", now.Add(-3*time.Hour)),
				hb("feeder-0412", now.Add(-3*time.Hour)),
				hb("feeder-0217", now.Add(-40*time.Minute)),
				hb("feeder-0217", now.Add(-2*time.Hour)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"feeder-0412"},
				Stale:   []string{"feeder-0217"},
				LastSeen: map[string]time.Time{
					"feeder-0412": now.Add(-2 * time.Minute),
					"feeder-0217": now.Add(-40 * time.Minute),
				},
			},
		},
		{
			name:      "a heartbeat after now is healthy and keeps its time",
			inventory: []string{"feeder-0314", "feeder-0315"},
			beats: []rollcall.Heartbeat{
				hb("feeder-0314", now.Add(45*time.Second)),
				hb("feeder-0315", now.Add(-3*time.Hour)),
				hb("feeder-0315", now.Add(26*time.Hour)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"feeder-0314", "feeder-0315"},
				LastSeen: map[string]time.Time{
					"feeder-0314": now.Add(45 * time.Second),
					"feeder-0315": now.Add(26 * time.Hour),
				},
			},
		},
		{
			// 10:30:00+02:00 is 08:30:00 UTC: exactly 30 minutes before now.
			name:      "heartbeats in +02:00, now in UTC",
			inventory: []string{"feeder-0313", "feeder-0316"},
			beats: []rollcall.Heartbeat{
				hb("feeder-0313", time.Date(2026, 10, 15, 10, 30, 0, 0, cest)),
				hb("feeder-0316", time.Date(2026, 10, 15, 10, 29, 59, 0, cest)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"feeder-0313"},
				Stale:   []string{"feeder-0316"},
				LastSeen: map[string]time.Time{
					"feeder-0313": time.Date(2026, 10, 15, 8, 30, 0, 0, time.UTC),
					"feeder-0316": time.Date(2026, 10, 15, 8, 29, 59, 0, time.UTC),
				},
			},
		},
		{
			// now is 11:00+02:00, the same instant as 09:00 UTC.
			name:      "heartbeats in UTC, now in +02:00",
			inventory: []string{"feeder-0313", "feeder-0316"},
			beats: []rollcall.Heartbeat{
				hb("feeder-0313", time.Date(2026, 10, 15, 8, 30, 0, 0, time.UTC)),
				hb("feeder-0316", time.Date(2026, 10, 15, 8, 29, 59, 0, time.UTC)),
			},
			now:        now.In(cest),
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"feeder-0313"},
				Stale:   []string{"feeder-0316"},
				LastSeen: map[string]time.Time{
					"feeder-0313": time.Date(2026, 10, 15, 8, 30, 0, 0, time.UTC),
					"feeder-0316": time.Date(2026, 10, 15, 8, 29, 59, 0, time.UTC),
				},
			},
		},
		{
			// 08:50 UTC is later than 10:40+02:00 (08:40 UTC), although 8 < 10.
			name:      "the latest heartbeat is decided by instant, not by wall clock",
			inventory: []string{"feeder-0412"},
			beats: []rollcall.Heartbeat{
				hb("feeder-0412", time.Date(2026, 10, 15, 8, 50, 0, 0, time.UTC)),
				hb("feeder-0412", time.Date(2026, 10, 15, 10, 40, 0, 0, cest)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy:  []string{"feeder-0412"},
				LastSeen: map[string]time.Time{"feeder-0412": time.Date(2026, 10, 15, 8, 50, 0, 0, time.UTC)},
			},
		},
		{
			name:      "an unknown feeder is unknown however old its heartbeat",
			inventory: []string{"hq-window-01"},
			beats: []rollcall.Heartbeat{
				hb("hq-window-01", now.Add(-1*time.Minute)),
				hb("feeder-0502", now.Add(-5*time.Hour)),
				hb("hq-window-02", now.Add(-1*time.Hour)),
				hb("hq-window-02", now.Add(-5*time.Hour)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"hq-window-01"},
				Unknown: []string{"feeder-0502", "hq-window-02"},
				LastSeen: map[string]time.Time{
					"hq-window-01": now.Add(-1 * time.Minute),
					"feeder-0502":  now.Add(-5 * time.Hour),
					"hq-window-02": now.Add(-1 * time.Hour),
				},
			},
		},
	})
}

// Step 4: what RollCall does with the shape of its inputs.
func TestRollCallInputs(t *testing.T) {
	runRollCallCases(t, []rollCallCase{
		{
			name:      "duplicates in the inventory appear once",
			inventory: []string{"hq-window-01", "feeder-0412", "hq-window-01", "feeder-0217", "feeder-0217", "reserve-02", "reserve-02"},
			beats: []rollcall.Heartbeat{
				hb("hq-window-01", now.Add(-1*time.Minute)),
				hb("feeder-0217", now.Add(-2*time.Hour)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"hq-window-01"},
				Stale:   []string{"feeder-0217"},
				Missing: []string{"feeder-0412", "reserve-02"},
				LastSeen: map[string]time.Time{
					"hq-window-01": now.Add(-1 * time.Minute),
					"feeder-0217":  now.Add(-2 * time.Hour),
				},
			},
		},
		{
			name:      "many heartbeats of 1 unknown feeder give 1 name",
			inventory: nil,
			beats: []rollcall.Heartbeat{
				hb("feeder-0501", now.Add(-1*time.Minute)),
				hb("feeder-0501", now.Add(-11*time.Minute)),
				hb("feeder-0501", now.Add(-21*time.Minute)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Unknown:  []string{"feeder-0501"},
				LastSeen: map[string]time.Time{"feeder-0501": now.Add(-1 * time.Minute)},
			},
		},
		{
			name:       "empty names are ignored in the inventory",
			inventory:  []string{"", "hq-window-01", ""},
			beats:      []rollcall.Heartbeat{hb("hq-window-01", now.Add(-1*time.Minute))},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy:  []string{"hq-window-01"},
				LastSeen: map[string]time.Time{"hq-window-01": now.Add(-1 * time.Minute)},
			},
		},
		{
			name:      "empty names are ignored in the heartbeats",
			inventory: []string{"hq-window-01"},
			beats: []rollcall.Heartbeat{
				hb("", now.Add(-1*time.Minute)),
				hb("hq-window-01", now.Add(-2*time.Minute)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy:  []string{"hq-window-01"},
				LastSeen: map[string]time.Time{"hq-window-01": now.Add(-2 * time.Minute)},
			},
		},
		{
			name:       "an empty name in both is still ignored",
			inventory:  []string{""},
			beats:      []rollcall.Heartbeat{hb("", now)},
			now:        now,
			staleAfter: 30 * time.Minute,
			want:       rollcall.Report{LastSeen: map[string]time.Time{}},
		},
	})
}

func TestRollCallInputsUnchanged(t *testing.T) {
	inventory := []string{"reserve-01", "hq-window-01", "feeder-0412", "hq-window-01", "", "feeder-0003"}
	beats := []rollcall.Heartbeat{
		hb("feeder-0412", now.Add(-3*time.Hour)),
		hb("hq-window-01", now.Add(-1*time.Minute)),
		hb("feeder-0999", now.Add(-2*time.Minute)),
		hb("", now),
		hb("feeder-0412", now.Add(-5*time.Minute)),
		hb("reserve-01", time.Date(2026, 10, 15, 10, 45, 0, 0, cest)),
	}
	inventoryBefore := slices.Clone(inventory)
	beatsBefore := slices.Clone(beats)
	call := describeCall(inventory, beats, now, 30*time.Minute)

	r := rollcall.RollCall(inventory, beats, now, 30*time.Minute)

	if !slices.Equal(inventory, inventoryBefore) {
		t.Errorf("%s\nchanged the inventory:\n before %q\n after  %q", call, inventoryBefore, inventory)
	}
	if !slices.Equal(beats, beatsBefore) {
		t.Errorf("%s\nchanged the heartbeats:\n before %s\n after  %s", call, formatBeats(beatsBefore), formatBeats(beats))
	}
	checkReport(t, call, r, rollcall.Report{
		Healthy: []string{"feeder-0412", "hq-window-01", "reserve-01"},
		Missing: []string{"feeder-0003"},
		Unknown: []string{"feeder-0999"},
		LastSeen: map[string]time.Time{
			"feeder-0412":  now.Add(-5 * time.Minute),
			"hq-window-01": now.Add(-1 * time.Minute),
			"feeder-0999":  now.Add(-2 * time.Minute),
			"reserve-01":   time.Date(2026, 10, 15, 8, 45, 0, 0, time.UTC),
		},
	})
}
