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

const llmDomainPkg = "github.com/job-finder/api/internal/platform/llm/domain"

var declaredInferencePackages = map[string]bool{
	"github.com/job-finder/api/internal/platform/llm/infrastructure/gateway": true,
}

var forbiddenAdapterPrefixes = []string{}

var declaredAdapterImporters = map[string]bool{}

func TestOnlyGatewayReachesInferenceOverHTTP(t *testing.T) {
	discovered := discoverInferencePackages(t)

	for pkg := range discovered {
		if !declaredInferencePackages[pkg] {
			t.Errorf("package %s imports both net/http and %s, meaning it can speak the wire "+
				"protocol to an inference endpoint directly, but is not declared in "+
				"declaredInferencePackages; FR-001 requires exactly one path (contracts/embeddings.md E5-1)",
				pkg, llmDomainPkg)
		}
	}
	for pkg := range declaredInferencePackages {
		if !discovered[pkg] {
			t.Errorf("package %s is declared the sole inference path but no longer imports both "+
				"net/http and %s; remove it from declaredInferencePackages rather than leaving a "+
				"stale exemption", pkg, llmDomainPkg)
		}
	}
}

func TestNoPackageReachesAnAdapterExceptGateway(t *testing.T) {
	root := repoInternalRoot(t)

	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		rel, rerr := filepath.Rel(root, filepath.Dir(path))
		if rerr != nil {
			return rerr
		}
		pkg := "github.com/job-finder/api/internal/" + filepath.ToSlash(rel)
		if declaredInferencePackages[pkg] || declaredAdapterImporters[pkg] {
			return nil
		}
		for _, spec := range file.Imports {
			imported, uerr := strconv.Unquote(spec.Path.Value)
			if uerr != nil {
				return uerr
			}
			for _, forbidden := range forbiddenAdapterPrefixes {
				if imported == forbidden {
					t.Errorf("package %s imports %s directly (forbidden adapter).\n"+
						"Business logic must reach inference through domain.Provider / application.Router, "+
						"never by importing an adapter package directly (FR-001). If this is meant to be a "+
						"composition point, add it to declaredAdapterImporters as a decision, not a discovery.",
						pkg, imported)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
}

func discoverInferencePackages(t *testing.T) map[string]bool {
	t.Helper()
	root := repoInternalRoot(t)

	found := map[string]bool{}
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}
		hasHTTP, hasDomain := false, false
		for _, spec := range file.Imports {
			imported, uerr := strconv.Unquote(spec.Path.Value)
			if uerr != nil {
				return uerr
			}
			if imported == "net/http" {
				hasHTTP = true
			}
			if imported == llmDomainPkg {
				hasDomain = true
			}
		}
		if hasHTTP && hasDomain {
			rel, rerr := filepath.Rel(root, filepath.Dir(path))
			if rerr != nil {
				return rerr
			}
			found["github.com/job-finder/api/internal/"+filepath.ToSlash(rel)] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/: %v", err)
	}
	return found
}

func repoInternalRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve internal/: %v", err)
	}
	return root
}
