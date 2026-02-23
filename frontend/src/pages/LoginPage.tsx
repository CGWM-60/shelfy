import { FormEvent, useState } from 'react'
import { useAPI } from '../api/context'

interface LoginPageProps {
  onLoggedIn: () => void
}

export function LoginPage({ onLoggedIn }: LoginPageProps) {
  const api = useAPI()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    setError('')
    setLoading(true)
    try {
      await api.authLogin({ username, password })
      onLoggedIn()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Impossible de se connecter')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="mx-auto flex min-h-[80vh] w-full max-w-5xl items-center justify-center px-4">
      <section className="grid w-full overflow-hidden rounded-3xl border border-slate-200 bg-white shadow-2xl md:grid-cols-[1.2fr_1fr]">
        <div className="relative hidden min-h-[460px] overflow-hidden bg-slate-950 p-10 text-white md:block">
          <div className="absolute inset-0 bg-[radial-gradient(circle_at_20%_20%,rgba(59,130,246,0.35),transparent_45%),radial-gradient(circle_at_80%_80%,rgba(14,165,233,0.25),transparent_50%)]" />
          <div className="relative z-10">
            <p className="mb-2 text-xs font-semibold uppercase tracking-[0.25em] text-slate-300">Shelfy Secure</p>
            <h1 className="m-0 text-4xl font-bold leading-tight">Connexion requise</h1>
            <p className="mt-4 text-sm text-slate-300">
              Accès protégé à la gestion des téléchargements, au streaming et aux intégrations débrideur + IA.
            </p>
          </div>
        </div>
        <div className="p-8 md:p-10">
          <h2 className="m-0 text-2xl font-bold text-slate-900">Se connecter</h2>
          <p className="mt-2 text-sm text-slate-600">Entre tes identifiants administrateur pour continuer.</p>

          <form className="mt-6 space-y-4" onSubmit={(event) => void submit(event)}>
            <label className="block text-sm font-semibold text-slate-700" htmlFor="login-username">
              Identifiant
            </label>
            <input
              id="login-username"
              className="w-full rounded-xl border border-slate-300 px-3 py-2 text-slate-900 focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-200"
              value={username}
              onChange={(event) => setUsername(event.target.value)}
              autoComplete="username"
              required
            />
            <label className="block text-sm font-semibold text-slate-700" htmlFor="login-password">
              Mot de passe
            </label>
            <input
              id="login-password"
              type="password"
              className="w-full rounded-xl border border-slate-300 px-3 py-2 text-slate-900 focus:border-brand-500 focus:outline-none focus:ring-2 focus:ring-brand-200"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              autoComplete="current-password"
              required
            />
            <button className="btn btn-primary w-full justify-center" type="submit" disabled={loading}>
              {loading ? 'Connexion...' : 'Connexion'}
            </button>
          </form>
          {error ? (
            <p className="mt-4 rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700" role="alert">
              {error}
            </p>
          ) : null}
        </div>
      </section>
    </div>
  )
}
