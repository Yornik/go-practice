// Package rollcall compares the feeders that should report with the feeders
// that do: it parses heartbeat lines such as
//
//	hq-window-01 2026-10-15T08:55:00Z
//
// and sorts every feeder of an inventory into healthy, stale and missing,
// plus the feeders that report but are not in the inventory.
package rollcall

import (
	"errors"
	"time"
)

// ErrMalformed is wrapped by the error ParseHeartbeat returns for a line it
// cannot read. Check for it with errors.Is.
var ErrMalformed = errors.New("rollcall: malformed heartbeat")

// Heartbeat is 1 report of 1 feeder: the feeder was alive at At.
type Heartbeat struct {
	Feeder string
	At     time.Time
}

// ParseHeartbeat reads a line "<feeder> <RFC 3339 time>", for example
// "hq-window-01 2026-10-15T08:55:00Z".
//
// The line must have exactly 2 fields separated by whitespace, as
// strings.Fields splits them. The time must parse with time.RFC3339, so a zone
// ("Z" or an offset such as "+02:00") is required and fractional seconds are
// fine.
//
// On a malformed line it returns the zero Heartbeat and an error that wraps
// ErrMalformed and contains the whole line quoted with %q.
func ParseHeartbeat(line string) (Heartbeat, error) {
	panic("TODO: implement")
}

// Report is the result of a roll call. Every list is sorted A to Z (plain
// string order) and holds each name once. An empty list may be nil.
type Report struct {
	Healthy  []string             // in the inventory, last heartbeat at most staleAfter before now
	Stale    []string             // in the inventory, last heartbeat more than staleAfter before now
	Missing  []string             // in the inventory, no heartbeat at all
	Unknown  []string             // sent heartbeats, not in the inventory
	LastSeen map[string]time.Time // latest heartbeat of every feeder that sent one
}

// RollCall compares the inventory (the feeders that should report) with the
// heartbeats (the feeders that did report).
//
// The last heartbeat of a feeder is its latest At, whatever its position in
// beats. A feeder of the inventory is healthy when now.Sub(last) <= staleAfter,
// so a heartbeat after now (clock skew) is healthy too. RollCall uses now and
// never reads the clock.
//
// Empty feeder names are ignored, in the inventory and in beats. RollCall does
// not modify its inputs. LastSeen is never nil. The README lists every rule.
func RollCall(inventory []string, beats []Heartbeat, now time.Time, staleAfter time.Duration) Report {
	panic("TODO: implement")
}
