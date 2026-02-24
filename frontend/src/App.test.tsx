import { render, screen } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { vi } from 'vitest'
import { APIProvider } from './api/context'
import { createMockClient } from './test/mockClient'
import App from './App'

test('affiche la navigation principale', async () => {
  render(
    <APIProvider client={createMockClient()}>
      <MemoryRouter>
        <App />
      </MemoryRouter>
    </APIProvider>
  )

  await screen.findByTestId('downloads-page')
  expect(screen.getByText('Téléchargements')).toBeInTheDocument()
  expect(screen.getByText('Stockage')).toBeInTheDocument()
  expect(screen.getByText('Terminal')).toBeInTheDocument()
  expect(screen.getByText('Rapport IA')).toBeInTheDocument()
  expect(screen.getByText('Débrideurs')).toBeInTheDocument()
  expect(screen.getByText('Paramètres')).toBeInTheDocument()
})

test('affiche la page de login quand auth activée sans session', async () => {
  const client = createMockClient()
  client.authStatus = vi.fn().mockResolvedValue({ enabled: true, authenticated: false, username: '' })

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <App />
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByText('Se connecter')).toBeInTheDocument()
  expect(screen.queryByText('Téléchargements')).not.toBeInTheDocument()
})
