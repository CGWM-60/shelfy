import { FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import { useAPI } from '../api/context'
import type { DebridAccount, DebridProvider } from '../api/types'

type AuthSession = { sessionId: string; userCode?: string; verificationUri?: string; intervalSec?: number; expiresIn?: number }

function renderAuthType(authType: string) {
  if (authType === 'oauth2_device') return 'Code à valider'
  if (authType === 'oauth2_password') return 'Login / mot de passe'
  if (authType === 'api_key') return 'Clé API'
  return 'Sans authentification'
}

export function DebridPage() {
  const api = useAPI()
  const [providers, setProviders] = useState<DebridProvider[]>([])
  const [accounts, setAccounts] = useState<DebridAccount[]>([])
  const [provider, setProvider] = useState('')
  const [label, setLabel] = useState('Compte principal')
  const [authSession, setAuthSession] = useState<Record<number, AuthSession>>({})
  const [statusMessage, setStatusMessage] = useState('')
  const [errorMessage, setErrorMessage] = useState('')
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)
  const [busyAccountID, setBusyAccountID] = useState<number | null>(null)
  const [pollingAccountIDs, setPollingAccountIDs] = useState<number[]>([])
  const [credentials, setCredentials] = useState<Record<number, { username: string; password: string }>>({})
  const pollingRef = useRef<Record<number, boolean>>({})

  const selectedProvider = useMemo(() => providers.find((row) => row.name === provider), [providers, provider])
  const providersByName = useMemo(() => new Map(providers.map((row) => [row.name, row])), [providers])

  async function refresh() {
    setErrorMessage('')
    try {
      const [providerRows, accountRows] = await Promise.all([api.listDebridProviders(), api.listDebridAccounts()])
      setProviders(providerRows)
      setAccounts(accountRows)
      setProvider((current) => {
        if (providerRows.length === 0) return ''
        if (current && providerRows.some((row) => row.name === current)) return current
        return providerRows[0].name
      })
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Impossible de charger les débrideurs')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void refresh()
  }, [])

  async function submit(e: FormEvent) {
    e.preventDefault()
    if (!provider) {
      setErrorMessage('Aucun provider disponible')
      return
    }
    setCreating(true)
    setErrorMessage('')
    try {
      await api.createDebridAccount({ provider, label, isActive: true, isDefault: accounts.length === 0 })
      setLabel('')
      setStatusMessage('Compte ajouté')
      await refresh()
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : "Impossible d'ajouter le compte")
    } finally {
      setCreating(false)
    }
  }

  async function toggleAccount(account: DebridAccount) {
    setBusyAccountID(account.id)
    try {
      await api.patchDebridAccount(account.id, { isActive: !account.isActive })
      setStatusMessage(account.isActive ? 'Compte désactivé' : 'Compte activé')
      await refresh()
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Impossible de modifier le compte')
    } finally {
      setBusyAccountID(null)
    }
  }

  async function setDefaultAccount(account: DebridAccount) {
    setBusyAccountID(account.id)
    try {
      await api.patchDebridAccount(account.id, { isDefault: true })
      setStatusMessage(`Compte par défaut: ${account.label}`)
      await refresh()
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Impossible de définir le compte par défaut')
    } finally {
      setBusyAccountID(null)
    }
  }

  async function deleteAccount(account: DebridAccount) {
    if (!window.confirm(`Supprimer le compte "${account.label}" ?`)) return
    setBusyAccountID(account.id)
    try {
      await api.deleteDebridAccount(account.id)
      setStatusMessage('Compte supprimé')
      setAuthSession((prev) => {
        const next = { ...prev }
        delete next[account.id]
        return next
      })
      await refresh()
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Impossible de supprimer le compte')
    } finally {
      setBusyAccountID(null)
    }
  }

  async function startAuth(account: DebridAccount) {
    setBusyAccountID(account.id)
    try {
      const session = await api.startDebridAuth(account.id)
      setAuthSession((prev) => ({ ...prev, [account.id]: session }))
      setStatusMessage(`Code reçu pour ${account.label}. Valide sur le site Debrid-Link, la connexion sera détectée automatiquement.`)
      void pollUntilDone(account, session)
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : "Impossible de lancer l'authentification")
    } finally {
      setBusyAccountID(null)
    }
  }

  async function pollUntilDone(account: DebridAccount, session: AuthSession) {
    if (pollingRef.current[account.id]) return
    pollingRef.current[account.id] = true
    setPollingAccountIDs((prev) => (prev.includes(account.id) ? prev : [...prev, account.id]))

    const intervalMs = Math.max(1, session.intervalSec ?? 2) * 1000
    const expiresInSec = Math.max(30, session.expiresIn ?? 600)
    const deadline = Date.now() + expiresInSec*1000

    try {
      while (Date.now() < deadline) {
        await new Promise((resolve) => window.setTimeout(resolve, intervalMs))
        const polled = await api.pollDebridAuth(account.id, session)
        if (polled.done) {
          setStatusMessage('Connexion validée')
          await refresh()
          return
        }
      }
      setStatusMessage("Session d'appairage expirée, relance la connexion")
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Erreur de vérification de connexion')
    } finally {
      pollingRef.current[account.id] = false
      setPollingAccountIDs((prev) => prev.filter((id) => id !== account.id))
    }
  }

  async function pollAuth(account: DebridAccount) {
    const session = authSession[account.id]
    if (!session) return
    setBusyAccountID(account.id)
    try {
      const polled = await api.pollDebridAuth(account.id, session)
      setStatusMessage(polled.done ? 'Connexion validée' : 'Validation en attente')
      if (polled.done) {
        await refresh()
      }
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Impossible de valider la connexion')
    } finally {
      setBusyAccountID(null)
    }
  }

  async function checkStatus(account: DebridAccount) {
    setBusyAccountID(account.id)
    try {
      const status = await api.getDebridStatus(account.id)
      setStatusMessage(`Statut: ${status.status}`)
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Impossible de vérifier le statut')
    } finally {
      setBusyAccountID(null)
    }
  }

  async function authPassword(account: DebridAccount) {
    const input = credentials[account.id] ?? { username: '', password: '' }
    if (!input.username || !input.password) {
      setErrorMessage('Renseigne login et mot de passe pour ce compte')
      return
    }
    setBusyAccountID(account.id)
    try {
      const result = await api.passwordDebridAuth(account.id, { username: input.username, password: input.password })
      setStatusMessage(result.done ? 'Connexion login/mot de passe validée' : 'Connexion en attente')
      setCredentials((prev) => ({ ...prev, [account.id]: { username: input.username, password: '' } }))
      await refresh()
    } catch (err) {
      setErrorMessage(err instanceof Error ? err.message : 'Impossible de se connecter avec login/mot de passe')
    } finally {
      setBusyAccountID(null)
    }
  }

  return (
    <section className="panel stack" data-testid="debrid-page">
      <div className="row">
        <div>
          <h1 className="m-0 text-2xl font-bold text-slate-900">Débrideurs</h1>
          <p className="m-0 mt-1 text-sm text-slate-600">Gère tes comptes Debrid-Link et autres providers compatibles.</p>
        </div>
      </div>

      {statusMessage ? <p role="status" className="rounded-xl border border-emerald-200 bg-emerald-50 px-3 py-2 text-sm text-emerald-700">{statusMessage}</p> : null}
      {errorMessage ? <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{errorMessage}</p> : null}

      <form onSubmit={submit} className="grid gap-3 rounded-2xl border border-slate-200 bg-slate-50/70 p-4 md:grid-cols-[1fr_2fr_auto]">
        <select className="field" value={provider} onChange={(e) => setProvider(e.target.value)} disabled={loading || providers.length === 0}>
          {providers.length === 0 ? <option value="">Aucun provider détecté</option> : null}
          {providers.map((row) => (
            <option value={row.name} key={row.name}>{row.name}</option>
          ))}
        </select>
        <input className="field" value={label} onChange={(e) => setLabel(e.target.value)} placeholder="Label" required />
        <button className="btn btn-primary" type="submit" disabled={creating || !provider}>
          {creating ? 'Ajout...' : 'Ajouter compte'}
        </button>
      </form>

      {selectedProvider ? (
        <p className="m-0 text-xs text-slate-500">
          Provider sélectionné: <strong>{selectedProvider.name}</strong> • Auth: <strong>{renderAuthType(selectedProvider.authType)}</strong>
        </p>
      ) : null}

      {loading ? <p className="m-0 text-sm text-slate-500">Chargement des comptes...</p> : null}
      {!loading && accounts.length === 0 ? (
        <div className="rounded-2xl border border-dashed border-slate-300 bg-white p-6 text-center text-sm text-slate-600">
          Aucun compte configuré pour le moment.
        </div>
      ) : null}

      <ul className="list">
        {accounts.map((account) => (
          <li key={account.id} className="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm">
            {(() => {
              const providerInfo = providersByName.get(account.provider)
              const accountInput = credentials[account.id] ?? { username: '', password: '' }
              const supportsPassword = providerInfo?.supportsPassword === true
              return (
                <>
            <div className="row">
              <div>
                <div className="flex items-center gap-2">
                  <strong className="text-base text-slate-900">{account.label}</strong>
                  {account.isDefault ? <span className="badge bg-brand-100 text-brand-700">Par défaut</span> : null}
                  {!account.isActive ? <span className="badge bg-amber-100 text-amber-700">Inactif</span> : null}
                </div>
                <small className="text-slate-500">{account.provider}</small>
                {pollingAccountIDs.includes(account.id) ? <small className="block text-xs text-brand-600">Connexion en attente de validation...</small> : null}
              </div>
            </div>

            {authSession[account.id] ? (
              <div className="mt-3 rounded-xl border border-brand-200 bg-brand-50 p-3 text-sm text-brand-900">
                <p className="m-0">Code: <strong>{authSession[account.id].userCode || '-'}</strong></p>
                {authSession[account.id].verificationUri ? (
                  <a className="mt-2 inline-block text-brand-700 underline hover:text-brand-800" href={authSession[account.id].verificationUri} target="_blank" rel="noreferrer">
                    Ouvrir page de validation
                  </a>
                ) : null}
              </div>
            ) : null}

            <div className="mt-4 flex flex-wrap gap-2">
              <button className="btn" disabled={busyAccountID === account.id} onClick={() => void toggleAccount(account)}>
                {account.isActive ? 'Désactiver' : 'Activer'}
              </button>
              <button className="btn" disabled={busyAccountID === account.id} onClick={() => void setDefaultAccount(account)}>
                Définir par défaut
              </button>
              <button className="btn" disabled={busyAccountID === account.id} onClick={() => void startAuth(account)}>
                Connecter Debrid-Link
              </button>
              <button className="btn btn-primary" disabled={!authSession[account.id] || busyAccountID === account.id} onClick={() => void pollAuth(account)}>
                J&apos;ai validé
              </button>
              <button className="btn" disabled={busyAccountID === account.id} onClick={() => void checkStatus(account)}>
                Vérifier statut
              </button>
              <button className="btn btn-danger" disabled={busyAccountID === account.id} onClick={() => void deleteAccount(account)}>
                Supprimer compte
              </button>
            </div>

            {supportsPassword ? (
              <div className="mt-3 grid gap-2 rounded-xl border border-slate-200 bg-slate-50 p-3 md:grid-cols-[1fr_1fr_auto]">
                <input
                  className="field"
                  placeholder="Login Debrid-Link"
                  value={accountInput.username}
                  onChange={(e) => setCredentials((prev) => ({ ...prev, [account.id]: { ...accountInput, username: e.target.value } }))}
                />
                <input
                  className="field"
                  type="password"
                  placeholder="Mot de passe Debrid-Link"
                  value={accountInput.password}
                  onChange={(e) => setCredentials((prev) => ({ ...prev, [account.id]: { ...accountInput, password: e.target.value } }))}
                />
                <button className="btn" disabled={busyAccountID === account.id} onClick={() => void authPassword(account)}>
                  Connexion login/mot de passe
                </button>
              </div>
            ) : null}
                </>
              )
            })()}
          </li>
        ))}
      </ul>
    </section>
  )
}
