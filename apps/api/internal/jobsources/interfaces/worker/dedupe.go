package worker

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// CanonicalURL strips the query string and trailing slashes from a job URL,
// matching `newJob.url.split('?')[0].replace(/\/+$/, ”)` in
// ingestion.processor.ts:74 exactly.
func CanonicalURL(rawURL string) string {
	return strings.TrimRight(strings.SplitN(rawURL, "?", 2)[0], "/")
}

// DedupeKey computes sha256(lower(company)|lower(title)|canonicalUrl) —
// must match ingestion.processor.ts:74 byte-for-byte or duplicate jobs flood in.
func DedupeKey(company, title, rawURL string) string {
	canonical := CanonicalURL(rawURL)
	sum := sha256.Sum256([]byte(strings.ToLower(company) + "|" + strings.ToLower(title) + "|" + canonical))
	return hex.EncodeToString(sum[:])
}
