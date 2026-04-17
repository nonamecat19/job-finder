import { Injectable } from '@nestjs/common';
import { mkdir, readFile, writeFile } from 'fs/promises';
import * as path from 'path';
import Handlebars from 'handlebars';
import { JsonResume } from '@job-finder/shared';
import { ScrapingService } from '../scraping/scraping.service';

export interface PdfRenderer {
  renderResume(resume: JsonResume, outName: string): Promise<string>;
  renderCoverLetter(text: string, meta: { name?: string; company: string; title: string }, outName: string): Promise<string>;
}

export const PDF_RENDERER = Symbol('PDF_RENDERER');

/**
 * v1 renderer: Handlebars HTML template → chromium page.pdf() (shared Playwright
 * browser from ScrapingService). RenderCV or others can slot in behind PdfRenderer.
 */
@Injectable()
export class HtmlPdfRenderer implements PdfRenderer {
  private templates = new Map<string, Handlebars.TemplateDelegate>();

  constructor(private readonly scraping: ScrapingService) {}

  private get outDir(): string {
    return process.env.DOCUMENTS_DIR ?? '/data/documents';
  }

  private async template(name: string): Promise<Handlebars.TemplateDelegate> {
    let tpl = this.templates.get(name);
    if (!tpl) {
      const file = path.join(__dirname, 'templates', `${name}.hbs`);
      tpl = Handlebars.compile(await readFile(file, 'utf8'));
      this.templates.set(name, tpl);
    }
    return tpl;
  }

  async renderResume(resume: JsonResume, outName: string): Promise<string> {
    const tpl = await this.template('resume');
    return this.htmlToPdf(tpl({ r: resume }), outName);
  }

  async renderCoverLetter(
    text: string,
    meta: { name?: string; company: string; title: string },
    outName: string,
  ): Promise<string> {
    const tpl = await this.template('cover-letter');
    const paragraphs = text
      .split(/\n{2,}|\n/)
      .map((p) => p.trim())
      .filter(Boolean);
    return this.htmlToPdf(tpl({ paragraphs, ...meta }), outName);
  }

  private async htmlToPdf(html: string, outName: string): Promise<string> {
    await mkdir(this.outDir, { recursive: true });
    const outPath = path.join(this.outDir, outName);
    const browser = await this.scraping.getBrowser();
    const page = await browser.newPage();
    try {
      await page.setContent(html, { waitUntil: 'networkidle' });
      const pdf = await page.pdf({
        format: 'A4',
        printBackground: true,
        margin: { top: '14mm', bottom: '14mm', left: '14mm', right: '14mm' },
      });
      await writeFile(outPath, pdf);
      return outPath;
    } finally {
      await page.close().catch(() => undefined);
    }
  }
}
