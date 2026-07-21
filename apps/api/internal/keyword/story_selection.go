package keyword

import (
	"sort"
	"strings"
)

// StarStory represents a user-authored STAR story from the master profile.
type StarStory struct {
	ID         string          `json:"id"`
	ProfileID  string          `json:"profile_id"`
	Title      string          `json:"title"`
	Situation  string          `json:"situation"`
	Task       string          `json:"task"`
	Action     string          `json:"action"`
	Result     string          `json:"result"`
	Skills     []string        `json:"skills"`
	Categories []StoryCategory `json:"categories"`
}

// StoryCategory is a thematic tag for a STAR story.
type StoryCategory string

const (
	StoryLeadership         StoryCategory = "leadership"
	StoryProblemSolving     StoryCategory = "problem_solving"
	StoryTeamwork           StoryCategory = "teamwork"
	StoryTechnical          StoryCategory = "technical"
	StoryCommunication      StoryCategory = "communication"
	StoryInitiative         StoryCategory = "initiative"
	StoryFailureResilience  StoryCategory = "failure_resilience"
	StoryConflictResolution StoryCategory = "conflict_resolution"
	StoryMentoring          StoryCategory = "mentoring"
	StoryProcessImprovement StoryCategory = "process_improvement"
)

// StoryMapping links a STAR story to an interview question with a relevance score.
type StoryMapping struct {
	StoryID        string   `json:"story_id"`
	StoryTitle     string   `json:"story_title"`
	RelevanceScore float64  `json:"relevance_score"`
	MatchedSkills  []string `json:"matched_skills"`
	Excerpt        string   `json:"excerpt"`
}

// StorySelectionResult holds the outcome of selecting stories for questions.
type StorySelectionResult struct {
	MappedQuestions []MappedQuestion `json:"mapped_questions"`
	UncoveredCount  int              `json:"uncovered_count"`
}

// MappedQuestion is an interview question with its best-matching stories.
type MappedQuestion struct {
	Question      InterviewQuestion `json:"question"`
	MappedStories []StoryMapping    `json:"mapped_stories"`
	IsCovered     bool              `json:"is_covered"`
}

// EmbeddingSimilarityFunc computes embedding similarity between a question
// and a story. Callers provide a cosine-similarity function backed by an
// embedding model; when nil, similarity defaults to 0.0.
type EmbeddingSimilarityFunc func(questionText string, story StarStory) float64

// scoring weights from spec §3.3
const (
	skillOverlapWeight   = 0.5
	categoryMatchWeight  = 0.3
	embeddingSimWeight   = 0.2
	minRelevanceScore    = 0.15
	maxStoriesPerQuestion = 3
)

// SelectStories matches the best STAR stories to each interview question
// using the algorithm from spec §3.3. Questions with no story meeting the
// minimum relevance threshold are marked uncovered.
func SelectStories(questions []InterviewQuestion, stories []StarStory, embedFn EmbeddingSimilarityFunc) StorySelectionResult {
	if embedFn == nil {
		embedFn = noEmbedding
	}

	var mapped []MappedQuestion
	uncovered := 0

	for _, q := range questions {
		qTopics := extractQuestionTopics(q.Text)

		var mappings []StoryMapping
		for _, s := range stories {
			score := scoreStory(qTopics, q.Category, s, embedFn, q.Text)
			if score < minRelevanceScore {
				continue
			}
			mappings = append(mappings, StoryMapping{
				StoryID:        s.ID,
				StoryTitle:     s.Title,
				RelevanceScore: score,
				MatchedSkills:  matchedSkills(qTopics, s.Skills),
				Excerpt:        truncate(s.Situation, 200),
			})
		}

		sort.Slice(mappings, func(i, j int) bool {
			return mappings[i].RelevanceScore > mappings[j].RelevanceScore
		})
		if len(mappings) > maxStoriesPerQuestion {
			mappings = mappings[:maxStoriesPerQuestion]
		}

		isCovered := len(mappings) > 0
		if !isCovered {
			uncovered++
		}

		mapped = append(mapped, MappedQuestion{
			Question:      q,
			MappedStories: mappings,
			IsCovered:     isCovered,
		})
	}

	return StorySelectionResult{
		MappedQuestions: mapped,
		UncoveredCount:  uncovered,
	}
}

func noEmbedding(_ string, _ StarStory) float64 { return 0 }

// scoreStory computes the weighted relevance score between a question and story.
func scoreStory(qTopics []string, cat QuestionCategory, s StarStory, embedFn EmbeddingSimilarityFunc, qText string) float64 {
	skillOverlap := jaccardSimilarity(qTopics, s.Skills)
	categoryMatch := categoryMatchScore(cat, s.Categories)
	embeddingSim := embedFn(qText, s)
	return skillOverlapWeight*skillOverlap +
		categoryMatchWeight*categoryMatch +
		embeddingSimWeight*embeddingSim
}

// jaccardSimilarity computes |A ∩ B| / |A ∪ B|. Returns 0 for empty sets.
func jaccardSimilarity(a, b []string) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(a))
	for _, s := range a {
		key := lowerASCII(strings.TrimSpace(s))
		if key != "" {
			set[key] = struct{}{}
		}
	}
	intersection := 0
	for _, s := range b {
		key := lowerASCII(strings.TrimSpace(s))
		if _, ok := set[key]; ok {
			intersection++
		}
	}
	union := len(a) + len(b) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// categoryMatchScore returns 1.0 when the story's categories are compatible
// with the question category, 0.0 otherwise.
func categoryMatchScore(qCat QuestionCategory, sCats []StoryCategory) float64 {
	for _, sc := range sCats {
		if questionCategoryCompatible(qCat, sc) {
			return 1.0
		}
	}
	return 0.0
}

// questionCategoryCompatible reports whether a StoryCategory is compatible
// with a QuestionCategory for matching purposes.
func questionCategoryCompatible(q QuestionCategory, s StoryCategory) bool {
	switch q {
	case CategoryTechnical:
		return s == StoryTechnical
	case CategoryBehavioral:
		return s != StoryTechnical
	default:
		return false
	}
}

// extractQuestionTopics tokenizes question text into normalized skill-like
// tokens for Jaccard matching against story skills.
func extractQuestionTopics(text string) []string {
	clean := stripPunctRe.ReplaceAllString(text, " ")
	clean = collapseSpace(strings.TrimSpace(clean))
	tokens := tokenRe.FindAllString(clean, -1)
	tokens = filterStopwords(tokens)

	seen := make(map[string]struct{}, len(tokens))
	var out []string
	for _, t := range tokens {
		normalized := lowerASCII(strings.TrimSpace(t))
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		resolved := resolveAlias(t)
		if resolved != "" {
			out = append(out, resolved)
		}
	}
	return out
}

// matchedSkills returns the intersection of question topics and story skills
// using case-insensitive comparison.
func matchedSkills(topics, skills []string) []string {
	skillSet := make(map[string]struct{}, len(skills))
	for _, s := range skills {
		skillSet[lowerASCII(strings.TrimSpace(s))] = struct{}{}
	}
	var out []string
	for _, t := range topics {
		lt := lowerASCII(strings.TrimSpace(t))
		if _, ok := skillSet[lt]; ok {
			out = append(out, t)
		}
	}
	return out
}

// truncate returns up to n runes of s, appending "…" if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

