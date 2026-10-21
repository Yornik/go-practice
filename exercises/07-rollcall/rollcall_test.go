package rollcall_test

import (
	"slices"
	"testing"
	"time"

	rollcall "github.com/yornik/go-practice/exercises/07-rollcall"
)

// Step 2: every heartbeat in these tests is a few minutes old, so no node
// is stale.
func TestRollCallSets(t *testing.T) {
	runRollCallCases(t, []rollCallCase{
		{
			name:      "healthy, missing and unknown",
			inventory: []string{"canary-h3-01", "vl-gpu-0217", "vl-gpu-0412"},
			beats: []rollcall.Heartbeat{
				hb("canary-h3-01", now.Add(-5*time.Minute)),
				hb("vl-gpu-0412", now.Add(-10*time.Minute)),
				hb("test-bench-9", now.Add(-1*time.Minute)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"canary-h3-01", "vl-gpu-0412"},
				Missing: []string{"vl-gpu-0217"},
				Unknown: []string{"test-bench-9"},
				LastSeen: map[string]time.Time{
					"canary-h3-01": now.Add(-5 * time.Minute),
					"vl-gpu-0412":  now.Add(-10 * time.Minute),
					"test-bench-9": now.Add(-1 * time.Minute),
				},
			},
		},
		{
			name:      "every list sorted A to Z",
			inventory: []string{"harbour-01", "vl-gpu-0010", "canary-h3-01", "vl-gpu-0002", "harbour-02", "vl-gpu-0003"},
			beats: []rollcall.Heartbeat{
				hb("vl-gpu-0501", now.Add(-2*time.Minute)),
				hb("vl-gpu-0010", now.Add(-3*time.Minute)),
				hb("vl-gpu-0002", now.Add(-4*time.Minute)),
				hb("test-bench-3", now.Add(-5*time.Minute)),
				hb("harbour-01", now.Add(-6*time.Minute)),
				hb("canary-h3-01", now.Add(-7*time.Minute)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"canary-h3-01", "harbour-01", "vl-gpu-0002", "vl-gpu-0010"},
				Missing: []string{"harbour-02", "vl-gpu-0003"},
				Unknown: []string{"test-bench-3", "vl-gpu-0501"},
				LastSeen: map[string]time.Time{
					"vl-gpu-0501":  now.Add(-2 * time.Minute),
					"vl-gpu-0010":  now.Add(-3 * time.Minute),
					"vl-gpu-0002":  now.Add(-4 * time.Minute),
					"test-bench-3": now.Add(-5 * time.Minute),
					"harbour-01":   now.Add(-6 * time.Minute),
					"canary-h3-01": now.Add(-7 * time.Minute),
				},
			},
		},
		{
			name:       "names are compared exactly, letter case too",
			inventory:  []string{"vl-gpu-0012"},
			beats:      []rollcall.Heartbeat{hb("VL-GPU-0012", now.Add(-1*time.Minute))},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Missing:  []string{"vl-gpu-0012"},
				Unknown:  []string{"VL-GPU-0012"},
				LastSeen: map[string]time.Time{"VL-GPU-0012": now.Add(-1 * time.Minute)},
			},
		},
		{
			name:      "empty inventory: every node is unknown",
			inventory: nil,
			beats: []rollcall.Heartbeat{
				hb("canary-h3-01", now.Add(-1*time.Minute)),
				hb("vl-gpu-0412", now.Add(-2*time.Minute)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Unknown: []string{"canary-h3-01", "vl-gpu-0412"},
				LastSeen: map[string]time.Time{
					"canary-h3-01": now.Add(-1 * time.Minute),
					"vl-gpu-0412":  now.Add(-2 * time.Minute),
				},
			},
		},
		{
			name:       "no heartbeats: every node is missing",
			inventory:  []string{"harbour-02", "canary-h3-01", "vl-gpu-0412"},
			beats:      nil,
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Missing:  []string{"canary-h3-01", "harbour-02", "vl-gpu-0412"},
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
			inventory:  []string{"harbour-01"},
			beats:      []rollcall.Heartbeat{hb("harbour-01", now.Add(-30*time.Minute))},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy:  []string{"harbour-01"},
				LastSeen: map[string]time.Time{"harbour-01": now.Add(-30 * time.Minute)},
			},
		},
		{
			name:       "1 ns older than staleAfter is stale",
			inventory:  []string{"vl-gpu-0311"},
			beats:      []rollcall.Heartbeat{hb("vl-gpu-0311", now.Add(-30*time.Minute-time.Nanosecond))},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Stale:    []string{"vl-gpu-0311"},
				LastSeen: map[string]time.Time{"vl-gpu-0311": now.Add(-30*time.Minute - time.Nanosecond)},
			},
		},
		{
			name:      "staleAfter 0: only a heartbeat at now or later is healthy",
			inventory: []string{"vl-gpu-0001", "vl-gpu-0002", "vl-gpu-0003"},
			beats: []rollcall.Heartbeat{
				hb("vl-gpu-0001", now),
				hb("vl-gpu-0002", now.Add(-time.Nanosecond)),
				hb("vl-gpu-0003", now.Add(time.Second)),
			},
			now:        now,
			staleAfter: 0,
			want: rollcall.Report{
				Healthy: []string{"vl-gpu-0001", "vl-gpu-0003"},
				Stale:   []string{"vl-gpu-0002"},
				LastSeen: map[string]time.Time{
					"vl-gpu-0001": now,
					"vl-gpu-0002": now.Add(-time.Nanosecond),
					"vl-gpu-0003": now.Add(time.Second),
				},
			},
		},
		{
			name:      "a node silent since Tuesday is stale",
			inventory: []string{"vl-gpu-0217", "canary-h3-01"},
			beats: []rollcall.Heartbeat{
				hb("canary-h3-01", now.Add(-2*time.Minute)),
				hb("vl-gpu-0217", time.Date(2026, 10, 13, 18, 42, 10, 0, cest)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"canary-h3-01"},
				Stale:   []string{"vl-gpu-0217"},
				LastSeen: map[string]time.Time{
					"canary-h3-01": now.Add(-2 * time.Minute),
					"vl-gpu-0217":  time.Date(2026, 10, 13, 16, 42, 10, 0, time.UTC),
				},
			},
		},
		{
			name:      "the latest heartbeat counts, not the last one in the slice",
			inventory: []string{"vl-gpu-0412", "vl-gpu-0217"},
			beats: []rollcall.Heartbeat{
				hb("vl-gpu-0412", now.Add(-2*time.Minute)),
				hb("vl-gpu-0217", now.Add(-3*time.Hour)),
				hb("vl-gpu-0412", now.Add(-3*time.Hour)),
				hb("vl-gpu-0217", now.Add(-40*time.Minute)),
				hb("vl-gpu-0217", now.Add(-2*time.Hour)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"vl-gpu-0412"},
				Stale:   []string{"vl-gpu-0217"},
				LastSeen: map[string]time.Time{
					"vl-gpu-0412": now.Add(-2 * time.Minute),
					"vl-gpu-0217": now.Add(-40 * time.Minute),
				},
			},
		},
		{
			name:      "a heartbeat after now is healthy and keeps its time",
			inventory: []string{"vl-gpu-0314", "vl-gpu-0315"},
			beats: []rollcall.Heartbeat{
				hb("vl-gpu-0314", now.Add(45*time.Second)),
				hb("vl-gpu-0315", now.Add(-3*time.Hour)),
				hb("vl-gpu-0315", now.Add(26*time.Hour)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"vl-gpu-0314", "vl-gpu-0315"},
				LastSeen: map[string]time.Time{
					"vl-gpu-0314": now.Add(45 * time.Second),
					"vl-gpu-0315": now.Add(26 * time.Hour),
				},
			},
		},
		{
			// 10:30:00+02:00 is 08:30:00 UTC: exactly 30 minutes before now.
			name:      "heartbeats in +02:00, now in UTC",
			inventory: []string{"vl-gpu-0313", "vl-gpu-0316"},
			beats: []rollcall.Heartbeat{
				hb("vl-gpu-0313", time.Date(2026, 10, 15, 10, 30, 0, 0, cest)),
				hb("vl-gpu-0316", time.Date(2026, 10, 15, 10, 29, 59, 0, cest)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"vl-gpu-0313"},
				Stale:   []string{"vl-gpu-0316"},
				LastSeen: map[string]time.Time{
					"vl-gpu-0313": time.Date(2026, 10, 15, 8, 30, 0, 0, time.UTC),
					"vl-gpu-0316": time.Date(2026, 10, 15, 8, 29, 59, 0, time.UTC),
				},
			},
		},
		{
			// now is 11:00+02:00, the same instant as 09:00 UTC.
			name:      "heartbeats in UTC, now in +02:00",
			inventory: []string{"vl-gpu-0313", "vl-gpu-0316"},
			beats: []rollcall.Heartbeat{
				hb("vl-gpu-0313", time.Date(2026, 10, 15, 8, 30, 0, 0, time.UTC)),
				hb("vl-gpu-0316", time.Date(2026, 10, 15, 8, 29, 59, 0, time.UTC)),
			},
			now:        now.In(cest),
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"vl-gpu-0313"},
				Stale:   []string{"vl-gpu-0316"},
				LastSeen: map[string]time.Time{
					"vl-gpu-0313": time.Date(2026, 10, 15, 8, 30, 0, 0, time.UTC),
					"vl-gpu-0316": time.Date(2026, 10, 15, 8, 29, 59, 0, time.UTC),
				},
			},
		},
		{
			// 08:50 UTC is later than 10:40+02:00 (08:40 UTC), although 8 < 10.
			name:      "the latest heartbeat is decided by instant, not by wall clock",
			inventory: []string{"vl-gpu-0412"},
			beats: []rollcall.Heartbeat{
				hb("vl-gpu-0412", time.Date(2026, 10, 15, 8, 50, 0, 0, time.UTC)),
				hb("vl-gpu-0412", time.Date(2026, 10, 15, 10, 40, 0, 0, cest)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy:  []string{"vl-gpu-0412"},
				LastSeen: map[string]time.Time{"vl-gpu-0412": time.Date(2026, 10, 15, 8, 50, 0, 0, time.UTC)},
			},
		},
		{
			name:      "an unknown node is unknown however old its heartbeat",
			inventory: []string{"canary-h3-01"},
			beats: []rollcall.Heartbeat{
				hb("canary-h3-01", now.Add(-1*time.Minute)),
				hb("vl-gpu-0502", now.Add(-5*time.Hour)),
				hb("canary-h3-02", now.Add(-1*time.Hour)),
				hb("canary-h3-02", now.Add(-5*time.Hour)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"canary-h3-01"},
				Unknown: []string{"canary-h3-02", "vl-gpu-0502"},
				LastSeen: map[string]time.Time{
					"canary-h3-01": now.Add(-1 * time.Minute),
					"vl-gpu-0502":  now.Add(-5 * time.Hour),
					"canary-h3-02": now.Add(-1 * time.Hour),
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
			inventory: []string{"canary-h3-01", "vl-gpu-0412", "canary-h3-01", "vl-gpu-0217", "vl-gpu-0217", "harbour-02", "harbour-02"},
			beats: []rollcall.Heartbeat{
				hb("canary-h3-01", now.Add(-1*time.Minute)),
				hb("vl-gpu-0217", now.Add(-2*time.Hour)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy: []string{"canary-h3-01"},
				Stale:   []string{"vl-gpu-0217"},
				Missing: []string{"harbour-02", "vl-gpu-0412"},
				LastSeen: map[string]time.Time{
					"canary-h3-01": now.Add(-1 * time.Minute),
					"vl-gpu-0217":  now.Add(-2 * time.Hour),
				},
			},
		},
		{
			name:      "many heartbeats of 1 unknown node give 1 name",
			inventory: nil,
			beats: []rollcall.Heartbeat{
				hb("vl-gpu-0501", now.Add(-1*time.Minute)),
				hb("vl-gpu-0501", now.Add(-11*time.Minute)),
				hb("vl-gpu-0501", now.Add(-21*time.Minute)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Unknown:  []string{"vl-gpu-0501"},
				LastSeen: map[string]time.Time{"vl-gpu-0501": now.Add(-1 * time.Minute)},
			},
		},
		{
			name:       "empty names are ignored in the inventory",
			inventory:  []string{"", "canary-h3-01", ""},
			beats:      []rollcall.Heartbeat{hb("canary-h3-01", now.Add(-1*time.Minute))},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy:  []string{"canary-h3-01"},
				LastSeen: map[string]time.Time{"canary-h3-01": now.Add(-1 * time.Minute)},
			},
		},
		{
			name:      "empty names are ignored in the heartbeats",
			inventory: []string{"canary-h3-01"},
			beats: []rollcall.Heartbeat{
				hb("", now.Add(-1*time.Minute)),
				hb("canary-h3-01", now.Add(-2*time.Minute)),
			},
			now:        now,
			staleAfter: 30 * time.Minute,
			want: rollcall.Report{
				Healthy:  []string{"canary-h3-01"},
				LastSeen: map[string]time.Time{"canary-h3-01": now.Add(-2 * time.Minute)},
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
	inventory := []string{"harbour-01", "canary-h3-01", "vl-gpu-0412", "canary-h3-01", "", "vl-gpu-0003"}
	beats := []rollcall.Heartbeat{
		hb("vl-gpu-0412", now.Add(-3*time.Hour)),
		hb("canary-h3-01", now.Add(-1*time.Minute)),
		hb("test-bench-9", now.Add(-2*time.Minute)),
		hb("", now),
		hb("vl-gpu-0412", now.Add(-5*time.Minute)),
		hb("harbour-01", time.Date(2026, 10, 15, 10, 45, 0, 0, cest)),
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
		Healthy: []string{"canary-h3-01", "harbour-01", "vl-gpu-0412"},
		Missing: []string{"vl-gpu-0003"},
		Unknown: []string{"test-bench-9"},
		LastSeen: map[string]time.Time{
			"vl-gpu-0412":  now.Add(-5 * time.Minute),
			"canary-h3-01": now.Add(-1 * time.Minute),
			"test-bench-9": now.Add(-2 * time.Minute),
			"harbour-01":   time.Date(2026, 10, 15, 8, 45, 0, 0, time.UTC),
		},
	})
}
