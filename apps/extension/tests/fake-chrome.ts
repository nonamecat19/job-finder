import { vi } from 'vitest';

type Listener = (msg: unknown, sender: unknown, sendResponse: (r: unknown) => void) => boolean | void;

/**
 * Minimal in-memory chrome.* stand-in. Hand-rolled rather than pulled from a
 * dependency: the surface we use is small, and the fake doubles as the
 * documentation of what the extension is allowed to touch.
 */
export function installFakeChrome() {
  const sync = new Map<string, unknown>();
  const local = new Map<string, unknown>();
  const runtimeListeners: Listener[] = [];
  const tabListeners: Listener[] = [];

  const store = (map: Map<string, unknown>) => ({
    get: vi.fn(async (keys?: string | string[] | Record<string, unknown>) => {
      if (keys === undefined) return Object.fromEntries(map);
      if (typeof keys === 'string') return { [keys]: map.get(keys) };
      if (Array.isArray(keys)) return Object.fromEntries(keys.map((k) => [k, map.get(k)]));
      // Object form: the keys are defaults, applied when nothing is stored.
      return Object.fromEntries(Object.entries(keys).map(([k, d]) => [k, map.has(k) ? map.get(k) : d]));
    }),
    set: vi.fn(async (items: Record<string, unknown>) => {
      for (const [k, v] of Object.entries(items)) map.set(k, v);
    }),
    remove: vi.fn(async (key: string) => {
      map.delete(key);
    }),
    clear: vi.fn(async () => map.clear()),
  });

  const dispatch = (listeners: Listener[], msg: unknown) =>
    new Promise<unknown>((resolve) => {
      if (listeners.length === 0) {
        resolve(undefined);
        return;
      }
      listeners[0](msg, {}, resolve);
    });

  const fake = {
    storage: { sync: store(sync), local: store(local) },
    runtime: {
      onMessage: { addListener: (l: Listener) => runtimeListeners.push(l) },
      sendMessage: vi.fn((msg: unknown) => dispatch(runtimeListeners, msg)),
      openOptionsPage: vi.fn(),
      lastError: undefined as { message: string } | undefined,
    },
    tabs: {
      query: vi.fn(async () => [{ id: 1, url: 'https://djinni.co/jobs/1-engineer' }]),
      sendMessage: vi.fn((_tabId: number, msg: unknown) => dispatch(tabListeners, msg)),
      onMessage: { addListener: (l: Listener) => tabListeners.push(l) },
    },
    permissions: {
      contains: vi.fn(async () => true),
      request: vi.fn(async () => true),
    },
  };

  (globalThis as unknown as { chrome: unknown }).chrome = fake;
  return { fake, sync, local, runtimeListeners, tabListeners };
}

export function uninstallFakeChrome() {
  delete (globalThis as unknown as { chrome?: unknown }).chrome;
}
