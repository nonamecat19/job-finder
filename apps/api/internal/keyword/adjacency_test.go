package keyword_test

import (
	"testing"

	"github.com/job-finder/api/internal/keyword"
)

func hasAdjacent(adj []keyword.Adjacency, term string) (keyword.Adjacency, bool) {
	for _, a := range adj {
		if a.Term == term {
			return a, true
		}
	}
	return keyword.Adjacency{}, false
}

// TestAdjacencyLookup covers term+context resolution: context-specific entries
// plus the always-applicable "any" context, closest proximity first.
func TestAdjacencyLookup(t *testing.T) {
	tests := []struct {
		name      string
		term      string
		context   string
		wantTerm  string
		wantProx  keyword.Proximity
		wantFound bool
	}{
		{"postgres->mysql any", "Postgres", "any", "MySQL", keyword.ProximityClose, true},
		{"postgres->mysql empty context", "Postgres", "", "MySQL", keyword.ProximityClose, true},
		{"case-insensitive term", "postgres", "any", "SQLite", keyword.ProximityClose, true},
		{"rust->go only in systems context", "Rust", "systems", "Go", keyword.ProximityModerate, true},
		{"rust->go absent in any context", "Rust", "any", "Go", "", false},
		{"unknown term yields nothing", "COBOL", "any", "Fortran", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adj := keyword.Adjacent(tt.term, tt.context)
			got, found := hasAdjacent(adj, tt.wantTerm)
			if found != tt.wantFound {
				t.Fatalf("Adjacent(%q,%q) found %q = %v, want %v (got %v)",
					tt.term, tt.context, tt.wantTerm, found, tt.wantFound, adj)
			}
			if found && got.Proximity != tt.wantProx {
				t.Errorf("proximity = %q, want %q", got.Proximity, tt.wantProx)
			}
		})
	}
}

// TestAdjacencyOrderedByProximity asserts lookups return closest-first.
func TestAdjacencyOrderedByProximity(t *testing.T) {
	adj := keyword.Adjacent("Postgres", "any")
	if len(adj) < 2 {
		t.Fatalf("expected multiple adjacents, got %v", adj)
	}
	for i := 1; i < len(adj); i++ {
		if rank(adj[i-1].Proximity) > rank(adj[i].Proximity) {
			t.Errorf("not ordered closest-first at %d: %v", i, adj)
		}
	}
}

func rank(p keyword.Proximity) int {
	switch p {
	case keyword.ProximityClose:
		return 0
	case keyword.ProximityModerate:
		return 1
	case keyword.ProximityDistant:
		return 2
	default:
		return 3
	}
}

// TestAdjacencySymmetry is the core acceptance test: adjacency lookups are
// symmetric where the map says they are, and one-directional where it does
// not. For every declared edge, if the edge is symmetric the reverse lookup
// must return the source term at the same proximity; if the edge is explicitly
// asymmetric, the reverse lookup must NOT contain it.
func TestAdjacencySymmetry(t *testing.T) {
	cfg := keyword.AdjacencyConfigForTest()
	symmetricChecked, asymmetricChecked := 0, 0

	for _, entry := range cfg.Entries {
		ctx := entry.Context
		for _, edge := range entry.Adjacent {
			reverse := keyword.Adjacent(edge.Term, ctx)
			back, found := hasAdjacent(reverse, entry.Term)

			symmetric := edge.Symmetric == nil || *edge.Symmetric
			if symmetric {
				symmetricChecked++
				if !found {
					t.Errorf("symmetric edge %q<->%q (ctx=%q): reverse lookup missing %q; got %v",
						entry.Term, edge.Term, ctx, entry.Term, reverse)
					continue
				}
				if back.Proximity != edge.Proximity {
					t.Errorf("symmetric edge %q<->%q: reverse proximity %q != %q",
						entry.Term, edge.Term, back.Proximity, edge.Proximity)
				}
			} else {
				asymmetricChecked++
				if found {
					t.Errorf("asymmetric edge %q->%q (ctx=%q): reverse lookup unexpectedly contains %q",
						entry.Term, edge.Term, ctx, entry.Term)
				}
			}
		}
	}

	if symmetricChecked == 0 {
		t.Error("no symmetric edges exercised — fixture data is degenerate")
	}
	if asymmetricChecked == 0 {
		t.Error("no asymmetric edges exercised — add one to guard the negative branch")
	}
}

// TestAdjacencyMapVersion asserts the map is versioned (spec §2.6).
func TestAdjacencyMapVersion(t *testing.T) {
	if v := keyword.AdjacencyMapVersion(); v < 1 {
		t.Errorf("AdjacencyMapVersion() = %d, want >= 1", v)
	}
}

// TestLoadAdjacencyMapIdempotent asserts the startup hook can be re-run.
func TestLoadAdjacencyMap(t *testing.T) {
	keyword.LoadAdjacencyMap()
	if _, found := hasAdjacent(keyword.Adjacent("Docker", "any"), "Podman"); !found {
		t.Error("after reload, Docker->Podman adjacency missing")
	}
}
