import { Injectable, Logger } from '@nestjs/common';
import * as cheerio from 'cheerio';
import { NormalizedJob, SearchQuery } from '@job-finder/shared';
import { ScrapingService } from '../../scraping/scraping.service';
import { JobSourceAdapter } from '../adapter.interface';

/**
 * Djinni (djinni.co) — Ukrainian dev job board, server-rendered HTML.
 * A session cookie (env DJINNI_SESSION_COOKIE or source config.sessionCookie)
 * unlocks full listings; anonymous browsing still returns public ones.
 * Selectors are best-effort and defensive: markup changes degrade this source only.
 */
@Injectable()
export class DjinniAdapter implements JobSourceAdapter {
  readonly key = 'djinni';
  readonly kind = 'scrape' as const;
  private readonly logger = new Logger(DjinniAdapter.name);

  constructor(private readonly scraping: ScrapingService) {}

  async search(query: SearchQuery, config: Record<string, unknown>): Promise<NormalizedJob[]> {
    const cookie = (config.sessionCookie as string) || process.env.DJINNI_SESSION_COOKIE;
    const params = new URLSearchParams();
    if (query.keywords) params.set('keywords', query.keywords);
    if (query.remote) params.set('employment', 'remote');
    const url = `https://djinni.co/jobs/?${params.toString()}`;

    const headers: Record<string, string> = {};
    if (cookie) headers['Cookie'] = `sessionid=${cookie}`;
    const html = await this.scraping.fetchHtml(url, headers);
    const $ = cheerio.load(html);

    const jobs: NormalizedJob[] = [];
    // list items look like <li id="job-item-123456"> with a title link to /jobs/<id>-slug/
    $('[id^="job-item-"], li.list-jobs__item').each((_, el) => {
      const item = $(el);
      const link = item.find('a[href^="/jobs/"]').first();
      const href = link.attr('href');
      const title = link.text().trim();
      if (!href || !title) return;

      const company = item
        .find('a[href^="/company/"], .js-analytics-company, [data-analytics="company_page"]')
        .first()
        .text()
        .trim();
      const description = item.find('.js-truncated-text, .js-original-text, .text-card').first().text().trim();
      const salary = item.find('.public-salary-item, .text-success').first().text().trim();
      const location = item.find('.location-text').first().text().trim();

      jobs.push({
        sourceKey: this.key,
        externalId: href.split('/').filter(Boolean).pop(),
        title,
        company: company || 'Unknown',
        location: location || undefined,
        remote: /remote|віддалено/i.test(item.text()),
        salaryRaw: salary || undefined,
        url: new URL(href, 'https://djinni.co').toString(),
        description,
        raw: { html: item.html()?.slice(0, 4000) },
      });
    });

    if (jobs.length === 0) {
      this.logger.warn('djinni returned 0 jobs — markup may have changed or session cookie expired');
    }
    return jobs;
  }

  async healthCheck(): Promise<boolean> {
    try {
      const html = await this.scraping.fetchHtml('https://djinni.co/jobs/');
      return html.includes('djinni');
    } catch {
      return false;
    }
  }
}
