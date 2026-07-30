package domain

import (
	"embed"
	"encoding/json"
)

//go:embed synonyms.json
var synonymsFS embed.FS

// synonymMap is the loaded canonical->aliases configuration. It is loaded once
// at package init (LoadSynonyms) so the extractor has it available without I/O
// at request time; see spec 008-1 §2.2 (config file loaded at startup,
// extendable without schema changes).
var synonymMap = defaultSynonymMap()

// canonicalByAlias indexes every alias back to its canonical form for O(1)
// resolution during normalization.
var canonicalByAlias = buildAliasIndex(synonymMap)

// SynonymConfig mirrors synonyms.json.
type SynonymConfig struct {
	Synonyms map[string][]string `json:"synonyms"`
}

// defaultSynonymMap returns the embedded synonym configuration. It is the
// fallback used when LoadSynonyms is not called explicitly.
func defaultSynonymMap() map[string][]string {
	cfg := mustLoadEmbedded()
	return cfg.Synonyms
}

func mustLoadEmbedded() SynonymConfig {
	data, err := synonymsFS.ReadFile("synonyms.json")
	if err != nil {
		// Embedding is compiled in; a failure here is a build/packaging bug.
		panic("keyword: cannot read embedded synonyms.json: " + err.Error())
	}
	var cfg SynonymConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		panic("keyword: cannot parse embedded synonyms.json: " + err.Error())
	}
	return cfg
}

// LoadSynonyms (re)loads the synonym map from the embedded config. Kept as the
// documented startup hook (spec 008-1 §2.2); the embedded data is also the
// default so callers that skip it still get a working extractor.
func LoadSynonyms() {
	synonymMap = defaultSynonymMap()
	canonicalByAlias = buildAliasIndex(synonymMap)
}

// buildAliasIndex inverts canonical->aliases into alias->canonical,
// lowercased for case-insensitive lookup.
func buildAliasIndex(m map[string][]string) map[string]string {
	idx := make(map[string]string, len(m))
	for canonical, aliases := range m {
		idx[lowerASCII(canonical)] = canonical
		for _, a := range aliases {
			idx[lowerASCII(a)] = canonical
		}
	}
	return idx
}

// resolveAlias returns the canonical form for a raw term if one is known,
// otherwise the original (cleaned) term.
func resolveAlias(raw string) string {
	if c, ok := canonicalByAlias[lowerASCII(raw)]; ok {
		return c
	}
	return raw
}
