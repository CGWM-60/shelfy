import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useAPI } from '../api/context'
import type { MediaItem } from '../api/types'

export function LibraryPage() {
  const api = useAPI()
  const [media, setMedia] = useState<MediaItem[]>([])
  const [query, setQuery] = useState('')
  const [kind, setKind] = useState('')
  const [loading, setLoading] = useState(true)
  const [toast, setToast] = useState('')
  const [error, setError] = useState('')

  async function refresh() {
    setError('')
    try {
      setMedia(await api.listMedia(query, kind))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Impossible de charger la bibliothèque')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
  }, [query, kind])

  async function scan() {
    setError('')
    try {
      const result = await api.scanMedia()
      setToast(`${result.count} élément(s) scanné(s)`)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Scan impossible')
    }
  }

  return (
    <section className="panel stack" data-testid="library-page">
      <div className="row">
        <div>
          <h1 className="m-0 text-2xl font-bold text-slate-900">Bibliothèque média</h1>
          <p className="m-0 mt-1 text-sm text-slate-600">Explore vidéos, audios, images et PDF avec filtres rapides.</p>
        </div>
        <button className="btn btn-primary" onClick={() => void scan()}>Scanner la bibliothèque</button>
      </div>

      {toast ? <p role="status" className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{toast}</p> : null}
      {error ? <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p> : null}

      <div className="grid gap-3 rounded-2xl border border-slate-200 bg-slate-50/70 p-4 md:grid-cols-[2fr_1fr]">
        <input className="field" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Rechercher..." />
        <select className="field" value={kind} onChange={(e) => setKind(e.target.value)}>
          <option value="">Tous types</option>
          <option value="video">Vidéo</option>
          <option value="audio">Audio</option>
          <option value="image">Image</option>
          <option value="pdf">PDF</option>
          <option value="other">Autre</option>
        </select>
      </div>

      {loading ? <p className="m-0 text-sm text-slate-500">Chargement de la bibliothèque...</p> : null}
      {!loading && media.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-slate-300 bg-white p-6 text-center text-sm text-slate-600">
          Aucun média trouvé.
        </div>
      ) : null}

      <ul className="grid list grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
        {media.map((item) => (
          <li key={item.id} className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm transition hover:-translate-y-0.5 hover:shadow-md">
            <div className="mb-3 flex items-center justify-between gap-2">
              <span className="badge bg-slate-100 text-slate-700">{item.kind}</span>
              <small className="truncate text-xs text-slate-500">{item.mimeType || 'type inconnu'}</small>
            </div>
            <Link className="text-base font-semibold text-slate-900 no-underline hover:text-brand-700" to={`/media/${item.id}`}>{item.title}</Link>
            <p className="mt-2 mb-0 truncate text-xs text-slate-500">{item.path}</p>
          </li>
        ))}
      </ul>
    </section>
  )
}
