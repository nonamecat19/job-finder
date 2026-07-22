import type { ExtProfile } from './types.js'
import { API_BASE_URL } from './config.js'

/** Fetches the authenticated user's profile (spec section 3). Callers must
 * pass an already-valid access token (see auth.getValidAccessToken) —
 * this module does no auth logic itself, it only speaks HTTP. */
export async function fetchProfile(accessToken: string): Promise<ExtProfile> {
  const res = await fetch(`${API_BASE_URL}/v1/ext/profile`, {
    headers: { Authorization: `Bearer ${accessToken}` },
  })
  if (!res.ok) {
    throw new Error(res.status === 401 ? 'Access token rejected' : `Failed to load profile (${res.status})`)
  }
  return (await res.json()) as ExtProfile
}
