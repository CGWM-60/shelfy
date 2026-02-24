import { useEffect, useMemo, useState } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { useAPI } from './api/context'
import { AuthError } from './api/client'
import { Nav } from './components/Nav'
import { DownloadsPage } from './pages/DownloadsPage'
import { LibraryPage } from './pages/LibraryPage'
import { DebridPage } from './pages/DebridPage'
import { MediaDetailPage } from './pages/MediaDetailPage'
import { SettingsPage } from './pages/SettingsPage'
import { AIReportPage } from './pages/AIReportPage'
import { LoginPage } from './pages/LoginPage'
import { StoragePage } from './pages/StoragePage'
import { MiniPlayer } from './components/MiniPlayer'
import { AskAIOverlay } from './components/AskAIOverlay'
import { applyTheme } from './theme'

type AuthState = {
  loading: boolean
  enabled: boolean
  authenticated: boolean
  username: string
}

const initialAuthState: AuthState = {
  loading: true,
  enabled: false,
  authenticated: true,
  username: ''
}

export default function App() {
  const api = useAPI()
  const [auth, setAuth] = useState<AuthState>(initialAuthState)

  async function refreshAuth() {
    try {
      const status = await api.authStatus()
      setAuth({
        loading: false,
        enabled: Boolean(status.enabled),
        authenticated: Boolean(status.authenticated),
        username: status.username || ''
      })
    } catch {
      setAuth({
        loading: false,
        enabled: false,
        authenticated: true,
        username: ''
      })
    }
  }

  useEffect(() => {
    void refreshAuth()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  useEffect(() => {
    if (auth.loading) return
    if (auth.enabled && !auth.authenticated) {
      applyTheme('clair')
      return
    }
    api.getSettings()
      .then((settings) => {
        applyTheme(settings.theme || 'clair')
      })
      .catch((err) => {
        if (err instanceof AuthError) {
          setAuth((prev) => ({ ...prev, authenticated: false }))
          return
        }
        applyTheme('clair')
      })
  }, [api, auth.enabled, auth.authenticated, auth.loading])

  const locked = useMemo(() => auth.enabled && !auth.authenticated, [auth.enabled, auth.authenticated])

  async function logout() {
    try {
      await api.authLogout()
    } finally {
      await refreshAuth()
    }
  }

  if (auth.loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <p className="text-sm text-slate-600">Initialisation...</p>
      </div>
    )
  }

  if (locked) {
    return (
      <Routes>
        <Route path="/login" element={<LoginPage onLoggedIn={() => void refreshAuth()} />} />
        <Route path="*" element={<Navigate to="/login" replace />} />
      </Routes>
    )
  }

  return (
    <div className="min-h-screen">
      <div className="mx-auto flex w-full max-w-7xl flex-col gap-6 px-4 py-5 md:px-8 md:py-8">
        <header className="panel">
          <div className="row">
            <div>
              <p className="mb-1 text-xs font-semibold uppercase tracking-[0.18em] text-brand-700">Centre multimédia</p>
              <h1 className="m-0 text-3xl font-bold tracking-tight text-slate-900">Shelfy v3</h1>
              <p className="m-0 mt-1 text-sm text-slate-600">Téléchargements, bibliothèque, débrideurs et IA dans une interface unifiée.</p>
            </div>
            {auth.enabled ? (
              <div className="flex items-center gap-2">
                <span className="rounded-full border border-slate-200 bg-slate-50 px-3 py-1 text-xs font-semibold text-slate-700">{auth.username || 'admin'}</span>
                <button className="btn" onClick={() => void refreshAuth()}>
                  Vérifier connexion
                </button>
                <button className="btn" onClick={() => void logout()}>
                  Déconnexion
                </button>
              </div>
            ) : (
              <div className="rounded-full border border-amber-200 bg-amber-50 px-3 py-1 text-xs font-semibold text-amber-800">
                Connexion désactivée (AUTH_ENABLED=false)
              </div>
            )}
          </div>
          <div className="mt-4">
            <Nav />
          </div>
        </header>
        <main className="pb-28">
          <Routes>
            <Route path="/" element={<DownloadsPage />} />
            <Route path="/library" element={<LibraryPage />} />
            <Route path="/storage" element={<StoragePage />} />
            <Route path="/media/:id" element={<MediaDetailPage />} />
            <Route path="/debrid" element={<DebridPage />} />
            <Route path="/ai/report" element={<AIReportPage />} />
            <Route path="/settings" element={<SettingsPage />} />
            <Route path="/login" element={<Navigate to="/" replace />} />
          </Routes>
        </main>
      </div>
      <AskAIOverlay />
      <MiniPlayer />
    </div>
  )
}
