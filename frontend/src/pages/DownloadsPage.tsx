import { FormEvent, useEffect, useMemo, useState } from 'react'
import { useAPI } from '../api/context'
import type { DebridAccount, DownloadJob } from '../api/types'

function asKB(value: number) {
  return `${Math.round(value / 1024)} Ko`
}

function progressPercent(job: DownloadJob) {
  if (!job.sizeBytes) return 0
  return Math.min(100, Math.round((job.downloadedBytes / job.sizeBytes) * 100))
}

function parseSSEData(event: MessageEvent): Record<string, unknown> {
  try {
    const parsed = JSON.parse(event.data) as Record<string, unknown>
    return parsed ?? {}
  } catch {
    return {}
  }
}

function readPayloadString(payload: Record<string, unknown>, key: string): string {
  const value = payload[key]
  return typeof value === 'string' ? value : ''
}

function readPayloadNumber(payload: Record<string, unknown>, key: string): number | undefined {
  const value = payload[key]
  if (typeof value !== 'number' || Number.isNaN(value)) return undefined
  return value
}

function displayNameFromRawPath(raw: string): string {
  const trimmed = raw.trim()
  if (!trimmed) return ''
  const normalized = trimmed.replace(/\\/g, '/')
  const parts = normalized.split('/').filter(Boolean)
  return parts.length > 0 ? parts[parts.length - 1] : normalized
}

function duplicateToastLabel(payload: Record<string, unknown>): string {
  const destination = readPayloadString(payload, 'destination')
  if (destination) {
    const name = displayNameFromRawPath(destination)
    if (name) return name
  }
  for (const key of ['direct', 'source']) {
    const value = readPayloadString(payload, key)
    if (!value) continue
    try {
      const parsed = new URL(value)
      const name = displayNameFromRawPath(decodeURIComponent(parsed.pathname))
      if (name) return name
    } catch {
      const name = displayNameFromRawPath(value)
      if (name) return name
    }
  }
  return 'élément'
}

function jobHost(job: DownloadJob): string {
  const candidate = job.directLink || job.sourceLink
  try {
    const parsed = new URL(candidate)
    return parsed.host || '-'
  } catch {
    return '-'
  }
}

function retryLabel(job: DownloadJob): string {
  const retry = job.retries ?? 0
  const max = job.maxRetries ?? 0
  return `${retry}/${max}`
}

function nextRetryLabel(job: DownloadJob): string {
  const ms = job.nextRetryInMs ?? 0
  if (!ms || ms <= 0) return '-'
  return `${Math.ceil(ms / 1000)}s`
}

const eventsBaseURL = import.meta.env.VITE_API_BASE ?? ''
const eventsURL = eventsBaseURL ? `${eventsBaseURL}/api/events` : '/api/events'

export function DownloadsPage() {
  const api = useAPI()
  const [jobs, setJobs] = useState<DownloadJob[]>([])
  const [accounts, setAccounts] = useState<DebridAccount[]>([])
  const [links, setLinks] = useState('')
  const [useDebrid, setUseDebrid] = useState(false)
  const [accountId, setAccountID] = useState<number | undefined>(undefined)
  const [debridPassword, setDebridPassword] = useState('')
  const [destinationDir, setDestinationDir] = useState('')
  const [destinationHints, setDestinationHints] = useState<string[]>([])
  const [destinationPreset, setDestinationPreset] = useState('')
  const [loading, setLoading] = useState(false)
  const [loadingList, setLoadingList] = useState(true)
  const [statusFilter, setStatusFilter] = useState<string>('all')
  const [toast, setToast] = useState('')
  const [error, setError] = useState('')

  async function refresh() {
    setError('')
    try {
      const [list, accountList, rootsPayload] = await Promise.all([api.listDownloads(), api.listDebridAccounts(), api.getFSRoots()])
      setJobs(list)
      setAccounts(accountList)
      const roots = rootsPayload.roots ?? []
      const hints = new Set<string>()
      for (const root of roots) {
        hints.add(root)
        try {
          const listing = await api.listFS(root)
          for (const item of listing.items) {
            if (item.isDir) {
              hints.add(item.path)
            }
          }
        } catch {
          // Keep other hints even if one root cannot be listed.
        }
      }
      setDestinationHints(Array.from(hints).sort((a, b) => a.localeCompare(b)))
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Impossible de charger les téléchargements')
    } finally {
      setLoadingList(false)
    }
  }

  useEffect(() => {
    void refresh()
  }, [])

  useEffect(() => {
    if (typeof window === 'undefined' || typeof window.EventSource === 'undefined') {
      return
    }

    const source = new window.EventSource(eventsURL)

    const applyStatus = (status: DownloadJob['status']) => (event: MessageEvent) => {
      const payload = parseSSEData(event)
      const id = String(payload.id ?? '')
      const eventError = readPayloadString(payload, 'error') || readPayloadString(payload, 'errorMessage')
      if (!id) return
      setJobs((prev) => prev.map((job) => {
        if (job.id !== id) return job
        const next: DownloadJob = { ...job, status }
        if (status === 'paused' || status === 'completed' || status === 'canceled' || status === 'failed') {
          next.speedBytes = 0
        }
        if (status === 'completed') {
          next.etaSeconds = 0
        }
        if (status === 'failed') {
          next.errorMessage = eventError || next.errorMessage || 'Échec sans détail'
        } else if (status === 'running' || status === 'queued' || status === 'completed') {
          next.errorMessage = ''
          next.nextRetryInMs = 0
        }
        return next
      }))
    }

    const onProgress = (event: MessageEvent) => {
      const payload = parseSSEData(event)
      const id = String(payload.id ?? '')
      if (!id) return
      const downloadedBytes = Number(payload.downloadedBytes ?? 0)
      const sizeBytes = Number(payload.sizeBytes ?? 0)
      const speedBytes = Number(payload.speedBytes ?? 0)
      const etaSeconds = Number(payload.etaSeconds ?? 0)

      setJobs((prev) => prev.map((job) => (
        job.id === id
          ? {
              ...job,
              status: job.status === 'queued' ? 'running' : job.status,
              downloadedBytes,
              sizeBytes,
              speedBytes,
              etaSeconds,
              errorMessage: ''
            }
          : job
      )))
    }

    const onRetry = (event: MessageEvent) => {
      const payload = parseSSEData(event)
      const id = String(payload.id ?? '')
      const eventError = readPayloadString(payload, 'error') || readPayloadString(payload, 'errorMessage')
      const retry = readPayloadNumber(payload, 'retry')
      const retryMax = readPayloadNumber(payload, 'retryMax')
      const nextRetryInMs = readPayloadNumber(payload, 'nextRetryInMs')
      if (!id) return
      setJobs((prev) => prev.map((job) => (
        job.id === id
          ? {
              ...job,
              status: 'running',
              errorMessage: eventError || job.errorMessage || '',
              retries: retry ?? job.retries,
              maxRetries: retryMax ?? job.maxRetries,
              nextRetryInMs: nextRetryInMs ?? job.nextRetryInMs
            }
          : job
      )))
    }

    const onDuplicateSkipped = (event: MessageEvent) => {
      const payload = parseSSEData(event)
      setToast(`Doublon ignoré: ${duplicateToastLabel(payload)}`)
    }

    source.addEventListener('download_progress', onProgress as EventListener)
    source.addEventListener('download_retry', onRetry as EventListener)
    source.addEventListener('download_queued', applyStatus('queued') as EventListener)
    source.addEventListener('download_started', applyStatus('running') as EventListener)
    source.addEventListener('download_paused', applyStatus('paused') as EventListener)
    source.addEventListener('download_completed', applyStatus('completed') as EventListener)
    source.addEventListener('download_failed', applyStatus('failed') as EventListener)
    source.addEventListener('download_canceled', applyStatus('canceled') as EventListener)
    source.addEventListener('download_duplicate_skipped', onDuplicateSkipped as EventListener)

    return () => {
      source.close()
    }
  }, [])

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setLoading(true)
    setError('')
    try {
      const created = await api.addDownloads({
        links: links.split('\n').map((v) => v.trim()).filter(Boolean),
        useDebrid,
        debridAccountId: accountId,
        debridPassword: debridPassword.trim() || undefined,
        destinationDir: destinationDir.trim() || undefined
      })
      setJobs((prev) => [...created, ...prev])
      setLinks('')
      setDebridPassword('')
      setToast(`${created.length} job(s) ajouté(s)`)
    } catch (err) {
      setError(err instanceof Error ? err.message : "Impossible d'ajouter le téléchargement")
    } finally {
      setLoading(false)
    }
  }

  const visibleJobs = useMemo(
    () => jobs.filter((job) => statusFilter === 'all' || job.status === statusFilter),
    [jobs, statusFilter]
  )

  async function runAction(action: () => Promise<void>) {
    setError('')
    try {
      await action()
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Action impossible')
    }
  }

  return (
    <section className="panel stack" data-testid="downloads-page">
      <div className="row">
        <div>
          <h1 className="m-0 text-2xl font-bold text-slate-900">Dashboard téléchargements</h1>
          <p className="m-0 mt-1 text-sm text-slate-600">Ajoute des liens, gère la queue et pilote les jobs en un clic.</p>
        </div>
      </div>

      {toast ? <p role="status" className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{toast}</p> : null}
      {error ? <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p> : null}

      <form onSubmit={onSubmit} className="grid gap-3 rounded-2xl border border-slate-200 bg-slate-50/70 p-4">
        <textarea
          className="field min-h-24"
          placeholder="Collez un ou plusieurs liens"
          value={links}
          onChange={(e) => setLinks(e.target.value)}
          rows={3}
          required
        />
        <input
          className="field"
          placeholder="Dossier destination (optionnel)"
          list="downloads-destination-hints"
          value={destinationDir}
          onChange={(e) => setDestinationDir(e.target.value)}
        />
        {destinationHints.length > 0 ? (
          <select
            className="field"
            aria-label="Dossiers disponibles"
            value={destinationPreset}
            onChange={(e) => {
              const next = e.target.value
              setDestinationPreset(next)
              if (next) {
                setDestinationDir(next)
              }
            }}
          >
            <option value="">Choisir un dossier existant (optionnel)</option>
            {destinationHints.map((path) => (
              <option value={path} key={path}>{path}</option>
            ))}
          </select>
        ) : null}
        <datalist id="downloads-destination-hints">
          {destinationHints.map((path) => (
            <option value={path} key={path} />
          ))}
        </datalist>
        <div className="row gap-3">
          <label className="inline-flex items-center gap-2 text-sm text-slate-700">
            <input className="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-300" type="checkbox" checked={useDebrid} onChange={(e) => setUseDebrid(e.target.checked)} />
            Utiliser un débrideur
          </label>
          <div className="flex w-full flex-col gap-2 md:max-w-sm">
            {useDebrid ? (
              <>
                <select className="field" value={accountId ?? ''} onChange={(e) => setAccountID(e.target.value ? Number(e.target.value) : undefined)}>
                  <option value="">Compte débrideur par défaut</option>
                  {accounts.map((account) => (
                    <option value={account.id} key={account.id}>
                      {account.label}
                    </option>
                  ))}
                </select>
                <input
                  className="field"
                  type="password"
                  placeholder="Mot de passe lien (optionnel)"
                  value={debridPassword}
                  onChange={(e) => setDebridPassword(e.target.value)}
                />
              </>
            ) : null}
          </div>
          <button className="btn btn-primary md:ml-auto" disabled={loading} type="submit">
            {loading ? 'Ajout...' : 'Ajouter'}
          </button>
        </div>
      </form>

      <div className="row rounded-2xl border border-slate-200 bg-white p-3">
        <label htmlFor="downloads-filter" className="text-sm font-medium text-slate-700">Filtrer</label>
        <select id="downloads-filter" className="field md:max-w-xs" value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
          <option value="all">Tous</option>
          <option value="queued">En attente</option>
          <option value="running">En cours</option>
          <option value="paused">En pause</option>
          <option value="completed">Terminés</option>
          <option value="failed">Échecs</option>
        </select>
      </div>

      {loadingList ? <p className="m-0 text-sm text-slate-500">Chargement des jobs...</p> : null}
      {!loadingList && visibleJobs.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-slate-300 bg-white p-6 text-center text-sm text-slate-600">
          Aucun téléchargement pour ce filtre.
        </div>
      ) : null}

      <ul className="list">
        {visibleJobs.map((job) => (
          <li key={job.id} className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
            <div className="row">
              <strong className="break-all text-base text-slate-900">{job.fileName || job.sourceLink}</strong>
              <span className={`status status-${job.status}`}>{job.status}</span>
            </div>

            <div className="mt-3 h-2 w-full overflow-hidden rounded-full bg-slate-200">
              <div className="h-full rounded-full bg-brand-500 transition-all" style={{ width: `${progressPercent(job)}%` }} />
            </div>

            <div className="mt-2 row text-sm text-slate-600">
              <small>{asKB(job.downloadedBytes)} / {asKB(job.sizeBytes)}</small>
              <small>vitesse {asKB(job.speedBytes)}/s</small>
              <small>ETA {job.etaSeconds > 0 ? `${job.etaSeconds}s` : '-'}</small>
            </div>
            <div className="mt-1 row text-xs text-slate-500">
              <small>Hôte {jobHost(job)}</small>
              <small>Retry {retryLabel(job)}</small>
              <small>Prochaine tentative {nextRetryLabel(job)}</small>
            </div>
            {job.errorMessage ? (
              <p className="mt-2 rounded-lg border border-rose-200 bg-rose-50 px-2 py-1 text-xs text-rose-700">
                Raison: {job.errorMessage}
              </p>
            ) : null}

            <div className="mt-4 flex flex-wrap gap-2">
              <button className="btn" onClick={() => void runAction(() => api.startDownload(job.id))}>Start</button>
              <button className="btn" onClick={() => void runAction(() => api.pauseDownload(job.id))}>Pause</button>
              <button className="btn" onClick={() => void runAction(() => api.resumeDownload(job.id))}>Resume</button>
              <button
                className="btn btn-danger"
                onClick={() => {
                  if (window.confirm('Supprimer ce téléchargement ?')) {
                    void runAction(async () => {
                      await api.deleteDownload(job.id)
                      setToast('Téléchargement supprimé')
                    })
                  }
                }}
              >
                Supprimer
              </button>
            </div>
          </li>
        ))}
      </ul>
    </section>
  )
}
