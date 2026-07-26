import { describe, it, expect } from 'vitest'
import { summarizeExpLevels, summarizeDjinniBasicSearch } from './djinniSearchSummary'

describe('summarizeExpLevels', () => {
  it('collapses consecutive 2y,3y,4y,5y to "2–5 years"', () => {
    expect(summarizeExpLevels(['2y', '3y', '4y', '5y'])).toBe('2\u20135 years')
  })

  it('collapses consecutive 1y,2y,3y to "1–3 years"', () => {
    expect(summarizeExpLevels(['1y', '2y', '3y'])).toBe('1\u20133 years')
  })

  it('renders non-consecutive 1y,3y as "1, 3 years"', () => {
    expect(summarizeExpLevels(['1y', '3y'])).toBe('1, 3 years')
  })

  it('deduplicates 2y,2y to "2 years"', () => {
    expect(summarizeExpLevels(['2y', '2y'])).toBe('2 years')
  })

  it('sorts out-of-order 3y,1y,2y to "1–3 years"', () => {
    expect(summarizeExpLevels(['3y', '1y', '2y'])).toBe('1\u20133 years')
  })

  it('returns empty string for empty input', () => {
    expect(summarizeExpLevels([])).toBe('')
  })

  it('renders single value as "3 years"', () => {
    expect(summarizeExpLevels(['3y'])).toBe('3 years')
  })

  it('falls back to list for non-integer token', () => {
    expect(summarizeExpLevels(['senior'])).toBe('senior')
  })

  it('uses en-dash character not hyphen', () => {
    const result = summarizeExpLevels(['2y', '3y', '4y', '5y'])
    expect(result).toBe('2\u20135 years')
    expect(result).not.toContain('-')
  })
})

describe('summarizeDjinniBasicSearch', () => {
  const nodeURL =
    'https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Node.js&salary=3000&exp_level=2y&exp_level=3y&exp_level=4y&exp_level=5y&employment=remote'

  const goURL =
    'https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang&salary=1500&exp_level=1y&exp_level=2y&exp_level=3y&employment=remote'

  it('renders Node.js summary with all filters', () => {
    const result = summarizeDjinniBasicSearch(nodeURL)
    expect(result).toBe('Node.js · $3000 · 2\u20135 years · remote')
  })

  it('renders Golang summary with all filters', () => {
    const result = summarizeDjinniBasicSearch(goURL)
    expect(result).toBe('Golang · $1500 · 1\u20133 years · remote')
  })

  it('omits absent filters cleanly', () => {
    const result = summarizeDjinniBasicSearch(
      'https://djinni.co/jobs/?search_type=basic-search&primary_keyword=Golang',
    )
    expect(result).toBe('Golang')
  })

  it('returns null for dashboard subs/{id}/ URL', () => {
    const result = summarizeDjinniBasicSearch('https://djinni.co/my/dashboard/subs/123/')
    expect(result).toBeNull()
  })

  it('returns null for non-djinni URL', () => {
    const result = summarizeDjinniBasicSearch('https://example.com/jobs/?search_type=basic-search&primary_keyword=Go')
    expect(result).toBeNull()
  })

  it('returns null for empty URL', () => {
    expect(summarizeDjinniBasicSearch('')).toBeNull()
  })

  it('returns null for garbage URL', () => {
    expect(summarizeDjinniBasicSearch('not a url')).toBeNull()
  })
})
