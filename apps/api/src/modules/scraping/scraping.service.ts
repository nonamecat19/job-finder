import { Injectable, Logger, OnModuleDestroy } from '@nestjs/common';
import axios from 'axios';
import { Browser, chromium } from 'playwright';

const UA =
  'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36';

@Injectable()
export class ScrapingService implements OnModuleDestroy {
  private readonly logger = new Logger(ScrapingService.name);
  private browser?: Browser;

  /** Fetch server-rendered HTML over plain HTTP with a browser-like UA. */
  async fetchHtml(url: string, headers: Record<string, string> = {}): Promise<string> {
    const res = await axios.get<string>(url, {
      responseType: 'text',
      timeout: 20_000,
      headers: { 'User-Agent': UA, ...headers },
      // treat only 5xx as errors; 4xx pages are still parseable HTML
      validateStatus: (s) => s < 500,
    });
    return typeof res.data === 'string' ? res.data : String(res.data);
  }

  /** Lazily launch a shared headless Chromium instance (used for PDF rendering). */
  async getBrowser(): Promise<Browser> {
    if (!this.browser || !this.browser.isConnected()) {
      this.browser = await chromium.launch({ headless: true });
    }
    return this.browser;
  }

  async onModuleDestroy() {
    if (this.browser) {
      await this.browser.close().catch((e) => this.logger.warn(`browser close failed: ${e}`));
      this.browser = undefined;
    }
  }
}
