import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { APIProvider } from '../api/context'
import { createMockClient } from '../test/mockClient'
import { SettingsPage } from './SettingsPage'

beforeEach(() => {
  document.documentElement.classList.remove('dark')
})

test('charge et sauvegarde les paramètres', async () => {
  const client = createMockClient()
  client.getDLNADevices = vi.fn().mockResolvedValue({
    enabled: false,
    devices: [],
    lastScan: ''
  })
  client.getSMBStatus = vi.fn().mockResolvedValue({
    enabled: false,
    running: false,
    backend: 'fake',
    shareName: 'shelfy',
    sharePath: '/tmp/media',
    updatedAt: new Date().toISOString(),
    clients: []
  })

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByText('Paramètres')).toBeInTheDocument()
  fireEvent.change(screen.getByDisplayValue('3'), { target: { value: '4' } })
  fireEvent.change(screen.getByDisplayValue('Clair'), { target: { value: 'sombre' } })
  await waitFor(() => expect(document.documentElement.classList.contains('dark')).toBe(true))
  fireEvent.click(screen.getByLabelText('DLNA activé'))
  fireEvent.click(screen.getByLabelText('SMB activé'))
  fireEvent.click(screen.getByText('Enregistrer'))
  await waitFor(() => expect(client.saveSettings).toHaveBeenCalled())
})

test('gestionnaire fichiers: crée, édite, déplace et supprime', async () => {
  const client = createMockClient()
  client.getFSRoots = vi.fn().mockResolvedValue({ roots: ['/tmp/media'] })
  client.listFS = vi.fn().mockResolvedValue({
    path: '/tmp/media',
    items: [{ name: 'film.mp4', path: '/tmp/media/film.mp4', isDir: false, sizeBytes: 100, modifiedAt: new Date().toISOString() }]
  })
  client.readFSFile = vi.fn().mockResolvedValue({ path: '/tmp/media/film.mp4', content: 'abc' })
  client.getSettings = vi.fn().mockResolvedValue({ downloadMaxConcurrent: 3, downloadAutoResume: true, downloadAutoGroup: true, downloadsPath: '/tmp/downloads', libraryPaths: ['/tmp/media'], globalRateLimitKB: 0, aiEnabled: true, aiTopK: 5, theme: 'clair', visibleColumns: ['nom'], fileServerAuthEnabled: false, fileServerAuthUser: '', dlnaEnabled: true, smbEnabled: true, smbShareName: 'shelfy', smbSharePath: '/tmp/media' })
  client.getDLNADevices = vi.fn().mockResolvedValue({
    enabled: true,
    devices: [{ usn: 'uuid:tv-1', st: 'upnp:rootdevice', server: 'DLNA/1.5', location: 'http://tv.local/device.xml', address: '192.168.1.20:1900', lastSeenAt: new Date().toISOString() }],
    lastScan: new Date().toISOString()
  })
  client.scanDLNADevices = vi.fn().mockResolvedValue({
    enabled: true,
    devices: [{ usn: 'uuid:tv-1', st: 'upnp:rootdevice', server: 'DLNA/1.5', location: 'http://tv.local/device.xml', address: '192.168.1.20:1900', lastSeenAt: new Date().toISOString() }],
    lastScan: new Date().toISOString()
  })
  client.getSMBStatus = vi.fn().mockResolvedValue({
    enabled: true,
    running: true,
    backend: 'fake',
    shareName: 'shelfy',
    sharePath: '/tmp/media',
    updatedAt: new Date().toISOString(),
    clients: [{ username: 'guest', machine: 'VLC', address: '192.168.1.33', connectedAt: new Date().toISOString() }]
  })
  client.refreshSMBStatus = vi.fn().mockResolvedValue({
    enabled: true,
    running: true,
    backend: 'fake',
    shareName: 'shelfy',
    sharePath: '/tmp/media',
    updatedAt: new Date().toISOString(),
    clients: [{ username: 'guest', machine: 'VLC', address: '192.168.1.33', connectedAt: new Date().toISOString() }]
  })

  const confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true)

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByText('Gestionnaire fichiers')).toBeInTheDocument()
  expect(screen.getByText('Serveur DLNA')).toBeInTheDocument()
  expect(screen.getByText('uuid:tv-1')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Actualiser clients' }))
  await waitFor(() => expect(client.scanDLNADevices).toHaveBeenCalled())
  expect(screen.getByText('Serveur SMB')).toBeInTheDocument()
  expect(screen.getByText('VLC')).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Actualiser SMB' }))
  await waitFor(() => expect(client.refreshSMBStatus).toHaveBeenCalled())

  fireEvent.change(screen.getByPlaceholderText('Nom dossier'), { target: { value: 'series' } })
  fireEvent.click(screen.getByRole('button', { name: 'Créer dossier' }))
  await waitFor(() => expect(client.createFSDir).toHaveBeenCalled())

  fireEvent.click(screen.getByRole('button', { name: 'Éditer' }))
  await waitFor(() => expect(client.readFSFile).toHaveBeenCalledWith('/tmp/media/film.mp4'))

  fireEvent.change(screen.getByPlaceholderText('/abs/path/source'), { target: { value: '/tmp/media/film.mp4' } })
  fireEvent.change(screen.getByPlaceholderText('/abs/path/destination'), { target: { value: '/tmp/media/film-renamed.mp4' } })
  fireEvent.click(screen.getByRole('button', { name: 'Déplacer' }))
  await waitFor(() => expect(client.moveFSPath).toHaveBeenCalled())

  fireEvent.click(screen.getByRole('button', { name: 'Supprimer' }))
  await waitFor(() => expect(client.deleteFSPath).toHaveBeenCalledWith('/tmp/media/film.mp4', false))

  confirmSpy.mockRestore()
})

test('tolère un statut SMB avec clients null sans écran blanc', async () => {
  const client = createMockClient()
  client.getSettings = vi.fn().mockResolvedValue({
    downloadMaxConcurrent: 3,
    downloadAutoResume: true,
    downloadAutoGroup: true,
    downloadsPath: '/tmp/downloads',
    libraryPaths: ['/tmp/media'],
    globalRateLimitKB: 0,
    aiEnabled: true,
    aiTopK: 5,
    theme: 'clair',
    visibleColumns: ['nom'],
    fileServerAuthEnabled: false,
    fileServerAuthUser: '',
    dlnaEnabled: false,
    smbEnabled: true,
    smbShareName: 'shelfy',
    smbSharePath: '/tmp/media'
  })
  client.getDLNADevices = vi.fn().mockResolvedValue({ enabled: false, devices: [], lastScan: '' })
  client.getSMBStatus = vi.fn().mockResolvedValue({
    enabled: true,
    running: true,
    backend: 'samba',
    shareName: 'shelfy',
    sharePath: '/tmp/media',
    updatedAt: new Date().toISOString(),
    clients: null
  } as any)

  render(
    <APIProvider client={client}>
      <MemoryRouter>
        <SettingsPage />
      </MemoryRouter>
    </APIProvider>
  )

  expect(await screen.findByText('Serveur SMB')).toBeInTheDocument()
  expect(screen.getByText('Aucun client SMB connecté pour le moment.')).toBeInTheDocument()
})
