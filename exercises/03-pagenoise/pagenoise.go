// Package pagenoise reads an export of on-call pages, 1 page per line:
//
//	2026-09-23 03:12 AirflowLow canary-h3-01
//
// and counts them per alert, so that a team can see which alerts page most
// and which ones page at night.
package pagenoise

// Page is 1 page: 1 alert that notified the person on call.
type Page struct {
	Day    string // "2026-09-14", as written; not checked
	Hour   int    // 0 to 23, the team's local time
	Minute int    // 0 to 59
	Alert  string
	Source string
}

// ParsePage reads a line "<day> <HH:MM> <alert> <source>".
//
// The line has exactly 4 fields, as strings.Fields splits them. The time is
// exactly 5 bytes: 2 ASCII digits, a colon and 2 ASCII digits, with an hour
// from 00 to 23 and a minute from 00 to 59. The day, the alert and the source
// are kept as written.
//
// On a bad line ParsePage returns the zero Page and an error that contains
// the whole line quoted with %q.
func ParsePage(line string) (Page, error) {
	panic("TODO: implement")
}

// AlertCount is what Rank reports for 1 alert.
type AlertCount struct {
	Alert   string
	Pages   int      // every page of this alert
	Night   int      // pages from 22:00 up to 06:59
	Sources []string // distinct sources, A to Z
}

// Rank returns 1 AlertCount per alert: most Pages first, then most Night,
// then Alert A to Z.
//
// Rank does not modify pages, and the same input always gives the same
// output. No pages give an empty result (nil or empty).
func Rank(pages []Page) []AlertCount {
	panic("TODO: implement")
}

// Pair is an alert together with the source that sent it.
type Pair struct{ Alert, Source string }

// CountPairs counts the pages per alert and source. It never returns nil.
func CountPairs(pages []Page) map[Pair]int {
	panic("TODO: implement")
}
