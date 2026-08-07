package domain

import (
	"hash/fnv"
	"math/bits"
	"regexp"
	"strings"
)

const MinHashableDescriptionLen = 120

const CrossBoardSimilarityThreshold = 3

var (
	htmlTagRe    = regexp.MustCompile(`<[^>]*>`)
	whitespaceRe = regexp.MustCompile(`\s+`)
	wordRe       = regexp.MustCompile(`[\p{L}\p{N}]+`)
)

func normalize(text string) string {
	t := htmlTagRe.ReplaceAllString(text, " ")
	t = strings.ToLower(t)
	t = whitespaceRe.ReplaceAllString(t, " ")
	return strings.TrimSpace(t)
}

const shingleSize = 3

func shingles(normalized string) []string {
	words := wordRe.FindAllString(normalized, -1)
	if len(words) == 0 {
		return nil
	}
	if len(words) < shingleSize {
		return []string{strings.Join(words, " ")}
	}
	out := make([]string, 0, len(words)-shingleSize+1)
	for i := 0; i+shingleSize <= len(words); i++ {
		out = append(out, strings.Join(words[i:i+shingleSize], " "))
	}
	return out
}

func Hash(text string) uint64 {
	sh := shingles(normalize(text))
	if len(sh) == 0 {
		return 0
	}
	var counts [64]int
	for _, s := range sh {
		h := fnv.New64a()
		_, _ = h.Write([]byte(s))
		v := h.Sum64()
		for bit := 0; bit < 64; bit++ {
			if v&(1<<uint(bit)) != 0 {
				counts[bit]++
			} else {
				counts[bit]--
			}
		}
	}
	var out uint64
	for bit := 0; bit < 64; bit++ {
		if counts[bit] > 0 {
			out |= 1 << uint(bit)
		}
	}
	return out
}

func HammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

func SimilarText(a, b string) bool {
	return HammingDistance(Hash(a), Hash(b)) <= CrossBoardSimilarityThreshold
}
