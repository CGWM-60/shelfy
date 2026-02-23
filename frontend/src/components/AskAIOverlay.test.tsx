import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { APIProvider } from '../api/context'
import { createMockClient } from '../test/mockClient'
import { AskAIOverlay, openAskAIOverlay } from './AskAIOverlay'

test('ouvre le panneau Ask AI depuis le bouton flottant et pose une question', async () => {
  const client = createMockClient()
  client.aiAsk = vi.fn().mockResolvedValue({
    answer: 'Réponse overlay',
    sources: [{ mediaId: 'm1', title: 'Film A', snippet: '...', score: 0.9 }]
  })

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <AskAIOverlay />
      </MemoryRouter>
    </APIProvider>
  )

  fireEvent.click(screen.getByRole('button', { name: 'Ouvrir Ask AI' }))
  expect(await screen.findByTestId('ask-ai-chat-window')).toBeInTheDocument()
  fireEvent.change(screen.getByPlaceholderText('Posez une question'), { target: { value: 'Que regarder ?' } })
  fireEvent.click(screen.getByRole('button', { name: 'Envoyer' }))

  await waitFor(() => expect(client.aiAsk).toHaveBeenCalledWith('Que regarder ?', false))
  expect(await screen.findByText('Réponse overlay')).toBeInTheDocument()
})

test('ouvre le panneau via évènement global avec question pré-remplie', async () => {
  const client = createMockClient()

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <AskAIOverlay />
      </MemoryRouter>
    </APIProvider>
  )

  act(() => {
    openAskAIOverlay({ question: 'Parle-moi de Film A' })
  })

  expect(await screen.findByTestId('ask-ai-chat-window')).toBeInTheDocument()
  expect(screen.getByDisplayValue('Parle-moi de Film A')).toBeInTheDocument()
})
