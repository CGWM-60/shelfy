import { DragEvent, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useAPI } from '../api/context'
import type { FSManagedEntry, MediaItem } from '../api/types'

function basename(path: string) {
  const normalized = path.replace(/\/+$/, '')
  const idx = normalized.lastIndexOf('/')
  if (idx < 0) return normalized
  return normalized.slice(idx + 1)
}

function parentPath(path: string) {
  const normalized = path.replace(/\/+$/, '')
  const idx = normalized.lastIndexOf('/')
  if (idx <= 0) return path
  return normalized.slice(0, idx)
}

function joinPath(base: string, name: string) {
  const cleanBase = base.replace(/\/+$/, '')
  const cleanName = name.replace(/^\/+/, '')
  return `${cleanBase}/${cleanName}`
}

export function LibraryPage() {
  const api = useAPI()
  const [mediaByPath, setMediaByPath] = useState<Record<string, MediaItem>>({})
  const [roots, setRoots] = useState<string[]>([])
  const [currentPath, setCurrentPath] = useState('')
  const [entries, setEntries] = useState<FSManagedEntry[]>([])
  const [query, setQuery] = useState('')
  const [dragSourcePath, setDragSourcePath] = useState('')
  const [loading, setLoading] = useState(true)
  const [toast, setToast] = useState('')
  const [error, setError] = useState('')

  async function refreshMediaIndex() {
    setError('')
    try {
      const media = await api.listMedia('', '')
      const next: Record<string, MediaItem> = {}
      for (const item of media) {
        next[item.path] = item
      }
      setMediaByPath(next)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Impossible de charger la bibliothèque')
    }
  }

  async function refreshPath(path = currentPath) {
    if (!path) return
    setError('')
    try {
      const listing = await api.listFS(path)
      setCurrentPath(listing.path || path)
      setEntries((listing.items ?? []).slice().sort((a, b) => {
        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
        return a.name.localeCompare(b.name)
      }))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Impossible de charger ce dossier')
    } finally {
      setLoading(false)
    }
  }

  async function refreshAll(initialPath?: string) {
    setLoading(true)
    try {
      const [rootsPayload] = await Promise.all([api.getFSRoots(), refreshMediaIndex()])
      const nextRoots = rootsPayload.roots ?? []
      setRoots(nextRoots)
      const selected = initialPath || currentPath || nextRoots[0] || ''
      if (selected) {
        await refreshPath(selected)
      }
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refreshAll()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function scan() {
    setError('')
    try {
      const result = await api.scanMedia(currentPath || undefined)
      setToast(`${result.count} élément(s) scanné(s)`)
      await refreshAll(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Scan impossible')
    }
  }

  async function deleteEntry(entry: FSManagedEntry) {
    const confirmed = window.confirm(entry.isDir ? `Supprimer le dossier "${entry.name}" et son contenu ?` : `Supprimer le fichier "${entry.name}" ?`)
    if (!confirmed) return
    setError('')
    try {
      await api.deleteFSPath(entry.path, entry.isDir)
      setToast(`Supprimé: ${entry.name}`)
      await refreshAll(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Suppression impossible')
    }
  }

  async function moveIntoFolder(targetFolderPath: string) {
    if (!dragSourcePath) return
    const sourceName = basename(dragSourcePath)
    const targetPath = joinPath(targetFolderPath, sourceName)
    if (targetPath === dragSourcePath) return
    setError('')
    try {
      await api.moveFSPath(dragSourcePath, targetPath)
      setToast(`Déplacé vers ${targetFolderPath}`)
      await refreshAll(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Déplacement impossible')
    } finally {
      setDragSourcePath('')
    }
  }

  function onDragStart(event: DragEvent<HTMLLIElement>, path: string) {
    setDragSourcePath(path)
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move'
      event.dataTransfer.setData('text/plain', path)
    }
  }

  const filteredEntries = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return entries
    return entries.filter((entry) => entry.name.toLowerCase().includes(q))
  }, [entries, query])

  return (
    <section className="panel stack" data-testid="library-page">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="m-0 text-2xl font-bold text-slate-900">Bibliothèque explorateur</h1>
          <p className="m-0 mt-1 text-sm text-slate-600">Mode explorateur avec suppression et glisser-déposer pour organiser les dossiers.</p>
        </div>
        <button className="btn btn-primary" onClick={() => void scan()}>Scanner la bibliothèque</button>
      </div>

      {toast ? <p role="status" className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{toast}</p> : null}
      {error ? <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p> : null}

      <div className="grid gap-3 rounded-2xl border border-slate-200 bg-slate-50/70 p-4 md:grid-cols-[2fr_1fr_1fr]">
        <input className="field" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Filtrer dans le dossier courant..." />
        <select className="field" value={currentPath} onChange={(e) => void refreshPath(e.target.value)} aria-label="Racine bibliothèque">
          {roots.map((root) => (
            <option value={root} key={root}>{root}</option>
          ))}
          {currentPath && !roots.includes(currentPath) ? <option value={currentPath}>{currentPath}</option> : null}
        </select>
        <div className="flex gap-2">
          <button className="btn" onClick={() => void refreshPath(parentPath(currentPath))} disabled={!currentPath}>Monter</button>
          <button className="btn" onClick={() => void refreshPath(currentPath)} disabled={!currentPath}>Rafraîchir</button>
        </div>
      </div>

      {loading ? <p className="m-0 text-sm text-slate-500">Chargement de la bibliothèque...</p> : null}
      {!loading && filteredEntries.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-slate-300 bg-white p-6 text-center text-sm text-slate-600">
          Aucun élément dans ce dossier.
        </div>
      ) : null}

      <div
        className="rounded-2xl border border-slate-200 bg-white"
        onDragOver={(event) => event.preventDefault()}
        onDrop={(event) => {
          event.preventDefault()
          if (currentPath) {
            void moveIntoFolder(currentPath)
          }
        }}
      >
        <div className="hidden border-b border-slate-200 px-3 py-2 text-xs font-semibold uppercase tracking-[0.14em] text-slate-500 sm:flex sm:items-center sm:justify-between">
          <span>Nom</span>
          <span>Actions</span>
        </div>
        <ul className="m-0 list-none p-0">
          {filteredEntries.map((entry) => {
            const mediaItem = mediaByPath[entry.path]
            return (
              <li
                key={entry.path}
                draggable
                onDragStart={(event) => onDragStart(event, entry.path)}
                onDragOver={(event) => {
                  if (!entry.isDir) return
                  event.preventDefault()
                }}
                onDrop={(event) => {
                  if (!entry.isDir) return
                  event.preventDefault()
                  void moveIntoFolder(entry.path)
                }}
                className="flex flex-col gap-2 border-t border-slate-100 px-3 py-2 sm:flex-row sm:items-center sm:justify-between"
                data-testid={`library-entry-${entry.name}`}
              >
                <div className="min-w-0 text-sm text-slate-700">
                  <p className="m-0 truncate font-semibold text-slate-900">
                    {entry.isDir ? '[DIR]' : '[FILE]'} {entry.name}
                  </p>
                  <p className="m-0 truncate text-xs text-slate-500">{entry.path}</p>
                </div>
                <div className="flex flex-wrap gap-2">
                  {entry.isDir ? (
                    <button className="btn" onClick={() => void refreshPath(entry.path)}>Ouvrir</button>
                  ) : null}
                  {!entry.isDir && mediaItem ? (
                    <Link className="btn" to={`/media/${mediaItem.id}`} aria-label={mediaItem.title}>{mediaItem.title}</Link>
                  ) : null}
                  <button
                    className="btn btn-danger"
                    onClick={() => void deleteEntry(entry)}
                    aria-label={`Supprimer ${entry.name}`}
                  >
                    Supprimer
                  </button>
                </div>
              </li>
            )
          })}
        </ul>
      </div>
      <p className="m-0 text-xs text-slate-500">
        Astuce: glisse un fichier/dossier et dépose-le sur un dossier (ou dans la zone actuelle) pour le déplacer.
      </p>
      <p className="m-0 text-xs text-slate-500">Chemin courant: {currentPath || '-'}</p>
    </section>
  )
}
