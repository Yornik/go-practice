package rollout_test

import (
	"testing"

	rollout "github.com/yornik/go-practice/exercises/21-rollout"
)

// Step 2: progress and StallTicks. No errors.

func TestWaitStalled(t *testing.T) {
	tests := []waitCase{
		{
			name:       "StallTicks 3: the 3rd poll without progress ends Wait",
			stallTicks: 3,
			script:     []result{ok(3, 1, 1), ok(3, 1, 1), ok(3, 1, 1), ok(3, 1, 1)},
			want:       st(3, 1, 1),
			wantErr:    rollout.ErrStalled,
			polls:      4,
		},
		{
			name:       "StallTicks 1: the 1st poll without progress ends Wait",
			stallTicks: 1,
			script:     []result{ok(3, 0, 0), ok(3, 0, 0)},
			want:       st(3, 0, 0),
			wantErr:    rollout.ErrStalled,
			polls:      2,
		},
		{
			name:       "progress resets the count",
			stallTicks: 3,
			script: []result{
				ok(3, 1, 0), ok(3, 1, 0), ok(3, 1, 0), // 2 polls without progress
				ok(3, 1, 1),                           // progress
				ok(3, 1, 1), ok(3, 1, 1), ok(3, 1, 1), // 3 polls without progress
			},
			want:    st(3, 1, 1),
			wantErr: rollout.ErrStalled,
			polls:   7,
		},
		{
			name:       "a drop in Ready is not progress",
			stallTicks: 2,
			script:     []result{ok(3, 2, 2), ok(3, 2, 1), ok(3, 2, 1)},
			want:       st(3, 2, 1),
			wantErr:    rollout.ErrStalled,
			polls:      3,
		},
		{
			name:       "a return to the highest Ready is not progress",
			stallTicks: 2,
			script:     []result{ok(3, 2, 2), ok(3, 2, 1), ok(3, 2, 2)},
			want:       st(3, 2, 2),
			wantErr:    rollout.ErrStalled,
			polls:      3,
		},
		{
			name:       "a higher Updated is progress while Ready drops",
			stallTicks: 2,
			script:     []result{ok(3, 1, 1), ok(3, 2, 0), ok(3, 2, 0), ok(3, 2, 1)},
			want:       st(3, 2, 1),
			wantErr:    rollout.ErrStalled,
			polls:      4,
		},
		{
			name:       "a change of Desired is not progress",
			stallTicks: 2,
			script:     []result{ok(3, 1, 1), ok(4, 1, 1), ok(5, 1, 1)},
			want:       st(5, 1, 1),
			wantErr:    rollout.ErrStalled,
			polls:      3,
		},
		{
			name:       "StallTicks 1 with progress at every poll",
			stallTicks: 1,
			script:     []result{ok(3, 1, 0), ok(3, 1, 1), ok(3, 2, 1), ok(3, 3, 2), ok(3, 3, 3)},
			want:       st(3, 3, 3),
			polls:      5,
		},
		{
			name:       "StallTicks 0 never stalls",
			stallTicks: 0,
			script: []result{
				ok(3, 1, 1), ok(3, 1, 1), ok(3, 1, 1), ok(3, 1, 1), ok(3, 1, 1),
				ok(3, 1, 1), ok(3, 1, 1), ok(3, 1, 1), ok(3, 1, 1), ok(3, 1, 1),
				ok(3, 3, 3),
			},
			want:  st(3, 3, 3),
			polls: 11,
		},
		{
			name:       "done is checked before stall",
			stallTicks: 1,
			script:     []result{ok(3, 2, 2), ok(2, 2, 2)}, // scaled down: done, without progress
			want:       st(2, 2, 2),
			polls:      2,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}
