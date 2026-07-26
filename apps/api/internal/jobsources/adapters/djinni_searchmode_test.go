package adapters

import (
	"testing"
)

func TestDjinniDetect(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want DjinniSearchMode
	}{
		{
			name: "dashboard URL",
			url:  "https://djinni.co/my/dashboard/subs/123/",
			want: DjinniModeDashboard,
		},
		{
			name: "dashboard URL without trailing slash",
			url:  "https://djinni.co/my/dashboard/subs/123",
			want: DjinniModeDashboard,
		},
		{
			name: "basic-search URL with all filters",
			url:  "https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Node.js&salary=3000&exp_level=2y&exp_level=3y&exp_level=4y&exp_level=5y&employment=remote",
			want: DjinniModeBasicSearch,
		},
		{
			name: "basic-search URL with only search_type+primary_keyword",
			url:  "https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang",
			want: DjinniModeBasicSearch,
		},
		{
			name: "basic-search URL missing search_type — NOT BasicSearch",
			url:  "https://djinni.co/jobs/?primary_keyword=Golang",
			want: DjinniModeUnknown,
		},
		{
			name: "single job-posting path without search_type — Unknown",
			url:  "https://djinni.co/jobs/12345",
			want: DjinniModeUnknown,
		},
		{
			name: "non-djinni.co host — Unknown",
			url:  "https://example.com/jobs/",
			want: DjinniModeUnknown,
		},
		{
			name: "www.djinni.co host with basic-search",
			url:  "https://www.djinni.co/jobs/?search_type=basic-search&primary_keyword=React",
			want: DjinniModeBasicSearch,
		},
		{
			name: "www.djinni.co dashboard",
			url:  "https://www.djinni.co/my/dashboard/subs/42/",
			want: DjinniModeDashboard,
		},
		{
			name: "/companies/ path — Unknown",
			url:  "https://djinni.co/companies/some-company",
			want: DjinniModeUnknown,
		},
		{
			name: "empty URL — Unknown",
			url:  "",
			want: DjinniModeUnknown,
		},
		{
			name: "basic-search trailing slash optional",
			url:  "https://djinni.co/jobs?search_type=basic-search&primary_keyword=Java",
			want: DjinniModeBasicSearch,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DjinniDetect(tt.url)
			if got != tt.want {
				t.Errorf("DjinniDetect(%q) = %d, want %d", tt.url, got, tt.want)
			}
		})
	}
}

func TestParseBasicSearch(t *testing.T) {
	t.Run("not basic-search returns false", func(t *testing.T) {
		_, ok := ParseBasicSearch("https://djinni.co/my/dashboard/subs/123/")
		if ok {
			t.Error("expected false for dashboard URL")
		}
	})

	t.Run("full filter set", func(t *testing.T) {
		f, ok := ParseBasicSearch("https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Node.js&salary=3000&exp_level=2y&exp_level=3y&exp_level=4y&exp_level=5y&employment=remote")
		if !ok {
			t.Fatal("expected true for basic-search URL")
		}
		if f.PrimaryKeyword != "Node.js" {
			t.Errorf("PrimaryKeyword = %q, want %q", f.PrimaryKeyword, "Node.js")
		}
		if f.Salary != "3000" {
			t.Errorf("Salary = %q, want %q", f.Salary, "3000")
		}
		if len(f.ExpLevels) != 4 {
			t.Errorf("ExpLevels length = %d, want 4", len(f.ExpLevels))
		}
		if f.Employment != "remote" {
			t.Errorf("Employment = %q, want %q", f.Employment, "remote")
		}
	})

	t.Run("duplicate exp_level values", func(t *testing.T) {
		f, ok := ParseBasicSearch("https://djinni.co/jobs/?search_type=basic-search&exp_level=2y&exp_level=2y&exp_level=3y")
		if !ok {
			t.Fatal("expected true")
		}
		if len(f.ExpLevels) != 3 {
			t.Errorf("ExpLevels length = %d, want 3 (preserves duplicates)", len(f.ExpLevels))
		}
	})

	t.Run("out-of-order exp_level", func(t *testing.T) {
		f, ok := ParseBasicSearch("https://djinni.co/jobs/?search_type=basic-search&exp_level=3y&exp_level=1y&exp_level=2y")
		if !ok {
			t.Fatal("expected true")
		}
		if f.ExpLevels[0] != "3y" || f.ExpLevels[1] != "1y" || f.ExpLevels[2] != "2y" {
			t.Error("exp_level order preserved as-received")
		}
	})

	t.Run("missing salary and employment", func(t *testing.T) {
		f, ok := ParseBasicSearch("https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang")
		if !ok {
			t.Fatal("expected true")
		}
		if f.Salary != "" {
			t.Errorf("Salary = %q, want empty", f.Salary)
		}
		if f.Employment != "" {
			t.Errorf("Employment = %q, want empty", f.Employment)
		}
	})

	t.Run("unrecognized extra query parameter preserved not error", func(t *testing.T) {
		_, ok := ParseBasicSearch("https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Go&extra_param=value")
		if !ok {
			t.Fatal("unrecognized param should not cause false")
		}
	})
}

func TestSummarizeExpLevels(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{name: "consecutive 2y-5y", values: []string{"2y", "3y", "4y", "5y"}, want: "2\u20135 years"},
		{name: "consecutive 1y-3y", values: []string{"1y", "2y", "3y"}, want: "1\u20133 years"},
		{name: "non-consecutive 1y,3y", values: []string{"1y", "3y"}, want: "1, 3 years"},
		{name: "duplicate 2y,2y", values: []string{"2y", "2y"}, want: "2 years"},
		{name: "out-of-order 3y,1y,2y", values: []string{"3y", "1y", "2y"}, want: "1\u20133 years"},
		{name: "empty", values: nil, want: ""},
		{name: "single value", values: []string{"3y"}, want: "3 years"},
		{name: "non-integer senior forces list fallback", values: []string{"senior"}, want: "senior"},
		{name: "mixed parseable and non-parseable forces list fallback", values: []string{"2y", "lead"}, want: "2y, lead"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarizeExpLevels(tt.values)
			if got != tt.want {
				t.Errorf("summarizeExpLevels(%v) = %q, want %q", tt.values, got, tt.want)
			}
		})
	}
}
