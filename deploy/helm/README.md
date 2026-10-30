# Sytue One — Despliegue en producción (Helm / Scaleway Kapsule)

Charts Helm para desplegar la suite **Sytue One** en **Scaleway Kapsule** (Kubernetes
1.31, **solo regiones UE**). El chart paraguas (umbrella) `sytue-one` despliega los
**servicios Go propios** (Sytue-native), el **SFU LiveKit** (Apache-2.0) y los
**frontends Astro+Preact**, sobre el ingress canónico **Traefik** y TLS automático
**cert-manager**.

> Fuente de verdad: [`docs/CONVENTIONS.md`](../../docs/CONVENTIONS.md) (v4 "Sytue
> Native"). Nombres, dominios, puertos, clients OIDC y licencias coinciden exactamente
> con el contrato. Paridad dev/prod: el entorno local vive en
> [`deploy/compose`](../compose).

> **Sin motores AGPL** (Keycloak, Nextcloud, OnlyOffice, Mattermost, Stalwart) y **sin
> Redis/Valkey**. Cada producto es un **servicio Go original** (clean-room sobre
> estándares abiertos) con **PostgreSQL** como único datastore. La única app de terceros
> es **LiveKit** (Apache-2.0) como SFU de medios de Meet.

## Estructura

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

Cada servicio Go incluye: **Deployment, Service, Ingress (className traefik), HPA,
NetworkPolicy y ServiceMonitor**. Todas las plantillas llevan las labels obligatorias
`app.kubernetes.io/part-of=sytue-one`, `sytue.eu/env` y `sytue.eu/stack=go`.

## Requisitos previos

- Cluster **Kapsule 1.31** (Terraform en [`infra/terraform`](../../infra/terraform)).
- **Postgres EXTERNO** (Scaleway RDB) — el chart **no** despliega Postgres. Es el único
  datastore (estado + colas + caché sobre Postgres; **sin Redis/Valkey**).
- **Object Storage S3** (fr-par, versionado) para blobs (Drive, Docs, Mail, PaaS).
- **Scaleway Container Registry** (`rg.fr-par.scw.cloud/sytue`) con las imágenes propias.
- Dependencias Helm preparadas (solo upstream permisivos):

  ```sh
  helm repo add traefik https://traefik.github.io/charts     # ingress CANÓNICO (MIT)
  helm repo update
  helm dependency build deploy/helm/sytue-one
  ```

## Ingress (Traefik canónico) y reparto de hosts

Traefik 3 es el **ingress canónico** (`CONVENTIONS.md §3`): `traefik.enabled=true` +
`IngressClass` por defecto. Todos los Ingress usan `className: traefik`.

En los subdominios compartidos, el **SPA Astro** (`sytue-frontend`) sirve `/` y el
**backend Go** expone su API/protocolo bajo prefijos de ruta (coexisten como dos Ingress
sobre el mismo host, fusionados por Traefik, con **un único secret TLS por host**):

| Host                 | `/` (frontend)        | Backend Go (prefijos)                |
|----------------------|-----------------------|--------------------------------------|
| `id.sytue.eu`        | —                     | `/` (OIDC/JWKS/discovery/SCIM)       |
| `drive.sytue.eu`     | SPA Drive             | `/api`, `/dav` (WebDAV)              |
| `docs.sytue.eu`      | Editor Docs           | `/api`, `/colab` (WS)                |
| `mail.sytue.eu`      | Webmail               | `/jmap`, `/api` (SMTP/IMAP → `mx`)   |
| `calendar.sytue.eu`  | SPA Calendar          | `/api`, `/dav` (CalDAV)              |
| `contacts.sytue.eu`  | SPA Contacts          | `/api`, `/dav` (CardDAV)             |
| `rooms.sytue.eu`     | SPA Rooms             | `/api`, `/ws`, `/hooks`              |
| `cloud.sytue.eu`     | Dashboard PaaS        | `/api` (control-plane)               |

`mx.sytue.eu` publica **SMTP (25/465/587)** e **IMAPS (993)** vía un Service
`LoadBalancer` dedicado (no Ingress HTTP).

## Secretos (NUNCA en git)

Se gestionan con **Scaleway Secret Manager** y `deploy/production/02b-secretos.sh`
los materializa como `Secret` nativos de Kubernetes. Los charts solo los referencian
por `existingSecret`: deben existir en el namespace **antes** de instalar. Solo se
muestran las claves esperadas (valores como **placeholders**):

| Secret                   | Claves                                              |
|--------------------------|-----------------------------------------------------|
| `sytue-registry`         | `.dockerconfigjson` (pull privado de `sytue-prod-platform`) |
| `sytue-postgresql`       | `host`, `port`                                      |
| `sytue-object-storage`   | `endpoint`, `access-key`, `secret-key`              |
| `sytue-id`               | `key-encryption-key`, `mollie-api-key` (prod)      |
| `sytue-id-db`            | `dsn`                                               |
| `sytue-<svc>`            | `oidc-client-secret` (svc: drive/docs/mail/calendar/contacts/rooms/hub) |
| `sytue-<svc>-db`         | `dsn` (un secret de BD por servicio)                |
| `sytue-livekit-keys`     | `api-key`, `api-secret`, `keys.yaml`                |
| `sytue-cloud-api`        | `oidc-client-secret`, `scaleway-api-key`, `control-db-url` |
| `sytue-cloud-api-db`     | `dsn`                                               |
| `sytue-cloud-builder`    | `scaleway-api-key`, `registry-token`                |
| `sytue-analytics`        | `key-encryption-key` (KEK AES-256 de la **sal diaria**; sin ella la sal se guardaría en claro) |
| `sytue-analytics-db`     | `dsn`, `dsn-direct` (la comparten colector y API)   |
| `sytue-frontend-bff`     | `cookie-secret`, `<app>-oidc-client-secret`         |

Materialización para producción (los valores no aparecen en stdout ni en Git):

```sh
cd deploy/production
./02b-secretos.sh prod
```

## Instalación

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

> El namespace del ingress controller debe llevar la etiqueta `sytue.eu/role=ingress`
> para que las NetworkPolicy permitan el tráfico entrante:
> `kubectl label ns <ns-ingress> sytue.eu/role=ingress`.

## Identidad (NUCLEAR)

**Sytue ID** (servicio Go propio) es la columna vertebral. El issuer OIDC
`https://id.<dominio>` (discovery estándar en `/.well-known/openid-configuration`,
**sin** `/realms/` — eso era Keycloak, retirado) se inyecta en cada componente;
**cada servicio Go valida los JWT vía JWKS**. Los clients OIDC (BFF por superficie) son `sytue-one`,
`sytue-console`, `sytue-cloud` (+ `sytue-cloud-api`), `sytue-mail`, `sytue-drive`,
`sytue-calendar`, `sytue-contacts`, `sytue-docs`, `sytue-chat`, `sytue-meet`,
`sytue-hub`. **No** hay clients de federación de motores (ya no hay motores).

## Multi-tenant — onboarding

Cada organización obtiene un namespace `tenant-<slug>` con ResourceQuota, LimitRange y
NetworkPolicy de aislamiento:

```sh
helm install tenant-acme deploy/helm/charts/tenant \
  -n sytue-prod \
  --set tenant.slug=acme \
  --set tenant.displayName="ACME, S.L." \
  --set tenant.env=prod \
  --set tenant.plan=team
```

El mapeo lógico (grupo `org/acme` en Sytue ID, esquema/RLS en PostgreSQL, dominio de
correo, prefijo S3) queda anotado en el namespace; el aprovisionamiento de datos por
tenant lo realizan los servicios Go.

## TLS

`cert-manager` 1.21 se instala una sola vez, como release cluster-wide independiente,
antes del umbrella (el flujo de producción lo hace automáticamente con sus CRD). Emite
certificados Let's Encrypt vía HTTP-01 sobre Traefik. Los `ClusterIssuer`
(`sytue-letsencrypt-staging` / `sytue-letsencrypt-prod`) los crea después el umbrella
(`templates/cluster-issuers.yaml`). El overlay selecciona el issuer por entorno.

## Observabilidad

Si `kube-prometheus-stack` está instalado (cluster-wide), los `ServiceMonitor` de cada
componente exponen métricas en `/metrics`. Etiqueta de descubrimiento configurable en
`global.metrics.serviceMonitor.labels` (por defecto `release: kube-prometheus-stack`).

## Validación local (sin desplegar)

```sh
helm lint deploy/helm/sytue-one -f deploy/helm/sytue-one/values-prod.yaml
helm template sytue-one deploy/helm/sytue-one \
  -f deploy/helm/sytue-one/values.yaml \
  -f deploy/helm/sytue-one/values-prod.yaml | less
```

## Licencias

Código de producto **Apache-2.0 (© 2026 Daniel Delgado Ramos (thinkddr))** — sin copyleft. Dependencias de terceros
solo **permisivas**: Traefik (MIT), cert-manager (Apache-2.0), **LiveKit** (Apache-2.0,
única app de terceros). **Prohibido** AGPL/GPL/source-available. Matriz en
[`docs/security.md#5-licencias-propiedad-y-cumplimiento`](../../docs/security.md#5-licencias-propiedad-y-cumplimiento).
