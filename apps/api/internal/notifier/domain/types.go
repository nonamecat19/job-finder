package domain

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// FreshWindow is how recently a job must have been posted to be flagged
// "fresh" on a notification.
const FreshWindow = 24 * time.Hour

// IsFresh reports whether postedAt falls within FreshWindow of now.
func IsFresh(postedAt pgtype.Timestamp) bool {
	if !postedAt.Valid {
		return false
	}
	return time.Since(postedAt.Time) < FreshWindow
}
