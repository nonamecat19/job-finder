package application

import (
	"context"
	"fmt"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/postage/domain"
)

// Service computes the post-age-at-apply vs response-rate signal from the
// ApplicationOutcome event log. It is a deterministic SQL aggregation — no
// model, no LLM in the signal path.
type Service struct {
	q domain.Repository
}

func NewService(q domain.Repository) *Service {
	return &Service{q: q}
}

// Compute returns the full post-age vs response-rate signal with cold-start
// honesty rules applied. The raw SQL aggregation is bucketed, then each bucket
// is classified as observed / insufficient / prior per the spec thresholds.
func (s *Service) Compute(ctx context.Context) (dto.PostAgeResponseDto, error) {
	rows, err := s.q.PostAgeResponseRate(ctx)
	if err != nil {
		return dto.PostAgeResponseDto{}, fmt.Errorf("postage: query failed: %w", err)
	}

	var totalApps int32
	for _, r := range rows {
		totalApps += r.N
	}

	globalState := dto.PostAgeStateObserved
	if totalApps < domain.GlobalColdStartThreshold {
		globalState = dto.PostAgeStatePrior
	}

	buckets := make([]dto.PostAgeBucketDto, 0, len(rows))
	for _, r := range rows {
		b := dto.PostAgeBucketDto{
			Bucket:    r.Bucket,
			N:         r.N,
			Responses: r.Responses,
			State:     bucketState(totalApps, r.N),
		}
		if b.State == dto.PostAgeStateObserved {
			rate := float64(r.Responses) / float64(r.N)
			b.Rate = &rate
		}
		buckets = append(buckets, b)
	}

	resp := dto.PostAgeResponseDto{
		Buckets:     buckets,
		TotalApps:   totalApps,
		GlobalState: globalState,
		PriorRate:   domain.DocumentedPriorRate,
		PriorLabel:  domain.DocumentedPriorLabel,
	}

	if globalState == dto.PostAgeStatePrior {
		remaining := domain.GlobalColdStartThreshold - totalApps
		msg := fmt.Sprintf("Need %d more applications before showing your personal response rate.", remaining)
		resp.ThresholdMsg = &msg
	}

	return resp, nil
}

// bucketState classifies a single bucket per the three-state rules:
//   - Total < global threshold → prior (handled by caller for the overall view;
//     per-bucket we still show insufficient when below per-bucket min)
//   - Total ≥ global threshold, bucket < per-bucket min → insufficient
//   - Total ≥ global threshold, bucket ≥ per-bucket min → observed
func bucketState(totalApps, bucketN int32) dto.PostAgeBucketState {
	if totalApps < domain.GlobalColdStartThreshold {
		return dto.PostAgeStatePrior
	}
	if bucketN < domain.PerBucketMinSample {
		return dto.PostAgeStateInsufficient
	}
	return dto.PostAgeStateObserved
}
