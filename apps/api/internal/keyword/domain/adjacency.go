package domain

import (
	"embed"
	"encoding/json"
	"log/slog"
	"sort"
)

//go:embed adjacency.json
var adjacencyFS embed.FS

type Proximity string

const (
	ProximityClose    Proximity = "close"
	ProximityModerate Proximity = "moderate"
	ProximityDistant  Proximity = "distant"
)

func proximityRank(p Proximity) int {
	switch p {
	case ProximityClose:
		return 0
	case ProximityModerate:
		return 1
	case ProximityDistant:
		return 2
	default:
		return 3
	}
}

type Adjacency struct {
	Term      string    `json:"term"`
	Proximity Proximity `json:"proximity"`
	Symmetric *bool     `json:"symmetric,omitempty"`
}

type AdjacencyEntry struct {
	Term     string      `json:"term"`
	Context  string      `json:"context"`
	Adjacent []Adjacency `json:"adjacent"`
}

type AdjacencyConfig struct {
	Version int              `json:"version"`
	Entries []AdjacencyEntry `json:"entries"`
}

type adjacencyIndexT map[string]map[string][]Adjacency

var (
	adjacencyConfig = mustLoadEmbeddedAdjacency()
	adjacencyIndex  = buildAdjacencyIndex(adjacencyConfig)
)

func mustLoadEmbeddedAdjacency() AdjacencyConfig {
	data, err := adjacencyFS.ReadFile("adjacency.json")
	if err != nil {
		panic("keyword: cannot read embedded adjacency.json: " + err.Error())
	}
	var cfg AdjacencyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		panic("keyword: cannot parse embedded adjacency.json: " + err.Error())
	}
	return cfg
}

func LoadAdjacencyMap() {
	adjacencyConfig = mustLoadEmbeddedAdjacency()
	adjacencyIndex = buildAdjacencyIndex(adjacencyConfig)
	slog.Info("keyword: loaded adjacency map",
		"version", adjacencyConfig.Version, "entries", len(adjacencyConfig.Entries))
}

func (a Adjacency) isSymmetric() bool {
	return a.Symmetric == nil || *a.Symmetric
}

func buildAdjacencyIndex(cfg AdjacencyConfig) adjacencyIndexT {
	idx := adjacencyIndexT{}
	addEdge := func(ctx, from string, edge Adjacency) {
		ctx = lowerASCII(ctx)
		if ctx == "" {
			ctx = "any"
		}
		if idx[ctx] == nil {
			idx[ctx] = map[string][]Adjacency{}
		}
		key := lowerASCII(from)
		idx[ctx][key] = append(idx[ctx][key], edge)
	}
	for _, e := range cfg.Entries {
		for _, a := range e.Adjacent {
			addEdge(e.Context, e.Term, a)
			if a.isSymmetric() {
				addEdge(e.Context, a.Term, Adjacency{
					Term:      e.Term,
					Proximity: a.Proximity,
					Symmetric: a.Symmetric,
				})
			}
		}
	}
	return idx
}

func Adjacent(term, context string) []Adjacency {
	merged := map[string]Adjacency{}
	collect := func(ctx string) {
		m, ok := adjacencyIndex[lowerASCII(ctx)]
		if !ok {
			return
		}
		for _, a := range m[lowerASCII(term)] {
			key := lowerASCII(a.Term)
			if prev, ok := merged[key]; !ok || proximityRank(a.Proximity) < proximityRank(prev.Proximity) {
				merged[key] = a
			}
		}
	}
	if context != "" && lowerASCII(context) != "any" {
		collect(context)
	}
	collect("any")

	out := make([]Adjacency, 0, len(merged))
	for _, a := range merged {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if proximityRank(out[i].Proximity) != proximityRank(out[j].Proximity) {
			return proximityRank(out[i].Proximity) < proximityRank(out[j].Proximity)
		}
		return out[i].Term < out[j].Term
	})
	return out
}

func AdjacencyMapVersion() int { return adjacencyConfig.Version }

func AdjacencyConfigForTest() AdjacencyConfig { return adjacencyConfig }
