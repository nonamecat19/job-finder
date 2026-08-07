package domain

import (
	"embed"
	"encoding/json"
)

//go:embed synonyms.json
var synonymsFS embed.FS

var synonymMap = defaultSynonymMap()

var canonicalByAlias = buildAliasIndex(synonymMap)

type SynonymConfig struct {
	Synonyms map[string][]string `json:"synonyms"`
}

func defaultSynonymMap() map[string][]string {
	cfg := mustLoadEmbedded()
	return cfg.Synonyms
}

func mustLoadEmbedded() SynonymConfig {
	data, err := synonymsFS.ReadFile("synonyms.json")
	if err != nil {
		panic("keyword: cannot read embedded synonyms.json: " + err.Error())
	}
	var cfg SynonymConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		panic("keyword: cannot parse embedded synonyms.json: " + err.Error())
	}
	return cfg
}

func LoadSynonyms() {
	synonymMap = defaultSynonymMap()
	canonicalByAlias = buildAliasIndex(synonymMap)
}

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

func resolveAlias(raw string) string {
	if c, ok := canonicalByAlias[lowerASCII(raw)]; ok {
		return c
	}
	return raw
}
