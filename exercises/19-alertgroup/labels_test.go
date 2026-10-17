package alertgroup_test

import (
	"testing"
	"time"

	alertgroup "github.com/yornik/go-practice/exercises/19-alertgroup"
)

// Step 1: labels.go. Run with -run 'TestLabels|TestAlert'.

// Rule 1.
func TestLabelsString(t *testing.T) {
	tests := []struct {
		name   string
		labels alertgroup.Labels
		want   string
	}{
		{"nil", nil, `{}`},
		{"empty", alertgroup.Labels{}, `{}`},
		{"one label", alertgroup.Labels{"alertname": "NodeDown"}, `{alertname="NodeDown"}`},
		{
			"sorted by name, separated by comma and space",
			alertgroup.Labels{"severity": "critical", "alertname": "NodeDown", "node": "n1"},
			`{alertname="NodeDown", node="n1", severity="critical"}`,
		},
		{
			"values quoted like strconv.Quote",
			alertgroup.Labels{"msg": `say "hi"`, "path": `C:\tmp`, "empty": ""},
			`{empty="", msg="say \"hi\"", path="C:\\tmp"}`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Map iteration order is random, so one call can be sorted by luck.
			// Twenty calls in a row cannot.
			for range 20 {
				if got := tc.labels.String(); got != tc.want {
					t.Fatalf("String() = %s, want %s", got, tc.want)
				}
			}
		})
	}
}

// Rule 2.
func TestLabelsFingerprint(t *testing.T) {
	// want is the FNV-1a 64-bit hash of stream. You can check a value
	// yourself: hash/fnv's New64a, Write the stream, Sum64.
	tests := []struct {
		name   string
		labels alertgroup.Labels
		stream string
		want   uint64
	}{
		{"nil: no bytes", nil, "", 14695981039346656037},
		{"empty: no bytes", alertgroup.Labels{}, "", 14695981039346656037},
		{"one label", alertgroup.Labels{"alertname": "NodeDown"}, "alertname\xffNodeDown\xff", 4523362236545305012},
		{
			"sorted by name",
			alertgroup.Labels{"severity": "critical", "node": "n1", "alertname": "NodeDown"},
			"alertname\xffNodeDown\xffnode\xffn1\xffseverity\xffcritical\xff",
			5078930053942210315,
		},
		{"empty value", alertgroup.Labels{"a": ""}, "a\xff\xff", 16536705566725055458},
		// Without the separators both of these would hash "abc" and be the
		// same alert.
		{"name a, value bc", alertgroup.Labels{"a": "bc"}, "a\xffbc\xff", 11575188015659155627},
		{"name ab, value c", alertgroup.Labels{"ab": "c"}, "ab\xffc\xff", 2358367985974424609},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			for range 20 {
				if got := tc.labels.Fingerprint(); got != tc.want {
					t.Fatalf("Labels(%v).Fingerprint() = %d, want %d (FNV-1a 64 of %q)",
						map[string]string(tc.labels), got, tc.want, tc.stream)
				}
			}
		})
	}

	t.Run("same labels filled in another order", func(t *testing.T) {
		a := alertgroup.Labels{}
		b := alertgroup.Labels{}
		names := []string{"alertname", "cluster", "instance", "job", "namespace", "node", "pod", "severity"}
		for i, name := range names {
			a[name] = name + "-value"
			b[names[len(names)-1-i]] = names[len(names)-1-i] + "-value"
		}
		fa, fb := a.Fingerprint(), b.Fingerprint()
		if fa != fb {
			t.Errorf("Fingerprint() of the same labels = %d and %d, want equal", fa, fb)
		}
	})
}

// Rule 3.
func TestAlertResolvedAt(t *testing.T) {
	now := t0
	tests := []struct {
		name   string
		endsAt time.Time
		want   bool
	}{
		{"zero EndsAt: firing", time.Time{}, false},
		{"EndsAt in the past: resolved", now.Add(-time.Second), true},
		{"EndsAt equal to now: resolved", now, true},
		{"EndsAt in the future: still firing", now.Add(time.Nanosecond), false},
	}
	for _, tc := range tests {
		a := alertgroup.Alert{Labels: ls("alertname=Disk"), StartsAt: startedAt, EndsAt: tc.endsAt}
		if got := a.ResolvedAt(now); got != tc.want {
			t.Errorf("%s: Alert{EndsAt: %v}.ResolvedAt(%v) = %v, want %v",
				tc.name, tc.endsAt, now, got, tc.want)
		}
	}
}
