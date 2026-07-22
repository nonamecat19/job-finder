import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { detectFormFields } from './detect.js'
import { buildFillPlan, applyFillEntry, resolveFieldValue, highlightField, clearHighlight } from './fill.js'
import type { ExtProfile } from './types.js'

function loadFixture(name: string): string {
  return readFileSync(resolve(__dirname, '..', 'fixtures', `${name}.html`), 'utf-8')
}

function testProfile(overrides: Partial<ExtProfile> = {}): ExtProfile {
  return {
    fullName: 'Jane Doe',
    email: 'jane@example.com',
    phone: '+1-555-1234',
    location: 'San Francisco, CA',
    headline: 'Senior Frontend Engineer',
    skills: ['React', 'TypeScript', 'Go'],
    workHistory: [],
    education: [],
    links: [{ url: 'https://linkedin.com/in/janedoe', label: 'LinkedIn' }],
    ...overrides,
  }
}

describe('buildFillPlan (Greenhouse fixture)', () => {
  it('resolves values for every detected profile field and skips unmatched ones', () => {
    document.body.innerHTML = loadFixture('greenhouse')
    const detection = detectFormFields(document.body, 'greenhouse')
    const plan = buildFillPlan(detection.fields, testProfile())

    const byField = (id: string) => plan.find((p) => p.element.id === id)

    expect(byField('first_name')?.value).toBe('Jane Doe')
    expect(byField('email')?.value).toBe('jane@example.com')
    expect(byField('phone')?.value).toBe('+1-555-1234')
    expect(byField('job_city')?.value).toBe('San Francisco, CA')
    expect(byField('headline')?.value).toBe('Senior Frontend Engineer')
    expect(byField('skills')?.value).toBe('React, TypeScript, Go')
    expect(byField('linkedin_profile_url')?.value).toBe('https://linkedin.com/in/janedoe')

    // "How did you hear about this job?" has no profileField — never in the plan.
    expect(byField('how_did_you_hear')).toBeUndefined()
  })

  it('drops fields whose resolved value is empty (e.g. missing profile data)', () => {
    document.body.innerHTML = loadFixture('greenhouse')
    const detection = detectFormFields(document.body, 'greenhouse')
    const plan = buildFillPlan(detection.fields, testProfile({ phone: '' }))

    expect(plan.find((p) => p.element.id === 'phone')).toBeUndefined()
  })
})

describe('applyFillEntry', () => {
  it('sets an input value and dispatches input+change events', () => {
    document.body.innerHTML = '<input id="name" name="name" />'
    const el = document.getElementById('name') as HTMLInputElement
    let inputFired = false
    let changeFired = false
    el.addEventListener('input', () => (inputFired = true))
    el.addEventListener('change', () => (changeFired = true))

    const applied = applyFillEntry({ element: el, profileField: 'fullName', value: 'Jane Doe' })

    expect(applied).toBe(true)
    expect(el.value).toBe('Jane Doe')
    expect(inputFired).toBe(true)
    expect(changeFired).toBe(true)
  })

  it('sets a textarea value', () => {
    document.body.innerHTML = '<textarea id="skills" name="skills"></textarea>'
    const el = document.getElementById('skills') as HTMLTextAreaElement
    const applied = applyFillEntry({ element: el, profileField: 'skills', value: 'React, Go' })
    expect(applied).toBe(true)
    expect(el.value).toBe('React, Go')
  })

  it('matches a select option by visible text, case-insensitively', () => {
    document.body.innerHTML =
      '<select id="loc"><option value="">--</option><option value="sf">San Francisco</option></select>'
    const el = document.getElementById('loc') as HTMLSelectElement
    const applied = applyFillEntry({ element: el, profileField: 'location', value: 'san francisco' })
    expect(applied).toBe(true)
    expect(el.value).toBe('sf')
  })

  it('leaves a select untouched when no option matches (never guesses)', () => {
    document.body.innerHTML =
      '<select id="loc"><option value="">--</option><option value="ny">New York</option></select>'
    const el = document.getElementById('loc') as HTMLSelectElement
    const before = el.value
    const applied = applyFillEntry({ element: el, profileField: 'location', value: 'San Francisco' })
    expect(applied).toBe(false)
    expect(el.value).toBe(before)
  })

  it('never fills a checkbox or radio input, even if a plan entry is somehow built for one', () => {
    // Defense in depth: buildFillPlan already excludes these input types, but
    // applyFillEntry re-checks so a bad plan can never write to one.
    document.body.innerHTML = '<input id="c" type="checkbox" />'
    const el = document.getElementById('c') as HTMLInputElement
    const applied = applyFillEntry({ element: el, profileField: 'fullName', value: 'Jane Doe' })
    expect(applied).toBe(false)
    expect(el.checked).toBe(false)
  })
})

describe('buildFillPlan input-type guard', () => {
  it('excludes checkbox/radio/file/hidden inputs from the plan even if detected', () => {
    document.body.innerHTML = '<input id="c" type="checkbox" name="name" autocomplete="name" />'
    const detection = detectFormFields(document.body, 'generic')
    const plan = buildFillPlan(detection.fields, testProfile())
    expect(plan.length).toBe(0)
  })
})

describe('resolveFieldValue: links disambiguation', () => {
  it('fills the only link when there is exactly one', () => {
    document.body.innerHTML = '<input id="url" name="portfolio_url" />'
    const field = { element: document.getElementById('url') as HTMLElement, profileField: 'links' as const, method: 'name' as const }
    const value = resolveFieldValue(field, testProfile())
    expect(value).toBe('https://linkedin.com/in/janedoe')
  })

  it('picks the link whose label matches the field name/label text when there are multiple', () => {
    document.body.innerHTML = '<label for="gh">GitHub URL</label><input id="gh" name="github_url" />'
    const field = { element: document.getElementById('gh') as HTMLElement, profileField: 'links' as const, method: 'name' as const }
    const profile = testProfile({
      links: [
        { url: 'https://linkedin.com/in/janedoe', label: 'LinkedIn' },
        { url: 'https://github.com/janedoe', label: 'GitHub' },
      ],
    })
    expect(resolveFieldValue(field, profile)).toBe('https://github.com/janedoe')
  })

  it('leaves the field blank when multiple links exist and none can be disambiguated', () => {
    document.body.innerHTML = '<input id="url" name="portfolio_url" />'
    const field = { element: document.getElementById('url') as HTMLElement, profileField: 'links' as const, method: 'name' as const }
    const profile = testProfile({
      links: [
        { url: 'https://linkedin.com/in/janedoe', label: 'LinkedIn' },
        { url: 'https://github.com/janedoe', label: 'GitHub' },
      ],
    })
    expect(resolveFieldValue(field, profile)).toBe('')
  })

  it('leaves the field blank when the profile has no links', () => {
    document.body.innerHTML = '<input id="url" name="portfolio_url" />'
    const field = { element: document.getElementById('url') as HTMLElement, profileField: 'links' as const, method: 'name' as const }
    expect(resolveFieldValue(field, testProfile({ links: [] }))).toBe('')
  })
})

describe('highlightField / clearHighlight', () => {
  it('applies and removes a visible outline', () => {
    document.body.innerHTML = '<input id="x" />'
    const el = document.getElementById('x') as HTMLElement
    highlightField(el)
    expect(el.getAttribute('data-jf-filled')).toBe('true')
    expect(el.style.outline).toContain('16a34a')

    clearHighlight(el)
    expect(el.getAttribute('data-jf-filled')).toBeNull()
    expect(el.style.outline).toBe('')
  })
})
