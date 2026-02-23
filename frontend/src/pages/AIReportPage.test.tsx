import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { APIProvider } from '../api/context'
import { createMockClient } from '../test/mockClient'
import { AIReportPage } from './AIReportPage'

test('affiche le rapport IA et permet de relancer une indexation', async () => {
  const client = createMockClient()
  client.aiReport = vi
    .fn()
    .mockResolvedValueOnce({
      embeddingProvider: 'fake',
      llmProvider: 'fake',
      startedAt: new Date().toISOString(),
      uptimeSec: 30,
      indexRuns: 2,
      lastIndexedCount: 4,
      indexedTotal: 8,
      searchRuns: 3,
      searchAvgLatencyMs: 12.5,
      lastSearchLatencyMs: 14,
      lastSearchResults: 5,
      askRuns: 2,
      askSearchOnlyRuns: 1,
      askAvgLatencyMs: 22.1,
      lastAskLatencyMs: 20,
      lastAskSources: 3,
      embeddingCalls: 6,
      embeddingTokensEstimated: 140,
      llmPromptTokensEstimated: 80,
      llmCompletionTokensEstimated: 20
    })
    .mockResolvedValueOnce({
      embeddingProvider: 'fake',
      llmProvider: 'fake',
      startedAt: new Date().toISOString(),
      uptimeSec: 33,
      indexRuns: 3,
      lastIndexedCount: 2,
      indexedTotal: 10,
      searchRuns: 3,
      searchAvgLatencyMs: 12.5,
      lastSearchLatencyMs: 14,
      lastSearchResults: 5,
      askRuns: 2,
      askSearchOnlyRuns: 1,
      askAvgLatencyMs: 22.1,
      lastAskLatencyMs: 20,
      lastAskSources: 3,
      embeddingCalls: 7,
      embeddingTokensEstimated: 180,
      llmPromptTokensEstimated: 90,
      llmCompletionTokensEstimated: 30
    })
  client.aiIndex = vi.fn().mockResolvedValue({ indexed: 2 })

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <AIReportPage />
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByText('Rapport IA')).toBeInTheDocument()
  expect(screen.getAllByText('fake')).toHaveLength(2)
  expect(screen.getByText('140')).toBeInTheDocument()

  fireEvent.click(screen.getByRole('button', { name: 'Indexer maintenant' }))

  await waitFor(() => expect(client.aiIndex).toHaveBeenCalled())
  await waitFor(() => expect(client.aiReport).toHaveBeenCalledTimes(2))
  expect(await screen.findByText('2 élément(s) indexé(s)')).toBeInTheDocument()
})
