<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr) -->

<a id="operación-y-despliegue--sytue-suite"></a>

# Operations and deployment — Sytue Suite

A complete guide to **configuring, deploying, verifying and operating** Sytue Suite locally, on a single server with Docker Compose or on a Kubernetes cluster with Helm:

1. **[Operations runbook](#1-runbook-de-operacion)** — `./sytue-deploy` interactive wizard, environment variables, Docker Compose, Kubernetes/Helm, migrations, identity, organization onboarding, email, backups, secret rotation, scaling and healthchecks.
2. **[CI/CD and supply chain](#2-cicd-y-cadena-de-suministro)** — GitHub Actions workflows and quality gates.
3. **[E2E tests (smoke test)](#3-pruebas-e2e-smoke-test)** — end-to-end validation.
4. **[Performance and SEO](#4-rendimiento-y-seo)** — SLO targets, budgets and continuous measurement.
5. **[Sytue Cloud go-live](#5-go-live-de-sytue-cloud)** — activating the PaaS execution plane on Kubernetes.

> Canonical contract: [`CONVENTIONS.md`](CONVENTIONS.md). Security and GDPR: [`security.md`](security.md). Architecture: [`architecture.md`](architecture.md).

---

<a id="1-runbook-de-operacion"></a>

<a id="1-runbook-de-operación"></a>

## 1. Operations runbook

<a id="10-convenciones-de-este-runbook"></a>

<a id="10-principios-operativos-y-prerrequisitos"></a>

### 1.0 Operating principles and prerequisites

- **Zero secrets in the repository:** no versioned file includes default passwords or keys. All credentials are generated at deployment time by the **`./sytue-deploy`** interactive wizard (or `make deploy`) using `openssl rand`.
- **Supported deployment modes:**
  1. **Docker Compose (`deploy/compose/`)**: ideal for local development (`*.sytue.localhost` with `mkcert`) or a quick deployment on a single server. Supports modular profiles (`core`, `office`, `full` or a custom list of services).
  2. **Kubernetes with Helm (`deploy/helm/sytue-one/`)**: intended for high-availability production or _staging_ environments, with a **Traefik 3** ingress, automatic certificates with **cert-manager** (Let's Encrypt), external PostgreSQL and S3 object storage.
- **Required tools depending on the mode:**
  - Common: `bash`, `openssl`, `git`, `make`.
  - Docker Compose mode: `docker` and `docker compose` v2, `mkcert` (for local TLS).
  - Kubernetes / Helm mode: `kubectl`, `helm`.

---

<a id="11-bootstrap-inicial-una-sola-vez-por-estacion--por-entorno"></a>

<a id="11-instalación-guiada-con-sytue-deploy"></a>

### 1.1 Guided installation with `./sytue-deploy`

The recommended way to install the suite is the wizard at the repository root
(user guide: [`QUICKSTART.md`](../QUICKSTART.md)):

```bash
./sytue-deploy          # o: make deploy
```

**Local mode (Docker Compose, recommended).** Five steps, each with a default value
that you can simply accept with Enter:

1. **Where to install**: this machine with Docker (`compose`) or a cluster (`helm`).
2. **Requirements**: Docker running, Compose v2, memory assigned to Docker (warning below
   4 GB; the full suite once started uses ~1.4 GB with the web apps compiled), `mkcert` and ports 80/443 free. If something is missing it explains
   how to install it and stops without writing anything.
3. **Services** (`COMPOSE_PROFILES`): `full` (everything), `office` (Drive, Docus/Calcus/Diapos/
   CertiPDF, Mail, Calendar, Contacts, portal and console), `core` (identity, portal and
   console) or a custom list (`mail,drive,docs,rooms`).
4. **Configuration and secrets**: administrator password (Enter = random), optional Sytue AI
   and `deploy/compose/.env` with unique values generated with `openssl rand`: the 17
   databases, AES-256 keys, OIDC/HMAC secrets, MinIO, LiveKit and PostgREST. If the
   `.env` already exists **it is reused** (its passwords are already in the PostgreSQL volume) and
   only the variables missing from earlier versions are filled in.
5. **Build and startup**: `mkcert` certificate for `*.sytue.localhost`,
   `docker compose build` + `up -d`, creation of the administrator in Sytue ID (`id seed` with
   the password from the `.env`), a wait until all containers are `healthy` and end-to-end
   verification with `scripts/e2e/smoke.sh` (result in `deploy/compose/.verificacion.log`).

The Compose environment always lives on `*.sytue.localhost` (the hosts are fixed in
`docker-compose.yml`, in Traefik and in the seeded OIDC clients). Accounts created:
`admin@sytue.localhost` (admin role, development TOTP `JBSWY3DPEHPK3PXP`) and
`member@sytue.localhost` (member role, no second factor), both in the `acme` organization.

Useful options:

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

<a id="referencia-de-variables-de-entorno"></a>

### Environment variable reference

No variable has predefined secrets in the repository. All the variables used by the deployment (`deploy/compose/.env` and Kubernetes Secrets) are detailed below:

<a id="variables-globales-y-de-acceso-inicial"></a>

#### Global and initial access variables

| Variable                                                                                   | Description                                                                                                                              | How it is generated / Example                   |
| ------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------- |
| `COMPOSE_PROFILES`                                                                         | Docker Compose profiles to bring up in addition to the core (`full`, or a comma-separated list: `mail,drive,docs,rooms,portal,console`). | `full`                                          |
| `SYTUE_ID_DEV_SEED_PASSWORD`                                                               | Password of `admin@sytue.localhost` and `member@sytue.localhost` (Compose). Applied by `./sytue-deploy` and `make dev-seed`.             | Chosen in the wizard or a random `Sytue-<hex>!` |
| `SYTUE_ID_BOOTSTRAP_EMAIL` · `SYTUE_ID_BOOTSTRAP_ORG_SLUG` · `SYTUE_ID_BOOTSTRAP_PASSWORD` | Initial owner in Kubernetes (`id bootstrap-owner`, launched by `deploy/helm/bootstrap-admin.sh`).                                        | Asked for by the wizard in Helm mode            |

<a id="variables-de-bases-de-datos-postgresql-aislamiento-por-servicio"></a>

#### PostgreSQL database variables (per-service isolation)

Each Go microservice has its own PostgreSQL database and role, initialized in `deploy/compose/postgres/init/01-databases.sh`:

| Service              | Variables (`*_DB_NAME`, `*_DB_USER`, `*_DB_PASSWORD`)                      | Default database                    |
| -------------------- | -------------------------------------------------------------------------- | ----------------------------------- |
| PG superuser         | `POSTGRES_SUPERUSER`, `POSTGRES_SUPERUSER_PASSWORD`                        | `postgres` (`openssl rand -hex 24`) |
| Sytue ID             | `ID_DB_NAME`, `ID_DB_USER`, `ID_DB_PASSWORD`                               | `sytue_id`                          |
| Sytue Drive          | `DRIVE_DB_NAME`, `DRIVE_DB_USER`, `DRIVE_DB_PASSWORD`                      | `sytue_drive`                       |
| Sytue Docs           | `DOCS_DB_NAME`, `DOCS_DB_USER`, `DOCS_DB_PASSWORD`                         | `sytue_docs`                        |
| Sytue Mail           | `MAIL_DB_NAME`, `MAIL_DB_USER`, `MAIL_DB_PASSWORD`                         | `sytue_mail`                        |
| Sytue Calendar       | `CALENDAR_DB_NAME`, `CALENDAR_DB_USER`, `CALENDAR_DB_PASSWORD`             | `sytue_calendar`                    |
| Sytue Contacts       | `CONTACTS_DB_NAME`, `CONTACTS_DB_USER`, `CONTACTS_DB_PASSWORD`             | `sytue_contacts`                    |
| SyERP                | `SYERP_DB_NAME`, `SYERP_DB_USER`, `SYERP_DB_PASSWORD`                      | `sytue_syerp`                       |
| Sytue Notepads       | `NOTES_DB_NAME`, `NOTES_DB_USER`, `NOTES_DB_PASSWORD`                      | `sytue_notes`                       |
| Sytue Photos         | `SYPHOTOS_DB_NAME`, `SYPHOTOS_DB_USER`, `SYPHOTOS_DB_PASSWORD`             | `sytue_syphotos`                    |
| Sytue Cast           | `CAST_DB_NAME`, `CAST_DB_USER`, `CAST_DB_PASSWORD`                         | `sytue_cast`                        |
| Sytue Web (Pages)    | `PAGES_DB_NAME`, `PAGES_DB_USER`, `PAGES_DB_PASSWORD`                      | `sytue_pages`                       |
| Sytue Bases          | `BASES_DB_NAME`, `BASES_DB_USER`, `BASES_DB_PASSWORD`                      | `sytue_bases`                       |
| Sytue Rooms          | `ROOMS_DB_NAME`, `ROOMS_DB_USER`, `ROOMS_DB_PASSWORD`                      | `sytue_rooms`                       |
| Sytue Hub            | `HUB_DB_NAME`, `HUB_DB_USER`, `HUB_DB_PASSWORD`                            | `sytue_hub`                         |
| Integration Gateway  | `INTEGRATIONS_DB_NAME`, `INTEGRATIONS_DB_USER`, `INTEGRATIONS_DB_PASSWORD` | `sytue_integrations`                |
| Sytue Cloud          | `CLOUD_DB_NAME`, `CLOUD_DB_USER`, `CLOUD_DB_PASSWORD`                      | `sytue_cloud`                       |
| Sytue Analytics      | `ANALYTICS_DB_NAME`, `ANALYTICS_DB_USER`, `ANALYTICS_DB_PASSWORD`          | `sytue_analytics`                   |
| PostgREST (Cloud DB) | `POSTGREST_ROLE`, `POSTGREST_ROLE_PASSWORD`, `POSTGREST_JWT_SECRET`        | `authenticator`                     |

<a id="claves-criptográficas-oidc-object-storage-y-tiempo-real"></a>

#### Cryptographic keys, OIDC, Object Storage and real time

| Variable                                               | Purpose                                                                                 | Generation                               |
| ------------------------------------------------------ | --------------------------------------------------------------------------------------- | ---------------------------------------- |
| `SYTUE_SESSION_SECRET`                                 | Key used to seal the HTTP-only session cookies of the web applications (`@sytue/auth`). | `openssl rand -hex 32`                   |
| `SYTUE_DEV_CLIENT_SECRET`                              | Shared secret of the confidential OIDC clients between the BFFs and Sytue ID.           | `openssl rand -hex 32`                   |
| `CLOUD_SECRET_ENC_KEY`                                 | AES-256-GCM KEK (32 bytes in Base64) to encrypt project secrets in Sytue Cloud.         | `openssl rand -base64 32`                |
| `DOCS_SECRET_ENC_KEY`                                  | AES-256-GCM KEK (32 bytes in Base64) to encrypt data connector credentials in Calcus.   | `openssl rand -base64 32`                |
| `SYERP_SECRET_ENC_KEY`                                 | AES-256-GCM KEK (32 bytes in Base64) to encrypt payment gateway secrets in SyERP.       | `openssl rand -base64 32`                |
| `INTEGRATIONS_KEY_ENCRYPTION_KEY`                      | AES-256-GCM KEK (32 bytes in Base64) to encrypt SaaS connector tokens at rest.          | `openssl rand -base64 32`                |
| `SYTUE_ANALYTICS_KEY_ENCRYPTION_KEY`                   | AES-256-GCM KEK (32 bytes in Base64) to encrypt the daily salt of Sytue Analytics.      | `openssl rand -base64 32`                |
| `ID_INTERNAL_SECRET`                                   | Internal HMAC secret between Sytue ID and Sytue Mail to verify IMAP/SMTP app passwords. | `openssl rand -hex 32`                   |
| `ROOMS_INTERNAL_SECRET`                                | Internal secret between Sytue Rooms and Sytue Drive to authorize channel attachments.   | `openssl rand -hex 32`                   |
| `CAST_INTERNAL_SECRET`                                 | Internal secret for control communication with Sytue Cast.                              | `openssl rand -hex 32`                   |
| `CAST_MEDIA_SIGNING_KEY`                               | Key to sign HLS segment and manifest URLs in Sytue Cast.                                | `openssl rand -hex 32`                   |
| `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD`              | Credentials of the local S3 storage (MinIO).                                            | `openssl rand -hex 24`                   |
| `LIVEKIT_API_KEY` / `LIVEKIT_API_SECRET`               | Key pair of the LiveKit WebRTC SFU to sign room tokens and webhooks.                    | `openssl rand -hex 32`                   |
| `AI_PROVIDER`, `AI_API_KEY`, `AI_MODEL`, `AI_BASE_URL` | Optional LLM provider configuration for the `services/ai` assistant.                    | Provider (`scaleway` or `openai-compat`) |

---

<a id="12-dev-local-en-un-solo-comando"></a>

<a id="12-despliegue-con-docker-compose"></a>

### 1.2 Deployment with Docker Compose

`./sytue-deploy` does everything in this section. To operate the suite afterwards:

| Command                                     | What it does                                                                                                                |
| ------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| `make dev-up`                               | Builds whatever has changed and starts the profiles in `COMPOSE_PROFILES` (if there is no `.env`, the wizard generates it). |
| `make dev-seed`                             | Creates or re-asserts the development accounts with the password from `SYTUE_ID_DEV_SEED_PASSWORD`. Idempotent.             |
| `make dev-app P=docs,mail`                  | Only the core and the given profiles.                                                                                       |
| `make dev-up-built`                         | Compiled frontends (less memory, no hot reload).                                                                            |
| `make dev-ps` · `make dev-logs SVC=id`      | Status and logs.                                                                                                            |
| `make dev-down`                             | Stops the containers, keeping the data.                                                                                     |
| `make dev-clean` / `./sytue-deploy --reset` | Deletes the volumes (the latter also regenerates the secrets and reinstalls).                                               |
| `scripts/e2e/smoke.sh`                      | End-to-end verification.                                                                                                    |

<a id="servidor-único-y-contenedor-único"></a>

#### Single server and single container

`./sytue-deploy --mode servidor --domain <dominio>` uses the same Compose files,
rendered for your domain in `deploy/compose/.servidor/` (outside Git), plus two
overlays:

| File                                                   | What changes                                                                                                                                                                                                                                                                                                        |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| [`servidor.yml`](../deploy/compose/servidor.yml)       | Traefik publishes 80/443 on all interfaces and without a dashboard; LiveKit publishes 7881/TCP and announces the public IP (or `SYTUE_IP_MEDIOS`); Sytue ID in self-hosted production (`SYTUE_ID_SELF_HOSTED`, encrypted signing key, SMTP, open or closed sign-up depending on `--usuarios`/`--registro-abierto`). |
| [`letsencrypt.yml`](../deploy/compose/letsencrypt.yml) | With `--tls letsencrypt` (the default): Traefik's ACME HTTP-01 issuer and an `acme` volume for the certificates. With `--tls interno`, an mkcert certificate for `*.<dominio>`.                                                                                                                                     |

After startup, the wizard sets the secrets of the apps' OIDC clients
(`id reconcile-client-secrets`), adds their redirect addresses on your domain and creates the
owner (`id bootstrap-owner`); all of it is idempotent. Mode-specific variables in
`deploy/compose/.env`: `SYTUE_DOMINIO`, `SYTUE_TLS`, `SYTUE_ACME_EMAIL`,
`SYTUE_PROPIETARIO`, `SYTUE_ORGANIZACION`, `SYTUE_PROPIETARIO_PASSWORD`, `SYTUE_USUARIOS`,
`SYTUE_REGISTRO_ABIERTO`, `ID_KEY_ENCRYPTION_KEY` and `SYTUE_SMTP_*` (empty: the suite's own
mail server sends Sytue ID's email).

[`deploy/todo-en-uno/`](../deploy/todo-en-uno/Dockerfile) packages exactly that mode
in an image with internal Docker: the container keeps the images, the
volumes, `.env`, the certificates and the CA in `/datos`, and on every startup it runs the
wizard again without questions (`--no-build` from the second time on).

---

<a id="13-prod-en-scaleway-terraform--helm"></a>

<a id="13-despliegue-en-kubernetes-con-helm-avanzado"></a>

### 1.3 Deployment on Kubernetes with Helm (advanced)

The `deploy/helm/sytue-one` chart does not deploy PostgreSQL or S3 and pulls the images from
`global.imageRegistry`. It needs Kubernetes 1.31+ with cert-manager already installed; it brings its
own Traefik, so on k3s install the cluster with `--disable traefik`. Hardware and network
requirements: [README § Minimum requirements](../README.md#requisitos-mínimos-para-desplegar-en-un-servidor).
The wizard prepares everything else:

```bash
./sytue-deploy --mode helm --domain suite.miempresa.eu
```

It generates in `deploy/helm/` (ignored by Git; they contain secrets):

| File                                                 | What for                                                                                                                                                                                                                                                |
| ---------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `values-custom.yaml`                                 | Domain, registry, PostgreSQL, S3, ACME, `livekit.webhook.apiKey` and the load balancer CIDR.                                                                                                                                                            |
| `apply-secrets.sh`                                   | All the Secrets referenced by the chart: DSNs of the 17 databases, S3, encryption keys, LiveKit, cookies and **one OIDC secret per web app** (the same in `sytue-frontend-bff` and in `sytue-id-client-secrets`, which Sytue ID reconciles on startup). |
| `create-databases.sh`                                | One role and one database per service in your PostgreSQL (idempotent).                                                                                                                                                                                  |
| `bootstrap-admin.sh`                                 | One-off Job that creates the initial owner (`id bootstrap-owner`).                                                                                                                                                                                      |
| `apply-integrations.sh` · `values-integrations.yaml` | Integration Gateway base and OAuth apps (README § integrations).                                                                                                                                                                                        |

And then, in order:

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

The owner signs in at `https://id.<dominio>/account` and enables their second factor: until
then the `owner` role keeps Sytue Admin closed. Email (SMTP/IMAP) uses the
`tls-wildcard-sytue-eu` Secret; if it does not exist, `apply-secrets.sh` creates a self-signed one so that
Mail starts: replace it with your domain's wildcard certificate. The
Dragonfly image (`ghcr.io/thinkddr/upstream-dragonfly`) is not built by `publicar-imagenes.sh`.

---

<a id="14-migraciones-de-postgresql-por-servicio-go"></a>

<a id="14-migraciones-de-postgresql"></a>

### 1.4 PostgreSQL migrations

Each Go microservice embeds its SQL migrations (`services/<svc>/db/migrations/*.sql`) via `embed.FS` and applies them automatically and transactionally on startup (`platform/pkg/pg/migrate.go`). They can also be run manually:

```bash
make migrate SVC=id
```

The `make migrate-check` gate verifies that there are no duplicate version numbers and no destructive operations without an explicit annotation.

---

<a id="15-identidad-sso-y-scim-sytue-id--servicio-go-nuclear"></a>

<a id="15-identidad-sso-y-scim-sytue-id"></a>

### 1.5 Identity, SSO and SCIM (Sytue ID)

**Sytue ID** (`services/id`, port `:8081`) is the suite's core OpenID Connect provider:

- **OIDC Discovery**: `GET /.well-known/openid-configuration` and public keys at `GET /.well-known/jwks.json`.
- **OAuth 2.1 + PKCE (`S256`)**: mandatory in all authorization flows.
- **Second factor (MFA)**: built-in support for TOTP (RFC 6238) and WebAuthn / Passkeys (FIDO2). The `owner` and `admin` roles require _step-up MFA_.
- **SCIM 2.0**: automated provisioning of users and groups under `/scim/v2/Users` and `/scim/v2/Groups` with per-organization Bearer tokens.

---

<a id="16-alta-de-un-tenant-nuevo"></a>

<a id="16-alta-de-organizaciones-multi-tenancy"></a>

### 1.6 Organization onboarding (Multi-tenancy)

Each organization is identified by an immutable `slug` (`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`) and is isolated in every business table by PostgreSQL `ENABLE` + `FORCE ROW LEVEL SECURITY` policies (`platform/pkg/pg/tx.go` sets `SET LOCAL app.tenant = $1` in every transaction).

To create additional organizations you can use the admin console (`https://admin.<dominio>`) or the Sytue ID API.

---

<a id="17-operacion-de-correo-sytue-mail-dns-y-prueba-de-entrega"></a>

<a id="17-operación-de-correo-sytue-mail-dns-y-dkim"></a>

### 1.7 Email operations (Sytue Mail): DNS and DKIM

To enable sending and receiving email on your own domain with `services/mail`:

1. **Required DNS records**:
   - `MX`: `@ IN MX 10 mx.<dominio>.`
   - `SPF`: `@ IN TXT "v=spf1 mx -all"`
   - `DKIM`: `sytue._domainkey IN TXT "v=DKIM1; k=rsa; p=<clave_publica>"`
   - `DMARC`: `_dmarc IN TXT "v=DMARC1; p=quarantine; rua=mailto:postmaster@<dominio>"`
2. **Exposed ports**: JMAP/HTTP on `:8086`, SMTP on `:25`, `:465` and `:587`, and IMAPS on `:993`.

---

<a id="19-backup-y-restore"></a>

<a id="18-copias-de-seguridad-y-restauración"></a>

### 1.8 Backups and restore

1. **PostgreSQL**: take logical dumps per database with `pg_dump -Fc --no-owner --no-acl -d <db> > <db>.dump` or continuous physical backups (PITR) of the PostgreSQL server.
2. **Object Storage (S3)**: enable bucket versioning in S3/MinIO and periodically replicate the `sytue-drive`, `sytue-mail`, `sytue-docs` and `sytue-cloud` buckets.
3. **Restore**: restore the databases with `pg_restore --clean --if-exists -d <db> <db>.dump` and check the health of the services with `/healthz` and `/readyz`.

---

<a id="110-rotacion-de-secretos"></a>

<a id="19-rotación-de-secretos-y-claves-de-firma"></a>

### 1.9 Rotation of secrets and signing keys

- **OIDC signing keys (JWKS)**: Sytue ID supports multiple active keys in `signing_keys`. When the primary key is rotated, the previous one stays published in `/.well-known/jwks.json` with the `retiring` status until in-flight tokens expire. In an emergency, `/app/id revoke-all` immediately invalidates all previous sessions and tokens.
- **Application and database secrets**: regenerate them by running `./sytue-deploy` or by updating the Kubernetes `Secret` objects and running a `kubectl rollout restart deployment` in the corresponding namespace.

---

<a id="112-escalado"></a>

<a id="110-escalado-y-comprobaciones-de-salud-healthchecks"></a>

### 1.10 Scaling and healthchecks

All Go services are _stateless_ and scale horizontally behind Traefik (`HorizontalPodAutoscaler` included in each subchart of `deploy/helm/charts/*`). Shared state lives in PostgreSQL, S3 and Dragonfly.

Each service exposes two standard probes (`platform/pkg/httpx/health.go`):

- `GET /healthz`: _liveness probe_ (checks that the HTTP process responds).
- `GET /readyz`: _readiness probe_ (checks active connectivity with PostgreSQL and critical dependencies).

---

<a id="1b-perfiles-de-despliegue-cual-es-cual"></a>
<a id="111-apps-oauth-de-la-plataforma-google-drive-onedrive-y-dropbox-como-almacen-conectado"></a>

<a id="111-almacenamiento-externo-conectado-google-drive-onedrive-dropbox-y-s3"></a>

### 1.11 Connected external storage (Google Drive, OneDrive, Dropbox and S3)

Sytue Drive can connect external storage accounts via OAuth 2.0 or S3 credentials:

- OAuth application credentials are configured through environment variables in `services/drive` (`DRIVE_GOOGLE_CLIENT_ID` / `DRIVE_GOOGLE_CLIENT_SECRET`, `DRIVE_ONEDRIVE_CLIENT_ID` / `DRIVE_ONEDRIVE_CLIENT_SECRET`, `DRIVE_DROPBOX_CLIENT_ID` / `DRIVE_DROPBOX_CLIENT_SECRET`).
- Each user's tokens are stored encrypted at rest with AES-256-GCM.

---

<a id="112-integraciones"></a>

<a id="112-integraciones-y-conectores-con-terceros"></a>

### 1.12 Third-party integrations and connectors

The **Sytue Integration Gateway** (`services/integrations`) connects the suite with some fifty external services: Google Workspace, Microsoft 365, Slack, Zoom, GitHub, Jira, Salesforce, Dropbox, WhatsApp Business, IMAP/POP3 mailboxes, SAML/OIDC providers and more. All traffic to third parties goes out through it, with per-organization policy, and the credentials of each connection are stored **sealed with AES-256**: never in plain text.

> **Requirement:** the gateway is deployed with **Kubernetes/Helm**. The Docker Compose profile does not include it yet, so the connectors are not available on Compose. The only external provider in that profile is Sytue AI, which the wizard configures in step 3.

<a id="activarlas-paso-a-paso-con-el-cli"></a>

#### Enabling them step by step with the CLI

```bash
# Dentro del despliegue completo (paso 4 de 6 del asistente):
./sytue-deploy --mode helm

# O solo las integraciones, sobre un despliegue Helm ya existente (se puede repetir;
# también `make deploy-integrations`):
./sytue-deploy --integrations --domain suite.miempresa.eu
```

The wizard goes through three levels:

1. **Gateway base (automatic).** It generates the KEK that seals the credentials, the secret of the `sytue-gateway` client in Sytue ID and the internal secret it shares with Drive. Without this base the integrations pod does not start. **No existing key is rotated when it is run again**: rotating the KEK would make the credentials already stored unreadable.
2. **Platform OAuth apps (optional, provider by provider).** For each provider it tells you where to register the app, which options to choose and which **redirect URI** to declare, and then asks you for the `client_id` and the `client_secret` (the latter without showing it on screen). With a platform app, each organization connects **with one click**: it only approves the permissions screen. If you do not register it, each organization will have to register its own (_BYO_).
3. **The rest of the connectors** (SAML, OIDC, IMAP/POP3, API tokens) need nothing in the deployment: each organization sets them up from Sytue Admin.

The redirect URI is **the same for all providers**:

```
https://integrations.<tu-dominio>/api/v1/oauth/callback
```

| Provider          | Where the app is registered                                                                                                                               | What it covers                                                   |
| ----------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------- |
| Google            | [Google Cloud Console → Credentials](https://console.cloud.google.com/apis/credentials) · OAuth client ID “Web application”                               | Workspace, Drive, Gmail, Chat and Meet (a single app)            |
| Microsoft         | [Microsoft Entra → App registrations](https://entra.microsoft.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade) · multitenant, “Web” redirect | Microsoft 365, Azure, OneDrive, Teams and Outlook (a single app) |
| Slack             | [api.slack.com/apps](https://api.slack.com/apps) · from a manifest ([`integraciones-marketplace/slack`](../integraciones-marketplace/slack))              | SSO/SCIM, Rooms channels and rooms                               |
| Zoom              | [Zoom App Marketplace](https://marketplace.zoom.us/develop/create) · “General App”                                                                        | Meetings                                                         |
| GitHub            | [New OAuth App](https://github.com/settings/applications/new)                                                                                             | Repositories and migration                                       |
| Jira Cloud        | [Atlassian Developer Console](https://developer.atlassian.com/console/myapps/) · OAuth 2.0 (3LO)                                                          | Jira and Atlassian SSO                                           |
| Salesforce        | Setup → App Manager → _Connected App_                                                                                                                     | CRM                                                              |
| ServiceNow        | System OAuth → _Application Registry_                                                                                                                     | Incidents and services                                           |
| Dropbox           | [Dropbox App Console](https://www.dropbox.com/developers/apps)                                                                                            | Connected storage in Drive                                       |
| Business Central  | App registration in Entra with the Dynamics 365 Business Central API                                                                                      | ERP                                                              |
| Databricks        | Account console → _App connections_                                                                                                                       | Data                                                             |
| Yahoo Mail        | [Yahoo Developer](https://developer.yahoo.com/apps/)                                                                                                      | Mailboxes and migration                                          |
| WhatsApp Business | [Meta for Developers](https://developers.facebook.com/apps/) · _Business_ type app                                                                        | Conversations in Rooms (webhook: `/api/v1/whatsapp/webhook`)     |

<a id="qué-genera-y-cómo-se-aplica"></a>

#### What it generates and how it is applied

| File (in `deploy/helm/`, outside Git)                               | Contents                                                                                                                                                                                                           |
| ------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `integrations-oauth.tsv` · `integrations-oauth-clients.json` (0600) | The registered OAuth apps. The wizard keeps them when it is run again.                                                                                                                                             |
| `apply-integrations.sh` (0700)                                      | Creates or completes the `sytue-integrations`, `sytue-integrations-platform-oauth`, `sytue-id-client-secrets` and `sytue-drive` Secrets (it only generates what is missing) and restarts the affected Deployments. |
| `values-integrations.yaml`                                          | Enables `id.clientSecrets` so that Sytue ID registers the `sytue-gateway` client.                                                                                                                                  |

```bash
bash deploy/helm/apply-secrets.sh          # si aún no lo has aplicado
bash deploy/helm/apply-integrations.sh
helm upgrade --install sytue-one deploy/helm/sytue-one -n sytue-prod \
  -f deploy/helm/sytue-one/values.yaml \
  -f deploy/helm/values-custom.yaml \
  -f deploy/helm/values-integrations.yaml
```

<a id="conectar-una-organización"></a>

#### Connecting an organization

With the suite running, an organization administrator goes to **Sytue Admin → Integrations** (`https://admin.<tu-dominio>/admin/apps`), clicks **Connect** on the application they want and approves the permissions at the provider. From then on, each person uses the connection from their app (Drive, Mail, Rooms…) without copying tokens or webhooks. To publish the apps in the Slack, Teams, Google Workspace or Zoom stores, see [`integraciones-marketplace/`](../integraciones-marketplace/README.md).

---

<a id="2-cicd-y-cadena-de-suministro"></a>

## 2. CI/CD and supply chain

The continuous integration workflows in `.github/workflows/` validate the repository on every run:

- **`ci-go.yml`**: build, `go vet`, unit tests with the race detector (`-race`) and `golangci-lint` v2.
- **`ci-frontend.yml`**: build of the `@sytue/*` packages, `typecheck`, `lint`, `format:check` and unit tests with Bun.
- **`ci-integration.yml`**: integration tests (`//go:build integration`) against a real PostgreSQL.
- **`security.yml`**: static vulnerability analysis (`govulncheck`, `osv-scanner`, `trivy`), secret detection (`gitleaks`), permissive license compliance and supply-chain control (`make eu-supply-chain-check`).

To run all the checks locally:

```bash
make gates
```

---

<a id="3-pruebas-e2e-smoke-test"></a>

## 3. E2E tests (smoke test)

With the environment up, the `scripts/e2e/smoke.sh` script verifies end to end the `/healthz` and `/readyz` probes of all services, Sytue ID's OIDC discovery, the Authorization Code + PKCE flow and the main Traefik routes:

```bash
bash scripts/e2e/smoke.sh
```

The full browser tests with Playwright are in `test/e2e/`.

---

<a id="4-rendimiento-y-seo"></a>

## 4. Performance and SEO

- **Latency targets (SLO)**: Go APIs `p95 < 150 ms` for reads and `< 300 ms` for writes; selective hydration in Astro (_Islands Architecture_) to minimize the JavaScript sent to the browser.
- **Core Web Vitals**: LCP `< 2.0 s`, CLS `< 0.05` and INP `< 150 ms`, verified with Lighthouse CI (`.lighthouserc.json`).
- **Compression and caching**: Brotli/Zstd/Gzip compression at the edge and immutable static assets with `Cache-Control: public, max-age=31536000, immutable`.

---

<a id="5-go-live-de-sytue-cloud"></a>

## 5. Sytue Cloud go-live

`services/cloud` runs in two modes configurable through `RUNNER_MODE`:

1. **Simulated mode (`noop`, the default in local development)**: lets you use the whole Sytue Cloud dashboard and API without requiring an underlying Kubernetes cluster.
2. **Kubernetes mode (`kube`, in production)**: `cloud-api` and `cloud-controller` create the real resources (`Namespace`, `Deployment`, `Service`, `Ingress`, `NetworkPolicy`) in the cluster and `cloud-builder` runs isolated container image builds.

The reference templates for enabling `kube` mode on a cluster are in `services/cloud/deploy/go-live/`.

<a id="sytue-office"></a>

## 6. Sytue Office: building, packaging and testing

Sytue Office ([ADR 0083](decisions.md#0083)) is the fifteen Fyne apps of `apps/fyne/cmd-local`,
built with the `goolm,edicionlocal` tags. The same `apps/fyne/scripts/empaquetar.sh` that packages
the connected apps packages them with `SYTUE_ARBOL=cmd-local`: executables `sytue-office-<app>`,
the desktop bundle `office`, identifiers `eu.sytue.office.<app>` and their own URL schemes
(`sytueoffice-<product>:`), so both editions install side by side. What each app does and does not
do yet: [`apps/fyne/SYTUE-OFFICE.md`](../apps/fyne/SYTUE-OFFICE.md).

<a id="sytue-office-publico"></a>

### 6.1 Sytue Office in this repository

It reaches this repository with the regular sync from sytue-one, together with what its CI needs
(the pinned versions in `deploy/sytue-git/versions.env`, the download and release-upload scripts
in `deploy/sytue-git/bin/` and the CI tool installer in `.github/actions/toolchain/`).

- **On a computer**: `./sytue-deploy --mode sytue-office` builds and installs the apps on Linux and
  macOS ([`QUICKSTART.md` § 9](../QUICKSTART.md#9-sytue-office)). It sets `SYTUE_PARA_ESTE_EQUIPO=1`,
  so a Linux newer than Debian 12 can build with its own glibc: the package is only for that
  computer.
- **On each release**: [`.github/workflows/office.yml`](../.github/workflows/office.yml) packages
  Linux x86-64 and ARM64 and Windows in Debian 12 (glibc 2.36, the floor `empaquetar.sh` enforces),
  macOS universal and Android, on GitHub-hosted runners only, and attaches each package and a
  `SHA256SUMS` to the release. They are **unsigned**: the signing certificates live in sytue-one.
  To rebuild a release that failed halfway: _Actions → office → Run workflow_ with its tag. A pull
  request that changes `office.yml` builds every package too, without attaching anything: that is
  how the workflow gets tested before the first release that uses it.
- **When merging the sync that brought it**: `upstream/sytue-one` has to move forward to the
  export it came in (it does not come from a sytue-one tag; it sits on top of the latest sync), so
  the next sync starts from there. If another sync landed in the meantime, that is no longer a
  fast-forward: export the edition again, with sytue-one's `main` merged, on top of the new one.
