---
name: split-commits
description: Split current uncommitted changes into multiple small, per-feature git commits instead of one bundled commit. Use whenever the user asks to "split this into commits", "commit in pieces", "make separate commits", "one commit per feature/change", or after a large multi-feature diff has piled up and needs cleaning into a reviewable history. Also use proactively before committing a diff that clearly spans more than one unrelated concern.
---

# Split Commits

Turn a messy working tree (many files, several unrelated changes) into a sequence of small commits, each scoped to one feature or fix. This user prefers small, per-feature commits over one bundled commit — a bundled commit hides which change caused a regression and makes review harder.

## Workflow

### 1. Survey the diff

```bash
git status
git diff
git diff --stat
```

Read the actual diff, not just filenames — two changes touching the same file can still be unrelated concerns, and two changes touching different files can be one feature.

### 2. Group into logical units

Group hunks/files by *what they accomplish*, not by file type or directory. A good group is something you could describe in one commit subject line without "and". Typical splits:

- New adapter/module + its tests + its testdata → one commit
- A refactor of existing code → separate from any new feature built on top of it
- Config/spec/doc scaffolding (e.g. `.specify/`, `specs/`) → often its own commit if unrelated to the code change
- Unrelated one-line fixes discovered incidentally → their own tiny commit, don't fold into a feature commit

If a single file mixes two concerns (e.g. one function changed for feature A, another for feature B), split by hunk rather than forcing the whole file into one group.

Show the user the proposed grouping before staging anything, unless they've asked you to just proceed.

### 3. Stage and commit each group independently

Stage only that group's files/hunks:

```bash
git add <specific files>
# or, when a file has multiple unrelated hunks:
git add -p <file>
```

Verify what's actually staged before committing — don't trust the group plan blindly:

```bash
git diff --cached --stat
```

Commit with a message describing that group's *why*, following this repo's existing commit style (check `git log` for tone/format). Use a HEREDOC for the message. Each commit should leave the repo in a working, self-consistent state where reasonable (don't split a change from the test that makes it pass, for instance).

### 4. Repeat until the working tree is clean

Loop steps 3 for each remaining group. Run `git status` between commits to confirm progress and catch anything left ungrouped.

### 5. Final check

```bash
git log --oneline -n <count>
git status
```

Confirm the tree is clean and the commit sequence tells a coherent story. Do not push unless the user explicitly asks.

## Notes

- Never use `git add -A` / `git add .` here — grouping requires precision, and a blanket add defeats the purpose.
- If a file contains a secret-looking value, flag it before including it in any commit.
- If two changes are too entangled to split cleanly (e.g. a rename touches every group), say so and propose the smallest reasonable split rather than forcing a false separation.
