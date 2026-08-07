package application

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"

	"github.com/job-finder/api/internal/keyword/domain"
)

const ReasonNoHonestRephrase = "no-honest-rephrase-available"

const rephraseAttempts = 2

type RephraseSuggestion struct {
	Term         string  `json:"term"`
	Canonical    string  `json:"canonical"`
	Rephrase     *string `json:"rephrase"`
	SourceBullet string  `json:"sourceBullet,omitempty"`
	Reason       string  `json:"reason,omitempty"`
}

type RephraseModel interface {
	Rephrase(ctx context.Context, prompt string) (string, error)
}

type EvidenceFinder interface {
	FindEvidence(term domain.DiffTerm, bullets []string) (bullet string, ok bool)
}

type Suggester struct {
	model  RephraseModel
	finder EvidenceFinder
	log    *slog.Logger
}

func NewSuggester(model RephraseModel) *Suggester {
	return &Suggester{
		model:  model,
		finder: LexicalEvidenceFinder{},
		log:    slog.Default(),
	}
}

func (s *Suggester) WithEvidenceFinder(f EvidenceFinder) *Suggester {
	if f != nil {
		s.finder = f
	}
	return s
}

func (s *Suggester) WithLogger(l *slog.Logger) *Suggester {
	if l != nil {
		s.log = l
	}
	return s
}

func (s *Suggester) SuggestAll(ctx context.Context, missingRequired []domain.DiffTerm, profileBullets []string) []RephraseSuggestion {
	out := make([]RephraseSuggestion, 0, len(missingRequired))
	for _, t := range missingRequired {
		out = append(out, s.Suggest(ctx, t, profileBullets))
	}
	return out
}

func (s *Suggester) Suggest(ctx context.Context, term domain.DiffTerm, profileBullets []string) RephraseSuggestion {
	res := RephraseSuggestion{Term: term.Term, Canonical: term.Canonical}

	bullet, ok := s.finder.FindEvidence(term, profileBullets)
	if !ok {
		res.Reason = ReasonNoHonestRephrase
		return res
	}

	allowedProper := properNounSet(profileBullets)
	sourceNums := numberSet(bullet)

	var lastViol []string
	for attempt := 0; attempt < rephraseAttempts; attempt++ {
		prompt := buildRephrasePrompt(term, bullet, lastViol)
		raw, err := s.model.Rephrase(ctx, prompt)
		if err != nil {
			s.log.WarnContext(ctx, "keyword: rephrase model error",
				"term", term.Term, "error", err)
			res.Reason = ReasonNoHonestRephrase
			return res
		}
		out := strings.TrimSpace(raw)

		lastViol = verifyRephraseGrounding(bullet, allowedProper, sourceNums, out)
		if len(lastViol) == 0 {
			r := out
			res.Rephrase = &r
			res.SourceBullet = bullet
			return res
		}

		s.log.WarnContext(ctx, "keyword: rejected rephrase generation (grounding violation)",
			"term", term.Term,
			"attempt", attempt+1,
			"violations", strings.Join(lastViol, "; "),
			"output", out)
	}

	res.Reason = ReasonNoHonestRephrase
	return res
}

func buildRephrasePrompt(term domain.DiffTerm, sourceBullet string, priorViolations []string) string {
	want := term.Term
	if term.Canonical != "" {
		want = term.Canonical
	}
	var b strings.Builder
	b.WriteString("Reframe the candidate's EXISTING resume bullet so that it honestly surfaces the target skill/term the job wants.\n\n")
	b.WriteString("STRICT RULES:\n")
	b.WriteString("- Rephrase ONLY the experience shown in the source bullet below.\n")
	b.WriteString("- Do NOT add any skill, technology, employer, job title, date, or metric that is not already in the source bullet.\n")
	b.WriteString("- If the source bullet does not genuinely support the target term, do not stretch it — return the bullet unchanged.\n")
	b.WriteString("- Output the single reframed bullet as plain text. No preamble, no quotes, no bullet marker.\n\n")
	b.WriteString("TARGET TERM: " + want + "\n\n")
	b.WriteString("SOURCE BULLET:\n" + sourceBullet + "\n")
	if len(priorViolations) > 0 {
		b.WriteString("\nYour previous attempt violated the no-invention rule:\n- ")
		b.WriteString(strings.Join(priorViolations, "\n- "))
		b.WriteString("\nRegenerate using only what the source bullet contains.\n")
	}
	return b.String()
}

var numberRe = regexp.MustCompile(`\d+(?:[.,]\d+)*%?`)

func verifyRephraseGrounding(sourceBullet string, allowedProper, sourceNums map[string]bool, rephrase string) []string {
	if strings.TrimSpace(rephrase) == "" {
		return []string{"empty rephrase"}
	}
	var violations []string

	for _, p := range properNounsInText(rephrase) {
		if !allowedProper[domain.LowerASCII(p)] {
			violations = append(violations, fmt.Sprintf("proper noun / technology %q not in profile", p))
		}
	}
	for _, n := range numberRe.FindAllString(rephrase, -1) {
		if !sourceNums[normNumber(n)] {
			violations = append(violations, fmt.Sprintf("metric/number %q not in source bullet", n))
		}
	}
	return violations
}

func properNounsInText(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, sentence := range splitSentences(s) {
		tokens := domain.TokenRe.FindAllString(sentence, -1)
		for i, tok := range tokens {
			if !looksProper(tok) {
				continue
			}
			if i == 0 && isPlainCapitalized(tok) {
				continue
			}
			key := domain.LowerASCII(tok)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, tok)
		}
	}
	return out
}

var sentenceSplitRe = regexp.MustCompile(`[.!?]\s+`)

func splitSentences(s string) []string { return sentenceSplitRe.Split(s, -1) }

func looksProper(tok string) bool {
	for _, r := range tok {
		if r >= 'A' && r <= 'Z' {
			return true
		}
	}
	return false
}

func isPlainCapitalized(tok string) bool {
	if tok == "" {
		return false
	}
	r := []rune(tok)
	if r[0] < 'A' || r[0] > 'Z' {
		return false
	}
	for _, c := range r[1:] {
		if c >= 'A' && c <= 'Z' {
			return false
		}
	}
	return true
}

func properNounSet(bullets []string) map[string]bool {
	set := map[string]bool{}
	for _, b := range bullets {
		for _, w := range domain.TokenRe.FindAllString(b, -1) {
			set[domain.LowerASCII(w)] = true
		}
	}
	return set
}

func numberSet(text string) map[string]bool {
	set := map[string]bool{}
	for _, n := range numberRe.FindAllString(text, -1) {
		set[normNumber(n)] = true
	}
	return set
}

func normNumber(n string) string {
	n = strings.TrimSuffix(n, "%")
	return strings.ReplaceAll(n, ",", "")
}

type LexicalEvidenceFinder struct{}

func (LexicalEvidenceFinder) FindEvidence(term domain.DiffTerm, bullets []string) (string, bool) {
	wants := termStems(term)
	if len(wants) == 0 {
		return "", false
	}
	for _, b := range bullets {
		for _, w := range domain.FilterStopwords(domain.TokenRe.FindAllString(b, -1)) {
			if wants[domain.Stem(w)] {
				return b, true
			}
		}
	}
	return "", false
}

func termStems(term domain.DiffTerm) map[string]bool {
	src := term.Canonical
	if src == "" {
		src = term.Term
	}
	out := map[string]bool{}
	for _, w := range domain.FilterStopwords(domain.TokenRe.FindAllString(src, -1)) {
		if s := domain.Stem(w); s != "" {
			out[s] = true
		}
	}
	return out
}
