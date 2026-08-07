package application

import (
	"context"
	"fmt"

	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/postage/domain"
)

type Service struct {
	q domain.Repository
}

func NewService(q domain.Repository) *Service {
	return &Service{q: q}
}

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

func bucketState(totalApps, bucketN int32) dto.PostAgeBucketState {
	if totalApps < domain.GlobalColdStartThreshold {
		return dto.PostAgeStatePrior
	}
	if bucketN < domain.PerBucketMinSample {
		return dto.PostAgeStateInsufficient
	}
	return dto.PostAgeStateObserved
}
