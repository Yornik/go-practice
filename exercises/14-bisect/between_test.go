package bisect_test

import (
	"testing"
	"time"

	bisect "github.com/yornik/go-practice/exercises/14-bisect"
)

func TestBetween(t *testing.T) {
	defer guard(t)()
	all := fixture()
	cases := []struct {
		name     string
		builds   []bisect.Build
		from, to time.Time
		want     []string
	}{
		{"every build", all, at(0, 0, 0), at(10, 0, 0),
			[]string{"v2.14.0-b0001", "v2.14.0-b0002", "v2.14.0-b0003", "v2.14.0-b0004", "v2.14.0-b0005"}},
		{"from exactly at a build: included", all, at(0, 11, 30), at(1, 9, 0),
			[]string{"v2.14.0-b0002", "v2.14.0-b0003"}},
		{"to exactly at a build: excluded", all, at(0, 9, 0), at(1, 8, 15),
			[]string{"v2.14.0-b0001", "v2.14.0-b0002"}},
		{"to 1 ns after a build", all, at(0, 9, 0), at(1, 8, 15).Add(time.Nanosecond),
			[]string{"v2.14.0-b0001", "v2.14.0-b0002", "v2.14.0-b0003"}},
		{"from 1 ns after a build", all, at(0, 9, 0).Add(time.Nanosecond), at(1, 0, 0),
			[]string{"v2.14.0-b0002"}},
		{"the last 2 builds", all, at(3, 14, 0), at(3, 14, 2),
			[]string{"v2.14.0-b0004", "v2.14.0-b0005"}},
		// The builds are in UTC: 04:00 New York is 09:00 UTC and 09:15 CET
		// is 08:15 UTC.
		{"bounds in other zones", all, time.Date(2026, 11, 2, 4, 0, 0, 0, nyc), time.Date(2026, 11, 3, 9, 15, 0, 0, cet),
			[]string{"v2.14.0-b0001", "v2.14.0-b0002"}},
		{"a window without a build", all, at(1, 9, 0), at(3, 0, 0), nil},
		{"before the first build", all, at(0, 0, 0), at(0, 9, 0), nil},
		{"after the last build", all, at(3, 14, 2), at(10, 0, 0), nil},
		{"from equal to to, at a build", all, at(1, 8, 15), at(1, 8, 15), nil},
		{"to before from", all, at(10, 0, 0), at(0, 0, 0), nil},
		{"empty list", []bisect.Build{}, at(0, 0, 0), at(10, 0, 0), nil},
		{"nil list", nil, at(0, 0, 0), at(10, 0, 0), nil},
	}
	for _, c := range cases {
		got := bisect.Between(c.builds, c.from, c.to)
		var want []bisect.Build
		for _, b := range c.builds {
			for _, v := range c.want {
				if b.Version == v {
					want = append(want, b)
				}
			}
		}
		if !sameBuilds(got, want) {
			t.Errorf("%s: Between(%q, %v, %v)\n got  %q\n want %q",
				c.name, versions(c.builds), c.from, c.to, versions(got), c.want)
		}
	}
}

func TestBetweenReturnsCopy(t *testing.T) {
	defer guard(t)()
	windows := []struct {
		name     string
		from, to time.Time
	}{
		{"every build", at(0, 0, 0), at(10, 0, 0)},
		{"the first 2 builds", at(0, 0, 0), at(1, 0, 0)},
		{"2 builds in the middle", at(0, 11, 30), at(3, 14, 0)},
	}
	for _, w := range windows {
		builds := fixture()

		got := bisect.Between(builds, w.from, w.to)
		if len(got) == 0 {
			t.Errorf("%s: Between returned no builds, want some", w.name)
			continue
		}
		got[0].Version = "changed"
		if !sameBuilds(builds, fixture()) {
			t.Errorf("%s: after result[0].Version = \"changed\", the input is %q, want it unchanged",
				w.name, versions(builds))
		}

		// An append that fits in the capacity of the result would write
		// into the input if the result were a part of it.
		got = bisect.Between(builds, w.from, w.to)
		_ = append(got, bisect.Build{Version: "appended"})
		if !sameBuilds(builds, fixture()) {
			t.Errorf("%s: after append(result, ...), the input is %q, want it unchanged",
				w.name, versions(builds))
		}
	}
}

func TestBetweenIsFast(t *testing.T) {
	defer guardFor(t, 3*speedLimit)()
	// 200 000 windows of 3 builds in 1 000 000 builds. Finding a window with
	// a binary search needs about 40 comparisons. A loop from the first
	// build needs up to 1 000 000, and stops at the limit.
	const (
		n     = 1_000_000
		calls = 200_000
	)
	builds := manyBuilds(n)
	began := time.Now()
	for k := range calls {
		if k%64 == 0 && time.Since(began) > speedLimit {
			t.Fatalf("still at call %d of %d after %v: Between must find the window in O(log n) time", k, calls, speedLimit)
		}
		i := int(uint64(k) * 7919 % (n - 3))
		from := builds[i].At.Add(30 * time.Second)
		to := builds[i+3].At.Add(30 * time.Second)
		got := bisect.Between(builds, from, to)
		if len(got) != 3 || got[0].Version != builds[i+1].Version || got[2].Version != builds[i+3].Version {
			t.Fatalf("Between(%d builds 1 minute apart, %v, %v) = %q, want %q",
				n, from, to, versions(got), versions(builds[i+1:i+4]))
		}
	}
}
