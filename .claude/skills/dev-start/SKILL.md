# dev-start

Starts Docker infrastructure, backend API server, and frontend dev server via process-hive.

## Description
Launches all three services needed to run job-finder locally: Docker Compose (Postgres, Redis, etc.), Go API server on port 3000, and React dev server on port 5173.

Services run in background via process-hive and persist across sessions.

## Usage
```bash
claude /dev-start
```

## Output
- `job-finder-infra`: Docker Compose (just up)
- `job-finder-backend`: Go API server (port 3000)
- `job-finder-frontend`: React dev (port 5173)

Access frontend at http://localhost:5173

---

# Implementation

```bash
#!/bin/bash

PROJECT_DIR=/home/nnc/Projects/job-finder

# Check if all services running
if process-hive ps --all | grep -E "(job-finder-infra|job-finder-backend|job-finder-frontend)" | grep -q running; then
  echo "✓ All services already running"
  process-hive ps
  exit 0
fi

echo "Starting infrastructure..."
process-hive start --name=job-finder-infra --cwd="$PROJECT_DIR" --restart=on-failure -- just up

sleep 5

echo "Starting backend..."
process-hive start --name=job-finder-backend --cwd="$PROJECT_DIR" --env-file="$PROJECT_DIR/.env" --restart=on-failure -- sh -c 'cd apps/api && go run ./cmd/server'

sleep 2

echo "Starting frontend..."
process-hive start --name=job-finder-frontend --cwd="$PROJECT_DIR" --restart=on-failure -- pnpm dev

echo ""
echo "✓ All services started"
process-hive ps
echo ""
echo "Frontend: http://localhost:5173"
echo "Backend API: http://localhost:3000"
```
