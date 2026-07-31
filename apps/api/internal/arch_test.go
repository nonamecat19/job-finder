package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const chiImport = "github.com/go-chi/chi/v5"

// exemptDirs are the two packages allowed to import chi from outside an
// interfaces/ package, each for a stated reason rather than because it was
// convenient:
//
//	httpapi  — is the router itself; chi is its subject matter.
//	httpx    — must not import chi at all. The depguard rule
//	           `httpx-stays-a-leaf` already enforces that, so exempting it here
//	           only keeps the two mechanisms from reporting one violation twice.
//	health   — cross-cutting readiness, owned by no feature, so it has no
//	           feature module to hold an interfaces/ layer (spec T039/T040).
//	testutil — builds requests against a router for other packages' tests. It
//	           serves nothing.
var exemptDirs = map[string]bool{
	"httpapi":  true,
	"httpx":    true,
	"health":   true,
	"testutil": true,
}

// TestHandlersLiveInInterfaces covers the half of FR-011 that depguard
// structurally cannot: depguard matches import paths, not file locations, so a
// handler placed inside its feature module but outside interfaces/http passes
// every import rule while still recreating the arrangement this feature
// removed (contracts/depguard.md §2).
//
// Limitation: keyed on the chi import, so a handler written against net/http
// alone would not be caught. Accepted — a handler that never touches the
// router cannot be registered.
func TestHandlersLiveInInterfaces(t *testing.T) {
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve internal/: %v", err)
	}

	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		feature, _, _ := strings.Cut(rel, string(filepath.Separator))
		if exemptDirs[feature] {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		for _, spec := range file.Imports {
			imported, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			if imported != chiImport {
				continue
			}
			if strings.Contains(filepath.ToSlash(rel), "/interfaces/") {
				continue
			}
			t.Errorf(
				"internal/%s imports chi outside an interfaces/ package.\n"+
					"HTTP handlers belong in internal/%s/interfaces/http/.",
				filepath.ToSlash(rel), feature,
			)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
}
