import { DragEvent, MouseEvent, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useAPI } from '../api/context'
import type { FSManagedEntry, MediaItem } from '../api/types'

function basename(path: string) {
  const normalized = path.replace(/\/+$/, '')
  const idx = normalized.lastIndexOf('/')
  if (idx < 0) return normalized
  return normalized.slice(idx + 1)
}

function normalizeFsPath(path: string) {
  if (!path) return ''
  const slashNormalized = path.replace(/\\/g, '/').replace(/\/+/g, '/')
  if (slashNormalized.length <= 1) return slashNormalized
  return slashNormalized.replace(/\/+$/, '')
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

function isWithinRoot(root: string, target: string) {
  const cleanRoot = root.replace(/\/+$/, '')
  if (target === cleanRoot) return true
  return target.startsWith(`${cleanRoot}/`)
}

function pickRootForPath(path: string, roots: string[]) {
  let best = ''
  for (const root of roots) {
    if (!isWithinRoot(root, path)) continue
    if (root.length > best.length) best = root
  }
  return best
}

function listDirEntries(items: FSManagedEntry[]) {
  return items
    .filter((item) => item.isDir)
    .slice()
    .sort((a, b) => a.name.localeCompare(b.name))
}

type Breadcrumb = {
  label: string
  path: string
}

type ContextMenuState = {
  x: number
  y: number
  entry: FSManagedEntry
  allowMutations: boolean
}

function buildBreadcrumbs(currentPath: string, roots: string[]): Breadcrumb[] {
  if (!currentPath) return []
  const root = pickRootForPath(currentPath, roots)
  if (!root) {
    return [{ label: basename(currentPath), path: currentPath }]
  }
  const rel = currentPath.slice(root.length).replace(/^\/+/, '')
  const crumbs: Breadcrumb[] = [{ label: basename(root) || root, path: root }]
  if (!rel) return crumbs
  let cursor = root
  for (const segment of rel.split('/').filter(Boolean)) {
    cursor = joinPath(cursor, segment)
    crumbs.push({ label: segment, path: cursor })
  }
  return crumbs
}

function buildAncestorChain(currentPath: string, roots: string[]): string[] {
  const root = pickRootForPath(currentPath, roots)
  if (!root) return [currentPath]
  const rel = currentPath.slice(root.length).replace(/^\/+/, '')
  const chain = [root]
  if (!rel) return chain
  let cursor = root
  for (const segment of rel.split('/').filter(Boolean)) {
    cursor = joinPath(cursor, segment)
    chain.push(cursor)
  }
  return chain
}

function relativeDepth(base: string, path: string) {
  if (path === base) return 0
  const rel = path.slice(base.length).replace(/^\/+/, '')
  if (!rel) return 0
  return rel.split('/').filter(Boolean).length
}

function entryFromPath(path: string, isDir: boolean): FSManagedEntry {
  return {
    name: basename(path),
    path,
    isDir,
    sizeBytes: 0,
    modifiedAt: ''
  }
}

export function LibraryPage() {
  const api = useAPI()
  const navigate = useNavigate()
  const [mediaByPath, setMediaByPath] = useState<Record<string, MediaItem>>({})
  const [roots, setRoots] = useState<string[]>([])
  const [treeFoldersByPath, setTreeFoldersByPath] = useState<Record<string, FSManagedEntry[]>>({})
  const [expanded, setExpanded] = useState<Record<string, boolean>>({})
  const [currentPath, setCurrentPath] = useState('')
  const [selectedPath, setSelectedPath] = useState('')
  const [entries, setEntries] = useState<FSManagedEntry[]>([])
  const [query, setQuery] = useState('')
  const [dragSourcePath, setDragSourcePath] = useState('')
  const [clipboardEntry, setClipboardEntry] = useState<FSManagedEntry | null>(null)
  const [moveModeEnabled, setMoveModeEnabled] = useState(false)
  const [lastMove, setLastMove] = useState<{ from: string; to: string } | null>(null)
  const [contextMenu, setContextMenu] = useState<ContextMenuState | null>(null)
  const [loading, setLoading] = useState(true)
  const [toast, setToast] = useState('')
  const [error, setError] = useState('')

  function findMediaByPath(index: Record<string, MediaItem>, path: string) {
    const direct = index[path]
    if (direct) return direct
    const normalizedPath = normalizeFsPath(path)
    const normalized = index[normalizedPath]
    if (normalized) return normalized
    return Object.values(index).find((item) => normalizeFsPath(item.path) === normalizedPath)
  }

  async function refreshMediaIndex() {
    setError('')
    try {
      const media = await api.listMedia('', '')
      const next: Record<string, MediaItem> = {}
      for (const item of media) {
        next[item.path] = item
        const normalized = normalizeFsPath(item.path)
        if (normalized) {
          next[normalized] = item
        }
      }
      setMediaByPath(next)
      return next
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Impossible de charger la bibliothèque')
      return {} as Record<string, MediaItem>
    }
  }

  async function listDirs(path: string) {
    const listing = await api.listFS(path)
    return listDirEntries(listing.items ?? [])
  }

  async function refreshPath(path = currentPath) {
    if (!path) return
    setError('')
    try {
      const listing = await api.listFS(path)
      setCurrentPath(listing.path || path)
      setSelectedPath(listing.path || path)
      setEntries((listing.items ?? []).slice().sort((a, b) => {
        if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
        return a.name.localeCompare(b.name)
      }))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Impossible de charger ce dossier')
    }
  }

  async function refreshAll(initialPath?: string) {
    setLoading(true)
    try {
      const [rootsPayload] = await Promise.all([api.getFSRoots(), refreshMediaIndex()])
      const nextRoots = rootsPayload.roots ?? []
      setRoots(nextRoots)
      const selected = initialPath || currentPath || nextRoots[0] || ''
      const nextFoldersByPath: Record<string, FSManagedEntry[]> = {}
      for (const root of nextRoots) {
        try {
          nextFoldersByPath[root] = await listDirs(root)
        } catch {
          nextFoldersByPath[root] = []
        }
      }
      if (selected) {
        await refreshPath(selected)
        const chain = buildAncestorChain(selected, nextRoots)
        const nextExpanded: Record<string, boolean> = {}
        for (const folderPath of chain) {
          nextExpanded[folderPath] = true
          if (!nextFoldersByPath[folderPath]) {
            try {
              nextFoldersByPath[folderPath] = await listDirs(folderPath)
            } catch {
              nextFoldersByPath[folderPath] = []
            }
          }
        }
        setExpanded(nextExpanded)
      } else {
        setEntries([])
        setExpanded({})
      }
      setTreeFoldersByPath(nextFoldersByPath)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refreshAll()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (!contextMenu) return
    const close = () => setContextMenu(null)
    window.addEventListener('click', close)
    window.addEventListener('resize', close)
    return () => {
      window.removeEventListener('click', close)
      window.removeEventListener('resize', close)
    }
  }, [contextMenu])

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
    if (!moveModeEnabled || !dragSourcePath) return
    const sourceName = basename(dragSourcePath)
    const targetPath = joinPath(targetFolderPath, sourceName)
    if (targetPath === dragSourcePath) return
    setError('')
    try {
      await api.moveFSPath(dragSourcePath, targetPath)
      setLastMove({ from: dragSourcePath, to: targetPath })
      setToast(`Déplacé vers ${targetFolderPath}`)
      await refreshAll(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Déplacement impossible')
    } finally {
      setDragSourcePath('')
    }
  }

  function onDragStart(event: DragEvent<HTMLLIElement>, path: string) {
    if (!moveModeEnabled) return
    setDragSourcePath(path)
    if (event.dataTransfer) {
      event.dataTransfer.effectAllowed = 'move'
      event.dataTransfer.setData('text/plain', path)
    }
  }

  async function toggleNode(path: string) {
    const willExpand = !expanded[path]
    setExpanded((prev) => ({ ...prev, [path]: willExpand }))
    if (!willExpand || treeFoldersByPath[path]) {
      return
    }
    try {
      const dirs = await listDirs(path)
      setTreeFoldersByPath((prev) => ({ ...prev, [path]: dirs }))
    } catch {
      setTreeFoldersByPath((prev) => ({ ...prev, [path]: [] }))
    }
  }

  async function moveEntryToParent(entry: FSManagedEntry) {
    const containerPath = parentPath(entry.path)
    if (!containerPath || containerPath === entry.path) return
    const targetParent = parentPath(containerPath)
    if (!targetParent || targetParent === containerPath) return
    const root = pickRootForPath(entry.path, roots)
    if (root && !isWithinRoot(root, targetParent)) return
    const toPath = joinPath(targetParent, entry.name)
    if (toPath === entry.path) return
    setError('')
    try {
      await api.moveFSPath(entry.path, toPath)
      setLastMove({ from: entry.path, to: toPath })
      setToast(`Déplacé vers ${targetParent}`)
      await refreshAll(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Déplacement impossible')
    }
  }

  async function undoLastMove() {
    if (!lastMove) return
    setError('')
    try {
      await api.moveFSPath(lastMove.to, lastMove.from)
      setToast('Dernier déplacement annulé')
      setLastMove(null)
      await refreshAll(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Impossible d’annuler le déplacement')
    }
  }

  async function moveCurrentFolderToParent() {
    if (!currentPath) return
    const root = pickRootForPath(currentPath, roots)
    if (!root || currentPath === root) return
    const containerPath = parentPath(currentPath)
    if (!containerPath || containerPath === currentPath) return
    const targetParent = parentPath(containerPath)
    if (!targetParent || targetParent === containerPath) return
    if (!isWithinRoot(root, targetParent)) return
    const targetPath = joinPath(targetParent, basename(currentPath))
    if (targetPath === currentPath) return
    setError('')
    try {
      await api.moveFSPath(currentPath, targetPath)
      setLastMove({ from: currentPath, to: targetPath })
      setToast(`Dossier remonté vers ${targetParent}`)
      await refreshAll(targetPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Déplacement impossible')
    }
  }

  function openContextMenu(event: MouseEvent, entry: FSManagedEntry, allowMutations = true) {
    event.preventDefault()
    setSelectedPath(entry.path)
    setContextMenu({ x: event.clientX, y: event.clientY, entry, allowMutations })
  }

  function openTreeContextMenu(event: MouseEvent, rowPath: string, isRoot: boolean) {
    const entry = entryFromPath(rowPath, true)
    openContextMenu(event, entry, !isRoot)
  }

  async function renameEntry(entry: FSManagedEntry) {
    const nextName = window.prompt('Nouveau nom', entry.name)
    if (nextName === null) return
    const cleanName = nextName.trim()
    if (!cleanName || cleanName === entry.name) return
    if (cleanName.includes('/') || cleanName.includes('\\')) {
      setError('Nom invalide')
      return
    }
    const targetPath = joinPath(parentPath(entry.path), cleanName)
    setError('')
    try {
      await api.moveFSPath(entry.path, targetPath)
      setLastMove({ from: entry.path, to: targetPath })
      setToast(`Renommé: ${entry.name} -> ${cleanName}`)
      await refreshAll(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Renommage impossible')
    }
  }

  async function createFolderIn(targetDir: string) {
    const nextName = window.prompt('Nom du nouveau dossier')
    if (nextName === null) return
    const cleanName = nextName.trim()
    if (!cleanName) return
    if (cleanName.includes('/') || cleanName.includes('\\')) {
      setError('Nom invalide')
      return
    }
    setError('')
    try {
      const targetPath = joinPath(targetDir, cleanName)
      await api.createFSDir(targetPath)
      setToast(`Dossier créé: ${cleanName}`)
      await refreshAll(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Création impossible')
    }
  }

  async function pasteInto(targetDir: string) {
    if (!clipboardEntry) return
    const targetPath = joinPath(targetDir, clipboardEntry.name)
    if (targetPath === clipboardEntry.path) return
    if (clipboardEntry.isDir && (targetDir === clipboardEntry.path || targetDir.startsWith(`${clipboardEntry.path}/`))) {
      setError('Déplacement invalide')
      return
    }
    setError('')
    try {
      await api.moveFSPath(clipboardEntry.path, targetPath)
      setLastMove({ from: clipboardEntry.path, to: targetPath })
      setClipboardEntry(null)
      setToast(`Collé dans ${targetDir}`)
      await refreshAll(currentPath)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Collage impossible')
    }
  }

  async function openEntry(entry: FSManagedEntry, mediaItem?: MediaItem) {
    if (entry.isDir) {
      await refreshPath(entry.path)
      return
    }
    const resolved = mediaItem || findMediaByPath(mediaByPath, entry.path)
    if (resolved) {
      navigate(`/media/${resolved.id}`)
      return
    }
    const refreshed = await refreshMediaIndex()
    const resolvedAfterRefresh = findMediaByPath(refreshed, entry.path)
    if (resolvedAfterRefresh) {
      navigate(`/media/${resolvedAfterRefresh.id}`)
      return
    }
    setError('Média non indexé. Lance un scan bibliothèque.')
  }

  const filteredEntries = useMemo(() => {
    const q = query.trim().toLowerCase()
    if (!q) return entries
    return entries.filter((entry) => entry.name.toLowerCase().includes(q))
  }, [entries, query])

  const currentRoot = useMemo(() => pickRootForPath(currentPath, roots), [currentPath, roots])
  const breadcrumbs = useMemo(() => buildBreadcrumbs(currentPath, roots), [currentPath, roots])
  const canGoUp = currentPath && parentPath(currentPath) !== currentPath
  const canMoveCurrentFolderUp = useMemo(() => {
    if (!currentPath) return false
    const root = pickRootForPath(currentPath, roots)
    if (!root || currentPath === root) return false
    const containerPath = parentPath(currentPath)
    if (!containerPath || containerPath === currentPath) return false
    const targetParent = parentPath(containerPath)
    if (!targetParent || targetParent === containerPath) return false
    return isWithinRoot(root, targetParent)
  }, [currentPath, roots])
  const contextTargetDir = useMemo(() => {
    if (!contextMenu) return ''
    if (contextMenu.entry.isDir) return contextMenu.entry.path
    return parentPath(contextMenu.entry.path)
  }, [contextMenu])
  const contextTargetRoot = useMemo(() => pickRootForPath(contextTargetDir, roots), [contextTargetDir, roots])
  const canWriteInContextTarget = Boolean(contextTargetDir && contextTargetRoot && isWithinRoot(contextTargetRoot, contextTargetDir))
  const canPasteInContextTarget = useMemo(() => {
    if (!clipboardEntry || !contextTargetDir || !canWriteInContextTarget) return false
    const targetPath = joinPath(contextTargetDir, clipboardEntry.name)
    if (targetPath === clipboardEntry.path) return false
    if (clipboardEntry.isDir && (contextTargetDir === clipboardEntry.path || contextTargetDir.startsWith(`${clipboardEntry.path}/`))) return false
    return true
  }, [canWriteInContextTarget, clipboardEntry, contextTargetDir])
  const treeRows = useMemo(() => {
    const rows: Array<{ path: string; depth: number; isRoot: boolean }> = []
    for (const root of roots) {
      rows.push({ path: root, depth: 0, isRoot: true })
      const stack = [...(treeFoldersByPath[root] ?? [])].reverse()
      while (stack.length > 0) {
        const folder = stack.pop()
        if (!folder) break
        const depth = relativeDepth(root, folder.path)
        rows.push({ path: folder.path, depth, isRoot: false })
        if (!expanded[folder.path]) continue
        const children = (treeFoldersByPath[folder.path] ?? []).slice().reverse()
        for (const child of children) {
          stack.push(child)
        }
      }
    }
    return rows
  }, [expanded, roots, treeFoldersByPath])

  return (
    <section className="panel stack" data-testid="library-page">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <h1 className="m-0 text-2xl font-bold text-slate-900">Bibliothèque explorateur</h1>
          <p className="m-0 mt-1 text-sm text-slate-600">Vue type Finder/Explorer: dossiers à gauche, contenu au centre.</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button className="btn btn-primary" onClick={() => void scan()}>Scanner la bibliothèque</button>
          {lastMove ? <button className="btn" onClick={() => void undoLastMove()}>Annuler dernier déplacement</button> : null}
        </div>
      </div>

      {toast ? <p role="status" className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{toast}</p> : null}
      {error ? <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p> : null}

      <div className="grid gap-4 md:grid-cols-[280px_minmax(0,1fr)]">
        <aside className="rounded-2xl border border-slate-200 bg-white p-3">
          <div className="mb-2">
            <p className="m-0 text-xs font-semibold uppercase tracking-[0.14em] text-slate-500">Dossiers</p>
          </div>
          {treeRows.length === 0 ? (
            <p className="m-0 text-sm text-slate-500">Aucun dossier configuré.</p>
          ) : (
            <ul className="m-0 list-none space-y-1 p-0">
              {treeRows.map((row) => {
                const isExpanded = expanded[row.path]
                const isActive = currentPath === row.path
                const hasKnownChildren = (treeFoldersByPath[row.path]?.length ?? 0) > 0
                return (
                  <li key={row.path}>
                    <div className="flex items-center gap-1">
                      <button
                        className="h-6 w-6 rounded border border-slate-200 text-xs text-slate-500 hover:bg-slate-100"
                        onClick={() => void toggleNode(row.path)}
                        aria-label={isExpanded ? 'Replier dossier' : 'Déplier dossier'}
                      >
                        {isExpanded ? '▾' : hasKnownChildren || row.isRoot ? '▸' : '•'}
                      </button>
                      <button
                        className={`min-w-0 flex-1 truncate rounded px-2 py-1 text-left text-sm ${isActive ? 'bg-brand-50 text-brand-700' : 'text-slate-700 hover:bg-slate-100'}`}
                        style={{ paddingLeft: `${row.depth * 12 + 8}px` }}
                        title={row.path}
                        data-testid={`library-tree-${basename(row.path) || row.path}`}
                        onClick={() => void refreshPath(row.path)}
                        onContextMenu={(event) => openTreeContextMenu(event, row.path, row.isRoot)}
                      >
                        {row.isRoot ? (basename(row.path) || row.path) : basename(row.path)}
                      </button>
                    </div>
                  </li>
                )
              })}
            </ul>
          )}
        </aside>

        <div className="stack rounded-2xl border border-slate-200 bg-white p-3">
          <div className="flex flex-wrap gap-2">
            {breadcrumbs.length === 0 ? <span className="text-sm text-slate-500">-</span> : null}
            {breadcrumbs.map((crumb, idx) => (
              <button
                key={crumb.path}
                className={`rounded px-2 py-1 text-sm ${idx === breadcrumbs.length - 1 ? 'bg-brand-50 text-brand-700' : 'bg-slate-100 text-slate-700 hover:bg-slate-200'}`}
                onClick={() => void refreshPath(crumb.path)}
              >
                {crumb.label}
              </button>
            ))}
          </div>

          <div className="grid gap-2 md:grid-cols-[2fr_auto_auto_auto]">
            <input className="field" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Rechercher dans le dossier courant..." />
            <button className="btn" onClick={() => void refreshPath(parentPath(currentPath))} disabled={!canGoUp}>Monter</button>
            <button className="btn" onClick={() => void refreshPath(currentPath)} disabled={!currentPath}>Rafraîchir</button>
            <label className="inline-flex items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-2 py-1 text-xs text-slate-700">
              <input className="h-4 w-4" type="checkbox" checked={moveModeEnabled} onChange={(e) => setMoveModeEnabled(e.target.checked)} />
              Mode déplacement
            </label>
          </div>

          {canMoveCurrentFolderUp ? (
            <div>
              <button className="btn" onClick={() => void moveCurrentFolderToParent()}>
                Remonter ce dossier d’un cran
              </button>
            </div>
          ) : null}

          <div className="rounded-xl border border-dashed border-slate-300 bg-slate-50 px-3 py-2 text-xs text-slate-600">
            {moveModeEnabled ? 'Déplace par glisser-déposer ou via "Déplacer au parent".' : 'Le glisser-déposer est désactivé (active "Mode déplacement").'}
            {currentRoot ? ` Racine active: ${currentRoot}` : ''}
            {clipboardEntry ? ` Presse-papiers: ${clipboardEntry.name}` : ''}
          </div>

          {loading ? <p className="m-0 text-sm text-slate-500">Chargement de la bibliothèque...</p> : null}
          {!loading && filteredEntries.length === 0 ? (
            <div className="rounded-2xl border border-dashed border-slate-300 bg-white p-6 text-center text-sm text-slate-600">
              Aucun élément dans ce dossier.
            </div>
          ) : null}

          <div
            className="rounded-2xl border border-slate-200 bg-white"
            onDragOver={(event) => {
              if (!moveModeEnabled) return
              event.preventDefault()
            }}
            onDrop={(event) => {
              if (!moveModeEnabled) return
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
                const containerPath = parentPath(entry.path)
                const targetParent = parentPath(containerPath)
                const canMoveUp = Boolean(currentRoot) && containerPath !== entry.path && targetParent !== containerPath && isWithinRoot(currentRoot || '', targetParent)
                return (
                  <li
                    key={entry.path}
                    draggable={moveModeEnabled}
                    onDragStart={(event) => onDragStart(event, entry.path)}
                    onDragOver={(event) => {
                      if (!entry.isDir || !moveModeEnabled) return
                      event.preventDefault()
                    }}
                    onDrop={(event) => {
                      if (!entry.isDir || !moveModeEnabled) return
                      event.preventDefault()
                      void moveIntoFolder(entry.path)
                    }}
                    className={`flex flex-col gap-2 border-t border-slate-100 px-3 py-2 sm:flex-row sm:items-center sm:justify-between ${selectedPath === entry.path ? 'bg-brand-50/40' : ''}`}
                    data-testid={`library-entry-${entry.name}`}
                    onClick={() => setSelectedPath(entry.path)}
                    onContextMenu={(event) => openContextMenu(event, entry, true)}
                  >
                    <div className="min-w-0 text-sm text-slate-700">
                      <p className="m-0 truncate font-semibold text-slate-900">
                        {entry.isDir ? '📁' : '📄'} {entry.name}
                      </p>
                      <p className="m-0 truncate text-xs text-slate-500">{entry.path}</p>
                    </div>
                    <div className="flex flex-wrap gap-2">
                      <button className="btn" onClick={() => void openEntry(entry, mediaItem)}>Ouvrir</button>
                      {canMoveUp ? (
                        <button className="btn" onClick={() => void moveEntryToParent(entry)}>Déplacer au parent</button>
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
            Astuce: active "Mode déplacement" pour éviter les déplacements accidentels.
          </p>
          <p className="m-0 text-xs text-slate-500">Chemin courant: {currentPath || '-'}</p>
        </div>
      </div>
      {contextMenu ? (
        <div
          className="fixed z-50 min-w-48 rounded-lg border border-slate-200 bg-white p-1 shadow-xl"
          style={{ top: Math.max(8, contextMenu.y), left: Math.max(8, contextMenu.x) }}
          role="menu"
          data-testid="library-context-menu"
          onClick={(event) => event.stopPropagation()}
        >
          <button
            className="w-full rounded px-3 py-2 text-left text-sm text-slate-700 hover:bg-slate-100"
            role="menuitem"
            onClick={() => {
              const entry = contextMenu.entry
              const mediaItem = mediaByPath[entry.path]
              void openEntry(entry, mediaItem)
              setContextMenu(null)
            }}
          >
            {contextMenu.entry.isDir ? 'Ouvrir dossier' : 'Ouvrir'}
          </button>
          {canWriteInContextTarget ? (
            <button
              className="w-full rounded px-3 py-2 text-left text-sm text-slate-700 hover:bg-slate-100"
              role="menuitem"
              onClick={() => {
                const targetDir = contextTargetDir
                setContextMenu(null)
                void createFolderIn(targetDir)
              }}
            >
              Nouveau dossier ici
            </button>
          ) : null}
          {canWriteInContextTarget ? (
            <button
              className={`w-full rounded px-3 py-2 text-left text-sm ${canPasteInContextTarget ? 'text-slate-700 hover:bg-slate-100' : 'cursor-not-allowed text-slate-400'}`}
              role="menuitem"
              disabled={!canPasteInContextTarget}
              onClick={() => {
                const targetDir = contextTargetDir
                setContextMenu(null)
                void pasteInto(targetDir)
              }}
            >
              Coller ici{clipboardEntry ? ` (${clipboardEntry.name})` : ''}
            </button>
          ) : null}
          {contextMenu.allowMutations ? (
            <>
              <button
                className="w-full rounded px-3 py-2 text-left text-sm text-slate-700 hover:bg-slate-100"
                role="menuitem"
                onClick={() => {
                  const entry = contextMenu.entry
                  setClipboardEntry(entry)
                  setToast(`Coupé: ${entry.name}`)
                  setContextMenu(null)
                }}
              >
                Couper
              </button>
              <button
                className="w-full rounded px-3 py-2 text-left text-sm text-slate-700 hover:bg-slate-100"
                role="menuitem"
                onClick={() => {
                  const entry = contextMenu.entry
                  setContextMenu(null)
                  void renameEntry(entry)
                }}
              >
                Renommer
              </button>
              <button
                className="w-full rounded px-3 py-2 text-left text-sm text-slate-700 hover:bg-slate-100"
                role="menuitem"
                onClick={() => {
                  const entry = contextMenu.entry
                  setContextMenu(null)
                  void moveEntryToParent(entry)
                }}
              >
                Déplacer au parent
              </button>
              <button
                className="w-full rounded px-3 py-2 text-left text-sm text-rose-600 hover:bg-rose-50"
                role="menuitem"
                onClick={() => {
                  const entry = contextMenu.entry
                  setContextMenu(null)
                  void deleteEntry(entry)
                }}
              >
                Supprimer
              </button>
            </>
          ) : null}
        </div>
      ) : null}
    </section>
  )
}
