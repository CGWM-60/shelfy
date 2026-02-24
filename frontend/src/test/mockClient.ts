import { vi } from 'vitest'
import type { APIClient } from '../api/client'

export function createMockClient(): APIClient {
  return {
    authStatus: vi.fn().mockResolvedValue({ enabled: false, authenticated: true, username: '' }),
    authLogin: vi.fn().mockResolvedValue({ authenticated: true, username: 'admin' }),
    authLogout: vi.fn().mockResolvedValue({ done: true }),

    listDownloads: vi.fn().mockResolvedValue([]),
    addDownloads: vi.fn().mockResolvedValue([]),
    startDownload: vi.fn().mockResolvedValue(undefined),
    pauseDownload: vi.fn().mockResolvedValue(undefined),
    resumeDownload: vi.fn().mockResolvedValue(undefined),
    deleteDownload: vi.fn().mockResolvedValue(undefined),

    listDebridProviders: vi.fn().mockResolvedValue([]),
    listDebridAccounts: vi.fn().mockResolvedValue([]),
    createDebridAccount: vi.fn().mockResolvedValue({ id: 1, provider: 'fake', label: 'Fake', isActive: true, isDefault: true, authType: 'none' }),
    patchDebridAccount: vi.fn().mockResolvedValue({ id: 1, provider: 'fake', label: 'Fake', isActive: true, isDefault: true, authType: 'none' }),
    deleteDebridAccount: vi.fn().mockResolvedValue(undefined),
    startDebridAuth: vi.fn().mockResolvedValue({ sessionId: 'sess', deviceCode: 'dev', userCode: 'ABCD-1234', verificationUri: 'https://fake.local/device', intervalSec: 1 }),
    pollDebridAuth: vi.fn().mockResolvedValue({ done: true }),
    passwordDebridAuth: vi.fn().mockResolvedValue({ done: true }),
    getDebridStatus: vi.fn().mockResolvedValue({ status: 'ok' }),

    scanMedia: vi.fn().mockResolvedValue({ count: 1 }),
    listMedia: vi.fn().mockResolvedValue([]),
    getMedia: vi.fn().mockResolvedValue({ item: { id: 'm1', title: 'Film', kind: 'video', path: '/tmp/m.mp4' }, progress: { mediaId: 'm1', positionMs: 1200 } }),
    saveMediaProgress: vi.fn().mockResolvedValue(undefined),

    getSettings: vi.fn().mockResolvedValue({ downloadMaxConcurrent: 3, downloadAutoResume: true, downloadsPath: '/tmp/downloads', libraryPaths: ['/tmp/media'], globalRateLimitKB: 0, aiEnabled: true, aiTopK: 5, theme: 'clair', visibleColumns: ['nom'], fileServerAuthEnabled: false, fileServerAuthUser: '', dlnaEnabled: false, smbEnabled: false, smbShareName: 'shelfy', smbSharePath: '/tmp/media' }),
    saveSettings: vi.fn().mockResolvedValue({ downloadMaxConcurrent: 3, downloadAutoResume: true, downloadsPath: '/tmp/downloads', libraryPaths: ['/tmp/media'], globalRateLimitKB: 0, aiEnabled: true, aiTopK: 5, theme: 'clair', visibleColumns: ['nom'], fileServerAuthEnabled: false, fileServerAuthUser: '', dlnaEnabled: false, smbEnabled: false, smbShareName: 'shelfy', smbSharePath: '/tmp/media' }),
    getDLNADevices: vi.fn().mockResolvedValue({ enabled: false, devices: [], lastScan: '' }),
    scanDLNADevices: vi.fn().mockResolvedValue({ enabled: false, devices: [], lastScan: '' }),
    getSMBStatus: vi.fn().mockResolvedValue({ enabled: false, running: false, backend: 'fake', shareName: 'shelfy', sharePath: '/tmp/media', updatedAt: new Date().toISOString(), clients: [] }),
    refreshSMBStatus: vi.fn().mockResolvedValue({ enabled: false, running: false, backend: 'fake', shareName: 'shelfy', sharePath: '/tmp/media', updatedAt: new Date().toISOString(), clients: [] }),
    getStorageStats: vi.fn().mockResolvedValue({
      generatedAt: new Date().toISOString(),
      roots: [
        { path: '/tmp/downloads', exists: true, totalBytes: 1024 * 1024 * 1024, freeBytes: 512 * 1024 * 1024, availableBytes: 500 * 1024 * 1024, knownUsedBytes: 128 * 1024 * 1024 }
      ],
      totals: {
        totalBytes: 1024 * 1024 * 1024,
        freeBytes: 512 * 1024 * 1024,
        availableBytes: 500 * 1024 * 1024,
        knownUsedBytes: 128 * 1024 * 1024
      }
    }),
    runStorageSpeedtest: vi.fn().mockResolvedValue({ path: '/tmp/downloads/.shelfy-speedtest.tmp', sampleMB: 8, writeMBps: 120.5, readMBps: 210.2, durationMs: 260 }),
    getFSRoots: vi.fn().mockResolvedValue({ roots: ['/tmp/downloads', '/tmp/media'] }),
    listFS: vi.fn().mockResolvedValue({ path: '/tmp/media', items: [] }),
    readFSFile: vi.fn().mockResolvedValue({ path: '/tmp/media/note.txt', content: '' }),
    createFSDir: vi.fn().mockResolvedValue(undefined),
    writeFSFile: vi.fn().mockResolvedValue(undefined),
    moveFSPath: vi.fn().mockResolvedValue(undefined),
    deleteFSPath: vi.fn().mockResolvedValue(undefined),

    aiStatus: vi.fn().mockResolvedValue({ embeddingProvider: 'fake', llmProvider: 'fake' }),
    aiReport: vi.fn().mockResolvedValue({
      embeddingProvider: 'fake',
      llmProvider: 'fake',
      startedAt: new Date().toISOString(),
      uptimeSec: 12,
      indexRuns: 1,
      lastIndexedCount: 2,
      indexedTotal: 2,
      searchRuns: 3,
      searchAvgLatencyMs: 5,
      lastSearchLatencyMs: 4,
      lastSearchResults: 2,
      askRuns: 1,
      askSearchOnlyRuns: 0,
      askAvgLatencyMs: 8,
      lastAskLatencyMs: 8,
      lastAskSources: 2,
      embeddingCalls: 4,
      embeddingTokensEstimated: 340,
      llmPromptTokensEstimated: 122,
      llmCompletionTokensEstimated: 66
    }),
    aiIndex: vi.fn().mockResolvedValue({ indexed: 1 }),
    aiSearch: vi.fn().mockResolvedValue([]),
    aiAsk: vi.fn().mockResolvedValue({ answer: 'Réponse', sources: [] })
  }
}
