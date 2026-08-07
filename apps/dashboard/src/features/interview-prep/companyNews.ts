import type { CompanyIntelDto } from '@job-finder/shared';

export const COMPANY_NEWS_STALE_MS = 30 * 24 * 60 * 60 * 1000;

export type CompanyNewsSource =
  | 'funding'
  | 'layoffs'
  | 'glassdoor'
  | 'headcount'
  | 'tech-stack';

export interface CompanyNewsItem {
  source: CompanyNewsSource;
  label: string;
  value: string;
  fetchedAt: string;
}

const ITEM_LABELS: Record<CompanyNewsSource, string> = {
  funding: 'Funding',
  layoffs: 'Layoffs',
  glassdoor: 'Glassdoor rating',
  headcount: 'Headcount',
  'tech-stack': 'Tech stack',
};

function isStale(fetchedAt: string, now: number, staleMs: number): boolean {
  const fetched = new Date(fetchedAt).getTime();
  if (Number.isNaN(fetched)) return true;
  return now - fetched > staleMs;
}

function buildItem(
  source: CompanyNewsSource,
  value: string | number | null | undefined,
): { source: CompanyNewsSource; label: string; value: string } | null {
  if (value == null || value === '') return null;
  return { source, label: ITEM_LABELS[source], value: String(value) };
}

export function assembleCompanyNews(
  intel: CompanyIntelDto | null | undefined,
  now: number = Date.now(),
  staleMs: number = COMPANY_NEWS_STALE_MS,
): CompanyNewsItem[] {
  if (!intel) return [];

  const candidates: Array<{ source: CompanyNewsSource; value: string | number | null | undefined }> = [
    { source: 'funding', value: intel.funding },
    { source: 'layoffs', value: intel.layoffs },
    { source: 'glassdoor', value: intel.glassdoorRating },
    { source: 'headcount', value: intel.headcount },
    { source: 'tech-stack', value: intel.techStack },
  ];

  const items: CompanyNewsItem[] = [];
  for (const candidate of candidates) {
    const built = buildItem(candidate.source, candidate.value);
    if (!built) continue;
    if (isStale(intel.fetchedAt, now, staleMs)) continue;
    items.push({ ...built, fetchedAt: intel.fetchedAt });
  }

  return items;
}

export function isCompanyNewsStale(
  intel: CompanyIntelDto | null | undefined,
  now: number = Date.now(),
  staleMs: number = COMPANY_NEWS_STALE_MS,
): boolean {
  if (!intel) return true;
  return isStale(intel.fetchedAt, now, staleMs);
}
