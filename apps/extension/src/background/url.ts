/**
 * Vacancy URLs reach us with tracking noise the stored URL does not have
 * (djinni appends ?from=, dou appends ?from=feed), and hosts differ by a "www."
 * that means nothing. Canonicalise before matching or caching.
 *
 * The path is kept verbatim apart from a trailing slash: the vacancy id lives
 * there.
 */
export function canonicalizeVacancyUrl(raw: string): string {
  let u: URL;
  try {
    u = new URL(raw);
  } catch {
    return raw.trim();
  }
  const host = u.hostname.toLowerCase().replace(/^www\./, '');
  const path = u.pathname.replace(/\/+$/, '');
  return `${u.protocol}//${host}${path}`;
}

/** True when two URLs point at the same vacancy once the noise is stripped. */
export function sameVacancy(a: string, b: string): boolean {
  return canonicalizeVacancyUrl(a) === canonicalizeVacancyUrl(b);
}
