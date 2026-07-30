package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dto"
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

// CanonicalURL strips the query string and trailing slashes from a job URL,
// matching `newJob.url.split('?')[0].replace(/\/+$/, ”)` in
// ingestion.processor.ts:74 exactly.
func CanonicalURL(rawURL string) string {
	return strings.TrimRight(strings.SplitN(rawURL, "?", 2)[0], "/")
}

// DedupeKey computes sha256(lower(company)|lower(title)|canonicalUrl) —
// must match ingestion.processor.ts:74 byte-for-byte or duplicate jobs flood in.
func DedupeKey(company, title, rawURL string) string {
	canonical := CanonicalURL(rawURL)
	sum := sha256.Sum256([]byte(strings.ToLower(company) + "|" + strings.ToLower(title) + "|" + canonical))
	return hex.EncodeToString(sum[:])
}

// FindMergeCandidate checks whether a new job from a board vendor can be
// merged into an existing job from a different source (typically an aggregator).
// It matches by normalized company name (case-insensitive) + similar title
// (significant word overlap). Embedding similarity is deferred to the match
// stage since jobs typically have no embedding at ingestion time.
func FindMergeCandidate(ctx context.Context, q Repository, j dto.NormalizedJob) (pgtype.UUID, error) {
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
