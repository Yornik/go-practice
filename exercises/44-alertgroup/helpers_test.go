package alertgroup_test

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	alertgroup "github.com/yornik/go-practice/exercises/44-alertgroup"
)

// The tests never sleep. Every call gets its time as an argument, so a
// scenario can cover 8 hours in a microsecond. All times are offsets from t0.

// t0 is the start of every scenario: 03:00 UTC, when pages hurt most.
var t0 = time.Date(2026, 10, 17, 3, 0, 0, 0, time.UTC)

// Short names for the durations in the scenarios.
const (
	second = time.Second
	minute = time.Minute
	hour   = time.Hour
)

// at returns t0 + d.
func at(d time.Duration) time.Time { return t0.Add(d) }

// startedAt is the StartsAt of the alerts the helpers build: Prometheus saw
// the problem 1 minute before t0. It is earlier than every Receive on
// purpose: GroupWait counts from the Receive that created the group, not from
// StartsAt (rule 6).
var startedAt = t0.Add(-time.Minute)

// defaultRoute is the route most tests use: one group per alertname, and
// Alertmanager's default timings.
func defaultRoute() alertgroup.Route {
	return alertgroup.Route{
		GroupBy:        []string{"alertname"},
		GroupWait:      30 * time.Second,
		GroupInterval:  5 * time.Minute,
		RepeatInterval: 4 * time.Hour,
	}
}

// ls parses "name=value name=value" into Labels. Values in the tests have no
// spaces.
func ls(s string) alertgroup.Labels {
	l := alertgroup.Labels{}
	for _, f := range strings.Fields(s) {
		name, value, _ := strings.Cut(f, "=")
		l[name] = value
	}
	return l
}

// firing returns a firing alert (zero EndsAt) with the labels s.
func firing(s string) alertgroup.Alert {
	return alertgroup.Alert{Labels: ls(s), StartsAt: startedAt}
}

// endsAt returns an alert with the labels s and EndsAt t0+d. Received at a
// time >= t0+d it resolves the alert. Received earlier, it is a "valid until"
// in the future, and the alert is firing until then (rule 3).
func endsAt(d time.Duration, s string) alertgroup.Alert {
	return alertgroup.Alert{Labels: ls(s), StartsAt: startedAt, EndsAt: at(d)}
}

// step is one moment of a scenario. At t0+at the Dispatcher receives the
// alerts in receive, in order. Then Flush(t0+at) runs and must return exactly
// want, one rendered line per notification (see render). An empty want means
// that nothing is due.
type step struct {
	at      time.Duration
	receive []alertgroup.Alert
	want    []string
}

// run plays the steps against a new Dispatcher. It stops at the first step
// whose Flush result differs, because every later step depends on the state.
func run(t *testing.T, route alertgroup.Route, rules []alertgroup.InhibitRule, steps []step) {
	t.Helper()
	d := alertgroup.New(route, rules)
	for _, s := range steps {
		now := at(s.at)
		for _, a := range s.receive {
			d.Receive(a, now)
		}
		got := render(d.Flush(now), route.GroupBy)
		if !slices.Equal(got, s.want) {
			t.Fatalf("Flush(t0+%v) returned%s\nwant%s", s.at, show(got), show(s.want))
		}
	}
}

// render turns each notification into one line that is easy to read and
// compare, for example
//
//	{alertname="Disk"} firing=[node=a node=b] resolved=[node=c]
//
// The group key is printed as Flush returned it. Each alert is printed as its
// labels in name order, without the labels named in hide (the GroupBy labels:
// the key already shows them). An alert with no other labels prints as {}.
// The alerts keep the order Flush returned them in.
func render(ns []alertgroup.Notification, hide []string) []string {
	var lines []string
	for _, n := range ns {
		lines = append(lines, fmt.Sprintf("%s firing=[%s] resolved=[%s]",
			n.GroupKey, renderAlerts(n.Firing, hide), renderAlerts(n.Resolved, hide)))
	}
	return lines
}

func renderAlerts(alerts []alertgroup.Alert, hide []string) string {
	var parts []string
	for _, a := range alerts {
		var kv []string
		for _, name := range slices.Sorted(maps.Keys(a.Labels)) {
			if !slices.Contains(hide, name) {
				kv = append(kv, name+"="+a.Labels[name])
			}
		}
		if len(kv) == 0 {
			parts = append(parts, "{}")
			continue
		}
		parts = append(parts, strings.Join(kv, ","))
	}
	return strings.Join(parts, " ")
}

// show prints rendered notifications one per line, for failure messages.
func show(lines []string) string {
	if len(lines) == 0 {
		return "\n    (no notifications)"
	}
	return "\n    " + strings.Join(lines, "\n    ")
}

// flushOne calls Flush and fails the test unless it returns exactly one
// notification.
func flushOne(t *testing.T, d *alertgroup.Dispatcher, now time.Duration) alertgroup.Notification {
	t.Helper()
	ns := d.Flush(at(now))
	if len(ns) != 1 {
		t.Fatalf("Flush(t0+%v) returned %d notifications, want 1:%s", now, len(ns), show(render(ns, nil)))
	}
	return ns[0]
}
