package events

import "testing"

func TestDecide_FixedLadder(t *testing.T) {
	failure := Failure{Category: FailureProviderUnavailable, Retryable: true}

	cases := []struct {
		currentAttempt int
		wantRung       string
	}{
		{0, "1s"},
		{1, "10s"},
		{2, "1m"},
		{3, "10m"},
		{4, "10m"}, // beyond the ladder's length reuses the longest rung (M4-2)
	}

	for _, tc := range cases {
		d := Decide("generate", tc.currentAttempt, failure)
		if d.DeadLetter {
			t.Fatalf("attempt %d: got dead-letter, want retry", tc.currentAttempt)
		}
		if d.Rung != tc.wantRung {
			t.Errorf("attempt %d: rung = %q, want %q", tc.currentAttempt, d.Rung, tc.wantRung)
		}
	}
}

func TestDecide_RateLimitedEntersAt1m(t *testing.T) {
	failure := Failure{Category: FailureRateLimited, Retryable: true}

	d := Decide("match", 0, failure)
	if d.DeadLetter {
		t.Fatal("got dead-letter, want retry")
	}
	if d.Rung != "1m" {
		t.Errorf("rung = %q, want 1m", d.Rung)
	}
}

func TestDecide_BudgetExhaustionDeadLetters(t *testing.T) {
	failure := Failure{Category: FailureProviderUnavailable, Retryable: true}

	// "generate" has a budget of 5 attempts (MaxAttempts). The 5th attempt
	// already happened (currentAttempt=5); the next one exceeds budget.
	d := Decide("generate", MaxAttempts("generate"), failure)
	if !d.DeadLetter {
		t.Fatalf("got retry at rung %q, want dead-letter", d.Rung)
	}
}

func TestDecide_IngestBudgetExhaustionDeadLetters(t *testing.T) {
	failure := Failure{Category: FailureProviderUnavailable, Retryable: true}

	if got := MaxAttempts("ingest"); got != 3 {
		t.Fatalf("MaxAttempts(ingest) = %d, want 3", got)
	}

	// Still within budget.
	d := Decide("ingest", 2, failure)
	if d.DeadLetter {
		t.Fatal("attempt 3 of 3: got dead-letter, want retry")
	}

	// Exhausted.
	d = Decide("ingest", 3, failure)
	if !d.DeadLetter {
		t.Fatal("attempt 4 of 3: got retry, want dead-letter")
	}
}

func TestDecide_NonRetryableGoesStraightToDLQ(t *testing.T) {
	failure := Failure{Category: FailureInvalidInput, Retryable: false}

	// Even on the very first attempt, with budget untouched, a
	// non-retryable failure must not consume retry budget (M4-3).
	d := Decide("match", 0, failure)
	if !d.DeadLetter {
		t.Fatal("got retry, want dead-letter for non-retryable failure")
	}
}

func TestDecide_RetryableReadFromFailureNotReDerived(t *testing.T) {
	// FailureRateLimited defaults to retryable, but the consumer must trust
	// the carried value, not re-derive it from the category (FR-004, E5-2).
	failure := Failure{Category: FailureRateLimited, Retryable: false}

	d := Decide("match", 0, failure)
	if !d.DeadLetter {
		t.Fatal("got retry, want dead-letter: Retryable=false on the failure must be honored even though the category defaults to retryable")
	}
}

func TestRateLimitRetryDelay_DoublingAndCeiling(t *testing.T) {
	cases := []struct {
		attempt int
		want    string
	}{
		{0, "1m0s"},
		{1, "2m0s"},
		{2, "4m0s"},
		{3, "8m0s"},
		{4, "15m0s"},  // would be 16m, capped at the 15m ceiling
		{10, "15m0s"}, // stays capped
	}
	for _, tc := range cases {
		got := RateLimitRetryDelay(tc.attempt)
		if got.String() != tc.want {
			t.Errorf("RateLimitRetryDelay(%d) = %s, want %s", tc.attempt, got, tc.want)
		}
	}
}
