package db

import "github.com/job-finder/api/internal/queue"

const backgroundConnectionSlots = 2

type CapacityBudget struct {
	WorkerSlots        int
	BackgroundSlots    int
	InteractiveReserve int
	ServerMax          int
}

func (b CapacityBudget) Required() int {
	return b.WorkerSlots + b.BackgroundSlots + b.InteractiveReserve
}

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
