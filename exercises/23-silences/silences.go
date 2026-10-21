// Package silences holds alert silences that end. A silence mutes the
// alerts it matches until its end time. Expire removes the silences that
// have ended and reports each one to a callback, so that the on-call
// channel hears when a silence ends.
package silences

import (
	"errors"
	"time"
)

// ErrInvalid is wrapped by the error that Add returns for a silence it
// rejects. Check for it with errors.Is.
var ErrInvalid = errors.New("silences: invalid silence")

// Silence mutes the alerts it matches until Until.
type Silence struct {
	ID     string    // unique in the store; Add with an existing ID replaces that silence
	Alert  string    // alert name; "" matches every alert
	Target string    // node or host; "" matches every target
	Until  time.Time // the silence is active while now < Until
}

// Store holds silences. Create it with New. All methods are safe for
// concurrent use.
type Store struct {
	// TODO: add fields.
}

// New returns an empty store. now is the store's clock; nil means
// time.Now. onExpire, if not nil, is called by Expire once for every
// silence that Expire removes.
func New(now func() time.Time, onExpire func(s Silence)) *Store {
	panic("TODO: implement")
}

// Add stores sil and returns nil. If the store holds a silence with the
// same ID, sil replaces it, whether that silence has ended or not.
//
// Add rejects a silence with an empty ID, a silence whose Alert and Target
// are both empty (it would match everything), and a silence whose Until is
// not after now. Then it returns an error that wraps ErrInvalid and
// contains the ID formatted with %q, and it does not change the store.
func (s *Store) Add(sil Silence) error {
	panic("TODO: implement")
}

// Match returns the active silence that matches the alert and the target,
// and true. A silence is active while now < Until. It matches when its
// Alert equals alert or is "", and its Target equals target or is "". Of
// several matches, Match returns the one with the latest Until, and of
// those the one with the smallest ID. With no match it returns the zero
// Silence and false.
func (s *Store) Match(alert, target string) (Silence, bool) {
	panic("TODO: implement")
}

// Expire reads the clock once, removes every silence with now >= Until, and
// returns them sorted by Until, then by ID. Then it calls onExpire for each
// of them, in that order, without holding a lock, before it returns. So
// onExpire may call any method of the store.
func (s *Store) Expire() []Silence {
	panic("TODO: implement")
}

// Len returns the number of silences in the store, including silences that
// have ended but that Expire has not removed yet.
func (s *Store) Len() int {
	panic("TODO: implement")
}
