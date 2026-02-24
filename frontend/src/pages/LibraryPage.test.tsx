import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { APIProvider } from '../api/context'
import { createMockClient } from '../test/mockClient'
import { LibraryPage } from './LibraryPage'

test('scan et affiche la bibliothèque en mode explorateur', async () => {
  const client = createMockClient()
  client.getFSRoots = vi.fn().mockResolvedValue({ roots: ['/tmp/media'] })
  client.listFS = vi.fn().mockResolvedValue({
    path: '/tmp/media',
    items: [
      { name: 'Saison 1', path: '/tmp/media/Saison 1', isDir: true, sizeBytes: 0, modifiedAt: new Date().toISOString() },
      { name: 'Film A.mp4', path: '/tmp/media/Film A.mp4', isDir: false, sizeBytes: 1000, modifiedAt: new Date().toISOString() }
    ]
  })
  client.listMedia = vi.fn().mockResolvedValue([{ id: 'm1', title: 'Film A', kind: 'video', path: '/tmp/media/Film A.mp4' }])

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <LibraryPage />
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByText('Bibliothèque explorateur')).toBeInTheDocument()
  expect(screen.getByText('[DIR] Saison 1')).toBeInTheDocument()
  expect(screen.getByText('[FILE] Film A.mp4')).toBeInTheDocument()
  fireEvent.click(screen.getByText('Scanner la bibliothèque'))
  await waitFor(() => expect(client.scanMedia).toHaveBeenCalled())
})

test('supprime et déplace via drag & drop', async () => {
  const client = createMockClient()
  client.getFSRoots = vi.fn().mockResolvedValue({ roots: ['/tmp/media'] })
  client.listFS = vi.fn().mockResolvedValue({
    path: '/tmp/media',
    items: [
      { name: 'Series', path: '/tmp/media/Series', isDir: true, sizeBytes: 0, modifiedAt: new Date().toISOString() },
      { name: 'episode1.mkv', path: '/tmp/media/episode1.mkv', isDir: false, sizeBytes: 2000, modifiedAt: new Date().toISOString() }
    ]
  })
  client.listMedia = vi.fn().mockResolvedValue([])
  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <LibraryPage />
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByText('[FILE] episode1.mkv')).toBeInTheDocument()
  fireEvent.click(screen.getByLabelText('Supprimer episode1.mkv'))
  await waitFor(() => expect(client.deleteFSPath).toHaveBeenCalledWith('/tmp/media/episode1.mkv', false))

  const source = screen.getByTestId('library-entry-episode1.mkv')
  const target = screen.getByTestId('library-entry-Series')
  fireEvent.dragStart(source)
  fireEvent.dragOver(target)
  fireEvent.drop(target)

  await waitFor(() => expect(client.moveFSPath).toHaveBeenCalledWith('/tmp/media/episode1.mkv', '/tmp/media/Series/episode1.mkv'))
  confirmSpy.mockRestore()
})
