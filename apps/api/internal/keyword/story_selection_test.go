package keyword_test

import (
	"testing"

	"github.com/job-finder/api/internal/keyword"
)

func TestSelectStories(t *testing.T) {
	question := keyword.InterviewQuestion{
		Text:          "The role requires Python. Can you describe your experience with Python?",
		Category:      keyword.CategoryTechnical,
		Source:        keyword.SourceRequiredSkill,
		SourceExcerpt: "Strong Python skills",
	}

	matchingStory := keyword.StarStory{
		ID:         "story-1",
		Title:      "Built data pipeline with Python",
		Situation:  "Our data team needed a real-time ETL pipeline to process 10M events daily.",
		Task:       "Design and build the pipeline using Python and Apache Kafka.",
		Action:     "Wrote Python microservices with asyncio and deployed on Kubernetes.",
		Result:     "Reduced data latency from 2 hours to under 30 seconds.",
		Skills:     []string{"Python", "Kafka", "Kubernetes"},
		Categories: []keyword.StoryCategory{keyword.StoryTechnical},
	}

	unrelatedStory := keyword.StarStory{
		ID:        "story-2",
		Title:     "Resolved team conflict",
		Situation: "Two senior engineers disagreed on the architecture approach.",
		Task:      "Facilitate a decision and restore team alignment.",
		Action:    "Organized a design review with benchmarks and consensus-building.",
		Result:    "Team agreed on a hybrid approach, shipped on schedule.",
		Skills:    []string{"Communication", "Mediation"},
		Categories: []keyword.StoryCategory{
			keyword.StoryLeadership,
			keyword.StoryConflictResolution,
		},
	}

	partialMatchStory := keyword.StarStory{
		ID:         "story-3",
		Title:      "Containerized microservices with Docker",
		Situation:  "Monolithic app needed to be broken into microservices.",
		Task:       "Containerize and orchestrate the microservices.",
		Action:     "Created Dockerfiles and Kubernetes manifests.",
		Result:     "Deployment time reduced from 30m to 5m.",
		Skills:     []string{"Docker", "Kubernetes"},
		Categories: []keyword.StoryCategory{keyword.StoryTechnical},
	}

	t.Run("best story ranked first by skill overlap", func(t *testing.T) {
		stories := []keyword.StarStory{matchingStory, partialMatchStory, unrelatedStory}
		result := keyword.SelectStories([]keyword.InterviewQuestion{question}, stories, nil)

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
		// Add 5 stories that all match the same question
		var manyStories []keyword.StarStory
		for i := 0; i < 5; i++ {
			manyStories = append(manyStories, keyword.StarStory{
				ID:         "many-py",
				Title:      "Python story",
				Situation:  "Something about Python development.",
				Task:       "Write Python code.",
				Action:     "Wrote Python code.",
				Result:     "Shipped feature.",
				Skills:     []string{"Python"},
				Categories: []keyword.StoryCategory{keyword.StoryTechnical},
			})
		}
		result := keyword.SelectStories([]keyword.InterviewQuestion{question}, manyStories, nil)
		mq := result.MappedQuestions[0]
		if len(mq.MappedStories) > 3 {
			t.Errorf("got %d stories, want cap of 3", len(mq.MappedStories))
		}
	})

	t.Run("empty stories yields uncovered", func(t *testing.T) {
		result := keyword.SelectStories([]keyword.InterviewQuestion{question}, nil, nil)
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
		result := keyword.SelectStories(nil,
			[]keyword.StarStory{matchingStory}, nil)
		if len(result.MappedQuestions) != 0 {
			t.Errorf("got %d questions, want 0", len(result.MappedQuestions))
		}
		if result.UncoveredCount != 0 {
			t.Errorf("uncovered count = %d, want 0", result.UncoveredCount)
		}
	})

	t.Run("custom embedding function affects scoring", func(t *testing.T) {
		embedFn := func(qText string, s keyword.StarStory) float64 {
			if s.ID == "story-1" {
				return 0.9
			}
			return 0.1
		}
		stories := []keyword.StarStory{matchingStory, unrelatedStory}
		result := keyword.SelectStories([]keyword.InterviewQuestion{question}, stories, embedFn)
		mq := result.MappedQuestions[0]
		if mq.MappedStories[0].StoryID != "story-1" {
			t.Errorf("top story = %q, want %q", mq.MappedStories[0].StoryID, "story-1")
		}
	})

	t.Run("behavioral question matches non-technical story categories", func(t *testing.T) {
		bq := keyword.InterviewQuestion{
			Text:          "Tell me about a time you had to lead a team through a difficult project.",
			Category:      keyword.CategoryBehavioral,
			Source:        keyword.SourceResponsibility,
			SourceExcerpt: "Lead cross-functional teams",
		}
		stories := []keyword.StarStory{
			matchingStory,  // technical, should not match behavioral
			unrelatedStory, // leadership+conflict, should match behavioral
		}
		result := keyword.SelectStories([]keyword.InterviewQuestion{bq}, stories, nil)
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
		// Story with non-matching category (StoryTeamwork vs CategoryTechnical)
		// and no skill overlap → score = 0.0 < 0.15
		veryWeakMatch := keyword.StarStory{
			ID:         "weak",
			Title:      "Team collaboration",
			Situation:  "A story about teamwork.",
			Task:       "Nothing related.",
			Action:     "Nothing related.",
			Result:     "Nothing related.",
			Skills:     []string{"Communication"},
			Categories: []keyword.StoryCategory{keyword.StoryTeamwork},
		}
		stories := []keyword.StarStory{veryWeakMatch}
		result := keyword.SelectStories([]keyword.InterviewQuestion{question}, stories, nil)
		mq := result.MappedQuestions[0]
		if mq.IsCovered {
			t.Fatal("expected question to be uncovered when score below threshold")
		}
	})
}

func TestSelectStoriesUncoveredQuestion(t *testing.T) {
	behavioralQuestion := keyword.InterviewQuestion{
		Text:          "Tell me about a time you had to mentor a junior engineer.",
		Category:      keyword.CategoryBehavioral,
		Source:        keyword.SourceResponsibility,
		SourceExcerpt: "Mentor junior engineers",
	}

	coveredTechnical := keyword.InterviewQuestion{
		Text:          "The role requires Python. Can you describe your experience with Python?",
		Category:      keyword.CategoryTechnical,
		Source:        keyword.SourceRequiredSkill,
		SourceExcerpt: "Python required",
	}

	uncoveredTechnical := keyword.InterviewQuestion{
		Text:          "The role requires Rust. Can you describe your experience with Rust?",
		Category:      keyword.CategoryTechnical,
		Source:        keyword.SourceRequiredSkill,
		SourceExcerpt: "Rust required",
	}

	uncoveredBehavioral := keyword.InterviewQuestion{
		Text:          "Tell me about a time you had to resolve a conflict.",
		Category:      keyword.CategoryBehavioral,
		Source:        keyword.SourceResponsibility,
		SourceExcerpt: "Resolve conflicts",
	}

	technicalStory := keyword.StarStory{
		ID:         "py-story",
		Title:      "Built Python ETL pipeline",
		Situation:  "Data team needed automation.",
		Task:       "Build the pipeline.",
		Action:     "Wrote Python ETL scripts.",
		Result:     "Automated data processing.",
		Skills:     []string{"Python"},
		Categories: []keyword.StoryCategory{keyword.StoryTechnical},
	}

	mentorStory := keyword.StarStory{
		ID:         "mentor-story",
		Title:      "Mentored junior engineer",
		Situation:  "Junior dev was struggling with onboarding.",
		Task:       "Help them ramp up.",
		Action:     "Set up pair programming and weekly 1:1s.",
		Result:     "Engineer was productive within 6 weeks.",
		Skills:     []string{"Mentoring"},
		Categories: []keyword.StoryCategory{keyword.StoryMentoring},
	}

	// Test 1: uncovered behavioral question — only technical stories available
	t.Run("behavioral question uncovered with only technical stories", func(t *testing.T) {
		result := keyword.SelectStories(
			[]keyword.InterviewQuestion{behavioralQuestion},
			[]keyword.StarStory{technicalStory},
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

	// Test 2: uncovered technical question — only behavioral stories available
	t.Run("technical question uncovered with only behavioral stories", func(t *testing.T) {
		result := keyword.SelectStories(
			[]keyword.InterviewQuestion{uncoveredTechnical},
			[]keyword.StarStory{mentorStory},
			nil,
		)
		if result.MappedQuestions[0].IsCovered {
			t.Fatal("expected technical question to be uncovered with only behavioral stories")
		}
		if result.UncoveredCount != 1 {
			t.Errorf("uncovered count = %d, want 1", result.UncoveredCount)
		}
	})

	// Test 3: mixed — one covered (technical with technical story), two uncovered
	// (behavioral with no behavioral story, technical with Rust and no technical
	// stories)
	t.Run("mixed covered and uncovered", func(t *testing.T) {
		result := keyword.SelectStories(
			[]keyword.InterviewQuestion{coveredTechnical, uncoveredBehavioral},
			[]keyword.StarStory{technicalStory},
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
	// Jaccard is unexported, so we test through SelectStories as a proxy
	q := keyword.InterviewQuestion{
		Text:     "The role requires Python and Docker.",
		Category: keyword.CategoryTechnical,
		Source:   keyword.SourceRequiredSkill,
	}

	highMatch := keyword.StarStory{
		ID:         "hi",
		Title:      "High match",
		Situation:  "Story with Python and Docker.",
		Task:       "Task",
		Action:     "Action",
		Result:     "Result",
		Skills:     []string{"Python", "Docker"},
		Categories: []keyword.StoryCategory{keyword.StoryTechnical},
	}

	lowMatch := keyword.StarStory{
		ID:         "lo",
		Title:      "Low match",
		Situation:  "Story with only one matching skill.",
		Task:       "Task",
		Action:     "Action",
		Result:     "Result",
		Skills:     []string{"Python", "Rust", "Go"},
		Categories: []keyword.StoryCategory{keyword.StoryTechnical},
	}

	noMatch := keyword.StarStory{
		ID:         "no",
		Title:      "No match",
		Situation:  "Story with no matching skills.",
		Task:       "Task",
		Action:     "Action",
		Result:     "Result",
		Skills:     []string{"React", "TypeScript"},
		Categories: []keyword.StoryCategory{keyword.StoryTechnical},
	}

	result := keyword.SelectStories([]keyword.InterviewQuestion{q},
		[]keyword.StarStory{noMatch, lowMatch, highMatch}, nil)

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
	// Topics from "The role requires Python and Docker.":
	//   filterStopwords → ["Python", "Docker"]  ("the" and "and" are stopwords;
	//   "role" and "requires" are not stopwords but get dropped during alias
	//   resolution since they have no canonical form → resolveAlias returns
	//   them as-is)

	// Actually extractQuestionTopics returns ["Python", "Docker"] because
	// "role" and "requires" are kept as-is via resolveAlias. Let's verify:
	// hi:  skills=[Python,Docker], Jaccard=2/(4+2-2)=0.5, category=1.0
	//      → score = 0.5*0.5 + 0.3*1.0 = 0.55
	// lo:  skills=[Python,Rust,Go], Jaccard=1/(4+3-1)=0.167, category=1.0
	//      → score = 0.5*0.167 + 0.3*1.0 = 0.383
	// no:  skills=[React,TypeScript], Jaccard=0/(4+2-0)=0, category=1.0
	//      → score = 0.5*0.0 + 0.3*1.0 = 0.3
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
	story := keyword.StarStory{
		ID:         "long-story",
		Title:      "Long story",
		Situation:  "This is a very long situation string that exceeds the two hundred character excerpt limit and should be truncated to show only the first part with an ellipsis character appended at the truncation point. The remaining content of the situation field is not included in the excerpt.",
		Task:       "Task",
		Action:     "Action",
		Result:     "Result",
		Skills:     []string{"Python"},
		Categories: []keyword.StoryCategory{keyword.StoryTechnical},
	}
	q := keyword.InterviewQuestion{
		Text:     "The role requires Python.",
		Category: keyword.CategoryTechnical,
		Source:   keyword.SourceRequiredSkill,
	}

	result := keyword.SelectStories([]keyword.InterviewQuestion{q}, []keyword.StarStory{story}, nil)
	excerpt := result.MappedQuestions[0].MappedStories[0].Excerpt
	if len([]rune(excerpt)) > 201 { // 200 chars + "…"
		t.Errorf("excerpt length = %d, want <= 201", len([]rune(excerpt)))
	}
	if len(excerpt) >= len(story.Situation) {
		t.Errorf("excerpt length = %d, should be shorter than situation (%d)", len(excerpt), len(story.Situation))
	}
}
