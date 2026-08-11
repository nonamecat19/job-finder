//go:build integration

package retrieval

import (
	"testing"

	"github.com/nonamecat19/jobscraper/ports"
)

func TestIntegration_NoDailyCap_250SequentialFetchesAllAttempted(t *testing.T) {
	sh := newStubHost(t)
	outcomes := sh.fetchMany(t, 250)

	if len(outcomes) != 250 {
		t.Fatalf("expected 250 outcomes, got %d", len(outcomes))
	}
	for i, o := range outcomes {
		if o.Status != ports.PageRead {
			t.Errorf("outcome %d: expected ports.PageRead, got %s (reason=%q)", i, o.Status, o.Reason)
		}
	}
}

func TestIntegration_NoOutcomeReasonMentionsBudget(t *testing.T) {
	sh := newStubHost(t)
	outcomes := sh.fetchMany(t, 250)
	assertNoDeferralsOrBudgetLanguage(t, outcomes)
}
