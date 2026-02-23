.PHONY: deps dev dev-api dev-worker test-back test-front e2e lint test ci prod prod-down prod-logs

FRONTEND_HOST ?= 0.0.0.0
FRONTEND_PORT ?= 5173
PROD_COMPOSE_FILE ?= deploy/dokploy/docker-compose.dokploy.yml

deps:
	go mod tidy
	npm install --prefix frontend

dev:
	@bash -c 'set -euo pipefail; \
		cleaned=0; \
		load_dotenv(){ \
			[ -f .env ] || return 0; \
			while IFS= read -r line || [ -n "$$line" ]; do \
				case "$$line" in ""|\#*) continue ;; esac; \
				case "$$line" in *=*) ;; *) continue ;; esac; \
				key=$${line%%=*}; \
				val=$${line#*=}; \
				key=$${key##[[:space:]]}; \
				key=$${key%%[[:space:]]}; \
				[[ "$$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$$ ]] || continue; \
				if [[ "$$val" =~ ^\".*\"$$ ]]; then val=$${val:1:$${#val}-2}; fi; \
				if [[ "$$val" =~ ^'\''.*'\''$$ ]]; then val=$${val:1:$${#val}-2}; fi; \
				export "$$key=$$val"; \
			done < .env; \
		}; \
		if [ ! -d frontend/node_modules ]; then \
			echo "frontend/node_modules absent: installation des dépendances..."; \
			$(MAKE) deps; \
		fi; \
		load_dotenv; \
		go run ./cmd/api & api_pid=$$!; \
		go run ./cmd/worker & worker_pid=$$!; \
		npm run dev --prefix frontend -- --host $(FRONTEND_HOST) --port $(FRONTEND_PORT) & front_pid=$$!; \
		cleanup(){ \
			[ "$$cleaned" -eq 0 ] || return 0; \
			cleaned=1; \
			echo ""; \
			echo "Arrêt des services..."; \
			kill $$api_pid $$worker_pid $$front_pid 2>/dev/null || true; \
			wait $$api_pid $$worker_pid $$front_pid 2>/dev/null || true; \
		}; \
		trap "cleanup; exit 0" INT TERM; \
		trap cleanup EXIT; \
		wait $$api_pid $$worker_pid $$front_pid'

dev-api:
	go run ./cmd/api

dev-worker:
	go run ./cmd/worker

test-back:
	go test ./... -race

test-front:
	npm run test --prefix frontend

e2e:
	npm run e2e --prefix frontend

lint:
	go vet ./...
	npm run lint --prefix frontend

test: lint test-back test-front

ci: lint test-back test-front e2e

prod:
	docker compose -f $(PROD_COMPOSE_FILE) up -d --build

prod-down:
	docker compose -f $(PROD_COMPOSE_FILE) down --remove-orphans

prod-logs:
	docker compose -f $(PROD_COMPOSE_FILE) logs -f --tail=200
