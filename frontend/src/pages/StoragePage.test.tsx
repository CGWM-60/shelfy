import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { APIProvider } from '../api/context'
import { StoragePage } from './StoragePage'
import { createMockClient } from '../test/mockClient'

test('affiche les statistiques de stockage', async () => {
  const client = createMockClient()
  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <StoragePage />
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByText('Stockage & Débit')).toBeInTheDocument()
  expect(screen.getByText('Capacité totale')).toBeInTheDocument()
  expect(screen.getByText('Dossiers gérés')).toBeInTheDocument()
})

test('lance un test de débit', async () => {
  const client = createMockClient()
  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <StoragePage />
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByText('Lancer le test')).toBeInTheDocument()
  fireEvent.click(screen.getByText('Lancer le test'))

  await waitFor(() => expect(client.runStorageSpeedtest).toHaveBeenCalled())
  const throughputs = await screen.findAllByText(/Mo\/s/)
  expect(throughputs.length).toBeGreaterThanOrEqual(2)
})
