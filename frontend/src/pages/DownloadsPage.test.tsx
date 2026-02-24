import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { APIProvider } from '../api/context'
import { createMockClient } from '../test/mockClient'
import { DownloadsPage } from './DownloadsPage'

class MockEventSource {
  static instances: MockEventSource[] = []

  url: string
  listeners: Record<string, Array<(event: MessageEvent) => void>> = {}

  constructor(url: string) {
    this.url = url
    MockEventSource.instances.push(this)
  }

  addEventListener(type: string, listener: (event: MessageEvent) => void) {
    if (!this.listeners[type]) {
      this.listeners[type] = []
    }
    this.listeners[type].push(listener)
  }

  close() {}

  emit(type: string, payload: unknown) {
    const handlers = this.listeners[type] ?? []
    for (const handler of handlers) {
      handler({ data: JSON.stringify(payload) } as MessageEvent)
    }
  }
}

test('affiche les jobs et ajoute un téléchargement', async () => {
  const client = createMockClient()
  client.listDownloads = vi.fn().mockResolvedValue([{ id: 'd1', sourceLink: 'a', directLink: 'a', fileName: 'a.bin', status: 'queued', downloadedBytes: 0, sizeBytes: 0, speedBytes: 0, etaSeconds: 0, useDebrid: false }])

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <DownloadsPage />
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByText('a.bin')).toBeInTheDocument()
  fireEvent.change(screen.getByPlaceholderText('Collez un ou plusieurs liens'), { target: { value: 'https://x.test/file.bin' } })
  fireEvent.change(screen.getByPlaceholderText('Dossier destination (optionnel)'), { target: { value: '/tmp/media' } })
  fireEvent.click(screen.getByText('Ajouter'))

  await waitFor(() => expect(client.addDownloads).toHaveBeenCalled())
  expect(client.addDownloads).toHaveBeenCalledWith(expect.objectContaining({ destinationDir: '/tmp/media' }))
})

test('met à jour la progression en temps réel via SSE', async () => {
  const originalEventSource = globalThis.EventSource
  MockEventSource.instances = []
  globalThis.EventSource = MockEventSource as unknown as typeof EventSource
  try {
    const client = createMockClient()
    client.listDownloads = vi.fn().mockResolvedValue([
      {
        id: 'd1',
        sourceLink: 'a',
        directLink: 'a',
        fileName: 'a.bin',
        status: 'running',
        downloadedBytes: 0,
        sizeBytes: 4096,
        speedBytes: 0,
        etaSeconds: 0,
        useDebrid: false
      }
    ])

    render(
      <APIProvider client={client}>
        <MemoryRouter>
          <DownloadsPage />
        </MemoryRouter>
      </APIProvider>
    )

    expect(await screen.findByText('a.bin')).toBeInTheDocument()
    expect(MockEventSource.instances[0]?.url).toBe('/api/events')

    await act(async () => {
      MockEventSource.instances[0].emit('download_progress', {
        id: 'd1',
        downloadedBytes: 2048,
        sizeBytes: 4096,
        speedBytes: 1024,
        etaSeconds: 2
      })
    })

    await waitFor(() => expect(screen.getByText('2 Ko / 4 Ko')).toBeInTheDocument())
    expect(screen.getByText('vitesse 1 Ko/s')).toBeInTheDocument()
  } finally {
    globalThis.EventSource = originalEventSource
  }
})

test("affiche la raison d'échec en temps réel via SSE", async () => {
  const originalEventSource = globalThis.EventSource
  MockEventSource.instances = []
  globalThis.EventSource = MockEventSource as unknown as typeof EventSource
  try {
    const client = createMockClient()
    client.listDownloads = vi.fn().mockResolvedValue([
      {
        id: 'd-failed',
        sourceLink: 'a',
        directLink: 'a',
        fileName: 'broken.bin',
        status: 'running',
        downloadedBytes: 0,
        sizeBytes: 4096,
        speedBytes: 0,
        etaSeconds: 0,
        useDebrid: false
      }
    ])

    render(
      <APIProvider client={client}>
        <MemoryRouter>
          <DownloadsPage />
        </MemoryRouter>
      </APIProvider>
    )

    expect(await screen.findByText('broken.bin')).toBeInTheDocument()

    await act(async () => {
      MockEventSource.instances[0].emit('download_failed', {
        id: 'd-failed',
        error: 'http status 502'
      })
    })

    await waitFor(() => expect(screen.getByText('Raison: http status 502')).toBeInTheDocument())
  } finally {
    globalThis.EventSource = originalEventSource
  }
})

test('affiche host + retries + prochaine tentative sur event retry', async () => {
  const originalEventSource = globalThis.EventSource
  MockEventSource.instances = []
  globalThis.EventSource = MockEventSource as unknown as typeof EventSource
  try {
    const client = createMockClient()
    client.listDownloads = vi.fn().mockResolvedValue([
      {
        id: 'd-retry',
        sourceLink: 'https://source.example.test/file.bin',
        directLink: 'https://cdn.example.test/file.bin',
        fileName: 'retry.bin',
        status: 'running',
        downloadedBytes: 0,
        sizeBytes: 4096,
        speedBytes: 0,
        etaSeconds: 0,
        useDebrid: false,
        retries: 0,
        maxRetries: 3
      }
    ])

    render(
      <APIProvider client={client}>
        <MemoryRouter>
          <DownloadsPage />
        </MemoryRouter>
      </APIProvider>
    )

    expect(await screen.findByText('retry.bin')).toBeInTheDocument()
    expect(screen.getByText('Hôte cdn.example.test')).toBeInTheDocument()
    expect(screen.getByText('Retry 0/3')).toBeInTheDocument()

    await act(async () => {
      MockEventSource.instances[0].emit('download_retry', {
        id: 'd-retry',
        retry: 1,
        retryMax: 3,
        nextRetryInMs: 1500,
        error: 'temporary gateway error'
      })
    })

    await waitFor(() => expect(screen.getByText('Retry 1/3')).toBeInTheDocument())
    expect(screen.getByText('Prochaine tentative 2s')).toBeInTheDocument()
    expect(screen.getByText('Raison: temporary gateway error')).toBeInTheDocument()
  } finally {
    globalThis.EventSource = originalEventSource
  }
})

test('affiche un toast quand un doublon est ignoré via SSE', async () => {
  const originalEventSource = globalThis.EventSource
  MockEventSource.instances = []
  globalThis.EventSource = MockEventSource as unknown as typeof EventSource
  try {
    const client = createMockClient()
    client.listDownloads = vi.fn().mockResolvedValue([])

    render(
      <APIProvider client={client}>
        <MemoryRouter>
          <DownloadsPage />
        </MemoryRouter>
      </APIProvider>
    )

    await screen.findByText('Dashboard téléchargements')

    await act(async () => {
      MockEventSource.instances[0].emit('download_duplicate_skipped', {
        source: 'https://example.test/folder/file-01.mkv',
        destination: '/tmp/downloads/Series/file-01.mkv'
      })
    })

    await waitFor(() => expect(screen.getByText('Doublon ignoré: file-01.mkv')).toBeInTheDocument())
  } finally {
    globalThis.EventSource = originalEventSource
  }
})
