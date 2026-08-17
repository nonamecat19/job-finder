// assetCache.ts — a Cache-API-backed fetch so the ~29 MB typstwasm binary and
// its ~59 MB of fonts download once per browser, not once per preview
// (specs/046-real-resume-preview/research.md, Decision 2 / Open risk).
//
// Every asset lives under /wasm/, which is never part of the dashboard's own
// build output (see apps/dashboard/public/wasm/README.md) — a dedicated cache
// name keyed to that prefix means clearing it can never touch anything the
// app itself needs to keep working offline.

const CACHE_NAME = "resume-preview-wasm-v1";

async function openCache(): Promise<Cache | null> {
  if (typeof caches === "undefined") return null;
  try {
    return await caches.open(CACHE_NAME);
  } catch {
    // Cache API can throw in some private-browsing modes; fall back to an
    // uncached fetch rather than failing the whole preview pipeline over it.
    return null;
  }
}

/** Fetches `url`, serving from the Cache API on a hit and populating it on a miss. */
export async function cachedFetch(url: string): Promise<Response> {
  const cache = await openCache();
  if (cache) {
    const hit = await cache.match(url);
    if (hit) return hit;
  }
  const response = await fetch(url);
  if (!response.ok) {
    throw new Error(`resume preview: failed to fetch ${url}: ${response.status} ${response.statusText}`);
  }
  if (cache) {
    await cache.put(url, response.clone());
  }
  return response;
}

export async function cachedFetchArrayBuffer(url: string): Promise<ArrayBuffer> {
  const res = await cachedFetch(url);
  return res.arrayBuffer();
}

export async function cachedFetchJSON<T>(url: string): Promise<T> {
  const res = await cachedFetch(url);
  return res.json() as Promise<T>;
}
