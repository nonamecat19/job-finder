package application

import (
	"context"
	"fmt"
	"math"

	"github.com/job-finder/api/internal/db/sqlcgen"
	"github.com/job-finder/api/internal/dbutil"
	"github.com/job-finder/api/internal/referral/domain"
)

type PathFinder struct {
	repo domain.Repository
}

func NewPathFinder(repo domain.Repository) *PathFinder {
	return &PathFinder{repo: repo}
}

type graphNode struct {
	contactID string
	company   string
}

func (pf *PathFinder) FindPathsToCompany(ctx context.Context, company string, maxDepth int) ([]domain.ReferralPath, error) {
	if maxDepth <= 0 {
		maxDepth = 3
	}

	allContacts, err := pf.repo.ListContacts(ctx)
	if err != nil {
		return nil, fmt.Errorf("referral: list contacts: %w", err)
	}

	allConnections, err := pf.repo.ListConnections(ctx)
	if err != nil {
		return nil, fmt.Errorf("referral: list connections: %w", err)
	}

	contactMap := make(map[string]sqlcgen.Contact, len(allContacts))
	for _, c := range allContacts {
		contactMap[dbutil.UUIDString(c.ID)] = c
	}

	adj := make(map[string][]string)
	for _, conn := range allConnections {
		from := dbutil.UUIDString(conn.FromContactId)
		to := dbutil.UUIDString(conn.ToContactId)
		adj[from] = append(adj[from], to)
		adj[to] = append(adj[to], from)
	}

	targetContacts, err := pf.repo.FindContactsAtCompany(ctx, &company)
	if err != nil {
		return nil, fmt.Errorf("referral: find contacts at company: %w", err)
	}

	targetIDs := make(map[string]bool)
	for _, tc := range targetContacts {
		targetIDs[dbutil.UUIDString(tc.ID)] = true
	}

	if len(targetIDs) == 0 {
		return nil, nil
	}

	var paths []domain.ReferralPath
	visited := make(map[string]bool)

	for startID := range contactMap {
		if visited[startID] {
			continue
		}
		if _, isTarget := targetIDs[startID]; isTarget {
			continue
		}

		for targetID := range targetIDs {
			path := pf.bfs(startID, targetID, adj, maxDepth)
			if path != nil {
				contactPath := make([]domain.Contact, 0, len(path))
				for _, id := range path {
					if c, ok := contactMap[id]; ok {
						contactPath = append(contactPath, sqlcContactToDomain(c))
					}
				}
				paths = append(paths, domain.ReferralPath{
					Path:   contactPath,
					Score:  scorePath(contactPath, allConnections, contactMap),
					Length: len(contactPath),
				})
			}
		}
	}

	return paths, nil
}

func (pf *PathFinder) bfs(start, target string, adj map[string][]string, maxDepth int) []string {
	if start == target {
		return []string{start}
	}

	type queueItem struct {
		node string
		path []string
	}

	queue := []queueItem{{node: start, path: []string{start}}}
	visited := map[string]bool{start: true}

	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]

		if len(item.path) > maxDepth {
			continue
		}

		for _, neighbor := range adj[item.node] {
			if neighbor == target {
				return append(item.path, neighbor)
			}
			if !visited[neighbor] {
				visited[neighbor] = true
				newPath := make([]string, len(item.path))
				copy(newPath, item.path)
				newPath = append(newPath, neighbor)
				queue = append(queue, queueItem{node: neighbor, path: newPath})
			}
		}
	}

	return nil
}

func scorePath(path []domain.Contact, connections []sqlcgen.ContactConnection, contactMap map[string]sqlcgen.Contact) float64 {
	if len(path) < 2 {
		return 0
	}

	totalStrength := 0.0
	edgeCount := 0

	connMap := make(map[string]float64)
	for _, conn := range connections {
		from := dbutil.UUIDString(conn.FromContactId)
		to := dbutil.UUIDString(conn.ToContactId)
		key := from + ":" + to
		connMap[key] = float64(conn.Strength)
		connMap[to+":"+from] = float64(conn.Strength)
	}

	for i := 0; i < len(path)-1; i++ {
		key := path[i].ID + ":" + path[i+1].ID
		if s, ok := connMap[key]; ok {
			totalStrength += s
			edgeCount++
		}
	}

	if edgeCount == 0 {
		return 0
	}

	avgStrength := totalStrength / float64(edgeCount)
	lengthPenalty := 1.0 / float64(len(path))
	return math.Round(avgStrength*lengthPenalty*100) / 100
}

func sqlcContactToDomain(c sqlcgen.Contact) domain.Contact {
	return domain.Contact{
		ID:             dbutil.UUIDString(c.ID),
		Name:           c.Name,
		Email:          c.Email,
		Company:        c.Company,
		Role:           c.Role,
		LinkedInURL:    c.LinkedinUrl,
		GitHubUsername: c.GithubUsername,
		Source:         c.Source,
		CreatedAt:      c.CreatedAt.Time,
	}
}
