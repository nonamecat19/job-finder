// Content script (spec 014-autofill-extension 1.3): detects fields, asks the
// service worker for the matching profile, shows what would be filled, and
// only writes to the DOM after the user explicitly confirms. It never calls
// form.submit() or clicks a submit button, and it never touches an auth
// token (see auth.ts's doc comment) — it only ever talks to the service
// worker via chrome.runtime.sendMessage.
import { detectAts } from './ats.js'
import { detectFormFields } from './detect.js'
import { buildFillPlan, applyFillEntry, highlightField, type FillPlanEntry } from './fill.js'
import type { ExtensionResponse, ExtProfile } from './types.js'

const OVERLAY_ID = 'jf-autofill-overlay'
const FIELD_LABELS: Record<string, string> = {
  fullName: 'Full name',
  email: 'Email',
  phone: 'Phone',
  location: 'Location',
  headline: 'Headline',
  skills: 'Skills',
  links: 'Link',
}

function sendMessage<T>(message: unknown): Promise<ExtensionResponse<T>> {
  return chrome.runtime.sendMessage(message)
}

function removeOverlay(): void {
  document.getElementById(OVERLAY_ID)?.remove()
}

function renderReadyBadge(plan: FillPlanEntry[]): void {
  removeOverlay()

  const overlay = document.createElement('div')
  overlay.id = OVERLAY_ID
  Object.assign(overlay.style, {
    position: 'fixed',
    bottom: '20px',
    right: '20px',
    zIndex: '2147483647',
    fontFamily: 'system-ui, sans-serif',
    fontSize: '13px',
    background: '#111827',
    color: '#f9fafb',
    borderRadius: '10px',
    boxShadow: '0 8px 24px rgba(0,0,0,0.35)',
    padding: '12px 14px',
    maxWidth: '280px',
  } satisfies Partial<CSSStyleDeclaration>)

  const heading = document.createElement('div')
  heading.textContent = `Job Finder: ${plan.length} field${plan.length === 1 ? '' : 's'} ready to fill`
  heading.style.fontWeight = '700'
  heading.style.marginBottom = '8px'
  overlay.appendChild(heading)

  const reviewBtn = document.createElement('button')
  reviewBtn.type = 'button'
  reviewBtn.textContent = 'Review & Fill'
  styleButton(reviewBtn, '#2563eb')
  reviewBtn.addEventListener('click', () => renderConfirmPanel(plan))
  overlay.appendChild(reviewBtn)

  const dismissBtn = document.createElement('button')
  dismissBtn.type = 'button'
  dismissBtn.textContent = 'Dismiss'
  styleButton(dismissBtn, 'transparent')
  dismissBtn.style.marginLeft = '8px'
  dismissBtn.addEventListener('click', removeOverlay)
  overlay.appendChild(dismissBtn)

  document.body.appendChild(overlay)
}

function renderConfirmPanel(plan: FillPlanEntry[]): void {
  removeOverlay()

  const overlay = document.createElement('div')
  overlay.id = OVERLAY_ID
  Object.assign(overlay.style, {
    position: 'fixed',
    bottom: '20px',
    right: '20px',
    zIndex: '2147483647',
    fontFamily: 'system-ui, sans-serif',
    fontSize: '13px',
    background: '#111827',
    color: '#f9fafb',
    borderRadius: '10px',
    boxShadow: '0 8px 24px rgba(0,0,0,0.35)',
    padding: '14px',
    width: '300px',
    maxHeight: '70vh',
    overflowY: 'auto',
  } satisfies Partial<CSSStyleDeclaration>)

  const heading = document.createElement('div')
  heading.textContent = 'Confirm fields to fill'
  heading.style.fontWeight = '700'
  heading.style.marginBottom = '8px'
  overlay.appendChild(heading)

  const note = document.createElement('div')
  note.textContent = "This only fills the form below — you still click the site's own submit button."
  note.style.color = '#9ca3af'
  note.style.marginBottom = '10px'
  overlay.appendChild(note)

  const list = document.createElement('div')
  list.style.display = 'flex'
  list.style.flexDirection = 'column'
  list.style.gap = '6px'
  list.style.marginBottom = '10px'

  const checkboxes: { checkbox: HTMLInputElement; entry: FillPlanEntry }[] = []
  for (const entry of plan) {
    const row = document.createElement('label')
    row.style.display = 'flex'
    row.style.alignItems = 'flex-start'
    row.style.gap = '6px'
    row.style.cursor = 'pointer'

    const checkbox = document.createElement('input')
    checkbox.type = 'checkbox'
    checkbox.checked = true
    row.appendChild(checkbox)

    const text = document.createElement('span')
    const label = FIELD_LABELS[entry.profileField] ?? entry.profileField
    text.textContent = `${label}: ${entry.value}`
    row.appendChild(text)

    list.appendChild(row)
    checkboxes.push({ checkbox, entry })
  }
  overlay.appendChild(list)

  const fillBtn = document.createElement('button')
  fillBtn.type = 'button'
  fillBtn.textContent = 'Fill selected'
  styleButton(fillBtn, '#16a34a')
  fillBtn.addEventListener('click', () => {
    const selected = checkboxes.filter((c) => c.checkbox.checked).map((c) => c.entry)
    applyFill(selected)
  })
  overlay.appendChild(fillBtn)

  const cancelBtn = document.createElement('button')
  cancelBtn.type = 'button'
  cancelBtn.textContent = 'Cancel'
  styleButton(cancelBtn, 'transparent')
  cancelBtn.style.marginLeft = '8px'
  cancelBtn.addEventListener('click', removeOverlay)
  overlay.appendChild(cancelBtn)

  document.body.appendChild(overlay)
}

function applyFill(entries: FillPlanEntry[]): void {
  let filled = 0
  for (const entry of entries) {
    // applyFillEntry only ever writes .value + dispatches input/change —
    // it never calls form.submit() or clicks anything (see fill.ts).
    if (applyFillEntry(entry)) {
      highlightField(entry.element)
      filled += 1
    }
  }
  renderDoneMessage(filled)
}

function renderDoneMessage(filled: number): void {
  removeOverlay()
  const overlay = document.createElement('div')
  overlay.id = OVERLAY_ID
  Object.assign(overlay.style, {
    position: 'fixed',
    bottom: '20px',
    right: '20px',
    zIndex: '2147483647',
    fontFamily: 'system-ui, sans-serif',
    fontSize: '13px',
    background: '#111827',
    color: '#f9fafb',
    borderRadius: '10px',
    boxShadow: '0 8px 24px rgba(0,0,0,0.35)',
    padding: '12px 14px',
    maxWidth: '280px',
  } satisfies Partial<CSSStyleDeclaration>)
  overlay.textContent = `Filled ${filled} field${filled === 1 ? '' : 's'} — review, then submit yourself.`
  document.body.appendChild(overlay)
  setTimeout(removeOverlay, 6000)
}

function styleButton(btn: HTMLButtonElement, background: string): void {
  Object.assign(btn.style, {
    background,
    color: '#f9fafb',
    border: background === 'transparent' ? '1px solid #4b5563' : 'none',
    borderRadius: '6px',
    padding: '6px 10px',
    fontSize: '12px',
    fontWeight: '600',
    cursor: 'pointer',
  } satisfies Partial<CSSStyleDeclaration>)
}

async function run(): Promise<void> {
  const ats = detectAts(document)
  const detection = detectFormFields(document.body, ats)
  const candidateCount = detection.fields.filter((f) => f.profileField !== null).length
  if (candidateCount === 0) return

  const response = await sendMessage<ExtProfile>({ type: 'profile:get' })
  if (!response.ok) {
    // Not authenticated (or profile fetch failed) — nothing to show. The
    // user connects via the popup, not via a page overlay demanding auth.
    return
  }

  const plan = buildFillPlan(detection.fields, response.data)
  if (plan.length === 0) return

  renderReadyBadge(plan)
}

run().catch((err) => {
  console.error('[Job Finder] content script error', err)
})

// The popup's "Fill Form" button re-runs detection on demand (e.g. the page
// finished loading before the user connected the extension).
chrome.runtime.onMessage.addListener((message: { type?: string }) => {
  if (message?.type === 'fill:trigger') {
    run().catch((err) => console.error('[Job Finder] content script error', err))
  }
})
