<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Daniel Delgado Ramos
-->

# Sytue Cloud (`services/cloud`)

Plataforma **PaaS Sytue-native** de la suite **Sytue One** — *Vercel × Supabase ×
Google Cloud*. **Control plane** en Go que conecta repositorios git, construye
imágenes con **Kaniko**, despliega apps de clientes en **Kapsule** con dominios y
**TLS automático**, gestiona **previews por PR**, e integra **servicios
gestionados** (Sytue Postgres/PostgREST, Storage, Functions, Cron, Auth). Código
propietario © Sytue; dependencias de terceros solo permisivas (BSD/MIT/Apache).
Reimplementación **clean-room** sobre estándares abiertos (OCI, Kubernetes, OIDC,
S3, PostgREST). **Soberanía 100 % UE** (Scaleway).

- **Puertos:** API `8090` · builder `8091` (salud) · controller `8093` (salud, local)
- **Subdominio:** `cloud.sytue.eu` (+ `cloud-api`) · apps `*.apps.sytue.eu` ·
  previews `<proyecto>-pr-<n>.preview.sytue.eu`
- **Datastore:** **PostgreSQL** para TODO el estado (organizaciones, proyectos,
  builds, deployments, dominios, env/secretos, servicios, uso) **y la cola de
  builds** (FOR UPDATE SKIP LOCKED) + bus de eventos (outbox + LISTEN/NOTIFY).
  **Sin Redis/Valkey.**
- **Blobs:** Object Storage S3 (cache de build + logs), bucket `build-cache`.
- **Auth:** JWT de **Sytue ID** validado vía JWKS (`platform/pkg/authn`), audience
  `sytue-cloud-api`. Webhooks git con firma HMAC del proveedor.
- **Multi-tenant:** aislamiento por **RLS** en PostgreSQL (`app.tenant`) ⇄
  namespace Kapsule `tenant-<slug>` ⇄ prefijo S3 por tenant.

## Arquitectura — tres binarios, un módulo

| Binario | Rol |
|---|---|
| **`cloud-api`** | API REST (chi + OpenAPI): orgs, proyectos, builds, deployments, dominios, env/secretos, servicios gestionados, uso/facturación y webhooks git. |
| **`cloud-builder`** | Worker que **drena la cola de builds** en PostgreSQL, detecta el framework, programa **Jobs de Kaniko** (Kapsule, rootless), empuja al **Container Registry** de Scaleway y cachea capas/logs en S3. |
| **`cloud-controller`** | **Reconciliador** que lleva el estado deseado al clúster: despliega apps en namespaces tenant (Deployment/Service/Ingress + **ResourceQuota/NetworkPolicy**), verifica dominios y emite **TLS (cert-manager)**, gestiona previews por PR, **aprovisiona servicios gestionados** e **inyecta sus credenciales** (cifradas en reposo) en los deployments. |

El flujo `git push → build → deploy → URL TLS` es: webhook (o acción manual) →
`cloud-api` encola build + crea deployment → `cloud-builder` construye la imagen →
`cloud-controller` la despliega y publica la URL. La cola y los eventos viven en
PostgreSQL (sin Redis); el controller reacciona a `LISTEN/NOTIFY` del canal
`sytue_cloud` y además reconcilia en bucle (idempotente).

## Servicios gestionados (DX Supabase, UE)

| Servicio | Qué aprovisiona |
|---|---|
| **postgres** | Base Sytue Postgres + **PostgREST** (API REST automática), pooling, backups, branching. Inyecta `DATABASE_URL`, `POSTGREST_URL`. |
| **storage** | Bucket S3 dedicado. Inyecta `S3_*`. |
| **functions** | Runtime serverless de funciones. |
| **cron** | Jobs programados. |
| **auth** | Auth gestionada vía **Sytue ID** (realm por proyecto). Inyecta `OIDC_*`. |

Las credenciales de cada servicio se **cifran con AES-256-GCM** (`SECRET_ENC_KEY`)
antes de persistir y solo el controller las descifra en memoria para inyectarlas
como Secrets de Kubernetes en el Deployment del proyecto.

## Estructura

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

## Endpoints (resumen)

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

Contrato completo en [`openapi.yaml`](./openapi.yaml).

## Configuración (entorno)

| Variable | Default (dev) | Uso |
|---|---|---|
| `PORT` | 8090 / 8091 / 8093 | escucha HTTP del binario |
| `DATABASE_URL` | — (obligatoria) | DSN PostgreSQL |
| `OIDC_ISSUER` / `OIDC_JWKS_URL` / `OIDC_AUDIENCE` | id.sytue.localhost / … / `sytue-cloud-api` | validación JWT |
| `S3_ENDPOINT` / `S3_REGION` / `S3_ACCESS_KEY` / `S3_SECRET_KEY` / `S3_BUCKET` | minio:9000 / fr-par / — / — / `sytue-dev-build-cache` | cache/logs de build |
| `REGISTRY_URL` / `REGISTRY_USERNAME` / `REGISTRY_PASSWORD` | rg.fr-par.scw.cloud/sytue | Container Registry |
| `BUILD_NAMESPACE` / `CERT_ISSUER` | sytue-builds / sytue-letsencrypt | Kapsule + cert-manager |
| `RUNNER_MODE` / `KUBECONFIG` / `KANIKO_IMAGE` | (auto por entorno) / (in-cluster) / kaniko executor | selección kube/stub + acceso a Kapsule |
| `APPS_DOMAIN` / `PREVIEW_DOMAIN` / `INGRESS_TARGET` | apps.sytue.localhost / preview.sytue.localhost / ingress.sytue.eu | URLs y CNAME |
| `SECRET_ENC_KEY` | — (obligatoria) | clave base64 de 32 B (AES-256) para cifrar secretos |
| `WEBHOOK_SECRET` | — | firma HMAC de webhooks git |
| `BUILD_TIMEOUT` / `RECONCILE_INTERVAL` / `MAX_PROJECTS_PER_ORG` | 20m / 15s / 100 | límites/cadencia |

Sin secretos en git: todo por entorno / Secret Manager.

## Seguridad y RGPD

- **RLS deny-by-default** por tenant en todas las tablas (`app.tenant`).
- **Secretos cifrados en reposo** (AES-256-GCM); nunca en logs ni respuestas.
- **Webhooks** verificados por firma HMAC (GitHub SHA-256/SHA-1, GitLab token),
  idempotentes por *delivery id*.
- Contenedores **distroless/rootless**; builds de cliente con **Kaniko rootless**,
  `ResourceQuota` + `NetworkPolicy` deny-by-default por namespace tenant.
- Datos y cómputo **solo en regiones UE** de Scaleway.

## Nota de despliegue (paridad dev/prod, §15)

El builder y el controller mantienen una única interfaz (`builder.Runner`,
`controller.Platform`, `controller.ServiceProvisioner`) con **dos
implementaciones** seleccionadas por configuración:

- **stub** (dev/local): no toca Kubernetes; el flujo
  cola → build → deploy → *ready* se recorre end-to-end sin clúster.
- **kube** (producción): implementación real con **client-go** (Apache-2.0) sobre
  **Kapsule**:
  - `builder.KubeJobClient` programa **Jobs de Kaniko** rootless (securityContext
    restrictivo, sin token de ServiceAccount, Secret del Registry montado en
    `/kaniko/.docker`), sigue su estado y recoge logs del Pod (→ S3).
  - `controller.KubePlatform` aplica de forma idempotente
    **Deployment + Service + Ingress (Traefik) + HPA** en el namespace del tenant
    (`tenant-<slug>`), que asegura con **ResourceQuota** y **NetworkPolicy
    deny-by-default**; gestiona **TLS con cert-manager** (recurso `Certificate`
    vía cliente dinámico) y verifica dominios propios por DNS.
  - `controller.KubeProvisioner` materializa los **servicios gestionados**
    (Sytue Postgres 18 + PostgREST con Secret/PVC/Deployment/Service; bucket S3
    para storage; aislamiento por tenant en Sytue ID para auth) e **inyecta las
    credenciales cifradas** (AES-256-GCM) en los deployments.

Selección: `RUNNER_MODE=kube|stub` (si se omite, se deduce del entorno: `prod` ⇒
kube, `dev`/`staging` ⇒ stub). Variables relevantes: `KUBECONFIG` (vacío ⇒
in-cluster), `BUILD_NAMESPACE`, `KANIKO_IMAGE`, `CERT_ISSUER`.

Los tests unitarios de la traducción a objetos de Kubernetes usan el *fake
clientset* de client-go (sin clúster); los que requieren un clúster real van tras
`//go:build integration` y se **saltan** si no hay `KUBECONFIG`.
