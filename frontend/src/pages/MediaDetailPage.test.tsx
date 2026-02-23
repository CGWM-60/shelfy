import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { APIProvider } from '../api/context'
import { createMockClient } from '../test/mockClient'
import { MediaDetailPage } from './MediaDetailPage'

test('affiche fiche média et sauvegarde progression', async () => {
  const client = createMockClient()
  client.getMedia = vi.fn().mockResolvedValue({
    item: { id: 'm1', title: 'Film A', kind: 'video', path: '/tmp/a.mp4' },
    progress: { mediaId: 'm1', positionMs: 5000 }
  })

  render(
    <APIProvider client={client}>
      <MemoryRouter initialEntries={['/media/m1']}>
        <Routes>
          <Route path="/media/:id" element={<MediaDetailPage />} />
        </Routes>
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByText('Film A')).toBeInTheDocument()
  fireEvent.click(screen.getByText('Sauvegarder position'))
  await waitFor(() => expect(client.saveMediaProgress).toHaveBeenCalled())
})

test('affiche viewer image', async () => {
  const client = createMockClient()
  client.getMedia = vi.fn().mockResolvedValue({
    item: { id: 'm2', title: 'Image A', kind: 'image', path: '/tmp/a.jpg' },
    progress: { mediaId: 'm2', positionMs: 0 }
  })

  render(
    <APIProvider client={client}>
      <MemoryRouter initialEntries={['/media/m2']}>
        <Routes>
          <Route path="/media/:id" element={<MediaDetailPage />} />
        </Routes>
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByAltText('Image A')).toBeInTheDocument()
})

test('affiche viewer pdf', async () => {
  const client = createMockClient()
  client.getMedia = vi.fn().mockResolvedValue({
    item: { id: 'm3', title: 'PDF A', kind: 'pdf', path: '/tmp/a.pdf' },
    progress: { mediaId: 'm3', positionMs: 0 }
  })

  render(
    <APIProvider client={client}>
      <MemoryRouter initialEntries={['/media/m3']}>
        <Routes>
          <Route path="/media/:id" element={<MediaDetailPage />} />
        </Routes>
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByTitle('PDF A')).toBeInTheDocument()
})
