# apps/dashboard/public/wasm/

Build output for the in-browser resume preview (feature 046-real-resume-preview), not committed source:

- `rendercv.wasm`, `wasm_exec.js` — `GOOS=js GOARCH=wasm` build of `../rendercv-go`'s `pkg/rendercv` (`ReadYAML`/`Build`/`GenerateTypst` only, per `specs/046-real-resume-preview/research.md` Decision 1).
- `typstwasm.wasm`, `fonts/`, `packages/` — copied as-is from `../rendercv-go/internal/renderer/typstc/assets/` (`typst.wasm`, `fonts/`, `packages/`), which already vendors everything the embedded Typst compiler needs, including the two non-obvious inputs the `tools/typstwasm` README calls out: the `@preview/fontawesome:0.6.0` package and the `rendercv-fonts` set.

Generate all of the above by running `apps/dashboard/scripts/build-wasm.sh` from a checkout with `../rendercv-go` present as a sibling directory. These files are gitignored (see repo root `.gitignore`) — they are large (~29 MB for `typstwasm.wasm` alone) and are fetched lazily by the dashboard at runtime, not part of the built JS bundle.
