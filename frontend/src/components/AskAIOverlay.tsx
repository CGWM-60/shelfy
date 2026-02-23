import { FormEvent, useEffect, useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import { useAPI } from '../api/context'
import type { AIResult } from '../api/types'

type AskAIEventDetail = {
  question?: string
}

const OPEN_EVENT = 'shelfy:ask-ai-open'

type ChatMessage = {
  id: string
  role: 'user' | 'assistant'
  text: string
  sources?: AIResult[]
}

function nextMessageID(prefix: string) {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`
}

export function openAskAIOverlay(detail: AskAIEventDetail = {}) {
  window.dispatchEvent(new CustomEvent<AskAIEventDetail>(OPEN_EVENT, { detail }))
}

export function AskAIOverlay() {
  const api = useAPI()
  const [isOpen, setIsOpen] = useState(false)
  const [question, setQuestion] = useState('')
  const [searchOnly, setSearchOnly] = useState(false)
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const hasMessages = useMemo(() => messages.length > 0, [messages])

  useEffect(() => {
    const onOpen = (event: Event) => {
      const custom = event as CustomEvent<AskAIEventDetail>
      if (custom.detail?.question) {
        setQuestion(custom.detail.question)
      }
      setIsOpen(true)
    }
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && isOpen) {
        setIsOpen(false)
      }
    }
    window.addEventListener(OPEN_EVENT, onOpen)
    window.addEventListener('keydown', onKeyDown)
    return () => {
      window.removeEventListener(OPEN_EVENT, onOpen)
      window.removeEventListener('keydown', onKeyDown)
    }
  }, [isOpen])

  async function onSubmit(event: FormEvent) {
    event.preventDefault()
    const value = question.trim()
    if (!value) return
    const userMessageID = nextMessageID('user')
    setMessages((prev) => [...prev, { id: userMessageID, role: 'user', text: value }])
    setLoading(true)
    setError('')
    try {
      const response = await api.aiAsk(value, searchOnly)
      const answerID = nextMessageID('assistant')
      setMessages((prev) => [...prev, { id: answerID, role: 'assistant', text: response.answer, sources: response.sources }])
      setQuestion('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Requête IA impossible')
    } finally {
      setLoading(false)
    }
  }

  return (
    <>
      <button
        type="button"
        className="fixed bottom-24 right-4 z-40 rounded-full border border-brand-700 bg-brand-600 px-4 py-3 text-sm font-semibold text-white shadow-xl transition hover:bg-brand-700 md:right-8"
        onClick={() => setIsOpen(true)}
        aria-label={isOpen ? 'Afficher fenêtre Ask AI' : 'Ouvrir Ask AI'}
      >
        Ask AI
      </button>

      {isOpen ? (
        <section
          aria-label="Fenêtre Ask AI"
          className="fixed bottom-24 right-2 z-50 flex h-[74vh] w-[calc(100%-1rem)] max-w-md flex-col overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-2xl md:bottom-8 md:right-8 md:h-[68vh]"
          data-testid="ask-ai-chat-window"
        >
          <div className="row border-b border-slate-200 bg-slate-50/80 px-4 py-3">
            <div>
              <h2 className="m-0 text-base font-bold text-slate-900">Ask AI</h2>
              <p className="m-0 mt-1 text-xs text-slate-500">Fenêtre de chat globale</p>
            </div>
            <button type="button" className="btn" onClick={() => setIsOpen(false)}>
              Réduire
            </button>
          </div>

          <div className="flex-1 overflow-y-auto bg-slate-100/70 px-3 py-3">
            {!hasMessages ? (
              <div className="rounded-xl border border-dashed border-slate-300 bg-white px-3 py-4 text-sm text-slate-600">
                Pose une question pour démarrer la conversation.
              </div>
            ) : null}
            <ul className="m-0 flex list-none flex-col gap-2 p-0">
              {messages.map((message) => (
                <li key={message.id} className={`max-w-[92%] rounded-2xl px-3 py-2 text-sm shadow-sm ${message.role === 'user' ? 'ml-auto bg-brand-600 text-white' : 'bg-white text-slate-800'}`}>
                  <p className="m-0 whitespace-pre-wrap leading-relaxed">{message.text}</p>
                  {message.sources && message.sources.length > 0 ? (
                    <ul className="mt-2 m-0 flex list-none flex-col gap-1 p-0">
                      {message.sources.map((source) => (
                        <li key={`${source.mediaId}-${source.score}`} className="rounded-lg border border-slate-200 bg-slate-50 px-2 py-1">
                          <Link
                            className="text-xs font-semibold text-slate-800 no-underline hover:text-brand-700"
                            to={`/media/${source.mediaId}`}
                            onClick={() => setIsOpen(false)}
                          >
                            {source.title || source.mediaId}
                          </Link>
                        </li>
                      ))}
                    </ul>
                  ) : null}
                </li>
              ))}
              {loading ? (
                <li className="max-w-[92%] rounded-2xl bg-white px-3 py-2 text-sm text-slate-500 shadow-sm">
                  IA en train de répondre...
                </li>
              ) : null}
            </ul>
          </div>

          <div className="border-t border-slate-200 bg-white p-3">
            {error ? <p role="alert" className="mb-2 rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-sm text-rose-700">{error}</p> : null}
            <form onSubmit={onSubmit} className="stack">
              <textarea
                className="field min-h-20"
                value={question}
                onChange={(e) => setQuestion(e.target.value)}
                placeholder="Posez une question"
                rows={2}
                required
              />
              <label className="inline-flex items-center gap-2 text-sm text-slate-700">
                <input className="h-4 w-4 rounded border-slate-300 text-brand-600 focus:ring-brand-300" type="checkbox" checked={searchOnly} onChange={(e) => setSearchOnly(e.target.checked)} />
                Mode Search only
              </label>
              <div className="flex flex-wrap gap-2">
                <button className="btn btn-primary" type="submit" disabled={loading}>{loading ? 'Chargement...' : 'Envoyer'}</button>
                <button
                  type="button"
                  className="btn"
                  onClick={() => {
                    setQuestion('')
                    setMessages([])
                    setError('')
                  }}
                >
                  Nouvelle conversation
                </button>
              </div>
            </form>
          </div>
        </section>
      ) : null}
    </>
  )
}
