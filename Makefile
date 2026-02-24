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
				key=$${key#"$${key%%[![:space:]]*}"}; \
				key=$${key%"$${key##*[![:space:]]}"}; \
				val=$${val%$$'\r'}; \
				val=$${val#"$${val%%[![:space:]]*}"}; \
				val=$${val%"$${val##*[![:space:]]}"}; \
				[[ "$$key" =~ ^[A-Za-z_][A-Za-z0-9_]*$$ ]] || continue; \
				if [[ "$$val" =~ ^\".*\"$$ ]]; then \
					val=$${val:1:$${#val}-2}; \
				elif [[ "$$val" =~ ^'\''.*'\''$$ ]]; then \
					val=$${val:1:$${#val}-2}; \
				else \
					val=$${val%%#*}; \
					val=$${val#"$${val%%[![:space:]]*}"}; \
					val=$${val%"$${val##*[![:space:]]}"}; \
				fi; \
				export "$$key=$$val"; \
			done < .env; \
		}; \
		need_front_install=0; \
		if [ ! -d frontend/node_modules ]; then need_front_install=1; fi; \
		if [ ! -d frontend/node_modules/@xterm/xterm ] || [ ! -d frontend/node_modules/@xterm/addon-fit ]; then need_front_install=1; fi; \
		if [ "$$need_front_install" -eq 1 ]; then \
			echo "Dépendances frontend manquantes: installation..."; \
			npm install --prefix frontend; \
		fi; \
		load_dotenv; \
		api_addr="$${HTTP_ADDR:-:8080}"; \
		api_host="127.0.0.1"; \
		if [[ "$$api_addr" == \[*\]:* ]]; then \
			api_host="$${api_addr%%]*}"; \
			api_host="$${api_host#[}"; \
			api_port="$${api_addr##*:}"; \
		elif [[ "$$api_addr" == *:* && "$$api_addr" != :* ]]; then \
			api_host="$${api_addr%:*}"; \
			api_port="$${api_addr##*:}"; \
		elif [[ "$$api_addr" == :* ]]; then \
			api_port="$${api_addr##*:}"; \
		else \
			api_port="8080"; \
		fi; \
		if [ -z "$$api_host" ] || [ "$$api_host" = "0.0.0.0" ] || [ "$$api_host" = "::" ] || [ "$$api_host" = "[::]" ]; then \
			api_host="127.0.0.1"; \
		fi; \
		export VITE_PROXY_TARGET="http://$${api_host}:$${api_port}"; \
		echo "Proxy frontend API: $$VITE_PROXY_TARGET"; \
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
