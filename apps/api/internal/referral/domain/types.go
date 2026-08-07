package domain

import "time"

type Contact struct {
	ID             string
	Name           string
	Email          *string
	Company        *string
	Role           *string
	LinkedInURL    *string
	GitHubUsername *string
	Source         string
	CreatedAt      time.Time
}

type ContactConnection struct {
	ID               string
	FromContactID    string
	ToContactID      string
	RelationshipType string
	Strength         float64
	CreatedAt        time.Time
}

type ReferralPath struct {
	Path   []Contact
	Score  float64
	Length int
}

type ImportSummary struct {
	Imported int
	Skipped  int
	Total    int
}

type GithubSyncResult struct {
	Contact          Contact
	FollowersScanned int
	FollowingScanned int
	ConnectionsMade  int
}
