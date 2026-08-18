// rendercvWasm.ts — loads and calls the GOOS=js/wasm build of rendercv-go's
// pkg/rendercv (ReadYAML -> Build -> GenerateTypst only; see
// specs/046-real-resume-preview/research.md Decision 1 for why GeneratePDF is
// excluded from this build). Singleton: the module is fetched, compiled, and
// started once per page load, and every call after that reuses the same
// running Go runtime instance.
import { Volume, createFsFromVolume } from "memfs";
import { cachedFetch } from "./assetCache";

declare global {
  var fs: unknown;
  // On globalThis rather than Window: this module also runs inside the preview
  // Web Worker (previewWorker.ts), where there is no window at all.
  var Go: (new () => GoInstance) | undefined;
  var rendercvBuildTypst: ((yaml: string) => Promise<string>) | undefined;
}

// Go's GOOS=js/wasm runtime does real file I/O for template partials and
// output (os.WriteFile/os.ReadFile inside pkg/rendercv), routed through
// globalThis.fs — but wasm_exec.js's own built-in `fs` is a bare stub whose
// every operation returns ENOSYS (verified empirically: GenerateTypst fails
// with "not implemented on js" against it). memfs implements the same
// Node-callback-style fs API Go's runtime was written against, entirely in
// memory, so it must be installed before wasm_exec.js runs its
// `if (!globalThis.fs)` check.
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
  // In a worker there is no document to hang a <script> on. wasm_exec.js is a
  // classic script that only ever touches globalThis (it assigns globalThis.Go
  // and reads globalThis.fs), so evaluating its source in worker scope has the
  // same effect as loading the tag would in the page.
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
    // instantiateStreaming needs the right MIME type from the server; a dev
    // server that serves .wasm as octet-stream falls back to the buffer path.
    const bytes = await wasmResponse.arrayBuffer();
    return WebAssembly.instantiate(bytes, go.importObject);
  });
  // go.run's promise resolves only when the Go program returns; cmd/wasm's
  // main() blocks forever (`select {}`) specifically so the registered
  // rendercvBuildTypst global stays alive — this call is intentionally not
  // awaited.
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

/** Loads (once) and returns a function that turns RenderCV YAML into Typst source. */
export async function ensureRendercvWasmLoaded(): Promise<void> {
  if (!loadPromise) {
    loadPromise = load().catch((err) => {
      loadPromise = null; // allow a retry on the next call
      throw err;
    });
  }
  return loadPromise;
}

/** RenderCV YAML -> Typst source, via the loaded WASM module. */
export async function buildTypst(yaml: string): Promise<string> {
  await ensureRendercvWasmLoaded();
  if (!globalThis.rendercvBuildTypst) {
    throw new Error("resume preview: rendercv.wasm is not loaded");
  }
  return globalThis.rendercvBuildTypst(yaml);
}
