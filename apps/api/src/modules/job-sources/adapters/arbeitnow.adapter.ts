import { Injectable } from '@nestjs/common';
import axios from 'axios';
import { NormalizedJob, SearchQuery } from '@job-finder/shared';
import { JobSourceAdapter } from '../adapter.interface';
import { htmlToText } from '../html.util';

interface ArbeitnowJob {
  slug: string;
  company_name: string;
  title: string;
  description: string; // HTML
  remote: boolean;
  url: string;
  tags: string[];
  job_types: string[];
  location: string;
  created_at: number; // unix seconds
}

@Injectable()
export class ArbeitnowAdapter implements JobSourceAdapter {
  readonly key = 'arbeitnow';
  readonly kind = 'api' as const;

  async search(query: SearchQuery): Promise<NormalizedJob[]> {
    // API has no keyword param — fetch first pages and filter locally.
    const collected: ArbeitnowJob[] = [];
    for (let page = 1; page <= 3; page++) {
      const res = await axios.get('https://api.arbeitnow.com/api/job-board-api', {
        params: { page },
        timeout: 30_000,
      });
      const data: ArbeitnowJob[] = res.data.data ?? [];
      collected.push(...data);
      if (!res.data.links?.next) break;
    }

    const terms = query.keywords.toLowerCase().split(/\s+/).filter(Boolean);
    const matches = collected.filter((j) => {
      const haystack = `${j.title} ${j.tags?.join(' ')} ${j.description}`.toLowerCase();
      return terms.length === 0 || terms.some((t) => haystack.includes(t));
    });

    return matches.map((j) => ({
      sourceKey: this.key,
      externalId: j.slug,
      title: j.title,
      company: j.company_name,
      location: j.location,
      remote: j.remote,
      url: j.url,
      description: htmlToText(j.description ?? ''),
      postedAt: j.created_at ? new Date(j.created_at * 1000).toISOString() : undefined,
      raw: j,
    }));
  }

  async healthCheck(): Promise<boolean> {
    try {
      const res = await axios.get('https://api.arbeitnow.com/api/job-board-api', {
        params: { page: 1 },
        timeout: 15_000,
      });
      return Array.isArray(res.data.data);
    } catch {
      return false;
    }
  }
}
