package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"

	"github.com/invopop/jsonschema"

	"github.com/job-finder/api/internal/events"
)

var schemas = []struct {
	name string
	typ  any
}{
	{"envelope", events.Envelope{}},
	{"failure", events.Failure{}},
	{"result", events.Result{}},
	{"usage", events.Usage{}},
	{"ingest_work", events.IngestWork{}},
	{"enrich_work", events.EnrichWork{}},
	{"match_work", events.MatchWork{}},
	{"generate_work", events.GenerateWork{}},
	{"salary_work", events.SalaryWork{}},
	{"ghost_work", events.GhostWork{}},
}

func main() {
	outDir := "internal/events/schema"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}

	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "contractsgen: %v\n", err)
		os.Exit(1)
	}

	reflector := &jsonschema.Reflector{
		DoNotReference: true,
		ExpandedStruct: true,
	}

	for _, s := range schemas {
		schema := reflector.ReflectFromType(reflect.TypeOf(s.typ))
		schema.ID = jsonschema.ID("https://jobfinder.internal/schema/" + s.name + ".schema.json")
		schema.Title = s.name

		out, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "contractsgen: marshal %s: %v\n", s.name, err)
			os.Exit(1)
		}
		out = append(out, '\n')

		path := filepath.Join(outDir, s.name+".schema.json")
		if err := os.WriteFile(path, out, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "contractsgen: write %s: %v\n", path, err)
			os.Exit(1)
		}
	}

	fmt.Printf("contractsgen: wrote %d schemas to %s\n", len(schemas), outDir)
}
