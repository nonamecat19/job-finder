package keyword

import (
	"fmt"
	"regexp"
	"strings"
)

// QuestionCategory classifies the type of interview question. Values mirror
// the shared frontend union in packages/shared/src/index.ts.
type QuestionCategory string

const (
	CategoryTechnical   QuestionCategory = "technical"
	CategoryBehavioral  QuestionCategory = "behavioral"
	CategoryExperience  QuestionCategory = "experience"
	CategorySituational QuestionCategory = "situational"
	CategoryCompany     QuestionCategory = "company"
	CategoryGap         QuestionCategory = "gap"
)

// QuestionSource identifies what in the JD triggered this question. Values
// mirror the shared frontend union in packages/shared/src/index.ts.
type QuestionSource string

const (
	SourceRequiredSkill  QuestionSource = "required_skill"
	SourcePreferredSkill QuestionSource = "preferred_skill"
	SourceResponsibility QuestionSource = "responsibility"
	SourceCompanyContext QuestionSource = "company_context"
	SourceGeneric        QuestionSource = "generic"
)

// InterviewQuestion is a derived interview question from the JD.
type InterviewQuestion struct {
	ID            string           `json:"id"`
	Text          string           `json:"text"`
	Category      QuestionCategory `json:"category"`
	Source        QuestionSource   `json:"source"`
	SourceExcerpt string           `json:"source_excerpt"`
}

// respHeaderPattern matches section headers that indicate responsibilities or
// role-description content (case-insensitive, with optional markdown formatting).
var respHeaderPattern = regexp.MustCompile(`(?i)^\s*(?:#{1,3}\s*|\*\*)?\s*(?:responsibilities|what you'?ll do|the role|what you will do|role description|job description)\b`)

// DeriveQuestions produces interview questions from classified terms and the
// original JD text. Each must-have term yields at least one technical question.
// Responsibility bullets from the JD yield behavioral questions.
//
// It reuses the 008-3 extractor output (via ClassifiedTerm.ExtractedTerm) and
// the 009-2 classifier verdict (via ClassifiedTerm.Class) rather than re-parsing
// the JD from scratch.
func DeriveQuestions(classified []ClassifiedTerm, jd string) []InterviewQuestion {
	var questions []InterviewQuestion

	for i, ct := range classified {
		if ct.Class != ClassMustHave {
			continue
		}
		questions = append(questions, InterviewQuestion{
			ID:            fmt.Sprintf("required-skill-%d", i),
			Text:          fmt.Sprintf("The role requires %s. Can you describe your experience with %s?", ct.Canonical, ct.Canonical),
			Category:      CategoryTechnical,
			Source:        SourceRequiredSkill,
			SourceExcerpt: ct.Evidence,
		})
	}

	for i, bullet := range extractResponsibilityBullets(jd) {
		questions = append(questions, InterviewQuestion{
			ID:            fmt.Sprintf("responsibility-%d", i),
			Text:          fmt.Sprintf("Tell me about a time you had to %s.", lowerFirst(bullet)),
			Category:      CategoryBehavioral,
			Source:        SourceResponsibility,
			SourceExcerpt: bullet,
		})
	}

	return questions
}

// extractResponsibilityBullets finds sections with responsibility-like headers
// and returns their bullet points. It stops collecting when it hits another
// section header.
func extractResponsibilityBullets(jd string) []string {
	var bullets []string
	lines := strings.Split(jd, "\n")
	inRespSection := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if respHeaderPattern.MatchString(trimmed) {
			inRespSection = true
			continue
		}
		if inRespSection && headerRe.MatchString(trimmed) {
			inRespSection = false
			continue
		}
		if inRespSection {
			if m := bulletRe.FindStringSubmatch(trimmed); m != nil {
				bullets = append(bullets, strings.TrimSpace(m[1]))
			}
		}
	}
	return bullets
}

// lowerFirst lowercases the first character of a string.
func lowerFirst(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
