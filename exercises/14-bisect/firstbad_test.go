package bisect_test

import (
	"errors"
	"fmt"
	"math/bits"
	"math/rand/v2"
	"strings"
	"testing"

	bisect "github.com/yornik/go-practice/exercises/14-bisect"
)

// probe plays the test of 1 bisect: builds 0 to first-1 are good, builds
// first to n-1 are bad. It records every call and stops the test at the
// first call that breaks a rule: an index outside 0 <= i < n, more than
// bits.Len(uint(n)) calls, or a call after an error.
type probe struct {
	t      *testing.T
	what   string // the call under test, for messages
	n      int
	first  int
	limit  int   // the most calls allowed
	failOn int   // the call (1, 2, ...) whose test returns errDeploy; 0: none
	calls  []int // the indexes, in call order
}

var errDeploy = errors.New("deploy failed: canary-h3-01 offline")

func newProbe(t *testing.T, fn string, n, first int) *probe {
	p := &probe{t: t, n: n, first: first}
	if n > 0 {
		p.limit = bits.Len(uint(n))
	}
	if first < n {
		p.what = fmt.Sprintf("%s(%d, ...) with first bad build %d", fn, n, first)
	} else {
		p.what = fmt.Sprintf("%s(%d, ...) with no bad build", fn, n)
	}
	return p
}

func (p *probe) record(i int) {
	p.t.Helper()
	p.calls = append(p.calls, i)
	if p.failOn > 0 && len(p.calls) > p.failOn {
		p.t.Fatalf("%s: test called again after call %d returned an error, want no more calls (calls: %v)",
			p.what, p.failOn, p.calls)
	}
	if i < 0 || i >= p.n {
		p.t.Fatalf("%s: called with build %d, want 0 <= i < %d (calls: %v)", p.what, i, p.n, p.calls)
	}
	if len(p.calls) > p.limit {
		p.t.Fatalf("%s: call number %d, want at most bits.Len(%d) = %d calls (calls: %v)",
			p.what, len(p.calls), p.n, p.limit, p.calls)
	}
}

// isBad is the function to pass to FirstBad.
func (p *probe) isBad(i int) bool {
	p.t.Helper()
	p.record(i)
	return i >= p.first
}

// test is the function to pass to FirstBadErr.
func (p *probe) test(i int) (bool, error) {
	p.t.Helper()
	p.record(i)
	if len(p.calls) == p.failOn {
		return false, errDeploy
	}
	return i >= p.first, nil
}

// mentionsBuild reports whether msg contains "build <i>" with i in decimal
// and no digit after it: "build 15" is not in "build 1536".
func mentionsBuild(msg string, i int) bool {
	word := fmt.Sprintf("build %d", i)
	for rest := msg; ; {
		_, after, found := strings.Cut(rest, word)
		if !found {
			return false
		}
		if after == "" || after[0] < '0' || after[0] > '9' {
			return true
		}
		rest = after
	}
}

func TestFirstBadEveryBoundary(t *testing.T) {
	defer guard(t)()
	// Every list from 0 to 100 builds, with the first bad build at every
	// position, and with no bad build (first == n).
	for n := 0; n <= 100; n++ {
		for first := 0; first <= n; first++ {
			p := newProbe(t, "FirstBad", n, first)
			if got := bisect.FirstBad(n, p.isBad); got != first {
				t.Fatalf("%s = %d, want %d (calls: %v)", p.what, got, first, p.calls)
			}
		}
	}
}

func TestFirstBadNoBuilds(t *testing.T) {
	defer guard(t)()
	for _, n := range []int{0, -1, -2048} {
		p := newProbe(t, "FirstBad", n, 0)
		if got := bisect.FirstBad(n, p.isBad); got != 0 {
			t.Errorf("FirstBad(%d, ...) = %d, want 0 (calls: %v)", n, got, p.calls)
		}
	}
}

func TestFirstBadHuge(t *testing.T) {
	defer guard(t)()
	// 2^30 builds: at most bits.Len(2^30) = 31 calls. A loop over the
	// builds would need up to a billion.
	const n = 1 << 30
	firsts := []int{0, 1, 2, n / 2, n - 2, n - 1, n}
	rnd := rand.New(rand.NewPCG(14, 2048))
	for range 20 {
		firsts = append(firsts, rnd.IntN(n+1))
	}
	for _, first := range firsts {
		p := newProbe(t, "FirstBad", n, first)
		if got := bisect.FirstBad(n, p.isBad); got != first {
			t.Errorf("%s = %d, want %d (calls: %v)", p.what, got, first, p.calls)
		}
	}
}

func TestFirstBadErrEveryBoundary(t *testing.T) {
	defer guard(t)()
	// Without an error, FirstBadErr follows the rules of FirstBad.
	for n := 0; n <= 100; n++ {
		for first := 0; first <= n; first++ {
			p := newProbe(t, "FirstBadErr", n, first)
			got, err := bisect.FirstBadErr(n, p.test)
			if got != first || err != nil {
				t.Fatalf("%s = %d, %v; want %d, nil (calls: %v)", p.what, got, err, first, p.calls)
			}
		}
	}
}

func TestFirstBadErrNoBuilds(t *testing.T) {
	defer guard(t)()
	for _, n := range []int{0, -1} {
		p := newProbe(t, "FirstBadErr", n, 0)
		got, err := bisect.FirstBadErr(n, p.test)
		if got != 0 || err != nil {
			t.Errorf("FirstBadErr(%d, ...) = %d, %v; want 0, nil (calls: %v)", n, got, err, p.calls)
		}
	}
}

func TestFirstBadErrStopsAtError(t *testing.T) {
	defer guard(t)()
	cases := []struct {
		n, first, failOn int
	}{
		{n: 2048, first: 1337, failOn: 1},
		{n: 2048, first: 1337, failOn: 2},
		{n: 2048, first: 1337, failOn: 3},
		{n: 2048, first: 2048, failOn: 3},
		{n: 1, first: 0, failOn: 1},
		{n: 3, first: 3, failOn: 1},
	}
	for _, c := range cases {
		p := newProbe(t, "FirstBadErr", c.n, c.first)
		p.failOn = c.failOn
		p.what += fmt.Sprintf(" and an error on call %d", c.failOn)
		got, err := bisect.FirstBadErr(c.n, p.test)
		if len(p.calls) < c.failOn {
			t.Errorf("%s = %d, %v after %d calls, want call %d to happen (calls: %v)",
				p.what, got, err, len(p.calls), c.failOn, p.calls)
			continue
		}
		failed := p.calls[c.failOn-1]
		if got != -1 {
			t.Errorf("%s: index = %d, want -1", p.what, got)
		}
		if !errors.Is(err, errDeploy) {
			t.Errorf("%s: error = %v, want an error that wraps %q (errors.Is)", p.what, err, errDeploy)
			continue
		}
		if !mentionsBuild(err.Error(), failed) {
			t.Errorf("%s: error = %q, want it to contain %q", p.what, err, fmt.Sprintf("build %d", failed))
		}
	}
}
