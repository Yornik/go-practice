// Package semver parses version tags such as "v2.10.0" and
// "v2.10.0-rc.1", compares them as numbers, and picks the newest release
// from a list of tags.
package semver

import "errors"

// ErrInvalid is wrapped by the error Parse returns for a string that is not
// a valid version. Check for it with errors.Is.
var ErrInvalid = errors.New("semver: invalid version")

// ErrNoRelease is wrapped by the error Latest returns when no tag is a valid
// release. Check for it with errors.Is.
var ErrNoRelease = errors.New("semver: no release")

// Version is a parsed version such as v2.10.0-rc.1.
type Version struct {
	Major, Minor, Patch int
	// Pre is the pre-release without the leading "-", for example "rc.1".
	// It is "" for a release.
	Pre string
}

// Parse reads a version of the form [v]MAJOR.MINOR.PATCH[-PRE], for example
// "v2.10.0", "2.10.0" or "v2.10.0-rc.1".
//
// The "v" is optional and lowercase. MAJOR, MINOR and PATCH are ASCII digits
// with no sign and no leading zero ("0" is fine, "02" is not), and each fits
// in an int. PRE is 1 or more bytes from A-Z, a-z, 0-9, "." and "-". Nothing
// else is allowed: no build metadata ("+build.5") and no whitespace.
//
// On error Parse returns the zero Version and an error that wraps ErrInvalid
// and contains the input quoted with %q.
func Parse(s string) (Version, error) {
	panic("TODO: implement")
}

// String returns the version with a lowercase "v": "v2.10.0" for a release,
// "v2.10.0-rc.1" for a pre-release. For every v that Parse returns,
// Parse(v.String()) gives v again.
func (v Version) String() string {
	panic("TODO: implement")
}

// IsRelease reports whether v is a release, that is, whether Pre is "".
func (v Version) IsRelease() bool {
	panic("TODO: implement")
}

// Compare returns -1 if a is older than b, 0 if they are equal and +1 if a
// is newer than b. It never returns another value. It has the signature
// that slices.SortFunc takes, so slices.SortFunc(vs, Compare) sorts vs
// oldest first.
//
// Major, Minor and Patch compare as numbers, in this order. When all 3 are
// equal, a release is newer than any pre-release, and 2 pre-releases compare
// with plain string order.
func Compare(a, b Version) int {
	panic("TODO: implement")
}

// Latest returns the newest release among the tags. It skips tags that Parse
// rejects and tags that are pre-releases.
//
// When no release is left, Latest returns the zero Version and an error that
// wraps ErrNoRelease.
func Latest(tags []string) (Version, error) {
	panic("TODO: implement")
}
