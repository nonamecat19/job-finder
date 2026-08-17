#!/usr/bin/env bash
# Builds the two WASM artifacts the in-browser resume preview needs
# (specs/046-real-resume-preview) and copies them, plus their static assets,
# into apps/dashboard/public/wasm/ (gitignored — see that directory's README).
#
# Requires ../rendercv-go present as a sibling checkout of this repo.
set -euo pipefail

SCRIPT_DIR="$(CDPATH="" cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DASHBOARD_DIR="$(CDPATH="" cd "$SCRIPT_DIR/.." && pwd)"
REPO_ROOT="$(CDPATH="" cd "$DASHBOARD_DIR/../.." && pwd)"
RENDERCV_GO_DIR="$REPO_ROOT/../rendercv-go"
OUT_DIR="$DASHBOARD_DIR/public/wasm"

if [ ! -d "$RENDERCV_GO_DIR" ]; then
    echo "error: expected a sibling checkout at $RENDERCV_GO_DIR (github.com/nonamecat19/rendercv-go)" >&2
    exit 1
fi

mkdir -p "$OUT_DIR"

echo "[build-wasm] building rendercv.wasm (GOOS=js GOARCH=wasm) from $RENDERCV_GO_DIR/cmd/wasm ..."
( cd "$RENDERCV_GO_DIR" && GOOS=js GOARCH=wasm go build -o "$OUT_DIR/rendercv.wasm" ./cmd/wasm )

echo "[build-wasm] copying wasm_exec.js glue from the local Go toolchain ..."
GOROOT="$(go env GOROOT)"
WASM_EXEC="$GOROOT/lib/wasm/wasm_exec.js"
if [ ! -f "$WASM_EXEC" ]; then
    # Older Go toolchains ship it under misc/wasm instead of lib/wasm.
    WASM_EXEC="$GOROOT/misc/wasm/wasm_exec.js"
fi
cp "$WASM_EXEC" "$OUT_DIR/wasm_exec.js"

ASSETS_DIR="$RENDERCV_GO_DIR/internal/renderer/typstc/assets"
echo "[build-wasm] copying the already-built typst.wasm, fonts/, and packages/ from $ASSETS_DIR ..."
cp "$ASSETS_DIR/typst.wasm" "$OUT_DIR/typstwasm.wasm"
rm -rf "$OUT_DIR/fonts" "$OUT_DIR/packages"
cp -r "$ASSETS_DIR/fonts" "$OUT_DIR/fonts"
cp -r "$ASSETS_DIR/packages" "$OUT_DIR/packages"

echo "[build-wasm] writing assets-manifest.json (typstWasi.ts's virtual-FS file list) ..."
(
    cd "$OUT_DIR"
    {
        echo '{'
        echo '  "fonts": ['
        find fonts -type f | sed 's#^fonts/##' | sort | sed 's/.*/"&"/' | paste -sd, - | sed 's/^/    /'
        echo '  ],'
        echo '  "packages": ['
        find packages -type f | sed 's#^packages/##' | sort | sed 's/.*/"&"/' | paste -sd, - | sed 's/^/    /'
        echo '  ]'
        echo '}'
    } > assets-manifest.json
)

echo "[build-wasm] done:"
du -sh "$OUT_DIR"/* 2>/dev/null | sed 's/^/  /'
