export type DownloadStatus = 'queued' | 'running' | 'paused' | 'completed' | 'failed' | 'canceled'

export interface DownloadJob {
  id: string
  sourceLink: string
  directLink: string
  fileName: string
  status: DownloadStatus
  downloadedBytes: number
  sizeBytes: number
  speedBytes: number
  etaSeconds: number
  useDebrid: boolean
  errorMessage?: string
  retries?: number
  maxRetries?: number
  nextRetryInMs?: number
}

export interface DebridProvider {
  name: string
  authType: string
  supportsPassword?: boolean
}

export interface DebridAccount {
  id: number
  provider: string
  label: string
  isActive: boolean
  isDefault: boolean
  authType: string
}

export interface MediaItem {
  id: string
  title: string
  kind: 'video' | 'audio' | 'image' | 'pdf' | 'other'
  path: string
  mimeType?: string
  sizeBytes?: number
  seriesName?: string
  season?: number
  episode?: number
}

export interface AIResult {
  mediaId: string
  snippet: string
  title: string
  score: number
}

export interface AIAskResponse {
  answer: string
  sources: AIResult[]
}

export interface AuthStatus {
  enabled: boolean
  authenticated: boolean
  username?: string
}

export interface AIReport {
  embeddingProvider: string
  llmProvider: string
  startedAt: string
  uptimeSec: number
  lastError?: string
  lastErrorAt?: string
  indexRuns: number
  lastIndexedCount: number
  indexedTotal: number
  lastIndexedAt?: string
  searchRuns: number
  searchAvgLatencyMs: number
  lastSearchLatencyMs: number
  lastSearchResults: number
  askRuns: number
  askSearchOnlyRuns: number
  askAvgLatencyMs: number
  lastAskLatencyMs: number
  lastAskSources: number
  embeddingCalls: number
  embeddingTokensEstimated: number
  llmPromptTokensEstimated: number
  llmCompletionTokensEstimated: number
}

export interface AppSettings {
  downloadMaxConcurrent: number
  downloadAutoResume: boolean
  downloadAutoGroup: boolean
  downloadsPath: string
  libraryPaths: string[]
  globalRateLimitKB: number
  aiEnabled: boolean
  aiTopK: number
  theme: string
  visibleColumns: string[]
  fileServerAuthEnabled: boolean
  fileServerAuthUser: string
  dlnaEnabled: boolean
  smbEnabled: boolean
  smbShareName: string
  smbSharePath: string
}

export interface DLNADevice {
  usn: string
  st: string
  server: string
  location: string
  address: string
  lastSeenAt: string
}

export interface DLNAStatus {
  enabled: boolean
  devices: DLNADevice[]
  lastScan: string
  lastError?: string
}

export interface SMBClient {
  username: string
  machine: string
  address: string
  connectedAt: string
}

export interface SMBStatus {
  enabled: boolean
  running: boolean
  backend: string
  shareName: string
  sharePath: string
  lastError?: string
  updatedAt: string
  clients: SMBClient[]
}

export interface FSManagedEntry {
  name: string
  path: string
  isDir: boolean
  sizeBytes: number
  modifiedAt: string
}

export interface FSPathListing {
  path: string
  items: FSManagedEntry[]
}

export interface StorageRootStat {
  path: string
  exists: boolean
  totalBytes: number
  freeBytes: number
  availableBytes: number
  knownUsedBytes: number
}

export interface StorageStats {
  generatedAt: string
  roots: StorageRootStat[]
  totals: {
    totalBytes: number
    freeBytes: number
    availableBytes: number
    knownUsedBytes: number
  }
}

export interface StorageSpeedtestResult {
  path: string
  sampleMB: number
  writeMBps: number
  readMBps: number
  durationMs: number
}
