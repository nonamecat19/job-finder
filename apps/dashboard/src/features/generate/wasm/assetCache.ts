

const CACHE_NAME = "resume-preview-wasm-v1";

async function openCache(): Promise<Cache | null> {
  if (typeof caches === "undefined") return null;
  try {
    return await caches.open(CACHE_NAME);
  } catch {

    return null;
  }
}

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
