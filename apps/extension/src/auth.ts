// Auth (spec 014-autofill-extension section 2). Only this module — and the
// service worker that calls it — ever touches an auth token. The content
// script and popup talk to the service worker over chrome.runtime messages
// and never receive a token, per spec 1.2/1.3: "The content script never
// holds the token."
//
// Storage split matches spec 2.1 exactly:
//   - access token  -> chrome.storage.session (ephemeral, cleared on browser close)
//   - refresh token -> chrome.storage.local   (persists, rotated on every use)
import type { StoredAccessToken, StoredRefreshToken, TokenPairResponse } from './types.js'
import { API_BASE_URL } from './config.js'

const ACCESS_TOKEN_KEY = 'jf.accessToken'
const REFRESH_TOKEN_KEY = 'jf.refreshToken'
const CACHED_PROFILE_KEY = 'jf.cachedProfile'

// Refresh proactively once fewer than this many ms remain on the access
// token, rather than waiting for a 401 — the profile fetch that triggers a
// refresh should not itself have to retry.
const REFRESH_MARGIN_MS = 60_000

function toEpochMs(iso: string): number {
  return new Date(iso).getTime()
}

async function storeTokenPair(pair: TokenPairResponse): Promise<void> {
  const access: StoredAccessToken = {
    accessToken: pair.accessToken,
    expiresAt: toEpochMs(pair.accessTokenExpiresAt),
  }
  const refresh: StoredRefreshToken = {
    refreshToken: pair.refreshToken,
    expiresAt: toEpochMs(pair.refreshTokenExpiresAt),
  }
  await chrome.storage.session.set({ [ACCESS_TOKEN_KEY]: access })
  await chrome.storage.local.set({ [REFRESH_TOKEN_KEY]: refresh })
}

async function getStoredAccessToken(): Promise<StoredAccessToken | null> {
  const result = await chrome.storage.session.get(ACCESS_TOKEN_KEY)
  return (result[ACCESS_TOKEN_KEY] as StoredAccessToken | undefined) ?? null
}

async function getStoredRefreshToken(): Promise<StoredRefreshToken | null> {
  const result = await chrome.storage.local.get(REFRESH_TOKEN_KEY)
  return (result[REFRESH_TOKEN_KEY] as StoredRefreshToken | undefined) ?? null
}

/** Clears every stored credential and the cached profile — forces the user
 * back through the dashboard bootstrap-code flow (spec 2.3's "user must
 * re-authenticate" fallback). Never logs what it clears. */
export async function clearAuth(): Promise<void> {
  await chrome.storage.session.remove(ACCESS_TOKEN_KEY)
  await chrome.storage.local.remove([REFRESH_TOKEN_KEY, CACHED_PROFILE_KEY])
}

export async function isAuthenticated(): Promise<boolean> {
  const refresh = await getStoredRefreshToken()
  if (!refresh) return false
  return refresh.expiresAt > Date.now()
}

/** Exchanges a one-time bootstrap code (from the dashboard) for a fresh
 * token pair (spec 2.1 steps 3-4). Throws with a user-safe message on
 * failure — never logs the code or any token. */
export async function exchangeBootstrapCode(code: string): Promise<void> {
  const res = await fetch(`${API_BASE_URL}/v1/ext/auth/exchange`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code }),
  })
  if (!res.ok) {
    throw new Error(
      res.status === 401 ? 'Invalid or expired pairing code' : `Failed to connect (${res.status})`,
    )
  }
  const pair = (await res.json()) as TokenPairResponse
  await storeTokenPair(pair)
}

/** Rotates the refresh token and mints a new access token (spec 2.3). On
 * any failure it clears stored auth so the caller's next getValidAccessToken
 * call correctly reports "not authenticated" instead of retrying forever. */
async function refreshTokens(): Promise<boolean> {
  const stored = await getStoredRefreshToken()
  if (!stored || stored.expiresAt <= Date.now()) {
    await clearAuth()
    return false
  }

  let res: Response
  try {
    res = await fetch(`${API_BASE_URL}/v1/ext/auth/refresh`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ refreshToken: stored.refreshToken }),
    })
  } catch {
    // Network failure: leave stored tokens intact and let the caller retry
    // later, rather than forcing a re-auth for a transient outage.
    return false
  }

  if (!res.ok) {
    await clearAuth()
    return false
  }

  const pair = (await res.json()) as TokenPairResponse
  await storeTokenPair(pair)
  return true
}

/** The one function every API call should go through: returns a live
 * access token, refreshing first if it's missing/expiring/expired, or null
 * if the user needs to re-authenticate via the dashboard. */
export async function getValidAccessToken(): Promise<string | null> {
  const access = await getStoredAccessToken()
  if (access && access.expiresAt - Date.now() > REFRESH_MARGIN_MS) {
    return access.accessToken
  }
  const refreshed = await refreshTokens()
  if (!refreshed) return null
  const fresh = await getStoredAccessToken()
  return fresh?.accessToken ?? null
}

export async function getCachedProfile<T>(): Promise<T | null> {
  const result = await chrome.storage.local.get(CACHED_PROFILE_KEY)
  return (result[CACHED_PROFILE_KEY] as T | undefined) ?? null
}

export async function setCachedProfile<T>(profile: T): Promise<void> {
  await chrome.storage.local.set({ [CACHED_PROFILE_KEY]: profile })
}
