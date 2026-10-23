# Sytue One — Entorno de desarrollo LOCAL (Docker Compose)

Levanta **toda la suite Sytue One** en tu máquina con un solo comando, bajo
`*.sytue.localhost` con TLS local (mkcert) vía Traefik.

**Stack único:** Go (backend) + Astro 7 / Preact 10.29 con Bun (frontend) + **PostgreSQL**
(estado, colas y caché) + **S3** (blobs; MinIO en dev). Cada producto es un
**servicio Go propio** (clean-room, Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr)) en `services/<svc>`.

**Sin motores AGPL** (Keycloak/Nextcloud/OnlyOffice/Mattermost/Stalwart quedan
**fuera**). **Sin Redis/Valkey.** Única app de terceros: **LiveKit** (Apache-2.0)
como SFU de medios de *Sytue Meet*. Paridad dev/prod: mismos clients OIDC,
dominios lógicos y servicios gestionados.

100 % permisivo · 100 % UE. Sin secretos reales: todo vía `.env` (placeholders).

## Requisitos

- Docker + Docker Compose v2
- [`mkcert`](https://github.com/FiloSottile/mkcert) (TLS local de confianza)
- Entradas `*.sytue.localhost` resueltas a `127.0.0.1`. En la mayoría de
  sistemas `*.localhost` ya resuelve a loopback; si no, añade a `/etc/hosts`
  los hosts de la tabla de URLs (o usa `dnsmasq`).

## Arranque en un comando

Desde la raíz del repo:

```bash
make dev-up
```

Eso, en orden, hace lo equivalente a:

```bash
cd deploy/compose

# 1) Variables locales
cp -n .env.example .env            # rellena los valores changeme-*

# 2) Certificado TLS local de confianza (mkcert)
mkcert -install
mkcert -cert-file traefik/certs/sytue-localhost.pem \
       -key-file  traefik/certs/sytue-localhost-key.pem \
       "sytue.localhost" "*.sytue.localhost"

# 3) Levantar toda la pila (build local de servicios Go + frontends Astro)
docker compose up -d --build
```

Comandos útiles:

| Acción | Comando |
|---|---|
| Levantar todo | `make dev-up` |
| Ver estado/salud | `docker compose ps` |
| Logs de un servicio | `docker compose logs -f id` |
| Sembrar datos (seed dev) | `make dev-seed` |
| Parar | `make dev-down` |
| Borrar volúmenes (reset total) | `make dev-clean` |

> `make` se define en la fase de integración (raíz del repo). Si aún no existe,
> usa los comandos `docker compose` equivalentes mostrados arriba.

## Memoria local: perfiles y frontends compilados (DEV-ONLY)

Estos ajustes son **solo de desarrollo**: acotan la RAM para que la suite
COMPLETA quepa en un local de **8 GB** (VM ~6 GiB). En **prod no aplican** — prod
se despliega por **Helm** (`deploy/helm`) con sus propios *requests/limits*. Los
valores del runtime Go (`GOMEMLIMIT`, `GOGC`, `GOMAXPROCS`, `DB_MAX_CONNS`) son
**variables de entorno**, no van compilados: la misma imagen sirve a prod con
otros valores.

**1) `make dev-up` sigue igual** (levanta la suite completa), pero ahora cada
contenedor tiene `mem_limit` y el runtime Go está afinado. `dev-up`/`dev-down`
leen `COMPOSE_PROFILES` del `.env` (por defecto `full`).

**2) Perfiles on-demand** — levanta solo lo que necesitas. El **núcleo**
(traefik, postgres, minio, dragonfly, id, hub) va SIEMPRE; cada app cuelga de su
perfil:

| Perfil | Servicios que añade al núcleo |
|---|---|
| `mail` | mail + mail-web |
| `drive` | drive + drive-web |
| `docs` | docs + docus-web + calcus-web + diapos-web + certipdf-web + ai |
| `calendar` | calendar + calendar-web |
| `contacts` | contacts + contacts-web |
| `rooms` | chat + chat-web + meet + livekit |
| `cloud` | cloud-api + cloud-builder + cloud-controller + postgrest + cloud-dashboard |
| `portal` | portal |
| `console` | console |
| `www` | www |
| `full` | **todas** las anteriores (suite completa) |

```bash
make dev-app P=docs,mail            # núcleo + docs + mail
COMPOSE_PROFILES=docs make dev-up   # equivalente para una sola app
```

**3) Frontends compilados (`frontlite.yml`)** — la palanca grande. En dev los 10
frontends corren como `astro dev` (~450 MiB c/u ≈ 4.4 GiB). El overlay
`frontlite.yml` los sirve **compilados** (SSR `runtime`, ~60-130 MiB c/u,
`mem_limit` 256m):

```bash
docker compose -f docker-compose.yml -f frontlite.yml up -d --build
```

Compila 7 de 10 (portal, mail, drive, calendar, contacts, docs, chat). Se
**excluyen** `console`, `cloud-dashboard` y `www` (siguen en modo dev): su nombre
de paquete `@sytue/app-*` no casa con el único `ARG APP` del Dockerfile (que lo
reutiliza para el filtro de bun **y** la ruta `apps/<APP>/dist`), y `www` es
`output: 'static'` (sin servidor SSR). El encabezado de `frontlite.yml` detalla
el uso y el *caveat* de las `PUBLIC_*` (requieren declarar `ARG PUBLIC_*` en el
Dockerfile para hornearse en el cliente; hoy solo declara `ARG APP`).

## URLs (dev, `*.sytue.localhost`)

Todos los frontends son Astro+Preact; todos los backends son **servicios Go**.

| Superficie | URL (cliente Astro) | Servicio Go (API) | Puerto Go |
|---|---|---|---|
| Sytue One (portal) | https://one.sytue.localhost | (usa Sytue ID + Hub) | — |
| Sytue ID (login/admin) | https://id.sytue.localhost | Sytue ID (OIDC/SCIM) | 8081 |
| Sytue Drive | https://drive.sytue.localhost | https://drive-dav.sytue.localhost (WebDAV) | 8082 |
| Sytue Docs | https://docs.sytue.localhost | https://docs-api.sytue.localhost (colab WS) | 8083 |
| **Sytue Rooms** (mensajería + llamadas) | https://rooms.sytue.localhost | https://rooms-api.sytue.localhost (WS) | 8085 |
| Sytue Mail | https://mail.sytue.localhost | https://mail-api.sytue.localhost (JMAP) · https://mx.sytue.localhost | 8086 |
| Sytue Calendar | https://calendar.sytue.localhost | https://calendar-dav.sytue.localhost (CalDAV) | 8087 |
| Sytue Contacts | https://contacts.sytue.localhost | https://contacts-dav.sytue.localhost (CardDAV) | 8088 |
| Sytue Cloud (dashboard) | https://cloud.sytue.localhost | https://cloud-api.sytue.localhost | 8090 |
| Sytue Cloud builder | — (interno) | — | 8091 |
| Sytue Cloud DB (PostgREST) | https://db.sytue.localhost | PostgREST | — |
| Sytue Admin | https://admin.sytue.localhost | (admin sobre Sytue ID + Hub) | — |
| Sytue Hub | — | https://hub.sytue.localhost | 8092 |
| Sitio público (www) | https://sytue.localhost · https://www.sytue.localhost | — | — |
| Traefik dashboard | https://traefik.sytue.localhost | :8080 | — |
| MinIO API / consola | https://minio.sytue.localhost · https://minio-admin.sytue.localhost | :9000 / :9001 | — |

**LiveKit (SFU de medios de Sytue Meet, Apache-2.0):** señalización WS en
https://rooms-sfu.sytue.localhost. **Hosts legados**: `chat.sytue.localhost` y
`meet.sytue.localhost` responden **301 permanente** a `rooms.sytue.localhost`
preservando el path (Sytue Rooms unifica ambos productos).
Puertos directos a host (sin Traefik):
`7880` (WS), `7881` (TCP media) y `50000-50100/udp` (media WebRTC).

Otros puertos directos a host: PostgreSQL `5432`, Sytue Mail SMTP/IMAP
`25/465/587/993`, Cloud builder `8091`. (Puertos según `docs/CONVENTIONS.md §6`.)

> **Paridad dev/prod.** El **cliente Astro+Preact** de cada producto ocupa el subdominio
> canónico (`drive/calendar/contacts/...sytue.localhost`), igual que en prod (`drive.sytue.eu`),
> y su BFF habla con el servicio Go por la red interna. Para probar los protocolos DAV
> directamente, cada backend expone además `*-dav.sytue.localhost` (WebDAV/CalDAV/CardDAV);
> las demás API usan subdominio propio (`docs-api`, `rooms-api`/`rooms-sfu`, `mail-api`/`mx`).

## Arquitectura del directorio

```
deploy/compose/
├── docker-compose.yml         # toda la pila (servicios Go + frontends + infra)
├── frontlite.yml              # overlay DEV-ONLY: frontends SSR compilados (−RAM)
├── docker-compose.override.yml # LOCAL, no versionado (p.ej. fijar imagen de postgres)
├── .env.example               # placeholders dev (copiar a .env)
├── traefik/
│   ├── traefik.yml            # config estática (entrypoints, docker provider)
│   ├── dynamic/tls.yml        # TLS mkcert + middlewares
│   └── certs/                 # certificados locales (no versionados)
├── postgres/init/             # creación de UNA BD/rol por servicio Go
├── minio/minio-init.sh        # buckets S3 dev (drive/mail/docs blobs, cloud…)
└── livekit/livekit.yaml       # config del SFU LiveKit (Sytue Meet)
```

## Identidad (Sytue ID es NUCLEAR)

**Sytue ID** es un servicio **Go propio** (`services/id`) que implementa
OIDC/OAuth2 (Authorization Code + PKCE, refresh, JWKS, discovery), SCIM y
MFA/WebAuthn. Cada servicio Go **valida los JWT de Sytue ID** vía JWKS
(middleware `authn` de `platform`). Los frontends usan `@sytue/auth` (OIDC+PKCE)
con sesión única en cookie httpOnly de dominio `.sytue.localhost`.

**Sytue Meet:** el BFF `meet` mintea el JWT de LiveKit (API key/secret de
`.env`) **tras** validar la sesión OIDC.

## Datos y blobs

- **PostgreSQL** es el único datastore: estado, colas y caché (LISTEN/NOTIFY,
  `pgmq`/`riverqueue`). Una **BD + rol por servicio** (aislamiento); multi-tenant
  por esquema/RLS dentro de cada BD. **Sin Redis/Valkey.**
- **S3 (MinIO en dev)** solo para blobs grandes: ficheros de Drive, adjuntos de
  Mail, documentos de Docs, build-cache del PaaS, backups.

## Notas

- **TLS**: instala el CA de mkcert (`mkcert -install`) para que el navegador
  confíe en `*.sytue.localhost`.
- **Build de servicios Go**: el contexto es la **raíz del repo** (módulo único
  `git.sytue.eu/sytue/one`); el `Dockerfile` vive en cada `services/<svc>/`
  (multi-stage `golang:1.23` → `distroless/static`).
- **Reset**: `make dev-clean` borra volúmenes (BD, buckets). Tras un reset, las
  BD por servicio y los buckets se recrean al arrancar.
```
