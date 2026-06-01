package generation

import (
	"testing"

	"github.com/job-finder/api-go/internal/dto"
)

func strp(s string) *string { return &s }

func TestVerifyGrounding_RejectsFabricatedEmployer(t *testing.T) {
	master := dto.JsonResume{
		Work: []dto.ResumeWork{
			{Name: "Acme Corp", StartDate: strp("2020-01"), EndDate: strp("2022-01")},
		},
	}
	tailored := dto.JsonResume{
		Work: []dto.ResumeWork{
			{Name: "Totally Fake Inc", StartDate: strp("2020-01"), EndDate: strp("2022-01")},
		},
	}
	violations := verifyGrounding(master, tailored)
	if len(violations) == 0 {
		t.Fatal("expected a violation for a fabricated employer")
	}
}

func TestVerifyGrounding_AllowsReorderingAndRephrasing(t *testing.T) {
	master := dto.JsonResume{
		Work: []dto.ResumeWork{
			{Name: "Acme Corp", StartDate: strp("2020-01"), EndDate: strp("2022-01"), Highlights: []string{"Built X"}},
		},
		Education: []dto.ResumeEducation{
			{Institution: "MIT", StartDate: strp("2016"), EndDate: strp("2020")},
		},
		Projects: []dto.ResumeProject{{Name: "Side Project"}},
	}
	tailored := dto.JsonResume{
		Work: []dto.ResumeWork{
			{Name: "acme corp", StartDate: strp("2020-01"), EndDate: strp("2022-01"), Highlights: []string{"Shipped X to prod"}},
		},
		Education: []dto.ResumeEducation{{Institution: "MIT", StartDate: strp("2016"), EndDate: strp("2020")}},
		Projects:  []dto.ResumeProject{{Name: "Side Project"}},
	}
	violations := verifyGrounding(master, tailored)
	if len(violations) != 0 {
		t.Fatalf("expected no violations for case-insensitive match + rephrased highlight, got %v", violations)
	}
}

func TestVerifyGrounding_RejectsFabricatedDate(t *testing.T) {
	master := dto.JsonResume{
		Work: []dto.ResumeWork{{Name: "Acme Corp", StartDate: strp("2020-01"), EndDate: strp("2022-01")}},
	}
	tailored := dto.JsonResume{
		Work: []dto.ResumeWork{{Name: "Acme Corp", StartDate: strp("2019-01"), EndDate: strp("2022-01")}},
	}
	violations := verifyGrounding(master, tailored)
	if len(violations) == 0 {
		t.Fatal("expected a violation for a fabricated start date")
	}
}

func TestVerifyGrounding_RejectsFabricatedProject(t *testing.T) {
	master := dto.JsonResume{Projects: []dto.ResumeProject{{Name: "Real Project"}}}
	tailored := dto.JsonResume{Projects: []dto.ResumeProject{{Name: "Imaginary Project"}}}
	violations := verifyGrounding(master, tailored)
	if len(violations) == 0 {
		t.Fatal("expected a violation for a fabricated project")
	}
}
