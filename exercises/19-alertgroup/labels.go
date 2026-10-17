package alertgroup

import "time"

// Labels is the set of labels of an alert, such as
// {alertname="NodeDown", node="n1"}. The labels are the alert's identity.
type Labels map[string]string

// String returns the labels sorted by name in the Prometheus notation:
// {alertname="NodeDown", node="n1"}. Labels are separated by ", " and values
// are quoted like strconv.Quote. An empty or nil Labels gives "{}".
func (l Labels) String() string {
	panic("TODO: implement")
}

// Fingerprint returns the FNV-1a 64-bit hash of this byte stream: for every
// label, sorted by name, the name, a byte 0xff, the value, a byte 0xff.
// Empty or nil Labels hash no bytes at all. This is the same value as
// Prometheus's LabelSet.Fingerprint. Equal labels always give the same
// fingerprint, whatever order the map was filled in.
func (l Labels) Fingerprint() uint64 {
	panic("TODO: implement")
}

// Alert is one alert as Prometheus sends it.
type Alert struct {
	Labels   Labels
	StartsAt time.Time
	// EndsAt is when the alert stops firing. The zero time means "firing until
	// further notice". Prometheus usually sets it a few minutes ahead, as a
	// "valid until", and re-sends the alert to extend it; when the problem is
	// gone it sends the alert once more with EndsAt set to the resolve time.
	EndsAt time.Time
}

// ResolvedAt reports whether the alert is resolved at now: EndsAt is not the
// zero time and is not after now.
func (a Alert) ResolvedAt(now time.Time) bool {
	panic("TODO: implement")
}
