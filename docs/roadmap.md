<!-- SPDX-License-Identifier: Apache-2.0 -->
<!-- Copyright (c) 2026 Daniel Delgado Ramos (thinkddr) -->

<a id="estado-y-hoja-de-ruta--sytue-suite"></a>

# Status and roadmap — Sytue Suite

Public summary of the architectural maturity of **Sytue Suite** and the planned lines of evolution:

1. **[Project status](#1-estado-del-proyecto)** — architecture and components of the suite.
2. **[Phased roadmap](#2-roadmap-por-fases)** — completed milestones (F0–F6).
3. **[Maturity by product](#3-paridad-panorama-y-backlog-transversal)** — current capabilities by service.
4. **[Sytue Rooms](#4-rooms--gaps-al-detalle-vs-mattermost--slack--teams)** — messaging, channels, video calls and E2EE encryption.
5. **[Native office suite and Fyne clients](#5-docs--paridad-onlyoffice-y-apps-nativas)** — Docus, Calcus, Diapos, CertiPDF and native apps.
6. **[Sytue Cloud](#6-sytue-cloud--vision-y-workstreams)** — PaaS platform and managed services.

> Architecture documentation: [`architecture.md`](architecture.md). Technical decisions: [`decisions.md`](decisions.md).

---

<a id="1-estado-del-proyecto"></a>

## 1. Project status

<a id="resumen-ejecutivo"></a>

### Executive summary

**Sytue Suite** is a sovereign suite for productivity, collaboration, business management (ERP) and cloud platform (PaaS), developed entirely from scratch (_clean-room_, license **Apache-2.0 © 2026 Daniel Delgado Ramos (thinkddr)**) on a unified stack:

- **Backend**: **Go** (single `go.mod`, 16 independent microservices under `services/<svc>/cmd/<svc>` and shared libraries in `platform/pkg/*`).
- **Web frontend**: **Astro + Preact (Bun)** in `apps/frontend/` (22 web applications and 10 shared `@sytue/*` packages, with accessible WCAG AA design, light/dark mode and a BFF pattern with no tokens exposed in the browser).
- **Native clients**: **Fyne (Go)** in `apps/fyne/` for desktop (Linux, macOS, Windows) and mobile (Android, iOS/iPadOS) with 15 native applications without WebView.
- **Storage**: **PostgreSQL 18** with multi-tenant isolation through _Row-Level Security_ (`ENABLE` + `FORCE ROW LEVEL SECURITY`), **Dragonfly** (ephemeral state and real-time _fan-out_) and **S3-compatible Object Storage** (MinIO, SeaweedFS or managed S3).

<a id="puertas-de-calidad-verificadas-reproducibles"></a>

<a id="verificación-continua-y-puertas-de-calidad"></a>

### Continuous verification and quality gates

| Check                 | Command                                                  | Purpose                                                                       |
| --------------------- | -------------------------------------------------------- | ----------------------------------------------------------------------------- |
| Go build and analysis | `go build ./... && go vet ./...`                         | Clean build of the 16 services and platform packages.                         |
| Go unit tests         | `go test -race ./...`                                    | Validation of domain logic, cryptography, OIDC/PKCE, parsers and policies.    |
| Integration tests     | `go test -tags=integration ./...`                        | Tests against a real PostgreSQL verifying migrations and RLS isolation.       |
| Web build and typing  | `cd apps/frontend && bun run build && bun run typecheck` | Build of the `@sytue/*` packages and strict type checking across the 22 apps. |
| Guided deployment     | `./sytue-deploy` / `make deploy`                         | Step-by-step wizard for Docker Compose and Kubernetes (Helm).                 |
| E2E smoke test        | `scripts/e2e/smoke.sh`                                   | Health check (`/healthz`, `/readyz`), OIDC discovery and PKCE flow.           |

<a id="componentes"></a>

<a id="componentes-principales"></a>

### Main components

- **`platform/pkg/`** (Go): `httpx` (HTTP router, CSP/HSTS/CSRF headers, rate limiting and health probes), `authn` (JWT/JWKS RS256 verification), `pg` (`pgx` pool, embedded migrations and transactions with per-organization RLS), `events` (`LISTEN/NOTIFY` and transactional outbox), `objectstore` (S3 client), `crypto` (AES-256-GCM), `telemetry` (OpenTelemetry) and `tenancy`.
- **`services/`** (16 Go services):
  - `id`: Core identity provider (OIDC/OAuth 2.1 with mandatory PKCE S256, TOTP MFA and WebAuthn/Passkeys, SCIM 2.0, Conditional Access, PIM and JWKS rotation).
  - `hub`: Cross-cutting activity aggregator, real-time notifications (SSE), global search and intents between applications.
  - `mail`: Email server and client (JMAP, IMAP, SMTP, DKIM signing, Sieve filters and legal hold).
  - `drive`: Cloud storage with REST API and WebDAV, versioning, shared links with password/expiry and unified trash.
  - `docs`: Collaborative engine for documents (**SyDocus**), spreadsheets (**SyCalcus**), presentations (**SyDiapos**), relational databases (**SyBases**) and PDF editing/signing (**SyCertiPDF**).
  - `photos`: Photo gallery (**SyPhotos**) with albums, timeline and thumbnails.
  - `notes`: Notebooks and quick notes (**SyNotepads**) with instant sync.
  - `calendar`: Calendars and events with CalDAV support, iMIP invitations and meeting scheduling.
  - `contacts`: Contact book and groups with CardDAV support.
  - `rooms`: Real-time instant messaging through channels and threads, end-to-end encryption (Matrix/Olm) and WebRTC video calls through **LiveKit**.
  - `cast`: Video-on-demand and live streaming platform (HLS) with an optional public front page.
  - `pages`: Visual builder for websites and online stores with immutable static publishing and code export.
  - `syerp`: Modular ERP system (invoicing, accounting, inventory, POS, HR, projects, expenses and VeriFactu cryptographic chaining).
  - `cloud`: PaaS platform for deploying applications from Git and provisioning managed databases on Kubernetes.
  - `analytics`: First-party web analytics without cookies or trackers.
  - `integrations`: Gateway for outbound connectors (BYO-SaaS, SSRF guard, egress auditing and migration wizards).
  - `ai` and `ocr`: Cross-cutting artificial intelligence assistant with quota control and optical character recognition service (Tesseract).

---

<a id="2-roadmap-por-fases"></a>

## 2. Phased roadmap

| Phase                                             | Scope                                                                                                                           | Status    |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- | --------- |
| **F0 — Foundations and canonical contract**       | Monorepo structure, `CONVENTIONS.md`, `platform/pkg/*` libraries and CI pipeline.                                               | Completed |
| **F1 — Core identity (Sytue ID) + Shell/Hub**     | OIDC/OAuth 2.1 + PKCE, MFA (TOTP/WebAuthn), SCIM 2.0, `@sytue/shell` and `services/hub`.                                        | Completed |
| **F2 — Collaboration and productivity core**      | Sytue Drive, Sytue Mail, Sytue Calendar, Sytue Contacts and Sytue Rooms (chat + LiveKit video).                                 | Completed |
| **F3 — Native office suite and PDF**              | Native editors for documents (Docus), spreadsheets (Calcus), presentations (Diapos) and CertiPDF with eIDAS signatures (PAdES). | Completed |
| **F4 — Business management and digital creation** | SyERP, Sytue Bases, Sytue Web (`services/pages`), Sytue Cast, Sytue Photos and Sytue Notepads.                                  | Completed |
| **F5 — Sytue Cloud (PaaS) and interoperability**  | Application deployment on Kubernetes, Integration Gateway, Slack/Teams compatibility layer and enterprise governance.           | Completed |
| **F6 — Cross-platform native clients**            | Suite of 15 native Fyne applications for Linux, macOS, Windows, Android and iOS/iPadOS.                                         | Completed |

---

<a id="3-paridad-panorama-y-backlog-transversal"></a>

<a id="3-madurez-por-producto-y-evolución-continua"></a>

## 3. Maturity by product and continuous evolution

| Product                           | Current status                                                                                                                                     | Next improvements on the roadmap                                                                          |
| --------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| **Sytue ID & Admin**              | OIDC/PKCE, MFA/Passkeys, SCIM 2.0, Conditional Access, PIM, dynamic groups, SIEM auditing.                                                         | Real-time SIEM export (syslog/CEF streaming) and advanced multi-issuer OIDC federation.                   |
| **Sytue Mail**                    | JMAP/SMTP/IMAP, DKIM, threads, labels, scheduled sending, Sieve rules, templates and eDiscovery.                                                   | Advanced phonetic search and extended offline sync in mobile clients.                                     |
| **Sytue Drive & Photos**          | Folder tree, WebDAV, S3 multipart upload, protected public links, versions and photo gallery.                                                      | Full write support in the WOPI protocol (`Lock`/`PutFile`) for third-party integrations.                  |
| **SyDocus / SyCalcus / SyDiapos** | Collaborative co-editing, OOXML (`.docx`, `.xlsx`, `.pptx`) and ODF (`.odt`, `.ods`, `.odp`) import/export, PDF/UA-1 export and sandboxed scripts. | Expansion of the catalog of statistical/financial functions in Calcus and advanced transitions in Diapos. |
| **SyCertiPDF**                    | SVG vector viewer, annotations, page merging/splitting, OCR with text layer and PAdES advanced digital signature.                                  | CLI validation of bulk signature batches and interactive AcroForm form templates.                         |
| **Sytue Rooms**                   | Public/private channels, direct messages, threads, reactions, screen sharing, WebRTC rooms and E2EE encryption.                                    | Optional automatic meeting transcription through `services/ai`.                                           |
| **SyERP**                         | Invoicing, accounting, inventory, POS, projects, expenses, bank reconciliation (Norma 43) and VeriFactu.                                           | Additional European open banking connectors (PSD2) and tax localization for more EU countries.            |
| **Sytue Cloud**                   | Deployment from Git, isolated builds on Kubernetes, PostgreSQL databases per project, domains with automatic TLS and web analytics.                | Horizontal autoscaling based on custom metrics and multiple regions in federated clusters.                |

---

<a id="4-rooms--gaps-al-detalle-vs-mattermost--slack--teams"></a>

<a id="4-sytue-rooms--arquitectura-de-comunicación-en-tiempo-real"></a>

## 4. Sytue Rooms — Real-time communication architecture

`services/rooms` unifies persistent channel messaging and video calls in a single database and service:

- **Real-time messaging**: WebSockets with _fan-out_ over PostgreSQL `LISTEN/NOTIFY` and Dragonfly for multi-replica deployments.
- **Integrated video calls**: Native integration with **LiveKit** (Apache-2.0) for audio, video and screen sharing, with signed tokens generated in the backend.
- **Webhook compatibility**: Inbound and outbound adapters compatible with the Slack and Microsoft Teams webhook format (`services/rooms/internal/compat`).
- **Security and moderation**: On-device encryption through Matrix, server-side user blocking and a reporting system for the organization's administrators.

---

<a id="5-docs--paridad-onlyoffice-y-apps-nativas"></a>

<a id="5-ofimática-nativa-y-aplicaciones-de-escritorio-y-móvil"></a>

## 5. Native office suite and desktop and mobile applications

The office suite is organized into specialized applications that share the collaborative engine of `services/docs` and store documents directly in the user's **Sytue Drive**:

- **SyDocus** (`apps/frontend/apps/docs`): Paginated word processor with styles, tables, track changes, comments, footnotes, table of contents and mail merge.
- **SyCalcus** (`apps/frontend/apps/sheets`): Spreadsheet with its own formula engine, number formats, conditional filters, pivot tables and charts.
- **SyDiapos** (`apps/frontend/apps/slides`): Presentation editor with themes, slide masters, speaker notes and presentation mode.
- **SyBases** (`apps/frontend/apps/bases`): Collaborative relational databases with table, kanban, calendar and form views.
- **Native applications (`apps/fyne`)**: Fifteen applications compiled natively in Go + Fyne that share single sign-on through the operating system keychain ([ADR 0050](decisions.md#0050)).

<a id="apps-nativas-sin-webview--y-paridad-de-superficies-decisión-2026-09-17"></a>

### Native apps without a webview — and surface parity (decision 2026-09-17)

The desktop and mobile apps are native Go + Fyne, without a webview. The rule is **parity first**: every web app feature also reaches desktop and mobile. The registry `apps/frontend/packages/config/src/paridad.ts` lists each feature with its status per surface (`si`, or `planificado` with the debt that points here), and the `ci-desktop` and `ci-mobile` gates check it whenever a web app changes. The TopBar is the only deliberate exception. Operating system integrations are shared by all apps: system notifications, “Add to the system calendar” and mounting Drive in the system.

---

<a id="6-sytue-cloud--vision-y-workstreams"></a>

<a id="6-sytue-cloud--plataforma-paas-y-servicios-gestionados"></a>

## 6. Sytue Cloud — PaaS platform and managed services

`services/cloud` provides an application deployment experience integrated with the rest of the suite:

- **Control plane (`cloud-api`)**: Management of projects, environments, environment variables encrypted with AES-256-GCM (`CLOUD_SECRET_ENC_KEY`), deployments, custom domains and optional spending limits.
- **Execution plane (`KubeRunner`)**: Declarative creation of `Namespace`, `Deployment`, `Service`, `Ingress` and `NetworkPolicy` for per-tenant isolation on Kubernetes.
- **Managed services**: Provisioning of dedicated PostgreSQL databases, cache and automatic injection of credentials into deployed applications.

<a id="sytue-office-lo-que-falta"></a>

## 7. Sytue Office (local edition) — what is left

Sytue Office is the fifteen Fyne apps whole on the device: no account, no own encryption and no
Sytue servers ([ADR 0083](decisions.md#0083)). It is built on the desktop from end to end; what is
left:

- **Devices and stores.** Android builds and packages (one AAB per app); iOS, the macOS `.dmg`
  signed and notarized, and the App Store and Google Play listings are not done. Nothing has been
  tried on a real phone yet.
- **Real services.** Outlook with OAuth, Teams and Google Chat in Rooms, and Calendar and Contacts
  against a real server have not been tried end to end.
- **The owner's registrations**: the Google desktop OAuth client and its verification, the Microsoft
  Entra public app, the Apple App Group `group.eu.sytue.office` for the `eu.sytue.office.*` App IDs,
  test accounts for each provider, and a legal review of VERI\*FACTU in a local program.
- **Release packages** ([operations](operations.md#sytue-office-publico)): `office.yml` has not yet
  run against a real release of this repository. The pull request that brings it builds every
  package without attaching anything; the upload to a release is only tested by the first one.

**Out of scope**, and not debt: serving anything to the local network, syncing between the person's
devices, push notifications with the app closed, Teams and Meet calls inside the app, collaboration,
`.doc`/`.xls`/`.ppt`/`.rtf`, own encryption, faces and places in Photos, a web build, and migrating
on its own from the connected edition beyond importing the "Your data" ZIP.
