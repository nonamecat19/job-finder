import { Injectable, Logger } from '@nestjs/common';
import * as cheerio from 'cheerio';
import { NormalizedJob, SearchQuery } from '@job-finder/shared';
import { ScrapingService } from '../../scraping/scraping.service';
import { JobSourceAdapter } from '../adapter.interface';

/**
 * DOU (jobs.dou.ua) — Ukrainian dev job board, server-rendered vacancy list.
 * Best-effort selectors; failures degrade this source only.
 */
@Injectable()
export class DouAdapter implements JobSourceAdapter {
  readonly key = 'dou';
  readonly kind = 'scrape' as const;
  private readonly logger = new Logger(DouAdapter.name);

  constructor(private readonly scraping: ScrapingService) {}

  async search(query: SearchQuery): Promise<NormalizedJob[]> {
    const params = new URLSearchParams();
    if (query.keywords) params.set('search', query.keywords);
    if (query.remote) params.set('remote', '');
    const url = `https://jobs.dou.ua/vacancies/?${params.toString()}`;

    const html = await this.scraping.fetchHtml(url);
    const $ = cheerio.load(html);

    const jobs: NormalizedJob[] = [];
    $('li.l-vacancy').each((_, el) => {
      const item = $(el);
      const link = item.find('a.vt').first();
      const href = link.attr('href');
      const title = link.text().trim();
      if (!href || !title) return;

      const company = item.find('a.company').first().text().trim();
      const cities = item.find('.cities').first().text().trim();
      const salary = item.find('span.salary').first().text().trim();
      const sh = item.find('.sh-info').first().text().trim();
      const date = item.find('.date').first().text().trim();

      jobs.push({
        sourceKey: this.key,
        externalId: href.split('/').filter(Boolean).pop(),
        title,
        company: company || 'Unknown',
        location: cities || undefined,
        remote: /remote|віддалено/i.test(cities),
        salaryRaw: salary || undefined,
        url: href,
        description: sh,
        postedAt: undefined, // DOU renders relative/localized dates ("14 июня"); skip parsing
        raw: { date, html: item.html()?.slice(0, 4000) },
      });
    });

    if (jobs.length === 0) {
      this.logger.warn('dou returned 0 jobs — markup may have changed');
    }
    return jobs;
  }

  async healthCheck(): Promise<boolean> {
    try {
      const html = await this.scraping.fetchHtml('https://jobs.dou.ua/vacancies/');
      return html.includes('vacanc');
    } catch {
      return false;
    }
  }
}
