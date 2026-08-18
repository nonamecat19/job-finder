---
title: The Python orchestration service
sidebar_position: 8
description: Changing a prompt or step without a backend release — how apps/ai's definitions work and how to iterate on one.
---

# The Python orchestration service

`apps/ai` (047-langchain-ai-service) is where prompt assembly, step
sequencing, tool loops and output validation live for every capability
routed to it. It reaches models only through the existing LiteLLM gateway,
by task key — never a provider directly.

## In-repo only, validated at startup

Every capability's prompt text and step definition lives in `apps/ai/src/jobfinder_ai/`
— never fetched from a database, a remote registry, or any source outside the
committed tree (FR-015a). `CapabilityRegistry.register()` (`capabilities/registry.py`)
validates every definition the moment it's registered: task key is one of the
fourteen declared in `gateway/config.yaml`, exactly one capability per task
key, every bound is positive, the prompt module and the input/output models
are actually importable. A malformed definition is a **startup failure
naming the capability** — the process never reaches "ready" with a broken
capability sitting behind a queue or an HTTP route, so the failure surfaces
in `docker compose logs ai` immediately, not the first time a job posting
happens to hit it.

## Changing a prompt

1. Edit the prompt text in `apps/ai/src/jobfinder_ai/prompts/<capability>.py`
   (or the step logic in `capabilities/single/` or `capabilities/graphs/`).
2. Restart only the AI service — no backend rebuild, no `apps/api` change:

   ```bash
   docker compose restart ai
   ```

3. Run the capability once (publish its work event, or call its HTTP route)
   and open the resulting trace in Langfuse. The new prompt's behavior is
   visible in that trace's spans immediately.

If the edit made a definition invalid — a bad bound, a typo'd prompt module
path — step 2 fails to come back healthy; `docker compose logs ai` names the
capability and the exact problem, not a stack trace at the first request.

## Telling before from after

Every trace carries `workflow_version` in its metadata (FR-015) — the
service's committed revision, resolved once at process start
(`tracing.resolve_workflow_version()`): the `WORKFLOW_VERSION` build arg in
production, `git rev-parse HEAD` for local/dev runs where no `.git` ships in
the image, or `"unknown"` rather than failing startup over a trace field.
Because prompts are in-repo (previous section), a revision identifies exact
prompt text — so two traces with different `workflow_version` values ran
different prompts, and you don't have to guess which deploy a given
regression came from.

## What this buys (SC-003)

Prompt iteration that used to mean a backend PR, review and deploy is now:
edit, restart one container, read the trace. The backend never rebuilds
because it never held the prompt in the first place — `apps/api` publishes
a work event or calls `/v1/capabilities/{name}/invoke`; everything about
*how* the model is asked lives on the other side of that boundary.
