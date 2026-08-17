package domain

import (
	"reflect"
	"testing"
)

func TestFilterGroundedRewordingsKeepsAGroundedProposal(t *testing.T) {
	source := "Rewrote the ingestion pipeline in Go, cutting p99 latency from 900ms to 120ms."
	proposals := []string{"Rebuilt the Go ingestion pipeline, taking p99 latency from 900ms to 120ms."}

	got := FilterGroundedRewordings(source, proposals)
	if !reflect.DeepEqual(got, proposals) {
		t.Errorf("got %v, want the proposal kept as-is", got)
	}
}

func TestFilterGroundedRewordingsDropsANewNumber(t *testing.T) {
	source := "Ran the quarterly cost review, reducing cloud spend 18%."
	proposals := []string{"Ran the quarterly cost review, reducing cloud spend 25%."}

	got := FilterGroundedRewordings(source, proposals)
	if len(got) != 0 {
		t.Errorf("got %v, want the ungrounded metric dropped", got)
	}
}

func TestFilterGroundedRewordingsDropsADuplicateOfSource(t *testing.T) {
	source := "Owned Postgres schema migrations across 14 services with zero-downtime deploys."
	proposals := []string{"  Owned Postgres schema migrations across 14 services with zero-downtime deploys.  "}

	got := FilterGroundedRewordings(source, proposals)
	if len(got) != 0 {
		t.Errorf("got %v, want the source-identical proposal dropped", got)
	}
}

func TestFilterGroundedRewordingsDropsEmptyAndDuplicateProposals(t *testing.T) {
	source := "Extended the Kafka topology to nine consumers and halved rebalance time."
	proposals := []string{
		"   ",
		"Grew the Kafka topology to nine consumers, cutting rebalance time in half.",
		"Grew the Kafka topology to nine consumers, cutting rebalance time in half.",
	}

	got := FilterGroundedRewordings(source, proposals)
	want := []string{"Grew the Kafka topology to nine consumers, cutting rebalance time in half."}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFilterGroundedRewordingsDropsAnUnrelatedProposal(t *testing.T) {
	source := "Built the billing reconciliation service in Go, closing a 0.4% revenue leak."
	proposals := []string{"Mentored three engineers through their first on-call rotation."}

	got := FilterGroundedRewordings(source, proposals)
	if len(got) != 0 {
		t.Errorf("got %v, want the unrelated proposal dropped", got)
	}
}
