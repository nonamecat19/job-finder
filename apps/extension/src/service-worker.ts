// Service worker (spec 014-autofill-extension 1.2): the only place in the
// extension that ever holds an auth token. Content scripts and the popup
// talk to it exclusively via chrome.runtime messages defined in types.ts —
// they never see auth.ts's storage or the API's Authorization header.
import { clearAuth, exchangeBootstrapCode, getCachedProfile, getValidAccessToken, isAuthenticated, setCachedProfile } from './auth.js'
import { fetchProfile } from './profile-api.js'
import type { ExtensionRequest, ExtensionResponse, ExtProfile } from './types.js'

chrome.runtime.onInstalled.addListener(() => {
  console.log('[Job Finder] service worker installed')
})

async function handleMessage(message: ExtensionRequest): Promise<ExtensionResponse<unknown>> {
  switch (message.type) {
    case 'auth:exchange': {
      try {
        await exchangeBootstrapCode(message.code)
        return { ok: true, data: null }
      } catch (err) {
        return { ok: false, error: err instanceof Error ? err.message : 'Failed to connect' }
      }
    }

    case 'auth:status': {
      const authenticated = await isAuthenticated()
      return { ok: true, data: { status: authenticated ? 'authenticated' : 'unauthenticated' } }
    }

    case 'auth:logout': {
      await clearAuth()
      return { ok: true, data: null }
    }

    case 'profile:get': {
      const token = await getValidAccessToken()
      if (!token) {
        return { ok: false, error: 'not authenticated' }
      }
      try {
        const profile = await fetchProfile(token)
        await setCachedProfile<ExtProfile>(profile)
        return { ok: true, data: profile }
      } catch (err) {
        // Fall back to the last cached profile (spec 1.2: "caches them in
        // chrome.storage.local") rather than failing the whole fill on a
        // transient network error.
        const cached = await getCachedProfile<ExtProfile>()
        if (cached) return { ok: true, data: cached }
        return { ok: false, error: err instanceof Error ? err.message : 'Failed to load profile' }
      }
    }

    default:
      return { ok: false, error: 'unknown message type' }
  }
}

chrome.runtime.onMessage.addListener((message: ExtensionRequest, _sender, sendResponse) => {
  handleMessage(message).then(sendResponse)
  return true // keep the message channel open for the async sendResponse
})
