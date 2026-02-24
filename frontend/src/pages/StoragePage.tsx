import { useEffect, useMemo, useState } from 'react'
import { useAPI } from '../api/context'
import type { StorageSpeedtestResult, StorageStats } from '../api/types'

function formatBytes(value: number) {
  if (!Number.isFinite(value) || value <= 0) return '0 o'
  const units = ['o', 'Ko', 'Mo', 'Go', 'To']
  let size = value
  let idx = 0
  while (size >= 1024 && idx < units.length - 1) {
    size /= 1024
    idx++
  }
  return `${size.toFixed(size >= 10 || idx === 0 ? 0 : 1)} ${units[idx]}`
}

export function StoragePage() {
  const api = useAPI()
  const [stats, setStats] = useState<StorageStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [speedError, setSpeedError] = useState('')
  const [speedBusy, setSpeedBusy] = useState(false)
  const [speedResult, setSpeedResult] = useState<StorageSpeedtestResult | null>(null)
  const [sampleMB, setSampleMB] = useState(8)

  async function loadStats() {
    setError('')
    try {
      const payload = await api.getStorageStats()
      setStats(payload)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Impossible de charger les stats stockage')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void loadStats()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  async function runSpeedtest() {
    setSpeedError('')
    setSpeedBusy(true)
    try {
      const payload = await api.runStorageSpeedtest(sampleMB)
      setSpeedResult(payload)
      await loadStats()
    } catch (err) {
      setSpeedError(err instanceof Error ? err.message : 'Échec du test de débit')
    } finally {
      setSpeedBusy(false)
    }
  }

  const usagePercent = useMemo(() => {
    if (!stats || stats.totals.totalBytes <= 0) return 0
    const used = Math.max(0, stats.totals.totalBytes - stats.totals.freeBytes)
    return Math.round((used / stats.totals.totalBytes) * 100)
  }, [stats])

  if (loading) {
    return (
      <section className="panel" data-testid="storage-page">
        <p className="m-0 text-sm text-slate-600">Chargement des statistiques stockage...</p>
      </section>
    )
  }

  return (
    <section className="stack" data-testid="storage-page">
      <header className="panel row">
        <div>
          <h1 className="m-0 text-2xl font-bold text-slate-900">Stockage & Débit</h1>
          <p className="m-0 mt-1 text-sm text-slate-600">Capacité des dossiers gérés, espace disponible et benchmark disque local.</p>
        </div>
        <button type="button" className="btn" onClick={() => void loadStats()}>
          Rafraîchir
        </button>
      </header>

      {error ? (
        <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
          {error}
        </p>
      ) : null}

      {stats ? (
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <article className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
            <p className="m-0 text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">Capacité totale</p>
            <p className="m-0 mt-2 text-2xl font-semibold text-slate-900">{formatBytes(stats.totals.totalBytes)}</p>
          </article>
          <article className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
            <p className="m-0 text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">Libre</p>
            <p className="m-0 mt-2 text-2xl font-semibold text-emerald-700">{formatBytes(stats.totals.freeBytes)}</p>
          </article>
          <article className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
            <p className="m-0 text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">Utilisé (FS)</p>
            <p className="m-0 mt-2 text-2xl font-semibold text-slate-900">{usagePercent}%</p>
          </article>
          <article className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
            <p className="m-0 text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">Utilisé connu (indexé)</p>
            <p className="m-0 mt-2 text-2xl font-semibold text-slate-900">{formatBytes(stats.totals.knownUsedBytes)}</p>
          </article>
        </div>
      ) : null}

      <article className="panel stack">
        <div className="row">
          <h2 className="m-0 text-lg font-semibold text-slate-900">Test de débit disque</h2>
          <div className="flex items-center gap-2">
            <label className="text-sm text-slate-600" htmlFor="speed-sample">Échantillon</label>
            <select
              id="speed-sample"
              className="rounded-lg border border-slate-300 bg-white px-2 py-1 text-sm"
              value={sampleMB}
              onChange={(event) => setSampleMB(Number(event.target.value))}
              disabled={speedBusy}
            >
              <option value={4}>4 Mo</option>
              <option value={8}>8 Mo</option>
              <option value={16}>16 Mo</option>
              <option value={32}>32 Mo</option>
            </select>
            <button type="button" className="btn btn-primary" onClick={() => void runSpeedtest()} disabled={speedBusy}>
              {speedBusy ? 'Test en cours...' : 'Lancer le test'}
            </button>
          </div>
        </div>
        {speedError ? (
          <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
            {speedError}
          </p>
        ) : null}
        {speedResult ? (
          <div className="grid gap-3 md:grid-cols-4">
            <article className="rounded-xl border border-slate-200 bg-white px-3 py-2">
              <p className="m-0 text-xs uppercase text-slate-500">Écriture</p>
              <p className="m-0 mt-1 text-lg font-semibold text-slate-900">{speedResult.writeMBps.toFixed(1)} Mo/s</p>
            </article>
            <article className="rounded-xl border border-slate-200 bg-white px-3 py-2">
              <p className="m-0 text-xs uppercase text-slate-500">Lecture</p>
              <p className="m-0 mt-1 text-lg font-semibold text-slate-900">{speedResult.readMBps.toFixed(1)} Mo/s</p>
            </article>
            <article className="rounded-xl border border-slate-200 bg-white px-3 py-2">
              <p className="m-0 text-xs uppercase text-slate-500">Durée</p>
              <p className="m-0 mt-1 text-lg font-semibold text-slate-900">{speedResult.durationMs} ms</p>
            </article>
            <article className="rounded-xl border border-slate-200 bg-white px-3 py-2">
              <p className="m-0 text-xs uppercase text-slate-500">Fichier test</p>
              <p className="m-0 mt-1 truncate text-sm text-slate-700" title={speedResult.path}>{speedResult.path}</p>
            </article>
          </div>
        ) : null}
      </article>

      <article className="panel stack">
        <h2 className="m-0 text-lg font-semibold text-slate-900">Dossiers gérés</h2>
        {!stats || stats.roots.length === 0 ? (
          <p className="m-0 text-sm text-slate-600">Aucun dossier configuré.</p>
        ) : (
          <ul className="m-0 list-none space-y-2 p-0">
            {stats.roots.map((root) => (
              <li key={root.path} className="rounded-xl border border-slate-200 bg-slate-50 px-3 py-2">
                <p className="m-0 text-sm font-semibold text-slate-800">{root.path}</p>
                <p className="m-0 mt-1 text-xs text-slate-600">
                  {root.exists ? 'Disponible' : 'Introuvable'} • Total {formatBytes(root.totalBytes)} • Libre {formatBytes(root.freeBytes)} • Connu {formatBytes(root.knownUsedBytes)}
                </p>
              </li>
            ))}
          </ul>
        )}
      </article>
    </section>
  )
}
