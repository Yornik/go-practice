package rollout_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	rollout "github.com/yornik/go-practice/exercises/21-rollout"
)

// Step 5: a closed Ticks channel and invalid options.

func TestWaitEndTicksClosed(t *testing.T) {
	tests := []struct {
		name      string
		maxErrors int
		ticks     int // the test closes Ticks after this many ticks
		script    []result
		want      rollout.Status
		wantErr   error // nil: Wait returns nil
	}{
		{
			name:    "closed before the first tick",
			ticks:   0,
			script:  []result{ok(3, 1, 1)},
			want:    st(3, 1, 1),
			wantErr: rollout.ErrStopped,
		},
		{
			name:    "closed after 2 ticks",
			ticks:   2,
			script:  []result{ok(3, 0, 0), ok(3, 1, 0), ok(3, 2, 1)},
			want:    st(3, 2, 1),
			wantErr: rollout.ErrStopped,
		},
		{
			name:      "closed after a failed poll",
			maxErrors: 1,
			ticks:     1,
			script:    []result{ok(3, 1, 1), fail(errRefused)},
			want:      st(3, 1, 1),
			wantErr:   rollout.ErrStopped,
		},
		{
			name:   "closed, but done at the first poll",
			ticks:  0,
			script: []result{ok(3, 3, 3)},
			want:   st(3, 3, 3),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			p := &poller{script: tc.script}
			opt := rollout.Options{Poll: p.Poll, MaxErrors: tc.maxErrors}
			c := start(t, context.Background(), opt)
			c.sendTicks(tc.ticks)
			close(c.ticks)
			got, err := c.result()

			in := fmt.Sprintf("%s, polls %s, Ticks closed after %d ticks", opts(opt), p.describe(), tc.ticks)
			switch {
			case tc.wantErr == nil && err != nil:
				t.Errorf("%s:\nWait = %+v, %v\nwant %+v, nil", in, got, err, tc.want)
			case tc.wantErr != nil && !errors.Is(err, tc.wantErr):
				t.Errorf("%s:\nWait = %+v, %v\nwant %+v and an error that wraps %q", in, got, err, tc.want, tc.wantErr)
			case got != tc.want:
				t.Errorf("%s:\nWait = %+v, %v\nwant status %+v", in, got, err, tc.want)
			}
			checkPolls(t, p, c.sent, len(tc.script))
		})
	}
}

func TestWaitEndInvalidOptions(t *testing.T) {
	ticks := make(chan time.Time) // nobody sends on it
	p := &poller{}
	tests := []struct {
		name string
		opt  rollout.Options
	}{
		{"zero Options", rollout.Options{}},
		{"nil Poll", rollout.Options{Ticks: ticks}},
		{"nil Ticks", rollout.Options{Poll: p.Poll}},
		{"StallTicks -1", rollout.Options{Poll: p.Poll, Ticks: ticks, StallTicks: -1}},
		{"MaxErrors -1", rollout.Options{Poll: p.Poll, Ticks: ticks, MaxErrors: -1}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defer catchPanic(t)
			before := p.Calls()
			got, err := startRaw(t, context.Background(), tc.opt).result()

			if !errors.Is(err, rollout.ErrInvalid) || got != (rollout.Status{}) {
				t.Errorf("%s:\nWait = %+v, %v\nwant the zero Status and an error that wraps %q", tc.name, got, err, rollout.ErrInvalid)
			}
			if n := p.Calls() - before; n != 0 {
				t.Errorf("%s: Poll was called %d times, want 0", tc.name, n)
			}
		})
	}
}
