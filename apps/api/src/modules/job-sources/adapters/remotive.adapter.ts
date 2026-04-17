import { Injectable } from '@nestjs/common';
import axios from 'axios';
import { NormalizedJob, SearchQuery } from '@job-finder/shared';
import { JobSourceAdapter } from '../adapter.interface';
import { htmlToText } from '../html.util';

interface RemotiveJob {
  id: number;
  title: string;
  company_name: string;
  candidate_required_location?: string;
  url: string;
  description: string; // HTML
  publication_date?: string;
  salary?: string;
}

@Injectable()
export class RemotiveAdapter implements JobSourceAdapter {
  readonly key = 'remotive';
  readonly kind = 'api' as const;

  async search(query: SearchQuery): Promise<NormalizedJob[]> {
    const res = await axios.get('https://remotive.com/api/remote-jobs', {
      params: { search: query.keywords, limit: 50 },
      timeout: 30_000,
    });
    const jobs: RemotiveJob[] = res.data.jobs ?? [];
    return jobs.map((j) => ({
      sourceKey: this.key,
      externalId: String(j.id),
      title: j.title,
      company: j.company_name,
      location: j.candidate_required_location,
      remote: true, // remote-only board
      salaryRaw: j.salary || undefined,
      url: j.url,
      description: htmlToText(j.description ?? ''),
      postedAt: j.publication_date,
      raw: j,
    }));
  }

  async healthCheck(): Promise<boolean> {
    try {
      const res = await axios.get('https://remotive.com/api/remote-jobs', {
        params: { limit: 1 },
        timeout: 15_000,
      });
      return Array.isArray(res.data.jobs);
    } catch {
      return false;
    }
  }
}
