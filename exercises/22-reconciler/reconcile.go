package reconciler

import "context"

// Action is what one call of Reconcile changed in the store.
type Action int

// The actions of Reconcile.
const (
	None    Action = iota // nothing to do, or nothing could be done
	Created               // the object was missing and was created
	Updated               // the object had the wrong spec and was updated
	Deleted               // the object was not desired and was deleted
)

// String returns the name of the action, for example "Created".
func (a Action) String() string {
	switch a {
	case None:
		return "None"
	case Created:
		return "Created"
	case Updated:
		return "Updated"
	case Deleted:
		return "Deleted"
	}
	return "Action(?)"
}

// Reconcile makes the object called name in s match its desired state:
// desired maps every name that should exist to its spec, and names that are
// not in desired should not exist. Reconcile reads the object, compares it
// with desired and does at most one write. It touches no other object.
//
// Reconcile is idempotent: calling it again right after a successful call
// returns None and writes nothing. It does not retry: if a store call fails,
// it returns an error that wraps the store's error, and the Action None.
// An Update sends the ResourceVersion that Reconcile read, so a concurrent
// change makes it fail with ErrConflict instead of being overwritten. A
// Delete that finds the object already gone is a success with Action None.
func Reconcile(ctx context.Context, desired map[string]map[string]string, s Store, name string) (Action, error) {
	panic("TODO: implement")
}
