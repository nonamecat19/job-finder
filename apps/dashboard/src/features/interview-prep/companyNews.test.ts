import { describe, expect, it } from 'vitest';
import type { CompanyIntelDto } from '@job-finder/shared';
import { assembleCompanyNews, COMPANY_NEWS_STALE_MS, isCompanyNewsStale } from './companyNews';

const NOW = Date.parse('2026-07-20T12:00:00.000Z');

function makeIntel(overrides: Partial<CompanyIntelDto> = {}): CompanyIntelDto {
  return {
    companyName: 'Acme Corp',
    website: 'https://acme.example.com',
    funding: 'Series B',
    layoffs: null,
    glassdoorRating: 3.8,
    headcount: '200-500',
    techStack: 'React, TypeScript, Go',
    fetchedAt: new Date(NOW - 1000).toISOString(),
    ...overrides,
  };
}

describe('assembleCompanyNews', () => {
  it('returns an empty array when intel is null', () => {
    expect(assembleCompanyNews(null, NOW)).toEqual([]);
  });

  it('returns an empty array when intel is undefined', () => {
    expect(assembleCompanyNews(undefined, NOW)).toEqual([]);
  });

  it('assembles dated, sourced items for fresh intel', () => {
    const items = assembleCompanyNews(makeIntel(), NOW);
    expect(items).toEqual([
      { source: 'funding', label: 'Funding', value: 'Series B', fetchedAt: expect.any(String) },
      { source: 'glassdoor', label: 'Glassdoor rating', value: '3.8', fetchedAt: expect.any(String) },
      { source: 'headcount', label: 'Headcount', value: '200-500', fetchedAt: expect.any(String) },
      { source: 'tech-stack', label: 'Tech stack', value: 'React, TypeScript, Go', fetchedAt: expect.any(String) },
    ]);
  });

  it('excludes null/empty signals (e.g. layoffs)', () => {
    const items = assembleCompanyNews(makeIntel(), NOW);
    expect(items.find((i) => i.source === 'layoffs')).toBeUndefined();
  });

  describe('staleness cutoff', () => {
    it('includes items fetched exactly at the threshold (not stale)', () => {
      const fetchedAt = new Date(NOW - COMPANY_NEWS_STALE_MS).toISOString();
      const items = assembleCompanyNews(makeIntel({ fetchedAt }), NOW);
      expect(items.length).toBeGreaterThan(0);
    });

    it('excludes fresh items once fetchedAt is one ms past the threshold', () => {
      const fetchedAt = new Date(NOW - COMPANY_NEWS_STALE_MS - 1).toISOString();
      const items = assembleCompanyNews(makeIntel({ fetchedAt }), NOW);
      expect(items).toEqual([]);
    });

    it('excludes everything when the whole briefing is stale', () => {
      const fetchedAt = new Date(NOW - 2 * COMPANY_NEWS_STALE_MS).toISOString();
      const items = assembleCompanyNews(makeIntel({ fetchedAt }), NOW);
      expect(items).toEqual([]);
    });

    it('treats an unparseable fetchedAt as stale and excludes it', () => {
      const items = assembleCompanyNews(makeIntel({ fetchedAt: 'not-a-date' }), NOW);
      expect(items).toEqual([]);
    });
  });
});

describe('isCompanyNewsStale', () => {
  it('reports null intel as stale', () => {
    expect(isCompanyNewsStale(null, NOW)).toBe(true);
  });

  it('reports fresh intel as not stale', () => {
    expect(isCompanyNewsStale(makeIntel(), NOW)).toBe(false);
  });

  it('reports past-threshold intel as stale', () => {
    const fetchedAt = new Date(NOW - COMPANY_NEWS_STALE_MS - 1).toISOString();
    expect(isCompanyNewsStale(makeIntel({ fetchedAt }), NOW)).toBe(true);
  });
});
