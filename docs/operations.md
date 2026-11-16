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

### 1.1 Instalación guiada con `./sytue-deploy`

La forma recomendada de instalar la suite es el asistente de la raíz del repositorio
(guía de usuario: [`QUICKSTART.md`](../QUICKSTART.md)):

```bash
./sytue-deploy          # o: make deploy
```

**Modo local (Docker Compose, recomendado).** Cinco pasos, todos con un valor por defecto
que basta aceptar con Enter:

1. **Dónde instalar**: este equipo con Docker (`compose`) o un clúster (`helm`).
2. **Requisitos**: Docker en marcha, Compose v2, memoria asignada a Docker (aviso por debajo
   de 8 GB con la suite completa), `mkcert` y puertos 80/443 libres. Si falta algo explica
   cómo instalarlo y se detiene sin escribir nada.
3. **Servicios** (`COMPOSE_PROFILES`): `full` (todo), `office` (Drive, Docus/Calcus/Diapos/
   CertiPDF, Mail, Calendar, Contacts, portal y consola), `core` (identidad, portal y
   consola) o una lista a medida (`mail,drive,docs,rooms`).
4. **Configuración y secretos**: contraseña del administrador (Enter = aleatoria), Sytue AI
   opcional y `deploy/compose/.env` con valores únicos generados con `openssl rand`: las 17
   bases de datos, claves AES-256, secretos OIDC/HMAC, MinIO, LiveKit y PostgREST. Si el
   `.env` ya existe **se reutiliza** (sus contraseñas ya están en el volumen de PostgreSQL) y
   solo se completan las variables que falten de versiones anteriores.
5. **Construcción y arranque**: certificado `mkcert` para `*.sytue.localhost`, `docker compose
   build` + `up -d`, alta del administrador en Sytue ID (`id seed` con la contraseña del
   `.env`), espera a que todos los contenedores estén `healthy` y verificación de extremo a
   extremo con `scripts/e2e/smoke.sh` (resultado en `deploy/compose/.verificacion.log`).

El entorno Compose vive siempre en `*.sytue.localhost` (los hosts están fijados en el
`docker-compose.yml`, en Traefik y en los clientes OIDC sembrados). Cuentas creadas:
`admin@sytue.localhost` (rol admin, TOTP de desarrollo `JBSWY3DPEHPK3PXP`) y
`member@sytue.localhost` (rol miembro, sin segundo factor), ambas en la organización `acme`.

Opciones útiles:

```bash
./sytue-deploy --yes                       # sin preguntas, suite completa
./sytue-deploy --yes --profile office      # sin preguntas, versión ligera
./sytue-deploy --only-config               # solo genera deploy/compose/.env
./sytue-deploy --reset                     # BORRA los datos locales y reinstala
./sytue-deploy --admin-password '…'        # fija la contraseña (también SYTUE_ADMIN_PASSWORD)
./sytue-deploy --help                      # todas las opciones
```

---

<a id="variables-de-entorno"></a>

### Referencia de variables de entorno

Ninguna variable tiene secretos predefinidos en el repositorio. A continuación se detallan todas las variables utilizadas por el despliegue (`deploy/compose/.env` y Secrets de Kubernetes):

#### Variables globales y de acceso inicial

| Variable | Descripción | Cómo se genera / Ejemplo |
| --- | --- | --- |
| `COMPOSE_PROFILES` | Perfiles de Docker Compose a levantar además del núcleo (`full`, o lista separada por comas: `mail,drive,docs,rooms,portal,console`). | `full` |
| `SYTUE_ID_DEV_SEED_PASSWORD` | Contraseña de `admin@sytue.localhost` y `member@sytue.localhost` (Compose). La aplican `./sytue-deploy` y `make dev-seed`. | Elegida en el asistente o `Sytue-<hex>!` aleatoria |
| `SYTUE_ID_BOOTSTRAP_EMAIL` · `SYTUE_ID_BOOTSTRAP_ORG_SLUG` · `SYTUE_ID_BOOTSTRAP_PASSWORD` | Propietario inicial en Kubernetes (`id bootstrap-owner`, lanzado por `deploy/helm/bootstrap-admin.sh`). | Pedidos por el asistente en modo Helm |

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

`./sytue-deploy` hace todo lo de esta sección. Para operar la suite después:

| Comando | Qué hace |
| --- | --- |
| `make dev-up` | Construye lo que haya cambiado y arranca los perfiles de `COMPOSE_PROFILES` (si no hay `.env`, lo genera el asistente). |
| `make dev-seed` | Crea o reafirma las cuentas de desarrollo con la contraseña de `SYTUE_ID_DEV_SEED_PASSWORD`. Idempotente. |
| `make dev-app P=docs,mail` | Solo el núcleo y los perfiles indicados. |
| `make dev-up-built` | Frontends compilados (menos memoria, sin recarga en caliente). |
| `make dev-ps` · `make dev-logs SVC=id` | Estado y logs. |
| `make dev-down` | Para los contenedores conservando los datos. |
| `make dev-clean` / `./sytue-deploy --reset` | Borra los volúmenes (el segundo además regenera los secretos y reinstala). |
| `scripts/e2e/smoke.sh` | Verificación de extremo a extremo. |

---

<a id="13-prod-en-scaleway-terraform--helm"></a>

### 1.3 Despliegue en Kubernetes con Helm (avanzado)

El chart `deploy/helm/sytue-one` no despliega PostgreSQL ni S3 y descarga las imágenes de
`global.imageRegistry`. El asistente prepara todo lo demás:

```bash
./sytue-deploy --mode helm --domain suite.miempresa.eu
```

Genera en `deploy/helm/` (Git los ignora; contienen secretos):

| Fichero | Para qué |
| --- | --- |
| `values-custom.yaml` | Dominio, registro, PostgreSQL, S3, ACME, `livekit.webhook.apiKey` y CIDR del balanceador. |
| `apply-secrets.sh` | Todos los Secrets que referencia el chart: DSN de las 17 bases, S3, claves de cifrado, LiveKit, cookies y **un secreto OIDC por app web** (el mismo en `sytue-frontend-bff` y en `sytue-id-client-secrets`, que Sytue ID reconcilia al arrancar). |
| `create-databases.sh` | Un rol y una base por servicio en tu PostgreSQL (idempotente). |
| `bootstrap-admin.sh` | Job de una ejecución que crea el propietario inicial (`id bootstrap-owner`). |
| `apply-integrations.sh` · `values-integrations.yaml` | Base del Integration Gateway y apps OAuth (§ integraciones del README). |

Y después, en orden:

```bash
docker login <registro>
bash deploy/helm/publicar-imagenes.sh <registro> suite.miempresa.eu   # construye y sube 42 imágenes
PGUSER=postgres PGPASSWORD=… bash deploy/helm/create-databases.sh
bash deploy/helm/apply-secrets.sh && bash deploy/helm/apply-integrations.sh
bash scripts/prepare-helm-vendor.sh
HELM_REPOSITORY_CONFIG=deploy/helm/.vendor/no-repositories.yaml \
HELM_REPOSITORY_CACHE=deploy/helm/.vendor/repository-cache \
  helm dependency build deploy/helm/sytue-one
helm upgrade --install sytue-one deploy/helm/sytue-one -n sytue-prod --create-namespace \
  -f deploy/helm/sytue-one/values.yaml -f deploy/helm/values-custom.yaml \
  -f deploy/helm/values-integrations.yaml
bash deploy/helm/bootstrap-admin.sh
```

El propietario entra en `https://id.<dominio>/account` y activa su segundo factor: hasta
entonces el rol `owner` mantiene cerrada Sytue Admin. El correo (SMTP/IMAP) usa el Secret
`tls-wildcard-sytue-eu`; si no existe, `apply-secrets.sh` crea uno autofirmado para que
Mail arranque: sustitúyelo por el certificado comodín de tu dominio. La imagen de
Dragonfly (`ghcr.io/thinkddr/upstream-dragonfly`) no la construye `publicar-imagenes.sh`.

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
