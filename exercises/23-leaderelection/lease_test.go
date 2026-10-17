package leaderelection_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	leaderelection "github.com/yornik/go-practice/exercises/23-leaderelection"
)

type Lease = leaderelection.Lease

// ----- Next: the pure core -----

func TestNext(t *testing.T) {
	const d = 10 * time.Second // the LeaseDuration of candidate "a"

	// heldByB is a lease that candidate "b" acquired a minute before t0.
	heldByB := func(renewed time.Time, dur time.Duration) Lease {
		return Lease{Holder: "b", Epoch: 4, AcquireTime: at(-60), RenewTime: renewed, Duration: dur, Version: 7}
	}
	// aTakesOver is what "a" writes when it acquires heldByB at now.
	aTakesOver := func(now time.Time) Lease {
		return Lease{Holder: "a", Epoch: 5, AcquireTime: now, RenewTime: now, Duration: d, Version: 7}
	}

	tests := []struct {
		name   string
		cur    Lease
		now    time.Time
		want   Lease
		wantOK bool
	}{
		{
			name:   "empty store: acquire with epoch 1",
			cur:    Lease{},
			now:    at(0),
			want:   Lease{Holder: "a", Epoch: 1, AcquireTime: at(0), RenewTime: at(0), Duration: d},
			wantOK: true,
		},
		{
			// b released the lease 1 s ago. By the times it would still be
			// valid, but nobody holds it: take it at once.
			name:   "released lease: acquire at once with the next epoch",
			cur:    Lease{Holder: "", Epoch: 4, AcquireTime: at(-60), RenewTime: at(-1), Duration: 15 * time.Second, Version: 7},
			now:    at(0),
			want:   aTakesOver(at(0)),
			wantOK: true,
		},
		{
			name:   "held by b, renewed 5s ago: wait",
			cur:    heldByB(at(-5), 15*time.Second),
			now:    at(0),
			want:   heldByB(at(-5), 15*time.Second),
			wantOK: false,
		},
		{
			name:   "held by b, 1ns before it expires: wait",
			cur:    heldByB(at(0), 15*time.Second),
			now:    at(15).Add(-time.Nanosecond),
			want:   heldByB(at(0), 15*time.Second),
			wantOK: false,
		},
		{
			name:   "held by b, expires exactly now: take over",
			cur:    heldByB(at(0), 15*time.Second),
			now:    at(15),
			want:   aTakesOver(at(15)),
			wantOK: true,
		},
		{
			name:   "held by b, expired an hour ago: take over",
			cur:    heldByB(at(0), 15*time.Second),
			now:    at(3600),
			want:   aTakesOver(at(3600)),
			wantOK: true,
		},
		{
			// Expiry uses the Duration in the lease, which the holder wrote:
			// b promised to hold it for 60 s. a's own d (10 s) does not matter.
			name:   "b's Duration of 60s counts, not a's 10s: wait",
			cur:    heldByB(at(0), 60*time.Second),
			now:    at(30),
			want:   heldByB(at(0), 60*time.Second),
			wantOK: false,
		},
		{
			name:   "b's Duration of 5s counts, not a's 10s: take over",
			cur:    heldByB(at(0), 5*time.Second),
			now:    at(5),
			want:   aTakesOver(at(5)),
			wantOK: true,
		},
		{
			name:   "own lease: renew with the same epoch and a new Duration",
			cur:    Lease{Holder: "a", Epoch: 4, AcquireTime: at(-60), RenewTime: at(-2), Duration: 15 * time.Second, Version: 7},
			now:    at(0),
			want:   Lease{Holder: "a", Epoch: 4, AcquireTime: at(-60), RenewTime: at(0), Duration: d, Version: 7},
			wantOK: true,
		},
		{
			// Nobody wrote the lease since a's last renew, so a was the only
			// leader all along: the epoch stays.
			name:   "own lease, expired, nobody took it: renew with the same epoch",
			cur:    Lease{Holder: "a", Epoch: 4, AcquireTime: at(-60), RenewTime: at(-30), Duration: 15 * time.Second, Version: 7},
			now:    at(0),
			want:   Lease{Holder: "a", Epoch: 4, AcquireTime: at(-60), RenewTime: at(0), Duration: d, Version: 7},
			wantOK: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := leaderelection.Next(tc.cur, "a", tc.now, d)
			if !sameLease(got, tc.want) || ok != tc.wantOK {
				t.Errorf("Next(%s, %q, %s, %v)\n got  %s, %v\n want %s, %v",
					show(tc.cur), "a", hms(tc.now), d, show(got), ok, show(tc.want), tc.wantOK)
			}
		})
	}
}

// ----- MemoryStore: compare-and-swap -----

func TestMemoryStoreStartsEmpty(t *testing.T) {
	s := leaderelection.NewMemoryStore()
	got, err := s.Get(context.Background())
	if err != nil || !sameLease(got, Lease{}) {
		t.Errorf("Get on a new store = %s, %v; want the zero Lease, nil", show(got), err)
	}
}

func TestMemoryStoreUpdate(t *testing.T) {
	ctx := context.Background()
	s := leaderelection.NewMemoryStore()

	// wantStored fails the test unless Get returns want.
	wantStored := func(t *testing.T, want Lease) {
		t.Helper()
		got, err := s.Get(ctx)
		if err != nil || !sameLease(got, want) {
			t.Fatalf("Get = %s, %v; want %s, nil", show(got), err, show(want))
		}
	}
	// update calls Update and checks its result.
	update := func(t *testing.T, l, want Lease) {
		t.Helper()
		got, err := s.Update(ctx, l)
		if err != nil || !sameLease(got, want) {
			t.Fatalf("Update(%s) = %s, %v; want %s, nil", show(l), show(got), err, show(want))
		}
	}
	// conflict calls Update and checks that it fails with ErrConflict.
	conflict := func(t *testing.T, l Lease) {
		t.Helper()
		if _, err := s.Update(ctx, l); !errors.Is(err, leaderelection.ErrConflict) {
			t.Fatalf("Update(%s) with a stale Version: error = %v, want ErrConflict", show(l), err)
		}
	}

	a := Lease{Holder: "a", Epoch: 1, AcquireTime: at(0), RenewTime: at(0), Duration: 10 * time.Second, Version: 0}
	a1 := a
	a1.Version = 1
	update(t, a, a1) // the store sets Version + 1 and returns the stored lease
	wantStored(t, a1)

	// b read the empty store (Version 0) at the same time as a, and writes now.
	conflict(t, Lease{Holder: "b", Epoch: 1, AcquireTime: at(0), RenewTime: at(0), Duration: 10 * time.Second, Version: 0})
	wantStored(t, a1) // unchanged

	// A Version from the future is not the stored one either.
	conflict(t, Lease{Holder: "b", Epoch: 2, Version: 5})
	wantStored(t, a1)

	// a renews, with the Version it got back from the last Update.
	renewed := a1
	renewed.RenewTime = at(2)
	renewed2 := renewed
	renewed2.Version = 2
	update(t, renewed, renewed2)
	wantStored(t, renewed2)

	// The old version is stale now.
	conflict(t, a1)
	wantStored(t, renewed2)
}

func TestMemoryStoreDoneContext(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	expired, cancel2 := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel2()

	tests := []struct {
		name string
		ctx  context.Context
		want error
	}{
		{"cancelled", cancelled, context.Canceled},
		{"deadline passed", expired, context.DeadlineExceeded},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := leaderelection.NewMemoryStore()
			if _, err := s.Get(tc.ctx); !errors.Is(err, tc.want) {
				t.Errorf("Get with a %s ctx: error = %v, want %v", tc.name, err, tc.want)
			}
			l := Lease{Holder: "a", Epoch: 1, RenewTime: at(0), Duration: 10 * time.Second}
			if _, err := s.Update(tc.ctx, l); !errors.Is(err, tc.want) {
				t.Errorf("Update with a %s ctx: error = %v, want %v", tc.name, err, tc.want)
			}
			if got, _ := s.Get(context.Background()); !sameLease(got, Lease{}) {
				t.Errorf("after Update with a %s ctx, the store holds %s, want it unchanged (empty)", tc.name, show(got))
			}
		})
	}
}

func TestMemoryStoreConcurrentUpdates(t *testing.T) {
	// 8 writers race to read-modify-write the lease. Each successful Update
	// adds 1 to the Epoch it read. If two updates based on the same read both
	// succeed (a "lost update"), the final Epoch is lower than the number of
	// successful updates.
	ctx := context.Background()
	s := leaderelection.NewMemoryStore()
	const writers, attempts = 8, 1000
	var (
		wins     atomic.Int64
		wg       sync.WaitGroup
		mu       sync.Mutex
		problems []string
	)
	report := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		problems = append(problems, fmt.Sprintf(format, args...))
	}
	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if v := recover(); v != nil {
					report("writer %d panicked: %v", w, v)
				}
			}()
			for range attempts {
				cur, err := s.Get(ctx)
				if err != nil {
					report("Get: %v", err)
					return
				}
				next := cur
				next.Holder = fmt.Sprintf("w%d", w)
				next.Epoch++
				_, err = s.Update(ctx, next)
				switch {
				case err == nil:
					wins.Add(1)
				case !errors.Is(err, leaderelection.ErrConflict):
					report("Update: error = %v, want nil or ErrConflict", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	for _, p := range problems {
		t.Error(p)
	}
	got, err := s.Get(ctx)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	n := wins.Load()
	if n == 0 {
		t.Fatalf("no Update succeeded, want some")
	}
	if got.Version != n {
		t.Errorf("Version = %d after %d successful updates, want %d", got.Version, n, n)
	}
	if got.Epoch != uint64(n) {
		t.Errorf("Epoch = %d after %d successful updates that each added 1: two updates based on the same read both succeeded; compare and store in one atomic step", got.Epoch, n)
	}
}
