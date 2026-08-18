
export function summarizeDjinniBasicSearch(url: string): string | null {
  if (!url) return null

  let parsed: URL
  try {
    parsed = new URL(url)
  } catch {
    return null
  }

  if (parsed.hostname !== 'djinni.co' && parsed.hostname !== 'www.djinni.co') {
    return null
  }

  if (
    !(parsed.pathname === '/jobs' || parsed.pathname === '/jobs/') ||
    parsed.searchParams.get('search_type') !== 'basic-search'
  ) {
    return null
  }

  const parts: string[] = []

  const keyword = parsed.searchParams.get('primary_keyword')
  if (keyword) parts.push(keyword)

  const salary = parsed.searchParams.get('salary')
  if (salary) parts.push(`$${salary}`)

  const expLevels = parsed.searchParams.getAll('exp_level')
  if (expLevels.length > 0) {
    const summary = summarizeExpLevels(expLevels)
    if (summary) parts.push(summary)
  }

  const employment = parsed.searchParams.get('employment')
  if (employment) parts.push(employment)

  if (parts.length === 0) return null
  return parts.join(' · ')
}

const EN_DASH = '\u2013'

function deduplicateInOrder(values: string[]): string[] {
  const seen = new Set<string>()
  const result: string[] = []
  for (const v of values) {
    if (!seen.has(v)) {
      seen.add(v)
      result.push(v)
    }
  }
  return result
}

export function summarizeExpLevels(values: string[]): string {
  if (values.length === 0) return ''

  const deduped = deduplicateInOrder(values)

  const years: number[] = []
  for (const v of deduped) {
    if (!v.endsWith('y')) {

      const list = [...new Set(values)].sort()
      return list.join(', ')
    }
    const n = parseInt(v.slice(0, -1), 10)
    if (isNaN(n)) {
      const list = [...new Set(values)].sort()
      return list.join(', ')
    }
    years.push(n)
  }

  years.sort((a, b) => a - b)
  if (years.length === 1) {
    return `${years[0]} years`
  }

  let consecutive = true
  for (let i = 1; i < years.length; i++) {
    if (years[i] - years[i - 1] !== 1) {
      consecutive = false
      break
    }
  }

  if (consecutive) {
    return `${years[0]}${EN_DASH}${years[years.length - 1]} years`
  }

  return `${years.join(', ')} years`
}
