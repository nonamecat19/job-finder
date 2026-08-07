package domain_test

import (
	"testing"

	"github.com/job-finder/api/internal/keyword/domain"
)

var _ domain.Extractor = (*domain.RegexExtractor)(nil)

type fixture struct {
	name      string
	jd        string
	required  map[string]bool
	preferred map[string]bool
}

func findTerm(res *domain.ExtractResult, canonical string) (domain.ExtractedTerm, bool) {
	for _, t := range res.Terms {
		if t.Canonical == canonical {
			return t, true
		}
	}
	return domain.ExtractedTerm{}, false
}

func TestExtractClassifiesRequiredVsPreferred(t *testing.T) {
	fixtures := []fixture{
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
			required: map[string]bool{
				"React": true, "TypeScript": true, "Kubernetes": true,
				"Docker": true, "Python": true, "JavaScript": true, "AWS": true,
			},
			preferred: map[string]bool{"Go": true, "gRPC": true},
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
			required: map[string]bool{
				"Go": true, "PostgreSQL": true, "gRPC": true,
				"Docker": true, "Kubernetes": true,
			},
			preferred: map[string]bool{"AWS": true, "Terraform": true},
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
			required: map[string]bool{
				"React": true, "TypeScript": true, "JavaScript": true,
			},
			preferred: map[string]bool{"GraphQL": true},
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
			required: map[string]bool{
				"Python": true, "SQL": true, "Machine Learning": true,
			},
			preferred: map[string]bool{"Artificial Intelligence": true, "Spark": true},
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
			required: map[string]bool{
				"AWS": true, "Terraform": true,
			},
			preferred: map[string]bool{"Kubernetes": true, "Ansible": true},
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
			required: map[string]bool{
				"Swift": true, "iOS": true, "SwiftUI": true,
			},
			preferred: map[string]bool{},
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
			required: map[string]bool{
				"Python": true,
			},
			preferred: map[string]bool{"Kubernetes": true, "AWS": true},
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
			required: map[string]bool{
				"Java": true, "Kotlin": true, "Kafka": true, "PostgreSQL": true,
			},
			preferred: map[string]bool{"React": true},
		},
	}

	ext := domain.NewExtractor()
	for _, f := range fixtures {
		t.Run(f.name, func(t *testing.T) {
			res, err := ext.Extract(f.jd)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			for want := range f.required {
				term, ok := findTerm(res, want)
				if !ok {
					t.Errorf("expected required term %q not found; got terms: %v", want, termList(res))
					continue
				}
				if term.Polarity != domain.PolarityRequired {
					t.Errorf("term %q: expected required, got %s", want, term.Polarity)
				}
			}
			for want := range f.preferred {
				term, ok := findTerm(res, want)
				if !ok {
					t.Errorf("expected preferred term %q not found; got terms: %v", want, termList(res))
					continue
				}
				if term.Polarity != domain.PolarityPreferred {
					t.Errorf("term %q: expected preferred, got %s", want, term.Polarity)
				}
			}
		})
	}
}

func termList(res *domain.ExtractResult) []string {
	out := make([]string, 0, len(res.Terms))
	for _, t := range res.Terms {
		out = append(out, t.Canonical+"("+string(t.Polarity)+")")
	}
	return out
}

func TestAcronymAndSynonymExpansion(t *testing.T) {
	tests := []struct {
		name      string
		jd        string
		canonical string
	}{
		{"k8s expands to kubernetes", "Requirements\n- Experience with K8s", "Kubernetes"},
		{"kube expands to kubernetes", "Requirements\n- Deploy on Kube", "Kubernetes"},
		{"js expands to javascript", "Requirements\n- Strong JS skills", "JavaScript"},
		{"ts expands to typescript", "Requirements\n- Write TS code", "TypeScript"},
		{"node expands to node.js", "Requirements\n- Use Node in production", "Node.js"},
		{"ci expands to continuous integration", "Requirements\n- Set up CI pipelines", "Continuous Integration"},
	}
	ext := domain.NewExtractor()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ext.Extract(tt.jd)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			term, ok := findTerm(res, tt.canonical)
			if !ok {
				t.Fatalf("expected canonical %q, got terms: %v", tt.canonical, termList(res))
			}
			if term.Canonical != tt.canonical {
				t.Errorf("canonical = %q, want %q", term.Canonical, tt.canonical)
			}
		})
	}
}

func TestStemmingNormalizesInflections(t *testing.T) {
	tests := []struct {
		name string
		jd   string
		stem string
	}{
		{"python services", "Requirements\n- Build Python services", "python"},
		{"containerize", "Requirements\n- Ability to containerize apps", "docker"},
		{"deploying", "Requirements\n- Experience deploying to cloud", "deploy"},
	}
	ext := domain.NewExtractor()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ext.Extract(tt.jd)
			if err != nil {
				t.Fatalf("Extract: %v", err)
			}
			found := false
			for _, term := range res.Terms {
				if term.Stemmed == tt.stem {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected a term with stem %q, got: %v", tt.stem, termList(res))
			}
		})
	}
}

func TestExtractEmptyReturnsError(t *testing.T) {
	_, err := domain.NewExtractor().Extract("   ")
	if err == nil {
		t.Fatal("expected error for empty JD, got nil")
	}
}

func TestNewServiceAcceptsExtractorPort(t *testing.T) {
	ext := domain.NewExtractor()
	if ext == nil {
		t.Fatal("NewExtractor returned nil")
	}
}
