import { canonicalizeVacancyUrl } from './url';

const KEY = 'urlToJob';
const HIT_TTL_MS = 7 * 24 * 60 * 60 * 1000;
// A miss is cached only briefly: the usual fix is "add it in the dashboard",
// and the user expects reopening the popup to notice.
const MISS_TTL_MS = 60 * 1000;

type Entry = { jobId: string | null; at: number };
type Table = Record<string, Entry>;

async function read(): Promise<Table> {
  const stored = (await chrome.storage.local.get(KEY)) as Record<string, Table | undefined>;
  return stored[KEY] ?? {};
}

/** undefined = nothing cached, null = cached miss. */
export async function getCachedJobId(url: string): Promise<string | null | undefined> {
  const table = await read();
  const entry = table[canonicalizeVacancyUrl(url)];
  if (!entry) return undefined;
  const ttl = entry.jobId ? HIT_TTL_MS : MISS_TTL_MS;
  if (Date.now() - entry.at > ttl) return undefined;
  return entry.jobId;
}

export async function setCachedJobId(url: string, jobId: string | null): Promise<void> {
  const table = await read();
  table[canonicalizeVacancyUrl(url)] = { jobId, at: Date.now() };
  await chrome.storage.local.set({ [KEY]: table });
}

export async function clearCache(): Promise<void> {
  await chrome.storage.local.remove(KEY);
}
