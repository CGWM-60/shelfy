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
  expect(screen.getByTestId('library-entry-Saison 1')).toBeInTheDocument()
  expect(screen.getByTestId('library-entry-Film A.mp4')).toBeInTheDocument()
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

  expect(await screen.findByTestId('library-entry-episode1.mkv')).toBeInTheDocument()
  fireEvent.click(screen.getByLabelText('Mode déplacement'))
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

test('menu clic droit: renommer un fichier', async () => {
  const client = createMockClient()
  client.getFSRoots = vi.fn().mockResolvedValue({ roots: ['/tmp/media'] })
  client.listFS = vi.fn().mockResolvedValue({
    path: '/tmp/media',
    items: [
      { name: 'episode1.mkv', path: '/tmp/media/episode1.mkv', isDir: false, sizeBytes: 2000, modifiedAt: new Date().toISOString() }
    ]
  })
  client.listMedia = vi.fn().mockResolvedValue([])
  const promptSpy = vi.spyOn(window, 'prompt').mockReturnValue('episode1-renamed.mkv')

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <LibraryPage />
      </MemoryRouter>
    </APIProvider>
  )

  const entry = await screen.findByTestId('library-entry-episode1.mkv')
  fireEvent.contextMenu(entry)

  expect(await screen.findByTestId('library-context-menu')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('menuitem', { name: 'Renommer' }))

  await waitFor(() => expect(client.moveFSPath).toHaveBeenCalledWith('/tmp/media/episode1.mkv', '/tmp/media/episode1-renamed.mkv'))
  promptSpy.mockRestore()
})

test('menu clic droit arborescence: renommer un dossier latéral', async () => {
  const client = createMockClient()
  client.getFSRoots = vi.fn().mockResolvedValue({ roots: ['/tmp/media'] })
  client.listFS = vi.fn().mockResolvedValue({
    path: '/tmp/media',
    items: [
      { name: 'Series', path: '/tmp/media/Series', isDir: true, sizeBytes: 0, modifiedAt: new Date().toISOString() }
    ]
  })
  client.listMedia = vi.fn().mockResolvedValue([])
  const promptSpy = vi.spyOn(window, 'prompt').mockReturnValue('Series-Renamed')

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <LibraryPage />
      </MemoryRouter>
    </APIProvider>
  )

  const treeFolder = await screen.findByTestId('library-tree-Series')
  fireEvent.contextMenu(treeFolder)
  expect(await screen.findByTestId('library-context-menu')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('menuitem', { name: 'Renommer' }))

  await waitFor(() => expect(client.moveFSPath).toHaveBeenCalledWith('/tmp/media/Series', '/tmp/media/Series-Renamed'))
  promptSpy.mockRestore()
})

test('menu clic droit arborescence: nouveau dossier depuis une racine', async () => {
  const client = createMockClient()
  client.getFSRoots = vi.fn().mockResolvedValue({ roots: ['/tmp/media'] })
  client.listFS = vi.fn().mockResolvedValue({
    path: '/tmp/media',
    items: []
  })
  client.listMedia = vi.fn().mockResolvedValue([])
  const promptSpy = vi.spyOn(window, 'prompt').mockReturnValue('Nouveau dossier')

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <LibraryPage />
      </MemoryRouter>
    </APIProvider>
  )

  const treeRoot = await screen.findByTestId('library-tree-media')
  fireEvent.contextMenu(treeRoot)
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Nouveau dossier ici' }))

  await waitFor(() => expect(client.createFSDir).toHaveBeenCalledWith('/tmp/media/Nouveau dossier'))
  promptSpy.mockRestore()
})

test('menu clic droit: couper puis coller dans un dossier', async () => {
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

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <LibraryPage />
      </MemoryRouter>
    </APIProvider>
  )

  const fileEntry = await screen.findByTestId('library-entry-episode1.mkv')
  fireEvent.contextMenu(fileEntry)
  fireEvent.click(await screen.findByRole('menuitem', { name: 'Couper' }))

  const folderEntry = screen.getByTestId('library-entry-Series')
  fireEvent.contextMenu(folderEntry)
  fireEvent.click(await screen.findByRole('menuitem', { name: /Coller ici/ }))

  await waitFor(() => expect(client.moveFSPath).toHaveBeenCalledWith('/tmp/media/episode1.mkv', '/tmp/media/Series/episode1.mkv'))
})
