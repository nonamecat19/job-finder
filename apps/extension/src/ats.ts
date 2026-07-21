import type { AtsVendor } from './types.js'

export function detectAts(doc: Document): AtsVendor {
  const url = doc.URL.toLowerCase()

  if (url.includes('greenhouse.io')) return 'greenhouse'
  if (url.includes('lever.co')) return 'lever'
  if (url.includes('myworkdayjobs.com') || url.includes('workday.com')) return 'workday'

  return 'generic'
}
