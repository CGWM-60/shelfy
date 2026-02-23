import { useCallback, useEffect, useMemo, useState } from 'react'
import { useAPI } from '../api/context'
import type { AIReport } from '../api/types'

function formatDate(value?: string) {
  if (!value) return 'N/A'
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return 'N/A'
  return new Intl.DateTimeFormat('fr-FR', { dateStyle: 'medium', timeStyle: 'short' }).format(date)
}

function formatDuration(sec: number) {
  const total = Math.max(0, Math.floor(sec))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  if (h > 0) return `${h}h ${m}m ${s}s`
  if (m > 0) return `${m}m ${s}s`
  return `${s}s`
}

function MetricCard({ title, value, hint }: { title: string; value: string | number; hint?: string }) {
  return (
    <article className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
      <p className="m-0 text-xs font-semibold uppercase tracking-[0.16em] text-slate-500">{title}</p>
      <p className="m-0 mt-2 text-2xl font-semibold text-slate-900">{value}</p>
      {hint ? <p className="m-0 mt-1 text-xs text-slate-500">{hint}</p> : null}
    </article>
  )
}

export function AIReportPage() {
  const api = useAPI()
  const [report, setReport] = useState<AIReport | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [action, setAction] = useState('')

  const load = useCallback(async () => {
    setError('')
    try {
      const data = await api.aiReport()
      setReport(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Impossible de charger le rapport IA')
    } finally {
      setLoading(false)
    }
  }, [api])

  useEffect(() => {
    void load()
  }, [load])

  const completionRatio = useMemo(() => {
    if (!report) return '0%'
    const total = report.llmPromptTokensEstimated + report.llmCompletionTokensEstimated
    if (total <= 0) return '0%'
    return `${Math.round((report.llmCompletionTokensEstimated / total) * 100)}%`
  }, [report])

  async function runIndex() {
    setAction('')
    setError('')
    try {
      const result = await api.aiIndex()
      setAction(`${result.indexed} élément(s) indexé(s)`)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Échec de l'indexation IA")
    }
  }

  if (loading) {
    return (
      <section className="panel" data-testid="ai-report-page">
        <p className="m-0 text-sm text-slate-600">Chargement du rapport IA...</p>
      </section>
    )
  }

  if (!report) {
    return (
      <section className="panel stack" data-testid="ai-report-page">
        <h1 className="m-0 text-2xl font-bold text-slate-900">Rapport IA</h1>
        <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">
          {error || 'Rapport indisponible'}
        </p>
      </section>
    )
  }

  return (
    <section className="stack" data-testid="ai-report-page">
      <header className="panel row">
        <div>
          <h1 className="m-0 text-2xl font-bold text-slate-900">Rapport IA</h1>
          <p className="m-0 mt-1 text-sm text-slate-600">Suivi embeddings, latence, indexation et consommation tokens estimée.</p>
        </div>
        <div className="flex flex-wrap gap-2">
          <button type="button" className="btn" onClick={() => void load()}>Rafraîchir</button>
          <button type="button" className="btn btn-primary" onClick={() => void runIndex()}>Indexer maintenant</button>
        </div>
      </header>

      {action ? <p role="status" className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{action}</p> : null}
      {error ? <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p> : null}

      <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
        <MetricCard title="Provider embeddings" value={report.embeddingProvider} />
        <MetricCard title="Provider LLM" value={report.llmProvider} />
        <MetricCard title="Uptime" value={formatDuration(report.uptimeSec)} hint={`Depuis ${formatDate(report.startedAt)}`} />
        <MetricCard title="Dernière erreur" value={report.lastError ? 'Oui' : 'Aucune'} hint={report.lastError ? formatDate(report.lastErrorAt) : 'Santé OK'} />
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <article className="panel stack">
          <h2 className="m-0 text-lg font-semibold text-slate-900">Indexation</h2>
          <div className="grid gap-3 sm:grid-cols-3">
            <MetricCard title="Runs" value={report.indexRuns} />
            <MetricCard title="Dernier batch" value={report.lastIndexedCount} />
            <MetricCard title="Total indexé" value={report.indexedTotal} hint={formatDate(report.lastIndexedAt)} />
          </div>
        </article>

        <article className="panel stack">
          <h2 className="m-0 text-lg font-semibold text-slate-900">Recherche sémantique</h2>
          <div className="grid gap-3 sm:grid-cols-3">
            <MetricCard title="Requêtes" value={report.searchRuns} />
            <MetricCard title="Latence moyenne" value={`${report.searchAvgLatencyMs.toFixed(1)} ms`} />
            <MetricCard title="Dernier résultat" value={report.lastSearchResults} hint={`Dernier appel: ${report.lastSearchLatencyMs} ms`} />
          </div>
        </article>
      </div>

      <div className="grid gap-4 xl:grid-cols-2">
        <article className="panel stack">
          <h2 className="m-0 text-lg font-semibold text-slate-900">Q/A</h2>
          <div className="grid gap-3 sm:grid-cols-4">
            <MetricCard title="Questions" value={report.askRuns} />
            <MetricCard title="Search only" value={report.askSearchOnlyRuns} />
            <MetricCard title="Latence moyenne" value={`${report.askAvgLatencyMs.toFixed(1)} ms`} />
            <MetricCard title="Dernières sources" value={report.lastAskSources} hint={`Dernier appel: ${report.lastAskLatencyMs} ms`} />
          </div>
        </article>

        <article className="panel stack">
          <h2 className="m-0 text-lg font-semibold text-slate-900">Tokens estimés</h2>
          <div className="grid gap-3 sm:grid-cols-2">
            <MetricCard title="Calls embeddings" value={report.embeddingCalls} />
            <MetricCard title="Tokens embeddings" value={report.embeddingTokensEstimated} />
            <MetricCard title="Tokens prompt LLM" value={report.llmPromptTokensEstimated} />
            <MetricCard title="Tokens réponse LLM" value={report.llmCompletionTokensEstimated} hint={`Part réponse: ${completionRatio}`} />
          </div>
        </article>
      </div>

      {report.lastError ? (
        <article className="rounded-2xl border border-amber-200 bg-amber-50 px-4 py-3">
          <p className="m-0 text-sm font-semibold text-amber-800">Dernière erreur IA</p>
          <p className="m-0 mt-1 text-sm text-amber-700">{report.lastError}</p>
        </article>
      ) : null}
    </section>
  )
}
