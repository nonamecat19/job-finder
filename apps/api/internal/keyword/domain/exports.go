package domain

// This file re-exports a handful of internal text-processing primitives that
// the application package's rephrase grounding-verification logic also needs
// (tokenization, ASCII lowercasing, stopword filtering, stemming). They stay
// implemented as the package-private functions above/in helpers.go/service.go
// — these are thin, deliberately named wrappers so cross-package callers
// don't reach into internal names.

// TokenRe is the word tokenizer regex used to split a phrase into tokens,
// keeping hyphenated compounds.
var TokenRe = tokenRe

// LowerASCII lowercases ASCII letters only, leaving other scripts untouched.
func LowerASCII(s string) string { return lowerASCII(s) }

// FilterStopwords drops connector/filler words from a token list.
func FilterStopwords(tokens []string) []string { return filterStopwords(tokens) }

// Stem runs the package's small deterministic English stemmer.
func Stem(s string) string { return stem(s) }
