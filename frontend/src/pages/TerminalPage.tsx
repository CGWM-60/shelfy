import { useEffect, useRef, useState } from 'react'
import '@xterm/xterm/css/xterm.css'

function buildTerminalWSURL() {
  const apiBase = (import.meta.env.VITE_API_BASE ?? '').trim()
  const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'

  if (!apiBase) {
    return `${wsProtocol}//${window.location.host}/api/terminal/ws`
  }
  if (apiBase.startsWith('http://') || apiBase.startsWith('https://')) {
    const url = new URL(apiBase)
    url.protocol = url.protocol === 'https:' ? 'wss:' : 'ws:'
    url.pathname = `${url.pathname.replace(/\/+$/, '')}/api/terminal/ws`
    return url.toString()
  }
  return `${wsProtocol}//${window.location.host}${apiBase.replace(/\/+$/, '')}/api/terminal/ws`
}

export function TerminalPage() {
  const containerRef = useRef<HTMLDivElement | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let disposed = false
    let socket: WebSocket | null = null
    let disposeInput: (() => void) | null = null
    let disposeTerminal: (() => void) | null = null
    let resizeObserver: ResizeObserver | null = null

    async function boot() {
      try {
        const [{ Terminal }, { FitAddon }] = await Promise.all([import('@xterm/xterm'), import('@xterm/addon-fit')])
        if (disposed || !containerRef.current) return

        const terminal = new Terminal({
          cursorBlink: true,
          fontSize: 13,
          scrollback: 5000,
          fontFamily: 'Menlo, Monaco, Consolas, "Liberation Mono", monospace',
          theme: {
            background: '#0f172a',
            foreground: '#e2e8f0',
            cursor: '#38bdf8'
          }
        })
        const fitAddon = new FitAddon()
        terminal.loadAddon(fitAddon)
        terminal.open(containerRef.current)
        fitAddon.fit()
        terminal.writeln('Shelfy Terminal')
        terminal.writeln('Session interactive connectee au serveur.')
        terminal.writeln('')

        socket = new WebSocket(buildTerminalWSURL())
        socket.binaryType = 'arraybuffer'
        socket.onopen = () => {
          terminal.writeln('Connexion etablie.')
        }
        socket.onmessage = (event) => {
          if (typeof event.data === 'string') {
            terminal.write(event.data)
            return
          }
          if (event.data instanceof ArrayBuffer) {
            terminal.write(new TextDecoder().decode(event.data))
          }
        }
        socket.onclose = () => {
          terminal.writeln('\r\nSession fermee.')
        }
        socket.onerror = () => {
          setError('Connexion terminal impossible')
        }

        const inputDisposable = terminal.onData((chunk) => {
          if (socket?.readyState === WebSocket.OPEN) {
            socket.send(chunk)
          }
        })
        disposeInput = () => inputDisposable.dispose()

        if ('ResizeObserver' in window) {
          resizeObserver = new ResizeObserver(() => {
            fitAddon.fit()
          })
          resizeObserver.observe(containerRef.current)
        }

        disposeTerminal = () => terminal.dispose()
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Terminal indisponible')
      }
    }

    void boot()

    return () => {
      disposed = true
      if (resizeObserver) resizeObserver.disconnect()
      if (disposeInput) disposeInput()
      if (socket && socket.readyState < WebSocket.CLOSING) socket.close()
      if (disposeTerminal) disposeTerminal()
    }
  }, [])

  return (
    <section className="panel stack" data-testid="terminal-page">
      <div>
        <h1 className="m-0 text-2xl font-bold text-slate-900">Terminal</h1>
        <p className="m-0 mt-1 text-sm text-slate-600">Console serveur en direct pour inspection rapide et commandes d’administration.</p>
      </div>
      {error ? <p role="alert" className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p> : null}
      <div className="overflow-hidden rounded-2xl border border-slate-800 bg-slate-950 p-2">
        <div ref={containerRef} className="h-[60vh] min-h-[320px] w-full" data-testid="terminal-xterm" />
      </div>
      <p className="m-0 text-xs text-slate-500">Raccourci: clique dans la console pour saisir des commandes.</p>
    </section>
  )
}
