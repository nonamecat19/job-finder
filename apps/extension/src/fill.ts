// Fill strategies per field type (spec 014-autofill-extension 1.3/3.2/4).
// Pure DOM functions — no chrome.* APIs here — so they're unit-testable the
// same way detect.ts is (fixture HTML + vitest/jsdom).
import type { DetectedField, ExtLink, ExtProfile, ProfileField } from './types.js'
import { getLabelText } from './detect.js'

export interface FillPlanEntry {
  element: HTMLElement
  profileField: ProfileField
  value: string
}

// Input types we will never write into, even if detect.ts happened to tag
// one (e.g. a mis-tagged checkbox): there is no sound way to turn a scalar
// profile string into a checked/unchecked/file value, and guessing wrong is
// worse than leaving it — "honest omission" (spec section 4/6).
const UNSUPPORTED_INPUT_TYPES = new Set(['checkbox', 'radio', 'file', 'hidden', 'submit', 'button', 'reset', 'image'])

function isFillable(el: HTMLElement): el is HTMLInputElement | HTMLTextAreaElement | HTMLSelectElement {
  if (el instanceof HTMLInputElement) return !UNSUPPORTED_INPUT_TYPES.has(el.type)
  return el instanceof HTMLTextAreaElement || el instanceof HTMLSelectElement
}

/** Picks which link to use for a `links` field. Multiple links are only
 * ever filled when the field's name/label/aria-label lets us tell which one
 * is meant (e.g. "LinkedIn URL" -> the link labeled "LinkedIn"); otherwise
 * the field is left blank rather than guessing (spec 4: "Ambiguous fields
 * are left unfilled"). */
function resolveLinkValue(el: HTMLElement, links: ExtLink[]): string {
  if (links.length === 0) return ''
  if (links.length === 1) return links[0].url

  const hint = [el.getAttribute('name'), el.getAttribute('aria-label'), getLabelText(el)]
    .filter(Boolean)
    .join(' ')
    .toLowerCase()
  if (!hint) return ''

  const match = links.find((l) => l.label.trim() !== '' && hint.includes(l.label.toLowerCase()))
  return match ? match.url : ''
}

export function resolveFieldValue(field: DetectedField, profile: ExtProfile): string {
  switch (field.profileField) {
    case 'fullName':
      return profile.fullName
    case 'email':
      return profile.email
    case 'phone':
      return profile.phone
    case 'location':
      return profile.location
    case 'headline':
      return profile.headline
    case 'skills':
      return profile.skills.join(', ')
    case 'links':
      return resolveLinkValue(field.element, profile.links)
    default:
      return ''
  }
}

/** Builds the ordered list of {element, value} pairs the user will be shown
 * and asked to confirm. Fields with no confident, non-empty value are
 * dropped entirely — they never appear in the confirmation UI and are never
 * touched (honest omission, spec 4/6). */
export function buildFillPlan(fields: DetectedField[], profile: ExtProfile): FillPlanEntry[] {
  const plan: FillPlanEntry[] = []
  for (const field of fields) {
    if (!field.profileField) continue
    if (!isFillable(field.element)) continue
    const value = resolveFieldValue(field, profile).trim()
    if (!value) continue
    plan.push({ element: field.element, profileField: field.profileField, value })
  }
  return plan
}

/** Sets an <option> by best-effort match against its value or visible text
 * (case-insensitive, substring). Returns true if a match was applied. A
 * <select> with no matching option is left untouched — never forced to an
 * arbitrary option (honest omission). */
function applySelect(el: HTMLSelectElement, value: string): boolean {
  const target = value.trim().toLowerCase()
  for (const option of Array.from(el.options)) {
    const optionValue = option.value.trim().toLowerCase()
    const optionText = option.text.trim().toLowerCase()
    if (optionValue === target || optionText === target) {
      el.value = option.value
      return true
    }
  }
  for (const option of Array.from(el.options)) {
    const optionText = option.text.trim().toLowerCase()
    if (optionText && (optionText.includes(target) || target.includes(optionText))) {
      el.value = option.value
      return true
    }
  }
  return false
}

/** Applies a single fill entry and dispatches input+change events so
 * React/Vue/Angular-controlled forms (which listen for those events, not
 * direct DOM property writes) pick up the new value — spec 3.2. Returns
 * true if the field was actually written. */
export function applyFillEntry(entry: FillPlanEntry): boolean {
  const el = entry.element
  if (!isFillable(el)) return false
  let applied = false

  if (el instanceof HTMLSelectElement) {
    applied = applySelect(el, entry.value)
  } else if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) {
    el.value = entry.value
    applied = true
  }

  if (applied) {
    el.dispatchEvent(new Event('input', { bubbles: true }))
    el.dispatchEvent(new Event('change', { bubbles: true }))
  }
  return applied
}

const HIGHLIGHT_ATTR = 'data-jf-filled'

/** Green-border highlight on a successfully filled field (spec 1.3 #4 /
 * 2.1's fill-UI flow) so the user can visually review before submitting. */
export function highlightField(el: HTMLElement): void {
  el.setAttribute(HIGHLIGHT_ATTR, 'true')
  el.style.outline = '2px solid #16a34a'
  el.style.outlineOffset = '1px'
}

export function clearHighlight(el: HTMLElement): void {
  el.removeAttribute(HIGHLIGHT_ATTR)
  el.style.outline = ''
  el.style.outlineOffset = ''
}
