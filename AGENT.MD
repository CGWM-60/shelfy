# AGENT.MD

## Mission
Maintenir Shelfy v3 en niveau production-ready:
- downloads engine (JDownloader2-like)
- bibliothèque + lecture (Plex-like)
- débrideurs provider-agnostic (Debrid-Link device code)
- IA provider-agnostic (Fake + Mistral)
- file server VLC-friendly

## Règles non négociables
1. Toute modification inclut des tests adaptés (back + front + e2e si parcours).
2. Aucun merge sans CI verte.
3. Aucun IO direct non mockable (HTTP/FS/DB/time via abstractions).
4. API tenue à jour dans `docs/openapi.yaml`.
5. Logs structurés + correlation-id.
6. Tests offline obligatoires via FakeProviders.

## CI bloquante
- `go vet ./...`
- `go test ./... -race`
- `npm run lint --prefix frontend`
- `npm run test --prefix frontend`
- `npm run e2e --prefix frontend`

## Définition DONE
- code compilable
- tests requis passants
- docs/env/openapi mis à jour
- `make ci` passe localement
