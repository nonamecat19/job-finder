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

var exemptDirs = map[string]bool{
	"httpapi":  true,
	"httpx":    true,
	"health":   true,
	"testutil": true,
}

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
