package retry_test

import (
	"errors"
	"math"
	"testing"
	"time"

	retry "github.com/yornik/go-practice/exercises/07-retry"
)

// Step 1: the policy. These are pure functions: no time, no context.

func TestPolicyValidate(t *testing.T) {
	defer catchPanic(t)
	valid := []struct {
		name string
		p    retry.Policy
	}{
		{"base policy", basePolicy},
		{"1 attempt means no retries", retry.Policy{MaxAttempts: 1, Initial: ms, Max: ms, Multiplier: 1}},
		{"Multiplier 1 is a constant backoff", retry.Policy{MaxAttempts: 3, Initial: time.Second, Max: time.Minute, Multiplier: 1}},
		{"Max equal to Initial", retry.Policy{MaxAttempts: 3, Initial: time.Second, Max: time.Second, Multiplier: 2}},
		{"fractional Multiplier", retry.Policy{MaxAttempts: 3, Initial: time.Second, Max: time.Minute, Multiplier: 1.5}},
		{"Jitter without Rand, Sleep and Rand nil", retry.Policy{MaxAttempts: 3, Initial: time.Second, Max: time.Minute, Multiplier: 2, Jitter: true}},
	}
	for _, tc := range valid {
		if err := tc.p.Validate(); err != nil {
			t.Errorf("%s: Validate() = %v, want nil", tc.name, err)
		}
	}

	invalid := []struct {
		name string
		p    retry.Policy
	}{
		{"zero Policy", retry.Policy{}},
		{"MaxAttempts 0", retry.Policy{MaxAttempts: 0, Initial: ms, Max: time.Second, Multiplier: 2}},
		{"MaxAttempts negative", retry.Policy{MaxAttempts: -1, Initial: ms, Max: time.Second, Multiplier: 2}},
		{"Initial 0", retry.Policy{MaxAttempts: 3, Initial: 0, Max: time.Second, Multiplier: 2}},
		{"Initial negative", retry.Policy{MaxAttempts: 3, Initial: -ms, Max: time.Second, Multiplier: 2}},
		{"Multiplier 0", retry.Policy{MaxAttempts: 3, Initial: ms, Max: time.Second, Multiplier: 0}},
		{"Multiplier below 1 would shrink the backoff", retry.Policy{MaxAttempts: 3, Initial: ms, Max: time.Second, Multiplier: 0.5}},
		{"Max below Initial", retry.Policy{MaxAttempts: 3, Initial: time.Second, Max: 500 * ms, Multiplier: 2}},
		{"Max 0", retry.Policy{MaxAttempts: 3, Initial: time.Second, Max: 0, Multiplier: 2}},
	}
	for _, tc := range invalid {
		err := tc.p.Validate()
		if !errors.Is(err, retry.ErrInvalidPolicy) {
			t.Errorf("%s: Validate() = %v, want an error that wraps ErrInvalidPolicy", tc.name, err)
		}
	}
}

func TestPolicyBackoff(t *testing.T) {
	defer catchPanic(t)
	triple := retry.Policy{MaxAttempts: 10, Initial: time.Second, Max: time.Minute, Multiplier: 3}
	constant := retry.Policy{MaxAttempts: 10, Initial: 250 * ms, Max: time.Second, Multiplier: 1}
	// 1.5 is not an integer: converting it to a time.Duration first gives 1.
	oneAndAHalf := retry.Policy{MaxAttempts: 10, Initial: 100 * ms, Max: time.Second, Multiplier: 1.5}

	tests := []struct {
		name    string
		p       retry.Policy
		attempt int
		want    time.Duration
	}{
		{"base, attempt 1 is Initial", basePolicy, 1, 100 * ms},
		{"base, attempt 2", basePolicy, 2, 200 * ms},
		{"base, attempt 3", basePolicy, 3, 400 * ms},
		{"base, attempt 4", basePolicy, 4, 800 * ms},
		{"base, attempt 5 reaches Max", basePolicy, 5, time.Second},
		{"base, attempt 6 stays at Max", basePolicy, 6, time.Second},
		{"triple, attempt 1", triple, 1, time.Second},
		{"triple, attempt 2", triple, 2, 3 * time.Second},
		{"triple, attempt 4", triple, 4, 27 * time.Second},
		{"triple, attempt 5 is capped", triple, 5, time.Minute},
		{"constant, attempt 1", constant, 1, 250 * ms},
		{"constant, attempt 7", constant, 7, 250 * ms},
		{"Multiplier 1.5, attempt 2", oneAndAHalf, 2, 150 * ms},
		{"Multiplier 1.5, attempt 3", oneAndAHalf, 3, 225 * ms},
		{"Multiplier 1.5, attempt 4 has a fraction of a ms", oneAndAHalf, 4, 337500 * time.Microsecond},
		{"Multiplier 1.5, attempt 6", oneAndAHalf, 6, 759375 * time.Microsecond},
		{"Multiplier 1.5, attempt 7 is capped", oneAndAHalf, 7, time.Second},
	}
	for _, tc := range tests {
		if got := tc.p.Backoff(tc.attempt); got != tc.want {
			t.Errorf("%s: Backoff(%d) = %v, want %v", tc.name, tc.attempt, got, tc.want)
		}
	}
}

func TestPolicyBackoffNoOverflow(t *testing.T) {
	defer catchPanic(t)
	// A controller that "retries forever" reaches attempt numbers where
	// Initial*Multiplier^(attempt-1) no longer fits in an int64 (attempt 35
	// with Initial 1s and Multiplier 2), and then not even in a float64 (it
	// becomes +Inf around attempt 1000). Backoff must still return Max.
	hour := retry.Policy{MaxAttempts: math.MaxInt, Initial: time.Second, Max: time.Hour, Multiplier: 2}
	// Max is the largest possible Duration: "no cap" in practice.
	noCap := retry.Policy{MaxAttempts: math.MaxInt, Initial: time.Second, Max: math.MaxInt64, Multiplier: 2}
	// Initial*Multiplier is exactly 2^63 ns, one more than the largest Duration.
	edge := retry.Policy{MaxAttempts: 3, Initial: 1 << 62, Max: math.MaxInt64, Multiplier: 2}

	tests := []struct {
		name    string
		p       retry.Policy
		attempt int
		want    time.Duration
	}{
		{"Max 1h, attempt 12 is below Max", hour, 12, 2048 * time.Second},
		{"Max 1h, attempt 13 is capped", hour, 13, time.Hour},
		{"Max 1h, attempt 32", hour, 32, time.Hour},
		{"Max 1h, attempt 64", hour, 64, time.Hour},
		{"Max 1h, attempt 65", hour, 65, time.Hour},
		{"Max 1h, attempt 100", hour, 100, time.Hour},
		{"Max 1h, attempt 1100", hour, 1100, time.Hour},
		{"Max 1h, attempt 100000", hour, 100000, time.Hour},
		{"largest Max, attempt 34", noCap, 34, (1 << 33) * time.Second},
		{"largest Max, attempt 100", noCap, 100, math.MaxInt64},
		{"largest Max, attempt 1100", noCap, 1100, math.MaxInt64},
		{"Initial 2^62, attempt 1", edge, 1, 1 << 62},
		{"Initial 2^62, attempt 2 overflows by 1", edge, 2, math.MaxInt64},
	}
	for _, tc := range tests {
		if got := tc.p.Backoff(tc.attempt); got != tc.want {
			t.Errorf("%s: Backoff(%d) = %v, want %v", tc.name, tc.attempt, got, tc.want)
		}
	}
}
