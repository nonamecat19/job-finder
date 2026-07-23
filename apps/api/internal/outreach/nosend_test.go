package outreach

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// forbiddenSendTokens are substrings that would indicate a send/transmit
// path exists in this bounded context — the hard product line is "012
// generates, the user sends" (spec.md, SC-001). This test walks the
// package's own non-test source (including domain/ and application/) so a
// later change cannot quietly add one.
var forbiddenSendTokens = []string{
	"net/smtp",
	"mailto:",
	".Send(",
	"SendMail",
	"smtp.",
}

// TestNoSendPath is the automated form of plan 012 §4.1: zero SMTP,
// mailto:, or delivery-integration code paths anywhere in this bounded
// context.
func TestNoSendPath(t *testing.T) {
	err := filepath.WalkDir(".", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasSuffix(name, "_test.go") || !strings.HasSuffix(name, ".go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		for _, tok := range forbiddenSendTokens {
			if strings.Contains(content, tok) {
				t.Errorf("%s contains forbidden send-path token %q — outreach is draft-only (Principle I)", path, tok)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk outreach package: %v", err)
	}
}
