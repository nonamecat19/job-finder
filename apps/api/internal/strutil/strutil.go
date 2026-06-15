// Package strutil holds tiny string helpers shared across service packages.
package strutil

// Truncate returns s capped to at most n runes. Unlike a raw byte slice
// (s[:n]), this never splits a multi-byte UTF-8 sequence, which would
// otherwise corrupt the last character (or produce invalid UTF-8 entirely)
// whenever the cut point landed inside a non-ASCII rune — a real risk here
// since job descriptions/HTML snippets routinely contain non-ASCII text
// (e.g. Cyrillic from the djinni/dou adapters, or "–"/smart quotes anywhere
// else). n is interpreted as a rune count, which is close enough to the
// original TS `.slice(0, n)` (UTF-16 code units) for every call site here —
// all of them just cap a prompt/snippet to an approximate size budget.
func Truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}
