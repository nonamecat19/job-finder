import { describe, expect, it } from 'vitest';

import { canonicalizeVacancyUrl, sameVacancy } from '@/background/url';

describe('canonicalizeVacancyUrl', () => {
  const cases: [string, string][] = [
    ['https://djinni.co/jobs/123-go-engineer/', 'https://djinni.co/jobs/123-go-engineer'],
    ['https://djinni.co/jobs/123-go-engineer?from=feed', 'https://djinni.co/jobs/123-go-engineer'],
    ['https://djinni.co/jobs/123-go-engineer#apply', 'https://djinni.co/jobs/123-go-engineer'],
    ['https://www.work.ua/jobs/1234567/', 'https://work.ua/jobs/1234567'],
    ['https://WORK.UA/jobs/1234567', 'https://work.ua/jobs/1234567'],
    ['https://jobs.dou.ua/companies/acme/vacancies/42/?from=list', 'https://jobs.dou.ua/companies/acme/vacancies/42'],
  ];

  it.each(cases)('%s -> %s', (raw, want) => {
    expect(canonicalizeVacancyUrl(raw)).toBe(want);
  });

  it('leaves an unparseable value alone rather than throwing', () => {
    expect(canonicalizeVacancyUrl('not a url')).toBe('not a url');
  });

  it('treats tracking-only differences as the same vacancy', () => {
    expect(sameVacancy('https://djinni.co/jobs/1-x?from=a', 'https://djinni.co/jobs/1-x/')).toBe(true);
    expect(sameVacancy('https://djinni.co/jobs/1-x', 'https://djinni.co/jobs/2-y')).toBe(false);
  });
});
