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

// ---------------------------------------------------------------------------
// Auth (spec 014-autofill-extension section 2)
// ---------------------------------------------------------------------------

/** Stored in chrome.storage.session — cleared when the browser closes. */
export interface StoredAccessToken {
  accessToken: string
  /** epoch ms */
  expiresAt: number
}

/** Stored in chrome.storage.local — rotated on every refresh. */
export interface StoredRefreshToken {
  refreshToken: string
  /** epoch ms */
  expiresAt: number
}

export interface TokenPairResponse {
  accessToken: string
  accessTokenExpiresAt: string
  refreshToken: string
  refreshTokenExpiresAt: string
  scope: string
}

// ---------------------------------------------------------------------------
// Profile (spec 014-autofill-extension section 3) — mirrors
// apps/api/internal/dto.ExtProfileDto field-for-field.
// ---------------------------------------------------------------------------

export interface ExtWorkEntry {
  employer: string
  role: string
  startDate: string
  endDate: string | null
  current: boolean
  description: string
}

export interface ExtEducationEntry {
  institution: string
  degree: string
  startDate: string
  endDate: string
}

export interface ExtLink {
  url: string
  label: string
}

export interface ExtProfile {
  fullName: string
  email: string
  phone: string
  location: string
  headline: string
  skills: string[]
  workHistory: ExtWorkEntry[]
  education: ExtEducationEntry[]
  links: ExtLink[]
}

// ---------------------------------------------------------------------------
// content script <-> service worker messages. The content script never sees
// an auth token — every message below is answered with data, not a token.
// ---------------------------------------------------------------------------

export type AuthStatus = 'authenticated' | 'unauthenticated'

export interface AuthStatusResponse {
  status: AuthStatus
}

export interface ExchangeCodeRequest {
  type: 'auth:exchange'
  code: string
}

export interface AuthStatusRequest {
  type: 'auth:status'
}

export interface LogoutRequest {
  type: 'auth:logout'
}

export interface GetProfileRequest {
  type: 'profile:get'
}

export type ExtensionRequest =
  | ExchangeCodeRequest
  | AuthStatusRequest
  | LogoutRequest
  | GetProfileRequest

export interface OkResponse<T> {
  ok: true
  data: T
}

export interface ErrResponse {
  ok: false
  error: string
}

export type ExtensionResponse<T> = OkResponse<T> | ErrResponse
