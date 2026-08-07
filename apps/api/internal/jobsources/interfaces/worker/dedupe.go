package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
	"github.com/job-finder/api/internal/jobsources/domain"
)

var boardVendors = []string{"greenhouse", "lever", "ashby", "workable", "smartrecruiters"}

func IsBoardVendor(sourceKey string) bool {
	return slices.Contains(boardVendors, sourceKey)
}

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "of": true,
	"in": true, "to": true, "for": true, "is": true, "at": true, "on": true,
	"with": true, "by": true, "as": true, "be": true, "it": true, "its": true,
	"not": true, "no": true, "are": true, "was": true, "we": true, "you": true,
	"our": true, "your": true, "all": true, "will": true, "from": true,
}

func normalizeWords(s string) []string {
	words := strings.Fields(strings.ToLower(s))
	var out []string
	for _, w := range words {
		w = strings.Trim(w, ",./;'[]()!?*&^%$#@~`\"-+=")
		if len(w) < 2 || stopWords[w] {
			continue
		}
		out = append(out, w)
	}
	return out
}

func titlesOverlap(a, b string) bool {
	if strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b)) {
		return true
	}
	wa := normalizeWords(a)
	wb := normalizeWords(b)
	if len(wa) == 0 || len(wb) == 0 {
		return false
	}
	la := strings.ToLower(strings.TrimSpace(a))
	lb := strings.ToLower(strings.TrimSpace(b))
	if strings.Contains(la, lb) || strings.Contains(lb, la) {
		return true
	}
	shared := 0
	for _, aw := range wa {
		for _, bw := range wb {
			if aw == bw {
				shared++
				break
			}
		}
	}
	minLen := len(wa)
	if len(wb) < minLen {
		minLen = len(wb)
	}
	return shared >= 2 || (minLen == 1 && shared >= 1)
}

func CanonicalURL(rawURL string) string {
	return strings.TrimRight(strings.SplitN(rawURL, "?", 2)[0], "/")
}

func DedupeKey(company, title, rawURL string) string {
	canonical := CanonicalURL(rawURL)
	sum := sha256.Sum256([]byte(strings.ToLower(company) + "|" + strings.ToLower(title) + "|" + canonical))
	return hex.EncodeToString(sum[:])
}

func FindMergeCandidate(ctx context.Context, q domain.SearchRepository, j dto.NormalizedJob) (pgtype.UUID, error) {
	if !IsBoardVendor(j.SourceKey) {
		return pgtype.UUID{}, nil
	}

	row, err := q.FindJobByCompany(ctx, sqlcgen.FindJobByCompanyParams{
		Lower:     j.Company,
		SourceKey: j.SourceKey,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, nil
	}
	if err != nil {
		return pgtype.UUID{}, err
	}

	if titlesOverlap(j.Title, row.Title) {
		return row.ID, nil
	}

	return pgtype.UUID{}, nil
}

func FindMergeCandidates(ctx context.Context, q domain.BatchRepository, postings []dto.NormalizedJob) (map[string]pgtype.UUID, int, error) {
	bySource := map[string][]dto.NormalizedJob{}
	for _, j := range postings {
		if IsBoardVendor(j.SourceKey) {
			bySource[j.SourceKey] = append(bySource[j.SourceKey], j)
		}
	}
	if len(bySource) == 0 {
		return nil, 0, nil
	}

	targets := map[string]pgtype.UUID{}
	statements := 0
	for _, sourceKey := range slices.Sorted(maps.Keys(bySource)) {
		group := bySource[sourceKey]
		seen := map[string]bool{}
		var companies []string
		for _, j := range group {
			company := strings.ToLower(j.Company)
			if !seen[company] {
				seen[company] = true
				companies = append(companies, company)
			}
		}

		rows, err := q.FindJobsByCompanies(ctx, sqlcgen.FindJobsByCompaniesParams{
			Companies: companies,
			SourceKey: sourceKey,
		})
		if err != nil {
			return nil, 0, fmt.Errorf("ingestion: find merge candidates: %w", err)
		}
		statements++

		byCompany := make(map[string]sqlcgen.FindJobsByCompaniesRow, len(rows))
		for _, row := range rows {
			byCompany[row.CompanyKey] = row
		}
		for _, j := range group {
			row, ok := byCompany[strings.ToLower(j.Company)]
			if ok && titlesOverlap(j.Title, row.Title) {
				targets[DedupeKey(j.Company, j.Title, j.URL)] = row.ID
			}
		}
	}
	return targets, statements, nil
}
