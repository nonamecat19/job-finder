import { Injectable } from '@nestjs/common';
import axios from 'axios';
import { NormalizedJob, SearchQuery } from '@job-finder/shared';
import { JobSourceAdapter } from '../adapter.interface';

interface SidecarJob {
  id?: string;
  site: string;
  title: string;
  company?: string;
  location?: string;
  is_remote?: boolean;
  salary?: string;
  job_url: string;
  description?: string;
  date_posted?: string;
}

/**
 * Delegates LinkedIn/Indeed/Glassdoor to the python jobspy sidecar
 * (apps/jobspy-sidecar). Best-effort: upstream scrapers break periodically.
 */
@Injectable()
export class JobSpyAdapter implements JobSourceAdapter {
  readonly key = 'jobspy';
  readonly kind = 'sidecar' as const;

  private baseUrl(config: Record<string, unknown>): string {
    return (config.url as string) || process.env.JOBSPY_URL || 'http://jobspy-sidecar:8000';
  }

  async search(query: SearchQuery, config: Record<string, unknown>): Promise<NormalizedJob[]> {
    const site = query.site || (config.site as string) || 'linkedin';
    const res = await axios.post(
      `${this.baseUrl(config)}/search`,
      {
        site,
        search_term: query.keywords,
        location: query.location,
        is_remote: query.remote ?? false,
        results_wanted: 30,
      },
      { timeout: 120_000 },
    );

    const jobs: SidecarJob[] = res.data.jobs ?? [];
    return jobs.map((j) => ({
      sourceKey: this.key,
      externalId: j.id,
      title: j.title,
      company: j.company ?? 'Unknown',
      location: j.location,
      remote: j.is_remote ?? false,
      salaryRaw: j.salary || undefined,
      url: j.job_url,
      description: j.description ?? '',
      postedAt: j.date_posted,
      raw: j,
    }));
  }

  async healthCheck(config: Record<string, unknown>): Promise<boolean> {
    try {
      const res = await axios.get(`${this.baseUrl(config)}/health`, { timeout: 10_000 });
      return res.data?.ok === true;
    } catch {
      return false;
    }
  }
}
