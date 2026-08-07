package infrastructure

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
)

const fallbackDocumentsDir = "./data/documents"

func ensureOutDir(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		if errors.Is(err, os.ErrPermission) && dir != fallbackDocumentsDir {
			slog.Warn("generation: outDir not writable, falling back to repo-relative dir",
				"outDir", dir, "fallback", fallbackDocumentsDir, "error", err)
			if fbErr := os.MkdirAll(fallbackDocumentsDir, 0o755); fbErr != nil {
				return "", fbErr
			}
			absDir, absErr := filepath.Abs(fallbackDocumentsDir)
			if absErr != nil {
				absDir = fallbackDocumentsDir
			}
			return absDir, nil
		}
		return "", err
	}
	absDir, absErr := filepath.Abs(dir)
	if absErr != nil {
		absDir = dir
	}
	return absDir, nil
}
