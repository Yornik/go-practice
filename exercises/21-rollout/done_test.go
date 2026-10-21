package rollout_test

import "testing"

// Step 1: polls until the rollout is done. No stall limit, no errors.

func TestWaitDone(t *testing.T) {
	tests := []waitCase{
		{
			name:   "done at the first poll",
			script: []result{ok(3, 3, 3)},
			want:   st(3, 3, 3),
			polls:  1,
		},
		{
			name:   "done after 3 ticks",
			script: []result{ok(3, 0, 0), ok(3, 1, 0), ok(3, 2, 1), ok(3, 3, 3)},
			want:   st(3, 3, 3),
			polls:  4,
		},
		{
			name:   "Desired 0 is done",
			script: []result{ok(0, 0, 0)},
			want:   st(0, 0, 0),
			polls:  1,
		},
		{
			name:   "all updated but not all ready",
			script: []result{ok(3, 3, 2), ok(3, 3, 3)},
			want:   st(3, 3, 3),
			polls:  2,
		},
		{
			name:   "all ready but not all updated",
			script: []result{ok(3, 2, 3), ok(3, 3, 3)},
			want:   st(3, 3, 3),
			polls:  2,
		},
		{
			name: "24 replicas",
			script: []result{
				ok(24, 6, 0), ok(24, 6, 6), ok(24, 12, 6), ok(24, 12, 12),
				ok(24, 18, 12), ok(24, 18, 18), ok(24, 24, 18), ok(24, 24, 24),
			},
			want:  st(24, 24, 24),
			polls: 8,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, tc.run)
	}
}
