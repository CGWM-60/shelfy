import { useEffect, useState } from 'react'
import { useParams } from 'react-router-dom'
import { useAPI } from '../api/context'
import type { MediaItem } from '../api/types'
import { openAskAIOverlay } from '../components/AskAIOverlay'
import { setMiniPlayerState } from '../components/miniPlayerState'

export function MediaDetailPage() {
  const api = useAPI()
  const { id = '' } = useParams()
  const [media, setMedia] = useState<MediaItem | null>(null)
  const [position, setPosition] = useState(0)
  const [error, setError] = useState('')
  const [status, setStatus] = useState('')

  useEffect(() => {
    if (!id) return
    setError('')
    api.getMedia(id).then((payload) => {
      setMedia(payload.item)
      if (payload.progress) {
        setPosition(payload.progress.positionMs)
      }
    }).catch((err: unknown) => {
      setError(err instanceof Error ? err.message : 'Impossible de charger la fiche média')
    })
  }, [id, api])

  async function saveProgress() {
    if (!id) return
    try {
      await api.saveMediaProgress(id, position)
      setStatus('Position sauvegardée')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Sauvegarde impossible')
    }
  }

  if (!media) {
    return <section className="panel">{error || 'Chargement média...'}</section>
  }

  const mediaStreamURL = `/files/${media.id}/stream`
  const mediaDownloadURL = `/files/${media.id}/download`

  return (
    <section className="panel stack" data-testid="media-detail-page">
      <div>
        <h1 className="m-0 text-2xl font-bold text-slate-900">{media.title}</h1>
        <p className="m-0 mt-1 text-sm text-slate-600">Type: {media.kind}</p>
      </div>
      {status ? <p role="status" className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{status}</p> : null}
      {error ? <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p> : null}

      {media.kind === 'video' ? (
        <video
          className="w-full rounded-2xl border border-slate-200 bg-black"
          controls
          src={mediaStreamURL}
          onPlay={() => setMiniPlayerState({ mediaId: media.id, title: media.title })}
          onTimeUpdate={(e) => setPosition(Math.floor((e.currentTarget.currentTime || 0) * 1000))}
        />
      ) : null}
      {media.kind === 'audio' ? (
        <audio
          className="w-full"
          controls
          src={mediaStreamURL}
          onPlay={() => setMiniPlayerState({ mediaId: media.id, title: media.title })}
          onTimeUpdate={(e) => setPosition(Math.floor((e.currentTarget.currentTime || 0) * 1000))}
        />
      ) : null}
      {media.kind === 'image' ? (
        <div className="image-viewer">
          <img src={mediaDownloadURL} alt={media.title} />
        </div>
      ) : null}
      {media.kind === 'pdf' ? (
        <iframe title={media.title} src={mediaDownloadURL} className="pdf-viewer" />
      ) : null}
      {media.kind === 'other' ? (
        <a className="text-brand-700 underline" href={mediaDownloadURL}>Télécharger le fichier</a>
      ) : null}
      <div className="flex flex-wrap gap-2">
        <button className="btn btn-primary" onClick={() => void saveProgress()}>Sauvegarder position</button>
        <button className="btn" onClick={() => openAskAIOverlay({ question: `Parle-moi de ${media.title}` })}>Ask AI about this</button>
      </div>
      <small className="text-slate-600">Position: {position} ms</small>
    </section>
  )
}
