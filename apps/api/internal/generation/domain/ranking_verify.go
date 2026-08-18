package domain

import "fmt"

type RankingViolationKind string

const (
	RankingOutOfRange RankingViolationKind = "out_of_range"
	RankingDuplicate  RankingViolationKind = "duplicate"
	RankingShort      RankingViolationKind = "short"
)

type RankingViolation struct {
	Kind    RankingViolationKind
	Index   int
	Message string
}

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

func VerifySkillGroupOrder(groupCount int, order []int) []RankingViolation {
	return VerifyRanking(groupCount, groupCount, order)
}

func MasterOrderRanking(available, target int) []int {
	k := RankingK(available, target)
	out := make([]int, k)
	for i := range out {
		out[i] = i
	}
	return out
}
