import type { ProfileField } from './types.js'

export const AUTOCOMPLETE_MAP: Record<string, ProfileField> = {
  'name': 'fullName',
  'given-name': 'fullName',
  'family-name': 'fullName',
  'email': 'email',
  'tel': 'phone',
  'tel-national': 'phone',
  'organization-title': 'headline',
  'url': 'links',
  'street-address': 'location',
  'address-level1': 'location',
  'address-level2': 'location',
  'address-level3': 'location',
  'country-name': 'location',
  'postal-code': 'location',
}

export const GREENHOUSE_FIELD_MAP: Record<string, ProfileField> = {
  'first_name': 'fullName',
  'last_name': 'fullName',
  'email': 'email',
  'phone': 'phone',
  'job_city': 'location',
  'job_location': 'location',
  'headline': 'headline',
  'skills': 'skills',
  'linkedin_profile_url': 'links',
  'portfolio_url': 'links',
  'github_url': 'links',
}

export const LEVER_FIELD_MAP: Record<string, ProfileField> = {
  'name': 'fullName',
  'email': 'email',
  'phone': 'phone',
  'headline': 'headline',
  'skills': 'skills',
  'urls[LinkedIn]': 'links',
  'urls[Github]': 'links',
  'urls[Portfolio]': 'links',
}

export const WORKDAY_FIELD_MAP: Record<string, ProfileField> = {
  'firstName': 'fullName',
  'lastName': 'fullName',
  'email': 'email',
  'phone': 'phone',
  'location': 'location',
  'headline': 'headline',
  'skill': 'skills',
}
