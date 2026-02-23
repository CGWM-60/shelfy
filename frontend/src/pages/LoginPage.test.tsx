import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { vi } from 'vitest'
import { APIProvider } from '../api/context'
import { createMockClient } from '../test/mockClient'
import { LoginPage } from './LoginPage'

test('login page submits credentials', async () => {
  const client = createMockClient()
  const onLoggedIn = vi.fn()

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <LoginPage onLoggedIn={onLoggedIn} />
      </MemoryRouter>
    </APIProvider>
  )

  fireEvent.change(screen.getByLabelText('Identifiant'), { target: { value: 'admin' } })
  fireEvent.change(screen.getByLabelText('Mot de passe'), { target: { value: 'secret' } })
  fireEvent.click(screen.getByRole('button', { name: 'Connexion' }))

  await waitFor(() => expect(client.authLogin).toHaveBeenCalledWith({ username: 'admin', password: 'secret' }))
  expect(onLoggedIn).toHaveBeenCalled()
})
