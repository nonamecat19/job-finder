package domain

import (
	"strings"
	"testing"
)

const baseJD = `We are looking for a Senior Backend Engineer to join our growing team.
You will design and build scalable services in Go, work closely with
product and design, and mentor junior engineers. Requirements: 5+ years
of backend experience, strong knowledge of distributed systems, and
excellent communication skills. We offer competitive salary, remote work,
and a generous benefits package including health insurance and paid time
off for the right candidate who joins our mission-driven company.`

func TestSimilarText_ReformattedCopyWithinThreshold(t *testing.T) {
	reformatted := "<p>" + strings.ReplaceAll(baseJD, "\n", "  ") + "</p>\n\n  extra   spacing "

	if !SimilarText(baseJD, reformatted) {
		t.Errorf("expected reformatted copy to be similar; hamming distance = %d",
			HammingDistance(Hash(baseJD), Hash(reformatted)))
	}
}

func TestSimilarText_DifferentJDsAreNotSimilar(t *testing.T) {
	other := `We need a Marketing Coordinator to manage our social media presence,
plan events, and coordinate with external agencies. The ideal candidate has
2+ years of marketing experience, a degree in communications, and is
comfortable with Adobe Creative Suite and basic analytics tools. This is an
in-office role based in our downtown location with standard business hours.`

	if SimilarText(baseJD, other) {
		t.Errorf("expected unrelated JDs to NOT be similar; hamming distance = %d",
			HammingDistance(Hash(baseJD), Hash(other)))
	}
}

func TestHash_CyrillicSurvivesNormalization(t *testing.T) {
	cyrillic := `Ми шукаємо досвідченого розробника Go для нашої команди. Ви будете
проєктувати та створювати масштабовані сервіси, тісно співпрацювати з
продуктовою командою та наставляти молодших інженерів. Вимоги: 5+ років
досвіду розробки бекенду, глибокі знання розподілених систем.`

	reformatted := strings.ToUpper(cyrillic) + "\n\n  "

	h1 := Hash(cyrillic)
	h2 := Hash(reformatted)
	if h1 == 0 || h2 == 0 {
		t.Fatal("expected non-zero hash for Cyrillic text")
	}
	if HammingDistance(h1, h2) > CrossBoardSimilarityThreshold {
		t.Errorf("expected case-insensitive Cyrillic re-wrap to be similar; distance = %d",
			HammingDistance(h1, h2))
	}
}

func TestHash_EmptyTextIsZero(t *testing.T) {
	if Hash("") != 0 {
		t.Error("expected empty text to hash to 0")
	}
	if Hash("   \n\t  ") != 0 {
		t.Error("expected whitespace-only text to hash to 0")
	}
}
