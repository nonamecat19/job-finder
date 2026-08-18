

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

export async function ensureTypstWasiLoaded(): Promise<void> {
  await Promise.all([loadCompiledModule(), loadStaticTree()]);
}

function todayISODate(): string {
  return new Date().toISOString().slice(0, 10);
}

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

    { debug: false },
  );

  const instance = await WebAssembly.instantiate(module, {
    wasi_snapshot_preview1: wasi.wasiImport,
  });

  const exitCode = wasi.start(instance as unknown as Parameters<WASI["start"]>[0]);
  if (exitCode !== 0) {
    throw new Error(`resume preview: typstwasm exited ${exitCode}${stderr ? `: ${stderr.trim()}` : ""}`);
  }

  const written = outDir.contents.get("output.pdf");
  if (!(written instanceof File) || written.data.byteLength === 0) {
    throw new Error(`resume preview: typstwasm produced no output${stderr ? `: ${stderr.trim()}` : ""}`);
  }
  return written.data;
}
