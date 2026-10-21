package silences_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	silences "github.com/yornik/go-practice/exercises/23-silences"
)

// Step 4: onExpire.

// heldLock is the likely cause when Expire does not return while onExpire
// calls the store.
const heldLock = "onExpire was called while Expire held its lock"

func TestExpireCallbackArgumentsAndOrder(t *testing.T) {
	h := newHarness(t)
	c := sil("c", "disk_full", "photo-store-3", 2*time.Hour)
	b := sil("b", "", "photo-store-2", time.Hour)
	a := sil("a", "disk_full", "", time.Hour)
	z := sil("z", "high_latency", "photo-store-1", 5*time.Hour)
	h.mustAdd(c, b, a, z)

	h.wantExpire()
	h.wantCalls("Expire()") // nothing ended: no call

	h.clock.Advance(3 * time.Hour)
	h.wantExpire(a, b, c)
	h.wantCalls("Expire()", a, b, c) // all calls happen before Expire returns

	h.wantExpire()
	h.wantCalls("the second Expire()")

	h.clock.Advance(2 * time.Hour)
	h.wantExpire(z)
	h.wantCalls("Expire()", z)
}

func TestExpireCallbackNil(t *testing.T) {
	clock := newClock()
	var s *silences.Store
	within(t, "New(clock.Now, nil)", lockHang, func() { s = silences.New(clock.Now, nil) })
	if s == nil {
		t.Fatal("New(clock.Now, nil) = nil, want a store")
	}
	disk2 := sil("disk-2", "disk_full", "photo-store-2", time.Hour)
	var err error
	within(t, "Add", lockHang, func() { err = s.Add(disk2) })
	if err != nil {
		t.Fatalf("Add(%s) = %v, want nil", show(disk2), err)
	}
	clock.Advance(time.Hour)
	var got []silences.Silence
	within(t, "Expire() with onExpire nil", lockHang, func() { got = s.Expire() })
	if !sameAll(got, []silences.Silence{disk2}) {
		t.Errorf("with onExpire nil: Expire() = %s, want %s", showAll(got), showAll([]silences.Silence{disk2}))
	}
}

func TestExpireCallbackCanExtend(t *testing.T) {
	// The "extend 1 h" button: onExpire adds the silence again with a new
	// Until. If Expire still holds its lock while it calls onExpire, Add
	// waits for that lock forever.
	clock := newClock()
	var (
		s      *silences.Store
		mu     sync.Mutex
		calls  []silences.Silence
		addErr []error
	)
	onExpire := func(ended silences.Silence) {
		extended := ended
		extended.Until = clock.Now().Add(time.Hour)
		err := s.Add(extended)
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, ended)
		addErr = append(addErr, err)
	}
	within(t, "New(clock.Now, onExpire)", lockHang, func() { s = silences.New(clock.Now, onExpire) })
	if s == nil {
		t.Fatal("New(clock.Now, onExpire) = nil, want a store")
	}
	disk2 := sil("disk-2", "disk_full", "photo-store-2", time.Hour)
	var err error
	within(t, "Add", lockHang, func() { err = s.Add(disk2) })
	if err != nil {
		t.Fatalf("Add(%s) = %v, want nil", show(disk2), err)
	}

	clock.Advance(time.Hour)
	var got []silences.Silence
	within(t, "Expire() with an onExpire that calls Add", heldLock, func() { got = s.Expire() })
	if !sameAll(got, []silences.Silence{disk2}) {
		t.Errorf("at t0+1h: Expire() = %s, want %s", showAll(got), showAll([]silences.Silence{disk2}))
	}
	mu.Lock()
	if !sameAll(calls, []silences.Silence{disk2}) {
		t.Errorf("at t0+1h: onExpire calls = %s, want %s", showAll(calls), showAll([]silences.Silence{disk2}))
	}
	if len(addErr) == 1 && addErr[0] != nil {
		t.Errorf("at t0+1h: Add inside onExpire returned %v, want nil", addErr[0])
	}
	mu.Unlock()

	extended := sil("disk-2", "disk_full", "photo-store-2", 2*time.Hour)
	var (
		match silences.Silence
		ok    bool
		n     int
	)
	within(t, "Match and Len after the extension", lockHang, func() {
		match, ok = s.Match("disk_full", "photo-store-2")
		n = s.Len()
	})
	if !ok || !same(match, extended) {
		t.Errorf("at t0+1h, after onExpire extended disk-2: Match(%q, %q) = %s, %v; want %s, true",
			"disk_full", "photo-store-2", show(match), ok, show(extended))
	}
	if n != 1 {
		t.Errorf("at t0+1h, after onExpire extended disk-2: Len() = %d, want 1", n)
	}
}

func TestExpireCallbackSeesFinishedChange(t *testing.T) {
	// When onExpire runs, Expire has removed every silence that ended. The
	// callback calls Len, Expire and Match on the same store.
	type seen struct {
		id     string
		length int    // Len() inside onExpire
		again  int    // len(Expire()) inside onExpire
		match  string // ID that Match("disk_full", "photo-store-1") returned inside onExpire
	}
	clock := newClock()
	var (
		s   *silences.Store
		mu  sync.Mutex
		got []seen
	)
	onExpire := func(ended silences.Silence) {
		x := seen{id: ended.ID, length: s.Len(), again: len(s.Expire())}
		if m, ok := s.Match("disk_full", "photo-store-1"); ok {
			x.match = m.ID
		}
		mu.Lock()
		defer mu.Unlock()
		got = append(got, x)
	}
	within(t, "New(clock.Now, onExpire)", lockHang, func() { s = silences.New(clock.Now, onExpire) })
	if s == nil {
		t.Fatal("New(clock.Now, onExpire) = nil, want a store")
	}
	list := []silences.Silence{
		sil("disk-1", "disk_full", "photo-store-1", time.Hour),
		sil("disk-2", "disk_full", "photo-store-2", time.Hour),
		sil("disks", "disk_full", "", 3*time.Hour),
	}
	within(t, "Add", lockHang, func() {
		for _, x := range list {
			if err := s.Add(x); err != nil {
				panic(fmt.Sprintf("Add(%s) = %v, want nil", show(x), err))
			}
		}
	})

	clock.Advance(2 * time.Hour)
	within(t, "Expire() with an onExpire that calls Len, Expire and Match", heldLock, func() { s.Expire() })
	want := []seen{
		{id: "disk-1", length: 1, again: 0, match: "disks"},
		{id: "disk-2", length: 1, again: 0, match: "disks"},
	}
	mu.Lock()
	defer mu.Unlock()
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("at t0+2h: inside onExpire {ID Len() len(Expire()) Match(\"disk_full\", \"photo-store-1\").ID} = %v,\nwant %v",
			got, want)
	}
}

func TestExpireCallbackDoesNotBlockOthers(t *testing.T) {
	// onExpire posts to the chat and that takes a while. Meanwhile other
	// goroutines must be able to use the store.
	clock := newClock()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock() // also when the test fails early

	var s *silences.Store
	onExpire := func(ended silences.Silence) {
		close(entered)
		<-release
	}
	within(t, "New(clock.Now, onExpire)", lockHang, func() { s = silences.New(clock.Now, onExpire) })
	if s == nil {
		t.Fatal("New(clock.Now, onExpire) = nil, want a store")
	}
	disk2 := sil("disk-2", "disk_full", "photo-store-2", time.Hour)
	within(t, "Add", lockHang, func() {
		if err := s.Add(disk2); err != nil {
			panic(fmt.Sprintf("Add(%s) = %v, want nil", show(disk2), err))
		}
	})
	clock.Advance(time.Hour)

	expired := make(chan any, 1)
	go func() {
		defer func() { expired <- recover() }()
		s.Expire()
	}()
	select {
	case <-entered:
	case v := <-expired:
		t.Fatalf("Expire() returned (panic: %v) without calling onExpire", v)
	case <-time.After(timeout):
		t.Fatalf("Expire() did not call onExpire within %v", timeout)
	}

	// onExpire for disk-2 is blocked in another goroutine right now.
	const busy = "a call waits for a lock that Expire holds while onExpire runs"
	within(t, "Add while onExpire runs in another goroutine", busy, func() {
		if err := s.Add(sil("disk-3", "disk_full", "photo-store-3", 2*time.Hour)); err != nil {
			panic(fmt.Sprintf("Add = %v, want nil", err))
		}
	})
	within(t, "Match while onExpire runs in another goroutine", busy, func() { s.Match("disk_full", "photo-store-3") })
	within(t, "Len while onExpire runs in another goroutine", busy, func() { s.Len() })
	within(t, "Expire while onExpire runs in another goroutine", busy, func() { s.Expire() })

	unblock()
	select {
	case v := <-expired:
		if v != nil {
			t.Fatalf("Expire() panicked: %v", v)
		}
	case <-time.After(timeout):
		t.Fatalf("Expire() did not return within %v after onExpire returned", timeout)
	}
}

func TestExpireCallbackReadmeExample(t *testing.T) {
	// The example in the README, with t0 as 04:00. onExpire extends disk-2
	// once, to 06:00.
	clock := newClock()
	var (
		s        *silences.Store
		extended bool
		addErr   error
	)
	onExpire := func(ended silences.Silence) {
		if ended.ID == "disk-2" && !extended {
			extended = true
			addErr = s.Add(sil("disk-2", "disk_full", "photo-store-2", 2*time.Hour))
		}
	}
	within(t, "New(clock.Now, onExpire)", lockHang, func() { s = silences.New(clock.Now, onExpire) })
	if s == nil {
		t.Fatal("New(clock.Now, onExpire) = nil, want a store")
	}
	h := &harness{t: t, clock: clock, rec: &recorder{}, s: s}

	disk2 := sil("disk-2", "disk_full", "photo-store-2", time.Hour)
	upgrade := sil("upgrade", "", "photo-store-2", 2*time.Hour)
	h.mustAdd(disk2, upgrade)
	h.addInvalid(silences.Silence{ID: "until-done", Alert: "disk_full"})
	h.wantMatch("disk_full", "photo-store-2", upgrade)
	h.wantNoMatch("disk_full", "photo-store-1")
	h.clock.Advance(59 * time.Minute)
	h.wantMatch("high_latency", "photo-store-2", upgrade)
	h.clock.Advance(time.Minute)
	h.wantLen(2)
	h.wantMatch("disk_full", "photo-store-2", upgrade)
	h.wantExpire(disk2)
	if !extended || addErr != nil {
		t.Fatalf("at t0+1h: onExpire called Add: %v, error %v; want true, nil", extended, addErr)
	}
	ext := sil("disk-2", "disk_full", "photo-store-2", 2*time.Hour)
	h.wantMatch("disk_full", "photo-store-2", ext)
	h.clock.Advance(30 * time.Minute)
	h.wantExpire()
	h.clock.Advance(30 * time.Minute)
	h.wantExpire(ext, upgrade)
	h.wantLen(0)
}
