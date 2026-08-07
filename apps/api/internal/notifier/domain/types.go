package domain

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

const FreshWindow = 24 * time.Hour

func IsFresh(postedAt pgtype.Timestamp) bool {
	if !postedAt.Valid {
		return false
	}
	return time.Since(postedAt.Time) < FreshWindow
}
