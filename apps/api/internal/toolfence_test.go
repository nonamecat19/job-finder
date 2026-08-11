package internal_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The read-only fence (037 FR-008, SC-005, contracts §5).
//
// A tool loop hands a model the ability to choose what runs next. Constitution
// Principle I says the platform does not act on the user's behalf without them,
// and a lookup that can send an email, post an application or reach the open
// internet turns "the model decided to look something up" into "the model did
// something". The fence is what makes that structural rather than a review
// habit.
//
// It works in three parts, and the third is the one the cited precedents in
// this repository do not do:
//
//  1. **Discovery** — walk internal/ for packages that directly import the
//     toolloop package. A package that registers tools must import it, so this
//     finds all of them.
//  2. **Declaration** — an explicit list below. The two sets must be equal, so a
//     new tool package cannot appear without a decision, and a listed one that
//     stops registering tools cannot linger as a stale exemption.
//  3. **Transitive closure** — `go list -deps` on each discovered package, so a
//     lookup cannot reach a forbidden capability through a harmless-looking
//     helper. Direct-import scanning would miss exactly that.
//
// # What this cannot catch — all three, stated because a fence whose limits are
// undocumented gets trusted for things it does not do
//
//  1. **A hand-built outbound request.** A lookup that imports `net/http` and
//     constructs its own request reaches the internet without importing any
//     forbidden package. `net/http` cannot be forbidden — the whole platform
//     uses it.
//  2. **A closure over an already-injected capability.** This is the largest
//     hole. Handlers are closures; one defined inside a service that already
//     holds an outreach client can call it while the lookup's own package
//     imports nothing at all. No import-graph analysis can see that. It is why
//     a small, enumerated, reviewed toolset is a *required complementary
//     control* and not a reassurance.
//  3. **Packages, not call paths.** A package that imports a forbidden one for
//     an unrelated reason fails this test even if no lookup can reach it. That
//     direction is deliberate — it fails closed — but it means a failure here
//     is not automatically a live vulnerability.
//
// Enforcement: this runs in the `go-test` job of .github/workflows/api-ci.yml.

const toolloopPkg = "github.com/job-finder/api/internal/platform/llm/application/toolloop"

// declaredToolPackages is the explicit list of packages allowed to register
// tools, in the style of gateway_config_test.go's requestedGenerationGroups.
// Adding one here is the decision point; discovery below makes it impossible to
// skip.
var declaredToolPackages = map[string]bool{
	"github.com/job-finder/api/internal/salary/application": true,
}

// forbiddenPrefixes are capabilities no lookup may reach, directly or through
// any dependency.
//
// internal/retrieval and internal/jobsources are on this list for a specific
// reason: retrieval performs outbound HTTP, drives a headless browser and calls
// FlareSolverr. A lookup importing it would have reached the open internet from
// inside a model's decision loop while passing a fence that only checked for
// the obvious write paths (FR-008b).
//
// The engine and the site sources now live in the jobscraper library (043).
// The app packages of the same name are thin wrappers over them, so listing
// only the app paths would leave the fence bypassable by importing the library
// directly — the library paths carry the same capability and belong here for
// the same reason. The scraper moved back into the app when the library made
// its own internal, so internal/scraping is listed too.
//
// jobscraper/adapter, jobscraper/ports and jobscraper/model are deliberately
// absent: they are interface and struct declarations with no I/O, the same way
// internal/dto is not forbidden.
var forbiddenPrefixes = []string{
	"github.com/job-finder/api/internal/notifier",
	"github.com/job-finder/api/internal/outreach",
	"github.com/job-finder/api/internal/postage",
	"github.com/job-finder/api/internal/applications",
	"github.com/job-finder/api/internal/retrieval",
	"github.com/job-finder/api/internal/jobsources",
	"github.com/job-finder/api/internal/scraping",
	"github.com/nonamecat19/jobscraper/retrieval",
	"github.com/nonamecat19/jobscraper/adapters",
	"github.com/nonamecat19/jobscraper/session",
}

func TestToolPackagesAreReadOnly(t *testing.T) {
	discovered := discoverToolPackages(t)

	// Part 2: the two sets must be equal, in both directions.
	for pkg := range discovered {
		if !declaredToolPackages[pkg] {
			t.Errorf("package %s imports the tool loop but is not in declaredToolPackages; "+
				"a package that can register tools must be a decision, not a discovery", pkg)
		}
	}
	for pkg := range declaredToolPackages {
		if !discovered[pkg] {
			t.Errorf("package %s is declared as a tool package but no longer imports the tool loop; "+
				"remove it from declaredToolPackages rather than leaving a stale exemption", pkg)
		}
	}

	// Part 3: transitive closure of every discovered package.
	for pkg := range discovered {
		deps := transitiveDeps(t, pkg)
		for _, dep := range deps {
			for _, forbidden := range forbiddenPrefixes {
				if dep == forbidden || strings.HasPrefix(dep, forbidden+"/") {
					t.Errorf("tool package %s can reach %s (forbidden: %s).\n"+
						"A lookup must be a read. If this dependency is genuinely read-only, "+
						"split the read out into a package that does not carry the write path with it.",
						pkg, dep, forbidden)
				}
			}
		}
	}
}

// discoverToolPackages walks internal/ and returns every package with a direct
// import of the toolloop package.
func discoverToolPackages(t *testing.T) map[string]bool {
	t.Helper()
	root, err := filepath.Abs(".")
	if err != nil {
		t.Fatalf("resolve internal/: %v", err)
	}

	found := map[string]bool{}
	fset := token.NewFileSet()
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
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
		for _, spec := range file.Imports {
			imported, uerr := strconv.Unquote(spec.Path.Value)
			if uerr != nil {
				return uerr
			}
			if imported != toolloopPkg {
				continue
			}
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

// transitiveDeps resolves a package's full dependency closure with `go list`.
//
// `go list` through os/exec rather than golang.org/x/tools/go/packages: that
// module is not in go.mod, and adding a dependency to build the control that
// protects a constitutional principle would break the no-new-dependency rule
// with the control itself (SC-009, C5-1).
//
// A failure to invoke it **fails the test** (C5-1a). A fence that switches
// itself off in an unusual environment is a fence that is off.
func transitiveDeps(t *testing.T, pkg string) []string {
	t.Helper()
	out, err := exec.Command("go", "list", "-deps", pkg).Output()
	if err != nil {
		t.Fatalf("go list -deps %s failed: %v\n"+
			"This test fails rather than skipping on purpose: without go list there is no "+
			"transitive resolution, and a fence that cannot run is a fence that is not enforcing.", pkg, err)
	}
	deps := strings.Split(strings.TrimSpace(string(out)), "\n")
	sort.Strings(deps)
	return deps
}
