package referral

import (
	"sort"
)

type Ranker struct{}

func NewRanker() *Ranker {
	return &Ranker{}
}

func (r *Ranker) Rank(paths []ReferralPath) []ReferralPath {
	ranked := make([]ReferralPath, len(paths))
	copy(ranked, paths)

	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].Length < ranked[j].Length
	})

	return ranked
}

func (r *Ranker) TopN(paths []ReferralPath, n int) []ReferralPath {
	ranked := r.Rank(paths)
	if len(ranked) > n {
		return ranked[:n]
	}
	return ranked
}
