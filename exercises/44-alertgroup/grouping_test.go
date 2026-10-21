package alertgroup_test

import (
	"maps"
	"testing"
	"time"

	alertgroup "github.com/yornik/go-practice/exercises/44-alertgroup"
)

// Step 2: grouping, dedup and the first notification of a group.
// Run with -run TestGroup.

// Rules 5, 11 and 18: group labels, group key, sort order.
func TestGroupKey(t *testing.T) {
	route := defaultRoute()
	// Not in name order on purpose: the key is GroupLabels.String(), so the
	// order of GroupBy does not matter.
	route.GroupBy = []string{"cluster", "alertname"}

	// Received in an order that is neither sorted by group nor by alert.
	alerts := []alertgroup.Alert{
		firing("alertname=Disk cluster=prod node=b"),
		firing("alertname=Disk cluster=prod node=a"),
		firing("alertname=Disk node=d"), // no cluster label
		firing("alertname=Disk cluster=dev node=a"),
		firing("alertname=CPU cluster=prod node=a"),
		firing("alertname=Disk node=c"), // no cluster label
		firing("alertname=Disk cluster=prod node=a path=/var"),
	}
	run(t, route, nil, []step{
		{at: 0, receive: alerts},
		{at: 30 * time.Second, want: []string{
			// Sorted by GroupKey as strings. ", " sorts before "}", so the
			// keys with a cluster come before {alertname="Disk"}.
			`{alertname="CPU", cluster="prod"} firing=[node=a] resolved=[]`,
			`{alertname="Disk", cluster="dev"} firing=[node=a] resolved=[]`,
			// Sorted by Labels.String(): {..., node="a", path="/var"} comes
			// before {..., node="a"} for the same reason.
			`{alertname="Disk", cluster="prod"} firing=[node=a,path=/var node=a node=b] resolved=[]`,
			// Alerts without a cluster label share a group without it.
			`{alertname="Disk"} firing=[node=c node=d] resolved=[]`,
		}},
	})

	t.Run("GroupLabels", func(t *testing.T) {
		d := alertgroup.New(route, nil)
		for _, a := range alerts {
			d.Receive(a, t0)
		}
		ns := d.Flush(at(30 * time.Second))
		want := []map[string]string{
			{"alertname": "CPU", "cluster": "prod"},
			{"alertname": "Disk", "cluster": "dev"},
			{"alertname": "Disk", "cluster": "prod"},
			{"alertname": "Disk"}, // no "cluster" key at all, not cluster=""
		}
		if len(ns) != len(want) {
			t.Fatalf("Flush returned %d notifications, want %d:%s", len(ns), len(want), show(render(ns, nil)))
		}
		for i, n := range ns {
			if got := map[string]string(n.GroupLabels); !maps.Equal(got, want[i]) {
				t.Errorf("notification %d (GroupKey %s): GroupLabels = %v, want %v", i, n.GroupKey, got, want[i])
			}
		}
	})
}

// Rule 5: empty GroupBy puts every alert in one group.
func TestGroupEmptyGroupBy(t *testing.T) {
	route := defaultRoute()
	route.GroupBy = nil
	run(t, route, nil, []step{
		{at: 0, receive: []alertgroup.Alert{
			firing("alertname=Disk node=a"),
			firing("alertname=CPU node=b"),
		}},
		{at: 30 * time.Second, want: []string{
			`{} firing=[alertname=CPU,node=b alertname=Disk,node=a] resolved=[]`,
		}},
	})

	d := alertgroup.New(route, nil)
	d.Receive(firing("alertname=Disk node=a"), t0)
	n := flushOne(t, d, 30*time.Second)
	if len(n.GroupLabels) != 0 {
		t.Errorf("GroupLabels = %v, want empty", map[string]string(n.GroupLabels))
	}
}

// Rules 6 and 11: GroupWait starts when the group is created, at the Receive
// of its first alert. Later alerts join the group without restarting the
// wait. The group notifies at creation + GroupWait, not 1 ns earlier, and once.
func TestGroupWait(t *testing.T) {
	run(t, defaultRoute(), nil, []step{
		// Creates the Disk group. StartsAt is t0-1m, but the wait counts
		// from now.
		{at: 0, receive: []alertgroup.Alert{firing("alertname=Disk node=a")}},
		{at: 10 * time.Second, receive: []alertgroup.Alert{firing("alertname=Disk node=b")}},
		// Creates the CPU group: it notifies at 20s + 30s = 50s.
		{at: 20 * time.Second, receive: []alertgroup.Alert{firing("alertname=CPU node=a")}},
		{at: 30*time.Second - time.Nanosecond},
		{at: 30 * time.Second, want: []string{
			`{alertname="Disk"} firing=[node=a node=b] resolved=[]`,
		}},
		{at: 31 * time.Second},
		{at: 49 * time.Second},
		{at: 50 * time.Second, want: []string{
			`{alertname="CPU"} firing=[node=a] resolved=[]`,
		}},
		{at: time.Minute},
	})

	// Flush is not called every second in real life. The first Flush after
	// the wait sends the notification.
	run(t, defaultRoute(), nil, []step{
		{at: 0, receive: []alertgroup.Alert{firing("alertname=Disk node=a")}},
		{at: 47 * time.Second, want: []string{
			`{alertname="Disk"} firing=[node=a] resolved=[]`,
		}},
	})
}

// Rule 4: the same labels are the same alert. An update takes the new EndsAt
// and keeps the earliest StartsAt.
func TestGroupDedup(t *testing.T) {
	d := alertgroup.New(defaultRoute(), nil)
	sends := []struct {
		now              time.Duration
		startsAt, endsAt time.Time
	}{
		{0, t0.Add(-time.Minute), t0.Add(10 * time.Minute)},                  // valid until t0+10m
		{5 * time.Second, t0.Add(5 * time.Second), t0.Add(12 * time.Minute)}, // later StartsAt: keep the old one
		{10 * time.Second, t0.Add(-3 * time.Minute), t0.Add(15 * time.Minute)},
		{20 * time.Second, t0.Add(-time.Minute), time.Time{}}, // zero EndsAt: firing until further notice
	}
	for i, s := range sends {
		// A new map every time, filled in a different order: the
		// fingerprint makes them the same alert.
		l := alertgroup.Labels{}
		if i%2 == 0 {
			l["alertname"], l["node"] = "Disk", "a"
		} else {
			l["node"], l["alertname"] = "a", "Disk"
		}
		d.Receive(alertgroup.Alert{Labels: l, StartsAt: s.startsAt, EndsAt: s.endsAt}, at(s.now))
	}

	n := flushOne(t, d, 30*time.Second)
	if len(n.Firing) != 1 {
		t.Fatalf("Firing has %d alerts, want 1 (4 receives of the same labels):%s",
			len(n.Firing), show(render([]alertgroup.Notification{n}, nil)))
	}
	got := n.Firing[0]
	if want := t0.Add(-3 * time.Minute); !got.StartsAt.Equal(want) {
		t.Errorf("StartsAt = %v, want %v (the earliest)", got.StartsAt, want)
	}
	if !got.EndsAt.IsZero() {
		t.Errorf("EndsAt = %v, want the zero time (from the last receive)", got.EndsAt)
	}
	if want := map[string]string{"alertname": "Disk", "node": "a"}; !maps.Equal(map[string]string(got.Labels), want) {
		t.Errorf("Labels = %v, want %v", map[string]string(got.Labels), want)
	}
}

// Rule 7: the caller may reuse its labels map after Receive.
func TestGroupReceiveCopiesLabels(t *testing.T) {
	d := alertgroup.New(defaultRoute(), nil)
	l := alertgroup.Labels{"alertname": "Disk", "node": "a"}
	d.Receive(alertgroup.Alert{Labels: l, StartsAt: startedAt}, t0)
	l["node"] = "b"
	l["extra"] = "x"

	n := flushOne(t, d, 30*time.Second)
	want := map[string]string{"alertname": "Disk", "node": "a"}
	if len(n.Firing) != 1 || !maps.Equal(map[string]string(n.Firing[0].Labels), want) {
		t.Fatalf("after the caller changed its map, Flush returned%s\nwant one alert with labels %v",
			show(render([]alertgroup.Notification{n}, nil)), want)
	}
}
