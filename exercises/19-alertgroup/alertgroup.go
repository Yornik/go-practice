// Package alertgroup is the core of an alert manager. It deduplicates alerts,
// groups them, decides when each group sends a notification, and mutes
// symptoms while their cause is firing (inhibition). It is driven by explicit
// timestamps instead of timers, so it is deterministic and easy to test.
package alertgroup

import "time"

// Route says how alerts are grouped and how often a group may notify.
type Route struct {
	// GroupBy lists the label names that form a group. Alerts with equal
	// values for these labels go into the same group and notification.
	GroupBy []string
	// GroupWait is how long a new group waits before its first notification,
	// so that related alerts arriving shortly after the first one, and the
	// alerts that inhibit them, are in it too.
	GroupWait time.Duration
	// GroupInterval is the minimum time between a notification and the next
	// one that reports a change in the group.
	GroupInterval time.Duration
	// RepeatInterval is how long after a notification an unchanged group with
	// firing alerts notifies again, as a reminder.
	RepeatInterval time.Duration
}

// InhibitRule mutes target alerts while a matching source alert is firing.
// The README lists the exact conditions.
type InhibitRule struct {
	SourceMatch Labels   // labels a source alert must have, e.g. {alertname="NodeDown"}
	TargetMatch Labels   // labels a target alert must have, e.g. {severity="warning"}
	Equal       []string // labels that must have equal values in source and target
}

// Notification is one message to the receiver about one group.
type Notification struct {
	GroupKey    string // GroupLabels.String()
	GroupLabels Labels
	Firing      []Alert // every firing, unmuted alert of the group
	Resolved    []Alert // alerts that the last notification listed as firing and that are resolved now
}

// Dispatcher holds the alerts and their groups. It is not safe for concurrent
// use: the caller serializes the calls and passes non-decreasing times.
type Dispatcher struct {
	// TODO: add fields.
}

// New returns a Dispatcher that groups alerts with r and mutes them with
// rules. rules may be nil.
func New(r Route, rules []InhibitRule) *Dispatcher {
	panic("TODO: implement")
}

// Receive stores a new alert or updates a known one at time now.
//
// Alerts with the same fingerprint are the same alert: an update takes the
// new EndsAt and keeps the earlier StartsAt. A new alert goes into the group
// of its group labels; if that group does not exist, Receive creates it with
// now as its creation time. Receive keeps its own copy of the labels.
func (d *Dispatcher) Receive(a Alert, now time.Time) {
	panic("TODO: implement")
}

// Flush returns the notifications that are due at now and updates the state:
// it records what it sent, removes resolved alerts that were reported or that
// nobody has heard of, and deletes groups without alerts.
//
// Notifications are sorted by GroupKey, and the alerts in Firing and Resolved
// by their labels' String. The returned values are copies: changing them does
// not change the Dispatcher. If nothing is due, Flush returns an empty or nil
// slice. The README lists the exact timing and inhibition rules.
func (d *Dispatcher) Flush(now time.Time) []Notification {
	panic("TODO: implement")
}
