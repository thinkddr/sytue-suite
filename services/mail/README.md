# Sytue Mail (`services/mail`)

**Sytue-native** mail server of the **Sytue One** suite: inbound (**MX**) and
outbound (**Submission/MSA**) SMTP, **IMAP** and an **in-house JMAP API** for
the web client. A **clean-room** reimplementation on open standards
(**RFC 5321/5322** SMTP/IMF, **RFC 3501/9051** IMAP, **RFC 8620/8621** JMAP,
**RFC 7208** SPF, **RFC 6376** DKIM, **RFC 7489** DMARC) with **permissive** Go
libraries (MIT: `emersion/go-smtp`, `go-imap/v2`, `go-message`,
`go-msgauth`). Code Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr). **No third-party
AGPL mail engines** (Stalwart, etc. are out).

- **Ports:** `8086` (HTTP/JMAP) · `25` (SMTP MX) · `465` (SMTPS) · `587`
  (Submission) · `993` (IMAPS) — docs/CONVENTIONS.md §6.
- **Subdomains:** `mail.sytue.eu` (web/JMAP), `mx.sytue.eu` (MX/EHLO).
- **Datastore:** **PostgreSQL** (accounts, mailboxes, message envelopes, **outbound
  queue**, rate limit, event outbox) — single source of truth. **No
  Redis/Valkey** (queue on Postgres with `FOR UPDATE SKIP LOCKED` + LISTEN/NOTIFY).
- **Blobs:** S3 Object Storage (Scaleway prod / MinIO dev), bucket `mail-blobs`,
  isolated by **tenant prefix**. Stores the **raw .eml** and the attachments.
- **Auth (JMAP/IMAP/SMTP):** **Sytue ID**. The JMAP API validates the JWT via JWKS
  (`platform/pkg/authn`); SMTP/IMAP AUTH present address + Sytue ID access
  token (or a JWT "app password" with `mail` scope) — no password store of
  its own.
- **Multi-tenant:** isolation via **RLS** in PostgreSQL (`app.tenant`) + per-tenant S3
  prefix.

<a id="componentes"></a>
## Components

| Subsystem | Standard | Description |
|---|---|---|
| **Inbound MX** (`internal/smtp`, `:25`) | RFC 5321 | Receives mail from other MTAs. Verifies **SPF/DKIM/DMARC**, rate-limits per IP, parses MIME, delivers to the Inbox (or Junk if DMARC fails). Does **not** offer AUTH (anti-relay / anti-brute-force). |
| **Submission/MSA** (`internal/smtp`, `:587`/`:465`) | RFC 6409 | **Authenticated** clients send mail. Requires AUTH (Sytue ID), validates `MAIL FROM` == identity (anti-spoofing), signs DKIM and enqueues. |
| **IMAP** (`internal/imap`, `:993`) | RFC 3501/9051 | Access for desktop/mobile clients. UID/UIDVALIDITY served from Postgres; bodies from S3. |
| **JMAP/REST** (`internal/jmap`, `:8086`) | RFC 8620/8621 | Modern API of the web client (`@sytue/api-clients`): session, mailboxes, messages, flags, move/delete, send. |
| **Delivery** (`internal/delivery`) | — | Orchestrates receiving and sending; **queue worker** on Postgres with retries and **exponential backoff + jitter**, outbound SMTP relay with **opportunistic STARTTLS** and MX resolution. |
| **Mail auth** (`internal/auth`) | RFC 6376/7208/7489 | DKIM signing (Ed25519/RSA) of outbound mail; SPF/DKIM/DMARC verification of inbound mail. |
| **MIME** (`internal/mime`) | RFC 5322/2045 | Parsing of the indexable envelope (subject, sender, recipients, Message-ID, threading, snippet, attachments) with `go-message`. |

<a id="estructura"></a>
## Structure

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

> **Third-party dependencies** (all permissive MIT, keep the NOTICE; the Platform
> agent adds them to the root `go.mod`): `github.com/emersion/go-smtp`,
> `github.com/emersion/go-imap/v2`, `github.com/emersion/go-message`,
> `github.com/emersion/go-msgauth`, `github.com/emersion/go-sasl`. The surface
> of `go-imap/v2` (`imapserver.Session`) evolves between versions; it is pinned by
> digest and reconciled at build time ("latest stable" policy of §3).

<a id="configuración-variables-de-entorno"></a>
## Configuration (environment variables)

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8086` | HTTP/JMAP port |
| `PUBLIC_URL` | `https://mail.sytue.localhost` | Public URL (JMAP URLs) |
| `MAIL_DOMAIN` | `sytue.localhost` | Primary mail domain |
| `MAIL_HOSTNAME` | `mx.sytue.localhost` | MTA FQDN (EHLO / Received) |
| `DATABASE_URL` | — (required) | PostgreSQL DSN |
| `OIDC_ISSUER` | `https://id.sytue.localhost` | Sytue ID issuer |
| `OIDC_JWKS_URL` | `<issuer>/.well-known/jwks.json` | JWKS endpoint |
| `OIDC_AUDIENCE` | `sytue-mail` | Expected token audience |
| `S3_ENDPOINT` | `minio:9000` | S3 endpoint (accepts `http(s)://host:port`) |
| `S3_REGION` | `fr-par` | S3 region (EU only) |
| `S3_ACCESS_KEY` / `S3_SECRET_KEY` | — (required) | S3 credentials |
| `S3_BUCKET` | `sytue-dev-mail-blobs` | Blob bucket |
| `S3_USE_SSL` | `false` | TLS to S3 (`true` in prod) |
| `SMTP_MX_ADDR` | `:25` | Inbound MX listener |
| `SMTP_SUBMISSION_ADDR` | `:587` | Submission listener (STARTTLS) |
| `SMTPS_ADDR` | `:465` | Implicit-TLS Submission listener |
| `IMAPS_ADDR` | `:993` | IMAPS listener |
| `TLS_CERT_FILE` / `TLS_KEY_FILE` | — | PEM cert/key (required in prod; in dev Traefik terminates TLS) |
| `MAX_MESSAGE_MB` | `50` | Maximum message size |
| `MAX_RECIPIENTS` | `100` | Recipients per SMTP transaction |
| `DKIM_SELECTOR` | `sytue` | DKIM DNS selector |
| `DKIM_PRIVATE_KEY` | — | DKIM PEM private key (Secret Manager; empty => no signing, dev only) |
| `QUEUE_WORKERS` | `4` | Outbound queue workers |
| `QUEUE_MAX_ATTEMPTS` | `8` | Retries before bounce |
| `QUEUE_BASE_BACKOFF` | `30s` | Base backoff (exponential + jitter) |
| `VERIFY_SPF` / `VERIFY_DKIM` / `ENFORCE_DMARC` | `true` | Inbound verification |
| `REJECT_ON_DMARC_FAIL` | `false` | Reject (vs. quarantine) when DMARC=reject |
| `MAX_INBOUND_PER_MIN_PER_IP` | `60` | MX rate limit per IP |

There are no secrets in the repository: credentials arrive via the environment
(Scaleway Secret Manager / Sealed Secrets in prod).

<a id="ejecución"></a>
## Running

```sh
# Migraciones (se aplican también al arrancar) + todos los servidores.
go run ./services/mail/cmd/mail
```

In the full suite it is brought up with `make dev-up` (Docker Compose); the image's build
context is the **repo root** (single module `git.sytue.eu/sytue/one`).

<a id="dns-requerido-por-dominio-servido"></a>
## DNS required per served domain

- **MX:** `mail IN MX 10 mx.sytue.eu.`
- **SPF:** `sytue.eu IN TXT "v=spf1 mx -all"`
- **DKIM:** `sytue._domainkey.sytue.eu IN TXT "v=DKIM1; k=ed25519; p=<pubkey>"`
- **DMARC:** `_dmarc.sytue.eu IN TXT "v=DMARC1; p=quarantine; rua=mailto:dmarc@sytue.eu"`
- **MTA-STS / TLS-RPT** recommended (CONVENTIONS §10).

<a id="nota-de-hardening-y-anti-abuso"></a>
## Hardening and anti-abuse note

Secure by default / by design (CONVENTIONS §0.7), a *Definition of
Done* criterion:

- **Anti-relay:** the **inbound MX does NOT offer AUTH** and only accepts `RCPT TO` for
  **known local** addresses (`550 5.1.1` rejection for unknown ones). The
  **Submission** requires AUTH before `MAIL FROM` and forces the sender to
  match the **authenticated identity** (`550 5.7.1`, anti-spoofing).
- **Inbound sender authentication:** **SPF + DKIM + DMARC**. DMARC `reject`
  can **reject** at SMTP time (configurable) or divert to **Junk** (quarantine); the
  verdict is persisted per message (RFC 8601) for auditing.
- **Rate limiting:** per **IP/minute** at the MX (window on Postgres, no
  Redis/Valkey) with a `421 4.7.0` response; periodic GC of the window.
- **Resource limits:** maximum message size, maximum number of
  recipients, line length, and read/write **timeouts** per
  SMTP/IMAP connection (anti-slowloris).
- **TLS:** **STARTTLS** on Submission (587) and **implicit TLS** on 465/993; the outbound
  MX uses **opportunistic STARTTLS**. In prod, TLS 1.3 (negotiated) with a
  cert-manager certificate; in dev Traefik terminates TLS.
- **Credentials:** delegated to **Sytue ID** (no password store; `mail` scope
  required — least privilege). DKIM key only in **Secret Manager**,
  never in git or in logs.
- **Multi-tenant isolation:** **deny-by-default RLS** on all tables; MX
  writes set `app.tenant` after resolving the destination address.
- **Robust queue:** deferred delivery with **retries + exponential backoff with
  jitter** and safe distribution (`FOR UPDATE SKIP LOCKED`); once attempts are exhausted,
  a **bounce** is recorded.
- **Privacy (GDPR):** data **only in the EU**; logs **without** mail content
  or PII; bodies encrypted at rest (S3); **soft** delete + deferred blob GC.
- **Still to harden before prod:** a dedicated/audited SPF evaluator,
  optional greylisting, configurable blocklist (RBL) integration, ARC for
  forwarding, and per-tenant outbound sending limits (outbound anti-spam).
