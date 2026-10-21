package silences_test

import (
	"testing"
	"time"

	silences "github.com/yornik/go-practice/exercises/23-silences"
)

// Step 2: Match.

// query is 1 call of Match. want is the ID of the silence it must return;
// "" means no match.
type query struct {
	alert, target string
	want          string
}

func TestMatch(t *testing.T) {
	const h1, h2, h3 = time.Hour, 2 * time.Hour, 3 * time.Hour
	cases := []struct {
		name    string
		add     []silences.Silence // added at t0, in this order
		advance time.Duration      // the clock moves this far before the queries
		queries []query
	}{
		{"empty store", nil, 0, []query{
			{"disk_full", "chkpt-store-2", ""},
		}},
		{"exact alert and target", []silences.Silence{sil("disk-2", "disk_full", "chkpt-store-2", h1)}, 0, []query{
			{"disk_full", "chkpt-store-2", "disk-2"},
			{"disk_full", "chkpt-store-1", ""},
			{"node_offline", "chkpt-store-2", ""},
			{"disk_full", "", ""},
			{"", "chkpt-store-2", ""},
		}},
		{"empty Alert matches every alert", []silences.Silence{sil("upgrade", "", "chkpt-store-2", h1)}, 0, []query{
			{"disk_full", "chkpt-store-2", "upgrade"},
			{"high_latency", "chkpt-store-2", "upgrade"},
			{"disk_full", "chkpt-store-3", ""},
		}},
		{"empty Target matches every target", []silences.Silence{sil("disks", "disk_full", "", h1)}, 0, []query{
			{"disk_full", "chkpt-store-1", "disks"},
			{"disk_full", "vl-gpu-0412", "disks"},
			{"node_offline", "vl-gpu-0412", ""},
		}},
		{"the latest Until wins", []silences.Silence{
			sil("a", "disk_full", "chkpt-store-2", h1),
			sil("b", "disk_full", "chkpt-store-2", h3),
			sil("c", "disk_full", "chkpt-store-2", h2),
		}, 0, []query{
			{"disk_full", "chkpt-store-2", "b"},
		}},
		{"the latest Until wins over a more exact silence", []silences.Silence{
			sil("exact", "disk_full", "chkpt-store-2", h1),
			sil("any-alert", "", "chkpt-store-2", h2),
			sil("any-target", "disk_full", "", h3),
		}, 0, []query{
			{"disk_full", "chkpt-store-2", "any-target"},
			{"high_latency", "chkpt-store-2", "any-alert"},
		}},
		{"a tie on Until goes to the smallest ID", []silences.Silence{
			sil("disk-d", "disk_full", "chkpt-store-2", h2),
			sil("disk-b", "disk_full", "chkpt-store-2", h2),
			sil("disk-f", "", "chkpt-store-2", h2),
			sil("disk-a", "disk_full", "", h2),
			sil("disk-e", "disk_full", "", h2),
			sil("disk-c", "", "chkpt-store-2", h2),
			sil("Disk-z", "disk_full", "chkpt-store-2", h1),
		}, 0, []query{
			{"disk_full", "chkpt-store-2", "disk-a"},
		}},
		{"1 ns before Until the silence is active", []silences.Silence{sil("disk-2", "disk_full", "chkpt-store-2", h1)}, h1 - time.Nanosecond, []query{
			{"disk_full", "chkpt-store-2", "disk-2"},
		}},
		{"at Until the silence has ended", []silences.Silence{sil("disk-2", "disk_full", "chkpt-store-2", h1)}, h1, []query{
			{"disk_full", "chkpt-store-2", ""},
		}},
		{"an ended silence is ignored before Expire removes it", []silences.Silence{
			sil("disk-2", "disk_full", "chkpt-store-2", h1),
			sil("disks", "disk_full", "", h3),
		}, h2, []query{
			{"disk_full", "chkpt-store-2", "disks"},
			{"disk_full", "chkpt-store-1", "disks"},
		}},
		{"the replacement counts, not the old silence", []silences.Silence{
			sil("disk-2", "disk_full", "chkpt-store-2", h1),
			sil("disk-2", "node_offline", "", h2),
		}, 0, []query{
			{"disk_full", "chkpt-store-2", ""},
			{"node_offline", "vl-gpu-0412", "disk-2"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for run := range runs {
				h := newHarness(t)
				h.mustAdd(tc.add...)
				h.clock.Advance(tc.advance)
				for _, q := range tc.queries {
					if q.want == "" {
						h.wantNoMatch(q.alert, q.target)
						continue
					}
					h.wantMatch(q.alert, q.target, latest(tc.add, q.want))
				}
				h.wantLen(len(ids(tc.add))) // Match removes nothing
				if t.Failed() {
					t.Fatalf("failed on fresh store %d of %d", run+1, runs)
				}
			}
		})
	}
}

// latest returns the last silence in list with the ID id: the one the
// store holds after the list was added in order.
func latest(list []silences.Silence, id string) silences.Silence {
	var found silences.Silence
	for _, s := range list {
		if s.ID == id {
			found = s
		}
	}
	return found
}

// ids returns the distinct IDs in list.
func ids(list []silences.Silence) map[string]bool {
	set := map[string]bool{}
	for _, s := range list {
		set[s.ID] = true
	}
	return set
}

func TestMatchExtend(t *testing.T) {
	// The "extend 1 h" button adds a silence with the same ID and a later
	// Until.
	h := newHarness(t)
	h.mustAdd(sil("disk-2", "disk_full", "chkpt-store-2", time.Hour))
	h.clock.Advance(50 * time.Minute)
	extended := sil("disk-2", "disk_full", "chkpt-store-2", 110*time.Minute)
	h.mustAdd(extended)
	h.clock.Advance(30 * time.Minute) // t0+80m: after the old Until
	h.wantMatch("disk_full", "chkpt-store-2", extended)
	h.clock.Advance(30 * time.Minute) // t0+110m: the new Until
	h.wantNoMatch("disk_full", "chkpt-store-2")
}

func TestMatchAfterRejectedAdd(t *testing.T) {
	h := newHarness(t)
	old := sil("disk-2", "disk_full", "chkpt-store-2", time.Hour)
	h.mustAdd(old)
	h.clock.Advance(30 * time.Minute)
	h.addInvalid(sil("disk-2", "disk_full", "chkpt-store-2", 10*time.Minute)) // ends before now
	h.addInvalid(sil("disk-2", "", "", 2*time.Hour))
	h.wantMatch("disk_full", "chkpt-store-2", old)
}
