# specs/archive/

Historical per-feature records, preserved as written at the time. **Not authoritative.**

Each `<nnn>-<slug>/` holds:

- `spec.md` — the original feature specification, unedited below a status banner that names
  its current standing and points at the domain doc that governs now.
- `contracts/` — interface sketches from the same run, where the feature had any.

The `plan.md`, `research.md`, `quickstart.md`, `tasks.md`, `data-model.md` and `checklists/`
files that accompanied each feature were removed once the feature shipped: they describe how
a thing was *going to be* built and validated, and the code on `master` answers all of that
more accurately. Recover any of them with:

```sh
git log --diff-filter=D --name-only -- 'specs/*'
git show <commit>^:specs/<nnn>-<slug>/plan.md
```

**Start at [`../README.md`](../README.md)** for the registry, and at
[`../domains/`](../domains/) for requirements that still bind.
