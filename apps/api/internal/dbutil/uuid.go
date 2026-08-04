// Package dbutil holds small conversion helpers shared by every service
// package: pgtype.UUID <-> string, pgtype.Timestamp <-> RFC3339 string/nil,
// and jsonb ([]byte) <-> typed Go values.
package dbutil

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

// UUIDString renders a pgtype.UUID as a canonical dashed string, "" if unset.
func UUIDString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// UUIDStringPtr is UUIDString but returns nil for an invalid/zero UUID —
// used for optional foreign keys like SourceRun.searchId equivalents.
func UUIDStringPtr(u pgtype.UUID) *string {
	if !u.Valid {
		return nil
	}
	s := UUIDString(u)
	return &s
}

// ParseUUID parses a canonical UUID string into pgtype.UUID for use as a
// query parameter.
func ParseUUID(s string) (pgtype.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("invalid id %q: %w", s, err)
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}

// Timestamp renders a pgtype.Timestamp as RFC3339 (UTC), "" if unset.
func Timestamp(t pgtype.Timestamp) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// TimestampPtr is Timestamp but returns nil for an unset timestamp —
// matches the TS `Date | null` -> `string | null` DTO fields.
func TimestampPtr(t pgtype.Timestamp) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.UTC().Format(time.RFC3339)
	return &s
}

// NowTimestamp builds a pgtype.Timestamp set to the current time (UTC),
// for query params that need "now()" from the application layer.
func NowTimestamp() pgtype.Timestamp {
	return pgtype.Timestamp{Time: time.Now().UTC(), Valid: true}
}

// TimestampAt builds a pgtype.Timestamp for t, normalized to UTC. All
// timestamp columns are naive (`timestamp`, no time zone) and every DB-side
// default is now() under an UTC-configured server, so any wall clock built
// from a local time.Now() would compare against them shifted by the local
// offset — e.g. a cutoff that sweeps rows the instant they are written.
func TimestampAt(t time.Time) pgtype.Timestamp {
	return pgtype.Timestamp{Time: t.UTC(), Valid: true}
}

// TimestampFromPtr converts an optional ISO date string (as adapters/DTOs
// carry) into a pgtype.Timestamp; nil/empty/unparseable -> invalid (NULL).
func TimestampFromPtr(s *string) pgtype.Timestamp {
	if s == nil || *s == "" {
		return pgtype.Timestamp{}
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		// try date-only / other common layouts the adapters may hand back
		if t2, err2 := time.Parse("2006-01-02", *s); err2 == nil {
			return pgtype.Timestamp{Time: t2, Valid: true}
		}
		return pgtype.Timestamp{}
	}
	return pgtype.Timestamp{Time: t.UTC(), Valid: true}
}

// UnmarshalJSONB decodes a jsonb column ([]byte) into dst; treats
// empty/nil/"null" as a no-op leaving dst at its zero value, matching how
// nullable jsonb columns (matchedSkills, missingSkills, ...) decode.
func UnmarshalJSONB(raw []byte, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	if string(raw) == "null" {
		return nil
	}
	return json.Unmarshal(raw, dst)
}

// MarshalJSONB encodes v to JSON for a jsonb column; a nil v encodes to "null".
func MarshalJSONB(v any) ([]byte, error) {
	return json.Marshal(v)
}
