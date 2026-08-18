import type { JobDto } from '@job-finder/shared';

import type { PageHints } from '@/shared/messages';
import { err, ok, type Result } from '@/shared/result';

import * as api from './api';
import { getCachedJobId, setCachedJobId } from './cache';
import { canonicalizeVacancyUrl, sameVacancy } from './url';

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

    if (job.error.code === 'not_found') await setCachedJobId(canonical, null);
    else return job;
  }

  const exact = await api.findJobByUrl(baseUrl, canonical);
  if (!exact.ok) return exact;
  if (exact.value) return finish(canonical, exact.value);

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
