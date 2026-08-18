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

const toolloopPkg = "github.com/job-finder/api/internal/platform/llm/application/toolloop"

var declaredToolPackages = map[string]bool{}

var forbiddenPrefixes = []string{
	"github.com/job-finder/api/internal/notifier",
	"github.com/job-finder/api/internal/outreach",
	"github.com/job-finder/api/internal/postage",
	"github.com/job-finder/api/internal/applications",
	"github.com/job-finder/api/internal/retrieval",
	"github.com/job-finder/api/internal/jobsources",
	"github.com/job-finder/api/internal/scraping",
	"github.com/nonamecat19/job-scraper/retrieval",
	"github.com/nonamecat19/job-scraper/adapters",
	"github.com/nonamecat19/job-scraper/session",
}

func TestToolPackagesAreReadOnly(t *testing.T) {
	discovered := discoverToolPackages(t)

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
