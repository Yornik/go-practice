package alertgroup_test

import (
	"maps"
	"testing"
	"time"

	alertgroup "github.com/yornik/go-practice/exercises/19-alertgroup"
)

// Step 3: what happens after the first notification. Run with -run TestTiming.
// All scenarios use defaultRoute: GroupWait 30s, GroupInterval 5m,
// RepeatInterval 4h, one group per alertname.

// Rule 12, first case: after a notification, changes wait for GroupInterval
// and are then sent together. GroupInterval is the minimum gap between two
// notifications, not a fixed tick.
func TestTimingGroupInterval(t *testing.T) {
	run(t, defaultRoute(), nil, []step{
		{at: 0, receive: []alertgroup.Alert{firing("alertname=Disk node=a")}},
		{at: 30 * second, want: []string{
			`{alertname="Disk"} firing=[node=a] resolved=[]`,
		}},
		// New alerts: a change, but the last notification was at 30s.
		{at: 1 * minute, receive: []alertgroup.Alert{firing("alertname=Disk node=b")}},
		// Prometheus re-sends a with every evaluation. That is the same
		// alert, not a change (rule 4).
		{at: 2 * minute, receive: []alertgroup.Alert{firing("alertname=Disk node=c"), firing("alertname=Disk node=a")}},
		{at: 5*minute + 30*second - time.Nanosecond},
		{at: 5*minute + 30*second, want: []string{
			`{alertname="Disk"} firing=[node=a node=b node=c] resolved=[]`,
		}},
		{at: 8 * minute, receive: []alertgroup.Alert{
			firing("alertname=Disk node=a"), firing("alertname=Disk node=b"), firing("alertname=Disk node=c"),
		}},
		// GroupInterval has passed, but nothing changed: no notification.
		{at: 10*minute + 30*second},
		// The gap since the last notification is already longer than
		// GroupInterval, so a change is sent at once.
		{at: 12 * minute, receive: []alertgroup.Alert{firing("alertname=Disk node=d")}, want: []string{
			`{alertname="Disk"} firing=[node=a node=b node=c node=d] resolved=[]`,
		}},
		{at: 12*minute + 1*second, receive: []alertgroup.Alert{firing("alertname=Disk node=e")}},
		{at: 16*minute + 59*second},
		{at: 17 * minute, want: []string{
			`{alertname="Disk"} firing=[node=a node=b node=c node=d node=e] resolved=[]`,
		}},
	})
}

// Rule 12, second case: an unchanged group with firing alerts repeats its
// notification RepeatInterval after the last notification, whatever kind
// that one was.
func TestTimingRepeatInterval(t *testing.T) {
	run(t, defaultRoute(), nil, []step{
		{at: 0, receive: []alertgroup.Alert{firing("alertname=Disk node=a")}},
		{at: 30 * second, want: []string{
			`{alertname="Disk"} firing=[node=a] resolved=[]`,
		}},
		{at: 2 * hour},
		{at: 4*hour + 29*second},
		{at: 4*hour + 30*second, want: []string{
			`{alertname="Disk"} firing=[node=a] resolved=[]`,
		}},
		{at: 4*hour + 1*minute, receive: []alertgroup.Alert{firing("alertname=Disk node=b")}},
		{at: 4*hour + 5*minute + 30*second, want: []string{
			`{alertname="Disk"} firing=[node=a node=b] resolved=[]`,
		}},
		// 4h after the repeat at 4h0m30s, but the last notification was the
		// change at 4h5m30s.
		{at: 8*hour + 30*second},
		{at: 8*hour + 5*minute + 30*second, want: []string{
			`{alertname="Disk"} firing=[node=a node=b] resolved=[]`,
		}},
	})
}

// Rules 9, 12, 13 and 14: a resolved alert that the receiver saw firing is
// reported once, at the next GroupInterval, and then forgotten. A group
// without alerts is deleted, and the next alert with its key starts over
// with GroupWait.
func TestTimingResolved(t *testing.T) {
	run(t, defaultRoute(), nil, []step{
		{at: 0, receive: []alertgroup.Alert{
			firing("alertname=Disk node=d"), firing("alertname=Disk node=c"),
			firing("alertname=Disk node=b"), firing("alertname=Disk node=a"),
		}},
		{at: 30 * second, want: []string{
			`{alertname="Disk"} firing=[node=a node=b node=c node=d] resolved=[]`,
		}},
		// Prometheus resolves a, c and d: it sends them again with EndsAt = now.
		{at: 2 * minute, receive: []alertgroup.Alert{
			endsAt(2*minute, "alertname=Disk node=d"),
			endsAt(2*minute, "alertname=Disk node=c"),
			endsAt(2*minute, "alertname=Disk node=a"),
		}},
		{at: 5*minute + 29*second},
		// Resolved is sorted too (rule 18), whatever order the alerts came in.
		{at: 5*minute + 30*second, want: []string{
			`{alertname="Disk"} firing=[node=b] resolved=[node=a node=c node=d]`,
		}},
		{at: 6 * minute},
		{at: 10*minute + 30*second},
		{at: 11 * minute, receive: []alertgroup.Alert{endsAt(11*minute, "alertname=Disk node=b")}, want: []string{
			`{alertname="Disk"} firing=[] resolved=[node=b]`,
		}},
		// The Disk group was deleted at 11m. This alert creates a new one.
		{at: 11*minute + 10*second, receive: []alertgroup.Alert{firing("alertname=Disk node=c")}},
		{at: 11*minute + 39*second},
		{at: 11*minute + 40*second, want: []string{
			`{alertname="Disk"} firing=[node=c] resolved=[]`,
		}},
	})
}

// Rules 3 and 4: an EndsAt in the future is a "valid until". Prometheus
// re-sends an alert that is still firing to push it further out. An alert
// that is not re-sent resolves by itself at its EndsAt. When the problem is
// gone, Prometheus sends EndsAt = now, which is earlier than the valid until
// it sent before: the new EndsAt wins.
func TestTimingValidUntil(t *testing.T) {
	run(t, defaultRoute(), nil, []step{
		{at: 0, receive: []alertgroup.Alert{
			endsAt(3*minute, "alertname=Disk node=a"),
			endsAt(5*minute+30*second, "alertname=Disk node=b"),
		}},
		{at: 30 * second, want: []string{
			`{alertname="Disk"} firing=[node=a node=b] resolved=[]`,
		}},
		// a is still firing: Prometheus extends it. b is not re-sent.
		{at: 2 * minute, receive: []alertgroup.Alert{endsAt(6*minute, "alertname=Disk node=a")}},
		{at: 3 * minute},
		{at: 5 * minute, receive: []alertgroup.Alert{endsAt(9*minute, "alertname=Disk node=a")}},
		// b's EndsAt is exactly now: it is resolved.
		{at: 5*minute + 30*second, want: []string{
			`{alertname="Disk"} firing=[node=a] resolved=[node=b]`,
		}},
		{at: 8 * minute, receive: []alertgroup.Alert{endsAt(12*minute, "alertname=Disk node=a")}},
		// a is fixed: Prometheus resolves it before its valid until of 12m.
		{at: 9 * minute, receive: []alertgroup.Alert{endsAt(9*minute, "alertname=Disk node=a")}},
		{at: 10*minute + 29*second},
		{at: 10*minute + 30*second, want: []string{
			`{alertname="Disk"} firing=[] resolved=[node=a]`,
		}},
	})
}

// Rules 10, 12 and 14: an alert that resolves before any notification listed
// it as firing never shows up, and it is not a change.
func TestTimingFlap(t *testing.T) {
	run(t, defaultRoute(), nil, []step{
		{at: 0, receive: []alertgroup.Alert{firing("alertname=Disk node=a")}},
		// a flaps: resolved within GroupWait. Nobody has heard of it, so it
		// is dropped, and the empty group is deleted.
		{at: 10 * second, receive: []alertgroup.Alert{endsAt(10*second, "alertname=Disk node=a")}},
		{at: 30 * second},
		// a fires again: a new group, created now.
		{at: 40 * second, receive: []alertgroup.Alert{firing("alertname=Disk node=a")}},
		{at: 1*minute + 9*second},
		{at: 1*minute + 10*second, want: []string{
			`{alertname="Disk"} firing=[node=a] resolved=[]`,
		}},
		// b arrives already resolved. c fires and resolves between two
		// notifications. The receiver never sees either of them.
		{at: 2 * minute, receive: []alertgroup.Alert{endsAt(1*minute, "alertname=Disk node=b")}},
		{at: 3 * minute, receive: []alertgroup.Alert{firing("alertname=Disk node=c")}},
		{at: 4 * minute, receive: []alertgroup.Alert{endsAt(4*minute, "alertname=Disk node=c")}},
		{at: 6*minute + 10*second},
		{at: 4*hour + 1*minute + 10*second, want: []string{
			`{alertname="Disk"} firing=[node=a] resolved=[]`,
		}},
	})
}

// Rule 19: the caller owns what Flush returns. A receiver that adds its own
// labels or edits an alert must not change the Dispatcher's state.
func TestTimingCallerOwnsNotifications(t *testing.T) {
	d := alertgroup.New(defaultRoute(), nil)
	d.Receive(firing("alertname=Disk node=a"), t0)

	n := flushOne(t, d, 30*second)
	if len(n.Firing) != 1 || n.GroupLabels == nil {
		t.Fatalf("Flush(t0+30s) returned%s\nwant one firing alert and GroupLabels",
			show(render([]alertgroup.Notification{n}, nil)))
	}
	n.GroupLabels["receiver"] = "pagerduty"
	n.Firing[0].Labels["node"] = "changed"
	n.Firing[0].EndsAt = t0 // would resolve the alert, if this were the Dispatcher's copy

	n = flushOne(t, d, 4*hour+30*second) // the repeat
	if got := render([]alertgroup.Notification{n}, nil); got[0] != `{alertname="Disk"} firing=[alertname=Disk,node=a] resolved=[]` {
		t.Errorf("after the caller changed the first notification, the repeat is%s\nwant%s",
			show(got), show([]string{`{alertname="Disk"} firing=[alertname=Disk,node=a] resolved=[]`}))
	}
	if want := map[string]string{"alertname": "Disk"}; !maps.Equal(map[string]string(n.GroupLabels), want) {
		t.Errorf("GroupLabels of the repeat = %v, want %v", map[string]string(n.GroupLabels), want)
	}
}
