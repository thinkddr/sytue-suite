<a id="sytue-one--despliegue-en-producción-helm--scaleway-kapsule"></a>
# Sytue One — Production deployment (Helm / Scaleway Kapsule)

Helm charts to deploy the **Sytue One** suite on **Scaleway Kapsule** (Kubernetes
1.31, **EU regions only**). The `sytue-one` umbrella chart deploys the
**in-house Go services** (Sytue-native), the **LiveKit SFU** (Apache-2.0) and the
**Astro+Preact frontends**, on top of the canonical **Traefik** ingress and automatic TLS with
**cert-manager**.

> Source of truth: [`docs/CONVENTIONS.md`](../../docs/CONVENTIONS.md) (v4 "Sytue
> Native"). Names, domains, ports, OIDC clients and licenses match the contract
> exactly. Dev/prod parity: the local environment lives in
> [`deploy/compose`](../compose).

> **No AGPL engines** (Keycloak, Nextcloud, OnlyOffice, Mattermost, Stalwart) and **no
> Redis/Valkey**. Each product is an **original Go service** (clean-room on
> open standards) with **PostgreSQL** as the only datastore. The only third-party app
> is **LiveKit** (Apache-2.0) as the media SFU for Meet.

<a id="estructura"></a>
## Structure

```
deploy/helm/
├── README.md
├── sytue-one/                 # umbrella chart
│   ├── Chart.yaml             # deps upstream: traefik + cert-manager
│   │                          #   subcharts propios: id, drive, docs, mail, calendar,
│   │                          #   contacts, rooms, hub, cloud-api, cloud-builder,
│   │                          #   sytue-frontend + livekit (Apache-2.0)
│   ├── values.yaml            # valores BASE
│   ├── values-staging.yaml    # overlay staging (staging.sytue.eu, ACME staging)
│   ├── values-prod.yaml       # overlay prod (sytue.eu, ACME prod, HA)
│   └── templates/             # ClusterIssuers, NetworkPolicy baseline, NOTES
└── charts/                    # subcharts
    ├── id/                    # Sytue ID — OIDC/OAuth2/SCIM/WebAuthn (id.sytue.eu)
    ├── drive/                 # Sytue Drive — WebDAV + API (drive.sytue.eu)
    ├── docs/                  # Sytue Docs — colab CRDT/OT WS (docs.sytue.eu)
    ├── mail/                  # Sytue Mail — JMAP/HTTP + SMTP/IMAP (mail./mx.sytue.eu)
    ├── calendar/              # Sytue Calendar — CalDAV (calendar.sytue.eu)
    ├── contacts/              # Sytue Contacts — CardDAV (contacts.sytue.eu)
    ├── rooms/                 # Sytue Rooms — chat + llamadas (rooms.sytue.eu)
    ├── hub/                   # Sytue Hub — integración (hub.sytue.eu)
    ├── cloud-api/             # PaaS control-plane (cloud.sytue.eu/api)
    ├── cloud-builder/         # PaaS builder (Kaniko Jobs, interno)
    ├── analytics-ingest/      # Analytics — colector público (8094); SIN host propio:
    │                          #   /_sytue va en el dominio de cada cliente (ADR 0026)
    ├── analytics-api/         # Analytics — API de lectura + crons (interna, 8095)
    ├── livekit/               # SFU de medios de Meet (Apache-2.0, single-node)
    ├── sytue-frontend/        # superficies Astro+Preact (portal, console, mail, ...)
    └── tenant/                # plantilla de onboarding de tenant (namespace+quota+netpol)
```

Each Go service includes: **Deployment, Service, Ingress (className traefik), HPA,
NetworkPolicy and ServiceMonitor**. All templates carry the mandatory labels
`app.kubernetes.io/part-of=sytue-one`, `sytue.eu/env` and `sytue.eu/stack=go`.

<a id="requisitos-previos"></a>
## Prerequisites

- **Kapsule 1.31** cluster (Terraform in [`infra/terraform`](../../infra/terraform)).
- **EXTERNAL Postgres** (Scaleway RDB) — the chart does **not** deploy Postgres. It is the only
  datastore (state + queues + cache on Postgres; **no Redis/Valkey**).
- **S3 Object Storage** (fr-par, versioned) for blobs (Drive, Docs, Mail, PaaS).
- **Scaleway Container Registry** (`rg.fr-par.scw.cloud/sytue`) with the in-house images.
- Helm dependencies prepared (permissive upstreams only):

  ```sh
  helm repo add traefik https://traefik.github.io/charts     # ingress CANÓNICO (MIT)
  helm repo update
  helm dependency build deploy/helm/sytue-one
  ```

<a id="ingress-traefik-canónico-y-reparto-de-hosts"></a>
## Ingress (canonical Traefik) and host allocation

Traefik 3 is the **canonical ingress** (`CONVENTIONS.md §3`): `traefik.enabled=true` +
default `IngressClass`. All Ingresses use `className: traefik`.

On the shared subdomains, the **Astro SPA** (`sytue-frontend`) serves `/` and the
**Go backend** exposes its API/protocol under path prefixes (they coexist as two Ingresses
on the same host, merged by Traefik, with **a single TLS secret per host**):

| Host                 | `/` (frontend)        | Go backend (prefixes)                |
|----------------------|-----------------------|--------------------------------------|
| `id.sytue.eu`        | —                     | `/` (OIDC/JWKS/discovery/SCIM)       |
| `drive.sytue.eu`     | Drive SPA             | `/api`, `/dav` (WebDAV)              |
| `docs.sytue.eu`      | Docs editor           | `/api`, `/colab` (WS)                |
| `mail.sytue.eu`      | Webmail               | `/jmap`, `/api` (SMTP/IMAP → `mx`)   |
| `calendar.sytue.eu`  | Calendar SPA          | `/api`, `/dav` (CalDAV)              |
| `contacts.sytue.eu`  | Contacts SPA          | `/api`, `/dav` (CardDAV)             |
| `rooms.sytue.eu`     | Rooms SPA             | `/api`, `/ws`, `/hooks`              |
| `cloud.sytue.eu`     | PaaS dashboard        | `/api` (control-plane)               |

`mx.sytue.eu` exposes **SMTP (25/465/587)** and **IMAPS (993)** through a dedicated
`LoadBalancer` Service (not an HTTP Ingress).

<a id="secretos-nunca-en-git"></a>
## Secrets (NEVER in git)

They are managed with **Scaleway Secret Manager**, and `deploy/production/02b-secretos.sh`
materializes them as native Kubernetes `Secret`s. The charts only reference them
via `existingSecret`: they must exist in the namespace **before** installing. Only the
expected keys are shown (values as **placeholders**):

| Secret                   | Keys                                                |
|--------------------------|-----------------------------------------------------|
| `sytue-registry`         | `.dockerconfigjson` (private pull from `sytue-prod-platform`) |
| `sytue-postgresql`       | `host`, `port`                                      |
| `sytue-object-storage`   | `endpoint`, `access-key`, `secret-key`              |
| `sytue-id`               | `key-encryption-key`, `mollie-api-key` (prod)      |
| `sytue-id-db`            | `dsn`                                               |
| `sytue-<svc>`            | `oidc-client-secret` (svc: drive/docs/mail/calendar/contacts/rooms/hub) |
| `sytue-<svc>-db`         | `dsn` (one DB secret per service)                   |
| `sytue-livekit-keys`     | `api-key`, `api-secret`, `keys.yaml`                |
| `sytue-cloud-api`        | `oidc-client-secret`, `scaleway-api-key`, `control-db-url` |
| `sytue-cloud-api-db`     | `dsn`                                               |
| `sytue-cloud-builder`    | `scaleway-api-key`, `registry-token`                |
| `sytue-analytics`        | `key-encryption-key` (AES-256 KEK for the **daily salt**; without it the salt would be stored in plaintext) |
| `sytue-analytics-db`     | `dsn`, `dsn-direct` (shared by the collector and the API) |
| `sytue-frontend-bff`     | `cookie-secret`, `<app>-oidc-client-secret`         |

Materialization for production (the values do not appear on stdout or in Git):

```sh
cd deploy/production
./02b-secretos.sh prod
```

<a id="instalación"></a>
## Installation

```sh
# Staging
helm upgrade --install sytue-one deploy/helm/sytue-one \
  -n sytue-staging --create-namespace \
  -f deploy/helm/sytue-one/values.yaml \
  -f deploy/helm/sytue-one/values-staging.yaml

# Producción
helm upgrade --install sytue-one deploy/helm/sytue-one \
  -n sytue-prod --create-namespace \
  -f deploy/helm/sytue-one/values.yaml \
  -f deploy/helm/sytue-one/values-prod.yaml
```

> The ingress controller's namespace must carry the `sytue.eu/role=ingress` label
> so that the NetworkPolicies allow inbound traffic:
> `kubectl label ns <ns-ingress> sytue.eu/role=ingress`.

<a id="identidad-nuclear"></a>
## Identity (CORE)

**Sytue ID** (in-house Go service) is the backbone. The OIDC issuer
`https://id.<dominio>` (standard discovery at `/.well-known/openid-configuration`,
**without** `/realms/` — that was Keycloak, now retired) is injected into every component;
**each Go service validates JWTs via JWKS**. The OIDC clients (one BFF per surface) are `sytue-one`,
`sytue-console`, `sytue-cloud` (+ `sytue-cloud-api`), `sytue-mail`, `sytue-drive`,
`sytue-calendar`, `sytue-contacts`, `sytue-docs`, `sytue-chat`, `sytue-meet`,
`sytue-hub`. There are **no** engine federation clients (there are no engines anymore).

## Multi-tenant — onboarding

Each organization gets a `tenant-<slug>` namespace with ResourceQuota, LimitRange and
isolation NetworkPolicy:

```sh
helm install tenant-acme deploy/helm/charts/tenant \
  -n sytue-prod \
  --set tenant.slug=acme \
  --set tenant.displayName="ACME, S.L." \
  --set tenant.env=prod \
  --set tenant.plan=team
```

The logical mapping (group `org/acme` in Sytue ID, schema/RLS in PostgreSQL, mail
domain, S3 prefix) is recorded as annotations on the namespace; per-tenant data
provisioning is done by the Go services.

## TLS

`cert-manager` 1.21 is installed only once, as an independent cluster-wide release,
before the umbrella (the production flow does this automatically along with its CRDs). It issues
Let's Encrypt certificates via HTTP-01 over Traefik. The `ClusterIssuer`s
(`sytue-letsencrypt-staging` / `sytue-letsencrypt-prod`) are created afterwards by the umbrella
(`templates/cluster-issuers.yaml`). The overlay selects the issuer per environment.

<a id="observabilidad"></a>
## Observability

If `kube-prometheus-stack` is installed (cluster-wide), each component's `ServiceMonitor`s
expose metrics at `/metrics`. The discovery label is configurable in
`global.metrics.serviceMonitor.labels` (default `release: kube-prometheus-stack`).

<a id="validación-local-sin-desplegar"></a>
## Local validation (without deploying)

```sh
helm lint deploy/helm/sytue-one -f deploy/helm/sytue-one/values-prod.yaml
helm template sytue-one deploy/helm/sytue-one \
  -f deploy/helm/sytue-one/values.yaml \
  -f deploy/helm/sytue-one/values-prod.yaml | less
```

<a id="licencias"></a>
## Licenses

Product code **Apache-2.0 (© 2026 Daniel Delgado Ramos (thinkddr))** — no copyleft. Third-party dependencies
**permissive** only: Traefik (MIT), cert-manager (Apache-2.0), **LiveKit** (Apache-2.0,
the only third-party app). AGPL/GPL/source-available is **forbidden**. Matrix in
[`docs/security.md#5-licencias-propiedad-y-cumplimiento`](../../docs/security.md#5-licencias-propiedad-y-cumplimiento).
