package domain

import (
	"fmt"
	"regexp"
	"strings"
)

type QuestionCategory string

const (
	CategoryTechnical   QuestionCategory = "technical"
	CategoryBehavioral  QuestionCategory = "behavioral"
	CategoryExperience  QuestionCategory = "experience"
	CategorySituational QuestionCategory = "situational"
	CategoryCompany     QuestionCategory = "company"
	CategoryGap         QuestionCategory = "gap"
)

type QuestionSource string

const (
	SourceRequiredSkill  QuestionSource = "required_skill"
	SourcePreferredSkill QuestionSource = "preferred_skill"
	SourceResponsibility QuestionSource = "responsibility"
	SourceCompanyContext QuestionSource = "company_context"
	SourceGeneric        QuestionSource = "generic"
)

type InterviewQuestion struct {
	ID            string           `json:"id"`
	Text          string           `json:"text"`
	Category      QuestionCategory `json:"category"`
	Source        QuestionSource   `json:"source"`
	SourceExcerpt string           `json:"source_excerpt"`
}

var respHeaderPattern = regexp.MustCompile(`(?i)^\s*(?:#{1,3}\s*|\*\*)?\s*(?:responsibilities|what you'?ll do|the role|what you will do|role description|job description)\b`)

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

func lowerFirst(s string) string {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
