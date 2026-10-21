package semver_test

import (
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	semver "github.com/yornik/go-practice/exercises/06-semver"
)

// readTags reads 1 tag per line, as `git tag` prints them.
func readTags(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	// Fields also drops the \r of a file checked out with Windows line endings.
	return strings.Fields(string(data))
}

// TestLatestIncidentTags uses the 40 tags of the agent repository on the
// night of the incident. A plain string sort puts v2.9.3 last.
func TestLatestIncidentTags(t *testing.T) {
	tags := readTags(t, "testdata/tags.txt")
	if len(tags) != 40 {
		t.Fatalf("testdata/tags.txt has %d tags, want 40", len(tags))
	}
	want := semver.Version{Major: 2, Minor: 10}

	for _, name := range []string{"as git tag lists them", "reversed"} {
		in := slices.Clone(tags)
		if name == "reversed" {
			slices.Reverse(in)
		}
		before := slices.Clone(in)
		got, err := semver.Latest(in)
		if err != nil || got != want {
			t.Errorf("%s: Latest(tags.txt) = %v, %v; want %v, nil", name, got, err, want)
		}
		if !slices.Equal(in, before) {
			t.Errorf("%s: Latest changed its argument: the tags are now %q, want them unchanged", name, in)
		}
	}
}

func TestLatestSkips(t *testing.T) {
	tests := []struct {
		name string
		tags []string
		want semver.Version
	}{
		{"1 release", []string{"v1.0.0"}, semver.Version{Major: 1}},
		{"numbers, not text", []string{"v2.9.3", "v2.10.0", "v2.9.10"}, semver.Version{Major: 2, Minor: 10}},
		{"newer pre-release is skipped", []string{"v2.10.0", "v2.11.0-rc.1"}, semver.Version{Major: 2, Minor: 10}},
		{"invalid tags are skipped", []string{"latest", "v0.9.0", "V3.0.0", "v3.0", "v3.00.0", "v3.0.0+build.1"}, semver.Version{Minor: 9}},
		{"release among pre-releases", []string{"v1.0.0-rc.1", "v0.9.0", "v1.0.0-rc.2"}, semver.Version{Minor: 9}},
		{"v0.0.0 is a release", []string{"v0.0.0", "nightly"}, semver.Version{}},
		{"same version with and without v", []string{"2.10.0", "v2.9.3", "v2.10.0"}, semver.Version{Major: 2, Minor: 10}},
	}
	for _, tc := range tests {
		got, err := semver.Latest(tc.tags)
		if err != nil || got != tc.want {
			t.Errorf("%s: Latest(%q) = %v, %v; want %v, nil", tc.name, tc.tags, got, err, tc.want)
		}
	}
}

func TestLatestNoRelease(t *testing.T) {
	tests := []struct {
		name string
		tags []string
	}{
		{"nil", nil},
		{"empty", []string{}},
		{"only pre-releases", []string{"v1.0.0-rc.1", "v2.10.0-rc.1", "v2.11.0-beta.1"}},
		{"only invalid tags", []string{"latest", "stable", "V2.10.0", "v2.10", ""}},
		{"pre-releases and invalid tags", []string{"v1.0.0-rc.1", "latest"}},
	}
	for _, tc := range tests {
		got, err := semver.Latest(tc.tags)
		if err == nil {
			t.Errorf("%s: Latest(%q) = %v, nil; want an error wrapping ErrNoRelease", tc.name, tc.tags, got)
			continue
		}
		if !errors.Is(err, semver.ErrNoRelease) {
			t.Errorf("%s: Latest(%q) error = %v; want it to wrap ErrNoRelease (errors.Is)", tc.name, tc.tags, err)
		}
		if got != (semver.Version{}) {
			t.Errorf("%s: Latest(%q) = %#v with an error; want the zero Version", tc.name, tc.tags, got)
		}
	}
}

func ExampleLatest() {
	tags := []string{"v2.9.3", "v2.10.0", "v2.11.0-rc.1", "latest"}
	v, err := semver.Latest(tags)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(v)
	// Output: v2.10.0
}
