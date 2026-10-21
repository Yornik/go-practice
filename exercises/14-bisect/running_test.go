package bisect_test

import (
	"testing"
	"time"

	bisect "github.com/yornik/go-practice/exercises/14-bisect"
)

func TestRunning(t *testing.T) {
	defer guard(t)()
	all := fixture()
	one := []bisect.Build{{Version: "v2.13.0", At: at(0, 9, 0)}}
	cases := []struct {
		name   string
		builds []bisect.Build
		t      time.Time
		want   string // the version; "" means false
	}{
		{"before the first build", all, at(0, 8, 59), ""},
		{"1 ns before the first build", all, at(0, 9, 0).Add(-time.Nanosecond), ""},
		{"exactly at the first build", all, at(0, 9, 0), "v2.14.0-b0001"},
		{"between 2 builds", all, at(0, 10, 0), "v2.14.0-b0001"},
		{"the night after a build", all, at(1, 3, 12), "v2.14.0-b0002"},
		{"1 ns before a build", all, at(1, 8, 15).Add(-time.Nanosecond), "v2.14.0-b0002"},
		{"exactly at a middle build", all, at(1, 8, 15), "v2.14.0-b0003"},
		{"builds 1 minute apart, at the first", all, at(3, 14, 0), "v2.14.0-b0004"},
		{"builds 1 minute apart, 59 s later", all, at(3, 14, 0).Add(59 * time.Second), "v2.14.0-b0004"},
		{"exactly at the last build", all, at(3, 14, 1), "v2.14.0-b0005"},
		{"long after the last build", all, at(30, 0, 0), "v2.14.0-b0005"},
		// The builds are in UTC. These times are the same instants in
		// other zones, so their wall clock reads differently.
		{"at a build, in CET", all, at(0, 11, 30).In(cet), "v2.14.0-b0002"},
		{"at the first build, in New York", all, time.Date(2026, 11, 2, 4, 0, 0, 0, nyc), "v2.14.0-b0001"},
		{"09:00 CET is before a build at 08:15 UTC", all, time.Date(2026, 11, 3, 9, 0, 0, 0, cet), "v2.14.0-b0002"},
		{"04:00 New York is after a build at 08:15 UTC", all, time.Date(2026, 11, 3, 4, 0, 0, 0, nyc), "v2.14.0-b0003"},
		{"empty list", []bisect.Build{}, at(1, 0, 0), ""},
		{"nil list", nil, at(1, 0, 0), ""},
		{"1 build, before it", one, at(0, 8, 0), ""},
		{"1 build, at it", one, at(0, 9, 0), "v2.13.0"},
		{"1 build, after it", one, at(5, 0, 0), "v2.13.0"},
	}
	for _, c := range cases {
		got, ok := bisect.Running(c.builds, c.t)
		switch {
		case c.want == "" && (ok || got != bisect.Build{}):
			t.Errorf("%s: Running(%q, %v) = %q, %v; want the zero Build, false",
				c.name, versions(c.builds), c.t, got.Version, ok)
		case c.want != "" && (!ok || got.Version != c.want):
			t.Errorf("%s: Running(%q, %v) = %q, %v; want %q, true",
				c.name, versions(c.builds), c.t, got.Version, ok, c.want)
		}
	}
}

func TestRunningIsFast(t *testing.T) {
	defer guardFor(t, 3*speedLimit)()
	// 1 000 000 lookups in 1 000 000 builds. A binary search needs about 20
	// comparisons per lookup. A loop over the builds needs up to 1 000 000,
	// and stops at the limit.
	const n = 1_000_000
	builds := manyBuilds(n)
	began := time.Now()
	for k := range n {
		if k%64 == 0 && time.Since(began) > speedLimit {
			t.Fatalf("still at lookup %d of %d after %v: Running must take O(log n) time", k, n, speedLimit)
		}
		// 7919 is prime, so this visits every build once, in a scrambled order.
		i := int(uint64(k) * 7919 % n)
		q := builds[i].At.Add(30 * time.Second)
		got, ok := bisect.Running(builds, q)
		if !ok || got.Version != builds[i].Version {
			t.Fatalf("Running(%d builds 1 minute apart, %v) = %q, %v; want %q, true",
				n, q, got.Version, ok, builds[i].Version)
		}
	}
}
