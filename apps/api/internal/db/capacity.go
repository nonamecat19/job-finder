package db

import "github.com/job-finder/api/internal/queue"

// backgroundConnectionSlots is the number of long-lived goroutines that hold a
// database connection outside the asynq worker pools: the ingestion scheduler
// (worker.Scheduler.Run) and the activity sweeper (activity.Sweeper.Run), both
// launched in cmd/server/servers.go:runServers.
//
// Raise this whenever another long-lived, connection-holding goroutine is
// launched there, or the derived capacity will under-provision by exactly the
// number of goroutines that were forgotten. capacity_test.go asserts this
// constant against the goroutines runServers actually starts.
const backgroundConnectionSlots = 2

// CapacityBudget derives how many database connections the process needs.
// Pure arithmetic, no I/O: the whole point is that capacity is a stated,
// testable decision rather than a property of the host's core count.
type CapacityBudget struct {
	// WorkerSlots is the sum of every asynq worker pool size.
	WorkerSlots int
	// BackgroundSlots covers non-worker goroutines that hold connections.
	BackgroundSlots int
	// InteractiveReserve is capacity held back for dashboard/API traffic.
	InteractiveReserve int
	// ServerMax is what the operator declares Postgres will grant.
	ServerMax int
}

// Required is the minimum pool size that lets every worker, every background
// goroutine, and the interactive reserve hold a connection simultaneously.
func (b CapacityBudget) Required() int {
	return b.WorkerSlots + b.BackgroundSlots + b.InteractiveReserve
}

// BudgetFromPolicies sums the worker pool sizes from the same policies that
// size each asynq.Server.
//
// It uses TaskPolicy.PoolSize() — max(local, hosted) — rather than the live
// provider class, so a runtime Settings change that flips a task to a hosted
// provider cannot push live concurrency past the budgeted ceiling (FR-013,
// research.md R3).
func BudgetFromPolicies(policies []queue.TaskPolicy, reserve, serverMax int) CapacityBudget {
	workers := 0
	for _, p := range policies {
		workers += p.PoolSize()
	}
	return CapacityBudget{
		WorkerSlots:        workers,
		BackgroundSlots:    backgroundConnectionSlots,
		InteractiveReserve: reserve,
		ServerMax:          serverMax,
	}
}
