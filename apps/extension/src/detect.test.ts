import { describe, it, expect } from 'vitest'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { detectFormFields } from './detect.js'
import { matchLabelToField } from './heuristics.js'
import { detectAts } from './ats.js'
import type { AtsVendor } from './types.js'

function loadFixture(name: string): string {
  return readFileSync(
    resolve(__dirname, '..', 'fixtures', `${name}.html`),
    'utf-8',
  )
}

function testDetection(
  fixtureName: string,
  ats: AtsVendor,
  expected: Record<string, string | null>,
) {
  const html = loadFixture(fixtureName)
  document.body.innerHTML = html

  const result = detectFormFields(document.body, ats)

  for (const [id, profileField] of Object.entries(expected)) {
    const field = result.fields.find(
      (f) => (f.element as HTMLElement).id === id,
    )
    expect(
      field,
      `Expected field #${id} to be detected`,
    ).toBeDefined()
    expect(
      field!.profileField,
      `Expected #${id} to map to ${profileField}, got ${field!.profileField}`,
    ).toBe(profileField)
  }
}

describe('heuristics: matchLabelToField', () => {
  it('matches common field labels', () => {
    expect(matchLabelToField('Name')).toBe('fullName')
    expect(matchLabelToField('Full Name')).toBe('fullName')
    expect(matchLabelToField('Email')).toBe('email')
    expect(matchLabelToField('Email Address')).toBe('email')
    expect(matchLabelToField('Phone')).toBe('phone')
    expect(matchLabelToField('Mobile')).toBe('phone')
    expect(matchLabelToField('Location')).toBe('location')
    expect(matchLabelToField('Headline')).toBe('headline')
    expect(matchLabelToField('Skills')).toBe('skills')
    expect(matchLabelToField('Technologies')).toBe('skills')
    expect(matchLabelToField('LinkedIn')).toBe('links')
    expect(matchLabelToField('Portfolio')).toBe('links')
  })

  it('returns null for unmatchable labels', () => {
    expect(matchLabelToField('Additional Information')).toBeNull()
    expect(matchLabelToField('Notes')).toBeNull()
    expect(matchLabelToField('How did you hear about us?')).toBeNull()
    expect(matchLabelToField('Salary Expectations')).toBeNull()
    expect(matchLabelToField('Cover Letter')).toBeNull()
    expect(matchLabelToField('')).toBeNull()
  })
})

describe('Greenhouse form detection', () => {
  it('maps all known fields correctly', () => {
    testDetection('greenhouse', 'greenhouse', {
      first_name: 'fullName',
      last_name: 'fullName',
      email: 'email',
      phone: 'phone',
      job_city: 'location',
      headline: 'headline',
      skills: 'skills',
      linkedin_profile_url: 'links',
    })
  })

  it('leaves unmatchable fields blank', () => {
    const html = loadFixture('greenhouse')
    document.body.innerHTML = html
    const result = detectFormFields(document.body, 'greenhouse')

    const unmapped = result.fields.find(
      (f) => (f.element as HTMLElement).id === 'how_did_you_hear',
    )
    expect(unmapped).toBeDefined()
    expect(unmapped!.profileField).toBeNull()
    expect(unmapped!.method).toBe('unknown')
  })

  it('reports correct detection method for autocomplete fields', () => {
    const html = loadFixture('greenhouse')
    document.body.innerHTML = html
    const result = detectFormFields(document.body, 'greenhouse')

    const email = result.fields.find(
      (f) => (f.element as HTMLElement).id === 'email',
    )
    expect(email?.method).toBe('autocomplete')
  })
})

describe('Lever form detection', () => {
  it('maps all known fields correctly', () => {
    testDetection('lever', 'lever', {
      'lever-name': 'fullName',
      'lever-email': 'email',
      'lever-phone': 'phone',
      'lever-headline': 'headline',
      'lever-skills': 'skills',
      'lever-linkedin': 'links',
    })
  })

  it('leaves unmatchable fields blank', () => {
    const html = loadFixture('lever')
    document.body.innerHTML = html
    const result = detectFormFields(document.body, 'lever')

    const unmapped = result.fields.find(
      (f) => (f.element as HTMLElement).id === 'lever-source',
    )
    expect(unmapped).toBeDefined()
    expect(unmapped!.profileField).toBeNull()
  })
})

describe('Workday form detection', () => {
  it('maps all known fields correctly', () => {
    testDetection('workday', 'workday', {
      firstName: 'fullName',
      lastName: 'fullName',
      email: 'email',
      phone: 'phone',
      location: 'location',
      headline: 'headline',
      skill: 'skills',
    })
  })

  it('leaves unmatchable fields blank', () => {
    const html = loadFixture('workday')
    document.body.innerHTML = html
    const result = detectFormFields(document.body, 'workday')

    const unmapped = result.fields.find(
      (f) => (f.element as HTMLElement).id === 'howHeard',
    )
    expect(unmapped).toBeDefined()
    expect(unmapped!.profileField).toBeNull()
  })

  it('uses name method for fields in the workday map', () => {
    const html = loadFixture('workday')
    document.body.innerHTML = html
    const result = detectFormFields(document.body, 'workday')

    const phone = result.fields.find(
      (f) => (f.element as HTMLElement).id === 'phone',
    )
    expect(phone?.profileField).toBe('phone')
    expect(phone?.method).toBe('name')
  })

  it('falls through to aria-label when label for lacks association', () => {
    document.body.innerHTML =
      '<input id="x" name="x" aria-label="Email Address" />'
    const result = detectFormFields(document.body, 'workday')
    expect(result.fields[0].profileField).toBe('email')
    expect(result.fields[0].method).toBe('aria-label')
  })
})

describe('generic (fallback) detection', () => {
  it('uses autocomplete attribute when available', () => {
    document.body.innerHTML =
      '<input id="e" name="foo" autocomplete="email" />'
    const result = detectFormFields(document.body, 'generic')
    expect(result.fields[0].profileField).toBe('email')
    expect(result.fields[0].method).toBe('autocomplete')
  })

  it('falls through to label heuristics when no ATS map matches', () => {
    document.body.innerHTML =
      '<label for="p">Phone</label><input id="p" name="x_custom" />'
    const result = detectFormFields(document.body, 'generic')
    expect(result.fields[0].profileField).toBe('phone')
    expect(result.fields[0].method).toBe('label')
  })

  it('uses aria-label as final fallback before unknown', () => {
    document.body.innerHTML =
      '<input id="a" aria-label="LinkedIn Profile" />'
    const result = detectFormFields(document.body, 'generic')
    expect(result.fields[0].profileField).toBe('links')
    expect(result.fields[0].method).toBe('aria-label')
  })

  it('returns null for truly ambiguous fields', () => {
    document.body.innerHTML =
      '<input id="x" name="custom_field" />'
    const result = detectFormFields(document.body, 'generic')
    expect(result.fields[0].profileField).toBeNull()
    expect(result.fields[0].method).toBe('unknown')
  })
})

describe('ambiguous form (no discernible profile fields)', () => {
  it('marks every field as null', () => {
    const html = loadFixture('ambiguous')
    document.body.innerHTML = html
    const result = detectFormFields(document.body, 'generic')

    expect(result.fields.length).toBeGreaterThan(0)

    const mappedCount = result.fields.filter(
      (f) => f.profileField !== null,
    ).length
    expect(mappedCount).toBe(0)

    for (const field of result.fields) {
      expect(
        field.profileField,
        `Expected field #${(field.element as HTMLElement).id} to be null, got ${field.profileField}`,
      ).toBeNull()
    }
  })
})

describe('detectAts', () => {
  it('detects Greenhouse from URL', () => {
    const doc = { URL: 'https://boards.greenhouse.io/jobs/123' } as Document
    expect(detectAts(doc)).toBe('greenhouse')
  })

  it('detects Lever from URL', () => {
    const doc = { URL: 'https://jobs.lever.co/acme/456' } as Document
    expect(detectAts(doc)).toBe('lever')
  })

  it('detects Workday from URL', () => {
    const doc = { URL: 'https://acme.myworkdayjobs.com/Careers/789' } as Document
    expect(detectAts(doc)).toBe('workday')
  })

  it('returns generic for unknown sites', () => {
    const doc = { URL: 'https://example.com/jobs/123' } as Document
    expect(detectAts(doc)).toBe('generic')
  })
})
