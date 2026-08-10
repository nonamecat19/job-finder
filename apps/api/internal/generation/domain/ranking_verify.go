package domain

import "fmt"

// RankingViolationKind names one of the three structural defects a ranking
// response can carry (contracts/llm-contracts.md §1). There is no relevance
// or quality check here — VerifyRanking is checkable in O(K) with no
// reference to what "relevant" means, which is what makes it a verifier
// rather than a judge (038 FR-003).
type RankingViolationKind string

const (
	RankingOutOfRange RankingViolationKind = "out_of_range"
	RankingDuplicate  RankingViolationKind = "duplicate"
	RankingShort      RankingViolationKind = "short"
)

// RankingViolation is one structural defect found in a ranking response.
// Index is the offending value for out_of_range/duplicate, and -1 for short
// (a property of the whole list, not of one entry).
type RankingViolation struct {
	Kind    RankingViolationKind
	Index   int
	Message string
}

// RankingK is K = min(2*target, available) — the exact candidate count
// research.md R2 fixes so "rank up to 2N and reject an omission" is
// checkable without superlinear cost: the response is invalid iff it fails
// to name K distinct in-range indices.
func RankingK(available, target int) int {
	if available < 0 {
		available = 0
	}
	k := 2 * target
	if k > available {
		k = available
	}
	if k < 0 {
		k = 0
	}
	return k
}

// VerifyRanking checks a ranking response structurally against K =
// RankingK(available, target): every index in [0, available), no index
// repeated, and at least K entries returned.
//
// len(ranking) > K is deliberately NOT a violation — extra ranked indices
// are accepted, and the display simply shows more ranked candidates than
// required. Rejecting a model that ranked more material than asked would be
// a rejection with no user-visible defect behind it
// (contracts/llm-contracts.md §1).
func VerifyRanking(available, target int, ranking []int) []RankingViolation {
	var violations []RankingViolation
	seen := make(map[int]bool, len(ranking))
	for _, idx := range ranking {
		switch {
		case idx < 0 || idx >= available:
			violations = append(violations, RankingViolation{
				Kind: RankingOutOfRange, Index: idx,
				Message: fmt.Sprintf("index %d is out of range [0,%d)", idx, available),
			})
		case seen[idx]:
			violations = append(violations, RankingViolation{
				Kind: RankingDuplicate, Index: idx,
				Message: fmt.Sprintf("index %d appears more than once", idx),
			})
		default:
			seen[idx] = true
		}
	}

	k := RankingK(available, target)
	if len(ranking) < k {
		violations = append(violations, RankingViolation{
			Kind: RankingShort, Index: -1,
			Message: fmt.Sprintf("ranking has %d entries, want at least %d (K = min(2*%d, %d))", len(ranking), k, target, available),
		})
	}
	return violations
}

// VerifySkillGroupOrder applies VerifyRanking's three structural checks to
// RankedSkills.GroupOrder (contracts/llm-contracts.md §1): every index in
// [0, groupCount), none repeated, and none missing. The prompt asks for every
// skill group index exactly once, so K here is groupCount itself — passing
// target == available collapses min(2*target, available) to available, which
// is what makes "short" mean "omitted a group" on this surface while it means
// "ranked fewer than 2N candidates" on an achievement ranking.
//
// The consequence of an omission is the same as it is for achievements: retry
// once, then fall back to master order (FR-010). A group is never dropped over
// a bad ranking, because the order and the list are separate concerns —
// SeedSkillItems emits every group whatever the order says.
func VerifySkillGroupOrder(groupCount int, order []int) []RankingViolation {
	return VerifyRanking(groupCount, groupCount, order)
}

// MasterOrderRanking is the FR-010 fallback: the first K indices in master
// order, used when a ranking is rejected twice.
func MasterOrderRanking(available, target int) []int {
	k := RankingK(available, target)
	out := make([]int, k)
	for i := range out {
		out[i] = i
	}
	return out
}
