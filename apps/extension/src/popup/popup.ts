import type { AuthStatusResponse, ExtensionResponse, ExtProfile } from '../types.js'

function el<T extends HTMLElement>(id: string): T {
  const found = document.getElementById(id)
  if (!found) throw new Error(`popup: missing #${id}`)
  return found as T
}

const statusChip = el<HTMLSpanElement>('status-chip')
const connectSection = el<HTMLElement>('connect-section')
const connectedSection = el<HTMLElement>('connected-section')
const codeInput = el<HTMLInputElement>('code-input')
const connectBtn = el<HTMLButtonElement>('connect-btn')
const connectError = el<HTMLParagraphElement>('connect-error')
const profileSummary = el<HTMLDivElement>('profile-summary')
const fillBtn = el<HTMLButtonElement>('fill-btn')
const disconnectBtn = el<HTMLButtonElement>('disconnect-btn')

function sendMessage<T>(message: unknown): Promise<ExtensionResponse<T>> {
  return chrome.runtime.sendMessage(message)
}

function showConnected(): void {
  statusChip.textContent = 'connected'
  statusChip.className = 'chip chip-connected'
  connectSection.hidden = true
  connectedSection.hidden = false
}

function showDisconnected(): void {
  statusChip.textContent = 'disconnected'
  statusChip.className = 'chip chip-disconnected'
  connectSection.hidden = false
  connectedSection.hidden = true
}

async function loadProfileSummary(): Promise<void> {
  const res = await sendMessage<ExtProfile>({ type: 'profile:get' })
  if (!res.ok) {
    profileSummary.textContent = 'Profile unavailable right now.'
    return
  }
  const profile = res.data
  profileSummary.innerHTML = ''
  const name = document.createElement('div')
  name.className = 'name'
  name.textContent = profile.fullName || '(no name on file)'
  const meta = document.createElement('div')
  meta.className = 'meta'
  meta.textContent = `${profile.skills.length} skill${profile.skills.length === 1 ? '' : 's'} · ${profile.workHistory.length} work entr${profile.workHistory.length === 1 ? 'y' : 'ies'}`
  profileSummary.appendChild(name)
  profileSummary.appendChild(meta)
}

async function refreshStatus(): Promise<void> {
  const res = await sendMessage<AuthStatusResponse>({ type: 'auth:status' })
  if (res.ok && res.data.status === 'authenticated') {
    showConnected()
    await loadProfileSummary()
  } else {
    showDisconnected()
  }
}

connectBtn.addEventListener('click', async () => {
  const code = codeInput.value.trim()
  connectError.hidden = true
  if (!code) return

  connectBtn.disabled = true
  connectBtn.textContent = 'Connecting…'
  try {
    const res = await sendMessage<null>({ type: 'auth:exchange', code })
    if (!res.ok) {
      connectError.textContent = res.error
      connectError.hidden = false
      return
    }
    codeInput.value = ''
    await refreshStatus()
  } finally {
    connectBtn.disabled = false
    connectBtn.textContent = 'Connect'
  }
})

disconnectBtn.addEventListener('click', async () => {
  await sendMessage({ type: 'auth:logout' })
  showDisconnected()
})

fillBtn.addEventListener('click', async () => {
  const [tab] = await chrome.tabs.query({ active: true, currentWindow: true })
  if (tab?.id === undefined) return
  await chrome.tabs.sendMessage(tab.id, { type: 'fill:trigger' })
  window.close()
})

refreshStatus().catch((err) => {
  console.error('[Job Finder] popup error', err)
  showDisconnected()
})
