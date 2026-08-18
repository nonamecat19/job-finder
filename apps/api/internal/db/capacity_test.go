package db

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/job-finder/api/internal/config"
	"github.com/job-finder/api/internal/queue"
)

func defaultConfig(t *testing.T) *config.Config {
	t.Helper()

	cfg, err := config.LoadNonAI()
	if err != nil {
		t.Fatalf("config.LoadNonAI: %v", err)
	}
	return cfg
}

func TestRequiredAtShippedDefaults(t *testing.T) {
	cfg := defaultConfig(t)
	policies, err := queue.PoliciesFromConfig(cfg)
	if err != nil {
		t.Fatalf("PoliciesFromConfig: %v", err)
	}

	budget := BudgetFromPolicies(policies, cfg.DBInteractiveReserve, cfg.DBServerMaxConns)

	if budget.WorkerSlots != 15 {
		t.Errorf("WorkerSlots = %d, want 15 (ingest 2 + match 3 + generate 3 + enrich 1 + salary 3 + ghost 3)", budget.WorkerSlots)
	}
	if got := budget.Required(); got != 25 {
		t.Errorf("Required() = %d, want 25 (15 workers + 2 background + 8 reserve)", got)
	}
}

func TestRequiredRisesWithCloudConcurrency(t *testing.T) {
	cfg := defaultConfig(t)
	base, err := queue.PoliciesFromConfig(cfg)
	if err != nil {
		t.Fatalf("PoliciesFromConfig: %v", err)
	}
	baseline := BudgetFromPolicies(base, cfg.DBInteractiveReserve, cfg.DBServerMaxConns).Required()

	cfg.AIConcurrencyCloud = 10
	raised, err := queue.PoliciesFromConfig(cfg)
	if err != nil {
		t.Fatalf("PoliciesFromConfig: %v", err)
	}
	got := BudgetFromPolicies(raised, cfg.DBInteractiveReserve, cfg.DBServerMaxConns).Required()

	if got <= baseline {
		t.Errorf("Required() = %d after raising AI_CONCURRENCY_CLOUD, want > baseline %d", got, baseline)
	}
	if want := baseline + 4*(10-3); got != want {
		t.Errorf("Required() = %d, want %d", got, want)
	}
}

func TestBudgetUsesPoolSizeCeiling(t *testing.T) {
	policies := []queue.TaskPolicy{
		{TaskType: "a", Concurrency: 9},
		{TaskType: "b", Concurrency: 4},
	}
	budget := BudgetFromPolicies(policies, 8, 100)

	if budget.WorkerSlots != 13 {
		t.Errorf("WorkerSlots = %d, want 13 (9 + 4)", budget.WorkerSlots)
	}
	if got := budget.Required(); got != 13+backgroundConnectionSlots+8 {
		t.Errorf("Required() = %d, want %d", got, 13+backgroundConnectionSlots+8)
	}
}

func TestBackgroundSlotsMatchesRunServers(t *testing.T) {
	const (
		connectionHolding = 2
		nonHolding        = 1
	)

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "../../cmd/server/servers.go", nil, 0)
	if err != nil {
		t.Fatalf("parse servers.go: %v", err)
	}

	var launched int
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "runServers" {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			if _, ok := n.(*ast.GoStmt); ok {
				launched++
			}
			return true
		})
	}

	const workerLoopAndHTTP = 2
	if want := connectionHolding + nonHolding + workerLoopAndHTTP; launched != want {
		t.Fatalf("runServers launches %d goroutines, expected %d. A goroutine was added or removed: "+
			"if it holds a database connection for the process lifetime, raise backgroundConnectionSlots (currently %d) "+
			"and update this test's accounting.", launched, want, backgroundConnectionSlots)
	}
	if backgroundConnectionSlots != connectionHolding {
		t.Errorf("backgroundConnectionSlots = %d, want %d", backgroundConnectionSlots, connectionHolding)
	}
}
