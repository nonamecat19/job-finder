package ingestion

import "testing"

// TestDedupeKey_MatchesNodeOutput was produced by running the actual Node
// logic from ingestion.processor.ts:74 via:
//
//	node -e 'const { createHash } = require("crypto"); ...'
//
// with company="Acme Corp", title="Senior Go Engineer",
// url="https://example.com/jobs/123/?utm=abc&ref=xyz". This proves the Go
// port matches Node's hash byte-for-byte.
func TestDedupeKey_MatchesNodeOutput(t *testing.T) {
	got := DedupeKey("Acme Corp", "Senior Go Engineer", "https://example.com/jobs/123/?utm=abc&ref=xyz")
	want := "b139cd3b60b3091dfff8b71ed55d484c58cb7053fcafe4b492ddb93cf421fe4f"
	if got != want {
		t.Fatalf("dedupe key mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestCanonicalURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://example.com/jobs/123/?utm=abc&ref=xyz", "https://example.com/jobs/123"},
		{"https://example.com/jobs/123", "https://example.com/jobs/123"},
		{"https://example.com/jobs/123///", "https://example.com/jobs/123"},
		{"https://example.com/jobs/123?", "https://example.com/jobs/123"},
	}
	for _, c := range cases {
		if got := CanonicalURL(c.in); got != c.want {
			t.Errorf("CanonicalURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDedupeKey_CaseInsensitive(t *testing.T) {
	a := DedupeKey("Acme Corp", "Senior Go Engineer", "https://example.com/jobs/123")
	b := DedupeKey("ACME CORP", "senior go engineer", "https://example.com/jobs/123")
	if a != b {
		t.Fatalf("expected case-insensitive dedupe keys to match: %s != %s", a, b)
	}
}
