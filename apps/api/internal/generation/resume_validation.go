package generation

import (
	"fmt"
	"strings"

	"github.com/job-finder/api/internal/dto"
)

type ValidationError struct {
	Path    string
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

func ValidateResume(resume dto.Resume) *ValidationError {
	if strings.TrimSpace(resume.Name) == "" {
		return &ValidationError{Path: "name", Message: "name is required"}
	}
	for si, sec := range resume.Sections {
		if strings.TrimSpace(sec.Name) == "" {
			return &ValidationError{Path: fmt.Sprintf("sections[%d].name", si), Message: "section name is required"}
		}
		for ei, e := range sec.Entries {
			path := fmt.Sprintf("sections[%d].entries[%d]", si, ei)
			if e.EndDate != nil && e.StartDate != nil && *e.EndDate != "present" && *e.EndDate < *e.StartDate {
				return &ValidationError{Path: path + ".endDate", Message: "end date must not be before start date"}
			}
			if entryIsBlank(sec.EntryType, e) {
				return &ValidationError{Path: path, Message: "entry cannot be entirely blank"}
			}
		}
	}
	return nil
}

func entryIsBlank(entryType dto.EntryType, e dto.Entry) bool {
	nonEmpty := func(s *string) bool { return s != nil && strings.TrimSpace(*s) != "" }
	switch entryType {
	case dto.EntryEducation:
		return !nonEmpty(e.Institution) && !nonEmpty(e.Area) && !nonEmpty(e.Degree) && !nonEmpty(e.Summary) && len(e.Highlights) == 0
	case dto.EntryExperience:
		return !nonEmpty(e.Company) && !nonEmpty(e.Position) && !nonEmpty(e.Summary) && len(e.Highlights) == 0
	case dto.EntryNormal:
		return !nonEmpty(e.Name) && !nonEmpty(e.URL) && !nonEmpty(e.Summary) && len(e.Highlights) == 0
	case dto.EntryPublication:
		return !nonEmpty(e.Title) && len(e.Authors) == 0
	case dto.EntryOneLine:
		return !nonEmpty(e.Label) && !nonEmpty(e.Details)
	case dto.EntryBullet:
		return !nonEmpty(e.Bullet)
	case dto.EntryNumbered:
		return !nonEmpty(e.Number)
	case dto.EntryReversedNumbered:
		return !nonEmpty(e.ReversedNumber)
	case dto.EntryText:
		return !nonEmpty(e.Text)
	default:
		return len(e.Unrecognized) == 0
	}
}
