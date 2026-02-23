import { test, expect, type Page, type Route } from '@playwright/test'

type Download = {
  id: string
  sourceLink: string
  directLink: string
  fileName: string
  status: string
  downloadedBytes: number
  sizeBytes: number
  speedBytes: number
  etaSeconds: number
  useDebrid: boolean
}

async function installMockAPI(page: Page, options?: { authEnabled?: boolean }) {
  let seq = 0
  const authEnabled = options?.authEnabled ?? false
  let authenticated = !authEnabled
  const downloads: Download[] = []
  const debridAccounts: Array<{ id: number; provider: string; label: string; isActive: boolean; isDefault: boolean; authType: string }> = []
  const media = [
    { id: 'm1', title: 'Film A', kind: 'video', path: '/tmp/a.mp4' },
    { id: 'm2', title: 'Image A', kind: 'image', path: '/tmp/a.jpg' },
    { id: 'm3', title: 'PDF A', kind: 'pdf', path: '/tmp/a.pdf' }
  ]
  const progress = new Map<string, number>()
  const settings = {
    downloadMaxConcurrent: 3,
    downloadAutoResume: true,
    downloadsPath: '/tmp/downloads',
    libraryPaths: ['/tmp/media'],
    globalRateLimitKB: 0,
    aiEnabled: true,
    aiTopK: 5,
    theme: 'clair',
    visibleColumns: ['nom', 'etat'],
    fileServerAuthEnabled: false,
    fileServerAuthUser: '',
    dlnaEnabled: false,
    smbEnabled: false,
    smbShareName: 'shelfy',
    smbSharePath: '/tmp/media'
  }
  const dlnaDevices = [
    {
      usn: 'uuid:livingroom-tv',
      st: 'urn:schemas-upnp-org:device:MediaRenderer:1',
      server: 'DLNA/1.5',
      location: 'http://192.168.1.44:8200/device.xml',
      address: '192.168.1.44:1900',
      lastSeenAt: new Date().toISOString()
    }
  ]
  const fsRoots = ['/tmp/downloads', '/tmp/media']
  const fsItemsByPath = new Map<string, Array<{ name: string; path: string; isDir: boolean; sizeBytes: number; modifiedAt: string }>>()
  const fsFiles = new Map<string, string>()
  const nowISO = () => new Date().toISOString()
  fsItemsByPath.set('/tmp/downloads', [])
  fsItemsByPath.set('/tmp/media', [{ name: 'notes.txt', path: '/tmp/media/notes.txt', isDir: false, sizeBytes: 7, modifiedAt: nowISO() }])
  fsFiles.set('/tmp/media/notes.txt', 'bonjour')

  const parentPath = (value: string) => {
    const normalized = value.replace(/\/+$/, '')
    const idx = normalized.lastIndexOf('/')
    if (idx <= 0) return normalized
    return normalized.slice(0, idx)
  }
  const baseName = (value: string) => value.split('/').filter(Boolean).pop() ?? value
  const ensureFolder = (folderPath: string) => {
    if (!fsItemsByPath.has(folderPath)) fsItemsByPath.set(folderPath, [])
  }
  const addChild = (folderPath: string, item: { name: string; path: string; isDir: boolean; sizeBytes: number; modifiedAt: string }) => {
    ensureFolder(folderPath)
    const list = fsItemsByPath.get(folderPath)!
    const idx = list.findIndex((entry) => entry.path === item.path)
    if (idx >= 0) {
      list[idx] = item
      return
    }
    list.push(item)
  }
  const removeChild = (folderPath: string, targetPath: string) => {
    const list = fsItemsByPath.get(folderPath) ?? []
    const idx = list.findIndex((entry) => entry.path === targetPath)
    if (idx >= 0) list.splice(idx, 1)
  }
  const removePathRecursive = (targetPath: string) => {
    for (const [folder, list] of fsItemsByPath.entries()) {
      fsItemsByPath.set(folder, list.filter((entry) => !(entry.path === targetPath || entry.path.startsWith(`${targetPath}/`))))
    }
    for (const key of Array.from(fsItemsByPath.keys())) {
      if (key === targetPath || key.startsWith(`${targetPath}/`)) {
        fsItemsByPath.delete(key)
      }
    }
    for (const key of Array.from(fsFiles.keys())) {
      if (key === targetPath || key.startsWith(`${targetPath}/`)) {
        fsFiles.delete(key)
      }
    }
  }

  await page.route('**/*', async (route) => {
    const req = route.request()
    const url = new URL(req.url())
    const path = url.pathname
    const method = req.method()
    if (!path.startsWith('/api/') && !path.startsWith('/files/')) {
      await route.continue()
      return
    }

    const json = (data: unknown, status = 200) =>
      route.fulfill({ status, contentType: 'application/json', body: JSON.stringify({ data }) })

    if (path === '/api/health' && method === 'GET') return json({ status: 'ok' })
    if (path === '/api/auth/status' && method === 'GET') return json({ enabled: authEnabled, authenticated, username: authenticated ? 'admin' : '' })
    if (path === '/api/auth/login' && method === 'POST') {
      const payload = req.postDataJSON() as { username?: string; password?: string }
      if (payload.username === 'admin' && payload.password === 'secret') {
        authenticated = true
        return json({ authenticated: true, username: 'admin' })
      }
      return route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ error: 'invalid credentials' }) })
    }
    if (path === '/api/auth/logout' && method === 'POST') {
      authenticated = false
      return json({ done: true })
    }
    if (authEnabled && !authenticated) {
      return route.fulfill({ status: 401, contentType: 'application/json', body: JSON.stringify({ error: 'unauthorized' }) })
    }

    if (path === '/api/downloads' && method === 'GET') return json(downloads)
    if (path === '/api/downloads' && method === 'POST') {
      const payload = req.postDataJSON() as { links: string[]; useDebrid: boolean }
      const created: Download[] = []
      for (const rawLink of payload.links) {
        if (rawLink.startsWith('fake:folder:')) {
          const count = Number(rawLink.split(':')[2])
          for (let i = 1; i <= count; i += 1) {
            created.push({
              id: `d-${++seq}`,
              sourceLink: rawLink,
              directLink: `https://files.test/file-${i}.bin`,
              fileName: `file-${i}.bin`,
              status: 'queued',
              downloadedBytes: 0,
              sizeBytes: 4096,
              speedBytes: 0,
              etaSeconds: 0,
              useDebrid: payload.useDebrid
            })
          }
        } else {
          created.push({
            id: `d-${++seq}`,
            sourceLink: rawLink,
            directLink: rawLink,
            fileName: rawLink.split('/').pop() ?? `job-${seq}.bin`,
            status: 'queued',
            downloadedBytes: 0,
            sizeBytes: 4096,
            speedBytes: 0,
            etaSeconds: 0,
            useDebrid: payload.useDebrid
          })
        }
      }
      downloads.unshift(...created)
      return json(created, 201)
    }

    const actionMatch = path.match(/^\/api\/downloads\/([^/]+)\/(start|pause|resume)$/)
    if (actionMatch && method === 'POST') {
      const [, id, action] = actionMatch
      const found = downloads.find((job) => job.id === id)
      if (found) {
        if (action === 'start' || action === 'resume') {
          found.status = 'running'
          found.downloadedBytes = 1024
          found.speedBytes = 512
        }
        if (action === 'pause') {
          found.status = 'paused'
          found.speedBytes = 0
        }
      }
      return route.fulfill({ status: 204, body: '' })
    }

    const deleteMatch = path.match(/^\/api\/downloads\/([^/]+)$/)
    if (deleteMatch && method === 'DELETE') {
      const id = deleteMatch[1]
      const idx = downloads.findIndex((job) => job.id === id)
      if (idx >= 0) downloads.splice(idx, 1)
      return route.fulfill({ status: 204, body: '' })
    }

    if (path === '/api/debrid/providers') {
      return json([{ name: 'fake', authType: 'oauth2_device', supportsPassword: true }])
    }
    if (path === '/api/debrid/accounts' && method === 'GET') {
      return json(debridAccounts)
    }
    if (path === '/api/debrid/accounts' && method === 'POST') {
      const payload = req.postDataJSON() as { provider: string; label: string; isActive: boolean; isDefault: boolean }
      const item = { id: debridAccounts.length + 1, provider: payload.provider, label: payload.label, isActive: payload.isActive, isDefault: payload.isDefault, authType: 'none' }
      debridAccounts.push(item)
      return json(item, 201)
    }

    const patchAccount = path.match(/^\/api\/debrid\/accounts\/(\d+)$/)
    if (patchAccount && method === 'PATCH') {
      const id = Number(patchAccount[1])
      const payload = req.postDataJSON() as { isActive?: boolean; isDefault?: boolean }
      const account = debridAccounts.find((it) => it.id === id)
      if (account) {
        if (payload.isActive !== undefined) account.isActive = payload.isActive
        if (payload.isDefault !== undefined) account.isDefault = payload.isDefault
      }
      return json(account)
    }
    const deleteAccount = path.match(/^\/api\/debrid\/accounts\/(\d+)$/)
    if (deleteAccount && method === 'DELETE') {
      const id = Number(deleteAccount[1])
      const idx = debridAccounts.findIndex((it) => it.id === id)
      if (idx >= 0) debridAccounts.splice(idx, 1)
      return route.fulfill({ status: 204, body: '' })
    }
    const startAuth = path.match(/^\/api\/debrid\/accounts\/(\d+)\/auth\/start$/)
    if (startAuth && method === 'POST') {
      return json({ sessionId: `sess-${startAuth[1]}`, deviceCode: 'device-1', userCode: 'ABCD-1234', verificationUri: 'https://fake.local/device', intervalSec: 1, expiresIn: 600 })
    }
    const pollAuth = path.match(/^\/api\/debrid\/accounts\/(\d+)\/auth\/poll$/)
    if (pollAuth && method === 'POST') {
      return json({ done: true })
    }
    const passwordAuth = path.match(/^\/api\/debrid\/accounts\/(\d+)\/auth\/password$/)
    if (passwordAuth && method === 'POST') {
      return json({ done: true })
    }
    const statusAuth = path.match(/^\/api\/debrid\/accounts\/(\d+)\/status$/)
    if (statusAuth && method === 'GET') {
      return json({ status: 'ok' })
    }

    if (path === '/api/media/scan' && method === 'POST') return json({ count: media.length })
    if (path === '/api/media' && method === 'GET') return json(media)

    const mediaMatch = path.match(/^\/api\/media\/([^/]+)$/)
    if (mediaMatch && method === 'GET') {
      const id = mediaMatch[1]
      const item = media.find((row) => row.id === id)
      return json({ item, progress: { mediaId: id, positionMs: progress.get(id) ?? 0 } })
    }

    const progressMatch = path.match(/^\/api\/media\/([^/]+)\/progress$/)
    if (progressMatch && method === 'POST') {
      const id = progressMatch[1]
      const payload = req.postDataJSON() as { positionMs: number }
      progress.set(id, payload.positionMs > 0 ? payload.positionMs : 5000)
      return route.fulfill({ status: 204, body: '' })
    }

    if (path === '/api/ai/ask' && method === 'POST') {
      return json({ answer: 'Réponse IA', sources: [{ mediaId: 'm1', title: 'Film A', snippet: '...', score: 0.8 }] })
    }

    if (path === '/api/ai/status' && method === 'GET') return json({ embeddingProvider: 'fake', llmProvider: 'fake' })
    if (path === '/api/ai/report' && method === 'GET') {
      return json({
        embeddingProvider: 'fake',
        llmProvider: 'fake',
        startedAt: new Date().toISOString(),
        uptimeSec: 120,
        indexRuns: 2,
        lastIndexedCount: 3,
        indexedTotal: 6,
        searchRuns: 5,
        searchAvgLatencyMs: 11,
        lastSearchLatencyMs: 9,
        lastSearchResults: 4,
        askRuns: 3,
        askSearchOnlyRuns: 1,
        askAvgLatencyMs: 18,
        lastAskLatencyMs: 17,
        lastAskSources: 3,
        embeddingCalls: 8,
        embeddingTokensEstimated: 500,
        llmPromptTokensEstimated: 190,
        llmCompletionTokensEstimated: 95
      })
    }
    if (path === '/api/ai/search' && method === 'POST') return json([])
    if (path === '/api/ai/index' && method === 'POST') return json({ indexed: 1 })
    if (path === '/api/settings' && method === 'GET') return json(settings)
    if (path === '/api/settings' && method === 'PUT') {
      Object.assign(settings, req.postDataJSON())
      return json(settings)
    }
    if (path === '/api/dlna/devices' && method === 'GET') return json({ enabled: settings.dlnaEnabled, devices: settings.dlnaEnabled ? dlnaDevices : [], lastScan: new Date().toISOString() })
    if (path === '/api/dlna/scan' && method === 'POST') return json({ enabled: settings.dlnaEnabled, devices: settings.dlnaEnabled ? dlnaDevices : [], lastScan: new Date().toISOString() })
    if (path === '/api/smb/status' && method === 'GET') {
      return json({
        enabled: settings.smbEnabled,
        running: settings.smbEnabled,
        backend: 'fake',
        shareName: settings.smbShareName,
        sharePath: settings.smbSharePath,
        updatedAt: new Date().toISOString(),
        clients: settings.smbEnabled ? [{ address: '192.168.1.60', machine: 'VLC', username: 'guest', connectedAt: new Date().toISOString() }] : []
      })
    }
    if (path === '/api/smb/refresh' && method === 'POST') {
      return json({
        enabled: settings.smbEnabled,
        running: settings.smbEnabled,
        backend: 'fake',
        shareName: settings.smbShareName,
        sharePath: settings.smbSharePath,
        updatedAt: new Date().toISOString(),
        clients: settings.smbEnabled ? [{ address: '192.168.1.60', machine: 'VLC', username: 'guest', connectedAt: new Date().toISOString() }] : []
      })
    }
    if (path === '/api/fs/roots' && method === 'GET') return json({ roots: fsRoots })
    if (path === '/api/fs/list' && method === 'GET') {
      const listingPath = url.searchParams.get('path') || fsRoots[0]
      ensureFolder(listingPath)
      return json({ path: listingPath, items: fsItemsByPath.get(listingPath) ?? [] })
    }
    if (path === '/api/fs/file' && method === 'GET') {
      const filePath = url.searchParams.get('path') || ''
      return json({ path: filePath, content: fsFiles.get(filePath) ?? '' })
    }
    if (path === '/api/fs/mkdir' && method === 'POST') {
      const payload = req.postDataJSON() as { path: string }
      const dirPath = payload.path
      ensureFolder(dirPath)
      addChild(parentPath(dirPath), { name: baseName(dirPath), path: dirPath, isDir: true, sizeBytes: 0, modifiedAt: nowISO() })
      return route.fulfill({ status: 204, body: '' })
    }
    if (path === '/api/fs/file' && method === 'PUT') {
      const payload = req.postDataJSON() as { path: string; content: string }
      fsFiles.set(payload.path, payload.content ?? '')
      addChild(parentPath(payload.path), { name: baseName(payload.path), path: payload.path, isDir: false, sizeBytes: (payload.content ?? '').length, modifiedAt: nowISO() })
      return route.fulfill({ status: 204, body: '' })
    }
    if (path === '/api/fs/move' && method === 'PATCH') {
      const payload = req.postDataJSON() as { fromPath: string; toPath: string }
      const source = payload.fromPath
      const target = payload.toPath
      let movedEntry: { name: string; path: string; isDir: boolean; sizeBytes: number; modifiedAt: string } | null = null
      for (const list of fsItemsByPath.values()) {
        const found = list.find((entry) => entry.path === source)
        if (found) {
          movedEntry = { ...found }
          break
        }
      }
      removeChild(parentPath(source), source)
      if (movedEntry) {
        movedEntry.path = target
        movedEntry.name = baseName(target)
        movedEntry.modifiedAt = nowISO()
        addChild(parentPath(target), movedEntry)
      }
      if (fsFiles.has(source)) {
        const content = fsFiles.get(source) ?? ''
        fsFiles.delete(source)
        fsFiles.set(target, content)
      }
      if (fsItemsByPath.has(source)) {
        const children = fsItemsByPath.get(source) ?? []
        fsItemsByPath.delete(source)
        fsItemsByPath.set(target, children.map((entry) => ({
          ...entry,
          path: entry.path.replace(`${source}/`, `${target}/`)
        })))
      }
      return route.fulfill({ status: 204, body: '' })
    }
    if (path === '/api/fs/item' && method === 'DELETE') {
      const targetPath = url.searchParams.get('path') || ''
      removeChild(parentPath(targetPath), targetPath)
      removePathRecursive(targetPath)
      return route.fulfill({ status: 204, body: '' })
    }

    if (path.startsWith('/files/')) {
      if (path.endsWith('/stream')) {
        return route.fulfill({ status: 206, headers: { 'Accept-Ranges': 'bytes', 'Content-Range': 'bytes 0-8/9' }, contentType: 'video/mp4', body: Buffer.from('fake-media') })
      }
      if (path.endsWith('/download')) {
        const mediaID = path.split('/')[2]
        if (mediaID === 'm2') {
          return route.fulfill({ status: 200, contentType: 'image/jpeg', body: Buffer.from('fake-image') })
        }
        if (mediaID === 'm3') {
          return route.fulfill({ status: 200, contentType: 'application/pdf', body: Buffer.from('%PDF-1.4') })
        }
        return route.fulfill({ status: 200, contentType: 'application/octet-stream', body: Buffer.from('download') })
      }
    }

    return route.fulfill({ status: 404, body: 'not mocked' })
  })
}

test('ajout download débridé dossier puis start/pause/resume/delete', async ({ page }) => {
  await installMockAPI(page)
  page.on('dialog', (dialog) => dialog.accept())
  await page.goto('/')

  await page.getByPlaceholder('Collez un ou plusieurs liens').fill('fake:folder:2:https://files.test')
  await page.getByLabel('Utiliser un débrideur').check()
  await page.getByRole('button', { name: 'Ajouter' }).click()

  await expect(page.getByText('file-1.bin')).toBeVisible()
  await expect(page.getByText('file-2.bin')).toBeVisible()

  await page.getByRole('button', { name: 'Start' }).first().click()
  await expect(page.getByText('running').first()).toBeVisible()

  await page.getByRole('button', { name: 'Pause' }).first().click()
  await expect(page.getByText('paused').first()).toBeVisible()

  await page.getByRole('button', { name: 'Resume' }).first().click()
  await expect(page.getByText('running').first()).toBeVisible()

  await page.getByRole('button', { name: 'Supprimer' }).first().click()
  await expect(page.getByText('file-1.bin')).toHaveCount(0)
})

test('scan bibliothèque, play, save progress, refresh, reprise', async ({ page }) => {
  await installMockAPI(page)
  await page.goto('/library')

  await page.getByRole('button', { name: 'Scanner la bibliothèque' }).click()
  await page.getByRole('link', { name: 'Film A' }).click()

  await page.evaluate(() => {
    const video = document.querySelector('video')
    if (video) {
      ;(video as HTMLVideoElement).currentTime = 5
      video.dispatchEvent(new Event('timeupdate', { bubbles: true }))
    }
  })

  await page.getByRole('button', { name: 'Sauvegarder position' }).click()
  await page.reload()
  await expect(page.getByText('Position: 5000 ms')).toBeVisible()
})

test('Ask AI overlay depuis fiche média avec navigation vers source', async ({ page }) => {
  await installMockAPI(page)
  await page.goto('/media/m1')
  await page.getByRole('button', { name: 'Ask AI about this' }).click()
  await expect(page.getByTestId('ask-ai-chat-window')).toBeVisible()
  await expect(page.getByTestId('ask-ai-chat-window').getByPlaceholder('Posez une question')).toHaveValue('Parle-moi de Film A')
  await page.getByRole('button', { name: 'Envoyer' }).click()

  await expect(page.getByText('Réponse IA')).toBeVisible()
  await page.getByTestId('ask-ai-chat-window').getByRole('link', { name: 'Film A' }).click()
  await expect(page).toHaveURL(/\/media\/m1$/)
  await expect(page.getByText('Film A')).toBeVisible()
})

test('overlay Ask AI accessible depuis toutes les pages', async ({ page }) => {
  await installMockAPI(page)
  await page.goto('/library')

  await page.getByRole('button', { name: 'Ouvrir Ask AI' }).click()
  await expect(page.getByTestId('ask-ai-chat-window')).toBeVisible()
  await page.getByPlaceholder('Posez une question').fill('Trouve un film')
  await page.getByRole('button', { name: 'Envoyer' }).click()

  await expect(page.getByText('Réponse IA')).toBeVisible()
  await page.getByTestId('ask-ai-chat-window').getByRole('link', { name: 'Film A' }).click()
  await expect(page).toHaveURL(/\/media\/m1$/)
})

test('connecter debrid via code puis utiliser au download', async ({ page }) => {
  await installMockAPI(page)
  page.on('dialog', (dialog) => dialog.accept())
  await page.goto('/debrid')

  await page.getByPlaceholder('Label').fill('Compte fake')
  await page.getByRole('button', { name: 'Ajouter compte' }).click()
  await expect(page.getByText('Compte fake')).toBeVisible()
  await page.getByRole('button', { name: 'Connecter Debrid-Link' }).click()
  await expect(page.getByText(/ABCD-1234/)).toBeVisible()
  await page.getByRole('button', { name: "J'ai validé" }).click()
  await expect(page.getByText('Connexion validée')).toBeVisible()

  await page.goto('/')
  await page.getByPlaceholder('Collez un ou plusieurs liens').fill('fake:direct:https://files.test/one.bin')
  await page.getByLabel('Utiliser un débrideur').check()
  await page.getByRole('button', { name: 'Ajouter' }).click()

  await expect(page.getByText('one.bin')).toBeVisible()

  await page.goto('/debrid')
  await page.getByRole('button', { name: 'Supprimer compte' }).click()
  await expect(page.getByText('Compte fake')).toHaveCount(0)
})

test('viewer image et pdf', async ({ page }) => {
  await installMockAPI(page)
  await page.goto('/library')

  await page.getByRole('link', { name: 'Image A' }).click()
  await expect(page.locator('.image-viewer img')).toBeVisible()
  await page.goto('/library')
  await page.getByRole('link', { name: 'PDF A' }).click()
  await expect(page.locator('iframe.pdf-viewer')).toBeVisible()
})

test('page rapport IA affiche les métriques', async ({ page }) => {
  await installMockAPI(page)
  await page.goto('/ai/report')

  await expect(page.getByRole('heading', { name: 'Rapport IA' })).toBeVisible()
  await expect(page.getByText('Tokens embeddings')).toBeVisible()
  await expect(page.getByText('500')).toBeVisible()
})

test('paramètres: gestionnaire fichiers create/edit/move/delete', async ({ page }) => {
  await installMockAPI(page)
  page.on('dialog', (dialog) => dialog.accept())
  await page.goto('/settings')

  await expect(page.getByText('Gestionnaire fichiers')).toBeVisible()
  await page.getByLabel('DLNA activé').check()
  await page.getByRole('button', { name: /^Enregistrer$/ }).click()
  await page.getByRole('button', { name: 'Actualiser clients' }).click()
  await expect(page.getByText('uuid:livingroom-tv')).toBeVisible()
  await page.getByLabel('Racine / dossier courant').selectOption('/tmp/media')

  await page.getByPlaceholder('Nom dossier').fill('series')
  await page.getByRole('button', { name: 'Créer dossier' }).click()
  await expect(page.getByText('Dossier créé')).toBeVisible()

  await page.getByRole('button', { name: 'Éditer' }).first().click()
  await expect(page.getByPlaceholder('/tmp/media/notes.txt')).toHaveValue('/tmp/media/notes.txt')
  await page.getByRole('button', { name: 'Enregistrer fichier' }).click()
  await expect(page.getByText('Fichier enregistré')).toBeVisible()

  await page.getByPlaceholder('/abs/path/source').fill('/tmp/media/notes.txt')
  await page.getByPlaceholder('/abs/path/destination').fill('/tmp/media/notes-renamed.txt')
  await page.getByRole('button', { name: 'Déplacer' }).click()
  await expect(page.getByText('Déplacement effectué')).toBeVisible()

  await page.getByRole('button', { name: 'Supprimer' }).first().click()
  await expect(page.getByText('Suppression effectuée')).toBeVisible()
})

test('auth activée: login requis puis accès dashboard', async ({ page }) => {
  await installMockAPI(page, { authEnabled: true })
  await page.goto('/')

  await expect(page.getByRole('heading', { name: 'Se connecter' })).toBeVisible()
  await page.getByLabel('Identifiant').fill('admin')
  await page.getByLabel('Mot de passe').fill('secret')
  await page.getByRole('button', { name: 'Connexion' }).click()

  await expect(page.getByTestId('downloads-page')).toBeVisible()
})
