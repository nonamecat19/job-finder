package outreach

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var forbiddenSendTokens = []string{
	"net/smtp",
	"mailto:",
	".Send(",
	"SendMail",
	"smtp.",
}

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
