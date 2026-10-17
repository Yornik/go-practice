package reconciler

import (
	"context"
	"errors"
)

// Object is a named object with a spec, like a Kubernetes ConfigMap.
type Object struct {
	Name string
	Spec map[string]string
	// ResourceVersion is the version of the object in the store. The store
	// sets it on every write. An Update must send the version it read.
	ResourceVersion int64
}

// DeepCopy returns a copy of o that shares no memory with o: changing the
// Spec of one does not change the other.
func (o Object) DeepCopy() Object {
	panic("TODO: implement")
}

// Errors a Store returns. They may be wrapped; use errors.Is.
var (
	ErrNotFound      = errors.New("object not found")
	ErrConflict      = errors.New("conflict: the object has been modified")
	ErrAlreadyExists = errors.New("object already exists")
)

// Store is the API that holds the actual state, like the Kubernetes API
// server. Implementations store and return copies, so the caller can change
// the objects it passes in or gets back.
type Store interface {
	// Get returns the object with this name, or ErrNotFound.
	Get(ctx context.Context, name string) (Object, error)
	// List returns every object, sorted by Name.
	List(ctx context.Context) ([]Object, error)
	// Create stores a new object and returns it with its ResourceVersion.
	// It returns ErrAlreadyExists if the name is taken.
	Create(ctx context.Context, obj Object) (Object, error)
	// Update replaces an object if obj.ResourceVersion is the current
	// version, and returns it with its new ResourceVersion. It returns
	// ErrNotFound if there is no such object, and ErrConflict if
	// obj.ResourceVersion is not the current version.
	Update(ctx context.Context, obj Object) (Object, error)
	// Delete removes the object with this name, or returns ErrNotFound.
	Delete(ctx context.Context, name string) error
}

// MemoryStore is a Store in memory. Create and Update give the object a
// ResourceVersion greater than every version the store handed out before,
// so the first write gets a version > 0. MemoryStore ignores ctx. It is safe
// for concurrent use.
type MemoryStore struct {
	// TODO: add fields.
}

var _ Store = (*MemoryStore)(nil)

// NewMemoryStore returns an empty store.
func NewMemoryStore() *MemoryStore {
	panic("TODO: implement")
}

// Get implements Store.
func (s *MemoryStore) Get(ctx context.Context, name string) (Object, error) {
	panic("TODO: implement")
}

// List implements Store. An empty store returns an empty list.
func (s *MemoryStore) List(ctx context.Context) ([]Object, error) {
	panic("TODO: implement")
}

// Create implements Store. It ignores obj.ResourceVersion.
func (s *MemoryStore) Create(ctx context.Context, obj Object) (Object, error) {
	panic("TODO: implement")
}

// Update implements Store. On error the stored object does not change.
func (s *MemoryStore) Update(ctx context.Context, obj Object) (Object, error) {
	panic("TODO: implement")
}

// Delete implements Store. It deletes whatever version is stored.
func (s *MemoryStore) Delete(ctx context.Context, name string) error {
	panic("TODO: implement")
}
