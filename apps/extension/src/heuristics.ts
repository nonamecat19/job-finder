import type { ProfileField } from './types.js'

const LABEL_PATTERNS: [RegExp, ProfileField][] = [
  [/^(full\s+)?name$/i, 'fullName'],
  [/^first\s+name$/i, 'fullName'],
  [/^last\s+name$/i, 'fullName'],
  [/^email/i, 'email'],
  [/^phone/i, 'phone'],
  [/^mobile/i, 'phone'],
  [/^telephone/i, 'phone'],
  [/^location/i, 'location'],
  [/^city/i, 'location'],
  [/^headline/i, 'headline'],
  [/^title/i, 'headline'],
  [/^skills/i, 'skills'],
  [/^technologies/i, 'skills'],
  [/^linkedin/i, 'links'],
  [/^portfolio/i, 'links'],
  [/^github/i, 'links'],
  [/^website/i, 'links'],
  [/^url/i, 'links'],
]

const compiledPatterns: { regex: RegExp; field: ProfileField }[] = LABEL_PATTERNS.map(
  ([regex, field]) => ({ regex, field }),
)

export function matchLabelToField(labelText: string): ProfileField | null {
  const text = labelText.trim()
  if (!text) return null

  for (const { regex, field } of compiledPatterns) {
    if (regex.test(text)) return field
  }

  return null
}
