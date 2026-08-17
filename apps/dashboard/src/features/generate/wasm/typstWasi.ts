// typstWasi.ts — loads tools/typstwasm (the Typst compiler, built for
// wasm32-wasip1) and runs it directly against a browser WASI shim instead of
// wazero, so a single interpretation layer separates the browser from the
// compiler (specs/046-real-resume-preview/research.md, Decision 2).
//
// The compiled binary is a CLI (`--root --pkg --font-dir --in --out
// --format`, see ../../../../../../rendercv-go/tools/typstwasm/README.md);
// there is no persistent process to keep alive the way rendercvWasm.ts's Go
// runtime is, so every call instantiates a fresh WASI environment. Typst
// itself is fast once its module is compiled and cached by the browser
// (assetCache.ts) — WebAssembly.compile result reuse is what actually avoids
// redoing the expensive part per call (see moduleCache below).

import type { Inode } from "@bjorn3/browser_wasi_shim";
import { Directory, File, OpenFile, ConsoleStdout, PreopenDirectory, WASI } from "@bjorn3/browser_wasi_shim";
import { cachedFetch, cachedFetchArrayBuffer, cachedFetchJSON } from "./assetCache";

interface AssetsManifest {
  fonts: string[];
  packages: string[];
}

let compiledModulePromise: Promise<WebAssembly.Module> | null = null;
let staticTreePromise: Promise<{ fonts: Directory; pkg: Directory }> | null = null;

async function loadCompiledModule(): Promise<WebAssembly.Module> {
  if (!compiledModulePromise) {
    compiledModulePromise = (async () => {
      const res = await cachedFetch("/wasm/typstwasm.wasm");
      return WebAssembly.compileStreaming(res.clone()).catch(async () => {
        const bytes = await res.arrayBuffer();
        return WebAssembly.compile(bytes);
      });
    })();
  }
  return compiledModulePromise;
}

/** Builds a nested Directory tree of Inodes from a flat list of relative paths under `baseUrl`. */
async function buildTree(baseUrl: string, paths: string[]): Promise<Directory> {
  const root = new Map<string, Inode>();

  const files = await Promise.all(
    paths.map(async (relPath) => {
      const bytes = await cachedFetchArrayBuffer(`${baseUrl}/${relPath}`);
      return { relPath, bytes };
    }),
  );

  for (const { relPath, bytes } of files) {
    const parts = relPath.split("/");
    let dir = root;
    for (let i = 0; i < parts.length - 1; i++) {
      const seg = parts[i];
      const existing = dir.get(seg);
      const next: Directory = existing instanceof Directory ? existing : new Directory([]);
      if (existing !== next) {
        dir.set(seg, next);
      }
      dir = next.contents;
    }
    dir.set(parts[parts.length - 1], new File(bytes));
  }

  return new Directory(root);
}

async function loadStaticTree(): Promise<{ fonts: Directory; pkg: Directory }> {
  if (!staticTreePromise) {
    staticTreePromise = (async () => {
      const manifest = await cachedFetchJSON<AssetsManifest>("/wasm/assets-manifest.json");
      const [fonts, pkg] = await Promise.all([
        buildTree("/wasm/fonts", manifest.fonts),
        buildTree("/wasm/packages", manifest.packages),
      ]);
      return { fonts, pkg };
    })();
  }
  return staticTreePromise;
}

/** Pre-fetches and caches everything compilePdf needs, so the first real call is fast. */
export async function ensureTypstWasiLoaded(): Promise<void> {
  await Promise.all([loadCompiledModule(), loadStaticTree()]);
}

function todayISODate(): string {
  return new Date().toISOString().slice(0, 10);
}

/** Compiles Typst source (as produced by rendercvWasm's buildTypst) to PDF bytes. */
export async function compilePdf(typstSource: string): Promise<Uint8Array> {
  const [module, { fonts, pkg }] = await Promise.all([loadCompiledModule(), loadStaticTree()]);

  const inputDir = new Directory([["input.typ", new File(new TextEncoder().encode(typstSource))]]);
  const outputFile = new File(new Uint8Array(0));
  const outDir = new Directory([["output.pdf", outputFile]]);

  const root = new PreopenDirectory(
    "/",
    new Map<string, Directory>([
      ["in", inputDir],
      ["pkg", pkg],
      ["fonts", fonts],
      ["out", outDir],
    ]),
  );

  const args = [
    "typstwasm",
    "--root",
    "/in",
    "--pkg",
    "/pkg",
    "--font-dir",
    "/fonts",
    "--in",
    "/in/input.typ",
    "--out",
    "/out/output.pdf",
    "--format",
    "pdf",
    // Required: this WASI shim doesn't implement clock_time_get, so without
    // an explicit --today typst fails with "unable to get the current date"
    // (verified empirically). Today's date is good enough for a resume
    // preview's "Last updated" footer — no locale conversion needed since
    // World::today only wants YYYY-MM-DD.
    "--today",
    todayISODate(),
  ];

  let stderr = "";
  const wasi = new WASI(
    args,
    [],
    [
      new OpenFile(new File(new Uint8Array(0))),
      ConsoleStdout.lineBuffered(() => {}),
      ConsoleStdout.lineBuffered((msg) => {
        stderr += `${msg}\n`;
      }),
      root,
    ],
  );

  const instance = await WebAssembly.instantiate(module, {
    wasi_snapshot_preview1: wasi.wasiImport,
  });

  const exitCode = wasi.start(instance as unknown as Parameters<WASI["start"]>[0]);
  if (exitCode !== 0) {
    throw new Error(`resume preview: typstwasm exited ${exitCode}${stderr ? `: ${stderr.trim()}` : ""}`);
  }

  // typst may have replaced the directory entry rather than writing into our
  // File instance in place — read back through the directory, not the
  // original reference.
  const written = outDir.contents.get("output.pdf");
  if (!(written instanceof File) || written.data.byteLength === 0) {
    throw new Error(`resume preview: typstwasm produced no output${stderr ? `: ${stderr.trim()}` : ""}`);
  }
  return written.data;
}
