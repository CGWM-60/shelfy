import { useEffect, useState } from 'react'
import { clearMiniPlayerState, MINI_PLAYER_UPDATED_EVENT, readMiniPlayerState, type MiniPlayerState } from './miniPlayerState'

export function MiniPlayer() {
  const [state, setState] = useState<MiniPlayerState | null>(null)

  useEffect(() => {
    const sync = () => setState(readMiniPlayerState())
    sync()

    const onStorage = () => {
      sync()
    }
    const onMiniPlayerUpdated = () => sync()
    window.addEventListener('storage', onStorage)
    window.addEventListener(MINI_PLAYER_UPDATED_EVENT, onMiniPlayerUpdated)
    return () => {
      window.removeEventListener('storage', onStorage)
      window.removeEventListener(MINI_PLAYER_UPDATED_EVENT, onMiniPlayerUpdated)
    }
  }, [])

  if (!state) {
    return null
  }

  return (
    <div className="mini-player" data-testid="mini-player">
      <div className="mb-2 flex items-center justify-between gap-3 md:mb-0">
        <div className="text-sm font-semibold">{state.title}</div>
        <button type="button" className="btn" onClick={() => clearMiniPlayerState()} aria-label="Fermer mini-player">
          Fermer
        </button>
      </div>
      <audio controls src={`/files/${state.mediaId}/stream`} className="w-full md:max-w-md" onEnded={() => clearMiniPlayerState()} />
    </div>
  )
}
