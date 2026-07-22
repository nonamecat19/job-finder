# Plan: Stoneforge Removal

**Status**: Pending — must be done LAST, after all feature plans are implemented.

## What to Remove

### Files and directories
- [ ] `.stoneforge/` — entire directory (DB, config, sync JSONL, worktrees, daemon state)
- [ ] `scripts/sf-unblock-sweep.py` — Stoneforge maintenance script
- [ ] `AGENTS.md` — Stoneforge workspace guide (replace with minimal project AGENTS.md if needed)

### Git worktrees
- [ ] Remove all 36 Stoneforge worktrees: `git worktree list` then `git worktree remove` for each
- [ ] Clean up `.stoneforge/.worktrees/` directory

### .gitignore
- [ ] Remove Stoneforge entries from `.gitignore`:
  ```
  /.stoneforge/
  /.stoneforge/.worktrees/
  ```

### Spec references
- [ ] Remove `sf docs add` reference from `specs/013-interview-prep-pack/spec.md` (line 413)

### Optional: Replace AGENTS.md
- [ ] If desired, create a minimal `AGENTS.md` with project-specific instructions (no Stoneforge references)

## Verification
- [ ] `git status` shows only intentional removals
- [ ] No `sf` CLI references remain in project files
- [ ] No `.stoneforge` directory remains
- [ ] `git worktree list` shows only the main worktree
- [ ] Project builds and tests pass without Stoneforge
