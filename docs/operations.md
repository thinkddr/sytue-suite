<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr) -->

# Operación y despliegue — Sytue Suite

Guía completa para **configurar, desplegar, verificar y operar** Sytue Suite en local, en un servidor único con Docker Compose o en un clúster de Kubernetes mediante Helm:

1. **[Runbook de operación](#1-runbook-de-operacion)** — asistente interactivo `./sytue-deploy`, variables de entorno, Docker Compose, Kubernetes/Helm, migraciones, identidad, alta de organizaciones, correo, backups, rotación de secretos, escalado y healthchecks.
2. **[CI/CD y cadena de suministro](#2-cicd-y-cadena-de-suministro)** — workflows de GitHub Actions y puertas de calidad.
3. **[Pruebas E2E (smoke test)](#3-pruebas-e2e-smoke-test)** — validación de extremo a extremo.
4. **[Rendimiento y SEO](#4-rendimiento-y-seo)** — objetivos SLO, presupuestos y medición continua.
5. **[Go-live de Sytue Cloud](#5-go-live-de-sytue-cloud)** — activación del plano de ejecución PaaS en Kubernetes.

> Contrato canónico: [`CONVENTIONS.md`](CONVENTIONS.md). Seguridad y RGPD: [`security.md`](security.md). Arquitectura: [`architecture.md`](architecture.md).

---

<a id="1-runbook-de-operacion"></a>

## 1. Runbook de operación

<a id="10-convenciones-de-este-runbook"></a>

### 1.0 Principios operativos y prerrequisitos

- **Cero secretos en el repositorio:** ningún archivo versionado incluye contraseñas ni claves por defecto. Todas las credenciales se generan en el momento del despliegue mediante el asistente interactivo **`./sytue-deploy`** (o `make deploy`) usando `openssl rand`.
- **Modos de despliegue soportados:**
  1. **Docker Compose (`deploy/compose/`)**: ideal para desarrollo local (`*.sytue.localhost` con `mkcert`) o despliegue rápido en un servidor único. Soporta perfiles modulares (`core`, `office`, `full` o lista personalizada de servicios).
  2. **Kubernetes con Helm (`deploy/helm/sytue-one/`)**: pensado para entornos de producción o *staging* con alta disponibilidad, ingress **Traefik 3**, certificados automáticos con **cert-manager** (Let's Encrypt), PostgreSQL externo y almacenamiento de objetos S3.
- **Herramientas requeridas según el modo:**
  - Común: `bash`, `openssl`, `git`, `make`.
  - Modo Docker Compose: `docker` y `docker compose` v2, `mkcert` (para TLS local).
  - Modo Kubernetes / Helm: `kubectl`, `helm`.

---

<a id="11-bootstrap-inicial-una-sola-vez-por-estacion--por-entorno"></a>

### 1.1 Despliegue guiado paso a paso con `./sytue-deploy`

La forma recomendada de configurar cualquier entorno por primera vez es ejecutar el asistente interactivo incluido en la raíz del repositorio:

```bash
./sytue-deploy
# o equivalente con Make:
make deploy
```

El asistente guía paso a paso por 5 etapas:

1. **Modo de despliegue**: elección entre **Docker Compose** (`compose`) o **Kubernetes / Helm** (`helm`).
2. **Topología y servicios**:
   - En **Docker Compose**: dominio base (por defecto `sytue.localhost`) y perfil de servicios:
     - `full`: toda la suite (16 servicios Go + 22 frontends web + infraestructura).
     - `office`: núcleo + Drive, Docs/Sheets/Slides/CertiPDF, Mail, Calendar, Contacts, Portal y Console.
     - `core`: solo el núcleo (Traefik, PostgreSQL, MinIO, Dragonfly, Sytue ID, Sytue Hub, Portal y Console).
     - `custom`: selección a medida (por ejemplo `mail,drive,docs,rooms,portal`).
   - En **Kubernetes / Helm**: dominio raíz (p. ej. `suite.miempresa.eu`), namespace de Kubernetes, correo para ACME/Let's Encrypt, endpoint de PostgreSQL y credenciales de Object Storage S3.
3. **Organización y administrador inicial**: slug de la primera organización (p. ej. `acme`), correo del administrador y contraseña (introducida manualmente o autogenerada con alta entropía), más configuración opcional de **Sytue AI**.
4. **Generación automática de secretos criptográficos**:
   - Crea contraseñas únicas de 192 bits (`openssl rand -hex 24`) para el superusuario de PostgreSQL y para cada una de las **17 bases de datos de servicio** (`id`, `drive`, `docs`, `mail`, `calendar`, `contacts`, `syerp`, `notes`, `photos`, `cast`, `pages`, `bases`, `rooms`, `hub`, `integrations`, `cloud`, `analytics`), además del rol `authenticator` de PostgREST y MinIO.
   - Genera claves AES-256-GCM de 32 bytes en Base64 (`openssl rand -base64 32`) para el cifrado en reposo de secretos en Cloud (`CLOUD_SECRET_ENC_KEY`), Docs (`DOCS_SECRET_ENC_KEY`), ERP (`SYERP_SECRET_ENC_KEY`), Integrations (`INTEGRATIONS_KEY_ENCRYPTION_KEY`) y la sal diaria de Analytics (`SYTUE_ANALYTICS_KEY_ENCRYPTION_KEY`).
   - Genera secretos de 256 bits (`openssl rand -hex 32`) para las cookies de sesión BFF (`SYTUE_SESSION_SECRET`), clientes OIDC (`SYTUE_DEV_CLIENT_SECRET`), comunicación interna entre microservicios (`ID_INTERNAL_SECRET`, `ROOMS_INTERNAL_SECRET`, `CAST_INTERNAL_SECRET`, `CAST_MEDIA_SIGNING_KEY`) y LiveKit (`LIVEKIT_API_KEY`, `LIVEKIT_API_SECRET`), sincronizando además `deploy/compose/livekit/livekit.yaml`.
   - Escribe `deploy/compose/.env` (modo Compose) o `deploy/helm/values-custom.yaml` + `deploy/helm/apply-secrets.sh` (modo Helm) con permisos restringidos (`0600` / `0700`).
5. **Puesta en marcha**: verifica las herramientas instaladas, ofrece emitir los certificados locales con `mkcert` y lanza el despliegue si el operador lo confirma.

También puede ejecutarse de forma no interactiva en pipelines o scripts automatizados:

```bash
# Generar configuración completa para Docker Compose sin preguntas:
./sytue-deploy --mode compose --profile full --non-interactive --only-config

# Generar overlay y script de secretos para Kubernetes/Helm:
./sytue-deploy --mode helm --domain suite.ejemplo.eu --namespace sytue-prod --non-interactive --only-config
```

---

<a id="variables-de-entorno"></a>

### Referencia de variables de entorno

Ninguna variable tiene secretos predefinidos en el repositorio. A continuación se detallan todas las variables utilizadas por el despliegue (`deploy/compose/.env` y Secrets de Kubernetes):

#### Variables globales y de acceso inicial

| Variable | Descripción | Cómo se genera / Ejemplo |
| --- | --- | --- |
| `COMPOSE_PROFILES` | Perfiles de Docker Compose a levantar además del núcleo (`full`, o lista separada por comas: `mail,drive,docs,rooms,portal,console`). | `full` |
| `SYTUE_DOMAIN` | Dominio base del despliegue. | `sytue.localhost` (local) o `ejemplo.eu` (prod) |
| `SYTUE_ADMIN_EMAIL` | Correo del propietario/administrador inicial. | `admin@sytue.localhost` |
| `SYTUE_ORG_SLUG` | Identificador (slug) de la organización inicial. | `acme` |
| `SYTUE_ID_DEV_SEED_PASSWORD` | Contraseña del administrador para `make dev-seed` en entorno `dev`. | Generada por `./sytue-deploy` |
| `SYTUE_ID_BOOTSTRAP_EMAIL` | Correo del propietario inicial para el comando `id bootstrap-owner` en producción. | `admin@ejemplo.eu` |
| `SYTUE_ID_BOOTSTRAP_ORG_SLUG` | Slug de la organización inicial en producción (`id bootstrap-owner`). | `acme` |
| `SYTUE_ID_BOOTSTRAP_PASSWORD` | Contraseña inicial (mínimo 12 caracteres) para `id bootstrap-owner`. | Generada por `./sytue-deploy` |

#### Variables de bases de datos PostgreSQL (aislamiento por servicio)

Cada microservicio Go dispone de su propia base de datos y rol de PostgreSQL inicializados en `deploy/compose/postgres/init/01-databases.sh`:

| Servicio | Variables (`*_DB_NAME`, `*_DB_USER`, `*_DB_PASSWORD`) | Base de datos por defecto |
| --- | --- | --- |
| Superusuario PG | `POSTGRES_SUPERUSER`, `POSTGRES_SUPERUSER_PASSWORD` | `postgres` (`openssl rand -hex 24`) |
| Sytue ID | `ID_DB_NAME`, `ID_DB_USER`, `ID_DB_PASSWORD` | `sytue_id` |
| Sytue Drive | `DRIVE_DB_NAME`, `DRIVE_DB_USER`, `DRIVE_DB_PASSWORD` | `sytue_drive` |
| Sytue Docs | `DOCS_DB_NAME`, `DOCS_DB_USER`, `DOCS_DB_PASSWORD` | `sytue_docs` |
| Sytue Mail | `MAIL_DB_NAME`, `MAIL_DB_USER`, `MAIL_DB_PASSWORD` | `sytue_mail` |
| Sytue Calendar | `CALENDAR_DB_NAME`, `CALENDAR_DB_USER`, `CALENDAR_DB_PASSWORD` | `sytue_calendar` |
| Sytue Contacts | `CONTACTS_DB_NAME`, `CONTACTS_DB_USER`, `CONTACTS_DB_PASSWORD` | `sytue_contacts` |
| SyERP | `SYERP_DB_NAME`, `SYERP_DB_USER`, `SYERP_DB_PASSWORD` | `sytue_syerp` |
| Sytue Notepads | `NOTES_DB_NAME`, `NOTES_DB_USER`, `NOTES_DB_PASSWORD` | `sytue_notes` |
| Sytue Photos | `SYPHOTOS_DB_NAME`, `SYPHOTOS_DB_USER`, `SYPHOTOS_DB_PASSWORD` | `sytue_syphotos` |
| Sytue Cast | `CAST_DB_NAME`, `CAST_DB_USER`, `CAST_DB_PASSWORD` | `sytue_cast` |
| Sytue Web (Pages) | `PAGES_DB_NAME`, `PAGES_DB_USER`, `PAGES_DB_PASSWORD` | `sytue_pages` |
| Sytue Bases | `BASES_DB_NAME`, `BASES_DB_USER`, `BASES_DB_PASSWORD` | `sytue_bases` |
| Sytue Rooms | `ROOMS_DB_NAME`, `ROOMS_DB_USER`, `ROOMS_DB_PASSWORD` | `sytue_rooms` |
| Sytue Hub | `HUB_DB_NAME`, `HUB_DB_USER`, `HUB_DB_PASSWORD` | `sytue_hub` |
| Integration Gateway | `INTEGRATIONS_DB_NAME`, `INTEGRATIONS_DB_USER`, `INTEGRATIONS_DB_PASSWORD` | `sytue_integrations` |
| Sytue Cloud | `CLOUD_DB_NAME`, `CLOUD_DB_USER`, `CLOUD_DB_PASSWORD` | `sytue_cloud` |
| Sytue Analytics | `ANALYTICS_DB_NAME`, `ANALYTICS_DB_USER`, `ANALYTICS_DB_PASSWORD` | `sytue_analytics` |
| PostgREST (Cloud DB) | `POSTGREST_ROLE`, `POSTGREST_ROLE_PASSWORD`, `POSTGREST_JWT_SECRET` | `authenticator` |

#### Claves criptográficas, OIDC, Object Storage y tiempo real

| Variable | Propósito | Generación |
| --- | --- | --- |
| `SYTUE_SESSION_SECRET` | Clave para sellar las cookies de sesión HTTP-only de las aplicaciones web (`@sytue/auth`). | `openssl rand -hex 32` |
| `SYTUE_DEV_CLIENT_SECRET` | Secreto compartido de los clientes confidenciales OIDC entre los BFF y Sytue ID. | `openssl rand -hex 32` |
| `CLOUD_SECRET_ENC_KEY` | Clave KEK AES-256-GCM (32 bytes en Base64) para cifrar secretos de proyectos en Sytue Cloud. | `openssl rand -base64 32` |
| `DOCS_SECRET_ENC_KEY` | Clave KEK AES-256-GCM (32 bytes en Base64) para cifrar credenciales de conectores de datos en Calcus. | `openssl rand -base64 32` |
| `SYERP_SECRET_ENC_KEY` | Clave KEK AES-256-GCM (32 bytes en Base64) para cifrar secretos de pasarelas en SyERP. | `openssl rand -base64 32` |
| `INTEGRATIONS_KEY_ENCRYPTION_KEY` | Clave KEK AES-256-GCM (32 bytes en Base64) para cifrar tokens de conectores SaaS en reposo. | `openssl rand -base64 32` |
| `SYTUE_ANALYTICS_KEY_ENCRYPTION_KEY` | Clave KEK AES-256-GCM (32 bytes en Base64) para cifrar la sal diaria de Sytue Analytics. | `openssl rand -base64 32` |
| `ID_INTERNAL_SECRET` | Secreto HMAC interno entre Sytue ID y Sytue Mail para verificar contraseñas de aplicación IMAP/SMTP. | `openssl rand -hex 32` |
| `ROOMS_INTERNAL_SECRET` | Secreto interno entre Sytue Rooms y Sytue Drive para autorizar adjuntos de canales. | `openssl rand -hex 32` |
| `CAST_INTERNAL_SECRET` | Secreto interno para comunicación de control con Sytue Cast. | `openssl rand -hex 32` |
| `CAST_MEDIA_SIGNING_KEY` | Clave para firmar URLs de segmentos y manifiestos HLS en Sytue Cast. | `openssl rand -hex 32` |
| `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` | Credenciales del almacenamiento S3 local (MinIO). | `openssl rand -hex 24` |
| `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET` | Par de claves del SFU WebRTC LiveKit para firmar tokens de sala y webhooks. | `openssl rand -hex 32` |
| `AI_PROVIDER`, `AI_API_KEY`, `AI_MODEL`, `AI_BASE_URL` | Configuración opcional del proveedor LLM para el asistente `services/ai`. | Proveedor (`scaleway` u `openai-compat`) |

---

<a id="12-dev-local-en-un-solo-comando"></a>

### 1.2 Despliegue con Docker Compose

Una vez generado `deploy/compose/.env` con `./sytue-deploy`:

```bash
# Levantar los servicios configurados en COMPOSE_PROFILES (con generación de certificados mkcert):
make dev-up

# Crear la organización inicial y el usuario administrador en Sytue ID:
make dev-seed

# (Opcional) Levantar únicamente perfiles concretos para ahorrar memoria RAM:
make dev-app P=docs,mail,portal

# (Opcional) Levantar la suite con los frontends precompilados (menor consumo de RAM):
make dev-up-built
```

Comandos de gestión:

- `make dev-ps`: muestra el estado y salud (`healthy`) de cada contenedor.
- `make dev-logs SVC=id`: sigue los logs de un servicio específico.
- `make dev-down`: detiene los contenedores conservando los volúmenes de datos.
- `make dev-clean`: detiene los contenedores y elimina todos los volúmenes locales.

---

<a id="13-prod-en-scaleway-terraform--helm"></a>

### 1.3 Despliegue en Kubernetes con Helm

Para desplegar en cualquier clúster de Kubernetes (k3s, Kapsule o clúster gestionado):

1. **Generar los secretos y el overlay de valores**:
   ```bash
   ./sytue-deploy --mode helm --domain suite.miempresa.eu --namespace sytue-prod --only-config
   ```
2. **Aplicar los Secrets en el clúster**:
   ```bash
   bash deploy/helm/apply-secrets.sh
   ```
3. **Preparar las dependencias vendorizadas del chart y desplegar con Helm**:
   ```bash
   make helm-vendor-prepare
   HELM_REPOSITORY_CONFIG="deploy/helm/.vendor/no-repositories.yaml" \
   HELM_REPOSITORY_CACHE="deploy/helm/.vendor/repository-cache" \
     helm dependency build deploy/helm/sytue-one

   helm upgrade --install sytue-one deploy/helm/sytue-one \
     --namespace sytue-prod --create-namespace \
     -f deploy/helm/sytue-one/values.yaml \
     -f deploy/helm/values-custom.yaml
   ```
4. **Dar de alta el propietario inicial y reconciliar los secretos OIDC en Sytue ID**:
   ```bash
   kubectl -n sytue-prod exec deploy/sytue-one-id -- /app/id bootstrap-owner
   ```

---

<a id="14-migraciones-de-postgresql-por-servicio-go"></a>

### 1.4 Migraciones de PostgreSQL

Cada microservicio Go embebe sus migraciones SQL (`services/<svc>/db/migrations/*.sql`) mediante `embed.FS` y las aplica automáticamente al arrancar de forma transaccional (`platform/pkg/pg/migrate.go`). También pueden ejecutarse manualmente:

```bash
make migrate SVC=id
```

El gate `make migrate-check` verifica que no existan números de versión duplicados ni operaciones destructivas sin anotación explícita.

---

<a id="15-identidad-sso-y-scim-sytue-id--servicio-go-nuclear"></a>

### 1.5 Identidad, SSO y SCIM (Sytue ID)

**Sytue ID** (`services/id`, puerto `:8081`) es el proveedor OpenID Connect nuclear de la suite:

- **Discovery OIDC**: `GET /.well-known/openid-configuration` y claves públicas en `GET /.well-known/jwks.json`.
- **OAuth 2.1 + PKCE (`S256`)**: obligatorio en todos los flujos de autorización.
- **Segundo factor (MFA)**: soporte integrado para TOTP (RFC 6238) y WebAuthn / Passkeys (FIDO2). Los roles `owner` y `admin` requieren *step-up MFA*.
- **SCIM 2.0**: aprovisionamiento automatizado de usuarios y grupos bajo `/scim/v2/Users` y `/scim/v2/Groups` mediante tokens Bearer por organización.

---

<a id="16-alta-de-un-tenant-nuevo"></a>

### 1.6 Alta de organizaciones (Multi-tenancy)

Cada organización se identifica por un `slug` inmutable (`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`) y está aislada en todas las tablas de negocio mediante políticas PostgreSQL `ENABLE` + `FORCE ROW LEVEL SECURITY` (`platform/pkg/pg/tx.go` fija `SET LOCAL app.tenant = $1` en cada transacción).

Para crear organizaciones adicionales puedes usar la consola de administración (`https://admin.<dominio>`) o la API de Sytue ID.

---

<a id="17-operacion-de-correo-sytue-mail-dns-y-prueba-de-entrega"></a>

### 1.7 Operación de correo (Sytue Mail): DNS y DKIM

Para habilitar el envío y recepción de correo en un dominio propio con `services/mail`:

1. **Registros DNS requeridos**:
   - `MX`: `@ IN MX 10 mx.<dominio>.`
   - `SPF`: `@ IN TXT "v=spf1 mx -all"`
   - `DKIM`: `sytue._domainkey IN TXT "v=DKIM1; k=rsa; p=<clave_publica>"`
   - `DMARC`: `_dmarc IN TXT "v=DMARC1; p=quarantine; rua=mailto:postmaster@<dominio>"`
2. **Puertos expuestos**: JMAP/HTTP en `:8086`, SMTP en `:25`, `:465` y `:587`, e IMAPS en `:993`.

---

<a id="19-backup-y-restore"></a>

### 1.8 Copias de seguridad y restauración

1. **PostgreSQL**: realizar volcados lógicos por base de datos con `pg_dump -Fc --no-owner --no-acl -d <db> > <db>.dump` o copias físicas continuas (PITR) del servidor PostgreSQL.
2. **Object Storage (S3)**: activar el versionado de buckets en S3/MinIO y replicar periódicamente los buckets `sytue-drive`, `sytue-mail`, `sytue-docs` y `sytue-cloud`.
3. **Restauración**: restaurar las bases de datos con `pg_restore --clean --if-exists -d <db> <db>.dump` y verificar la salud de los servicios con `/healthz` y `/readyz`.

---

<a id="110-rotacion-de-secretos"></a>

### 1.9 Rotación de secretos y claves de firma

- **Claves de firma OIDC (JWKS)**: Sytue ID soporta múltiples claves activas en `signing_keys`. Al rotar la clave primaria, la anterior permanece publicada en `/.well-known/jwks.json` con estado `retiring` hasta que expiran los tokens en vuelo. En caso de emergencia, `/app/id revoke-all` invalida inmediatamente todas las sesiones y tokens anteriores.
- **Secretos de aplicación y base de datos**: regenerar ejecutando `./sytue-deploy` o actualizando los `Secret` de Kubernetes y realizando un `kubectl rollout restart deployment` en el namespace correspondiente.

---

<a id="112-escalado"></a>

### 1.10 Escalado y comprobaciones de salud (Healthchecks)

Todos los servicios Go son *stateless* y escalan horizontalmente detrás de Traefik (`HorizontalPodAutoscaler` incluido en cada subchart de `deploy/helm/charts/*`). El estado compartido reside en PostgreSQL, S3 y Dragonfly.

Cada servicio expone dos sondas estándar (`platform/pkg/httpx/health.go`):

- `GET /healthz`: *liveness probe* (verifica que el proceso HTTP responde).
- `GET /readyz`: *readiness probe* (verifica conectividad activa con PostgreSQL y dependencias críticas).

---

<a id="1b-perfiles-de-despliegue-cual-es-cual"></a>
<a id="111-apps-oauth-de-la-plataforma-google-drive-onedrive-y-dropbox-como-almacen-conectado"></a>

### 1.11 Almacenamiento externo conectado (Google Drive, OneDrive, Dropbox y S3)

Sytue Drive permite conectar cuentas externas de almacenamiento mediante OAuth 2.0 o credenciales S3:

- Las credenciales de aplicación OAuth se configuran mediante variables de entorno en `services/drive` (`DRIVE_GOOGLE_CLIENT_ID` / `DRIVE_GOOGLE_CLIENT_SECRET`, `DRIVE_ONEDRIVE_CLIENT_ID` / `DRIVE_ONEDRIVE_CLIENT_SECRET`, `DRIVE_DROPBOX_CLIENT_ID` / `DRIVE_DROPBOX_CLIENT_SECRET`).
- Los tokens de cada usuario se almacenan cifrados en reposo con AES-256-GCM.

---

<a id="2-cicd-y-cadena-de-suministro"></a>

## 2. CI/CD y cadena de suministro

Los flujos de integración continua en `.github/workflows/` validan el repositorio en cada ejecución:

- **`ci-go.yml`**: compilación, `go vet`, tests unitarios con detector de carreras (`-race`) y `golangci-lint` v2.
- **`ci-frontend.yml`**: compilación de paquetes `@sytue/*`, `typecheck`, `lint`, `format:check` y tests unitarios con Bun.
- **`ci-integration.yml`**: tests de integración (`//go:build integration`) contra PostgreSQL real.
- **`security.yml`**: análisis estático de vulnerabilidades (`govulncheck`, `osv-scanner`, `trivy`), detección de secretos (`gitleaks`), cumplimiento de licencias permisivas y control de cadena de suministro (`make eu-supply-chain-check`).

Para ejecutar todas las comprobaciones localmente:

```bash
make gates
```

---

<a id="3-pruebas-e2e-smoke-test"></a>

## 3. Pruebas E2E (smoke test)

Con el entorno levantado, el script `scripts/e2e/smoke.sh` verifica de extremo a extremo las sondas `/healthz` y `/readyz` de todos los servicios, el descubrimiento OIDC de Sytue ID, el flujo Authorization Code + PKCE y las rutas principales de Traefik:

```bash
bash scripts/e2e/smoke.sh
```

Las pruebas completas de navegador con Playwright se encuentran en `test/e2e/`.

---

<a id="4-rendimiento-y-seo"></a>

## 4. Rendimiento y SEO

- **Objetivos de latencia (SLO)**: APIs Go `p95 < 150 ms` en lecturas y `< 300 ms` en escrituras; hidratación selectiva en Astro (*Islands Architecture*) para minimizar el JavaScript enviado al navegador.
- **Core Web Vitals**: LCP `< 2.0 s`, CLS `< 0.05` e INP `< 150 ms`, verificados con Lighthouse CI (`.lighthouserc.json`).
- **Compresión y caché**: compresión Brotli/Zstd/Gzip en el borde y activos estáticos inmutables con `Cache-Control: public, max-age=31536000, immutable`.

---

<a id="5-go-live-de-sytue-cloud"></a>

## 5. Go-live de Sytue Cloud

`services/cloud` opera en dos modos configurables por `RUNNER_MODE`:

1. **Modo simulado (`noop`, por defecto en desarrollo local)**: permite utilizar todo el dashboard y la API de Sytue Cloud sin requerir un clúster Kubernetes subyacente.
2. **Modo Kubernetes (`kube`, en producción)**: `cloud-api` y `cloud-controller` crean los recursos reales (`Namespace`, `Deployment`, `Service`, `Ingress`, `NetworkPolicy`) en el clúster y `cloud-builder` ejecuta las construcciones de imágenes de contenedor aisladas.

Las plantillas de referencia para activar el modo `kube` en un clúster se encuentran en `services/cloud/deploy/go-live/`.
