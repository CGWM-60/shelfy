---
name: shelfy-v3-delivery
description: Développer Shelfy v3 (downloads + media + debrid + ai + fileserver) avec workflow tests-first strict, offline-compatible via fake providers.
---

# Workflow
1. Implémenter en vertical slice.
2. Ajouter/mettre à jour les tests de la slice.
3. Vérifier localement:
   - `go vet ./...`
   - `go test ./... -race`
   - `npm run lint --prefix frontend`
   - `npm run test --prefix frontend`
   - `npm run e2e --prefix frontend`
4. Mettre à jour `README.md` et `docs/openapi.yaml` si API/config changent.

# Points d'attention
- Debrid-Link: device code flow (`/api/oauth/device/code` puis `/api/oauth/token` poll).
- IA: providers agnostiques + Mistral activé par env + Fake offline.
- File server: endpoints `/files/` + stream Range + auth optionnelle.
- UI FR friendly: états vides/loading/error, confirmations, responsive.
- Config: priorité `env > config file > defaults`.
