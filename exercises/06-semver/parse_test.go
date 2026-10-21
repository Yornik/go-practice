package semver_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	semver "github.com/yornik/go-practice/exercises/06-semver"
)

func TestParseValid(t *testing.T) {
	tests := []struct {
		in   string
		want semver.Version
	}{
		{"v2.10.0", semver.Version{Major: 2, Minor: 10}},
		{"2.10.0", semver.Version{Major: 2, Minor: 10}},
		{"v2.9.3", semver.Version{Major: 2, Minor: 9, Patch: 3}},
		{"v0.0.0", semver.Version{}},
		{"v10.20.30", semver.Version{Major: 10, Minor: 20, Patch: 30}},
		{"v2026.10.21", semver.Version{Major: 2026, Minor: 10, Patch: 21}},
		{"v1.0.100", semver.Version{Major: 1, Patch: 100}},
		{"v2.10.0-rc.1", semver.Version{Major: 2, Minor: 10, Pre: "rc.1"}},
		{"1.0.0-alpha", semver.Version{Major: 1, Pre: "alpha"}},
		{"v1.0.0-RC.1", semver.Version{Major: 1, Pre: "RC.1"}},
		{"v1.0.0-0.3.7", semver.Version{Major: 1, Pre: "0.3.7"}},
		{"v1.0.0-x.7.z.92", semver.Version{Major: 1, Pre: "x.7.z.92"}},
		// The first - starts the pre-release; later ones belong to it.
		{"v1.0.0-x-y-z.--", semver.Version{Major: 1, Pre: "x-y-z.--"}},
		{"v1.0.0-beta-2", semver.Version{Major: 1, Pre: "beta-2"}},
	}
	for _, tc := range tests {
		got, err := semver.Parse(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("Parse(%q) = %#v, %v; want %#v, nil", tc.in, got, err, tc.want)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	tests := []struct {
		in  string
		why string
	}{
		{"", "empty"},
		{"v", "no numbers"},
		{"latest", "not a version"},
		{"2.10", "2 numbers"},
		{"v2", "1 number"},
		{"v2.10.0.1", "4 numbers"},
		{"V2.10.0", "uppercase V"},
		{"vv2.10.0", "2 v's"},
		{"v02.1.0", "leading zero in major"},
		{"v2.01.0", "leading zero in minor"},
		{"v2.1.00", "leading zero in patch"},
		{"v2.-1.0", "minus sign"},
		{"v2.+1.0", "plus sign"},
		{"v+2.1.0", "plus sign before major"},
		{"v2..0", "empty minor"},
		{"v2.1.", "empty patch"},
		{".2.1.0", "empty major"},
		{"v2.x.0", "letter instead of a number"},
		{"v2.1.0-", "empty pre-release"},
		{"v2.1.0+build", "build metadata"},
		{"v2.1.0-rc.1+build.5", "pre-release with build metadata"},
		{"v2.1.0-rc_1", "_ in the pre-release"},
		{"v2.1.0-rc 1", "space in the pre-release"},
		{"v2.1.0-ünicode", "non-ASCII letter in the pre-release"},
		{" v2.1.0", "leading space"},
		{"v2.1.0 ", "trailing space"},
		{"v2.1.0\n", "trailing newline"},
		{"v 2.1.0", "space after the v"},
		{"v2. 1.0", "space before a number"},
		{"v١.2.3", "non-ASCII digit"},
		{"v99999999999999999999.0.0", "major does not fit in an int"},
		{"v1.0.99999999999999999999", "patch does not fit in an int"},
	}
	for _, tc := range tests {
		got, err := semver.Parse(tc.in)
		if err == nil {
			t.Errorf("Parse(%q) = %#v, nil; want an error wrapping ErrInvalid (%s)", tc.in, got, tc.why)
			continue
		}
		if !errors.Is(err, semver.ErrInvalid) {
			t.Errorf("Parse(%q) error = %v; want it to wrap ErrInvalid (errors.Is)", tc.in, err)
		}
		if quoted := fmt.Sprintf("%q", tc.in); !strings.Contains(err.Error(), quoted) {
			t.Errorf("Parse(%q) error = %q; want it to contain the input as %s", tc.in, err, quoted)
		}
		if got != (semver.Version{}) {
			t.Errorf("Parse(%q) = %#v with an error; want the zero Version", tc.in, got)
		}
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		v    semver.Version
		want string
	}{
		{semver.Version{Major: 2, Minor: 10}, "v2.10.0"},
		{semver.Version{Major: 2, Minor: 9, Patch: 3}, "v2.9.3"},
		{semver.Version{}, "v0.0.0"},
		{semver.Version{Major: 2, Minor: 10, Pre: "rc.1"}, "v2.10.0-rc.1"},
		{semver.Version{Major: 1, Pre: "x-y"}, "v1.0.0-x-y"},
	}
	for _, tc := range tests {
		if got := tc.v.String(); got != tc.want {
			t.Errorf("%#v.String() = %q, want %q", tc.v, got, tc.want)
		}
		// fmt calls String for %v and %s, because Version is a fmt.Stringer.
		if got := fmt.Sprint(tc.v); got != tc.want {
			t.Errorf("fmt.Sprint(%#v) = %q, want %q", tc.v, got, tc.want)
		}
	}
}

func TestStringRoundTrip(t *testing.T) {
	tests := []struct {
		in   string
		want string // String() of the parsed version
	}{
		{"v2.10.0", "v2.10.0"},
		{"2.10.0", "v2.10.0"},
		{"v0.0.0", "v0.0.0"},
		{"v2.10.0-rc.1", "v2.10.0-rc.1"},
		{"1.0.0-x-y-z.--", "v1.0.0-x-y-z.--"},
	}
	for _, tc := range tests {
		v, err := semver.Parse(tc.in)
		if err != nil {
			t.Errorf("Parse(%q) error = %v, want nil", tc.in, err)
			continue
		}
		s := v.String()
		if s != tc.want {
			t.Errorf("Parse(%q).String() = %q, want %q", tc.in, s, tc.want)
		}
		again, err := semver.Parse(s)
		if err != nil || again != v {
			t.Errorf("Parse(%q) = %#v, %v; want %#v, nil (the version that %q parsed to)", s, again, err, v, tc.in)
		}
	}
}

func TestIsRelease(t *testing.T) {
	tests := []struct {
		v    semver.Version
		want bool
	}{
		{semver.Version{Major: 2, Minor: 10}, true},
		{semver.Version{}, true},
		{semver.Version{Major: 2, Minor: 10, Pre: "rc.1"}, false},
		{semver.Version{Major: 1, Pre: "0"}, false},
	}
	for _, tc := range tests {
		if got := tc.v.IsRelease(); got != tc.want {
			t.Errorf("%#v.IsRelease() = %v, want %v", tc.v, got, tc.want)
		}
	}
}
