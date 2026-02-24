import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { APIProvider } from '../api/context'
import { createMockClient } from '../test/mockClient'
import { TerminalPage } from './TerminalPage'

class MockWebSocket {
  static OPEN = 1
  static CLOSING = 2
  static instances: MockWebSocket[] = []

  readyState = MockWebSocket.OPEN
  binaryType = ''
  url: string
  onopen: (() => void) | null = null
  onmessage: ((event: MessageEvent) => void) | null = null
  onclose: (() => void) | null = null
  onerror: (() => void) | null = null
  send = vi.fn()
  close = vi.fn()

  constructor(url: string) {
    this.url = url
    MockWebSocket.instances.push(this)
    setTimeout(() => this.onopen?.(), 0)
  }
}

vi.mock('@xterm/xterm', () => {
  class FakeTerminal {
    loadAddon() {}
    open() {}
    writeln() {}
    write() {}
    onData() {
      return { dispose() {} }
    }
    dispose() {}
  }
  return { Terminal: FakeTerminal }
})

vi.mock('@xterm/addon-fit', () => {
  class FakeFitAddon {
    fit() {}
  }
  return { FitAddon: FakeFitAddon }
})

afterEach(() => {
  vi.unstubAllGlobals()
})

test('affiche la page terminal et ouvre un websocket', async () => {
  MockWebSocket.instances = []
  vi.stubGlobal('WebSocket', MockWebSocket as unknown as typeof WebSocket)

  render(
    <APIProvider client={createMockClient()}>
      <MemoryRouter>
        <TerminalPage />
      </MemoryRouter>
    </APIProvider>
  )

  expect(screen.getByTestId('terminal-page')).toBeInTheDocument()
  expect(screen.getByTestId('terminal-xterm')).toBeInTheDocument()
  await waitFor(() => expect(MockWebSocket.instances.length).toBe(1))
  expect(MockWebSocket.instances[0].url).toContain('/api/terminal/ws')
})
