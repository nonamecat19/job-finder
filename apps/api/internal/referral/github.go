package referral

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type GitHubProfile struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	Company   string `json:"company"`
	Email     string `json:"email"`
	Bio       string `json:"bio"`
	Followers int    `json:"followers"`
	Following int    `json:"following"`
}

type GitHubCrossReferencer struct {
	client  *http.Client
	baseURL string
}

func NewGitHubCrossReferencer() *GitHubCrossReferencer {
	return &GitHubCrossReferencer{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		baseURL: "https://api.github.com",
	}
}

func (g *GitHubCrossReferencer) FetchProfile(ctx context.Context, username string) (*GitHubProfile, error) {
	url := fmt.Sprintf("%s/users/%s", g.baseURL, username)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("referral: github request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "job-finder/1.0")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("referral: github fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("referral: github status %d", resp.StatusCode)
	}

	var profile GitHubProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return nil, fmt.Errorf("referral: github decode: %w", err)
	}
	return &profile, nil
}

func (g *GitHubCrossReferencer) FetchFollowers(ctx context.Context, username string) ([]GitHubProfile, error) {
	url := fmt.Sprintf("%s/users/%s/followers?per_page=100", g.baseURL, username)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("referral: github followers request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "job-finder/1.0")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("referral: github followers fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("referral: github followers status %d", resp.StatusCode)
	}

	var followers []GitHubProfile
	if err := json.NewDecoder(resp.Body).Decode(&followers); err != nil {
		return nil, fmt.Errorf("referral: github followers decode: %w", err)
	}
	return followers, nil
}

func (g *GitHubCrossReferencer) FetchFollowing(ctx context.Context, username string) ([]GitHubProfile, error) {
	url := fmt.Sprintf("%s/users/%s/following?per_page=100", g.baseURL, username)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("referral: github following request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "job-finder/1.0")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("referral: github following fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("referral: github following status %d", resp.StatusCode)
	}

	var following []GitHubProfile
	if err := json.NewDecoder(resp.Body).Decode(&following); err != nil {
		return nil, fmt.Errorf("referral: github following decode: %w", err)
	}
	return following, nil
}
