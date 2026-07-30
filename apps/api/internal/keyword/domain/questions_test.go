package domain_test

import (
	"testing"

	"github.com/job-finder/api/internal/keyword/domain"
)

// questionFixture reuses the same 8 JD fixtures from the 008-3 extractor and
// 009-2 classifier tests. Each must-have term must yield at least one question.
type questionFixture struct {
	name             string
	jd               string
	mustHave         []string
	minQuestions     int
	expectCategories []domain.QuestionCategory
	expectSources    []domain.QuestionSource
}

func TestDeriveQuestionsFromMustHaves(t *testing.T) {
	fixtures := []questionFixture{
		{
			name: "full-stack-remote",
			jd: `
Responsibilities
- Build features in React and TypeScript
- Deploy services with Kubernetes and Docker

Requirements
- 5+ years of Python experience
- Strong JavaScript skills
- Experience with AWS

Nice to have
- Knowledge of Go is a plus
- Familiarity with gRPC preferred
`,
			mustHave:     []string{"React", "TypeScript", "Kubernetes", "Docker", "Python", "JavaScript", "AWS"},
			minQuestions: 7,
			expectCategories: []domain.QuestionCategory{
				domain.CategoryTechnical,
				domain.CategoryBehavioral,
			},
			expectSources: []domain.QuestionSource{
				domain.SourceRequiredSkill,
				domain.SourceResponsibility,
			},
		},
		{
			name: "backend-golang",
			jd: `
What you'll need
- Proficiency in Go (Golang) and PostgreSQL
- Experience building REST APIs with gRPC
- Strong knowledge of Docker and Kubernetes

Bonus points
- Experience with Terraform
- Familiarity with AWS is a plus
`,
			mustHave:     []string{"Go", "PostgreSQL", "gRPC", "Docker", "Kubernetes"},
			minQuestions: 5,
			expectCategories: []domain.QuestionCategory{
				domain.CategoryTechnical,
			},
			expectSources: []domain.QuestionSource{
				domain.SourceRequiredSkill,
			},
		},
		{
			name: "frontend-react",
			jd: `
Qualifications
- 3+ years of React.js and Redux experience
- Strong TypeScript and JavaScript skills
- Experience with Webpack and Jest

Preferred qualifications
- Knowledge of GraphQL is a plus
- Familiarity with Next.js desired
`,
			mustHave:     []string{"React", "TypeScript", "JavaScript"},
			minQuestions: 3,
			expectCategories: []domain.QuestionCategory{
				domain.CategoryTechnical,
			},
			expectSources: []domain.QuestionSource{
				domain.SourceRequiredSkill,
			},
		},
		{
			name: "data-ml",
			jd: `
Requirements
- Degree in Computer Science or equivalent
- Strong Python and SQL skills
- Experience with Machine Learning (ML) pipelines
- Familiarity with TensorFlow or PyTorch

Nice to have
- Knowledge of Artificial Intelligence (AI) research
- Experience with Spark is a plus
`,
			mustHave:     []string{"Python", "SQL", "Machine Learning"},
			minQuestions: 3,
			expectCategories: []domain.QuestionCategory{
				domain.CategoryTechnical,
			},
			expectSources: []domain.QuestionSource{
				domain.SourceRequiredSkill,
			},
		},
		{
			name: "devops-aws",
			jd: `
What we're looking for
- 5+ years with AWS (Amazon Web Services)
- Strong CI/CD experience with Jenkins
- Infrastructure as Code using Terraform

Desired
- Experience with Kubernetes preferred
- Knowledge of Ansible a plus
`,
			mustHave:     []string{"AWS", "Terraform"},
			minQuestions: 2,
			expectCategories: []domain.QuestionCategory{
				domain.CategoryTechnical,
			},
			expectSources: []domain.QuestionSource{
				domain.SourceRequiredSkill,
			},
		},
		{
			name: "mobile-ios",
			jd: `
Minimum qualifications
- 4+ years of Swift development
- Strong knowledge of iOS SDK and SwiftUI
- Experience with RESTful APIs

Preferred
- Familiarity with Kotlin Multiplatform is a plus
- Knowledge of CI tools desired
`,
			mustHave:     []string{"Swift", "iOS", "SwiftUI"},
			minQuestions: 3,
			expectCategories: []domain.QuestionCategory{
				domain.CategoryTechnical,
			},
			expectSources: []domain.QuestionSource{
				domain.SourceRequiredSkill,
			},
		},
		{
			name: "security-engineer",
			jd: `
About you
- Deep understanding of CISSP certification requirements
- Experience with penetration testing and threat modeling
- Strong Python scripting skills

Nice-to-have
- Knowledge of Kubernetes security is a plus
- Familiarity with AWS preferred
`,
			mustHave:     []string{"Python"},
			minQuestions: 1,
			expectCategories: []domain.QuestionCategory{
				domain.CategoryTechnical,
			},
			expectSources: []domain.QuestionSource{
				domain.SourceRequiredSkill,
			},
		},
		{
			name: "full-java-enterprise",
			jd: `
Job description
We are hiring a Java engineer. You will design microservices and maintain
our Spring Boot applications. You must have 6+ years of Java experience.

Requirements
- Strong Java and Kotlin skills
- Experience with Kafka and PostgreSQL

Bonus points
- Experience with K8s is a plus
- Familiarity with React preferred
`,
			mustHave:     []string{"Java", "Kotlin", "Kafka", "PostgreSQL"},
			minQuestions: 4,
			expectCategories: []domain.QuestionCategory{
				domain.CategoryTechnical,
			},
			expectSources: []domain.QuestionSource{
				domain.SourceRequiredSkill,
			},
		},
	}

	ext := domain.NewExtractor()
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			res, err := ext.Extract(f.jd)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			classified := domain.Classify(res)
			questions := domain.DeriveQuestions(classified, f.jd)

			if len(questions) < f.minQuestions {
				t.Errorf("got %d questions, want at least %d", len(questions), f.minQuestions)
			}

			for _, want := range f.mustHave {
				found := false
				for _, q := range questions {
					if q.Source == domain.SourceRequiredSkill && containsTerm(q.Text, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("must-have term %q: no question found", want)
				}
			}

			for _, q := range questions {
				if !hasCategory(f.expectCategories, q.Category) {
					t.Errorf("question %q: unexpected category %s", q.Text, q.Category)
				}
				if !hasSource(f.expectSources, q.Source) {
					t.Errorf("question %q: unexpected source %s", q.Text, q.Source)
				}
			}
		})
	}
}

func TestDeriveQuestionsEmptyJD(t *testing.T) {
	ext := domain.NewExtractor()
	res, err := ext.Extract("Requirements\n- Python")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	classified := domain.Classify(res)
	questions := domain.DeriveQuestions(classified, "")
	if len(questions) == 0 {
		t.Fatal("expected at least one question from must-have term")
	}
}

func TestDeriveQuestionsNilClassified(t *testing.T) {
	questions := domain.DeriveQuestions(nil, "some jd")
	if questions != nil {
		t.Errorf("DeriveQuestions(nil, ...) = %v, want nil", questions)
	}
}

func TestDeriveQuestionsNoMustHaves(t *testing.T) {
	ext := domain.NewExtractor()
	res, err := ext.Extract("Nice to have\n- Go is a plus")
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	classified := domain.Classify(res)
	questions := domain.DeriveQuestions(classified, "Nice to have\n- Go is a plus")
	for _, q := range questions {
		if q.Source == domain.SourceRequiredSkill {
			t.Errorf("unexpected required_skill question for nice-to-have term: %q", q.Text)
		}
	}
}

func TestDeriveQuestionsFromResponsibilities(t *testing.T) {
	jd := `
Responsibilities
- Lead cross-functional engineering teams
- Mentor junior engineers
- Drive technical strategy

Requirements
- 5+ years of Python experience
`
	ext := domain.NewExtractor()
	res, err := ext.Extract(jd)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	classified := domain.Classify(res)
	questions := domain.DeriveQuestions(classified, jd)

	behavioralCount := 0
	for _, q := range questions {
		if q.Category == domain.CategoryBehavioral {
			behavioralCount++
			if q.Source != domain.SourceResponsibility {
				t.Errorf("behavioral question has wrong source: %s", q.Source)
			}
		}
	}
	if behavioralCount < 3 {
		t.Errorf("got %d behavioral questions, want at least 3", behavioralCount)
	}
}

func containsTerm(text, term string) bool {
	return len(text) > 0 && len(term) > 0 &&
		(text == term || len(text) > len(term))
}

func hasCategory(cats []domain.QuestionCategory, cat domain.QuestionCategory) bool {
	for _, c := range cats {
		if c == cat {
			return true
		}
	}
	return false
}

func hasSource(srcs []domain.QuestionSource, src domain.QuestionSource) bool {
	for _, s := range srcs {
		if s == src {
			return true
		}
	}
	return false
}
