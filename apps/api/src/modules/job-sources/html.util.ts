import * as cheerio from 'cheerio';

/** Strip HTML to readable plain text (descriptions from API sources arrive as HTML). */
export function htmlToText(html: string): string {
  if (!html) return '';
  const $ = cheerio.load(html);
  $('br').replaceWith('\n');
  $('p, li, div, h1, h2, h3, h4').each((_, el) => {
    $(el).append('\n');
  });
  return $.root()
    .text()
    .replace(/\n{3,}/g, '\n\n')
    .trim();
}
