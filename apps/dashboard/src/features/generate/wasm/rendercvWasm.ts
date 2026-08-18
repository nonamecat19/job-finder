

import { Volume, createFsFromVolume } from "memfs";
import { cachedFetch } from "./assetCache";

declare global {
  var fs: unknown;

  var Go: (new () => GoInstance) | undefined;
  var rendercvBuildTypst: ((yaml: string) => Promise<string>) | undefined;
}

function installMemFs(): void {
  if (globalThis.fs) return;
  const volume = new Volume();
  volume.mkdirSync("/preview", { recursive: true });
  globalThis.fs = createFsFromVolume(volume);
}

interface GoInstance {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

let loadPromise: Promise<void> | null = null;

async function loadWasmExecScript(): Promise<void> {
  if (typeof globalThis.Go !== "undefined") return;

  if (typeof document === "undefined") {
    const source = await (await fetch("/wasm/wasm_exec.js")).text();
    (0, eval)(source);
    if (typeof globalThis.Go === "undefined") {
      throw new Error("resume preview: /wasm/wasm_exec.js did not define Go");
    }
    return;
  }
  return new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = "/wasm/wasm_exec.js";
    script.onload = () => resolve();
    script.onerror = () => reject(new Error("resume preview: failed to load /wasm/wasm_exec.js"));
    document.head.appendChild(script);
  });
}

async function load(): Promise<void> {
  installMemFs();
  await loadWasmExecScript();
  const go = new globalThis.Go!();
  const wasmResponse = await cachedFetch("/wasm/rendercv.wasm");
  const { instance } = await WebAssembly.instantiateStreaming(wasmResponse.clone(), go.importObject).catch(async () => {

    const bytes = await wasmResponse.arrayBuffer();
    return WebAssembly.instantiate(bytes, go.importObject);
  });

  void go.run(instance);

  await waitForGlobal();
}

function waitForGlobal(timeoutMs = 10_000): Promise<void> {
  const start = Date.now();
  return new Promise((resolve, reject) => {
    const check = () => {
      if (typeof globalThis.rendercvBuildTypst === "function") {
        resolve();
        return;
      }
      if (Date.now() - start > timeoutMs) {
        reject(new Error("resume preview: rendercv.wasm did not register rendercvBuildTypst in time"));
        return;
      }
      setTimeout(check, 20);
    };
    check();
  });
}

export async function ensureRendercvWasmLoaded(): Promise<void> {
  if (!loadPromise) {
    loadPromise = load().catch((err) => {
      loadPromise = null;
      throw err;
    });
  }
  return loadPromise;
}

export async function buildTypst(yaml: string): Promise<string> {
  await ensureRendercvWasmLoaded();
  if (!globalThis.rendercvBuildTypst) {
    throw new Error("resume preview: rendercv.wasm is not loaded");
  }
  return globalThis.rendercvBuildTypst(yaml);
}
