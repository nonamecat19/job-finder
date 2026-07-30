package domain_test

import (
	"testing"

	"github.com/job-finder/api/internal/keyword/domain"
)

// classifyFixture reuses the same JD fixture set exercised by the 008-3
// extractor tests (service_test.go): every term the JD frames as a hard
// requirement must classify as must_have, every optional term as nice_to_have.
type classifyFixture struct {
	name      string
	jd        string
	mustHave  []string // canonical terms expected must_have
	niceToHav []string // canonical terms expected nice_to_have
}

func findClassified(cs []domain.ClassifiedTerm, canonical string) (domain.ClassifiedTerm, bool) {
	for _, c := range cs {
		if c.Canonical == canonical {
			return c, true
		}
	}
	return domain.ClassifiedTerm{}, false
}

func classifiedList(cs []domain.ClassifiedTerm) []string {
	out := make([]string, 0, len(cs))
	for _, c := range cs {
		out = append(out, c.Canonical+"("+string(c.Class)+")")
	}
	return out
}

func TestClassifyMustHaveVsNiceToHave(t *testing.T) {
	fixtures := []classifyFixture{
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
			mustHave:  []string{"React", "TypeScript", "Kubernetes", "Docker", "Python", "JavaScript", "AWS"},
			niceToHav: []string{"Go", "gRPC"},
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
			mustHave:  []string{"Go", "PostgreSQL", "gRPC", "Docker", "Kubernetes"},
			niceToHav: []string{"AWS", "Terraform"},
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
			mustHave:  []string{"React", "TypeScript", "JavaScript"},
			niceToHav: []string{"GraphQL"},
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
			mustHave:  []string{"Python", "SQL", "Machine Learning"},
			niceToHav: []string{"Artificial Intelligence", "Spark"},
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
			mustHave:  []string{"AWS", "Terraform"},
			niceToHav: []string{"Kubernetes", "Ansible"},
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
			mustHave:  []string{"Swift", "iOS", "SwiftUI"},
			niceToHav: []string{},
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
			mustHave:  []string{"Python"},
			niceToHav: []string{"Kubernetes", "AWS"},
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
			mustHave:  []string{"Java", "Kotlin", "Kafka", "PostgreSQL"},
			niceToHav: []string{"React"},
		},
	}

	ext := domain.NewExtractor()
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			res, err := ext.Extract(f.jd)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			cs := domain.Classify(res)
			for _, want := range f.mustHave {
				c, ok := findClassified(cs, want)
				if !ok {
					t.Errorf("expected must_have term %q not found; got: %v", want, classifiedList(cs))
					continue
				}
				if c.Class != domain.ClassMustHave {
					t.Errorf("term %q: class = %s, want must_have (signals=%v)", want, c.Class, c.Signals)
				}
			}
			for _, want := range f.niceToHav {
				c, ok := findClassified(cs, want)
				if !ok {
					t.Errorf("expected nice_to_have term %q not found; got: %v", want, classifiedList(cs))
					continue
				}
				if c.Class != domain.ClassNiceToHave {
					t.Errorf("term %q: class = %s, want nice_to_have (signals=%v)", want, c.Class, c.Signals)
				}
			}
		})
	}
}

// TestClassifySignals asserts the classifier reports the concrete JD phrasing
// signal behind each verdict (task 009-2: 'required', 'must have',
// years-of-experience qualifiers).
func TestClassifySignals(t *testing.T) {
	tests := []struct {
		name    string
		jd      string
		term    string
		class   domain.Class
		signals []domain.Signal // subset that MUST be present
	}{
		{
			name:    "years-of-experience is a hard signal",
			jd:      "Requirements\n- 5+ years of Python experience",
			term:    "Python",
			class:   domain.ClassMustHave,
			signals: []domain.Signal{domain.SignalYearsOfExperience, domain.SignalRequiredSection},
		},
		{
			name:    "inline must-have promotes an optional-section term",
			jd:      "Nice to have\n- You must have Kubernetes in production",
			term:    "Kubernetes",
			class:   domain.ClassMustHave,
			signals: []domain.Signal{domain.SignalMustHavePhrase},
		},
		{
			name:    "inline required phrase is a hard signal",
			jd:      "About the role\n- AWS is required for this position",
			term:    "AWS",
			class:   domain.ClassMustHave,
			signals: []domain.Signal{domain.SignalRequiredPhrase},
		},
		{
			name:    "a plus stays nice-to-have",
			jd:      "Bonus points\n- Experience with Terraform is a plus",
			term:    "Terraform",
			class:   domain.ClassNiceToHave,
			signals: []domain.Signal{domain.SignalPreferredPhrase},
		},
	}
	ext := domain.NewExtractor()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ext.Extract(tt.jd)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			cs := domain.Classify(res)
			c, ok := findClassified(cs, tt.term)
			if !ok {
				t.Fatalf("term %q not found; got: %v", tt.term, classifiedList(cs))
			}
			if c.Class != tt.class {
				t.Errorf("class = %s, want %s (signals=%v)", c.Class, tt.class, c.Signals)
			}
			for _, want := range tt.signals {
				if !hasSignal(c.Signals, want) {
					t.Errorf("missing signal %q; got %v", want, c.Signals)
				}
			}
		})
	}
}

func hasSignal(sigs []domain.Signal, want domain.Signal) bool {
	for _, s := range sigs {
		if s == want {
			return true
		}
	}
	return false
}

func TestClassifyNilResult(t *testing.T) {
	if got := domain.Classify(nil); got != nil {
		t.Errorf("Classify(nil) = %v, want nil", got)
	}
}
