package referral

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
)

// ErrContactNotFound is returned when a referenced contact does not exist.
var ErrContactNotFound = errors.New("referral: contact not found")

// ErrNoGithubUsername is returned when GitHub sync is requested for a
// contact that has no githubUsername on file.
var ErrNoGithubUsername = errors.New("referral: contact has no github username")

// JobRepository is the slice of job persistence the referral service needs
// to resolve a job's company for a warm-path lookup.
type JobRepository interface {
	GetJobByID(ctx context.Context, id pgtype.UUID) (sqlcgen.Job, error)
}

// ImportSummary reports the outcome of a CSV contact import.
type ImportSummary struct {
	Imported int
	Skipped  int
	Total    int
}

// GithubSyncResult reports the outcome of cross-referencing one contact's
// GitHub followers/following against the existing contact book.
type GithubSyncResult struct {
	Contact          Contact
	FollowersScanned int
	FollowingScanned int
	ConnectionsMade  int
}

// Service orchestrates CSV import, GitHub cross-referencing, and warm-path
// finding/ranking over the Contact/ContactConnection graph.
type Service struct {
	repo   Repository
	jobs   JobRepository
	gh     *GitHubCrossReferencer
	finder *PathFinder
	ranker *Ranker
}

func NewService(repo Repository, jobs JobRepository, gh *GitHubCrossReferencer) *Service {
	return &Service{
		repo:   repo,
		jobs:   jobs,
		gh:     gh,
		finder: NewPathFinder(repo),
		ranker: NewRanker(),
	}
}

// ImportCSV parses a contacts CSV and inserts one Contact row per record
// that has a non-empty name. Rows with an empty name are silently skipped
// (ParseCSV already drops them), so Skipped only reflects rows that failed
// to insert (e.g. a transient DB error) rather than blank rows.
func (s *Service) ImportCSV(ctx context.Context, r io.Reader) (ImportSummary, error) {
	rows, err := ParseCSV(r)
	if err != nil {
		return ImportSummary{}, err
	}

	summary := ImportSummary{Total: len(rows)}
	for _, row := range rows {
		_, err := s.repo.InsertContact(ctx, sqlcgen.InsertContactParams{
			Name:           row.Name,
			Email:          strPtrOrNil(row.Email),
			Company:        strPtrOrNil(row.Company),
			Role:           strPtrOrNil(row.Role),
			LinkedinUrl:    strPtrOrNil(row.LinkedInURL),
			GithubUsername: strPtrOrNil(row.GitHubUsername),
			Source:         "csv",
		})
		if err != nil {
			summary.Skipped++
			continue
		}
		summary.Imported++
	}
	return summary, nil
}

// SyncGithub fetches the given contact's GitHub followers and following,
// then cross-references each returned login against the existing contact
// book (by githubUsername). Every match becomes (or strengthens) a
// ContactConnection between the two contacts — the "GitHub cross-reference"
// step that turns two independently-imported contacts into a warm-path edge.
func (s *Service) SyncGithub(ctx context.Context, contactID string) (*GithubSyncResult, error) {
	uid, err := dbutil.ParseUUID(contactID)
	if err != nil {
		return nil, err
	}
	contact, err := s.repo.GetContactByID(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrContactNotFound, contactID)
	}
	if contact.GithubUsername == nil || strings.TrimSpace(*contact.GithubUsername) == "" {
		return nil, ErrNoGithubUsername
	}
	username := strings.TrimSpace(*contact.GithubUsername)

	followers, err := s.gh.FetchFollowers(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("referral: fetch followers for %s: %w", username, err)
	}
	following, err := s.gh.FetchFollowing(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("referral: fetch following for %s: %w", username, err)
	}

	result := &GithubSyncResult{
		Contact:          sqlcContactToDomain(contact),
		FollowersScanned: len(followers),
		FollowingScanned: len(following),
	}

	made := make(map[string]bool)
	link := func(login string, strength float32) {
		matches, err := s.repo.FindContactsByGithubUsername(ctx, &login)
		if err != nil || len(matches) == 0 {
			return
		}
		for _, match := range matches {
			if match.ID == contact.ID {
				continue
			}
			key := dbutil.UUIDString(match.ID)
			if made[key] {
				continue
			}
			if _, err := s.repo.InsertContactConnection(ctx, sqlcgen.InsertContactConnectionParams{
				FromContactId:    contact.ID,
				ToContactId:      match.ID,
				RelationshipType: "github",
				Strength:         strength,
			}); err == nil {
				made[key] = true
				result.ConnectionsMade++
			}
		}
	}

	followingSet := make(map[string]bool, len(following))
	for _, p := range following {
		followingSet[strings.ToLower(p.Login)] = true
	}

	for _, p := range followers {
		// Mutual (follows each other) is a stronger tie than one-directional.
		strength := float32(0.5)
		if followingSet[strings.ToLower(p.Login)] {
			strength = 0.8
		}
		link(p.Login, strength)
	}
	for _, p := range following {
		if strings.EqualFold(p.Login, username) {
			continue
		}
		link(p.Login, 0.5)
	}

	return result, nil
}

// ListContacts returns every contact in the book (CSV-imported or GitHub-
// discovered), most-recently-added first — the source list for the
// contacts UI and for picking a contact to run GitHub sync against.
func (s *Service) ListContacts(ctx context.Context) ([]Contact, error) {
	rows, err := s.repo.ListContacts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Contact, 0, len(rows))
	for _, r := range rows {
		out = append(out, sqlcContactToDomain(r))
	}
	return out, nil
}

// FindReferralPaths resolves the job's company, walks the contact graph for
// warm paths into it, and returns the top-N paths ranked by strength.
func (s *Service) FindReferralPaths(ctx context.Context, jobID string, maxDepth, topN int) ([]ReferralPath, error) {
	uid, err := dbutil.ParseUUID(jobID)
	if err != nil {
		return nil, err
	}
	job, err := s.jobs.GetJobByID(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("referral: job %s not found: %w", jobID, err)
	}
	if strings.TrimSpace(job.Company) == "" {
		return nil, nil
	}

	paths, err := s.finder.FindPathsToCompany(ctx, job.Company, maxDepth)
	if err != nil {
		return nil, err
	}
	if topN > 0 {
		return s.ranker.TopN(paths, topN), nil
	}
	return s.ranker.Rank(paths), nil
}

func strPtrOrNil(s string) *string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return &s
}
