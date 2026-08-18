import type { JobDto } from '@job-finder/shared';

import type { PageHints } from '@/shared/messages';
import { err, ok, type Result } from '@/shared/result';

import * as api from './api';
import { getCachedJobId, setCachedJobId } from './cache';
import { canonicalizeVacancyUrl, sameVacancy } from './url';

/**
 * Tab URL -> job.
 *
 * The exact ?url= filter answers this in one request. The title search is a
 * fallback for the case where the stored URL differs from the canonical form of
 * the tab URL (a redirect, a locale prefix, an id-only slug change).
 *
 * A miss is a miss: POST /jobs/manual is never called from here. It scrapes and
 * writes, and its dedupe hashes the freshly scraped company+title — a one
 * character drift creates a second job row. Adding a vacancy stays an explicit
 * user action.
 */
export async function resolveJob(
  baseUrl: string,
  rawUrl: string,
  hints?: PageHints,
): Promise<Result<JobDto>> {
  const canonical = canonicalizeVacancyUrl(rawUrl);

  const cached = await getCachedJobId(canonical);
  if (cached === null) {
    return err('not_found', "This vacancy isn't in job-finder yet.");
  }
  if (typeof cached === 'string') {
    const job = await api.getJob(baseUrl, cached);
    if (job.ok) return job;
    // A cached id that no longer resolves (job deleted) must not be sticky.
    if (job.error.code === 'not_found') await setCachedJobId(canonical, null);
    else return job;
  }

  const exact = await api.findJobByUrl(baseUrl, canonical);
  if (!exact.ok) return exact;
  if (exact.value) return finish(canonical, exact.value);

  // The stored URL may not be canonical; compare client-side over a title search.
  const title = hints?.title?.trim();
  if (title) {
    const found = await api.searchJobs(baseUrl, title);
    if (!found.ok) return found;
    const match = found.value.find((job) => sameVacancy(job.url, rawUrl));
    if (match) return finish(canonical, match);
  }

  await setCachedJobId(canonical, null);
  return err('not_found', "This vacancy isn't in job-finder yet.");
}

async function finish(canonical: string, job: JobDto): Promise<Result<JobDto>> {
  await setCachedJobId(canonical, job.id);
  return ok(job);
}
