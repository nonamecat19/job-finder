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

// The single inference path (044 FR-001, SC-001; contracts/embeddings.md E5-1;
// tasks.md T017).
//
// 044's whole premise is that there is exactly one way out of this codebase to
// an LLM or embedding provider: internal/platform/llm/infrastructure/gateway,
// which speaks LiteLLM's OpenAI-compatible wire format over net/http. Every
// other package reaches inference only through domain.Provider (an interface;
// no wire knowledge) or through application.Router (which resolves to a
// domain.Provider). This test makes that structural rather than a review
// habit, mirroring internal/toolfence_test.go's three-part mechanism:
//
//  1. **Discovery** — walk internal/ for packages whose non-test source
//     directly imports BOTH net/http and the LLM domain package
//     (github.com/job-finder/api/internal/platform/llm/domain). That
//     combination is the fingerprint of a package that can itself construct
//     an HTTP request understood as an llm.domain.Provider implementation —
//     i.e. an inference adapter, not a caller of one.
//  2. **Declaration** — an explicit list below. The two sets must be equal,
//     so a new adapter cannot appear without a decision, and a listed one
//     that stops being an adapter cannot linger as a stale exemption.
//  3. **Adapter-import check** — every internal/ package's direct imports,
//     other than the declared composition point(s), so a business-logic
//     package cannot import an adapter package directly instead of going
//     through domain.Provider / application.Router. Unlike toolfence_test.go's
//     transitive `go list -deps` closure, this one stays at direct imports —
//     see the comment on TestNoPackageReachesAnAdapterExceptGateway for why
//     that is the right scope here rather than a shortcut.
//
// # What this cannot catch — stated because a fence whose limits are
// undocumented gets trusted for things it does not do (same acknowledgement
// as toolfence_test.go)
//
//  1. **A hand-built outbound request that never imports the domain
//     package.** net/http cannot be forbidden outright — retrieval, the
//     scraper and observability all use it legitimately for unrelated
//     reasons. A package that opened a raw connection to an LLM's HTTP API
//     without importing internal/platform/llm/domain at all would not be
//     caught by the fingerprint in part 1, though it would still have no way
//     to satisfy domain.Provider without that import.
//  2. **The composition root.** cmd/server/compose.go legitimately
//     constructs adapters and wires them into routers; it is outside
//     internal/ (this file's walk root, matching toolfence_test.go) and is
//     not evaluated here.
//
// Enforcement: intended for the `go-test` job of .github/workflows/api-ci.yml
// alongside toolfence_test.go.

const llmDomainPkg = "github.com/job-finder/api/internal/platform/llm/domain"

// declaredInferencePackages is the explicit list of packages allowed to speak
// the wire protocol to an inference endpoint directly, in the style of
// toolfence_test.go's declaredToolPackages. Adding one here is the decision
// point; discovery below makes it impossible to skip.
var declaredInferencePackages = map[string]bool{
	"github.com/job-finder/api/internal/platform/llm/infrastructure/gateway": true,
}

// forbiddenAdapterPrefixes are inference-adapter packages that only a small,
// declared set of composition points (declaredAdapterImporters) may import
// directly. After T023 deletes infrastructure/ollama, this set is empty —
// kept as a slice rather than removed outright so a future second adapter has
// somewhere obvious to register.
var forbiddenAdapterPrefixes = []string{}

// declaredAdapterImporters are the composition points allowed to directly
// import a forbidden adapter package in order to construct it and hand it to
// a Router as a domain.Provider. Empty since T024 deleted llm.NewOllama, the
// facade's only reason to import an adapter directly.
var declaredAdapterImporters = map[string]bool{}

func TestOnlyGatewayReachesInferenceOverHTTP(t *testing.T) {
	discovered := discoverInferencePackages(t)

	// Part 2: the two sets must be equal, in both directions.
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

// TestNoPackageReachesAnAdapterExceptGateway checks DIRECT imports of a
// forbidden adapter package, not the full transitive closure toolfence_test.go
// computes with `go list -deps`.
//
// That is a deliberate, stated departure from mirroring toolfence exactly, for
// a reason specific to this fence rather than that one: toolfence's transitive
// check exists because a tool handler is a *closure* that can call into a
// capability its own package never imports (see toolfence_test.go's "closure
// over an already-injected capability"). There is no equivalent smuggling
// path here — calling an adapter constructor requires importing that
// adapter's package in the calling file, full stop. A transitive check would
// instead just flag every package that happens to depend on the shared
// internal/platform/llm facade (essentially the whole service layer, since it
// depends on internal/queue), which is true but not informative: it is the
// facade's own direct import that matters, and that is exactly what this
// check inspects.
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

// discoverInferencePackages walks internal/ and returns every package whose
// non-test source imports both net/http and the LLM domain package.
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
