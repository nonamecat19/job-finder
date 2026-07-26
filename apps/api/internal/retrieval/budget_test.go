package retrieval

import (
	"testing"
)

func TestParseCrawlDelay(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int32
	}{
		{
			name: "standard",
			body: "User-agent: *\nCrawl-delay: 5\nDisallow: /admin",
			want: 5,
		},
		{
			name: "case insensitive",
			body: "CRAWL-DELAY: 10",
			want: 10,
		},
		{
			name: "mixed case",
			body: "Crawl-Delay: 3",
			want: 3,
		},
		{
			name: "no delay",
			body: "User-agent: *\nDisallow: /admin",
			want: 0,
		},
		{
			name: "zero delay",
			body: "Crawl-delay: 0",
			want: 0,
		},
		{
			name: "negative delay",
			body: "Crawl-delay: -5",
			want: 0,
		},
		{
			name: "empty body",
			body: "",
			want: 0,
		},
		{
			name: "surrounded by whitespace",
			body: "Crawl-delay:  15  ",
			want: 15,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCrawlDelay(tt.body)
			if got != tt.want {
				t.Errorf("parseCrawlDelay(%q) = %d, want %d", tt.body, got, tt.want)
			}
		})
	}
}

func TestCrawlDelayRe(t *testing.T) {
	tests := []struct {
		body string
		hit  bool
	}{
		{"Crawl-delay: 5", true},
		{"crawl-delay: 10", true},
		{"CRAWL-DELAY: 3", true},
		{"Crawl-delay: 0", true},
		{"User-agent: *\nCrawl-delay: 7\n", true},
		{"no match here", false},
		{"Delay: 5", false},
		{"Crawl-delay:", false},
	}

	for _, tt := range tests {
		hit := crawlDelayRe.MatchString(tt.body)
		if hit != tt.hit {
			t.Errorf("crawlDelayRe.MatchString(%q) = %v, want %v", tt.body, hit, tt.hit)
		}
	}
}

func TestBudgetExhaustionLogic(t *testing.T) {
	tests := []struct {
		name       string
		budgetUsed int32
		budgetLimit int32
		count      int32
		wantAllowed bool
		wantRemaining int
	}{
		{
			name:         "under budget",
			budgetUsed:   10,
			budgetLimit:  200,
			count:        5,
			wantAllowed:  true,
			wantRemaining: 185,
		},
		{
			name:         "at budget boundary",
			budgetUsed:   195,
			budgetLimit:  200,
			count:        5,
			wantAllowed:  true,
			wantRemaining: 0,
		},
		{
			name:         "over budget",
			budgetUsed:   199,
			budgetLimit:  200,
			count:        5,
			wantAllowed:  false,
			wantRemaining: 1, // remaining = limit - used = 1
		},
		{
			name:         "exactly at limit",
			budgetUsed:   200,
			budgetLimit:  200,
			count:        1,
			wantAllowed:  false,
			wantRemaining: 0,
		},
		{
			name:         "count exceeds limit",
			budgetUsed:   0,
			budgetLimit:  5,
			count:        10,
			wantAllowed:  false,
			wantRemaining: 5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, remaining := checkBudgetLogic(tt.budgetUsed, tt.budgetLimit, tt.count)
			if allowed != tt.wantAllowed {
				t.Errorf("allowed = %v, want %v", allowed, tt.wantAllowed)
			}
			if remaining != tt.wantRemaining {
				t.Errorf("remaining = %d, want %d", remaining, tt.wantRemaining)
			}
		})
	}
}

func checkBudgetLogic(used, limit, count int32) (allowed bool, remaining int) {
	total := int(used) + int(count)
	if total > int(limit) {
		return false, int(limit) - int(used)
	}
	return true, int(limit) - total
}
