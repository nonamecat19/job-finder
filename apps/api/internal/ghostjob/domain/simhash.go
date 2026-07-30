package domain

import (
	"hash/fnv"
	"math/bits"
	"regexp"
	"strings"
)

// MinHashableDescriptionLen is the floor below which a description is
// treated as a teaser, not real content: too short to compare meaningfully,
// and a short teaser matches too many unrelated postings to be evidence
// (spec edge case). Below this, the cross-board-duplicate signal is
// reported unknown rather than a guess.
const MinHashableDescriptionLen = 120

// CrossBoardSimilarityThreshold is the maximum Hamming distance (out of 64
// bits) between two simhashes for their descriptions to be considered "the
// same JD, reformatted". Named + tunable without touching the SQL query,
// per tasks.md T015. Small enough that unrelated JDs practically never
// collide, generous enough to absorb a board's own re-wrapping/markup.
const CrossBoardSimilarityThreshold = 3

var (
	htmlTagRe    = regexp.MustCompile(`<[^>]*>`)
	whitespaceRe = regexp.MustCompile(`\s+`)
	wordRe       = regexp.MustCompile(`[\p{L}\p{N}]+`)
)

// normalize lowercases, strips HTML markup, and collapses whitespace so
// boards that reformat/re-wrap the same JD still compare equal-ish.
// strings.ToLower is Unicode-aware, so Cyrillic (and other non-ASCII) text
// survives unmangled.
func normalize(text string) string {
	t := htmlTagRe.ReplaceAllString(text, " ")
	t = strings.ToLower(t)
	t = whitespaceRe.ReplaceAllString(t, " ")
	return strings.TrimSpace(t)
}

// shingleSize is the word-gram length used to build shingles. 3-word
// shingles are short enough to survive minor rewording/reordering across
// boards while still being distinctive enough to avoid false positives.
const shingleSize = 3

// shingles splits normalized text into word tokens and returns overlapping
// word-grams of length shingleSize. Falls back to the whole token list as a
// single shingle when there aren't enough words.
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

// Hash computes a 64-bit simhash of text: normalize -> shingle -> hash each
// shingle (fnv-1a) -> weighted bit-vote -> collapse to the majority bit per
// position. Stdlib only, no new dependency (tasks.md T015).
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

// HammingDistance returns the number of differing bits between two hashes.
func HammingDistance(a, b uint64) int {
	return bits.OnesCount64(a ^ b)
}

// SimilarText reports whether two raw texts hash within
// CrossBoardSimilarityThreshold of each other.
func SimilarText(a, b string) bool {
	return HammingDistance(Hash(a), Hash(b)) <= CrossBoardSimilarityThreshold
}
