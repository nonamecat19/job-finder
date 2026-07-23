package infrastructure

import (
	"errors"
	"log/slog"
	"os"
	"path/filepath"
)

// fallbackDocumentsDir is used when the configured output directory can't be
// created. DOCUMENTS_DIR defaults to a repo-relative path (see
// config.go), but a stale env var, leftover shell export, or misconfigured
// deployment can still point it at the container-only "/data/documents"
// path — which a bare-metal/host process can't mkdir ("permission denied").
// Rather than hard-failing every resume/cover-letter generation job in that
// case, we degrade to this writable, repo-relative directory and log a
// warning so the misconfiguration is visible without blocking users.
const fallbackDocumentsDir = "./data/documents"

// ensureOutDir creates dir (and its parents), falling back to
// fallbackDocumentsDir if dir can't be created due to a permission error.
// Returns the directory that was actually created and is safe to write into.
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
