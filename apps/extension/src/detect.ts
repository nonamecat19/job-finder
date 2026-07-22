import type { ProfileField, AtsVendor, DetectedField, DetectionResult } from './types.js'
import { AUTOCOMPLETE_MAP, GREENHOUSE_FIELD_MAP, LEVER_FIELD_MAP, WORKDAY_FIELD_MAP } from './maps.js'
import { matchLabelToField } from './heuristics.js'

function getFieldMap(ats: AtsVendor): Record<string, ProfileField> {
  switch (ats) {
    case 'greenhouse': return GREENHOUSE_FIELD_MAP
    case 'lever': return LEVER_FIELD_MAP
    case 'workday': return WORKDAY_FIELD_MAP
    default: return {}
  }
}

export function getLabelText(el: HTMLElement): string {
  if (
    (el instanceof HTMLInputElement ||
      el instanceof HTMLTextAreaElement ||
      el instanceof HTMLSelectElement) &&
    el.labels &&
    el.labels.length > 0
  ) {
    for (const label of el.labels) {
      const text = label.textContent?.trim()
      if (text) return text
    }
  }

  const labelledBy = el.getAttribute('aria-labelledby')
  if (labelledBy) {
    const labelEl = el.ownerDocument?.getElementById(labelledBy)
    if (labelEl) {
      return labelEl.textContent?.trim() ?? ''
    }
  }

  return ''
}

function detectField(
  el: HTMLElement,
  ats: AtsVendor,
): DetectedField {
  const isInput =
    el instanceof HTMLInputElement ||
    el instanceof HTMLTextAreaElement ||
    el instanceof HTMLSelectElement
  if (!isInput) {
    return { element: el, profileField: null, method: 'unknown' }
  }

  const input = el as HTMLInputElement

  if (input.autocomplete) {
    const token = input.autocomplete.toLowerCase()
    const profileField = AUTOCOMPLETE_MAP[token]
    if (profileField) {
      return { element: el, profileField, method: 'autocomplete' }
    }
  }

  const name = input.getAttribute('name')
  if (name) {
    const atsMap = getFieldMap(ats)
    const profileField = atsMap[name]
    if (profileField) {
      return { element: el, profileField, method: 'name' }
    }
  }

  const labelText = getLabelText(el)
  if (labelText) {
    const profileField = matchLabelToField(labelText)
    if (profileField) {
      return { element: el, profileField, method: 'label' }
    }
  }

  const ariaLabel = el.getAttribute('aria-label')
  if (ariaLabel) {
    const profileField = matchLabelToField(ariaLabel)
    if (profileField) {
      return { element: el, profileField, method: 'aria-label' }
    }
  }

  return { element: el, profileField: null, method: 'unknown' }
}

export function detectFormFields(
  container: HTMLElement,
  ats: AtsVendor,
): DetectionResult {
  const elements = container.querySelectorAll<HTMLElement>(
    'input, select, textarea',
  )

  const fields: DetectedField[] = []
  for (const el of elements) {
    fields.push(detectField(el, ats))
  }

  return { ats, fields }
}
