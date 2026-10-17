// Package broadcaster fans out a stream of events from one producer to many
// subscribers. A slow subscriber never blocks the producer: every
// subscription has a bounded buffer and a policy for what happens when that
// buffer is full. A subscriber can resume from a sequence number as long as
// the broadcaster still keeps the events after it.
package broadcaster

import (
	"errors"
	"fmt"
)

// Policy decides what happens when an event arrives for a subscription whose
// buffer is full.
type Policy int

const (
	// DropOldest discards the oldest buffered event to make room for the new
	// one. The consumer always sees the most recent events.
	DropOldest Policy = iota
	// DropNewest discards the new event and keeps the buffer as it is.
	DropNewest
	// Disconnect ends the subscription with ErrSlowConsumer. Nothing is
	// skipped: the consumer receives a run of events without gaps and can
	// resume with SubscribeFrom.
	Disconnect
)

// String returns the name of the policy, for example "DropOldest".
func (p Policy) String() string {
	switch p {
	case DropOldest:
		return "DropOldest"
	case DropNewest:
		return "DropNewest"
	case Disconnect:
		return "Disconnect"
	}
	return fmt.Sprintf("Policy(%d)", int(p))
}

// Event is one published value with its sequence number. Seq is 1 for the
// first event a Broadcaster publishes and grows by 1 with every Publish.
type Event[T any] struct {
	Seq   uint64
	Value T
}

var (
	// ErrClosed is returned by Publish and SubscribeFrom after Close, and by
	// Subscription.Err when the subscription ended because of Close.
	ErrClosed = errors.New("broadcaster: closed")
	// ErrSlowConsumer is returned by Subscription.Err when a subscription
	// with the Disconnect policy ended because its buffer was full.
	ErrSlowConsumer = errors.New("broadcaster: slow consumer disconnected")
	// ErrGone is wrapped by the error SubscribeFrom returns when it cannot
	// deliver every event after the given sequence number. The caller has to
	// rebuild its state from the source of truth (re-list) and subscribe
	// again.
	ErrGone = errors.New("broadcaster: sequence number gone")
)

// Broadcaster delivers every published event to every active subscription.
// Create one with New. All methods are safe for concurrent use.
type Broadcaster[T any] struct {
	// TODO: add fields.
}

// New returns a Broadcaster that keeps the last history published events,
// so that SubscribeFrom can replay them. history <= 0 keeps none.
func New[T any](history int) *Broadcaster[T] {
	panic("TODO: implement")
}

// Publish sends v to every active subscription and returns the sequence
// number of the new event. It never waits for a consumer: when a
// subscription's buffer is full, its policy decides, and that is done before
// Publish returns. After Close, Publish publishes nothing and returns 0 and
// ErrClosed.
func (b *Broadcaster[T]) Publish(v T) (uint64, error) {
	panic("TODO: implement")
}

// Subscribe starts a subscription that receives the events published after
// Subscribe returns. Its buffer holds at most buf events (buf < 1 counts as
// 1) and p decides what happens when the buffer is full. After Close,
// Subscribe returns a subscription that has already ended: its channel is
// closed and Err returns ErrClosed.
func (b *Broadcaster[T]) Subscribe(buf int, p Policy) *Subscription[T] {
	panic("TODO: implement")
}

// SubscribeFrom starts a subscription that receives every event with
// Seq > after: first the kept events, then the live ones, without a gap or a
// duplicate between them. The replayed events go through the subscription's
// buffer and policy just like live events.
//
// If an event with Seq > after is no longer kept, or after is larger than
// the sequence number of the last published event, SubscribeFrom returns
// nil and an error that wraps ErrGone. After Close it returns nil and
// ErrClosed.
func (b *Broadcaster[T]) SubscribeFrom(after uint64, buf int, p Policy) (*Subscription[T], error) {
	panic("TODO: implement")
}

// Close ends every active subscription with ErrClosed and makes Publish fail.
// Each subscriber still receives the events already in its buffer, then its
// channel is closed. Close is idempotent.
func (b *Broadcaster[T]) Close() {
	panic("TODO: implement")
}

// Subscribers returns the number of active subscriptions: started and not
// yet ended by Unsubscribe, Disconnect or Close.
func (b *Broadcaster[T]) Subscribers() int {
	panic("TODO: implement")
}

// Subscription is one consumer's view of a Broadcaster. All methods are safe
// for concurrent use.
type Subscription[T any] struct {
	// TODO: add fields.
}

// C returns the channel the events arrive on, in Seq order and without
// duplicates. It always returns the same channel. When the subscription ends,
// the channel is closed after the events already buffered.
func (s *Subscription[T]) C() <-chan Event[T] {
	panic("TODO: implement")
}

// Unsubscribe ends the subscription. Err then returns nil. It is idempotent
// and safe to call at any time from any goroutine, also after the
// subscription has already ended for another reason.
func (s *Subscription[T]) Unsubscribe() {
	panic("TODO: implement")
}

// Err reports why the subscription ended: nil after Unsubscribe,
// ErrSlowConsumer after a Disconnect, ErrClosed after Close. The first reason
// wins. Err returns nil while the subscription is active; call it after C is
// closed.
func (s *Subscription[T]) Err() error {
	panic("TODO: implement")
}

// Dropped returns how many events the DropOldest or DropNewest policy has
// discarded for this subscription. It is always 0 for Disconnect.
func (s *Subscription[T]) Dropped() uint64 {
	panic("TODO: implement")
}
