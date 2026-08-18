
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

export function sameVacancy(a: string, b: string): boolean {
  return canonicalizeVacancyUrl(a) === canonicalizeVacancyUrl(b);
}
