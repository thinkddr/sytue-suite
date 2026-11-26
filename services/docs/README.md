<!--
SPDX-License-Identifier: Apache-2.0
Copyright (c) 2026 Daniel Delgado Ramos
-->

# Sytue Docs/Sheets/Slides (`services/docs`)

Go backend for the **collaborative office suite** of Sytue One (`docs.sytue.eu`, dev
port **8083**). Text documents, spreadsheets and presentations, with
**real-time editing** over WebSocket, **versioning**, **comments**,
**permissions** and **OOXML/ODF exports**. A **Sytue-native, proprietary
(© Sytue)** and **clean-room** product: implemented on open standards (ECMA-376
OOXML, OASIS ODF) and the public CRDT literature; **no** AGPL engines
(OnlyOffice/Nextcloud are out) and **no Redis** (state, operations
and cross-replica broadcasting live in **PostgreSQL**).

> Part of the single Go module `git.sytue.eu/sytue/one` (go.mod at the repo **root**).
> It reuses `platform/pkg/*` (config, log, httpx, pg, objectstore, authn,
> events, tenancy). The **visual editor** lives in `apps/frontend/packages/office` and is mounted by the Docus, Calcus, Diapos and CertiPDF apps (`apps/frontend/apps/{docus,calcus,diapos,certipdf}`, ADR 0076).

<a id="estrategia-de-colaboración-crdt-no-ot"></a>
## Collaboration strategy: CRDT (not OT)

Convergence is achieved with an **RGA-style sequence CRDT** (Replicated
Growable Array) with dense, independent identities (`Dot{actor, counter}`)
and a **Lamport clock**, plus **LWW registers** for cells/formatting/properties.
Decision and rationale:

- **The server only SEQUENCES and persists** operations (append-only in
  `doc_operations`); it **does not transform** them (it is not OT). Merging is deterministic on
  client and server (`internal/domain/crdt.go`), which avoids a transformation
  server with complex state and fits a single datastore.
- **Offline-first / reconnection:** a client sends its `last_seq` (`hello`) and
  receives the *replay* of later operations; it converges without conflicts.
- **Idempotence and commutativity:** applying an operation twice or in a
  different order produces the same state (CRDT property) — see the tests.
- **Multi-replica without Redis:** if there are several replicas of the service, cross-replica
  broadcasting goes through the **event bus on PostgreSQL**
  (`platform/pkg/events`, LISTEN/NOTIFY + outbox). The in-memory `Hub` is the
  local per-process fan-out.

The per-document-type model is built on these primitives:

| Type | Model |
|------|--------|
| `document` | RGA sequence of characters + formatting marks (LWW) |
| `spreadsheet` | LWW map of cells (`"Hoja!fila:col" → valor/fórmula`) |
| `presentation` | RGA sequence of slides + LWW of properties |

<a id="arquitectura-capas"></a>
## Architecture (layers)

```
services/docs/
├── cmd/docs/main.go            # binario: carga config, migra, cablea y sirve
├── internal/
│   ├── config/                 # configuración (prefijo SYTUE_DOCS)
│   ├── domain/                 # modelo, CRDT, autorización, export, Service
│   ├── store/                  # persistencia pgx (RLS por tenant)
│   ├── collab/                 # hub + clientes WebSocket (coder/websocket)
│   └── http/                   # router chi, handlers, mapeo de errores
├── db/migrations/              # esquema versionado (golang-migrate)
├── openapi.yaml                # contrato HTTP
└── Dockerfile                  # multi-stage -> distroless static (rootless)
```

<a id="datos-postgresql-multi-tenant-con-rls"></a>
## Data (PostgreSQL, multi-tenant with RLS)

- `documents` — metadata + `seq` (logical version vector) + trash.
- `doc_operations` — **immutable** log of CRDT operations (append-only).
- `doc_snapshots` — consolidated CRDT state (inline in `jsonb` or in S3).
- `doc_versions` — history of named/automatic versions (restorable).
- `doc_permissions` — **deny-by-default** ACL (user/group/tenant + role).
- `doc_share_links` — public links (sha-256 **hashed** token).
- `doc_comments` — comments anchored to stable CRDT references.
- `doc_blobs` — metadata of exports in S3 (OOXML/ODF/PDF/HTML/MD).
- `events_outbox` — transactional event bus.

Multi-tenant isolation through **RLS** (`current_setting('app.tenant')`, set by
`platform/pkg/pg.SetTenant`). The **only** cross-tenant read allowed is
resolving a public link, via the minimal-surface `SECURITY DEFINER` function
`docs_resolve_share(token_hash)` (it returns only
`document_id/tenant/role` of active links).

<a id="permisos-deny-by-default"></a>
## Permissions (deny-by-default)

Increasing roles: `viewer < commenter < editor < owner`. The document's `owner_sub`
is always `owner`. For everyone else, the effective role is the **maximum**
among the applicable ACL entries (user, Sytue ID groups, tenant). No
applicable entry ⇒ **no access**. Share links never grant
`owner`.

<a id="identidad"></a>
## Identity

Every request is validated against **Sytue ID** (OIDC/JWKS) with the platform's
`authn.RequireAuth` middleware. The `sub`, the groups and the `tenant` are
extracted from the JWT to resolve authorization. The WebSocket channel requires the same
JWT (Bearer) and a minimum role of `viewer`; editing requires `editor`.

<a id="exports-ooxmlodf-clean-room"></a>
## OOXML/ODF exports (clean-room)

`internal/domain/export.go` generates **OPC** packages (OOXML, ECMA-376: docx/xlsx/
pptx) and **ODF** packages (OASIS 1.3: odt/ods/odp) with the Go stdlib (`archive/zip`,
`encoding/xml`), as well as PDF/HTML/Markdown. Original work based on the public
specifications; the binaries are materialized in S3 Object Storage
(bucket `docs-blobs`).

<a id="configuración-entorno-prefijo-sytue_docs_"></a>
## Configuration (environment, prefix `SYTUE_DOCS_`)

| Variable | Default | Description |
|---|---|---|
| `SYTUE_DOCS_HTTP_ADDR` | `:8083` | HTTP listen address |
| `SYTUE_DOCS_DATABASE_URL` | — (required) | PostgreSQL DSN |
| `SYTUE_DOCS_OIDC_ISSUER` | `https://id.sytue.eu` | Sytue ID issuer |
| `SYTUE_DOCS_OIDC_AUDIENCE` | `sytue-docs` | expected audience |
| `SYTUE_DOCS_OIDC_JWKS_URL` | (derived from the issuer) | JWKS endpoint |
| `SYTUE_DOCS_S3_ENDPOINT` | — | Object Storage (MinIO dev / Scaleway prod) |
| `SYTUE_DOCS_S3_REGION` | `fr-par` | S3 region (EU only) |
| `SYTUE_DOCS_S3_ACCESS_KEY` / `_SECRET_KEY` | — | S3 credentials |
| `SYTUE_DOCS_S3_BUCKET` | `docs-blobs` | blobs/exports bucket |
| `SYTUE_DOCS_S3_USE_SSL` | `false` | TLS to S3 |
| `SYTUE_DOCS_SNAPSHOT_INLINE_LIMIT` | `262144` | cap (bytes) for inline snapshots |
| `SYTUE_DOCS_DRIVE_URL` | `http://drive:8082` | Sytue Drive: the drive where each document's file lives |
| `SYTUE_DOCS_DRIVE_TIMEOUT` | `15s` | cap per call to Drive |
| `SYTUE_DOCS_LOG_LEVEL` / `_LOG_FORMAT` | `info` / `json` | logging |

> No secrets in code or in logs: everything via the environment / secrets manager.

> **The document's file lives in Sytue Drive.** A new document is also created
> as `.odt`/`.ods`/`.odp` in the `Sytue Docs/` folder of its creator's
> drive, and is born anchored to it (`source_ref`, mig. 0006). The call is made with
> **that person's JWT**, not with a service secret, so that Drive applies
> its own authorization. Visible consequence: creating a document **consumes Drive
> quota**, and when the drive is full creation responds 507. The full design,
> what is missing and the plan for earlier documents are in
> [`docs/architecture.md#docs-en-drive`](../../docs/architecture.md#docs-en-drive).

<a id="protocolo-websocket-v1documentsidws"></a>
## WebSocket protocol (`/v1/documents/{id}/ws`)

JSON messages. Client→server: `hello` (synchronization by `last_seq`), `ops`
(editing), `cursor` (ephemeral presence), `ping`. Server→client: `welcome`
(role + seq), `ops` (sequenced operations), `ack` (acknowledgement with seq),
`presence`, `error`.

<a id="api-rest"></a>
## REST API

Full contract in [`openapi.yaml`](./openapi.yaml). Summary:

- `GET/POST /v1/documents`, `GET/PATCH/DELETE /v1/documents/{id}`, `PUT …/star`
- `GET …/state`, `GET/POST …/ops`, `GET …/ws`
- `GET/POST …/versions`, `POST …/versions/{vid}/restore`
- `GET/POST …/comments`, `PUT …/comments/{cid}/resolve`, `DELETE …/comments/{cid}`
- `GET/PUT/DELETE …/permissions`, `GET/POST …/shares`, `DELETE …/shares/{lid}`
- `GET /v1/share/{token}` (public, no JWT)
- `GET /v1/share/{token}/view` (public, no JWT): its read-only HTML view, the one behind `/s/{token}` on the web
- `POST /v1/share/redeem` `{token}` (with a session): redeems an editor or commenter link for a permission for the caller, also from another organization (ADR 0052)
- `GET/POST …/exports`, `GET …/exports/{bid}`
- Probes `/healthz` (liveness) and `/readyz` (readiness: PostgreSQL + S3).

## Tests

```sh
go test ./services/docs/...
```

They cover **CRDT convergence** (order independence, idempotence,
tombstones, LWW), **role resolution** (deny-by-default) and **exports**
(docx as a valid OPC ZIP, ODF with `mimetype` first and uncompressed). They do not
require PostgreSQL/MinIO/Sytue ID.
