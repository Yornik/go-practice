package rollout_test

import (
	"testing"

	rollout "github.com/yornik/go-practice/exercises/21-rollout"
)

// Step 3: failed polls and MaxErrors.

func TestWaitErrors(t *testing.T) {
	tests := []waitCase{
		{
			name:    "MaxErrors 0: the first poll fails",
			script:  []result{fail(errRefused)},
			want:    st(0, 0, 0),
			wantErr: errRefused,
			polls:   1,
		},
		{
			name:    "MaxErrors 0: a poll fails after a success",
			script:  []result{ok(3, 1, 1), fail(errRefused)},
			want:    st(3, 1, 1),
			wantErr: errRefused,
			polls:   2,
		},
		{
			name:      "MaxErrors 2: the 3rd failure in a row ends Wait with the last error",
			maxErrors: 2,
			script:    []result{ok(3, 1, 1), fail(errRefused), fail(errTimeout), fail(errDenied)},
			want:      st(3, 1, 1),
			wantErr:   errDenied,
			polls:     4,
		},
		{
			name:      "MaxErrors 2: a success resets the count",
			maxErrors: 2,
			script: []result{
				ok(3, 0, 0), fail(errRefused), fail(errTimeout),
				ok(3, 1, 1), fail(errRefused), fail(errTimeout),
				ok(3, 3, 3),
			},
			want:  st(3, 3, 3),
			polls: 7,
		},
		{
			name:      "MaxErrors 2: failures before the first success",
			maxErrors: 2,
			script:    []result{fail(errRefused), fail(errTimeout), ok(3, 3, 3)},
			want:      st(3, 3, 3),
			polls:     3,
		},
		{
			name:      "the status of a failed poll is ignored for done",
			maxErrors: 1,
			script:    []result{{status: st(3, 3, 3), err: errTimeout}, ok(3, 3, 3)},
			want:      st(3, 3, 3),
			polls:     2,
		},
		{
			name:    "the status of a failed poll is not returned",
			script:  []result{ok(3, 1, 1), {status: st(3, 2, 2), err: errTimeout}},
			want:    st(3, 1, 1),
			wantErr: errTimeout,
			polls:   2,
		},
		{
			name:       "a failed poll is a poll without progress",
			maxErrors:  5,
			stallTicks: 2,
			script:     []result{ok(3, 1, 1), fail(errRefused), fail(errTimeout)},
			want:       st(3, 1, 1),
			wantErr:    rollout.ErrStalled,
			polls:      3,
		},
		{
			name:       "a success without progress after a failure does not reset the stall count",
			maxErrors:  1,
			stallTicks: 3,
			script:     []result{ok(3, 1, 1), ok(3, 1, 1), fail(errRefused), ok(3, 1, 1)},
			want:       st(3, 1, 1),
			wantErr:    rollout.ErrStalled,
			polls:      4,
		},
		{
			name:       "errors are checked before stall",
			maxErrors:  1,
			stallTicks: 2,
			script:     []result{ok(3, 1, 1), fail(errRefused), fail(errTimeout)},
			want:       st(3, 1, 1),
			wantErr:    errTimeout,
			notErr:     rollout.ErrStalled,
			polls:      3,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}
