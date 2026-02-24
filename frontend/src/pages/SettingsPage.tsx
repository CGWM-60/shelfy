import { FormEvent, useEffect, useState } from 'react'
import { useAPI } from '../api/context'
import type { AppSettings, AuthStatus, DLNAStatus, FSManagedEntry, SMBStatus } from '../api/types'
import { applyTheme } from '../theme'

const initialState: AppSettings = {
  downloadMaxConcurrent: 3,
  downloadAutoResume: true,
  downloadsPath: '',
  libraryPaths: [],
  globalRateLimitKB: 0,
  aiEnabled: true,
  aiTopK: 5,
  theme: 'clair',
  visibleColumns: ['nom', 'etat'],
  fileServerAuthEnabled: false,
  fileServerAuthUser: '',
  dlnaEnabled: false,
  smbEnabled: false,
  smbShareName: 'shelfy',
  smbSharePath: ''
}

function asArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : []
}

function normalizeSettings(raw: Partial<AppSettings> | undefined): AppSettings {
  const next = raw ?? {}
  return {
    ...initialState,
    ...next,
    libraryPaths: asArray(next.libraryPaths).filter((v): v is string => typeof v === 'string'),
    visibleColumns: asArray(next.visibleColumns).filter((v): v is string => typeof v === 'string')
  }
}

function normalizeDLNAStatus(raw: Partial<DLNAStatus> | undefined): DLNAStatus {
  const next = raw ?? {}
  return {
    enabled: Boolean(next.enabled),
    devices: asArray(next.devices),
    lastScan: typeof next.lastScan === 'string' ? next.lastScan : '',
    lastError: next.lastError
  }
}

function normalizeSMBStatus(raw: Partial<SMBStatus> | undefined): SMBStatus {
  const next = raw ?? {}
  return {
    enabled: Boolean(next.enabled),
    running: Boolean(next.running),
    backend: typeof next.backend === 'string' ? next.backend : 'none',
    shareName: typeof next.shareName === 'string' && next.shareName.trim() ? next.shareName : 'shelfy',
    sharePath: typeof next.sharePath === 'string' ? next.sharePath : '',
    updatedAt: typeof next.updatedAt === 'string' ? next.updatedAt : '',
    lastError: next.lastError,
    clients: asArray(next.clients)
  }
}

export function SettingsPage() {
  const api = useAPI()
  const [settings, setSettings] = useState<AppSettings>(initialState)
  const [saved, setSaved] = useState('')
  const [error, setError] = useState('')
  const [roots, setRoots] = useState<string[]>([])
  const [currentPath, setCurrentPath] = useState('')
  const [items, setItems] = useState<FSManagedEntry[]>([])
  const [fsLoading, setFSLoading] = useState(false)
  const [fsError, setFSError] = useState('')
  const [fsStatus, setFSStatus] = useState('')
  const [newFolderName, setNewFolderName] = useState('')
  const [editorPath, setEditorPath] = useState('')
  const [editorContent, setEditorContent] = useState('')
  const [moveFromPath, setMoveFromPath] = useState('')
  const [moveToPath, setMoveToPath] = useState('')
  const [dlnaStatus, setDLNAStatus] = useState<DLNAStatus>({ enabled: false, devices: [], lastScan: '' })
  const [dlnaLoading, setDLNALoading] = useState(false)
  const [dlnaError, setDLNAError] = useState('')
  const [smbStatus, setSMBStatus] = useState<SMBStatus>({ enabled: false, running: false, backend: 'none', shareName: 'shelfy', sharePath: '', updatedAt: '', clients: [] })
  const [smbLoading, setSMBLoading] = useState(false)
  const [smbError, setSMBError] = useState('')
  const [authStatus, setAuthStatus] = useState<AuthStatus>({ enabled: false, authenticated: true, username: '' })
  const [authLoading, setAuthLoading] = useState(false)

  useEffect(() => {
    Promise.all([api.getSettings(), api.getFSRoots(), api.getDLNADevices(), api.getSMBStatus(), api.authStatus()])
      .then(([loadedSettings, loadedRoots, loadedDLNA, loadedSMB, loadedAuth]) => {
        setSettings(normalizeSettings(loadedSettings))
        const safeRoots = asArray(loadedRoots?.roots)
        setRoots(safeRoots)
        setDLNAStatus(normalizeDLNAStatus(loadedDLNA))
        setSMBStatus(normalizeSMBStatus(loadedSMB))
        setAuthStatus({
          enabled: Boolean(loadedAuth?.enabled),
          authenticated: Boolean(loadedAuth?.authenticated),
          username: loadedAuth?.username || ''
        })
        const firstRoot = safeRoots[0] ?? ''
        if (firstRoot) {
          void refreshFS(firstRoot)
        }
      })
      .catch((err: unknown) => {
        setError(err instanceof Error ? err.message : 'Impossible de charger les paramètres')
      })
  }, [api])

  async function refreshAuthStatus() {
    setAuthLoading(true)
    try {
      const next = await api.authStatus()
      setAuthStatus({
        enabled: Boolean(next.enabled),
        authenticated: Boolean(next.authenticated),
        username: next.username || ''
      })
    } catch (err) {
      setError(err instanceof Error ? err.message : "Impossible de charger l'état connexion")
    } finally {
      setAuthLoading(false)
    }
  }

  async function logoutSession() {
    setAuthLoading(true)
    try {
      await api.authLogout()
      await refreshAuthStatus()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Impossible de se déconnecter')
    } finally {
      setAuthLoading(false)
    }
  }

  useEffect(() => {
    applyTheme(settings.theme || 'clair')
  }, [settings.theme])

  function joinPath(base: string, name: string) {
    const cleanBase = base.endsWith('/') ? base.slice(0, -1) : base
    const cleanName = name.startsWith('/') ? name.slice(1) : name
    return `${cleanBase}/${cleanName}`
  }

  function parentPath(path: string) {
    const normalized = path.replace(/\/+$/, '')
    const idx = normalized.lastIndexOf('/')
    if (idx <= 0) {
      return path
    }
    return normalized.slice(0, idx)
  }

  async function refreshFS(path?: string) {
    const nextPath = path ?? currentPath
    if (!nextPath) return
    setFSLoading(true)
    setFSError('')
    try {
      const listing = await api.listFS(nextPath)
      setCurrentPath(typeof listing?.path === 'string' && listing.path ? listing.path : nextPath)
      setItems(asArray(listing?.items))
    } catch (err) {
      setFSError(err instanceof Error ? err.message : 'Impossible de charger les fichiers')
    } finally {
      setFSLoading(false)
    }
  }

  async function submit(e: FormEvent) {
    e.preventDefault()
    setSaved('')
    setError('')
    try {
      const updated = await api.saveSettings(settings)
      setSettings(normalizeSettings(updated))
      if (updated.dlnaEnabled) {
        await refreshDLNA(true)
      } else {
        setDLNAStatus({ enabled: false, devices: [], lastScan: '' })
      }
      if (updated.smbEnabled) {
        await refreshSMB(true)
      } else {
        setSMBStatus((prev) => ({ ...prev, enabled: false, running: false, clients: [] }))
      }
      setSaved('Paramètres enregistrés')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Échec de sauvegarde')
    }
  }

  async function refreshDLNA(scan: boolean) {
    setDLNALoading(true)
    setDLNAError('')
    try {
      const status = scan ? await api.scanDLNADevices() : await api.getDLNADevices()
      setDLNAStatus(normalizeDLNAStatus(status))
    } catch (err) {
      setDLNAError(err instanceof Error ? err.message : 'Impossible de scanner les appareils DLNA')
    } finally {
      setDLNALoading(false)
    }
  }

  async function refreshSMB(forceRefresh: boolean) {
    setSMBLoading(true)
    setSMBError('')
    try {
      const status = forceRefresh ? await api.refreshSMBStatus() : await api.getSMBStatus()
      setSMBStatus(normalizeSMBStatus(status))
    } catch (err) {
      setSMBError(err instanceof Error ? err.message : 'Impossible de rafraîchir SMB')
    } finally {
      setSMBLoading(false)
    }
  }

  async function createFolder() {
    if (!currentPath || !newFolderName.trim()) return
    setFSError('')
    setFSStatus('')
    try {
      await api.createFSDir(joinPath(currentPath, newFolderName.trim()))
      setNewFolderName('')
      setFSStatus('Dossier créé')
      await refreshFS(currentPath)
    } catch (err) {
      setFSError(err instanceof Error ? err.message : 'Création dossier impossible')
    }
  }

  async function saveFile() {
    if (!editorPath.trim()) return
    setFSError('')
    setFSStatus('')
    try {
      await api.writeFSFile(editorPath.trim(), editorContent)
      setFSStatus('Fichier enregistré')
      await refreshFS(currentPath || roots[0] || '')
    } catch (err) {
      setFSError(err instanceof Error ? err.message : 'Enregistrement fichier impossible')
    }
  }

  async function loadFile(path: string) {
    setFSError('')
    try {
      const payload = await api.readFSFile(path)
      setEditorPath(payload.path)
      setEditorContent(payload.content)
    } catch (err) {
      setFSError(err instanceof Error ? err.message : 'Lecture fichier impossible')
    }
  }

  async function movePath() {
    if (!moveFromPath.trim() || !moveToPath.trim()) return
    setFSError('')
    setFSStatus('')
    try {
      await api.moveFSPath(moveFromPath.trim(), moveToPath.trim())
      setFSStatus('Déplacement effectué')
      await refreshFS(currentPath || roots[0] || '')
    } catch (err) {
      setFSError(err instanceof Error ? err.message : 'Déplacement impossible')
    }
  }

  async function deletePath(item: FSManagedEntry) {
    const recursive = item.isDir
    const confirmed = window.confirm(item.isDir ? `Supprimer le dossier "${item.name}" et tout son contenu ?` : `Supprimer le fichier "${item.name}" ?`)
    if (!confirmed) return
    setFSError('')
    setFSStatus('')
    try {
      await api.deleteFSPath(item.path, recursive)
      setFSStatus('Suppression effectuée')
      if (editorPath === item.path) {
        setEditorPath('')
        setEditorContent('')
      }
      await refreshFS(currentPath || roots[0] || '')
    } catch (err) {
      setFSError(err instanceof Error ? err.message : 'Suppression impossible')
    }
  }

  return (
    <section className="panel stack" data-testid="settings-page">
      <div>
        <h1 className="m-0 text-2xl font-bold text-slate-900">Paramètres</h1>
        <p className="m-0 mt-1 text-sm text-slate-600">Configure concurrence, stockage, IA et expérience UI.</p>
      </div>
      {saved ? <p role="status" className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{saved}</p> : null}
      {error ? <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p> : null}

      <article className="stack rounded-2xl border border-slate-200 bg-white p-4">
        <div className="row">
          <div>
            <h2 className="m-0 text-lg font-semibold text-slate-900">Connexion application</h2>
            <p className="m-0 mt-1 text-sm text-slate-600">Contrôle rapide de la session de connexion et état de la protection API.</p>
          </div>
          <div className="flex gap-2">
            <button type="button" className="btn" onClick={() => void refreshAuthStatus()} disabled={authLoading}>
              {authLoading ? 'Vérification...' : 'Vérifier'}
            </button>
            {authStatus.enabled && authStatus.authenticated ? (
              <button type="button" className="btn" onClick={() => void logoutSession()} disabled={authLoading}>
                Déconnexion
              </button>
            ) : null}
          </div>
        </div>
        <p className="m-0 text-sm text-slate-700">
          Protection: <strong>{authStatus.enabled ? 'active' : 'désactivée'}</strong> • Session: <strong>{authStatus.authenticated ? 'ouverte' : 'fermée'}</strong>
          {authStatus.username ? (
            <>
              {' '}• Utilisateur: <strong>{authStatus.username}</strong>
            </>
          ) : null}
        </p>
        {!authStatus.enabled ? (
          <p className="m-0 rounded-xl border border-amber-200 bg-amber-50 px-3 py-2 text-sm text-amber-800">
            Active `AUTH_ENABLED=true` + `AUTH_USER` + `AUTH_PASS` + `AUTH_SESSION_SECRET` dans `.env`, puis redémarre pour afficher la page login au démarrage.
          </p>
        ) : null}
      </article>

      <form onSubmit={submit} className="grid gap-4 rounded-2xl border border-slate-200 bg-slate-50/70 p-4 md:grid-cols-2">
        <label className="stack text-sm text-slate-700">
          Téléchargements concurrents
          <input className="field" type="number" min={1} value={settings.downloadMaxConcurrent} onChange={(e) => setSettings({ ...settings, downloadMaxConcurrent: Number(e.target.value) })} />
        </label>
        <label className="stack text-sm text-slate-700">
          Top K IA
          <input className="field" type="number" min={1} value={settings.aiTopK} onChange={(e) => setSettings({ ...settings, aiTopK: Number(e.target.value) })} />
        </label>
        <label className="stack text-sm text-slate-700 md:col-span-2">
          Dossier téléchargements
          <input className="field" value={settings.downloadsPath} onChange={(e) => setSettings({ ...settings, downloadsPath: e.target.value })} />
        </label>
        <label className="stack text-sm text-slate-700 md:col-span-2">
          Dossiers bibliothèque (séparés par virgule)
          <input className="field" value={settings.libraryPaths.join(',')} onChange={(e) => setSettings({ ...settings, libraryPaths: e.target.value.split(',').map((v) => v.trim()).filter(Boolean) })} />
        </label>
        <label className="inline-flex items-center gap-2 text-sm text-slate-700">
          <input className="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-300" type="checkbox" checked={settings.downloadAutoResume} onChange={(e) => setSettings({ ...settings, downloadAutoResume: e.target.checked })} />
          Auto-resume au redémarrage
        </label>
        <label className="inline-flex items-center gap-2 text-sm text-slate-700">
          <input className="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-300" type="checkbox" checked={settings.aiEnabled} onChange={(e) => setSettings({ ...settings, aiEnabled: e.target.checked })} />
          IA activée
        </label>
        <label className="inline-flex items-center gap-2 text-sm text-slate-700">
          <input className="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-300" type="checkbox" checked={settings.dlnaEnabled} onChange={(e) => setSettings({ ...settings, dlnaEnabled: e.target.checked })} />
          DLNA activé
        </label>
        <label className="inline-flex items-center gap-2 text-sm text-slate-700">
          <input className="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-300" type="checkbox" checked={settings.smbEnabled} onChange={(e) => setSettings({ ...settings, smbEnabled: e.target.checked })} />
          SMB activé
        </label>
        <label className="stack text-sm text-slate-700">
          Nom partage SMB
          <input className="field" value={settings.smbShareName} onChange={(e) => setSettings({ ...settings, smbShareName: e.target.value })} placeholder="shelfy" />
        </label>
        <label className="stack text-sm text-slate-700 md:col-span-2">
          Chemin partage SMB
          <input className="field" value={settings.smbSharePath} onChange={(e) => setSettings({ ...settings, smbSharePath: e.target.value })} placeholder="/chemin/partage" />
        </label>
        <label className="stack text-sm text-slate-700">
          Thème
          <select className="field" value={settings.theme} onChange={(e) => setSettings({ ...settings, theme: e.target.value })}>
            <option value="clair">Clair</option>
            <option value="sombre">Sombre</option>
          </select>
        </label>
        <div className="md:col-span-2">
          <button className="btn btn-primary" type="submit">Enregistrer</button>
        </div>
      </form>

      <article className="stack rounded-2xl border border-slate-200 bg-white p-4">
        <div className="row">
          <div>
            <h2 className="m-0 text-lg font-semibold text-slate-900">Serveur DLNA</h2>
            <p className="m-0 mt-1 text-sm text-slate-600">Active le serveur DLNA et consulte les clients qui se connectent (TV, VLC, box).</p>
          </div>
          <button type="button" className="btn" disabled={!settings.dlnaEnabled || dlnaLoading} onClick={() => void refreshDLNA(true)}>
            {dlnaLoading ? 'Actualisation...' : 'Actualiser clients'}
          </button>
        </div>
        {!settings.dlnaEnabled ? <p className="m-0 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">DLNA est désactivé dans les paramètres.</p> : null}
        {dlnaError ? <p role="alert" className="m-0 rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{dlnaError}</p> : null}
        {settings.dlnaEnabled && !dlnaLoading && dlnaStatus.devices.length === 0 ? (
          <p className="m-0 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">Aucun client connecté au serveur DLNA pour le moment.</p>
        ) : null}
        {dlnaStatus.devices.length > 0 ? (
          <ul className="m-0 list-none space-y-2 p-0" data-testid="dlna-device-list">
            {dlnaStatus.devices.map((device) => (
              <li key={device.usn || device.location} className="rounded-xl border border-slate-200 bg-slate-50/70 px-3 py-2">
                <p className="m-0 text-sm font-semibold text-slate-900">{device.usn || 'Appareil sans USN'}</p>
                <p className="m-0 text-xs text-slate-600">{device.server || device.st || 'Type inconnu'}</p>
                <p className="m-0 truncate text-xs text-slate-500">{device.location || device.address}</p>
              </li>
            ))}
          </ul>
        ) : null}
      </article>

      <article className="stack rounded-2xl border border-slate-200 bg-white p-4">
        <div className="row">
          <div>
            <h2 className="m-0 text-lg font-semibold text-slate-900">Serveur SMB</h2>
            <p className="m-0 mt-1 text-sm text-slate-600">Partage réseau SMB avec le nom de partage configuré (par défaut: shelfy).</p>
          </div>
          <button type="button" className="btn" disabled={!settings.smbEnabled || smbLoading} onClick={() => void refreshSMB(true)}>
            {smbLoading ? 'Actualisation...' : 'Actualiser SMB'}
          </button>
        </div>
        {!settings.smbEnabled ? <p className="m-0 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">SMB est désactivé dans les paramètres.</p> : null}
        {smbError ? <p role="alert" className="m-0 rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{smbError}</p> : null}
        {settings.smbEnabled ? (
          <p className="m-0 text-sm text-slate-700">
            Statut: <strong>{smbStatus.running ? 'En ligne' : 'Hors ligne'}</strong> | Share: <code>{smbStatus.shareName || settings.smbShareName || 'shelfy'}</code>
          </p>
        ) : null}
        {settings.smbEnabled && smbStatus.clients.length === 0 ? (
          <p className="m-0 rounded-xl border border-slate-200 bg-slate-50 px-3 py-2 text-sm text-slate-600">Aucun client SMB connecté pour le moment.</p>
        ) : null}
        {smbStatus.clients.length > 0 ? (
          <ul className="m-0 list-none space-y-2 p-0" data-testid="smb-client-list">
            {smbStatus.clients.map((client, index) => (
              <li key={`${client.address}-${index}`} className="rounded-xl border border-slate-200 bg-slate-50/70 px-3 py-2">
                <p className="m-0 text-sm font-semibold text-slate-900">{client.machine || 'Client SMB'}</p>
                <p className="m-0 text-xs text-slate-600">{client.username || 'guest'} • {client.address || 'adresse inconnue'}</p>
              </li>
            ))}
          </ul>
        ) : null}
      </article>

      <article className="stack rounded-2xl border border-slate-200 bg-white p-4">
        <div className="row">
          <div>
            <h2 className="m-0 text-lg font-semibold text-slate-900">Gestionnaire fichiers</h2>
            <p className="m-0 mt-1 text-sm text-slate-600">Créer, modifier, déplacer et supprimer fichiers/dossiers (suppression récursive incluse).</p>
          </div>
        </div>

        {fsStatus ? <p role="status" className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{fsStatus}</p> : null}
        {fsError ? <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{fsError}</p> : null}

        <div className="grid gap-3 md:grid-cols-3">
          <label className="stack text-sm text-slate-700 md:col-span-2">
            Racine / dossier courant
            <select className="field" value={currentPath} onChange={(e) => void refreshFS(e.target.value)}>
              {roots.map((root) => (
                <option value={root} key={root}>
                  {root}
                </option>
              ))}
              {currentPath && !roots.includes(currentPath) ? <option value={currentPath}>{currentPath}</option> : null}
            </select>
          </label>
          <div className="mt-auto flex gap-2">
            <button type="button" className="btn" onClick={() => void refreshFS(parentPath(currentPath))} disabled={!currentPath}>
              Monter
            </button>
            <button type="button" className="btn" onClick={() => void refreshFS(currentPath)} disabled={!currentPath}>
              Rafraîchir
            </button>
          </div>
        </div>

        <div className="grid gap-3 md:grid-cols-3">
          <label className="stack text-sm text-slate-700">
            Nouveau dossier
            <input className="field" placeholder="Nom dossier" value={newFolderName} onChange={(e) => setNewFolderName(e.target.value)} />
          </label>
          <div className="mt-auto">
            <button type="button" className="btn" onClick={() => void createFolder()} disabled={!currentPath || !newFolderName.trim()}>
              Créer dossier
            </button>
          </div>
        </div>

        <div className="rounded-2xl border border-slate-200">
          <div className="row border-b border-slate-200 px-3 py-2 text-xs font-semibold uppercase tracking-[0.14em] text-slate-500">
            <span>Nom</span>
            <span>Actions</span>
          </div>
          {fsLoading ? <p className="m-0 px-3 py-3 text-sm text-slate-500">Chargement...</p> : null}
          {!fsLoading && items.length === 0 ? <p className="m-0 px-3 py-3 text-sm text-slate-500">Aucun élément</p> : null}
          <ul className="m-0 list-none p-0">
            {items.map((item) => (
              <li key={item.path} className="row border-t border-slate-100 px-3 py-2">
                <div className="min-w-0 text-sm text-slate-700">
                  <strong className="font-semibold text-slate-900">{item.isDir ? '[DIR]' : '[FILE]'} {item.name}</strong>
                  <p className="m-0 truncate text-xs text-slate-500">{item.path}</p>
                </div>
                <div className="flex flex-wrap gap-2">
                  {item.isDir ? (
                    <button type="button" className="btn" onClick={() => void refreshFS(item.path)}>Ouvrir</button>
                  ) : (
                    <button type="button" className="btn" onClick={() => void loadFile(item.path)}>Éditer</button>
                  )}
                  <button type="button" className="btn" onClick={() => {
                    setMoveFromPath(item.path)
                    setMoveToPath(item.path)
                  }}>
                    Préparer move
                  </button>
                  <button type="button" className="btn btn-danger" onClick={() => void deletePath(item)}>
                    Supprimer
                  </button>
                </div>
              </li>
            ))}
          </ul>
        </div>

        <div className="grid gap-3 md:grid-cols-2">
          <label className="stack text-sm text-slate-700">
            From path
            <input className="field" value={moveFromPath} onChange={(e) => setMoveFromPath(e.target.value)} placeholder="/abs/path/source" />
          </label>
          <label className="stack text-sm text-slate-700">
            To path
            <input className="field" value={moveToPath} onChange={(e) => setMoveToPath(e.target.value)} placeholder="/abs/path/destination" />
          </label>
          <div>
            <button type="button" className="btn" onClick={() => void movePath()} disabled={!moveFromPath.trim() || !moveToPath.trim()}>
              Déplacer
            </button>
          </div>
        </div>

        <div className="stack rounded-2xl border border-slate-200 bg-slate-50/70 p-3">
          <h3 className="m-0 text-sm font-semibold text-slate-900">Éditeur fichier</h3>
          <label className="stack text-sm text-slate-700">
            Chemin fichier
            <input className="field" value={editorPath} onChange={(e) => setEditorPath(e.target.value)} placeholder={currentPath ? `${currentPath}/notes.txt` : '/abs/path/file.txt'} />
          </label>
          <label className="stack text-sm text-slate-700">
            Contenu
            <textarea className="field min-h-28" value={editorContent} onChange={(e) => setEditorContent(e.target.value)} />
          </label>
          <div>
            <button type="button" className="btn btn-primary" onClick={() => void saveFile()} disabled={!editorPath.trim()}>
              Enregistrer fichier
            </button>
          </div>
        </div>
      </article>
    </section>
  )
}
