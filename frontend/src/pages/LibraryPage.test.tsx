import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { APIProvider } from '../api/context'
import { createMockClient } from '../test/mockClient'
import { LibraryPage } from './LibraryPage'

test('scan et liste la bibliothèque', async () => {
  const client = createMockClient()
  client.listMedia = vi.fn().mockResolvedValue([{ id: 'm1', title: 'Film A', kind: 'video', path: '/tmp/a.mp4' }])

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <LibraryPage />
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByText('Film A')).toBeInTheDocument()
  fireEvent.click(screen.getByText('Scanner la bibliothèque'))
  await waitFor(() => expect(client.scanMedia).toHaveBeenCalled())
})
