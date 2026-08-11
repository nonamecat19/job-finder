package main

import (
	"testing"

	jsretrieval "github.com/nonamecat19/job-scraper/retrieval"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/db"
)

// The paced transport is a package-level value in the library, configured once
// at composition. It was left unconfigured for several releases after the
// composition root moved out of platform.go, which silently pinned every host
// to the default rate: no advertised crawl delay was honoured (017-FR-009) and
// no operator override applied. These tests fail if that call goes missing
// again — a build-green regression the unit tests could not otherwise see.

func TestComposeRetrievalWiresTheRateResolver(t *testing.T) {
	restore := jsretrieval.DefaultTransport.RateResolver
	t.Cleanup(func() { jsretrieval.DefaultTransport.RateResolver = restore })
	jsretrieval.DefaultTransport.RateResolver = nil

	p := &Platform{Config: &config.Config{BrowserIdentityVersion: "chrome126"}, DB: &db.DB{}}
	if _, err := composeRetrieval(p); err != nil {
		t.Fatalf("composeRetrieval: %v", err)
	}

	if jsretrieval.DefaultTransport.RateResolver == nil {
		t.Fatal("composeRetrieval left DefaultTransport.RateResolver nil: crawl delays and overrides are ignored")
	}
}

func TestRateOverridesCarriesTheDjinniPin(t *testing.T) {
	if got := rateOverrides(&config.Config{}); len(got) != 0 {
		t.Fatalf("no override configured, want empty map, got %v", got)
	}

	got := rateOverrides(&config.Config{DjinniRateOverrideRPS: 0.5})
	if got["djinni.co"] != 0.5 {
		t.Fatalf("djinni.co rate = %v, want 0.5", got["djinni.co"])
	}
}

func TestConfiguredResolverAppliesTheOverride(t *testing.T) {
	restore := jsretrieval.DefaultTransport.RateResolver
	t.Cleanup(func() { jsretrieval.DefaultTransport.RateResolver = restore })

	p := &Platform{Config: &config.Config{BrowserIdentityVersion: "chrome126", DjinniRateOverrideRPS: 0.5}, DB: &db.DB{}}
	if _, err := composeRetrieval(p); err != nil {
		t.Fatalf("composeRetrieval: %v", err)
	}

	rps, source := jsretrieval.DefaultTransport.RateFor("djinni.co")
	if rps != 0.5 || source != "override" {
		t.Fatalf("djinni.co paced at %v (%s), want 0.5 (override)", rps, source)
	}
}
