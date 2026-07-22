package outreach

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenSendTokens are substrings that would indicate a send/transmit
// path exists in this package — the hard product line is "012 generates,
// the user sends" (spec.md, SC-001). This test greps the package's own
// non-test source so a later change cannot quietly add one.
var forbiddenSendTokens = []string{
	"net/smtp",
	"mailto:",
	".Send(",
	"SendMail",
	"smtp.",
}

// TestNoSendPath is the automated form of plan 012 §4.1: zero SMTP,
// mailto:, or delivery-integration code paths anywhere in this package.
func TestNoSendPath(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasSuffix(e.Name(), "_test.go") || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(".", e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		content := string(data)
		for _, tok := range forbiddenSendTokens {
			if strings.Contains(content, tok) {
				t.Errorf("%s contains forbidden send-path token %q — outreach is draft-only (Principle I)", e.Name(), tok)
			}
		}
	}
}
