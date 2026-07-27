//go:build integration

package retrieval

import "testing"

// FR-001/FR-002: a host must never be refused for volume. This drives a
// single host past the old 200-request daily cap in one session; before the
// budget removal every request from #201 onward comes back PageDeferred
// with a "daily budget exhausted" reason.
func TestIntegration_NoDailyCap_250SequentialFetchesAllAttempted(t *testing.T) {
	sh := newStubHost(t)
	outcomes := sh.fetchMany(t, 250)

	if len(outcomes) != 250 {
		t.Fatalf("expected 250 outcomes, got %d", len(outcomes))
	}
	for i, o := range outcomes {
		if o.Status != PageRead {
			t.Errorf("outcome %d: expected PageRead, got %s (reason=%q)", i, o.Status, o.Reason)
		}
	}
}

// FR-017: no reason string may ever cite a budget, even in outcomes that are
// otherwise fine. Reuses the shared assertion helper from testhelpers_test.go.
func TestIntegration_NoOutcomeReasonMentionsBudget(t *testing.T) {
	sh := newStubHost(t)
	outcomes := sh.fetchMany(t, 250)
	assertNoDeferralsOrBudgetLanguage(t, outcomes)
}
