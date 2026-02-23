export interface MiniPlayerState {
  mediaId: string
  title: string
}

const STORAGE_KEY = 'mini-player'
export const MINI_PLAYER_UPDATED_EVENT = 'shelfy:mini-player-updated'

function isValidState(value: unknown): value is MiniPlayerState {
  if (!value || typeof value !== 'object') return false
  const candidate = value as Record<string, unknown>
  return typeof candidate.mediaId === 'string' && candidate.mediaId.length > 0 && typeof candidate.title === 'string' && candidate.title.length > 0
}

export function readMiniPlayerState(): MiniPlayerState | null {
  const raw = localStorage.getItem(STORAGE_KEY)
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw)
    if (isValidState(parsed)) {
      return parsed
    }
  } catch {
    // ignore invalid storage payloads
  }
  localStorage.removeItem(STORAGE_KEY)
  return null
}

export function setMiniPlayerState(state: MiniPlayerState) {
  localStorage.setItem(STORAGE_KEY, JSON.stringify(state))
  window.dispatchEvent(new CustomEvent(MINI_PLAYER_UPDATED_EVENT))
}

export function clearMiniPlayerState() {
  localStorage.removeItem(STORAGE_KEY)
  window.dispatchEvent(new CustomEvent(MINI_PLAYER_UPDATED_EVENT))
}
