import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { APIProvider } from '../api/context'
import { createMockClient } from '../test/mockClient'
import { DebridPage } from './DebridPage'

test('gère les comptes débrideurs', async () => {
  vi.spyOn(window, 'confirm').mockReturnValue(true)
  const client = createMockClient()
  client.listDebridProviders = vi.fn().mockResolvedValue([{ name: 'fake', authType: 'oauth2_device', supportsPassword: true }])
  client.listDebridAccounts = vi.fn().mockResolvedValue([{ id: 1, provider: 'fake', label: 'Compte A', isActive: true, isDefault: true, authType: 'none' }])

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <DebridPage />
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByText('Compte A')).toBeInTheDocument()
  fireEvent.change(screen.getByPlaceholderText('Label'), { target: { value: 'Nouveau compte' } })
  fireEvent.click(screen.getByText('Ajouter compte'))
  await waitFor(() => expect(client.createDebridAccount).toHaveBeenCalled())

  fireEvent.click(screen.getByText('Connecter Debrid-Link'))
  await waitFor(() => expect(client.startDebridAuth).toHaveBeenCalled())
  expect(await screen.findByText(/Code:/)).toBeInTheDocument()
  fireEvent.click(screen.getByText("J'ai validé"))
  await waitFor(() => expect(client.pollDebridAuth).toHaveBeenCalled())

  fireEvent.change(screen.getByPlaceholderText('Login Debrid-Link'), { target: { value: 'demo@example.com' } })
  fireEvent.change(screen.getByPlaceholderText('Mot de passe Debrid-Link'), { target: { value: 'secret' } })
  fireEvent.click(screen.getByText('Connexion login/mot de passe'))
  await waitFor(() => expect(client.passwordDebridAuth).toHaveBeenCalledWith(1, { username: 'demo@example.com', password: 'secret' }))

  fireEvent.click(screen.getByText('Supprimer compte'))
  await waitFor(() => expect(client.deleteDebridAccount).toHaveBeenCalledWith(1))
})

test("affiche l'erreur si création de compte échoue", async () => {
  const client = createMockClient()
  client.listDebridProviders = vi.fn().mockResolvedValue([{ name: 'fake', authType: 'oauth2_device', supportsPassword: true }])
  client.createDebridAccount = vi.fn().mockRejectedValue(new Error('provider indisponible'))

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <DebridPage />
      </MemoryRouter>
    </APIProvider>
  )

  fireEvent.change(await screen.findByPlaceholderText('Label'), { target: { value: 'Compte B' } })
  fireEvent.click(screen.getByText('Ajouter compte'))
  expect(await screen.findByRole('alert')).toHaveTextContent('provider indisponible')
})
