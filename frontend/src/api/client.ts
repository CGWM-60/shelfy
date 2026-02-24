import type { AIAskResponse, AIReport, AIResult, AppSettings, AuthStatus, DebridAccount, DebridProvider, DLNAStatus, DownloadJob, FSPathListing, MediaItem, SMBStatus, StorageSpeedtestResult, StorageStats } from './types'

export class AuthError extends Error {
  status: number

  constructor(message = 'unauthorized') {
    super(message)
    this.name = 'AuthError'
    this.status = 401
  }
}

export interface APIClient {
  authStatus(): Promise<AuthStatus>
  authLogin(payload: { username: string; password: string }): Promise<{ authenticated: boolean; username?: string }>
  authLogout(): Promise<{ done: boolean }>

  listDownloads(): Promise<DownloadJob[]>
  addDownloads(payload: { links: string[]; useDebrid: boolean; provider?: string; accountId?: number; debridAccountId?: number; debridPassword?: string; destinationDir?: string; maxParallel?: number }): Promise<DownloadJob[]>
  startDownload(id: string): Promise<void>
  pauseDownload(id: string): Promise<void>
  resumeDownload(id: string): Promise<void>
  deleteDownload(id: string): Promise<void>

  listDebridProviders(): Promise<DebridProvider[]>
  listDebridAccounts(): Promise<DebridAccount[]>
  createDebridAccount(payload: { provider: string; label: string; apiKey?: string; isActive: boolean; isDefault: boolean }): Promise<DebridAccount>
  patchDebridAccount(id: number, payload: { label?: string; isActive?: boolean; isDefault?: boolean }): Promise<DebridAccount>
  deleteDebridAccount(id: number): Promise<void>
  startDebridAuth(id: number): Promise<{ sessionId: string; deviceCode?: string; userCode?: string; verificationUri?: string; intervalSec?: number; expiresIn?: number }>
  pollDebridAuth(id: number, payload: { sessionId: string; deviceCode?: string; userCode?: string }): Promise<{ done: boolean }>
  passwordDebridAuth(id: number, payload: { username: string; password: string }): Promise<{ done: boolean }>
  getDebridStatus(id: number): Promise<{ status: string }>

  scanMedia(path?: string): Promise<{ count: number }>
  listMedia(query?: string, kind?: string): Promise<MediaItem[]>
  getMedia(id: string): Promise<{ item: MediaItem; progress?: { mediaId: string; positionMs: number } }>
  saveMediaProgress(id: string, positionMs: number): Promise<void>

  getSettings(): Promise<AppSettings>
  saveSettings(payload: AppSettings): Promise<AppSettings>
  getDLNADevices(): Promise<DLNAStatus>
  scanDLNADevices(): Promise<DLNAStatus>
  getSMBStatus(): Promise<SMBStatus>
  refreshSMBStatus(): Promise<SMBStatus>
  getStorageStats(): Promise<StorageStats>
  runStorageSpeedtest(sizeMB: number): Promise<StorageSpeedtestResult>

  getFSRoots(): Promise<{ roots: string[] }>
  listFS(path?: string): Promise<FSPathListing>
  readFSFile(path: string): Promise<{ path: string; content: string }>
  createFSDir(path: string): Promise<void>
  writeFSFile(path: string, content: string): Promise<void>
  moveFSPath(fromPath: string, toPath: string): Promise<void>
  deleteFSPath(path: string, recursive: boolean): Promise<void>

  aiStatus(): Promise<Record<string, string>>
  aiReport(): Promise<AIReport>
  aiIndex(): Promise<{ indexed: number }>
  aiSearch(query: string, limit?: number): Promise<AIResult[]>
  aiAsk(question: string, searchOnly: boolean): Promise<AIAskResponse>
}

const baseURL = import.meta.env.VITE_API_BASE ?? ''

async function req<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${baseURL}${path}`, {
    credentials: 'include',
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...(init?.headers ?? {})
    }
  })
  if (res.status === 401) {
    throw new AuthError()
  }
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `HTTP ${res.status}`)
  }
  if (res.status === 204) {
    return undefined as T
  }
  const body = await res.json()
  return body.data as T
}

export const apiClient: APIClient = {
  authStatus: () => req('/api/auth/status'),
  authLogin: (payload) => req('/api/auth/login', { method: 'POST', body: JSON.stringify(payload) }),
  authLogout: () => req('/api/auth/logout', { method: 'POST' }),

  listDownloads: () => req('/api/downloads'),
  addDownloads: (payload) => req('/api/downloads', { method: 'POST', body: JSON.stringify(payload) }),
  startDownload: (id) => req(`/api/downloads/${id}/start`, { method: 'POST' }),
  pauseDownload: (id) => req(`/api/downloads/${id}/pause`, { method: 'POST' }),
  resumeDownload: (id) => req(`/api/downloads/${id}/resume`, { method: 'POST' }),
  deleteDownload: (id) => req(`/api/downloads/${id}`, { method: 'DELETE' }),

  listDebridProviders: () => req('/api/debrid/providers'),
  listDebridAccounts: () => req('/api/debrid/accounts'),
  createDebridAccount: (payload) => req('/api/debrid/accounts', { method: 'POST', body: JSON.stringify(payload) }),
  patchDebridAccount: (id, payload) => req(`/api/debrid/accounts/${id}`, { method: 'PATCH', body: JSON.stringify(payload) }),
  deleteDebridAccount: (id) => req(`/api/debrid/accounts/${id}`, { method: 'DELETE' }),
  startDebridAuth: (id) => req(`/api/debrid/accounts/${id}/auth/start`, { method: 'POST' }),
  pollDebridAuth: (id, payload) => req(`/api/debrid/accounts/${id}/auth/poll`, { method: 'POST', body: JSON.stringify(payload) }),
  passwordDebridAuth: (id, payload) => req(`/api/debrid/accounts/${id}/auth/password`, { method: 'POST', body: JSON.stringify(payload) }),
  getDebridStatus: (id) => req(`/api/debrid/accounts/${id}/status`),

  scanMedia: (path) => req('/api/media/scan', { method: 'POST', body: JSON.stringify({ path }) }),
  listMedia: (query = '', kind = '') => {
    const params = new URLSearchParams()
    if (query) params.set('q', query)
    if (kind) params.set('kind', kind)
    const suffix = params.toString() ? `?${params.toString()}` : ''
    return req(`/api/media${suffix}`)
  },
  getMedia: (id) => req(`/api/media/${id}`),
  saveMediaProgress: (id, positionMs) => req(`/api/media/${id}/progress`, { method: 'POST', body: JSON.stringify({ positionMs }) }),

  getSettings: () => req('/api/settings'),
  saveSettings: (payload) => req('/api/settings', { method: 'PUT', body: JSON.stringify(payload) }),
  getDLNADevices: () => req('/api/dlna/devices'),
  scanDLNADevices: () => req('/api/dlna/scan', { method: 'POST' }),
  getSMBStatus: () => req('/api/smb/status'),
  refreshSMBStatus: () => req('/api/smb/refresh', { method: 'POST' }),
  getStorageStats: () => req('/api/system/storage'),
  runStorageSpeedtest: (sizeMB) => req('/api/system/speedtest', { method: 'POST', body: JSON.stringify({ sizeMB }) }),

  getFSRoots: () => req('/api/fs/roots'),
  listFS: (path) => {
    const params = new URLSearchParams()
    if (path) params.set('path', path)
    const suffix = params.toString() ? `?${params.toString()}` : ''
    return req(`/api/fs/list${suffix}`)
  },
  readFSFile: (path) => {
    const params = new URLSearchParams()
    params.set('path', path)
    return req(`/api/fs/file?${params.toString()}`)
  },
  createFSDir: (path) => req('/api/fs/mkdir', { method: 'POST', body: JSON.stringify({ path }) }),
  writeFSFile: (path, content) => req('/api/fs/file', { method: 'PUT', body: JSON.stringify({ path, content }) }),
  moveFSPath: (fromPath, toPath) => req('/api/fs/move', { method: 'PATCH', body: JSON.stringify({ fromPath, toPath }) }),
  deleteFSPath: (path, recursive) => {
    const params = new URLSearchParams()
    params.set('path', path)
    params.set('recursive', String(recursive))
    return req(`/api/fs/item?${params.toString()}`, { method: 'DELETE' })
  },

  aiStatus: () => req('/api/ai/status'),
  aiReport: () => req('/api/ai/report'),
  aiIndex: () => req('/api/ai/index', { method: 'POST' }),
  aiSearch: (query, limit = 5) => req('/api/ai/search', { method: 'POST', body: JSON.stringify({ query, limit }) }),
  aiAsk: (question, searchOnly) => req('/api/ai/ask', { method: 'POST', body: JSON.stringify({ question, searchOnly }) })
}
