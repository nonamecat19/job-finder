import { Injectable } from '@nestjs/common';
import axios from 'axios';
import { NormalizedJob, SearchQuery } from '@job-finder/shared';
import { JobSourceAdapter } from '../adapter.interface';

interface AdzunaResult {
  id: string;
  title: string;
  company?: { display_name?: string };
  location?: { display_name?: string };
  redirect_url: string;
  description: string;
  salary_min?: number;
  salary_max?: number;
  created?: string;
  contract_time?: string;
}

@Injectable()
export class AdzunaAdapter implements JobSourceAdapter {
  readonly key = 'adzuna';
  readonly kind = 'api' as const;

  async search(query: SearchQuery, config: Record<string, unknown>): Promise<NormalizedJob[]> {
    const appId = (config.appId as string) || process.env.ADZUNA_APP_ID;
    const appKey = (config.appKey as string) || process.env.ADZUNA_APP_KEY;
    if (!appId || !appKey) throw new Error('adzuna: appId/appKey not configured');
    const country = query.country || (config.country as string) || process.env.ADZUNA_COUNTRY || 'gb';

    const res = await axios.get(
      `https://api.adzuna.com/v1/api/jobs/${country.toLowerCase()}/search/1`,
      {
        params: {
          app_id: appId,
          app_key: appKey,
          what: query.keywords,
          where: query.location,
          salary_min: query.salaryMin,
          results_per_page: 50,
          'content-type': 'application/json',
        },
        timeout: 30_000,
      },
    );

    const results: AdzunaResult[] = res.data.results ?? [];
    return results.map((r) => ({
      sourceKey: this.key,
      externalId: String(r.id),
      title: r.title,
      company: r.company?.display_name ?? 'Unknown',
      location: r.location?.display_name,
      remote: /remote/i.test(`${r.title} ${r.description}`),
      salaryRaw:
        r.salary_min || r.salary_max
          ? `${r.salary_min ?? '?'} – ${r.salary_max ?? '?'}`
          : undefined,
      url: r.redirect_url,
      description: r.description ?? '',
      postedAt: r.created,
      raw: r,
    }));
  }

  async healthCheck(config: Record<string, unknown>): Promise<boolean> {
    try {
      await this.search({ keywords: 'developer' }, config);
      return true;
    } catch {
      return false;
    }
  }
}
