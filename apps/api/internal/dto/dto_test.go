package dto

import (
	"testing"
)

func TestNormalizedJob(t *testing.T) {
	job := NormalizedJob{
		SourceKey:   "linkedin",
		ExternalID:  strPtr("12345"),
		Title:       "Go Developer",
		Company:     "Tech Corp",
		Location:    strPtr("Remote"),
		Remote:      true,
		SalaryRaw:   strPtr("$120k-$180k"),
		URL:         "https://example.com/job/12345",
		Description: "We are looking for a Go developer",
	}

	if job.Title != "Go Developer" {
		t.Errorf("expected title 'Go Developer', got %q", job.Title)
	}
	if job.Company != "Tech Corp" {
		t.Errorf("expected company 'Tech Corp', got %q", job.Company)
	}
	if job.SourceKey != "linkedin" {
		t.Errorf("expected source 'linkedin', got %q", job.SourceKey)
	}
	if job.ExternalID == nil || *job.ExternalID != "12345" {
		t.Errorf("expected external ID '12345'")
	}
	if !job.Remote {
		t.Error("expected remote to be true")
	}
}

func strPtr(s string) *string {
	return &s
}

func TestSearchQuery(t *testing.T) {
	query := SearchQuery{
		Keywords: "go developer",
		Location: strPtr("Remote"),
	}

	if query.Keywords != "go developer" {
		t.Errorf("expected keywords 'go developer', got %q", query.Keywords)
	}
	if query.Location == nil || *query.Location != "Remote" {
		t.Error("expected location 'Remote'")
	}
}

func TestApplicationStatus(t *testing.T) {
	tests := []struct {
		status ApplicationStatus
		valid  bool
	}{
		{StatusFound, true},
		{StatusShortlisted, true},
		{StatusDocsGenerated, true},
		{StatusApplied, true},
		{StatusInterview, true},
		{StatusOffer, true},
		{StatusRejected, true},
		{ApplicationStatus("invalid"), false},
		{ApplicationStatus(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			valid := IsValidApplicationStatus(string(tt.status))
			if valid != tt.valid {
				t.Errorf("for status %q, expected valid=%v, got %v", tt.status, tt.valid, valid)
			}
		})
	}
}

func TestIsValidApplicationStatus(t *testing.T) {
	validStatuses := []ApplicationStatus{
		StatusFound,
		StatusShortlisted,
		StatusDocsGenerated,
		StatusApplied,
		StatusInterview,
		StatusOffer,
		StatusRejected,
	}

	for _, status := range validStatuses {
		if !IsValidApplicationStatus(string(status)) {
			t.Errorf("expected status %q to be valid", status)
		}
	}

	invalidStatuses := []string{
		"invalid",
		"pending",
		"APPLIED",
		"",
	}

	for _, status := range invalidStatuses {
		if IsValidApplicationStatus(status) {
			t.Errorf("expected status %q to be invalid", status)
		}
	}
}

func TestNormalizedJobWithNilOptionalFields(t *testing.T) {
	job := NormalizedJob{
		Title:   "Developer",
		Company: "Company",
	}

	if job.ExternalID != nil {
		t.Error("expected nil external ID")
	}
	if job.Location != nil {
		t.Error("expected nil location")
	}
	if job.SalaryRaw != nil {
		t.Error("expected nil salary")
	}
	if job.PostedAt != nil {
		t.Error("expected nil posted at")
	}
}

func TestNormalizedJobWithAllFields(t *testing.T) {
	job := NormalizedJob{
		SourceKey:   "linkedin",
		ExternalID:  strPtr("12345"),
		Title:       "Senior Go Developer",
		Company:     "Tech Corp",
		Location:    strPtr("Remote"),
		Remote:      true,
		SalaryRaw:   strPtr("$150k-$200k"),
		URL:         "https://example.com/job/12345",
		Description: "We are looking for a senior Go developer",
		PostedAt:    strPtr("2024-01-15"),
		Raw:         map[string]string{"source": "linkedin"},
	}

	if job.Title == "" || job.Company == "" {
		t.Error("expected all basic fields to be set")
	}
	if job.ExternalID == nil || job.Location == nil || job.SalaryRaw == nil {
		t.Error("expected all optional fields to be set")
	}
	if job.PostedAt == nil {
		t.Error("expected posted at to be set")
	}
}

func TestSourceKind(t *testing.T) {
	tests := []struct {
		kind SourceKind
		str  string
	}{
		{SourceKindAPI, "api"},
		{SourceKindScrape, "scrape"},
		{SourceKindSidecar, "sidecar"},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if string(tt.kind) != tt.str {
				t.Errorf("expected %q, got %q", tt.str, string(tt.kind))
			}
		})
	}
}

func TestDocumentType(t *testing.T) {
	tests := []struct {
		docType DocumentType
		str     string
	}{
		{DocumentTypeResume, "resume"},
		{DocumentTypeCoverLetter, "cover_letter"},
	}

	for _, tt := range tests {
		t.Run(tt.str, func(t *testing.T) {
			if string(tt.docType) != tt.str {
				t.Errorf("expected %q, got %q", tt.str, string(tt.docType))
			}
		})
	}
}

func TestSearchQueryDefaults(t *testing.T) {
	query := SearchQuery{}

	if query.Keywords != "" {
		t.Errorf("expected empty keywords, got %q", query.Keywords)
	}
	if query.Location != nil {
		t.Error("expected nil location")
	}
	if query.Remote != nil {
		t.Error("expected nil remote")
	}
	if query.SalaryMin != nil {
		t.Error("expected nil salary min")
	}
}

func TestJobListResponse(t *testing.T) {
	resp := JobListResponse{
		Items: []JobDto{
			{ID: "1", Title: "Job 1"},
			{ID: "2", Title: "Job 2"},
		},
		Total:    100,
		Page:     1,
		PageSize: 20,
	}

	if len(resp.Items) != 2 {
		t.Errorf("expected 2 items, got %d", len(resp.Items))
	}
	if resp.Total != 100 {
		t.Errorf("expected total 100, got %d", resp.Total)
	}
	if resp.Page != 1 {
		t.Errorf("expected page 1, got %d", resp.Page)
	}
	if resp.PageSize != 20 {
		t.Errorf("expected page size 20, got %d", resp.PageSize)
	}
}

func TestApplicationStatuses(t *testing.T) {
	expected := []ApplicationStatus{
		StatusFound,
		StatusShortlisted,
		StatusDocsGenerated,
		StatusApplied,
		StatusInterview,
		StatusOffer,
		StatusRejected,
	}

	if len(ApplicationStatuses) != len(expected) {
		t.Errorf("expected %d statuses, got %d", len(expected), len(ApplicationStatuses))
	}

	for i, status := range ApplicationStatuses {
		if status != expected[i] {
			t.Errorf("status %d: expected %q, got %q", i, expected[i], status)
		}
	}
}

func TestSearchQueryWithSite(t *testing.T) {
	query := SearchQuery{
		Keywords: "developer",
		Site:     strPtr("linkedin"),
	}

	if query.Site == nil || *query.Site != "linkedin" {
		t.Error("expected site 'linkedin'")
	}
}

func TestSearchQueryWithSubscriptionURL(t *testing.T) {
	query := SearchQuery{
		SubscriptionURL: "https://djinni.co/my/dashboard/subs/123/",
	}

	if query.SubscriptionURL != "https://djinni.co/my/dashboard/subs/123/" {
		t.Errorf("expected subscription URL, got %q", query.SubscriptionURL)
	}
}

func TestMatchResultDto(t *testing.T) {
	match := MatchResultDto{
		ID:         "match-1",
		JobID:      "job-1",
		Similarity: 0.85,
		Model:      "gpt-4",
		CreatedAt:  "2024-01-15T10:00:00Z",
	}

	if match.Similarity != 0.85 {
		t.Errorf("expected similarity 0.85, got %f", match.Similarity)
	}
}

func TestJobDto(t *testing.T) {
	job := JobDto{
		ID:       "job-1",
		Title:    "Go Developer",
		Company:  "Tech Corp",
		Status:   string(StatusFound),
		Remote:   true,
		SalaryRaw: strPtr("$120k"),
	}

	if job.Title != "Go Developer" {
		t.Errorf("expected title 'Go Developer', got %q", job.Title)
	}
	if job.Status != string(StatusFound) {
		t.Errorf("expected status 'found', got %q", job.Status)
	}
}

func TestApplicationDto(t *testing.T) {
	app := ApplicationDto{
		ID:     "app-1",
		JobID:  "job-1",
		Status: StatusApplied,
		Events: []ApplicationEvent{
			{Status: string(StatusFound), At: "2024-01-15T10:00:00Z"},
		},
	}

	if app.Status != StatusApplied {
		t.Errorf("expected status 'applied', got %q", app.Status)
	}
	if len(app.Events) != 1 {
		t.Errorf("expected 1 event, got %d", len(app.Events))
	}
}

func TestJsonResume(t *testing.T) {
	resume := JsonResume{
		Basics: &ResumeBasics{
			Name:  strPtr("John Doe"),
			Email: strPtr("john@example.com"),
		},
		Skills: []ResumeSkill{
			{Name: "Go", Level: strPtr("Expert")},
		},
	}

	if resume.Basics == nil || *resume.Basics.Name != "John Doe" {
		t.Error("expected name 'John Doe'")
	}
	if len(resume.Skills) != 1 {
		t.Error("expected 1 skill")
	}
}
