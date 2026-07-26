package matching

import (
	"encoding/json"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/dto"
)

func jsonOrNull(v []string) ([]byte, error) {
	if v == nil {
		return []byte("null"), nil
	}
	return json.Marshal(v)
}

func toDto(r sqlcgen.MatchResult) dto.MatchResultDto {
	var score *int
	if r.Score != nil {
		v := int(*r.Score)
		score = &v
	}
	var matched, missing, redFlags *[]string
	var m, mi, rf []string
	if dbutil.UnmarshalJSONB(r.MatchedSkills, &m) == nil && r.MatchedSkills != nil && string(r.MatchedSkills) != "null" {
		matched = &m
	}
	if dbutil.UnmarshalJSONB(r.MissingSkills, &mi) == nil && r.MissingSkills != nil && string(r.MissingSkills) != "null" {
		missing = &mi
	}
	if dbutil.UnmarshalJSONB(r.RedFlags, &rf) == nil && r.RedFlags != nil && string(r.RedFlags) != "null" {
		redFlags = &rf
	}
	return dto.MatchResultDto{
		ID:            dbutil.UUIDString(r.ID),
		JobID:         dbutil.UUIDString(r.JobId),
		Similarity:    r.Similarity,
		Score:         score,
		MatchedSkills: matched,
		MissingSkills: missing,
		Summary:       r.Summary,
		RedFlags:      redFlags,
		Model:         r.Model,
		CreatedAt:     dbutil.Timestamp(r.CreatedAt),
	}
}
