package domain_test

import (
	"testing"

	"github.com/job-finder/api/internal/keyword/domain"
)

func TestSelectStories(t *testing.T) {
	question := domain.InterviewQuestion{
		Text:          "The role requires Python. Can you describe your experience with Python?",
		Category:      domain.CategoryTechnical,
		Source:        domain.SourceRequiredSkill,
		SourceExcerpt: "Strong Python skills",
	}

	matchingStory := domain.StarStory{
		ID:         "story-1",
		Title:      "Built data pipeline with Python",
		Situation:  "Our data team needed a real-time ETL pipeline to process 10M events daily.",
		Task:       "Design and build the pipeline using Python and Apache Kafka.",
		Action:     "Wrote Python microservices with asyncio and deployed on Kubernetes.",
		Result:     "Reduced data latency from 2 hours to under 30 seconds.",
		Skills:     []string{"Python", "Kafka", "Kubernetes"},
		Categories: []domain.StoryCategory{domain.StoryTechnical},
	}

	unrelatedStory := domain.StarStory{
		ID:        "story-2",
		Title:     "Resolved team conflict",
		Situation: "Two senior engineers disagreed on the architecture approach.",
		Task:      "Facilitate a decision and restore team alignment.",
		Action:    "Organized a design review with benchmarks and consensus-building.",
		Result:    "Team agreed on a hybrid approach, shipped on schedule.",
		Skills:    []string{"Communication", "Mediation"},
		Categories: []domain.StoryCategory{
			domain.StoryLeadership,
			domain.StoryConflictResolution,
		},
	}

	partialMatchStory := domain.StarStory{
		ID:         "story-3",
		Title:      "Containerized microservices with Docker",
		Situation:  "Monolithic app needed to be broken into microservices.",
		Task:       "Containerize and orchestrate the microservices.",
		Action:     "Created Dockerfiles and Kubernetes manifests.",
		Result:     "Deployment time reduced from 30m to 5m.",
		Skills:     []string{"Docker", "Kubernetes"},
		Categories: []domain.StoryCategory{domain.StoryTechnical},
	}

	t.Run("best story ranked first by skill overlap", func(t *testing.T) {
		stories := []domain.StarStory{matchingStory, partialMatchStory, unrelatedStory}
		result := domain.SelectStories([]domain.InterviewQuestion{question}, stories, nil)

		if len(result.MappedQuestions) != 1 {
			t.Fatalf("got %d mapped questions, want 1", len(result.MappedQuestions))
		}
		mq := result.MappedQuestions[0]
		if !mq.IsCovered {
			t.Fatal("expected question to be covered")
		}
		if len(mq.MappedStories) == 0 {
			t.Fatal("expected at least one mapped story")
		}
		if mq.MappedStories[0].StoryID != "story-1" {
			t.Errorf("top story = %q, want %q", mq.MappedStories[0].StoryID, "story-1")
		}
		if mq.MappedStories[0].RelevanceScore <= 0 {
			t.Errorf("top story score = %f, want > 0", mq.MappedStories[0].RelevanceScore)
		}
		if result.UncoveredCount != 0 {
			t.Errorf("uncovered count = %d, want 0", result.UncoveredCount)
		}
	})

	t.Run("cap at 3 stories per question", func(t *testing.T) {
		var manyStories []domain.StarStory
		for i := 0; i < 5; i++ {
			manyStories = append(manyStories, domain.StarStory{
				ID:         "many-py",
				Title:      "Python story",
				Situation:  "Something about Python development.",
				Task:       "Write Python code.",
				Action:     "Wrote Python code.",
				Result:     "Shipped feature.",
				Skills:     []string{"Python"},
				Categories: []domain.StoryCategory{domain.StoryTechnical},
			})
		}
		result := domain.SelectStories([]domain.InterviewQuestion{question}, manyStories, nil)
		mq := result.MappedQuestions[0]
		if len(mq.MappedStories) > 3 {
			t.Errorf("got %d stories, want cap of 3", len(mq.MappedStories))
		}
	})

	t.Run("empty stories yields uncovered", func(t *testing.T) {
		result := domain.SelectStories([]domain.InterviewQuestion{question}, nil, nil)
		if len(result.MappedQuestions) != 1 {
			t.Fatalf("got %d mapped questions, want 1", len(result.MappedQuestions))
		}
		if result.MappedQuestions[0].IsCovered {
			t.Fatal("expected question to be uncovered with no stories")
		}
		if result.UncoveredCount != 1 {
			t.Errorf("uncovered count = %d, want 1", result.UncoveredCount)
		}
	})

	t.Run("empty questions returns empty result", func(t *testing.T) {
		result := domain.SelectStories(nil,
			[]domain.StarStory{matchingStory}, nil)
		if len(result.MappedQuestions) != 0 {
			t.Errorf("got %d questions, want 0", len(result.MappedQuestions))
		}
		if result.UncoveredCount != 0 {
			t.Errorf("uncovered count = %d, want 0", result.UncoveredCount)
		}
	})

	t.Run("custom embedding function affects scoring", func(t *testing.T) {
		embedFn := func(qText string, s domain.StarStory) float64 {
			if s.ID == "story-1" {
				return 0.9
			}
			return 0.1
		}
		stories := []domain.StarStory{matchingStory, unrelatedStory}
		result := domain.SelectStories([]domain.InterviewQuestion{question}, stories, embedFn)
		mq := result.MappedQuestions[0]
		if mq.MappedStories[0].StoryID != "story-1" {
			t.Errorf("top story = %q, want %q", mq.MappedStories[0].StoryID, "story-1")
		}
	})

	t.Run("behavioral question matches non-technical story categories", func(t *testing.T) {
		bq := domain.InterviewQuestion{
			Text:          "Tell me about a time you had to lead a team through a difficult project.",
			Category:      domain.CategoryBehavioral,
			Source:        domain.SourceResponsibility,
			SourceExcerpt: "Lead cross-functional teams",
		}
		stories := []domain.StarStory{
			matchingStory,
			unrelatedStory,
		}
		result := domain.SelectStories([]domain.InterviewQuestion{bq}, stories, nil)
		mq := result.MappedQuestions[0]
		if !mq.IsCovered {
			t.Fatal("expected behavioral question to be covered by leadership story")
		}
		if len(mq.MappedStories) != 1 {
			t.Fatalf("got %d stories, want 1", len(mq.MappedStories))
		}
		if mq.MappedStories[0].StoryID != "story-2" {
			t.Errorf("top story = %q, want %q", mq.MappedStories[0].StoryID, "story-2")
		}
	})

	t.Run("score below threshold yields uncovered", func(t *testing.T) {
		veryWeakMatch := domain.StarStory{
			ID:         "weak",
			Title:      "Team collaboration",
			Situation:  "A story about teamwork.",
			Task:       "Nothing related.",
			Action:     "Nothing related.",
			Result:     "Nothing related.",
			Skills:     []string{"Communication"},
			Categories: []domain.StoryCategory{domain.StoryTeamwork},
		}
		stories := []domain.StarStory{veryWeakMatch}
		result := domain.SelectStories([]domain.InterviewQuestion{question}, stories, nil)
		mq := result.MappedQuestions[0]
		if mq.IsCovered {
			t.Fatal("expected question to be uncovered when score below threshold")
		}
	})
}

func TestSelectStoriesUncoveredQuestion(t *testing.T) {
	behavioralQuestion := domain.InterviewQuestion{
		Text:          "Tell me about a time you had to mentor a junior engineer.",
		Category:      domain.CategoryBehavioral,
		Source:        domain.SourceResponsibility,
		SourceExcerpt: "Mentor junior engineers",
	}

	coveredTechnical := domain.InterviewQuestion{
		Text:          "The role requires Python. Can you describe your experience with Python?",
		Category:      domain.CategoryTechnical,
		Source:        domain.SourceRequiredSkill,
		SourceExcerpt: "Python required",
	}

	uncoveredTechnical := domain.InterviewQuestion{
		Text:          "The role requires Rust. Can you describe your experience with Rust?",
		Category:      domain.CategoryTechnical,
		Source:        domain.SourceRequiredSkill,
		SourceExcerpt: "Rust required",
	}

	uncoveredBehavioral := domain.InterviewQuestion{
		Text:          "Tell me about a time you had to resolve a conflict.",
		Category:      domain.CategoryBehavioral,
		Source:        domain.SourceResponsibility,
		SourceExcerpt: "Resolve conflicts",
	}

	technicalStory := domain.StarStory{
		ID:         "py-story",
		Title:      "Built Python ETL pipeline",
		Situation:  "Data team needed automation.",
		Task:       "Build the pipeline.",
		Action:     "Wrote Python ETL scripts.",
		Result:     "Automated data processing.",
		Skills:     []string{"Python"},
		Categories: []domain.StoryCategory{domain.StoryTechnical},
	}

	mentorStory := domain.StarStory{
		ID:         "mentor-story",
		Title:      "Mentored junior engineer",
		Situation:  "Junior dev was struggling with onboarding.",
		Task:       "Help them ramp up.",
		Action:     "Set up pair programming and weekly 1:1s.",
		Result:     "Engineer was productive within 6 weeks.",
		Skills:     []string{"Mentoring"},
		Categories: []domain.StoryCategory{domain.StoryMentoring},
	}

	t.Run("behavioral question uncovered with only technical stories", func(t *testing.T) {
		result := domain.SelectStories(
			[]domain.InterviewQuestion{behavioralQuestion},
			[]domain.StarStory{technicalStory},
			nil,
		)
		if len(result.MappedQuestions) != 1 {
			t.Fatalf("got %d questions, want 1", len(result.MappedQuestions))
		}
		if result.MappedQuestions[0].IsCovered {
			t.Fatal("expected behavioral question to be uncovered with only technical stories")
		}
		if result.UncoveredCount != 1 {
			t.Errorf("uncovered count = %d, want 1", result.UncoveredCount)
		}
	})

	t.Run("technical question uncovered with only behavioral stories", func(t *testing.T) {
		result := domain.SelectStories(
			[]domain.InterviewQuestion{uncoveredTechnical},
			[]domain.StarStory{mentorStory},
			nil,
		)
		if result.MappedQuestions[0].IsCovered {
			t.Fatal("expected technical question to be uncovered with only behavioral stories")
		}
		if result.UncoveredCount != 1 {
			t.Errorf("uncovered count = %d, want 1", result.UncoveredCount)
		}
	})

	t.Run("mixed covered and uncovered", func(t *testing.T) {
		result := domain.SelectStories(
			[]domain.InterviewQuestion{coveredTechnical, uncoveredBehavioral},
			[]domain.StarStory{technicalStory},
			nil,
		)
		if result.UncoveredCount != 1 {
			t.Fatalf("uncovered count = %d, want 1", result.UncoveredCount)
		}
		if !result.MappedQuestions[0].IsCovered {
			t.Error("question 0 (Python technical) should be covered")
		}
		if result.MappedQuestions[1].IsCovered {
			t.Error("question 1 (conflict behavioral) should be uncovered")
		}
	})
}

func TestJaccardSimilarity(t *testing.T) {
	q := domain.InterviewQuestion{
		Text:     "The role requires Python and Docker.",
		Category: domain.CategoryTechnical,
		Source:   domain.SourceRequiredSkill,
	}

	highMatch := domain.StarStory{
		ID:         "hi",
		Title:      "High match",
		Situation:  "Story with Python and Docker.",
		Task:       "Task",
		Action:     "Action",
		Result:     "Result",
		Skills:     []string{"Python", "Docker"},
		Categories: []domain.StoryCategory{domain.StoryTechnical},
	}

	lowMatch := domain.StarStory{
		ID:         "lo",
		Title:      "Low match",
		Situation:  "Story with only one matching skill.",
		Task:       "Task",
		Action:     "Action",
		Result:     "Result",
		Skills:     []string{"Python", "Rust", "Go"},
		Categories: []domain.StoryCategory{domain.StoryTechnical},
	}

	noMatch := domain.StarStory{
		ID:         "no",
		Title:      "No match",
		Situation:  "Story with no matching skills.",
		Task:       "Task",
		Action:     "Action",
		Result:     "Result",
		Skills:     []string{"React", "TypeScript"},
		Categories: []domain.StoryCategory{domain.StoryTechnical},
	}

	result := domain.SelectStories([]domain.InterviewQuestion{q},
		[]domain.StarStory{noMatch, lowMatch, highMatch}, nil)

	mq := result.MappedQuestions[0]
	if !mq.IsCovered {
		t.Fatal("expected coverage")
	}
	if len(mq.MappedStories) != 3 {
		t.Fatalf("got %d stories, want 3", len(mq.MappedStories))
	}
	if mq.MappedStories[0].StoryID != "hi" {
		t.Errorf("rank 1 = %q, want %q (Jaccard: hi=2/2, lo=1/4, no=0/5)",
			mq.MappedStories[0].StoryID, "hi")
	}
	if mq.MappedStories[1].StoryID != "lo" {
		t.Errorf("rank 2 = %q, want %q", mq.MappedStories[1].StoryID, "lo")
	}
	if mq.MappedStories[2].StoryID != "no" {
		t.Errorf("rank 3 = %q, want %q", mq.MappedStories[2].StoryID, "no")
	}

	if mq.MappedStories[0].StoryID != "hi" {
		t.Errorf("rank 1 = %q, want %q", mq.MappedStories[0].StoryID, "hi")
	}
	if mq.MappedStories[1].StoryID != "lo" {
		t.Errorf("rank 2 = %q, want %q", mq.MappedStories[1].StoryID, "lo")
	}
	if mq.MappedStories[2].StoryID != "no" {
		t.Errorf("rank 3 = %q, want %q", mq.MappedStories[2].StoryID, "no")
	}
	if mq.MappedStories[0].RelevanceScore <= mq.MappedStories[1].RelevanceScore {
		t.Error("rank 1 score should be > rank 2 score")
	}
	if mq.MappedStories[1].RelevanceScore <= mq.MappedStories[2].RelevanceScore {
		t.Error("rank 2 score should be > rank 3 score")
	}
}

func TestStoryExcerpt(t *testing.T) {
	story := domain.StarStory{
		ID:         "long-story",
		Title:      "Long story",
		Situation:  "This is a very long situation string that exceeds the two hundred character excerpt limit and should be truncated to show only the first part with an ellipsis character appended at the truncation point. The remaining content of the situation field is not included in the excerpt.",
		Task:       "Task",
		Action:     "Action",
		Result:     "Result",
		Skills:     []string{"Python"},
		Categories: []domain.StoryCategory{domain.StoryTechnical},
	}
	q := domain.InterviewQuestion{
		Text:     "The role requires Python.",
		Category: domain.CategoryTechnical,
		Source:   domain.SourceRequiredSkill,
	}

	result := domain.SelectStories([]domain.InterviewQuestion{q}, []domain.StarStory{story}, nil)
	excerpt := result.MappedQuestions[0].MappedStories[0].Excerpt
	if len([]rune(excerpt)) > 201 {
		t.Errorf("excerpt length = %d, want <= 201", len([]rune(excerpt)))
	}
	if len(excerpt) >= len(story.Situation) {
		t.Errorf("excerpt length = %d, should be shorter than situation (%d)", len(excerpt), len(story.Situation))
	}
}
