export type ProfileField =
  | 'fullName'
  | 'email'
  | 'phone'
  | 'location'
  | 'headline'
  | 'skills'
  | 'links'

export type AtsVendor = 'greenhouse' | 'lever' | 'workday' | 'generic'

export type DetectionMethod =
  | 'autocomplete'
  | 'name'
  | 'label'
  | 'aria-label'
  | 'unknown'

export interface DetectedField {
  element: HTMLElement
  profileField: ProfileField | null
  method: DetectionMethod
}

export interface DetectionResult {
  ats: AtsVendor
  fields: DetectedField[]
}
