# =============================================================================
# Sytue One — Makefile raíz (orquestación del monorepo)
#
# Stack ÚNICO: Go + Astro/Preact + PostgreSQL (+ S3 para blobs). 100 % UE / Scaleway.
# Fuente de verdad: docs/CONVENTIONS.md. Sin Redis/Valkey, sin motores AGPL.
# Convenciones: dev = Docker Compose local (*.sytue.localhost); prod = Scaleway
# (Terraform + Helm/Kapsule). Sin secretos en el Makefile.
# =============================================================================

# --- Rutas ------------------------------------------------------------------
ROOT_DIR     := $(abspath $(dir $(lastword $(MAKEFILE_LIST))))
COMPOSE_DIR  := $(ROOT_DIR)/deploy/compose
HELM_DIR     := $(ROOT_DIR)/deploy/helm
FRONTEND_DIR := $(ROOT_DIR)/apps/frontend
SERVICES_DIR := $(ROOT_DIR)/services

# --- Parámetros -------------------------------------------------------------
# Entorno de despliegue: dev | staging | prod
ENV          ?= dev
# Release Helm (umbrella chart)
HELM_RELEASE ?= sytue-one
HELM_CHART   := $(HELM_DIR)/sytue-one
HELM_VALUES  := $(HELM_CHART)/values-$(ENV).yaml
HELM_OFFLINE := HELM_REPOSITORY_CONFIG="$(HELM_DIR)/.vendor/no-repositories.yaml" HELM_REPOSITORY_CACHE="$(HELM_DIR)/.vendor/repository-cache"
# Namespace de la plataforma
HELM_NS      ?= sytue-system

# Servicio Go concreto para `migrate` (services/<SVC>): id|drive|docs|mail|...
SVC          ?=

COMPOSE      := docker compose

.DEFAULT_GOAL := help

# =============================================================================
# Ayuda y Asistente de Despliegue
# =============================================================================
.PHONY: help
help: ## Muestra esta ayuda
	@echo "Sytue Suite — targets disponibles:"
	@echo ""
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
		| sort \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-16s\033[0m %s\n", $$1, $$2}'
	@echo ""
	@echo "Variables: ENV=$(ENV) (dev|staging|prod), HELM_RELEASE=$(HELM_RELEASE), SVC=$(SVC), P=$(P) (perfiles dev-app)"

.PHONY: deploy deploy-wizard
deploy: ## Ejecuta el asistente interactivo paso a paso de despliegue (./sytue-deploy)
	@$(ROOT_DIR)/sytue-deploy

deploy-wizard: deploy ## Alias de `make deploy`

# =============================================================================
# Desarrollo LOCAL (Docker Compose en deploy/compose)
# =============================================================================
.PHONY: dev-up
dev-up: ## Levanta toda la suite en local (TLS mkcert + servicios Go + frontends)
	@# Lee COMPOSE_PROFILES del .env (por defecto `full` = suite completa, como siempre).
	@test -f $(COMPOSE_DIR)/.env || cp $(COMPOSE_DIR)/.env.example $(COMPOSE_DIR)/.env
	@$(MAKE) dev-certs
	@# BuildKit es obligatorio (los Dockerfile Go usan `RUN --mount=type=cache`). En
	@# entornos sin el plugin `docker buildx`, `up --build` cae al builder legacy y
	@# falla; por eso se SEPARA el build (con DOCKER_BUILDKIT) del up.
	DOCKER_BUILDKIT=1 COMPOSE_DOCKER_CLI_BUILD=1 $(COMPOSE) -f $(COMPOSE_DIR)/docker-compose.yml build
	$(COMPOSE) -f $(COMPOSE_DIR)/docker-compose.yml up -d

.PHONY: dev-up-built
dev-up-built: ## Suite COMPLETA en modo COMPILADO (SSR runtime, ~1.2 GB vs ~4.8 GB de dev; sin HMR)
	@# Cada frontend se sirve compilado (target runtime, ~80 MB) en vez de astro dev
	@# (~450 MB). Representativo de prod. Para desarrollar UNA app con HMR usa dev-app.
	@test -f $(COMPOSE_DIR)/.env || cp $(COMPOSE_DIR)/.env.example $(COMPOSE_DIR)/.env
	@$(MAKE) dev-certs
	DOCKER_BUILDKIT=1 COMPOSE_DOCKER_CLI_BUILD=1 $(COMPOSE) -f $(COMPOSE_DIR)/docker-compose.yml -f $(COMPOSE_DIR)/frontlite.yml build
	$(COMPOSE) -f $(COMPOSE_DIR)/docker-compose.yml -f $(COMPOSE_DIR)/frontlite.yml up -d

.PHONY: dev-app
dev-app: ## Solo núcleo + apps concretas (DEV, cabe en 8 GB): make dev-app P=docs,mail
	@# DEV-ONLY: núcleo (traefik/postgres/minio/dragonfly/id/hub) + los perfiles de P.
	@# COMPOSE_PROFILES en la línea de comandos GANA al `full` del .env (mismo compose).
	@test -n "$(P)" || (echo "ERROR: indica el/los perfiles, p.ej. make dev-app P=docs,mail" && exit 1)
	@test -f $(COMPOSE_DIR)/.env || cp $(COMPOSE_DIR)/.env.example $(COMPOSE_DIR)/.env
	@$(MAKE) dev-certs
	DOCKER_BUILDKIT=1 COMPOSE_DOCKER_CLI_BUILD=1 COMPOSE_PROFILES=$(P) $(COMPOSE) -f $(COMPOSE_DIR)/docker-compose.yml build
	COMPOSE_PROFILES=$(P) $(COMPOSE) -f $(COMPOSE_DIR)/docker-compose.yml up -d

.PHONY: dev-certs
dev-certs: ## Genera el certificado TLS local de confianza (mkcert)
	mkcert -install
	mkcert -cert-file $(COMPOSE_DIR)/traefik/certs/sytue-localhost.pem \
	       -key-file  $(COMPOSE_DIR)/traefik/certs/sytue-localhost-key.pem \
	       "sytue.localhost" "*.sytue.localhost"

.PHONY: dev-down
dev-down: ## Para la suite local (conserva volúmenes)
	$(COMPOSE) -f $(COMPOSE_DIR)/docker-compose.yml down

.PHONY: dev-clean
dev-clean: ## Para la suite y BORRA volúmenes (reset total)
	$(COMPOSE) -f $(COMPOSE_DIR)/docker-compose.yml down -v

.PHONY: dev-logs
dev-logs: ## Sigue los logs (SVC=id para uno concreto)
	$(COMPOSE) -f $(COMPOSE_DIR)/docker-compose.yml logs -f $(SVC)

.PHONY: dev-ps
dev-ps: ## Estado/salud de los servicios locales
	$(COMPOSE) -f $(COMPOSE_DIR)/docker-compose.yml ps

.PHONY: dev-seed
dev-seed: ## Crea el usuario admin de desarrollo en Sytue ID
	@# Requiere la pila levantada (make dev-up). Ejecuta el seed DENTRO del
	@# contenedor id usando el binario ya presente en la imagen (/app/id).
	@# Idempotente y solo-dev. Alternativa sin Docker (Postgres local en :5432):
	@#   SYTUE_ID_ENV=dev \
	@#   SYTUE_ID_DATABASE_DSN="postgres://sytue_id:PASS@127.0.0.1:5432/sytue_id?sslmode=disable" \
	@#   go run ./services/id/cmd/id seed
	@# La contraseña por defecto es sytue-dev-admin; cámbiala con
	@# SYTUE_ID_DEV_SEED_PASSWORD.
	cd "$(COMPOSE_DIR)" && { \
	  if [ -z "$$DOCKER_HOST" ] && [ -S "$$HOME/.colima/default/docker.sock" ]; then \
	    export DOCKER_HOST="unix://$$HOME/.colima/default/docker.sock"; \
	  fi; \
	  $(COMPOSE) exec -T id /app/id seed; \
	}

.PHONY: dev-demo
dev-demo: ## Puebla la suite local con datos de ejemplo (Mail/Contacts/Calendar/Docs/Drive/Rooms)
	@# Requiere la pila levantada y `make dev-seed` hecho (usuarios de dev). Escribe
	@# por los MISMOS BFF que usa el navegador, con la sesión OIDC real del admin:
	@# si el seed pasa, esos caminos funcionan. Idempotente (no duplica si ya corrió).
	@# --retries=0 a propósito: un reintento de Playwright volvería a sembrar lo ya
	@# escrito antes del fallo y dejaría los datos por duplicado.
	cd "$(ROOT_DIR)"/test/e2e && bun install --frozen-lockfile && \
	  SYTUE_SEED=1 bunx playwright test seed-demo --project=chromium --retries=0 --reporter=list

# =============================================================================
# Backend Go (módulo único git.sytue.eu/sytue/one)
# =============================================================================
.PHONY: go-build
go-build: ## Compila todos los servicios Go (go build ./...)
	cd "$(ROOT_DIR)" && go build ./...

.PHONY: go-test
go-test: ## Ejecuta los tests del módulo (go test ./...)
	cd "$(ROOT_DIR)" && go test ./...

.PHONY: test-integration
test-integration: ## Tests de integración con Postgres real (build-tag integration; requiere Docker/Colima)
	cd "$(ROOT_DIR)" && { \
	  if [ -z "$$DOCKER_HOST" ] && [ -S "$$HOME/.colima/default/docker.sock" ]; then \
	    export DOCKER_HOST="unix://$$HOME/.colima/default/docker.sock"; \
	    export TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE=$${TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE:-/var/run/docker.sock}; \
	  fi; \
	  export TESTCONTAINERS_RYUK_DISABLED=$${TESTCONTAINERS_RYUK_DISABLED:-true}; \
	  go test -tags=integration -count=1 -p 2 -timeout 20m ./...; \
	}

.PHONY: go-vet
go-vet: ## go vet sobre todo el módulo
	cd "$(ROOT_DIR)" && go vet ./...

.PHONY: go-lint
go-lint: ## Lint del backend Go (golangci-lint)
	cd "$(ROOT_DIR)" && golangci-lint run ./...

.PHONY: go-tidy
go-tidy: ## Resuelve y fija dependencias (go mod tidy + download)
	cd "$(ROOT_DIR)" && go mod tidy && go mod download

.PHONY: go-fmt
go-fmt: ## Formatea todo el código Go (gofmt)
	cd "$(ROOT_DIR)" && gofmt -w $$(find . -name '*.go' -not -path './vendor/*')

.PHONY: migrate
migrate: ## Aplica migraciones de un servicio Go (uso: make migrate SVC=id)
	@test -n "$(SVC)" || (echo "ERROR: indica el servicio, p.ej. make migrate SVC=id" && exit 1)
	cd "$(ROOT_DIR)" && go run ./services/$(SVC)/cmd/$(SVC) migrate

.PHONY: migrate-check
migrate-check: ## Gate: rechaza migraciones destructivas sin marca (el mismo que corre en CI)
	cd "$(ROOT_DIR)" && go test ./platform/pkg/migcheck/

.PHONY: toolchain-check
toolchain-check: ## Comprueba que runtimes, CI, contenedores y workspaces no divergen
	cd "$(ROOT_DIR)" && ./scripts/check-toolchain-versions.sh

.PHONY: eu-supply-chain-check
eu-supply-chain-check: ## Gate: rechaza proveedores desconocidos, excepciones caducadas y nueva cadena no UE
	cd "$(ROOT_DIR)" && go run ./tools/eusupply

.PHONY: docs-links
docs-links: ## Gate: los enlaces internos de la documentación llevan a algún sitio
	@# La documentación es la «fuente de verdad» del repositorio: un enlace a un ancla que
	@# se renombró deja a quien lo sigue sin el dato justo cuando lo necesita, y eso no lo
	@# ve ningún compilador. El formato de estos .md lo vigila `format:check` (apps/frontend).
	cd "$(ROOT_DIR)" && go run ./tools/docslinks

# =============================================================================
# Frontend (Bun en apps/frontend — Astro 7 + Preact 10.29; Bun sustituye Node+pnpm)
# =============================================================================
.PHONY: sync-branding
sync-branding: ## Vendoriza los assets de marca (branding/ canónico) dentro de @sytue/ui
	cd "$(ROOT_DIR)" && bash scripts/sync-branding.sh

.PHONY: fe-install
fe-install: sync-branding ## Instala dependencias del frontend (bun install)
	cd "$(FRONTEND_DIR)" && bun install

.PHONY: fe-dev
fe-dev: ## Arranca las apps Astro en modo desarrollo (4321-4331)
	cd "$(FRONTEND_DIR)" && bun run dev

.PHONY: fe-build
fe-build: ## Compila todos los paquetes y apps del frontend
	cd "$(FRONTEND_DIR)" && bun run build

.PHONY: fe-lint
fe-lint: ## Lint del frontend (oxlint + ESLint para *.astro, vía Bun)
	cd "$(FRONTEND_DIR)" && bun run lint

# =============================================================================
# Interfaz nativa en Fyne (apps/fyne) y capturas de referencia
# =============================================================================
.PHONY: fyne-cmd-check
fyne-cmd-check: ## Type-chequea los diez `cmd/` de apps/fyne (sin CGO, segundos)
	@# LOS `cmd/` NO LOS MIRA NADIE hasta cortar una release: el piloto no los
	@# construye —le faltan `pkg-config` y las cabeceras de Wayland— y el CI no
	@# dispara en push. Había TRES binarios en `main` que no compilaban en NINGUNA
	@# plataforma: uno comparaba con `nil` un valor que no es puntero y dos pasaban
	@# un `string` donde va un `Locale`.
	@#
	@# `GOOS=js GOARCH=wasm` los TIPA enteros sin CGO y sin bibliotecas gráficas, en
	@# segundos. OJO: wasm aquí es UNA HERRAMIENTA DE TYPE-CHECK y nada más. NO es
	@# un destino que vayamos a sostener —los destinos son macOS, Linux, Windows,
	@# iOS y Android, `docs/CONVENTIONS.md` §13.6— y que este comando pase no
	@# significa que la suite corra en el navegador.
	bash "$(ROOT_DIR)/apps/fyne/scripts/preparar-livekit.sh"
	cd "$(ROOT_DIR)/apps/fyne" && GOOS=js GOARCH=wasm go build -tags goolm -o /dev/null ./...

.PHONY: capturas-check
capturas-check: ## Trinquete de las capturas rancias de test/e2e/shots (color, no geometría)
	"$(ROOT_DIR)/scripts/capturas-rancias.sh"

# =============================================================================
# Despliegue — Helm (deploy/helm, umbrella chart sytue-one)
# =============================================================================
.PHONY: helm-lint
helm-lint: helm-vendor-prepare ## Lint del umbrella chart sytue-one (valores base + cada overlay)
	$(HELM_OFFLINE) helm dependency build "$(HELM_CHART)"
	helm lint "$(HELM_CHART)"
	@for v in $(wildcard $(HELM_CHART)/values-*.yaml); do \
		echo "helm lint con $$(basename $$v)"; \
		helm lint "$(HELM_CHART)" -f "$$v" || exit 1; \
	done

.PHONY: helm-vendor-prepare
helm-vendor-prepare: ## Verifica y prepara charts upstream vendorizados, sin red
	cd "$(ROOT_DIR)" && ./scripts/prepare-helm-vendor.sh

.PHONY: helm-install
helm-install: helm-vendor-prepare ## Instala el umbrella chart en el entorno $(ENV)
	$(HELM_OFFLINE) helm dependency build "$(HELM_CHART)"
	helm install $(HELM_RELEASE) "$(HELM_CHART)" \
		--namespace $(HELM_NS) --create-namespace \
		-f "$(HELM_VALUES)"

.PHONY: helm-upgrade
helm-upgrade: helm-vendor-prepare ## Actualiza el umbrella chart en el entorno $(ENV)
	$(HELM_OFFLINE) helm dependency build "$(HELM_CHART)"
	helm upgrade $(HELM_RELEASE) "$(HELM_CHART)" \
		--namespace $(HELM_NS) \
		-f "$(HELM_VALUES)"

# =============================================================================
# Calidad de código (transversal)
# =============================================================================
.PHONY: fmt
fmt: go-fmt ## Formatea backend Go + frontend
	cd "$(FRONTEND_DIR)" && bun run format

.PHONY: sh-lint
sh-lint: ## Lint de los scripts de shell (shellcheck siguiendo los `source`)
	shellcheck -x -P SCRIPTDIR $$(find . -name '*.sh' -not -path '*/node_modules/*' -not -path './.git/*')

.PHONY: lint
lint: go-lint fe-lint sh-lint ## Lint del backend Go + frontend + scripts

.PHONY: gates
gates: ## Corre TODOS los gates de calidad en local
	$(MAKE) toolchain-check
	$(MAKE) eu-supply-chain-check
	go build ./... && go vet ./... && go test ./... && golangci-lint run
	cd "$(FRONTEND_DIR)" && bun run build && bun run typecheck && bun run lint && bun run format:check && bun run test
	cd "$(ROOT_DIR)"/test/e2e && bun run typecheck
	cd "$(ROOT_DIR)"/apps/mobile/suite && bun run typecheck
	cd "$(ROOT_DIR)"/apps/mobile/rooms && bun run typecheck
	$(MAKE) sh-lint
	$(MAKE) docs-links
	$(MAKE) fyne-cmd-check
	$(MAKE) capturas-check
	@echo "✓ Gates en verde."
