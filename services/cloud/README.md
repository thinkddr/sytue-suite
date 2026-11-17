<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Daniel Delgado Ramos
-->

# Sytue Cloud (`services/cloud`)

**Sytue-native PaaS** platform for the **Sytue One** suite — *Vercel × Supabase ×
Google Cloud*. A Go **control plane** that connects git repositories, builds
images with **Kaniko**, deploys customer apps on **Kapsule** with domains and
**automatic TLS**, manages **per-PR previews**, and integrates **managed
services** (Sytue Postgres/PostgREST, Storage, Functions, Cron, Auth). Code
Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr); third-party dependencies are permissive only (BSD/MIT/Apache).
**Clean-room** reimplementation on open standards (OCI, Kubernetes, OIDC,
S3, PostgREST). **100% EU sovereignty** (Scaleway).

- **Ports:** API `8090` · builder `8091` (health) · controller `8093` (health, local)
- **Subdomain:** `cloud.sytue.eu` (+ `cloud-api`) · apps `*.apps.sytue.eu` ·
  previews `<proyecto>-pr-<n>.preview.sytue.eu`
- **Datastore:** **PostgreSQL** for ALL state (organizations, projects,
  builds, deployments, domains, env/secrets, services, usage) **and the build
  queue** (FOR UPDATE SKIP LOCKED) + event bus (outbox + LISTEN/NOTIFY).
  **No Redis/Valkey.**
- **Blobs:** S3 Object Storage (build cache + logs), bucket `build-cache`.
- **Auth:** **Sytue ID** JWT validated via JWKS (`platform/pkg/authn`), audience
  `sytue-cloud-api`. Git webhooks with the provider's HMAC signature.
- **Multi-tenant:** isolation through **RLS** in PostgreSQL (`app.tenant`) ⇄
  Kapsule namespace `tenant-<slug>` ⇄ per-tenant S3 prefix.

<a id="arquitectura--tres-binarios-un-módulo"></a>
## Architecture — three binaries, one module

| Binary | Role |
|---|---|
| **`cloud-api`** | REST API (chi + OpenAPI): orgs, projects, builds, deployments, domains, env/secrets, managed services, usage/billing and git webhooks. |
| **`cloud-builder`** | Worker that **drains the build queue** in PostgreSQL, detects the framework, schedules **Kaniko Jobs** (Kapsule, rootless), pushes to the Scaleway **Container Registry** and caches layers/logs in S3. |
| **`cloud-controller`** | **Reconciler** that brings the desired state to the cluster: deploys apps into tenant namespaces (Deployment/Service/Ingress + **ResourceQuota/NetworkPolicy**), verifies domains and issues **TLS (cert-manager)**, manages per-PR previews, **provisions managed services** and **injects their credentials** (encrypted at rest) into the deployments. |

The `git push → build → deploy → URL TLS` flow is: webhook (or manual action) →
`cloud-api` enqueues a build + creates a deployment → `cloud-builder` builds the image →
`cloud-controller` deploys it and publishes the URL. The queue and the events live in
PostgreSQL (no Redis); the controller reacts to `LISTEN/NOTIFY` on the
`sytue_cloud` channel and also reconciles in a loop (idempotent).

<a id="servicios-gestionados-dx-supabase-ue"></a>
## Managed services (Supabase DX, EU)

| Service | What it provisions |
|---|---|
| **postgres** | Sytue Postgres database + **PostgREST** (automatic REST API), pooling, backups, branching. Injects `DATABASE_URL`, `POSTGREST_URL`. |
| **storage** | Dedicated S3 bucket. Injects `S3_*`. |
| **functions** | Serverless functions runtime. |
| **cron** | Scheduled jobs. |
| **auth** | Managed auth via **Sytue ID** (realm per project). Injects `OIDC_*`. |

Each service's credentials are **encrypted with AES-256-GCM** (`SECRET_ENC_KEY`)
before being persisted, and only the controller decrypts them in memory to inject them
as Kubernetes Secrets into the project's Deployment.

<a id="estructura"></a>
## Structure

```
services/cloud/
├── cmd/
│   ├── cloud-api/main.go         # API (serve | migrate | healthcheck)
│   ├── cloud-builder/main.go     # worker de builds (Kaniko)
│   └── cloud-controller/main.go  # reconciliador (deploy + dominios + servicios)
├── internal/
│   ├── config/                   # configuración (env, 12-factor)
│   ├── crypto/                   # AES-256-GCM para secretos/credenciales
│   ├── domain/                   # tipos y reglas de negocio
│   ├── store/                    # persistencia pgx + RLS por tenant
│   ├── service/                  # lógica de aplicación + parsing de webhooks
│   ├── http/                     # router chi + handlers REST
│   ├── builder/                  # cola, detección de framework, runner Kaniko
│   └── controller/               # Platform (Kapsule) + ServiceProvisioner
├── db/migrations/                # esquema + RLS (golang-migrate)
├── openapi.yaml
├── Dockerfile.api / .builder / .controller
└── README.md
```

<a id="endpoints-resumen"></a>
## Endpoints (summary)

```
GET    /api/v1/org                          PATCH /api/v1/org
GET    /api/v1/projects                     POST  /api/v1/projects
GET    /api/v1/projects/{id}                PATCH /api/v1/projects/{id}   DELETE …
POST   /api/v1/projects/{id}/deployments    GET   /api/v1/projects/{id}/deployments
GET    /api/v1/deployments/{id}             POST  /api/v1/deployments/{id}/rollback   DELETE …
GET    /api/v1/projects/{id}/builds         GET   /api/v1/builds/{id}
GET/POST /api/v1/projects/{id}/domains      DELETE /api/v1/domains/{id}
GET/PUT  /api/v1/projects/{id}/env          DELETE /api/v1/env/{id}
GET/POST /api/v1/projects/{id}/services     GET/DELETE /api/v1/services/{id}
GET    /api/v1/usage                        GET   /api/v1/usage/detail
POST   /webhooks/git                        (firma HMAC; sin auth de Sytue ID)
```

Full contract in [`openapi.yaml`](./openapi.yaml).

<a id="configuración-entorno"></a>
## Configuration (environment)

| Variable | Default (dev) | Use |
|---|---|---|
| `PORT` | 8090 / 8091 / 8093 | the binary's HTTP listener |
| `DATABASE_URL` | — (required) | PostgreSQL DSN |
| `OIDC_ISSUER` / `OIDC_JWKS_URL` / `OIDC_AUDIENCE` | id.sytue.localhost / … / `sytue-cloud-api` | JWT validation |
| `S3_ENDPOINT` / `S3_REGION` / `S3_ACCESS_KEY` / `S3_SECRET_KEY` / `S3_BUCKET` | minio:9000 / fr-par / — / — / `sytue-dev-build-cache` | build cache/logs |
| `REGISTRY_URL` / `REGISTRY_USERNAME` / `REGISTRY_PASSWORD` | rg.fr-par.scw.cloud/sytue | Container Registry |
| `BUILD_NAMESPACE` / `CERT_ISSUER` | sytue-builds / sytue-letsencrypt | Kapsule + cert-manager |
| `RUNNER_MODE` / `KUBECONFIG` / `KANIKO_IMAGE` | (auto by environment) / (in-cluster) / kaniko executor | kube/stub selection + access to Kapsule |
| `APPS_DOMAIN` / `PREVIEW_DOMAIN` / `INGRESS_TARGET` | apps.sytue.localhost / preview.sytue.localhost / ingress.sytue.eu | URLs and CNAME |
| `SECRET_ENC_KEY` | — (required) | 32-byte base64 key (AES-256) to encrypt secrets |
| `WEBHOOK_SECRET` | — | HMAC signature for git webhooks |
| `BUILD_TIMEOUT` / `RECONCILE_INTERVAL` / `MAX_PROJECTS_PER_ORG` | 20m / 15s / 100 | limits/cadence |

No secrets in git: everything via environment / Secret Manager.

<a id="seguridad-y-rgpd"></a>
## Security and GDPR

- **Deny-by-default RLS** per tenant on every table (`app.tenant`).
- **Secrets encrypted at rest** (AES-256-GCM); never in logs or responses.
- **Webhooks** verified by HMAC signature (GitHub SHA-256/SHA-1, GitLab token),
  idempotent per *delivery id*.
- **Distroless/rootless** containers; customer builds with **rootless Kaniko**,
  deny-by-default `ResourceQuota` + `NetworkPolicy` per tenant namespace.
- Data and compute **only in Scaleway EU regions**.

<a id="nota-de-despliegue-paridad-devprod-15"></a>
## Deployment note (dev/prod parity, §15)

The builder and the controller keep a single interface (`builder.Runner`,
`controller.Platform`, `controller.ServiceProvisioner`) with **two
implementations** selected by configuration:

- **stub** (dev/local): does not touch Kubernetes; the
  queue → build → deploy → *ready* flow runs end-to-end without a cluster.
- **kube** (production): real implementation with **client-go** (Apache-2.0) on
  **Kapsule**:
  - `builder.KubeJobClient` schedules rootless **Kaniko Jobs** (restrictive
    securityContext, no ServiceAccount token, Registry Secret mounted at
    `/kaniko/.docker`), tracks their status and collects the Pod's logs (→ S3).
  - `controller.KubePlatform` idempotently applies
    **Deployment + Service + Ingress (Traefik) + HPA** in the tenant's namespace
    (`tenant-<slug>`), which it secures with **ResourceQuota** and **deny-by-default
    NetworkPolicy**; manages **TLS with cert-manager** (`Certificate` resource
    via the dynamic client) and verifies custom domains via DNS.
  - `controller.KubeProvisioner` materializes the **managed services**
    (Sytue Postgres 18 + PostgREST with Secret/PVC/Deployment/Service; S3 bucket
    for storage; per-tenant isolation in Sytue ID for auth) and **injects the
    encrypted credentials** (AES-256-GCM) into the deployments.

Selection: `RUNNER_MODE=kube|stub` (if omitted, it is inferred from the environment: `prod` ⇒
kube, `dev`/`staging` ⇒ stub). Relevant variables: `KUBECONFIG` (empty ⇒
in-cluster), `BUILD_NAMESPACE`, `KANIKO_IMAGE`, `CERT_ISSUER`.

The unit tests for the translation into Kubernetes objects use client-go's *fake
clientset* (no cluster); the ones that require a real cluster sit behind
`//go:build integration` and are **skipped** if there is no `KUBECONFIG`.
