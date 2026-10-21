package leaderelection

import "errors"

// ErrStaleToken is wrapped by the error FencedResource.Write returns for a
// token lower than one it has already seen.
var ErrStaleToken = errors.New("leaderelection: stale fencing token")

// FencedResource is a value that only the current leader may change, like a
// row in a database, a file in object storage or the replica count of a
// Deployment. Every write carries the writer's fencing token (its lease
// Epoch). The resource remembers the highest token it has accepted and
// rejects writes with a lower one, so a leader that was replaced while it was
// frozen cannot overwrite the work of its successor.
//
// The zero value is ready to use: no token seen yet, any token is accepted.
// A FencedResource is safe for concurrent use. It must not be copied after
// first use.
type FencedResource struct {
	// TODO: add fields.
}

// Write stores v if token is at least the highest token accepted so far,
// and then remembers token as the highest. The same token may write many
// times. If token is lower, Write changes nothing and returns an error that
// wraps ErrStaleToken. Checking the token and storing v are one atomic step.
func (r *FencedResource) Write(token uint64, v string) error {
	panic("TODO: implement")
}

// Read returns the last value written and the highest token accepted. For a
// new resource it returns "" and 0.
func (r *FencedResource) Read() (v string, token uint64) {
	panic("TODO: implement")
}
