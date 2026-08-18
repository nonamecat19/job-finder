import { beforeEach, describe, expect, it, vi } from 'vitest';

import { clearCache, setCachedJobId } from '@/background/cache';
import { resolveJob } from '@/background/resolve';

import { jsonResponse as json, stubFetch, urlOf } from './fetch-mock';

const BASE = 'http://localhost:3000';
const URL_TAB = 'https://djinni.co/jobs/123-go-engineer?from=feed';
const URL_CANON = 'https://djinni.co/jobs/123-go-engineer';

const JOB = { id: 'job-1', title: 'Go Engineer', company: 'Acme', url: URL_CANON, status: 'found' };

describe('resolveJob', () => {
  beforeEach(async () => {
    vi.restoreAllMocks();
    await clearCache();
  });

  it('resolves through the exact ?url= filter in a single request', async () => {
    const fetchMock = stubFetch(async () => json({ items: [JOB], total: 1, page: 1, pageSize: 5 }));

    const res = await resolveJob(BASE, URL_TAB);

    expect(res.ok).toBe(true);
    if (res.ok) expect(res.value.id).toBe('job-1');
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('falls back to a title search when the stored URL is not the canonical one', async () => {
    const stored = { ...JOB, url: 'https://djinni.co/jobs/123-go-engineer/?utm=x' };
    const responses = [
      json({ items: [], total: 0, page: 1, pageSize: 5 }),
      json({ items: [stored], total: 1, page: 1, pageSize: 100 }),
    ];
    let call = 0;
    const fetchMock = stubFetch(async () => responses[call++]);

    const res = await resolveJob(BASE, URL_TAB, { title: 'Go Engineer' });

    expect(res.ok).toBe(true);
    expect(urlOf(fetchMock, 1).searchParams.get('q')).toBe('Go Engineer');
  });

  it('reports not_found rather than creating a job', async () => {
    const fetchMock = stubFetch(async () => json({ items: [], total: 0, page: 1, pageSize: 5 }));

    const res = await resolveJob(BASE, URL_TAB, { title: 'Go Engineer' });

    expect(res.ok).toBe(false);
    if (!res.ok) expect(res.error.code).toBe('not_found');

    for (const call of fetchMock.mock.calls) {
      expect(call[1]?.method ?? 'GET').toBe('GET');
    }
  });

  it('serves a cached hit with one request for the job itself', async () => {
    await setCachedJobId(URL_CANON, 'job-1');
    const fetchMock = stubFetch(async () => json(JOB));

    const res = await resolveJob(BASE, URL_TAB);

    expect(res.ok).toBe(true);
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(String(fetchMock.mock.calls[0][0])).toBe(`${BASE}/api/jobs/job-1`);
  });

  it('does not re-sweep on a cached miss', async () => {
    await setCachedJobId(URL_CANON, null);
    const fetchMock = stubFetch(async () => json({ items: [], total: 0, page: 1, pageSize: 5 }));

    const res = await resolveJob(BASE, URL_TAB);

    expect(res.ok).toBe(false);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('recovers when a cached job id no longer exists', async () => {
    await setCachedJobId(URL_CANON, 'deleted-job');
    const responses = [new Response('job not found', { status: 404 }), json({ items: [JOB], total: 1, page: 1, pageSize: 5 })];
    let call = 0;
    stubFetch(async () => responses[call++]);

    const res = await resolveJob(BASE, URL_TAB);

    expect(res.ok).toBe(true);
    if (res.ok) expect(res.value.id).toBe('job-1');
  });
});
