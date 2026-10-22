# Sytue Mail (`services/mail`)

Servidor de correo **Sytue-native** de la suite **Sytue One**: SMTP entrante
(**MX**) y saliente (**Submission/MSA**), **IMAP** y una **API JMAP propia** para
el cliente web. Reimplementación **clean-room** sobre estándares abiertos
(**RFC 5321/5322** SMTP/IMF, **RFC 3501/9051** IMAP, **RFC 8620/8621** JMAP,
**RFC 7208** SPF, **RFC 6376** DKIM, **RFC 7489** DMARC) con librerías Go
**permisivas** (MIT: `emersion/go-smtp`, `go-imap/v2`, `go-message`,
`go-msgauth`). Código propietario © Sytue. **Sin motores de correo de terceros
AGPL** (Stalwart, etc. quedan fuera).

- **Puertos:** `8086` (HTTP/JMAP) · `25` (SMTP MX) · `465` (SMTPS) · `587`
  (Submission) · `993` (IMAPS) — docs/CONVENTIONS.md §6.
- **Subdominios:** `mail.sytue.eu` (web/JMAP), `mx.sytue.eu` (MX/EHLO).
- **Datastore:** **PostgreSQL** (cuentas, buzones, sobres de mensaje, **cola de
  salida**, rate-limit, outbox de eventos) — única fuente de verdad. **Sin
  Redis/Valkey** (cola sobre Postgres con `FOR UPDATE SKIP LOCKED` + LISTEN/NOTIFY).
- **Blobs:** Object Storage S3 (Scaleway prod / MinIO dev), bucket `mail-blobs`,
  aislados por **prefijo de tenant**. Guarda el **.eml crudo** y los adjuntos.
- **Auth (JMAP/IMAP/SMTP):** **Sytue ID**. La API JMAP valida el JWT vía JWKS
  (`platform/pkg/authn`); SMTP/IMAP AUTH presentan dirección + token de acceso de
  Sytue ID (o "contraseña de aplicación" JWT con scope `mail`) — sin almacén de
  contraseñas propio.
- **Multi-tenant:** aislamiento por **RLS** en PostgreSQL (`app.tenant`) + prefijo
  S3 por tenant.

## Componentes

| Subsistema | Estándar | Descripción |
|---|---|---|
| **MX entrante** (`internal/smtp`, `:25`) | RFC 5321 | Recibe correo de otros MTA. Verifica **SPF/DKIM/DMARC**, rate-limit por IP, parsea MIME, entrega al Inbox (o Junk si DMARC falla). **No** ofrece AUTH (anti-relay / anti-fuerza bruta). |
| **Submission/MSA** (`internal/smtp`, `:587`/`:465`) | RFC 6409 | Clientes **autenticados** envían correo. Exige AUTH (Sytue ID), valida `MAIL FROM` == identidad (anti-spoofing), firma DKIM y encola. |
| **IMAP** (`internal/imap`, `:993`) | RFC 3501/9051 | Acceso de clientes de escritorio/móvil. UID/UIDVALIDITY servidos desde Postgres; cuerpos desde S3. |
| **JMAP/REST** (`internal/jmap`, `:8086`) | RFC 8620/8621 | API moderna del cliente web (`@sytue/api-clients`): sesión, buzones, mensajes, flags, mover/borrar, envío. |
| **Entrega** (`internal/delivery`) | — | Orquesta recepción y envío; **worker de cola** sobre Postgres con reintentos y **backoff exponencial + jitter**, relay SMTP saliente con **STARTTLS oportunista** y resolución MX. |
| **Auth de correo** (`internal/auth`) | RFC 6376/7208/7489 | Firma DKIM (Ed25519/RSA) de salientes; verificación SPF/DKIM/DMARC de entrantes. |
| **MIME** (`internal/mime`) | RFC 5322/2045 | Parseo del sobre indexable (asunto, remitente, destinatarios, Message-ID, threading, snippet, adjuntos) con `go-message`. |

## Estructura

```
services/mail/
├── cmd/mail/main.go            # binario único: arranca HTTP/JMAP + SMTP(MX/MSA) + IMAP + worker de cola
├── internal/
│   ├── config/                 # carga de configuración (env, 12-factor)
│   ├── domain/                 # tipos y reglas de negocio (sin transporte)
│   ├── store/                  # persistencia pgx + RLS por tenant; cola FOR UPDATE SKIP LOCKED
│   ├── mime/                   # parseo RFC 5322 / MIME (go-message)
│   ├── auth/                   # firma DKIM + verificación SPF/DKIM/DMARC
│   ├── delivery/               # recepción, envío y worker de cola con reintentos
│   ├── smtp/                   # backends MX (entrante) y submission (saliente)
│   ├── imap/                   # servidor IMAP (go-imap v2)
│   ├── jmap/                   # API JMAP + REST de conveniencia (httpx + authn)
│   └── idauth/                 # AUTH SMTP/IMAP delegada en Sytue ID (JWKS)
├── db/migrations/              # migraciones golang-migrate (.up/.down.sql)
├── openapi.yaml                # contrato de la API JMAP/REST
└── Dockerfile                  # multi-stage golang -> distroless/static (rootless)
```

> **Dependencias de terceros** (todas permisivas MIT, conservar NOTICE; el agente
> de Plataforma las añade al `go.mod` de la raíz): `github.com/emersion/go-smtp`,
> `github.com/emersion/go-imap/v2`, `github.com/emersion/go-message`,
> `github.com/emersion/go-msgauth`, `github.com/emersion/go-sasl`. La superficie
> de `go-imap/v2` (`imapserver.Session`) evoluciona entre versiones; se fija por
> digest y se concilia en build (política "última estable" de §3).

## Configuración (variables de entorno)

| Variable | Defecto | Descripción |
|---|---|---|
| `PORT` | `8086` | Puerto HTTP/JMAP |
| `PUBLIC_URL` | `https://mail.sytue.localhost` | URL pública (URLs JMAP) |
| `MAIL_DOMAIN` | `sytue.localhost` | Dominio de correo principal |
| `MAIL_HOSTNAME` | `mx.sytue.localhost` | FQDN del MTA (EHLO / Received) |
| `DATABASE_URL` | — (obligatoria) | DSN PostgreSQL |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Issuer de Sytue ID |
| `OIDC_JWKS_URL` | `<issuer>/.well-known/jwks.json` | Endpoint JWKS |
| `OIDC_AUDIENCE` | `sytue-mail` | Audiencia esperada del token |
| `S3_ENDPOINT` | `minio:9000` | Endpoint S3 (acepta `http(s)://host:port`) |
| `S3_REGION` | `fr-par` | Región S3 (solo UE) |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | — (obligatorias) | Credenciales S3 |
| `S3_BUCKET` | `sytue-dev-mail-blobs` | Bucket de blobs |
| `S3_USE_SSL` | `false` | TLS hacia S3 (`true` en prod) |
| `SMTP_MX_ADDR` | `:25` | Listener MX entrante |
| `SMTP_SUBMISSION_ADDR` | `:587` | Listener Submission (STARTTLS) |
| `SMTPS_ADDR` | `:465` | Listener Submission TLS implícito |
| `IMAPS_ADDR` | `:993` | Listener IMAPS |
| `TLS_CERT_FILE` / `TLS_KEY_FILE` | — | Cert/clave PEM (obligatorios en prod; en dev Traefik termina TLS) |
| `MAX_MESSAGE_MB` | `50` | Tamaño máximo de mensaje |
| `MAX_RECIPIENTS` | `100` | Destinatarios por transacción SMTP |
| `DKIM_SELECTOR` | `sytue` | Selector DNS DKIM |
| `DKIM_PRIVATE_KEY` | — | Clave privada DKIM PEM (Secret Manager; vacía => sin firma, solo dev) |
| `QUEUE_WORKERS` | `4` | Workers de la cola de salida |
| `QUEUE_MAX_ATTEMPTS` | `8` | Reintentos antes de bounce |
| `QUEUE_BASE_BACKOFF` | `30s` | Backoff base (exponencial + jitter) |
| `VERIFY_SPF` / `VERIFY_DKIM` / `ENFORCE_DMARC` | `true` | Verificación de entrantes |
| `REJECT_ON_DMARC_FAIL` | `false` | Rechazar (vs. cuarentena) cuando DMARC=reject |
| `MAX_INBOUND_PER_MIN_PER_IP` | `60` | Rate-limit del MX por IP |

No hay secretos en el repositorio: las credenciales llegan por entorno
(Scaleway Secret Manager / Sealed Secrets en prod).

## Ejecución

```sh
# Migraciones (se aplican también al arrancar) + todos los servidores.
go run ./services/mail/cmd/mail
```

En la suite completa se levanta con `make dev-up` (Docker Compose); el contexto
de build de la imagen es la **raíz del repo** (módulo único `git.sytue.eu/sytue/one`).

## DNS requerido por dominio servido

- **MX:** `mail IN MX 10 mx.sytue.eu.`
- **SPF:** `sytue.eu IN TXT "v=spf1 mx -all"`
- **DKIM:** `sytue._domainkey.sytue.eu IN TXT "v=DKIM1; k=ed25519; p=<pubkey>"`
- **DMARC:** `_dmarc.sytue.eu IN TXT "v=DMARC1; p=quarantine; rua=mailto:dmarc@sytue.eu"`
- **MTA-STS / TLS-RPT** recomendados (CONVENTIONS §10).

## Nota de hardening y anti-abuso

Seguridad por defecto / por diseño (CONVENTIONS §0.7), criterio de *Definition of
Done*:

- **Anti-relay:** el **MX entrante NO ofrece AUTH** y solo acepta `RCPT TO` de
  direcciones **locales conocidas** (rechazo `550 5.1.1` en desconocidas). El
  **Submission** exige AUTH antes de `MAIL FROM` y obliga a que el remitente
  coincida con la **identidad autenticada** (`550 5.7.1`, anti-spoofing).
- **Autenticación de remitente entrante:** **SPF + DKIM + DMARC**. DMARC `reject`
  puede **rechazar** en SMTP (configurable) o desviar a **Junk** (cuarentena); el
  veredicto se persiste por mensaje (RFC 8601) para auditoría.
- **Rate-limiting:** por **IP/minuto** en el MX (ventana sobre Postgres, sin
  Redis/Valkey) con respuesta `421 4.7.0`; GC periódico de la ventana.
- **Límites de recurso:** tamaño máximo de mensaje, número máximo de
  destinatarios, longitud de línea, y **timeouts** de lectura/escritura por
  conexión SMTP/IMAP (anti-slowloris).
- **TLS:** **STARTTLS** en Submission (587) y **TLS implícito** en 465/993; el MX
  saliente usa **STARTTLS oportunista**. En prod, TLS 1.3 (negociado) con cert de
  cert-manager; en dev Traefik termina TLS.
- **Credenciales:** delegadas en **Sytue ID** (sin almacén de contraseñas; scope
  `mail` requerido — mínimo privilegio). Clave DKIM solo en **Secret Manager**,
  nunca en git ni en logs.
- **Aislamiento multi-tenant:** **RLS deny-by-default** en todas las tablas; las
  escrituras del MX fijan `app.tenant` tras resolver la dirección destino.
- **Cola robusta:** entrega diferida con **reintentos + backoff exponencial con
  jitter** y reparto seguro (`FOR UPDATE SKIP LOCKED`); agotados los intentos,
  **bounce** registrado.
- **Privacidad (RGPD):** datos **solo en UE**; logs **sin** contenido de correo
  ni PII; cuerpos cifrados en reposo (S3); borrado **soft** + GC diferido de blobs.
- **Pendiente de endurecer antes de prod:** evaluador SPF dedicado/auditado,
  greylisting opcional, integración de listas (RBL) configurable, ARC para
  reenvíos, y límites de envío saliente por tenant (anti-spam de salida).
