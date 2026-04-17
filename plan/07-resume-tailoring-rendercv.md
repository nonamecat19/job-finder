# 07 — Vacancy-Tailored Resume (RenderCV path)

## Why

The user maintains a hand-tuned RenderCV master (`resume/resume.yaml`, Typst `harvard` theme,
custom templates/locale/design) that already renders a polished PDF via the `rendercv` CLI. We
want: paste a vacancy → AI selects/reorders/rephrases the applicable technologies and achievements
(and, configurably, adds vacancy-relevant adjacent skills) → render a tailored PDF. This must be
drivable primarily from **opencode**, with the tailoring+render logic living in the existing
`GenerationModule`. RenderCV YAML ≠ JSON Resume (it carries `design`/`templates`/`locale`), so we
tailor **natively in YAML space** rather than routing through the JSON-Resume pipeline — the theme
and structure are preserved byte-for-byte.

## Design

Only the **content** is tailored; everything else is copied verbatim.

1. Parse `resume.yaml` with `yaml` → object.
2. LLM (`LlmProvider.completeStructured`) returns only the editable sections, schema-validated
   (`tailoredSectionsSchema`): summary (one paragraph), skills keyed by group **index**, experience
   highlights keyed by **company**.
3. `mergeTailored(master, payload)` deep-clones the master and overwrites only
   `summary` / `skills[].details` / `experience[].highlights`. Companies, positions, dates,
   education, `design`, `templates`, `locale`, `settings`, header are untouched → the structural
   grounding rules hold by construction.
4. `verifyRendercvGrounding(master, merged, level)` post-check + retry (2 attempts).
5. `RenderCvRenderer.render()` dumps YAML and shells
   `rendercv render <yaml> -o <dir> -pdf <name>.pdf -nopng -nohtml -nomd`.

### Grounding levels (`RESUME_GROUNDING_LEVEL`, default `moderate`)

- **strict** — reorder/rephrase/omit only; post-check flags any skill token absent from the master.
- **moderate** — may add skills adjacent to the existing stack (prompt-enforced adjacency).
- **aggressive** — may add whatever the vacancy requires (opt-in; highest fabrication risk).
Employers, dates, degrees, projects and numeric metrics are never invented at any level.

## Files

- `apps/api/src/modules/generation/rendercv-tailor.ts` — schema, per-level prompt, `mergeTailored`, token helpers.
- `apps/api/src/modules/generation/rendercv-grounding.ts` — `verifyRendercvGrounding`.
- `apps/api/src/modules/generation/rendercv-renderer.ts` — `RenderCvRenderer` (YAML → `rendercv` → PDF).
- `apps/api/src/modules/generation/generation.service.ts` — `generateRendercvFromText` (ad-hoc, no DB job) + `tailorRendercvResume`.
- `apps/api/src/modules/generation/documents.controller.ts` — `POST /api/documents/tailor`.
- `apps/api/src/modules/generation/generation.module.ts` — registers `RenderCvRenderer`.
- `apps/api/scripts/tailor-resume.ts` — standalone CLI (Ollama + rendercv, no Nest).
- `~/.config/opencode/command/tailor-resume.md`, `resume/AGENTS.md` — opencode bridge + rules.
- env: `RESUME_MASTER_PATH`, `RESUME_GROUNDING_LEVEL`, `RENDERCV_BIN`, reuse `DOCUMENTS_DIR`.

## Entry points

- **opencode** `/tailor-resume "<vacancy>" [level:…]` → `POST /api/documents/tailor`, or the CLI fallback.
- **HTTP** `POST /api/documents/tailor { vacancy, company?, title?, groundingLevel? }` → `{ yamlPath, pdfPath, groundingLevel }`.
- **jobId path** (future): branch existing `generate()` with a `rendercv` renderer flavor to persist a `GeneratedDocument`.

## Not done / follow-ups

- Ad-hoc path renders to disk only (no `GeneratedDocument` — its `jobId` is mandatory). Persisting a
  pasted vacancy would require creating a `manual` Job first; deferred until the tracker needs it.
- Dashboard UI to pick grounding level per generation.
