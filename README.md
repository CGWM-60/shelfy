# Shelfy v3

Monorepo production-ready Go + React pour une app:
- JDownloader2-like (queue, multi-liens, pause/reprise, retry, reprise après redémarrage)
- Plex-like (scan bibliothèque, lecture vidéo/audio/image/pdf, progression)
- Débrideurs provider-agnostic (focus Debrid-Link device code)
- IA provider-agnostic (Fake offline + Mistral embeddings/Q&A)
- File server VLC-friendly (`/files/`, stream Range, download)

## Stack
- Backend: Go 1.22+, `chi`, `gorm`, SQLite dev, MariaDB prod
- Frontend: React + TypeScript + Vite
- Tests: Go unit/HTTP/intégration + race, Vitest/RTL, Playwright
- API doc: `docs/openapi.yaml`

## Démarrage
Premier lancement:
```bash
make deps
make dev
```

Ensuite (commande unique API + worker + frontend):
```bash
make dev
```
Le frontend Vite proxy automatiquement `/api` et `/files` vers `http://localhost:8080` en local.
Par défaut, `make dev` expose aussi le front sur le réseau (`0.0.0.0:5173`), donc accès via `http://IP_DU_SERVEUR:5173`.
Tu peux surcharger avec:
```bash
make dev FRONTEND_HOST=0.0.0.0 FRONTEND_PORT=4173
```

Mode production (Docker Compose Dokploy):
```bash
make prod
```
Arrêt:
```bash
make prod-down
```
Logs:
```bash
make prod-logs
```

## Configuration (priorité)
1. Variables d'env
2. Fichier `config.json` (ou `CONFIG_FILE=/chemin/config.json`)
3. Valeurs par défaut

Variables importantes:
- `HTTP_ADDR`, `DB_DRIVER`, `DB_DSN`
- `STORAGE_PATH`, `MEDIA_PATH` ou `MEDIA_PATHS`
- `DOWNLOAD_MAX_CONCURRENT`, `DOWNLOAD_AUTO_RESUME`
- `MISTRAL_API_KEY`, `MISTRAL_EMBED_MODEL`, `MISTRAL_CHAT_MODEL`, `AI_ENABLED`, `AI_TOP_K`
- `DEBRIDLINK_API_BASE`, `DEBRIDLINK_OAUTH_CLIENT_ID`, `DEBRIDLINK_OAUTH_CLIENT_SECRET`, `DEBRIDLINK_OAUTH_SCOPE`
- `FILESERVER_AUTH_USER`, `FILESERVER_AUTH_PASS`, `FILESERVER_ENABLED`
- `DLNA_ENABLED`, `DLNA_BASE_URL` (serveur UPnP/DLNA; `DLNA_BASE_URL` force l'URL annoncée en SSDP)
- `SMB_ENABLED`, `SMB_SHARE_NAME`, `SMB_SHARE_PATH`, `SMB_BINARY`, `SMB_STATUS_BINARY`
- `AUTH_ENABLED`, `AUTH_USER`, `AUTH_PASS`, `AUTH_SESSION_SECRET`, `AUTH_SESSION_TTL_HOURS`

Note Debrid-Link: `DEBRIDLINK_OAUTH_CLIENT_ID` et `DEBRIDLINK_OAUTH_CLIENT_SECRET` identifient l'application OAuth.  
Le login/mot de passe utilisateur (OAuth password grant) se saisit dans l'UI Débrideurs.

## Commandes qualité
```bash
make lint
make test-back
make test-front
make e2e
make ci
```

## Endpoints clés
- Downloads: `/api/downloads*` (inclut `destinationDir`), `/api/events`
- Auth: `/api/health`, `/api/auth/status`, `/api/auth/login`, `/api/auth/logout`
- Debrid: `/api/debrid/providers`, `/api/debrid/accounts*`, `/api/debrid/accounts/{id}/auth/start|poll`
- Media: `/api/media*`, `/api/stream/{id}`, `/api/subtitles/{id}`
- File server: `/files/`, `/files/{id}/stream`, `/files/{id}/download`
- IA: `/api/ai/index|search|ask|status|report`
- Settings: `/api/settings`
- DLNA status/UI: `/api/dlna/devices`, `/api/dlna/scan`
- DLNA debug: `/api/dlna/debug`
- SMB status/UI: `/api/smb/status`, `/api/smb/refresh`
- DLNA server: `/dlna/device.xml`, `/dlna/scpd/content_directory.xml`, `/dlna/scpd/connection_manager.xml`, `/dlna/control/content_directory`, `/dlna/control/connection_manager`, `/dlna/media/{id}`
- UPnP aliases: `/upnp/device.xml`, `/upnp/scpd/content_directory.xml`, `/upnp/scpd/connection_manager.xml`, `/upnp/control/content_directory`, `/upnp/control/connection_manager`, `/upnp/media/{id}`
- Gestion fichiers: `/api/fs/roots|list|file|mkdir|move|item`

## Offline tests
Tous les tests sont exécutables sans internet grâce aux providers fake:
- `internal/debrid/providers/fake`
- `internal/ai/fake_provider.go`

## Déploiement QNAP (Container Station)
Un pack de déploiement est fourni dans `deploy/qnap`.

Prérequis:
- Container Station installé sur le NAS
- Un partage pour la persistance (ex: `/share/Container/shelfy/data`)
- Un partage média monté en lecture seule (ex: `/share/Media`)

### 1) Préparer l'env QNAP
```bash
cd deploy/qnap
# un fichier prêt à l'emploi existe déjà: .env.qnap
# (sinon: cp .env.qnap.example .env.qnap)
```

Édite `.env.qnap`:
- `SHELFY_SRC_PATH=/share/Container/shelfy` (chemin du repo complet sur le NAS)
- `DLNA_BASE_URL=http://IP_DU_NAS:8080`
- clés API (`MISTRAL_API_KEY`, Debrid-Link) si besoin

### 2) Lancer les conteneurs
Depuis SSH sur le NAS (ou via projet Compose dans Container Station):
```bash
cd /share/Container/shelfy/deploy/qnap
docker compose -f docker-compose.qnap.yml up -d --build
```

Option "images déjà buildées/importées" (sans `.env`, variables en dur dans le YAML):
```bash
cd /share/Container/shelfy/deploy/qnap
docker compose -f docker-compose.qnap.images.yml up -d
```
Ce mode utilise les images locales `shelfy-api:latest`, `shelfy-web:latest`, `shelfy-worker:latest`.

Services:
- UI web: `http://IP_DU_NAS:8080`
- API interne: `127.0.0.1:18080` (sur le NAS, derrière le reverse-proxy Nginx)

Important: `api` ne sert pas l'UI React à `/` (404 normal).  
L'UI est servie par le service `web` sur le port web.

### 3) Volumes
Dans `docker-compose.qnap.yml`:
- `/share/Container/shelfy/data:/data`
- `/share/Media:/media:ro`

Adapte ces chemins à ton NAS.

### 4) Vérifier DLNA/UPnP
```bash
curl -s http://IP_DU_NAS:8080/api/dlna/debug
curl -s http://IP_DU_NAS:8080/dlna/device.xml | grep -E "URLBase|ConnectionManager|ContentDirectory"
```

Si VLC ne voit toujours pas le serveur:
- vérifier `DLNA_BASE_URL`
- redémarrer VLC
- vérifier que le NAS et le client VLC sont sur le même sous-réseau

## Déploiement Dokploy
Un compose dédié est fourni: `deploy/dokploy/docker-compose.dokploy.yml`.

Lancement:
```bash
cd deploy/dokploy
docker compose -f docker-compose.dokploy.yml up -d --build
```

Services:
- `web` exposé sur le port `3000`
- `api` interne sur `:8080`
- `worker` pour le moteur de téléchargement

Important sécurité:
- le compose Dokploy active l'auth session (`AUTH_ENABLED=true`)
- change immédiatement `AUTH_USER`, `AUTH_PASS` et `AUTH_SESSION_SECRET`
