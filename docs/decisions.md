<a id="decisiones-de-arquitectura-adr--sytue-one"></a>

# Architecture decisions (ADR) — Sytue One

Single record of the significant architectural decisions: **what** was decided,
**why**, what was ruled out, and what consequences it has. It replaces the old
`docs/adr/` directory (one file per decision).

> The **why** lives here. The normative **what** (names, domains, ports, versions,
> licenses) lives in [`CONVENTIONS.md`](CONVENTIONS.md) and prevails if they diverge.

**Rules:** sequential, immutable numbering; one decision per entry; an accepted
decision is not edited to change it — another one is added that supersedes it. Trivial
decisions, or ones reversible at no cost, do not need an entry.

<a id="indice"></a>

<a id="índice"></a>

## Index

| #    | Decision                                                                                                                                                | Status                                                                         |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ |
| 0001 | [100 % EU sovereignty on Scaleway](#0001)                                                                                                               | accepted                                                                       |
| 0002 | [Keycloak as the core IdP](#0002)                                                                                                                       | superseded → 0020                                                              |
| 0003 | [Nextcloud + OnlyOffice for files and office suite](#0003)                                                                                              | superseded → 0017                                                              |
| 0004 | [Astro + Preact as the frontend stack](#0004)                                                                                                           | accepted                                                                       |
| 0005 | [Mattermost for chat and video](#0005)                                                                                                                  | superseded → 0017, 0021                                                        |
| 0006 | [Stalwart as the mail server](#0006)                                                                                                                    | superseded → 0017                                                              |
| 0007 | [Sytue Shell + Sytue Hub](#0007)                                                                                                                        | accepted (reconciled)                                                          |
| 0008 | [Valkey instead of Redis](#0008)                                                                                                                        | superseded → 0019                                                              |
| 0009 | [All free FOSS and fork strategy](#0009)                                                                                                                | superseded → 0017, 0018                                                        |
| 0010 | [Own clients on top of the upstream UI](#0010)                                                                                                          | superseded → 0017                                                              |
| 0011 | [PaaS model and multi-tenancy](#0011)                                                                                                                   | accepted                                                                       |
| 0012 | [Sytue Cloud managed services](#0012)                                                                                                                   | accepted (reconciled)                                                          |
| 0013 | [Build pipeline with Kaniko](#0013)                                                                                                                     | accepted (⚠ review pending)                                                    |
| 0014 | [Dual IaC: local Compose + Helm/Terraform](#0014)                                                                                                       | accepted                                                                       |
| 0015 | [AGPL-3.0 licensing of our own code](#0015)                                                                                                             | superseded → 0018                                                              |
| 0016 | [Single stack: Go + Astro/Preact + PostgreSQL](#0016)                                                                                                   | accepted                                                                       |
| 0017 | [Native clean-room reimplementation](#0017)                                                                                                             | accepted                                                                       |
| 0018 | [Apache-2.0 licensing (© 2026 Daniel Delgado Ramos (thinkddr))](#0018)                                                                                  | accepted                                                                       |
| 0019 | [PostgreSQL as the single datastore](#0019)                                                                                                             | accepted                                                                       |
| 0020 | [Sytue ID as a native IdP in Go](#0020)                                                                                                                 | accepted                                                                       |
| 0021 | [LiveKit as the media SFU for Meet](#0021)                                                                                                              | accepted                                                                       |
| 0023 | [Removal of third-party AGPL engines](#0023)                                                                                                            | accepted                                                                       |
| 0023 | [Sytue Cloud substrate: Kapsule](#0023)                                                                                                                 | accepted                                                                       |
| 0024 | [Desktop (Fyne) and mobile (PWA + Capacitor) apps](#0024)                                                                                               | accepted                                                                       |
| 0025 | [Sytue Rooms: messaging and video calls, one service and one database](#0025)                                                                           | accepted                                                                       |
| 0026 | [Sytue Analytics: anonymity by construction, first-party measurement](#0026)                                                                            | accepted                                                                       |
| 0027 | [Temporary Bun/Kaniko exception for internal v0.1.6](#0027)                                                                                             | accepted · expires 2026-10-01                                                  |
| 0028 | [Anonymous benchmarks across Sytue Cloud customers](#0028)                                                                                              | accepted                                                                       |
| 0029 | [Open-source apps as a service: closed catalog, not any image](#0029)                                                                                   | accepted                                                                       |
| 0030 | [Digital signatures: what is promised under eIDAS and where the key comes from](#0030)                                                                  | accepted                                                                       |
| 0031 | [Crossplane adopts infrastructure by risk; RDB and Kapsule stay out](#0031)                                                                             | accepted                                                                       |
| 0032 | [Open-source products on Sytue Cloud: copyleft yes, “source available” no; and who gets published](#0032)                                               | accepted                                                                       |
| 0033 | [Sytue PDF is drawn in SVG, not rasterized](#0033)                                                                                                      | accepted (extended)                                                            |
| 0034 | [OCR: self-hosted Tesseract in our own service; the text layer is written by Sytue PDF](#0034)                                                          | accepted · **the OCR engine is revised by [0035](#0035)**                      |
| 0035 | [Sytue AI: two models on Scaleway, soft per-user cap and OCR via a vision model](#0035)                                                                 | accepted                                                                       |
| 0036 | [Cloudflare out: the portal comes in through Pangolin and DNS is served by Scaleway](#0036)                                                             | accepted                                                                       |
| 0037 | [Shared plumbing is truly shared](#0037)                                                                                                                | accepted                                                                       |
| 0038 | [Personal accounts: free sign-up with 5 GB and a 1 TB plan](#0038)                                                                                      | accepted · **sign-up is revised by [0040](#0040)**                             |
| 0040 | [Global @username and conversations with people from other organizations](#0040)                                                                        | accepted · **sign-up is revised by [0041](#0041)**                             |
| 0041 | [Mobile number verified by SMS at sign-up and four sign-in identifiers](#0041)                                                                          | accepted                                                                       |
| 0042 | [Spain without an accounting agency: the bank comes in via Norma 43 and filing on your behalf waits for the certificate and VeriFactu](#0042)           | accepted · **the invoice PDF and the email notice are added by [0043](#0043)** |
| 0043 | [The ERP belongs to the whole organization: per-module permissions, tasks and documents, and Sytue AI that creates with confirmation](#0043)            | accepted                                                                       |
| 0044 | [VERI\*FACTU inside the ERP: the database guarantees the chain and submission goes out with a service certificate](#0044)                               | accepted                                                                       |
| 0045 | [A single Gio interface for web, desktop and mobile: sytue.com in Gio, sytue.eu in the current one](#0045)                                              | superseded → 0048 (desktop and mobile) and → 0049 (sytue.com)                  |
| 0046 | [Rooms media is not written in Go: transport yes, codecs and capture no](#0046)                                                                         | accepted · bounds [0045](#0045) · amended 2026-09-25 (native voice and video)  |
| 0047 | [Android notifications come from the Hub SSE, and the packager is patched](#0047)                                                                       | accepted · implements [0039](#0039)                                            |
| 0048 | [The native interface is rebuilt in Fyne with our own widgets, and what is not part of the toolkit moves to `apps/nucleo`](#0048)                       | accepted · supersedes [0045](#0045) on desktop and mobile                      |
| 0049 | [`sytue.com` serves the Astro suite with a second set of images, not the Gio one](#0049)                                                                | accepted · supersedes [0045](#0045) on the web                                 |
| 0050 | [A single sign-in for the thirteen native apps: keychain for the secret, shared folder for preferences, server for presence](#0050)                     | accepted · implements [0048](#0048)                                            |
| 0051 | [Signing with the system certificate, in two steps, with a qualified Izenpe timestamp, and “Request signature”](#0051)                                  | accepted · develops [0030](#0030)                                              |
| 0052 | [Redeeming a Docs link, also from another organization](#0052)                                                                                          | accepted · applies [0040](#0040) to documents                                  |
| 0053 | [A notification belongs to the person: the bell shows those from all their organizations](#0053)                                                        | accepted · completes [0040](#0040) and [0052](#0052)                           |
| 0054 | [“Delete my account”: the person unsubscribes on their own, with 30 days to undo it, and their own data is deleted but not their organization's](#0054) | accepted                                                                       |
| 0055 | [The canonical URL of each product is its product host; the organization goes in the PATH, not the subdomain](#0055)                                    | accepted                                                                       |
| 0056 | [The organization as a PATH segment within each product](#0056)                                                                                         | accepted                                                                       |
| 0057 | [Requesting a signature from someone in another organization: the PDF goes out through a Drive link](#0057)                                             | accepted · completes [0051](#0051) §7 · applies [0040](#0040)                  |
| 0058 | [CI runs on GitHub Actions; workflows are ported without changing what they check](#0058)                                                               | accepted · points 1 and 5 replaced by [0060](#0060)                            |
| 0059 | [Signature requests from outside: the recipient blocks and their organization decides, without the requester finding out](#0059)                        | accepted · completes [0057](#0057)                                             |
| 0060 | [Our own runners on the front node, images on ghcr.io and forks on GitHub](#0060)                                                                       | accepted · replaces points 1 and 5 of [0058](#0058)                            |
| 0061 | [Sytue Cast: on-demand and live video, with a public front page and Drive quota](#0061)                                                                 | accepted                                                                       |
| 0066 | [CI builds with our own rootless BuildKit, not with the classic builder through the Docker proxy](#0066)                                                | accepted · completes [0060](#0060)                                             |
| 0077 | [Rooms encrypts on devices through Matrix; external channels keep their provider's real protection](#0077)                                              | accepted · explicit exception to 0016, 0018 and the removal of AGPL engines    |
| 0078 | [Native widgets: expiring counters in an App Group, Apple behind a switch and nothing on Windows/Linux](#0078)                                          | accepted                                                                       |
| 0079 | [Sytue Notepads on “paper”: a neutral chrome chosen by the app](#0079)                                                                                  | accepted                                                                       |
| 0080 | [The whole suite on “paper”: the Notepads chrome moves to all fifteen apps](#0080)                                                                      | accepted · extends [0079](#0079)                                               |
| 0081 | [The Matrix recovery key is held by Rooms for the OIDC session](#0081)                                                                                  | superseded → 0082                                                              |
| 0082 | [The Matrix recovery key, wrapped on the device with the password or the passkey](#0082)                                                                | accepted · replaces [0081](#0081)                                              |
| 0086 | [Direct post-quantum vault (`age-MLKEM768-X25519-v1`) for the suite's storage, without Synapse/Matrix](#0086)                                           | accepted · replaces Synapse/Matrix in the storage of 13 apps                   |
| 0087 | [Sytue Mail on the direct vault: signing key in the vault, anchored by Sytue ID](#0087)                                                                 | accepted · Phase 2 of [0086](#0086) · deliveries 1 and 2 of 5                  |

<a id="el-pivote-sytue-native-2026-07-01"></a>

### The “Sytue Native” pivot (2026-07-01)

Decisions **0016–0022** formalize the shift to a **single stack** (Go + Astro/Preact +
PostgreSQL), **proprietary products reimplemented clean-room** and the **removal of
all third-party AGPL engines** (Keycloak, Nextcloud, OnlyOffice, Mattermost,
Stalwart) and of Redis/Valkey. As a result 0002, 0003, 0005, 0006, 0008, 0009, 0010 and
0015 are superseded — they are kept in summary form as a historical record of the _why_.

- **0016** is the parent decision: it conditions 0017, 0019, 0020, 0021, 0022.
- **0022** retires the engines; it enables **0017** (how things are reimplemented) and **0018** (license).
- **0021** (LiveKit) is the only exception to “no third-party apps in the customer runtime”,
  bounded to the media plane.
- **0001** (EU sovereignty) conditions every choice of provider and region.
- **0007** and **0012** remain in force with a reconciliation note: the product decision
  stands, its implementation is realigned to the pivot stack.

---

<a id="0001"></a>

<a id="0001--soberanía-100--ue-sobre-scaleway"></a>

## 0001 — 100 % EU sovereignty on Scaleway

`accepted` · 2026-06-30 · infra, gdpr, sovereignty

**Context.** The value proposition against Google/Microsoft is verifiable digital
sovereignty: no personal data leaves the EU or is subject to extraterritorial access
(_US CLOUD Act_). We need an EU provider with managed Kubernetes, Postgres, S3,
registry, DNS, LB, secrets and a mature Terraform provider.

**Decision.** **Scaleway** as the single production provider, EU regions only: primary
`fr-par` (zones 1/2/3), DR in `nl-ams`. Compute on **Kapsule**, data on **RDB
(PostgreSQL)**, blobs on versioned **Object Storage S3**, images on **Container
Registry**, secrets on **Secret Manager**, materialized directly as Kubernetes `Secret`
objects by the production bootstrap (never in git). No
US provider for personal data; no transfer outside the EU.

**Ruled out.** US hyperscalers (CLOUD Act, incompatible with the core principle);
OVHcloud (valid and sovereign, worse catalog/DX fit — it remains as a portability path);
Hetzner (no managed K8s or PaaS catalog); multi-cloud from day one (complexity
that dilutes time-to-market).

**Consequences.** GDPR compliance by design and a differentiating sales message; in exchange,
provider **lock-in**, mitigated by using portable primitives (K8s, S3, PostgreSQL, OIDC),
a smaller catalog than a hyperscaler (some pieces are built in-house) and DR limited to
`nl-ams`. Every later infrastructure decision is bounded by “does it exist and is it sovereign
on Scaleway EU?”.

---

<a id="0002"></a>

<a id="0002--keycloak-como-idp-nuclear--superseded-por-0020"></a>

## 0002 — Keycloak as the core IdP · superseded by [0020](#0020)

`superseded` · 2026-06-30 · identity, sso

**Decision (historical).** **Keycloak 26** as the core IdP (Sytue ID brand, single realm
`sytue`), with OIDC clients per surface, federation clients per engine
(nextcloud/mattermost/onlyoffice/stalwart), SCIM/event provisioning via Sytue Hub,
`org/<tenant>` groups with `owner`/`admin`/`member`/`billing` roles, mandatory MFA/WebAuthn
for admin and owner, and host-only `__Host-` cookies with OAuth SSO through Sytue ID.

**Ruled out at the time.** Ory (Hydra/Kratos) for assembly cost; Authentik/Zitadel for
maturity; US SaaS (Auth0/Okta/Entra) for sovereignty; our own IdP for risk — a criterion the
pivot ended up reversing with proven OIDC libraries.

**Why it fell.** The pivot removes the federated engines and unifies the backend in Go: a
third-party Java engine as a core piece contradicts the single stack. See [0020](#0020).

---

<a id="0003"></a>

<a id="0003--nextcloud--onlyoffice--superseded-por-0017"></a>

## 0003 — Nextcloud + OnlyOffice · superseded by [0017](#0017)

`superseded` · 2026-06-30 · drive, docs

**Decision (historical).** **Nextcloud 30** as the Drive engine (WebDAV/OCS) and backend for
Calendar (CalDAV) and Contacts (CardDAV); **OnlyOffice Document Server 8.2** as the
office-suite engine, embedded in our own client. Without exposing their native UIs.

**Ruled out at the time.** Seafile (limited CalDAV/CardDAV), ownCloud/OCIS (maturity),
dedicated DAV servers (more pieces), Collabora (lower OOXML fidelity), CryptPad
(different document model).

**Why it fell.** Third-party AGPL engines; they are reimplemented as our own Go services.

---

<a id="0004"></a>

<a id="0004--astro--preact-como-stack-de-frontend"></a>

## 0004 — Astro + Preact as the frontend stack

`accepted` · 2026-06-30 · frontend, design-system

**Context.** All surfaces must look and behave like **a single application**,
mixing marketing pages and rich interactive apps, with minimal JS by default and a
BFF pattern for OIDC+PKCE.

**Decision.** **Astro + Preact** (islands) mandatory on all web surfaces, with
**Tailwind** and the `@sytue/ui` tokens. Monorepo in `apps/frontend/` with shared
packages `@sytue/{ui,shell,auth,api-clients,config}` and one app per product. The
**Astro endpoints act as the BFF**: after OIDC+PKCE they issue an httpOnly cookie for the
`.sytue.eu` domain that allows jumping between subdomains without logging in again; **no secrets on the
client**. Light/dark mode and **WCAG AA** are mandatory.

**Ruled out.** Next.js/React (more JS by default), SvelteKit (ecosystem, duplicated design
system), pure SPA without SSR (SEO/initial performance), Preact alone (reinventing routing/SSR).

**Consequences.** Excellent performance and accessibility by default; a single stack and a
single design system. Risk of visual divergence if an app creates ad-hoc components:
forbidden — canonical components live **only once** in `@sytue/ui`. With the pivot,
the apps are repointed to the Go services; the framework does not change.

---

<a id="0005"></a>

<a id="0005--mattermost-para-chat-y-vídeo--superseded-por-0017-0021"></a>

## 0005 — Mattermost for chat and video · superseded by [0017](#0017), [0021](#0021)

`superseded` · 2026-06-30 · chat, meet

**Decision (historical).** **Mattermost Team Edition** as the Chat engine and **Mattermost
Calls + rtcd** for Meet, with the webapp themed/forked to build the Sytue Shell.
A deliberate exception to “own client from scratch”: its UI _was_ the aesthetic reference.

**Ruled out at the time.** Matrix/Element (aesthetic alignment and operating Synapse),
Rocket.Chat (add-on licensing), Zulip (different UX), Jitsi (a second, separate product).

**Why it fell.** The engine is retired: Chat becomes a Go service (WebSocket) and Meet combines
Go signaling + LiveKit as the SFU.

---

<a id="0006"></a>

<a id="0006--stalwart-como-servidor-de-correo--superseded-por-0017"></a>

## 0006 — Stalwart as the mail server · superseded by [0017](#0017)

`superseded` · 2026-06-30 · mail

**Decision (historical).** **Stalwart** as the single SMTP/IMAP/**JMAP** server and our own
Gmail-like Astro+Preact client on top of JMAP, with SPF/DKIM/DMARC/MTA-STS/TLS-RPT and an
encrypted store.

**Ruled out at the time.** Postfix+Dovecot+JMAP bridge (operational surface), Cyrus
(complexity), Mailu/Mailcow (no native JMAP), existing webmails (impossible to align UX).

**Why it fell.** Third-party engine: Mail is reimplemented in Go (`emersion/*` MIT + our own
JMAP). It is the largest challenge of the pivot.

---

<a id="0007"></a>

<a id="0007--sytue-shell--sytue-hub-para-integración-seamless"></a>

## 0007 — Sytue Shell + Sytue Hub for seamless integration

`accepted` (reconciled by the pivot) · 2026-06-30 · integration, shell, hub

**Context.** Seamless Google Workspace-style integration: single login, common shell,
app switcher, global search and notifications, presence and cross actions, without
each app reinventing the topbar or the cross-app logic.

**Decision.** Two complementary pieces:

1. **Sytue Shell** (`@sytue/shell`) — unified topbar, app switcher, global search,
   notification center, presence and account menu. It is mounted in **every** app.
2. **Sytue Hub** — integration service with federated search, unified notifications
   (SSE/WebSocket), presence and **cross-app intents** (attach from Drive, create an event from
   an email, share in Chat).

No app talks directly to the backends for cross-cutting functions.

**Ruled out.** Each app implementing its own (divergence), Shell without Hub (coupling and
scattered secrets), a single monolith (breaks independent deployment).

**Reconciliation (pivot).** The Hub is a **Go service** (not TypeScript/Fastify), it federates
our own Go services, **the SCIM bridge to engines disappears**, it authorizes with Sytue ID
JWTs and uses **PostgreSQL** —without Valkey— for cache/events.

**Consequences.** The Hub is a critical service and a possible bottleneck: HA, asynchronous
design and graceful degradation of federated search.

---

<a id="0008"></a>

<a id="0008--valkey-en-lugar-de-redis--superseded-por-0019"></a>

## 0008 — Valkey instead of Redis · superseded by [0019](#0019)

`superseded` · 2026-06-30 · cache, licensing

**Decision (historical).** **Valkey** (BSD fork of Redis, Linux Foundation) as cache and queue,
after Redis switched to RSALv2/SSPL — not free and therefore incompatible with the
FOSS principle of the time. Ruled out KeyDB (momentum), DragonflyDB (BSL) and non-Redis brokers
(rewriting integrations).

**Why it fell.** The consumers that required the Redis API disappear; the single
datastore is Postgres.

---

<a id="0009"></a>

<a id="0009--todo-foss-libre-y-estrategia-de-fork--superseded-por-0017-0018"></a>

## 0009 — All free FOSS and fork strategy · superseded by [0017](#0017), [0018](#0018)

`superseded` · 2026-06-30 · licensing, strategy

**Decision (historical).** The entire stack free FOSS (Apache-2.0/MIT/AGPL-3.0/BSD) with a
**graduated customization strategy**: (1) theming, (2) our own client on top of a free
API/protocol, (3) forking the frontend when the integration required it — keeping the
upstream backend. SSPL/BSL/source-available were forbidden.

**Why it fell.** With no engines to fork and with original work of our own, the policy becomes
a proprietary product with permissive-only dependencies.

---

<a id="0010"></a>

<a id="0010--clientes-propios-sobre-la-ui-upstream--superseded-por-0017"></a>

## 0010 — Own clients on top of the upstream UI · superseded by [0017](#0017)

`superseded` · 2026-06-30 · frontend, ux

**Decision (historical).** For Drive, Mail, Calendar, Contacts and Docs, **our own
Astro+Preact clients from scratch** consuming only the engine's free protocols (WebDAV/OCS,
JMAP, CalDAV, CardDAV, embedded editor), with `@sytue/shell` + `@sytue/ui`. Exception:
Chat/Meet reused the Mattermost webapp.

**Ruled out at the time.** Theming the native UI (low customization ceiling), forking
five frontends (maintenance), iframes (fragile and no cross-app), a single monolith.

**Why it fell.** There is no longer an “upstream UI”: the clients consume our own Go services. The
frontend stack ([0004](#0004)) remains in force.

---

<a id="0011"></a>

<a id="0011--modelo-paas-y-multi-tenancy"></a>

## 0011 — PaaS model and multi-tenancy

`accepted` · 2026-06-30 · paas, multi-tenant, k8s

**Decision.** Hierarchical model **Organization (tenant) → Workspace → Users**, with a 1:1
mapping across the whole stack: `tenant <slug>` ⇄ identity group `org/<slug>` ⇄ **Kapsule
namespace `tenant-<slug>`** (with `ResourceQuota` + `NetworkPolicy`) ⇄ tenant storage
(S3 buckets/prefixes) ⇄ the tenant's product surfaces.

- Roles `owner`, `admin`, `member`, `billing` within the organization's group.
- Compute isolation per namespace, with normative labels
  (`app.kubernetes.io/part-of=sytue-one`, `sytue.eu/env`, `sytue.eu/tenant`).
- PaaS: `git push → webhook → build rootless → registry → deploy → URL con TLS`. Apps at
  `<app>.apps.sytue.eu`, previews at `<proyecto>-pr-<n>.preview.sytue.eu`, custom domains
  via CNAME to `ingress.sytue.eu`.
- Quotas, metrics and billing **per tenant**.

**Ruled out.** Multi-tenancy only via `tenant_id` in shared tables (weak isolation),
one cluster per tenant (cost), namespaces without NetworkPolicy/quota (noisy neighbor), a
different tenancy model per product (breaks “one sign-up = access to everything”).

**Consequences.** Real isolation suitable for GDPR and usage-based billing; in exchange,
idempotent reconciliation of the mapping, per-tenant overhead (automate onboarding) and
pressure on the control plane as namespaces grow (see the cell/sharding of
[0023](#0023)).

---

<a id="0012"></a>

<a id="0012--servicios-gestionados-de-sytue-cloud-vercel--supabase"></a>

## 0012 — Sytue Cloud managed services (Vercel × Supabase)

`accepted` (reconciled by the pivot) · 2026-06-30 · paas, dx

**Decision.** Sytue Cloud combines a **Vercel-style deploy flow** (connect repo →
build → preview per PR → promotion to prod → rollback, with automatic domains and TLS) with a
**Supabase-style catalog of managed services**, all sovereign and with dev/prod
parity:

- **Sytue Postgres** — managed PostgreSQL with an **automatic REST API (PostgREST)**,
  pooling (PgBouncer), backups and data branching.
- **Sytue DB Studio** — table explorer, SQL editor and API/credentials generator,
  with Sytue design, embedded in the dashboard.
- **Sytue Storage** (S3 per project), **Cache/Queue**, **Functions**, **Cron**,
  **Secrets/Env** and optional **managed Auth** for customer apps.
- **Zero-friction connection:** creating a service automatically injects its credentials into
  the project's deployments. Observability in plain language, without cloud jargon.

**Ruled out.** Using the real Vercel/Supabase/GCP (US, proprietary); adopting an existing FOSS
PaaS as the product (none integrates the catalog with our tenancy/IdP — ideas and
components are reused); exposing raw Kubernetes (contradicts “simpler than Google Cloud”);
Hasura instead of PostgREST (heavier for the simplicity we seek).

**Reconciliation (pivot).** The control plane is **Go** (`services/cloud/`); Cache/Queue is
**PostgreSQL**, not Valkey; managed Auth is backed by **Sytue ID**; the code is
proprietary. PostgREST/PgBouncer are kept.

---

<a id="0013"></a>

<a id="0013--pipeline-de-build-con-kaniko"></a>

## 0013 — Build pipeline with Kaniko

`accepted` · 2026-06-30 · paas, ci, build · ⚠ **review pending**

**Context.** Building images inside the cluster with Docker `build` requires a daemon and
privileges: unacceptable in multi-tenant.

**Decision.** **Kaniko** as the PaaS build tool: each build is a **rootless Kubernetes
Job** that takes the repo after the webhook, detects the framework or uses the
Dockerfile, builds the OCI image **without a daemon or privileges**, publishes it to the
EU registry and triggers the deploy. Build cache in a versioned S3 bucket.

**Ruled out.** Docker-in-Docker / host socket (escape to host), rootless BuildKit
(better caching but one more daemon to operate — under watch), Buildah (somewhat worse fit as an ephemeral
Job), Buildpacks (complementary, not a substitute), external SaaS CI (sovereignty).

**⚠ Follow-up.** **Kaniko was archived by Google (2025)**: a corrective decision is due
to migrate to **rootless BuildKit** or to the maintained fork **before GA** — it runs
customer code, and unpatched CVEs are unacceptable. The external contract (push → URL) does not
change.

---

<a id="0014"></a>

<a id="0014--iac-dual-compose-local--helmterraform-en-scaleway"></a>

## 0014 — Dual IaC: local Compose + Helm/Terraform on Scaleway

`accepted` · 2026-06-30 · infra, devex, iac

**Decision.** **Logical parity** with two tools:

- **Development = LOCAL with Docker Compose.** `make dev-up` brings up the whole suite in
  `deploy/compose/`, under `*.sytue.localhost` with **local TLS (mkcert)** via **Traefik**, with
  sample data. The Cloud managed services have a local equivalent (PostgreSQL +
  PostgREST + MinIO).
- **Production = SCALEWAY.** **Terraform** provisions the infrastructure in
  `infra/terraform/` with reusable modules (`network`, `kapsule`, `database`,
  `object-storage`, `registry`, `dns`, `secrets`, `loadbalancer`) and
  dev/staging/prod environments; **Helm** deploys the `sytue-one` umbrella chart on Kapsule from
  `deploy/helm/`. Terraform state in an S3 backend.
- **Same logical configuration** (domains, OIDC clients, variables) parameterized per
  environment: endpoints and scale change, **not the architecture**. Secrets never in git.

**Ruled out.** Local K8s (kind/k3d) by default (heavy startup, worse DX — kept as an
advanced profile), cloud only (expensive and slow), Helmfile/Kustomize as the base, Pulumi, and forcing
a single tool for both environments.

**Consequences.** Fast onboarding and less “works on my machine”; in exchange, **two
definitions to maintain** (risk of divergence: every new service is added to both
Compose and the chart at the same time) and failures that only show up on Kapsule (NetworkPolicy, quotas,
scheduling) → staging before prod.

---

<a id="0015"></a>

<a id="0015--licenciamiento-agpl-30-del-código-propio--superseded-por-0018"></a>

## 0015 — AGPL-3.0 licensing of our own code · superseded by [0018](#0018)

`superseded` · 2026-06-30 · licensing

**Decision (historical).** Our own code under **AGPL-3.0**: the network clause forces
anyone offering a modified version as a service to publish the source, closing the GPL's
“ASP loophole”. Compatible with the free engines of the time. Ruled out
permissive licenses (they would allow closed SaaS with nothing in return), GPL-3.0 (does not cover services),
open core (non-free parts) and source-available (not OSI).

**Why it fell.** With no AGPL engines and each product being original work, copyleft is no longer
necessary and it limits the business.

---

<a id="0016"></a>

<a id="0016--stack-único-y-total-go--astropreact--postgresql"></a>

## 0016 — A single, total stack: Go + Astro/Preact + PostgreSQL

`accepted` · 2026-07-01 · **parent decision of the pivot**

**Context.** The 0002–0015 design assembled third-party engines: a polyglot stack (PHP,
Rust, Java, TypeScript, Go), five AGPL engines to operate/theme/fork/federate, and an
in-memory datastore in addition to Postgres.

**Decision.** A **single, total** stack, with no language exceptions in the backend:

- **Backend = Go.** Single module `git.sytue.eu/sytue/one`; shared libraries in
  `platform/pkg/*`; one binary per service in `services/<svc>/cmd/<svc>/main.go`; layers
  `internal/{http,domain,store}`; `pgx` + `sqlc`, `goose`/`golang-migrate` migrations,
  `chi` router, `slog` logs, `context` in all I/O; multi-stage Dockerfile to distroless;
  proprietary SPDX header per file.
- **Frontend = Astro + Preact** (islands), Tailwind — see [0004](#0004); the
  `@sytue/api-clients` SDKs now point to the Go services.
- **SINGLE datastore = PostgreSQL.** State, queues and cache on Postgres ([0019](#0019)).
- **Large blobs** in Object Storage S3 (Scaleway prod / MinIO dev).

The only third-party application exception: **LiveKit** ([0021](#0021)).

**Ruled out.** Keeping the polyglot stack; a TypeScript/Node backend (worse fit for
long-lived servers and SMTP/IMAP/DAV protocols); Rust (learning curve and a less mature permissive
groupware ecosystem); keeping Valkey; allowing “islands” of other languages per service.

**Consequences.** A single mental model and a minimal operational surface (one datastore, one
runtime, one front stack); in exchange, what the engines provided must be **reimplemented** —
a large effort, with the risk of falling short of upstream in edge cases (antispam,
CalDAV recurrences, OOXML fidelity). Every new backend is born as `services/<svc>/` in Go.

---

<a id="0017"></a>

<a id="0017--reimplementación-nativa-clean-room-sobre-estándares-abiertos"></a>

## 0017 — Native clean-room reimplementation on open standards

`accepted` · 2026-07-01 · supersedes 0003, 0005, 0006, 0009, 0010

**Context.** With the engines retired, we need an engineering policy that produces
**original proprietary code** that interoperates with standard clients without creating a
derivative work of copyleft code.

**Decision.** Each product is reimplemented in Go as **original work through a
clean-room process** on open standards and RFCs, using **only permissive libraries**
(MIT/BSD/Apache/ISC):

| Product                      | Standard implemented                                   | Supporting libraries                                       |
| ---------------------------- | ------------------------------------------------------ | ---------------------------------------------------------- |
| **Sytue ID**                 | OIDC/OAuth2 (Auth Code+PKCE), JWKS, SCIM, MFA/WebAuthn | `ory/fosite` or `zitadel/oidc`                             |
| **Sytue Drive**              | WebDAV + our own REST API                              | `x/net/webdav`, `emersion/go-webdav`                       |
| **Sytue Docs/Sheets/Slides** | OOXML/ODF document model + CRDT/OT collab (WS)         | our own editor + `coder/websocket`                         |
| **Sytue Mail**               | SMTP/IMAP/JMAP                                         | `emersion/go-smtp`, `go-imap`, `go-message` + our own JMAP |
| **Sytue Calendar**           | CalDAV + REST API                                      | `emersion/go-webdav`, `emersion/go-ical`                   |
| **Sytue Contacts**           | CardDAV + REST API                                     | `emersion/go-webdav`, `emersion/go-vcard`                  |
| **Sytue Rooms**              | WebSocket + REST API                                   | `coder/websocket`                                          |

**Clean-room rules (binding).**

- **The source code of the AGPL engines is not consulted.** Work is done from RFCs,
  specifications and public APIs: ideas, functionality, _look & feel_ and
  protocols are not copyrighted; **code is**. Copying, translating or porting is forbidden.
- Linked libraries must be permissive (license gate in CI); NOTICE
  files are preserved.
- Each implementation records its **reference standard** and its **libraries**; proprietary
  SPDX header per file.
- **Interoperability as an acceptance criterion**: each service is validated against standard
  clients of its protocol, not against the engine it replaces.

**Ruled out.** Keeping the engines and building only clients; rewriting “reading the AGPL
code as a reference” (derivative work — unacceptable legal risk); acquiring a third-party
proprietary codebase; implementing closed protocols instead of standards.

**Risks (explicit magnitude).** **Mail is the biggest challenge** (deliverability, antispam,
queues, IP reputation, SPF/DKIM/DMARC/MTA-STS/TLS-RPT); **the office suite is the second**
(OOXML/ODF fidelity + collaboration). Mitigation: phased scope, proven permissive
libraries and interoperability suites per protocol.

---

<a id="0018"></a>

<a id="0018--licenciamiento-propietario-del-código-de-producto--sytue"></a>

## 0018 — Proprietary licensing of the product code (© Sytue)

`accepted` · 2026-07-01 · supersedes 0015

**Decision.** Sytue's product code is **Apache-2.0 (© 2026 Daniel Delgado Ramos (thinkddr))**, with no copyleft or
obligation to publish source: it covers `services/*`, `platform/pkg/*`, the
Astro+Preact surfaces and the `@sytue/*` packages.

- **SPDX header per Go file:** `MIT` + `© Sytue`; each frontend
  package with its proprietary `LICENSE`.
- **Dependencies incorporated into the product: permissive only** — MIT/BSD/Apache-2.0/ISC/
  PostgreSQL and, with caution, MPL-2.0. LICENSE/NOTICE files are preserved.
- **AGPL/GPL and source-available (SSPL/BSL/MSL/Elastic/RSAL) are forbidden** as a linked
  or incorporated dependency or as an engine of the product delivered to the customer. Operational
  infrastructure services require an individual decision and are never presumed allowed.
- **LiveKit (Apache-2.0)** is the only third-party application allowed in the customer runtime.
- **Product license gate in CI** with SBOM (CycloneDX/SPDX): any non-allowed
  dependency that reaches the artifact breaks the build. An unknown license requires a decision;
  strong copyleft and _source-available_ remain forbidden in the product perimeter.

**Scope amendment — 2026-08-01.** Internal engineering tools are evaluated in
a separate perimeter. A GPL/AGPL executable or service may be used for version control,
forge, CI, analysis or administration when it is standalone, is not linked or packaged with Sytue
artifacts, is not offered as functionality to customers, is inventoried and all its
obligations are fully met (including offering source and modifications where applicable). Its output is not
automatically considered derivative: the gate verifies that nothing from the tool gets into the
binaries, images or installers. _Source-available_ licenses remain blocked by default
for tooling too and require a specific ADR. This separation does not relax the EU-origin policy,
which applies independently to product, infrastructure and tooling.

**Ruled out.** Staying with AGPL, a permissive license for our own code, open core, source-available.

**Consequences.** Full control of the product and the business model; in exchange, strict
compliance discipline (headers, SBOM, NOTICE) and the risk of pulling in transitive copyleft
→ automated blocking in CI. It does not affect customers' data or code.

---

<a id="0019"></a>

<a id="0019--postgresql-como-datastore-único-colas-y-caché-sobre-postgres"></a>

## 0019 — PostgreSQL as the single datastore (queues and cache on Postgres)

`accepted` · 2026-07-01 · supersedes 0008

**Decision.** **PostgreSQL is the SINGLE datastore**: state, queues and cache on Postgres.
Valkey/Redis is removed from the stack.

- **Queues / background jobs:** `riverqueue` or `pgmq` (transactional queues) and/or
  `LISTEN`/`NOTIFY` for fan-out; Go workers, _at-least-once_, retries and dead-letter.
- **Cache:** tables with TTL/invalidation or in-process cache within each service.
- **Realtime/pub-sub:** `LISTEN`/`NOTIFY` as a lightweight bus; WebSocket/SSE to the client
  from the Go services themselves.
- **Large blobs:** to Object Storage, **not** to Postgres.
- **Multi-tenant:** schema/RLS per tenant; S3 prefixes per tenant.

**Ruled out.** Keeping Valkey (an extra piece with no consumers that require it), going back to
Redis (license), a dedicated broker (NATS/RabbitMQ/Kafka — oversized; can be reconsidered
with a new decision if scale demands it), our own distributed cache.

**Consequences.** A single data piece to operate and back up, and **transactional
queues** (enqueuing in the same transaction as the state change eliminates
“committed in the DB but not in the queue”); in exchange, a different performance ceiling than an in-memory
KV → `LISTEN/NOTIFY` instead of aggressive polling, tuned autovacuum and indexes,
partitioning/archiving of the queue tables and in-process cache for _hot paths_.

> **Later note (in force).** The scale invariant led to adopting **DragonflyDB** as
> large-scale cache/pub-sub for internal use; see `CONVENTIONS.md`. The “no external
> KV” rule of this decision is limited to what Postgres covers well.

---

<a id="0020"></a>

<a id="0020--sytue-id-como-idp-nativo-en-go-oidcoauth2scimmfa"></a>

## 0020 — Sytue ID as a native IdP in Go (OIDC/OAuth2/SCIM/MFA)

`accepted` · 2026-07-01 · supersedes 0002

**Decision.** **Sytue ID** is our own Go service that implements, on top of
Apache-2.0 libraries (`ory/fosite` or `zitadel/oidc`): **OIDC/OAuth2** Authorization Code + **PKCE**,
refresh tokens, **JWKS**, discovery, **SCIM** and **MFA/WebAuthn** (SAML as needed for
enterprise). Identity data in PostgreSQL.

- **OIDC clients** (confidential BFF per surface): one per app. There are no engine
  federation clients.
- **Each Go service validates JWTs via JWKS** with the `authn` middleware from
  `platform/pkg/authn`, and applies authorization by roles/tenant.
- **Groups/roles:** `org/<tenant>` with `owner`, `admin`, `member`, `billing`; **MFA/WebAuthn
  mandatory** for `admin`/`owner`. One sign-up in Sytue ID → access to the whole suite.
- **SSO:** host-only `__Host-` cookie per BFF after OIDC+PKCE; Sytue ID avoids re-entering credentials.
- The Meet BFF mints the LiveKit JWT **after** validating OIDC.

**Ruled out.** Keeping Keycloak (a third-party Java engine as a core piece); another deployed
third-party IdP (we do use their libraries); US identity SaaS; implementing OIDC “by
hand” without a mature library (extremely high security risk).

**Dominant risk.** It is the **highest security-risk decision in the suite**: a flaw
in tokens, sessions, PKCE, refresh, JWKS or WebAuthn compromises **all** products.
Mandatory mitigation: proven libraries for the cryptographic flows, **external security
audit before production**, OIDC conformance and pentest. It is also a single point
of failure for SSO → HA, replicated Postgres and a priority recovery runbook.

---

<a id="0021"></a>

<a id="0021--livekit-apache-20-como-sfu-de-medios-de-sytue-rooms"></a>

## 0021 — LiveKit (Apache-2.0) as the media SFU for Sytue Rooms

`accepted` · 2026-07-01 · meet, webrtc

**Context.** Signaling and rooms can be our own Go service, but the **WebRTC media
plane** —an SFU with simulcast, congestion control, loss handling,
TURN/ICE— is years of specialized engineering.

**Decision.** **Sytue Rooms** = **our own Go signaling/rooms service** (rooms, permissions,
presence and state in PostgreSQL, OIDC against Sytue ID) + **LiveKit** (Apache-2.0, written
in Go) as the **media SFU**, the **only third-party application allowed in the customer
runtime**. The BFF mints
the LiveKit JWT after validating OIDC; the frontend uses the web SDK inside the Astro app.

**Ruled out.** Reimplementing the SFU (years of work and extremely high risk), Mattermost
Calls/rtcd (coupled to the retired engine), Jitsi (Java stack and a complete product that does not
fit), building on Pion (amounts to reimplementing the SFU), US video SaaS.

**Consequences.** Production-quality video without taking on the SFU risk; in exchange,
dependency on a third-party component (pinned version, CVEs, NOTICE) and demanding WebRTC
operations (TURN/ICE, NAT, UDP range). A **bounded** exception: no other third-party app without
a recorded decision. State lives in Postgres; LiveKit only transports media.

---

<a id="0022"></a>

<a id="0023--eliminación-de-los-motores-agpl-de-terceros"></a>

## 0023 — Removal of third-party AGPL engines

`accepted` · 2026-07-01 · engines, licensing

**Decision.** **All** third-party AGPL engines are removed from the stack — Keycloak,
Nextcloud, OnlyOffice, Mattermost and Stalwart — plus Valkey/Redis:

| Retired engine   | Function                   | Sytue-native replacement (Go)                           |
| ---------------- | -------------------------- | ------------------------------------------------------- |
| **Keycloak**     | Identity/SSO/SCIM/MFA      | **Sytue ID** ([0020](#0020))                            |
| **Nextcloud**    | Files + CalDAV + CardDAV   | **Drive** + **Calendar** + **Contacts** ([0017](#0017)) |
| **OnlyOffice**   | Collaborative office suite | **Docs/Sheets/Slides** ([0017](#0017))                  |
| **Stalwart**     | SMTP/IMAP/JMAP mail        | **Sytue Mail** ([0017](#0017))                          |
| **Mattermost**   | Chat + video               | **Sytue Rooms** ([0017](#0017), [0021](#0021))          |
| **Valkey/Redis** | Cache/queue                | **PostgreSQL** ([0019](#0019))                          |

The Keycloak configuration, the TypeScript Hub and PaaS and the engine themes are left out
of the repo; no charts for AGPL engines and no federation OIDC clients.

**Ruled out.** Keeping the engines; **retiring only some** (it would keep a foreign engine in the
customer runtime, its obligations and the heterogeneity we want to eliminate); replacing them with
other permissive third-party engines, except LiveKit.

**Consequences.** Zero AGPL dependencies incorporated into the product and zero AGPL engines in its runtime;
a single stack with no fork rebases; in exchange, loss of mature functionality that must be rebuilt
(see magnitudes in [0017](#0017)) and **clean-room compliance risk**: guaranteeing that nobody
consults the AGPL source. This decision does not govern standalone internal tooling executables, whose
perimeter was separated in the amendment to [0018](#0018).

---

<a id="0023"></a>

<a id="0023--sustrato-de-sytue-cloud-kapsule-coolify-como-referencia-firecracker-como-escotilla"></a>

## 0023 — Sytue Cloud substrate: Kapsule, Coolify as a reference, Firecracker as an escape hatch

`accepted` · 2026-07-10 · paas, substrate

**Context.** Can a refactored Coolify serve as the base for Sytue Cloud? Is
Kubernetes/Kapsule the right substrate, or is **Firecracker** (the Fly.io model) better for
hostile multi-tenant isolation? The decision affects the scale invariant (hundreds of
thousands of users), “backend always Go” and the tenancy model already set.

**Decision.**

1. **Coolify: a domain reference, not a source. No MVP shortcut.** Porting or
   forking it is ruled out, and so is running the real Coolify behind the Astro UI via its API: it contributes ~zero
   reusable code to a Go stack (its value is the agnostic engine it orchestrates, which Go handles
   natively), its architecture is **single-tenant** and the shortcut violates “backend always Go” +
   scale. Its **domain model** (apps/services/envs/deploys/preview/one-click
   catalog) is taken as a reference.
2. **Substrate: managed Kubernetes (Kapsule)**, reaffirming [0011](#0011) — tenant →
   namespace with ResourceQuota + NetworkPolicy. Already in production in the repo:
   `services/cloud/internal/controller/kubeplatform.go` (Server-Side Apply of
   Deployment/Service/Ingress/HPA), the daemonless OCI builder, the charts and the Terraform
   modules. Kapsule provides the pooled control plane; the ecosystem is permissive.
3. **Firecracker: rejected as the initial substrate, preserved as an escape hatch.** A
   `flyd`-style orchestrator is 2-3 engineer-years of distributed systems that K8s already paid for, and
   it would throw away the `services/cloud` that already works. The option is preserved via the
   **`controller.Platform`** interface: a future Firecracker/Kata cell plugs in without touching the control
   plane. The shared kernel is K8s's honest weakness; it is mitigated (not matched)
   with PodSecurity `restricted`, seccomp, `automountServiceAccountToken=false`,
   NetworkPolicy and **dedicated pools per plan**.

**Ruled out.** Fork/port of Coolify; Firecracker as primary; **Nomad** (BSL 1.1).

**Follow-up (gaps detected, by value).**

1. **Static-first offload:** serve static sites from S3 via a shared `cloud-edge`
   instead of an always-on container per site (~10× cheaper at 100k sites).
2. **`NetworkPolicy` egress:** allow the Internet **except** RFC1918, the cluster CIDR and
   `169.254.169.254`, so that a tenant cannot reach internal services.
3. **`LimitRange` per tenant namespace**, in addition to the `ResourceQuota`.
4. **Listing scale:** **keyset** cursors (not OFFSET) + a purge/rollup cron for
   builds/webhooks/outbox/retired previews/usage.
5. **Sharding into cells:** an `organizations.cell` column to distribute orgs across clusters.
6. **`scale-to-zero`** of previews/free with KEDA (paid prod `minReplicas≥1`).
7. ⚠ **Kaniko archived** → migrate to rootless BuildKit before GA (see [0013](#0013)).

---

<a id="0024"></a>

<a id="0024--apps-de-escritorio-fyne-y-móvil-pwa--capacitor"></a>

## 0024 — Desktop (Fyne) and mobile (PWA + Capacitor) apps

`accepted` · 2026-07-11 (revised 2026-09-20) · desktop, mobile

**Context.** Parity with OnlyOffice requires desktop and mobile apps, with
constraints: **single codebase** (no separate native editors to maintain), permissive clean-room,
sovereignty, reasonable offline support and **a single systems language (Go)**.

**Decision.** Keep the shared domain and APIs, with native interfaces where they are
installed as an application:

- **Desktop → Fyne 2.8.1** (Go, BSD-3). Views are rendered natively on
  macOS, Windows and Linux, without WebView2, WKWebView, WebKitGTK or Chromium. The shell and its
  views live in `apps/desktop/shell/gui/`; they consume the same authenticated Sytue
  ID APIs and Go services as the web apps.
- **Mobile → installable PWA as the base, Capacitor (MIT) for the stores** and native APIs,
  reusing the same web app.
- **Offline by capability, not by platform.** The Fyne apps keep their local state and
  sync it with the APIs on reconnect; the mobile apps keep their PWA/
  Capacitor strategy. Irreversible actions, or ones that depend on real-time truth, are not simulated. The
  Go backend is **not** packaged in the client. Contract: `docs/roadmap.md#offline-native`.

**Ruled out.** **Tauri 2** (Rust core: a second systems language, no net advantage
over Fyne), **Wails** (depends on a WebView), **Electron** (bundles Chromium, 100+ MB),
React Native / native / Flutter (separate codebase → duplicating the editors), PWA only without
Capacitor (no stores).

**Cost / follow-up.** New pipelines (code signing, macOS notarization, Xcode/Android
SDK and store accounts); maintain tests for recovery/conflicts, OIDC PKCE, `sytue://` deep
links and secure local storage.

---

<a id="0025"></a>

<a id="0025--sytue-rooms-mensajería-y-videollamada-un-servicio-y-una-base"></a>

## 0025 — Sytue Rooms: messaging and video calls, one service and one database

`accepted` · 2026-07-28 · rooms, architecture

**Context.** Chat and Meet were born as two Go services with two databases, and from the
first day they had to talk to each other: the call starts FROM a channel, so the video must
ask “can this person read the channel?” before opening the room, and on hanging up it must
post the “call ended” card IN that channel. That conversation traveled over HTTP
between processes with a shared secret.

The coupling was not theoretical: if the secret was missing in an environment, the bridge was left nil and
the channel gate failed **closed** — opening a call returned 403 with no possible explanation
for whoever hit it. It happened in dev and in the chart. And the end-of-call card can never be
atomic with the message while there are two databases.

For the user, moreover, they were never two products: the call is a button in the channel.

**Decision.** **One service** (`services/rooms`) and **one database** (`sytue_rooms`). Messaging
lives in `internal/`, video calls in `internal/media/`, and the bridge between them is an
in-process call (`media/service.InProcessNotifier`) within the same transaction. A single
`events_outbox`, a single deployment, a single host (`rooms.<dominio>`: the frontend serves `/` and the
Go service `/api`, `/ws` and `/hooks`). The LiveKit SFU remains a separate component
([0021](#0021)) at `rooms-sfu.<dominio>`: it transports media, it does not store state.

**Ruled out.** Keeping the two services and fixing the secret (leaves standing the impossibility
of atomicity and two deployments to version together); a database shared between two
services (worse: schema coupling without the advantage of the transaction); a monolith of the
entire suite (Rooms is merged because it shares a domain —the channel—, not as a fad).

**Consequences.** `CHAT_INTERNAL_SECRET` disappears on this path, along with a chart, an image and
a database. Video scaling is no longer independent of messaging scaling — this is accepted
because the heavy lifting of video is done by the SFU, not by this process, which only orchestrates rooms,
tokens and notifications. Migrating an already-deployed environment requires downtime and is written up in
[`operations.md` §1.4.4](./operations.md#44-fusión-de-sytue_chat--sytue_meet--sytue_rooms-una-sola-vez-por-entorno);
rolling back after the first write loses data.

---

<a id="0026"></a>

<a id="0026--sytue-analytics-anonimato-por-construcción-medición-en-primera-parte"></a>

## 0026 — Sytue Analytics: anonymity by construction, first-party measurement

`accepted` · 2026-07-29 · analytics, gdpr, privacy, cloud

**Context.** Whoever deploys an app on Sytue Cloud has no way of knowing how many people
visit it. Adopting something existing is ruled out before we even start, and not out of preference:
**Plausible (AGPL) and Matomo (GPL)** are analytics applications that would fall into
**perimeter B** (runtime that processes customer data), not mere internal tools. §12 does not
claim that a separate service makes the product copyleft, but it requires an ADR, isolation,
source compliance, EU origin and an exit plan; moreover, they would break the principle of not adding another
product engine. **Umami** is MIT, so the license is not the problem, but it is Node and
[`§0`](./CONVENTIONS.md#0-principios-nucleares-en-orden-de-prioridad) principle 1 closes the door
—“no language exceptions in the backend”—; **GA4, PostHog, Fathom, Vercel Analytics** are
US processors and clash with [0001](#0001) and with `security.md §1` (“no US analytics
services are used”). **It is built in Go, in the monorepo.**

What remains to be decided is everything else, and **a single piece decides it**: counting unique
visitors without cookies and without storing IPs.

<a id="1-identidad-efímera-hmac-con-sal-diaria-que-se-destruye"></a>

### 1. Ephemeral identity: HMAC with a daily salt that is destroyed

`visitor = HMAC-SHA256(sal_del_día, site_id ‖ ip ‖ user_agent)`, truncated to 16 bytes. HMAC and not
`hash(sal‖datos)`: it is the right primitive for keyed hashing, it is in the stdlib
(`crypto/hmac`) and it costs nothing. **The salt is generated once a day, stored encrypted with
[`platform/pkg/keycrypto`](../platform/pkg/keycrypto/keycrypto.go) in a row of
`sytue_analytics`, cached in process, and the cron DELETES it when the day changes.** Encrypted because a
Postgres dump without the KEK from the secret manager does not allow recomputing any hash; in
Postgres and not in Valkey because §0.1 requires a performance justification and here it is **one read per
day**, not a hot path.

This is not a privacy policy: it is what the product **can** and **cannot** do.

1. **Without the salt there is no correlation across days.** Nobody —not even Sytue with root access— can
   reconstruct yesterday's visitor. Long-term profiling stops being forbidden and becomes
   **impossible**. Its price, which is accepted and documented in the interface: **there are no
   “unique visitors for the month”, no returning visitors, no cohorts.** A long range shows the **sum of
   daily uniques**, labeled as such — never a deduplicated monthly unique, which would be a lie.
   A visit that crosses midnight counts as two.
2. **`site_id` inside the HMAC.** The same person on the sites of two customers produces
   **uncorrelatable** identifiers, and not by contract but because the data to cross them does not
   exist. “No tracking outside each customer's services” is here a **structural
   impossibility**.
3. **Without cookies no banner is needed** — but only if the device is truly not touched, so
   it becomes a code invariant, not a promise: **`document.cookie`,
   `localStorage`, `sessionStorage`, `IndexedDB`, the Cache API and any active fingerprinting (canvas,
   WebGL, fonts, `hardwareConcurrency`, `deviceMemory`) are forbidden** — **including `screen.*`**, which the
   EDPB Guidelines 2/2023 treat as access to the terminal equipment; the device type is deduced
   from the user-agent, which the browser already sends. **A CI test searches for those identifiers in the
   COMPILED script and breaks the build.** Honesty about the scope: for 24 h the hash is pseudonymized
   personal data (legal basis: legitimate interest); **once the salt is destroyed, it is anonymous**. The
   exact legal text is signed by the owner with legal advice (see “Business decisions” in the report).

<a id="2-minimización-como-esquema-lo-que-no-se-escribe-no-se-filtra"></a>

### 2. Minimization as a schema: what is not written cannot leak

| Stored                    | Source                | Rule                                             |
| ------------------------- | --------------------- | ------------------------------------------------ |
| `visitor`                 | HMAC from §1          | `bytea(16)`, lives 48 h                          |
| `path`                    | `location.pathname`   | **no query string**, ≤255 chars                  |
| `referrer_host`           | `Referer`             | **host only**; if it is the site itself → `NULL` |
| `utm_*`, `ref`            | query string          | **allowlist of 6 exact keys**, ≤64 chars each    |
| `country`                 | IP, in memory         | ISO-3166-1 alpha-2; unknown → `ZZ`               |
| `browser`, `os`, `device` | user-agent, in memory | 3 small enums                                    |
| `event`                   | declared attribute    | **per-site allowlist**, ≤50 names                |

**Never, in any table or log: the IP** (neither hashed nor truncated to /24 — it still identifies),
**the full URL, the full `Referer`, the raw user-agent, the language** (a bit of
re-identification that adds nothing to the product), nor any identifier from the customer's app.

- **The query string is discarded IN THE BROWSER**, not on the server. An `?email=`, a
  `?reset_token=` or a session id that reaches the server has already crossed the network and gone through an
  access log or a dump. The server **also** cuts at `?` everything it receives, because the
  collector is public and anyone can send it whatever they want. Both, and the browser's
  first.
- **Country without IP.** **DB-IP IP-to-Country Lite (CC BY 4.0)** file embedded in the image,
  binary search over sorted `net/netip` ranges (~40 lines, zero dependencies, IPv4 and IPv6
  alike). The IP is a **local variable** of the handler: it goes into the HMAC, into the country
  lookup, into the rate limit, and **is not assigned to any field that is persisted**. An operational
  consequence that must be met or the product lies: **the Traefik access logs for the ingestion
  hostname do not retain client IPs.**
- **Path cardinality:** it is not normalized automatically (guessing is worse); what bounds
  growth is that the aggregate keeps **top-N per day** plus an `(otras)` (“others”) bucket.

<a id="3-volumen-partición-diaria--agregados-y-el-panel-nunca-toca-el-crudo"></a>

### 3. Volume: daily partitioning + aggregates, and the dashboard never touches the raw data

The same pattern as the `usage_records` rollup
([`store/purge.go`](../services/cloud/internal/store/purge.go)): short detail, long aggregate.

- `analytics_events` **partitioned by RANGE on `ts`, one partition per day**; the cron creates 7 days
  ahead and **`DROP TABLE`s** the expired ones — deleting millions of rows without vacuum or WAL.
- **Raw data retention: 48 h.** Deliberately short: it is where the pseudonymous hash lives. An
  accepted and explicit cost: **the definitions of the aggregates cannot be changed
  retroactively.**
- `analytics_hourly` and `analytics_daily` (totals) + `analytics_daily_breakdown`
  `(site, day, dim, value, pageviews, visitors)`. Two totals tables and not one because **uniques
  do not add up**: the day is computed over the raw data, not by summing hours. The aggregates **contain
  no visitor identifier** — they are anonymous, and that is why they can live for years.
- **Visits are reconstructed in the rollup** (same `visitor`, gap < 30 min), not in the
  browser: **there is no session cookie and no session id sent**, and the reconstruction dies with the
  raw data.
- **A concrete trap:** under `FORCE ROW LEVEL SECURITY`, PostgreSQL **rejects `COPY … FROM`**. The
  obvious firehose design is not available: writes use batched **multi-row `INSERT`**
  (~1 s or N events).

<a id="4-multi-tenant-el-que-escribe-no-puede-leer"></a>

### 4. Multi-tenant: the writer cannot read

`ENABLE` + `FORCE` + an explicit predicate, like the rest. In addition, two policies per command —the
ingestion process is intrinsically cross-tenant and has no reason at all to read:

```sql
CREATE POLICY analytics_events_tenant ON analytics_events
    USING (tenant = current_setting('app.tenant', true));
CREATE POLICY analytics_events_ingest ON analytics_events
    FOR INSERT WITH CHECK (current_setting('app.ingest', true) = 'on');
```

The rollup and the purge run under `app.maintenance='on'`, identical to `runtime_logs` and
`PurgeExpired`. **Never a role with `BYPASSRLS`.**

<a id="5-dónde-vive-servicesanalytics-y-por-qué-no-dentro-de-servicescloud"></a>

### 5. Where it lives: `services/analytics`, and why NOT inside `services/cloud`

**Its own service**, database `sytue_analytics`, two binaries: `analytics-ingest` (public, no auth,
minimal surface) and `analytics-api` (JWT via `platform/pkg/authn` + crons). The decisive argument
is not purity but the **blast radius**: the collector is the **public, unauthenticated,
highest-volume endpoint of the whole platform** —it receives the aggregated traffic of all
customers' apps— and `services/cloud` holds `SECRET_ENC_KEY`, Kubernetes
credentials and billing in memory. A bug in the beacon parser cannot be one hop away from that. On
top of that, its database is the one that decides whether a deploy can happen, and it cannot depend on a
firehose.

This is not the case of [0025](#0025): there the merge was required by **a transaction** (the message and the
end-of-call card). Here there is no transaction that crosses over. The composition —creating the
site, turning on the switch— is done by the **dashboard BFF with the user's token**, in two
calls: **no shared internal secret is created**, which was the lesson of 0025.

**The frontend is NOT a new app**: it is a section of `cloud-dashboard`
(`/projects/[slug]/analytics`). One fewer domain, OIDC client and switcher entry.

<a id="6-en-primera-parte-_sytue-en-el-propio-dominio-del-cliente"></a>

### 6. First-party: `/_sytue/` on the customer's own domain

**The script and the collector are served under the site's own domain**, not from a Sytue host.
`applyIngress` already builds the Ingress rules
([`kubeplatform.go`](../services/cloud/internal/controller/kubeplatform.go)): turning on analytics
**adds a second `/_sytue` (Prefix) route** that Traefik prioritizes over `/` because it is more
specific, pointing to an `ExternalName` `Service` in the tenant namespace towards
`analytics-ingest`. The price is `allowExternalNameServices=true` in Traefik — bounded because
tenants have no access to the Kubernetes API: Ingresses are written only by the control plane.

What it buys, and it is much more than it seems:

- **Nothing third-party**: no external domain to block, **zero cross-origin requests, zero
  CORS, and the customer does not have to open up their CSP** — a third-party analytics script forces
  all its customers to relax `script-src`.
- **The site is identified by the `Host`**, so **the script carries no identifier**: the
  snippet is an unconfigured tag. And the whole class of attacks
  “inflate another customer's numbers by forging their `site_id`” disappears by construction: **you cannot point the beacon at
  A while claiming to be B, because the site IS the hostname.**
- The honest floor that remains: **anyone who can send HTTP requests to a site can inflate its
  counts.** It is irreducible in any public cookieless collector. It is dampened with
  `Sec-Fetch-Site: cross-site` → rejection (a forbidden header, JS cannot forge it),
  mismatched `Origin`/`Host` → rejection, mandatory `POST` (no GET pixel: it gets cached and
  prefetched, and pollutes), a body limit with `httpx.limits` and an **in-memory token bucket per IP**
  (`httpx.TokenBucket`; known ceiling: N replicas → N× the limit, enough to dampen
  abuse, and without a trip to Postgres per event).

<a id="7-inyección-interruptor-en-el-panel-y-en-tiempo-de-build"></a>

### 7. Injection: a switch in the dashboard, and at BUILD time

Of the three real options, **none works on its own**:

- _Having the template ship with it enabled_ — `domain.TemplateManifest` is a **closed schema**
  (`DisallowUnknownFields`) that declares build, env and services, **not files**: the HTML comes from the
  user's repo. It works for new projects and does nothing for existing ones.
- _A switch that injects an environment variable_ — `SYTUE_ANALYTICS_*` does nothing on its
  own: someone has to read it and emit the tag. **That is an SDK under another name**, and it does not
  exist for a static site.
- _Injection at the edge (Traefik)_ — **ruled out**: there is no proxy that modifies responses and
  building one forces buffering or parsing the HTML of every app, decompressing and recompressing,
  breaking SSR streaming and `Content-Length`, and silently modifying the customer's output.

**The switch does two real things, and there is a third, manual path:**

1. **It publishes `/_sytue` in the project's Ingress** — the part the app cannot do on its own.
2. **It injects `<script defer src="/_sytue/s.js"></script>` before `</head>` at build time**,
   into the files that **the platform itself produces** (`detect.go` already generates the Dockerfile that
   copies `dist/` to nginx). Deterministic, zero per-request cost, and **it works on already
   existing projects on their next deployment**.
3. For SSR and apps with a server, where there is no static artifact to touch: **the one-line
   snippet**, with a copy button in the dashboard. No keys, no configuration.

<a id="8-el-script-los-bots-y-las-señales-del-navegador"></a>

### 8. The script, bots and browser signals

- **Budget: < 1.5 KB gzip, with a budget that breaks the build in CI** (§0.8 already does this with
  bundles). `navigator.sendBeacon` (does not block navigation, survives `unload`, and with
  `text/plain` does not trigger a preflight), `fetch(…, {keepalive:true})` as a fallback. It hooks
  `pushState`/`popstate` for SPAs. Silent `try/catch`: **it never breaks the customer's page**.
- **DNT and GPC: non-negotiable, no switch.** `navigator.globalPrivacyControl === true` or
  `navigator.doNotTrack === '1'` → **the script sends nothing**; and the server also honors
  `Sec-GPC: 1`, in case someone bypasses the script. Reason: the whole product is sold as
  “privacy by impossibility, not by policy”, and a switch to ignore GPC
  **reintroduces exactly the policy we removed**; it would also force us to explain to the customer that
  they can increase their numbers by turning it off, which is the conversation we do not want to have. It costs a few
  points of counting and buys the whole claim.
- **Bots.** Without filtering, the numbers are garbage. The **`crawler-user-agents` (MIT)** list is embedded,
  matched against the user-agent **before any write**; they are discarded, not flagged (flagging
  means storing more). An honest, written-down ceiling: **user-agent filtering is defeated by anyone
  who wants to defeat it** — it removes the honest garbage (crawlers, monitors, previewers), not
  an attacker. Filtering by datacenter ranges (a second dataset, ASN) **stays out until
  a customer reports numbers inflated from clouds**.
- **Clicks and events without an SDK.** Automatic, without touching anything: **outbound links** (only the destination
  host), **downloads** (extension from a fixed list) and `mailto:`/`tel:`. For everything else, **an
  HTML attribute**: `data-sytue-event="alta"`, with **a single delegated listener**. And the part that makes it
  a real GUI: **goals by path pattern** (`/gracias*`) declared in the dashboard, which are
  computed over data that already exists and **do not require a single line in the customer's app**. Event
  names are a **per-site allowlist**: without it, `data-sytue-event="pepe@correo.com"`
  turns the “anonymous” store into an exfiltration channel. **There is no JS API** (`window.sytue(…)`)
  in v1; it would be added only if something appears that is neither a page nor a click.

**Ruled out.** Plausible and Matomo (AGPL/GPL and additional engines in perimeter B: obligations,
operations and exit that are unnecessary compared with our own service); Umami (MIT but Node — the license
is fine, the language is not, §0 principle 1); GA4/PostHog/
Fathom/Vercel Analytics (US processors, [0001](#0001)); **ClickHouse** as the store (Apache-2.0,
so the license is not the problem: it is a **second datastore to operate** for a free
feature, against [0019](#0019) — partitioning + rollup keeps Postgres standing, and the trigger
to reopen it is written below); cookies, `localStorage` and fingerprinting; storing the IP in
any form; visitor identity across days; **edge injection rewriting HTML**;
a JavaScript SDK; a third-party script domain (`t.sytue.eu`) — which will only be built if
the business decides to sell this to sites **outside** Sytue Cloud.

**Consequences.**

- One more service, one more database and two more binaries to operate; **no new third-party dependency**
  except two data files (DB-IP CC BY 4.0, `crawler-user-agents` MIT) that require
  **attribution in NOTICE**.
- The dashboard **never queries the raw data**, which moreover only lives 48 h — and that is why **historical
  metrics cannot be redefined after the fact**.
- **Chart primitives are missing:** there are **none** in the repo (`@sytue/ui` has not a single one;
  Sheets' `Chart.tsx` is 980 lines tied to the formula engine). Three minimal dependency-free SVG pieces must be created in
  `@sytue/ui` —only once, as [0004](#0004) mandates—: a time series, a bar list and a statistic
  tile (which `usage.astro` already improvises inline and which this also fixes).
- **Requires updating [`CONVENTIONS.md`](./CONVENTIONS.md)**: a product row in §1, ports
  **8094** (api) and **8095** (ingest) in §6, and `ANALYTICS_DB_NAME=sytue_analytics`. Without that, the
  canonical contract and the code diverge.
- The **5 languages** (EN base + ES/FR/DE/IT) are a condition of done
  ([§13.1](./CONVENTIONS.md#13-1-idiomas-de-producto)), including metric names and the
  texts that explain what CANNOT be measured.
- **Written triggers to reopen this decision:** if a single day's raw data does not fit in a
  manageable partition in Postgres (or the rollup does not close the day within the hour),
  ClickHouse comes back to the table with a new decision; if numbers inflated from cloud ranges appear,
  ASN filtering comes in; if a customer demands certified figures, the IAB/ABC list is paid
  and is a business decision.

---

<a id="0027"></a>

<a id="0027--excepción-temporal-bunkaniko-para-v016-interna"></a>

## 0027 — Temporary Bun/Kaniko exception for internal `v0.1.6`

`accepted` · 2026-08-01 · security, sovereignty, release · **expires 2026-10-01**

**Context.** The first real production will have at most ten internal users, no customers,
and serves to validate iterations. The `v0.1.6` candidate has passed build, tests, PostgreSQL
integration, vulnerability scanning, secrets, licenses and SBOM generation; the release
workflow also requires signing each image before publishing the draft. Even so, the
SSR/BFF frontend runs Bun —a product under the control of Anthropic (USA)— and the OCI build uses
Kaniko —originated by Google and archived—. The general policy only exempts GitHub and would therefore
prevent publishing the candidate without an express decision by the owner.

**Decision.** **A single exception** is authorized, exclusively to produce and operate
`v0.1.6` in the internal validation of Sytue One:

- Bun may remain as the runtime of the `v0.1.6` frontends and as a build tool.
- Kaniko may build the images of that release and the Sytue Cloud test builds.
- The exception includes downloading their official images pinned by digest within GitHub
  Actions; all resulting images live privately in Scaleway Registry, with SBOM and signature.
- It does not authorize customers, GA, new dependencies of non-EU origin, extending Sytue Cloud to
  third-party workloads, or reusing the exception for `v0.1.7` or any later release.

**Compensating controls.** Private EU registry; pinned images and Actions; cosign signature;
CycloneDX SBOM; OSV/Trivy/gitleaks scanning; rootless execution, seccomp, dropped capabilities,
read-only filesystem and bounded egress. The providers of origin do not process customer
data: the runtime runs on Scaleway and this phase has no customers.

**Mandatory exit.** Before another release, the BFF/SSR are moved to Go and
static frontends are generated without Bun at runtime; Kaniko is replaced by a maintained chain approved by
the origin gate. On 2026-10-01 the exception expires even if no replacement exists. Renewing it requires
another ADR and a new express approval; silence does not extend it.

<a id="0028"></a>

<a id="0028--benchmarks-anónimos-entre-clientes-de-sytue-cloud"></a>

## 0028 — Anonymous benchmarks across Sytue Cloud customers

- **Date:** 2026-08-05
- **Status:** accepted
- **Context:** ADR 0023 (Kapsule), ADR 0026 (Analytics), `services/cloud/internal/domain/benchmark.go`

**Context.** The sizing engine (`RecommendInstanceSize`) decides with the project's own
history and requires three days of observation to propose a downsize. That leaves a gap exactly
where the most money is lost: the first days, when the customer chose a size blindly and almost
always overshot. During that window the only honest answer we knew how to give was “we are still
measuring”.

The data that would fill the gap exists, but it belongs to other customers: hundreds of projects with the same
framework, the same services and the same size already tell us how much an app like that consumes. Using it
collides head-on with the invariant that holds up the platform —RLS exists precisely so that
one tenant's data does not reach another—, so “anonymizing” is not enough: we have to
decide under what conditions that line is crossed and make them verifiable.

**Decision.** Cross-tenant aggregation is authorized **only** to publish memory occupancy
percentiles per cohort, and under five conditions that are part of the decision, not of the
implementation:

1. **Cohort = framework + services + size.** No identifiers, names, domains or
   activity traces. The key describes the technical shape of an app, not who it belongs to.
2. **k-anonymity threshold of 5 distinct COMPANIES**, counted by tenant and not by project.
   Counting projects would turn the threshold into decoration: a single customer would only need to create
   five projects to “anonymize” itself.
3. **The threshold is applied when PUBLISHING, not when querying.** A cohort that does not reach it is not
   written, so the read path cannot filter it wrongly because it does not exist. The
   `sizing_benchmarks` table has no `tenant` column —a row belongs to nobody— and the threshold is
   replicated as `CHECK (tenants >= 5)`: changing it requires touching the code and the migration.
4. **Only percentages of the size limit**, never absolute figures. Knowing that the median of
   a cohort is 12 % does not allow deducing the consumption of any specific company.
5. **Full replacement on each recomputation.** A cohort that today gathers four companies because the
   fifth unsubscribed disappears. With `upsert` it would remain published forever with the figures
   from the day it did reach the threshold.

And two limits on what the cohort can do with the recommendation:

- **The project's own history always wins.** The cohort only fills in `insufficient_data`; as soon as the
  project has its own data, its own data rules.
- **The cohort NEVER proposes upsizing, and downsizes by a single step.** Upsizing based on third-party consumption is
  charging more to someone we know nothing about; and with zero own data, jumping from `pro` to
  `basic` in one click is a gamble. One step corrects itself the next day and the savings arrive
  all the same. It also requires the same margin as the engine with its own data: the cohort's p90 has
  to fit comfortably in the smaller size.

The reason the interface sees is `cohort_over_provisioned`, deliberately different from `over_provisioned`:
the screen cannot say “we have measured your app” when what we have measured are those of
others, and the text names how many companies are behind it so the customer can check it.

**Consequences.** A newly created project receives an actionable savings recommendation on day
zero instead of waiting three days, which was the moment when over-provisioning turned into an
invoice. In exchange, the platform maintains an ownerless global table: it is the only one in Sytue Cloud
that is read without a tenant predicate, and that is why its write policy is maintenance-only and its
content is bounded by `CHECK`. The recomputation goes through all tenants and is the most
expensive query of the reconciler; it is spaced six hours apart and uses a seven-day window so that the
weekend does not make the recommendation oscillate.

**Alternatives ruled out.** Publishing the aggregate without a threshold (a cohort of two customers IS a
customer); computing the cohort on the fly on each request (it would require reading raw rows from other
tenants on the user's read path, which is exactly what RLS prevents); asking for
explicit consent to participate (the result would be a sample biased towards those who are already
optimized, that is, a benchmark that worsens the advice for those who need it most).

<a id="0029"></a>

<a id="0029--apps-open-source-como-servicio-catálogo-cerrado-no-imagen-libre"></a>

## 0029 — Open-source apps as a service: closed catalog, not any image

- **Date:** 2026-08-05
- **Status:** accepted
- **Context:** ADR 0022 (AGPL engines out), ADR 0023 (Kapsule), `internal/domain/appcatalog.go`

**Context.** Sytue Cloud's promise is composition: bring up a cache, a search engine or a queue
next to your app, one at a time, and have them connect on their own. The obvious way to implement it —“tell me the
name of an image and I'll start it”— turns the control plane into an **executor of arbitrary
third-party code inside the cluster**, and that cannot be fixed afterwards with network policies: the
container is already running, already has the project's variables and is already inside the customer's
namespace.

**Decision.** The customer **chooses from the catalog, does not write an image**. The catalog is code
—`services/cloud/internal/domain/apps/*.json`, embedded and reviewed— and each entry satisfies four
invariants that are checked in a test, not by word of mouth:

1. **Digest, never tag.** `valkey:8` points to one content today and another tomorrow, so what
   was reviewed is not what runs. A `@sha256:…` is the only identifier that does not change under
   our feet, and it is what allows signing and auditing.
2. **Our registry.** All images are replicated to the EU registry. Depending on Docker
   Hub on the provisioning path means depending on its rate limit for a customer to be able to
   create a service.
3. **License checked, by allowlist.** Apache-2.0, BSD-2/3, ISC, MIT, MPL-2.0 and
   PostgreSQL. AGPL was out for the same reason as in [0022](#0022) —**revised by
   [0032](#0032)**, which admits it along with GPL/LGPL for images that run unmodified—; and
   so were the “source available” licenses that are advertised as open source and are not —SSPL, BUSL,
   Elastic v2, Sustainable Use—, because **they forbid exactly this**: offering them as a managed
   service, and that stays the same. It is an allowlist and not a denylist because those licenses appear faster
   than a list of forbidden ones would be updated.
4. **No privileges possible.** The manifest has no field for `privileged`, `hostPath`,
   `hostNetwork` or capabilities. It is not that they are validated: they cannot be expressed.

Integration between services —the “they connect even if you create them one at a time”— is solved by
`exports`: each entry declares which variables it injects into the customer's app (`VALKEY_URL`,
`MEILI_URL`…). Its value is a template with **two** substitutions, `{host}` and `{port}`, and no
more. It is deliberately not a language: that value ends up inside the customer's app, and every
capability added —reading another service, composing a credential, looking at the environment— would be
a new way for a catalog entry to leak something that does not belong to it.

CPU, memory and disk limits are **mandatory** in each entry and are capped. A
service without a ceiling eats the project's quota and brings down the app it came to help.

**Consequences.** You cannot deploy “anything from Docker Hub”, and that is a real limitation
compared with Coolify or Dokku. In exchange, everything that runs in the cluster is reviewed,
pinned and audited, and the customer does not have to understand licenses to stay out of legal
trouble. Expanding the catalog is a pull request with a replicated image: cheap, but reviewed.

A written entry whose image **is not yet replicated** is loaded, validated and published as
`pendiente de espejo` (“pending mirror”) —it is not offered and not hidden either—: offering it would fail when pulling the
image, after having promised the service to the customer; hiding it would mean nobody would ever replicate
the image because nobody would miss it.

**The registry is not written in the catalog.** Each entry stores the digest of our copy
(`mirrored_digest`) and NOTHING about the host: the reference is composed by `ImageRef()` with the deployment's
`REGISTRY_URL`. Writing it in the manifest left it pointing to a namespace that in production is
named differently —the PaaS one is `sytue-<env>-paas`, not the dev default—, and the failure did not show up
until a customer tried to create the service. Empty = pending mirror.

**Mirroring is done by CI, not a laptop.** `.sygit/workflows/mirror-catalog.yml` (and its GitHub
twin) replicates whatever is missing and records the digest in the manifest. It is triggered when the catalog changes,
not with the release: adding an app has no reason to wait for the next version of the suite. The
reason it is not manual is concrete: pushing requires a credential with
`ContainerRegistryFullAccess`, and the alternative was handing out a production write key
among laptops for a task done three times a year.

**The mirror is a command, and it verifies what it writes.** `scripts/mirror-catalog-images.sh` copies
with `crane` —never `docker pull` + `push`, which rebuilds the manifest, uploads a single
architecture and therefore CHANGES the digest—, checks that the digest was preserved and only then
pins it in the manifest. That way “the catalog says replicated” and “it is replicated” are the same thing. Each
entry also stores `upstream` and `upstream_digest`, so that the mirror can be audited and
redone without guessing anything.

**What the first batch taught us, and why the list came out shorter.** When resolving the images
for real —versions, digests, user and volumes read from the registry, not from memory— two
candidates fell off on their own:

- **OpenSearch** requires `vm.max_map_count=262144`, a NODE sysctl. PodSecurity `restricted` does not
  allow requesting it from the pod, and setting it on all nodes for one tenant's service is not
  a decision that is theirs to make. Meilisearch covers search without any of that.
- **Ghost** needs MySQL to work in production, so as a standalone service it does not start:
  it would come in when the catalog knows how to compose an entry with its database.

And a third lesson stayed in the manifest: four of the five images **do not declare a user**
—they run as root— and under `restricted` they would not start. That is why `run_as_user` is mandatory, and
that is why its value is read from the image's `/etc/passwd` instead of being assumed: RabbitMQ's is **100**,
not the 999 the convention suggested. `writable_paths` was born from the same thing: RabbitMQ's startup
generates its configuration in `/etc/rabbitmq`, and with a read-only root it would die.

**Alternatives ruled out.** Any image with prior scanning (scanning says nothing about what
the container DOES at runtime); any image in an isolated namespace without access to the
project's services (then it composes nothing, which was the goal); a catalog with a license
denylist (always arrives late).

---

<a id="0030"></a>

<a id="0030--firma-digital-qué-se-promete-bajo-eidas-y-de-dónde-sale-la-clave"></a>

## 0030 — Digital signatures: what is promised under eIDAS and where the key comes from

- **Date:** 2026-08-08
- **Status:** accepted · **3 owner decisions open** (see the end)
- **Context:** [`roadmap.md#3`](./roadmap.md) «Transversal — FALTA: firma digital» (“Cross-cutting — MISSING: digital signatures”); wave **P3** of
  Sytue PDF (“signing with a certificate (PKI)”, what separates the Pro edition from the basic one);
  ADR [0001](#0001) (EU-only), [0017](#0017)/[0018](#0018) (clean-room, proprietary),
  [0023](#0023) (no third-party engines), [0024](#0024) (desktop apps);
  `services/mail/internal/smime`, `platform/pkg/cms`, `platform/pkg/keycrypto`

**Context.** “Digital signature” is a checkbox in the roadmap, and underneath it there are **three
different products with different legal consequences**. Confusing them is the expensive mistake: it costs nothing
to put up a button that says “sign”, and it costs a lot for a customer to use it believing it has the value
of a handwritten signature and discover in court that it does not. The product is EU-only, so what
decides what is valid is **eIDAS** (EU Regulation 910/2014, revised by 2024/1183), and the level that
is promised changes the whole design: the signature, the storage and even who may hold the key.

Moreover, this arrives in a repo where **signing already happens**: `services/mail/internal/smime` builds a
CMS SignedData by hand with `encoding/asn1` since August, and it is verified in its tests with `openssl`.
So the question “can it be done without dependencies?” is not open for half of the problem — it is already
answered by working code.

<a id="1-qué-se-promete-firma-avanzada-ades-y-se-dice-con-esas-palabras"></a>

### 1. What is promised: **advanced** signatures (AdES), and it is said in those words

eIDAS defines three levels, and only the third is equivalent to a handwritten signature:

| Level               | What it requires                                                                                                                                                                                      | Where it is in the product                    |
| ------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------- |
| **Simple (SES)**    | Nothing more than being electronic. Art. 25.1: it cannot be denied legal effect _for being_ electronic, but whoever invokes it bears the burden of proving it belongs to the signer                   | The **drawn** signature already planned in P3 |
| **Advanced (AdES)** | Art. 26: uniquely linked to the signer, capable of identifying them, created using data the signer uses **under their sole control**, and detects any subsequent change to the document               | **What Sytue promises**                       |
| **Qualified (QES)** | AdES + a **qualified certificate** from a provider on the EU trusted list + **QSCD** (qualified signature creation device). Art. 25.2: effect equivalent to a handwritten signature throughout the EU | **Not promised**: it does not depend on us    |

**Decision: AdES is promised**, and the interface **names the level on every signature**. QES stays out
of scope until there is a contract with a qualified provider, because it is not a matter of
code: it requires an accredited and audited third party, and there is no way to manufacture it in-house.

From this comes the hard rule, which applies to the UI and to sales material: **we never say “legal
signature” or “valid signature” on its own.** We say what level it is, who issued the certificate, and against
which anchors it was checked. A green “✓ Signed” on a signature the recipient cannot verify is
a lie with style.

<a id="2-de-dónde-sale-la-clave-no-la-custodiamos"></a>

### 2. Where the key comes from: **we do not hold it**

Three possible sources, and the order matters because the first is the one that unblocks P3:

- **(a) The user's own certificate, in transit and never at rest.** They upload their PKCS#12 at the
  moment of signing, the key lives in memory during the operation and is destroyed when it finishes. There
  is no column, no backup, no rotation: there is nothing to rotate.
- **(b) Remote qualified provider (QTSP) with cloud signing.** The key is born and dies inside
  the provider's QSCD; we send the digest and receive the signature. It is the **only** path
  to QES and the only one that also does not make us custodians of anything.
- **(c) Card or national eID (DNIe and equivalents).** Requires PKCS#11 middleware on the
  user's machine: the browser cannot reach it. Its natural home is the **Fyne desktop app** of
  [ADR-0024](#0024), not the web. Noted, not planned.

**And what is NOT done, which is the important part: Sytue does not store a person's private signing
key.** Not even encrypted with `platform/pkg/keycrypto`, which technically we could do
this afternoon. Three reasons, said out loud:

1. **It breaks art. 26.c.** “Sole control” means that the signer, and only the signer, can sign.
   If we have the key, we can sign without them; and that is literally the first question
   an expert witness asks when someone repudiates a signature. An AdES held by the provider is
   weaker than a real AdES, and the difference is not fixed with a legal notice.
2. **It is the most valuable target the platform would have.** A store with the signing keys of
   thousands of organizations is not remedied by rotating a secret when it leaks: it is remedied by
   **repudiating signatures already issued**, retroactively, and without knowing which ones.
3. **Storing someone's private key is a liability, not a feature.** It is
   always presented as convenience (“sign with one click, without uploading anything each time”) and that convenience
   is real; what is being bought with it is a risk that cannot be undone.

> **Why the S/MIME CA is not a precedent.** `services/mail/internal/smime/ca.go` does hold
> keys, and that is fine: they are keys that **we issue** for an organization's mailbox identity,
> with our own anchor, and they are never presented as the advanced signature of a natural
> person. There the alternative was _not signing the email at all_. Here the alternative is a
> signature the recipient rejects, which is worse than none.

<a id="3-cero-dependencias-nuevas-qué-sale-con-la-stdlib-y-qué-no"></a>

### 3. Zero new dependencies: what the stdlib can do and what it cannot

Determined piece by piece, not from memory:

| Piece                                      | Stdlib only?                    | Evidence                                                                                                                                         |
| ------------------------------------------ | ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------ |
| **CMS/PKCS#7 SignedData packaging**        | **Yes**                         | `platform/pkg/cms` (delivered with this ADR): `encoding/asn1` + `crypto/x509`. Cross-checked with `openssl cms` in **both directions**           |
| **Verifying someone else's signature**     | **Yes**                         | same: digest, signed attributes, `signing-certificate-v2` and chain                                                                              |
| Certificate chain, expiry and usages       | **Yes**                         | `x509.Certificate.Verify`                                                                                                                        |
| Revocation via CRL                         | **Yes**                         | `x509.ParseRevocationList`                                                                                                                       |
| Revocation via OCSP                        | **Yes, no new dependency**      | `golang.org/x/crypto/ocsp`, already direct in `go.mod`                                                                                           |
| RFC 3161 timestamp request and validation  | **Yes (the code)**              | ASN.1 over HTTP; the token **is** a CMS SignedData that `platform/pkg/cms` already verifies. What is missing is not code, it is the TSA — §4     |
| Embedding the signature in the PDF (PAdES) | **Yes**                         | incremental update + `/ByteRange`; the `services/docs` engine already reads (`pdfread.go`) and writes (`export_pdf.go`) PDF without dependencies |
| **Reading a real PKCS#12**                 | **NO**                          | see below                                                                                                                                        |
| **Qualified signature (QES)**              | **NO, and not because of code** | requires QTSP + QSCD                                                                                                                             |

**The only technical gap is PKCS#12, and it is not the one it seems.** The comfortable answer —“done,
`golang.org/x/crypto/pkcs12` comes with a dependency we already have”— is **false**, and this was checked
instead of assumed. That package is **frozen** (its own documentation says it does not accept
new functionality and refers to another one) and it only understands 1990s encryption:

- A `.p12` exported by **OpenSSL 3 with its default values** fails with
  `pkcs12: unknown digest algorithm: 2.16.840.1.101.3.4.2.1` — it does not even accept the MAC being
  SHA-256, let alone PBES2/AES.
- And one generated with `-legacy` **but with the issuer's chain inside** (which is what
  any CA delivers) fails with `pkcs12: expected exactly two safe bags in the PFX PDU`.

That is: the file a real user has on their desktop today cannot be read. Possible ways out,
all three honest:

1. **`software.sslmate.com/src/go-pkcs12`** (BSD-3, permissive, within the allowlist of
   `security.md §5`). It is the one recommended by the frozen stdlib package's own documentation.
2. **Writing it in-house.** PBES2 is **PBKDF2 + AES-CBC**, and both are in the stdlib
   (`crypto/pbkdf2` since Go 1.24, `crypto/aes`): it would not be inventing cryptography, it would be parsing an
   envelope. But it is several hundred lines of ASN.1 over a hostile format, and all that parsing surface
   is fed by **a file uploaded by a stranger**.
3. **Never reading PKCS#12**: the key source is (b) or (c) from §2, and we depend on the QTSP sooner.

**Recommendation: option 1**, limited to an isolated import function. But the dependency
decision belongs to the owner, not to whoever writes the ADR — and **until it is made, P3 advances
entirely except for importing your own certificate**: verifying, packaging, embedding and signing
from (b) or (c) do not depend on this.

<a id="4-sello-de-tiempo-sin-él-la-firma-caduca-con-el-certificado"></a>

### 4. Timestamp: without it the signature expires with the certificate

A signature can be verified while the certificate is valid and it can be checked that it was not
revoked. The day the certificate expires —two or three years— a verifier can **no longer
distinguish “it was signed while the certificate was valid” from “it was signed afterwards”**, and the signature stops
holding up anything. A contract signed today has to remain verifiable in 2040.

What fixes it are two things, and neither is optional in the long term:

- **An RFC 3161 timestamp** from a third party, which is what fixes the _when_ in a way that does not depend
  on the signer's clock or ours.
- **Embedded validation data** (OCSP/CRL inside the document, the PAdES DSS): they allow
  checking revocation ten years from now, when the issuer's OCSP server has long been
  switched off.

**Target profile: PAdES-B-LT** (signature + timestamp + validation data). PAdES-B-LTA —the archive
timestamp that is re-stamped periodically— is left for when there are documents that truly live
for decades; adding it later is one more layer, not a rewrite.

**The RFC 3161 client is DELIVERED** (2026-08-14, `services/docs/internal/domain/pdftimestamp.go`):
request, `PKIStatus` validation, nonce against replays, `certReq` so the token stands on its own
inside the file, and the token attached to the signature as an **unsigned** attribute —the only place
where it fits, because the timestamp is computed OVER the signature and does not exist until the signature is done—.
The chart always passes `SYTUE_DOCS_SELLO_URL` and the credential goes through a Secret.

> **Revised by [0051](#0051) (2026-09-26):** the authority is now **Izenpe**, qualified
> (free Certum is not), the token is authenticated against its root, and in two-step signing an
> authority failure does NOT silently sign without a timestamp: the user is asked. Key source (c) from §2
> —card, DNIe and the system stores— is developed by the same ADR: the two-step service
> is delivered and each platform's stores arrive one by one.

**The chosen TSA (2026-08-14): CERTUM**, `http://time.certum.pl` — Asseco Data Systems S.A.,
Poland. It is the **only free timestamping service operated by a provider that is on the EU
trusted list**, so it meets the sovereignty invariant ([ADR-0001](#0001)) at no cost and without a
contract, and its root is in the usual stores: a validator shows a recognized timestamp and not
an “unknown issuer”. Verified against the real service, and also against Izenpe
(`tsa.izenpe.com`) and QuoVadis/DigiCert Europe (`ts.quovadisglobal.com/eu`), which remain as
alternatives — the first is the Basque public sector and its free service is not intended for commercial
use; the second has a US parent company, which is what rules it out compared with Certum.

**The fine print, and it is not minor: the free service does NOT issue QUALIFIED timestamps.** Free
ones never are — a qualified timestamp (eIDAS art. 42) is a paid service. What is gained
anyway is the TECHNICAL part, which is most of the problem: the signature keeps verifying when the
certificate expires. What is NOT gained is the legal presumption of accuracy of the date (art. 41.2);
it remains admissible evidence, but without that presumption. **Upgrading to qualified means changing the URL to
the paid service of the SAME provider: not a single line of code.** That turns what was a
blocking decision into a decision of when.

**And an operational consequence that had to be solved: TSAs speak PLAIN HTTP.** All of them — none
of the three offers HTTPS, and it is defensible, because a hash travels outbound and something signed by
the authority comes back. What an intermediary can do is **replace the token with another authentic one for
ANOTHER digest**, and against that there is `comprobarToken`: it validates that the token's TSTInfo stamps the
requested digest and carries this request's nonce. Without that check the PDF would be saved with a
timestamp that does not cover its signature, without any error, and it would be discovered the day the
evidence was needed. The chart opens port 80 only for this, with the reason written next to it.

**And a design decision that matters more than it seems: a TSA failure does NOT prevent signing.** If
the authority is down or takes longer than the timeout, the signature is issued WITHOUT a timestamp. The opposite would turn
a third party into a single point of failure for the product: nobody could sign a contract because a
foreign server is not responding. It has its own check.

**The rule while there is no TSA** —and this is what avoids the “green ✓” problem—: a signature without a
timestamp is issued, stored and **shown saying it does not have one**: «sin sello de tiempo:
verificable hasta _(fecha de caducidad del certificado)_» (“no timestamp: verifiable until _(certificate expiry date)_”). A clock that does not exist is not drawn. Along
the same line, `signing-time` is **what the signer declares**, not proof, and that is how it is
commented in the code that reads it.

<a id="5-dónde-vive-cada-cosa-y-por-qué-así-no-bloquea-nada"></a>

### 5. Where each thing lives, and why this way nothing is blocked

- **`platform/pkg/cms`** (new, delivered with this ADR) is the common envelope: an S/MIME signature,
  a signature inside a PDF and an RFC 3161 timestamp are **all three** a CMS SignedData. What changes
  between them is where the result is attached, not what is signed.
- **PAdES embedding goes in `services/docs`**, which is where the PDF engine is.
- **The key source plugs in as a `crypto.Signer`** —the stdlib interface, not one of
  ours—, and that is the piece that keeps §2 from blocking §3: an ephemeral in-memory key, an
  HSM via PKCS#11 and a remote QTSP are all three a `crypto.Signer`. It is decided later without
  rewriting anything.
- `services/mail/internal/smime` keeps its own CMS until it migrates, with the same precedent
  as `platform/pkg/keycrypto` versus `services/id`. It is not touched from here.

A design consequence that deserves its own name: **`cms.Verify` refuses to verify unless it is
told which anchors it trusts** (`ErrSinAnclas`). The system root store is **not** the
EU trusted list, and letting the function fall back to it would be making the trust decision
silently, with whatever the container image happened to bring.

**Consequences.**

- **Verification is no longer blocked today.** Opening a PDF signed by a customer and saying
  who signed it, with which certificate and against which anchor, does not depend on any of the three
  open decisions. It is half the value of P3 and it is done.
- P3 can deliver the drawn signature (SES) and verification of others' signatures immediately, and
  AdES as soon as there is a key source.
- **QES is not advertised in any sales material** until there is a contract with a QTSP.
- Sytue **does not offer “store my certificate”** as a feature. If a customer asks for it, the
  answer is (b), not a checkbox.

**Alternatives ruled out.**

- **Holding the user's key encrypted with `keycrypto`.** It was the comfortable and cheap option: sign with
  one click, without uploading anything each time, and the encryption-at-rest machinery already exists. It breaks the
  sole control of art. 26.c, turns the platform into the most valuable target it would have,
  and its failure is not remedied by rotating a secret but by repudiating signatures already issued.
- **Issuing the signing certificates ourselves with our own CA**, as is done for S/MIME.
  Ruled out for documents: a CA of ours is not on any trusted list, so the
  signature would only be valid in-house — and what P3 sells is exactly that it is valid **outside**.
- **Bringing in a PDF library that already signs** (PDFBox, iText, PDF.js…). AGPL or a commercial license,
  against [0017](#0017) and [0022](#0022), and against the fact that our own engine already reads and writes.
- **Implementing PBES2 inside `platform/pkg/cms` without asking**, to plug the PKCS#12 gap
  and be able to show the function finished. Ruled out by the invariant: the dependency decision
  belongs to the owner, and hiding it inside another package is precisely the way to make sure nobody
  reviews it. **The gap is documented, not filled.**
- **Reselling a non-qualified third-party signing service** of the kind advertised as “legal
  signature” without being on the trusted list. Ruled out: it is exactly the confusion this ADR
  exists to avoid, with our brand on top.
- **Waiting for a QTSP to start.** Ruled out: packaging and verification do not depend
  on that decision, and leaving them on hold would have kept “digital signature — MISSING” another quarter for
  a reason that was not technical.

**What the owner has to decide.**

1. **PKCS#12** — (i) add `software.sslmate.com/src/go-pkcs12` (BSD-3), (ii) write the PBES2
   parsing in-house on top of `crypto/pbkdf2` + `crypto/aes`, or (iii) never read P12 and depend on the
   QTSP. **Recommendation: (i).** It only blocks importing your own certificate.
2. **Qualified provider (QTSP/TSA)** — which one, from the EU trusted list and hosted in the
   EU, for timestamps; and if QES is also wanted, for remote signing. Contract and cost per
   timestamp.
3. **Trusted list** — whether the European LOTL is ingested (signed XML, with its maintenance) or
   each deployment declares its anchors. It determines what can be said to a user who opens a
   PDF signed outside Sytue.

---

<a id="0031"></a>

<a id="0031--crossplane-adopta-la-infraestructura-por-riesgo-rdb-y-kapsule-se-quedan-fuera"></a>

## 0031 — Crossplane adopts infrastructure by risk; RDB and Kapsule stay out

`accepted` · 2026-08-10 · infra, iac, crossplane

**Context.** [0014](#0014) put the production infrastructure in Terraform/OpenTofu.
It works, but the state lives in an S3 bucket and the deployment depends on it: `02b-secretos.sh`
and `03-desplegar.sh` read **8 values** with `tofu output` (the RDB endpoint, the two
Registry endpoints, the ingress IP, the LiveKit media IP, the private CIDR, the
S3 endpoint and the kubeconfig). Without a credential for the state bucket you cannot deploy —hence
the recovery mode `SYTUE_INFRA_FROM_ENV=1`—, and a `tofu init` in `environments/prod`
takes more than 30 minutes because it drags in a 1.4 GB AWS provider that no resource uses.

Crossplane puts reconciliation inside the cluster and eliminates the external state. The question
was not whether to migrate, but **what** to migrate and **in what order**, with all resources already serving
traffic.

**Decision.**

1. **Our own provider is generated with Upjet** from the Scaleway Terraform provider
   (2.80.0, the same one pinned by the OpenTofu lock), in **no-fork** mode: resources are
   reconciled with the Go schema inside the process, without running Terraform or bundling its
   binary. That way **BUSL does not enter the supply chain**.
2. **Resources are generated in order of risk if recreated**, not of convenience:
   DNS → Object Storage and Registry → network/IP/machines/secrets. 5 of 22 types are done.
3. **RDB (`scaleway_rdb_*`) and Kapsule (`scaleway_k8s_*`) stay out**, and not as “for
   later”: as a decision. Crossplane runs **on** Kapsule, so managing the cluster
   from inside is circular —a reconciliation failure can leave the pool without nodes and the
   controller that would fix it is on those nodes— and the `kapsule_kubeconfig` is precisely
   the credential to get there. And `scaleway_rdb_instance` is not a resource that “gets recreated”:
   it takes the data with it. There is no operational benefit that compensates for that risk.
4. **Adoption is always in two stages**: `managementPolicies: ["Observe"]` first,
   check that `spec.forProvider` matches `status.atProvider`, `tofu state rm` afterwards,
   and only then grant `Create`/`Update`. **`Delete` is not granted** on buckets with
   customer data. The full procedure, with what to look at before and after each step,
   is in [`infra/crossplane/ADOPCION.md`](../infra/crossplane/ADOPCION.md).
5. **The `output`s are replaced by a ConfigMap** `sytue-infra-salidas` published by
   `infra/crossplane/publicar-salidas.sh` reading the `status.atProvider`, **with the same
   key names** as the OpenTofu outputs so that migrating a consumer means changing where it
   reads from, not what format it reads.

**Alternatives ruled out.**

- **Migrating everything at once.** The Scaleway provider has hundreds of resources; generating them
  all yields a module nobody reviews, and adopting 55 blocks at once means the first
  external-name error is discovered when several resources have already been recreated.
- **Using a third party's Scaleway Crossplane provider.** There is no maintained one with the
  coverage needed, and a third-party provider on the provisioning path is one more
  actor with administrative credentials for the project.
- **Upjet CLI mode** (each reconciliation launches a Terraform binary). It puts Terraform and
  the Scaleway provider **binaries** into the image, with BUSL behind them, to do the same thing.
- **Installing `crossplane-contrib/provider-kubernetes`** and publishing the outputs with an `Object`
  and `spec.references[].patchesFrom`, which would keep them up to date on their own. It is the textbook solution,
  but it costs another provider in the cluster and in the supply chain to refresh values
  that are only read at deploy time. It will be reconsidered the day something inside the cluster
  needs them live.
- **Also adopting the `tfstate` bucket.** It is OpenTofu's own state: as long as OpenTofu
  manages something, the dependency is circular.

**Consequences.**

- **OpenTofu does not disappear.** Even if everything adoptable were adopted, `environments/prod` would still
  exist for RDB and Kapsule, and the deployment would still take the kubeconfig from its state.
  The goal is not “Crossplane replaces OpenTofu”, it is **“OpenTofu only touches what cannot
  be recreated”**.
- **The `plan` is lost.** OpenTofu shows what is going to happen before it happens; Crossplane does not.
  The Observe phase is the substitute, and it is manual: it does not scale to dozens of objects. It is the
  acknowledged cost of this decision.
- **Two definitions of the same infrastructure for as long as the migration lasts**, with the same
  risk of divergence that [0014](#0014) already pointed out for Compose and Helm. It is bounded by the
  rule that a resource leaves the `.tf` (a `removed` block with `destroy = false`) in the same
  change in which it enters Crossplane.
- **Today Crossplane manages nothing in production.** What exists is the generation pipeline,
  five resource types, manifests in Observe and the procedure. Nothing applied.

<a id="0032--crossplane-al-100--no-es-viable-y-al-máximo-posible-sale-caro-en-cadena-de-suministro"></a>

## 0032 — Crossplane at 100 % is not viable, and at the maximum possible it is expensive in supply chain

**Date**: 2026-08-10 · **Status**: accepted · **Partially replaces**: 0031

<a id="el-contexto-que-faltaba"></a>

### The missing context

ADR 0031 decided to adopt the infrastructure by risk and leave RDB and Kapsule
out. That decision still stands, but it was made without measuring what Crossplane ADDS
to the stack. With the criterion of keeping the supply surface as small as
possible, that number outweighs the others.

<a id="lo-medido"></a>

### What was measured

**Crossplane with Upjet does not replace Terraform: it bundles it.** The generated
provider declares `github.com/scaleway/terraform-provider-scaleway/v2 v2.80.0` and
`github.com/hashicorp/terraform-plugin-sdk/v2` as DIRECT dependencies. What
today is a binary that runs and exits would become a library running
inside a pod, always on, with permissions over the whole Scaleway account.

|                                         | Modules in the tree |
| --------------------------------------- | ------------------- |
| The whole Sytue suite (twelve services) | 526                 |
| **The Crossplane provider, alone**      | **483**             |
| Of those, NEW to our stack              | **286**             |

Doubling the audited supply surface to manage nineteen infrastructure
resources is not a maintenance improvement, it is the opposite. And the 286
new ones are not niche code: 29 HashiCorp modules, 21 from go-openapi, **14 from
AWS** —in an infrastructure that does not have a single AWS resource—, 7 from sigstore
and 5 from etcd.

The resulting binary is 99 MB. `tofu` is a binary that does not live in the cluster and
only downloads providers when it runs.

**And there is a permanent cost**: Crossplane requires its core (`crossplane` +
`rbac-manager`) and the provider pod always running, plus ~3 CRDs per resource
type —15 with five types, so the 22 would be about 66—. In a two-node
cluster with a budget of 100 EUR/month, those are pods and memory that are not spent
today.

<a id="la-decisión"></a>

### The decision

**Neither 100 % nor the maximum possible is pursued.** Crossplane stays as it is:
DNS, Object Storage and Registry generated, with the adoption procedure written
(`infra/crossplane/ADOPCION.md`) and not installed in production, available the day
there is a reason that outweighs this cost.

**OpenTofu remains the infrastructure tool.** It is a binary, with
nothing running, with three providers pinned by lockfile and verified by hash.

<a id="lo-que-sí-hay-que-hacer-y-es-la-mitad-del-beneficio-por-una-centésima-del-coste"></a>

### What does need to be done, and it is half the benefit for a hundredth of the cost

The original reason to migrate was that `tofu init` takes **more than 30 minutes** and
blocks every deployment. That is NOT caused by OpenTofu: it is caused by a 1.4 GB AWS provider
declared in `environments/prod/providers.tf` to read already-retired modules from the state,
**without a single `aws_*` resource in the repo**. The file's own note
says it should be removed “after that apply and checking `tofu providers`”.
Nobody did it.

Removing it, together with the shared plugin cache, recovers almost all the lost
time. It is an afternoon of work versus the weeks of a migration that would also
double the supply surface.

**Done the same day, and measured:**

|                                | Before  | After     |
| ------------------------------ | ------- | --------- |
| Clean `tofu init`              | >30 min | **2.7 s** |
| Providers in the prod lockfile | 3       | 1         |
| `.terraform` on disk           | 3.7 GB  | 204 MB    |

The remote state no longer references them —from 37 resources with 3 `aws` and 2
`random` (serial 27) to 34, all Scaleway (serial 50)—, so the declaration
only served to download. `tofu validate` passes with the pruned tree. The other
three environments (dev, staging, sytue-git) never had the dead weight.

Two things were added so it does not come back:

- **A gate**, in `deploy/production/tests/deriva-infra-test.sh`: it fails if the
  lockfile of any environment pins a provider without a single resource of its own in the
  code. It looks at the LOCKFILE and not only at the `.tf`, because it is what governs what gets
  downloaded and an entry can survive there after the declaration is cleaned up.
- **A shared plugin cache** (`TF_PLUGIN_CACHE_DIR`) in
  `lib-tofu-backend.sh`. Isolation via `TF_DATA_DIR` is kept —it is
  correct and prevents a historical cache from claiming retired providers— but the
  binary is stored only once instead of once per directory.

While writing that gate a trap came up that had nothing to do with it and applies to the whole
repo: `«$var»` **kills the script**. The `»` is the bytes `C2 BB`; under the C locale
bash swallows them as part of the variable name, and with `set -u` that is an
`unbound variable`. The worst part is where it lives: in the error branches, that is, it only
blows up when the gate has just detected something, and it takes the diagnosis down
with it. There was another identical case in `runner-flota-test.sh`. **A variable stuck to
« »: always use braces.**

<a id="cuándo-reconsiderarlo"></a>

### When to reconsider it

- If the infrastructure comes to be defined by API instead of by files —customers
  creating resources on demand—, which is the use case Crossplane
  exists for and which we do not have today.
- If Scaleway publishes a NATIVE Crossplane provider, without Upjet or the Terraform
  SDK inside. Then most of the 286 modules disappear.
- If OpenTofu stops being maintained or changes its license.

None of the three depends on us, and that is why the work done is kept instead of
being deleted: the day it happens, the pipeline already works.

<a id="0032"></a>

<a id="0032--productos-open-source-en-sytue-cloud-copyleft-sí-source-available-no-y-quién-se-publica"></a>

## 0032 — Open-source products on Sytue Cloud: copyleft yes, “source available” no; and who gets published

- **Date:** 2026-08-12
- **Status:** accepted
- **Context:** ADR [0022](#0022) (AGPL engines out of the suite), ADR [0029](#0029) (closed
  catalog), `services/cloud/internal/domain/appcatalog.go`

**Context.** The 0029 catalog was born to bring up SUPPORTING pieces next to the customer's
app: a cache, a search engine, a queue. The next turn of the product asks for something else —choosing
SEVERAL complete products, like Nextcloud or a CMS, and having those that integrate with each other start already
connected—, and there 0029 clashed with itself on two points:

- Its allowlist was permissive-only, and **practically all free product software is
  GPL or AGPL**: Nextcloud, WordPress, Grafana, Metabase, Vaultwarden, Mastodon. With the list as
  it was, the catalog could never get beyond infrastructure.
- No entry had an Ingress, on purpose. Correct for a cache; for a CMS it means
  deploying it and not being able to open it.

**Decision 1 — AGPL, GPL and LGPL come in; “source available” stays out.**

[0022](#0022) removes the AGPL engines from INSIDE the suite, and that decision is not touched: there,
Sytue's proprietary code integrates with them, and that combination is what copyleft reaches.
Here the case is different: the image runs **as is, unmodified**, in the customer's namespace, without
anything of ours being linked with it. That is aggregation, not a derivative work.

And “unmodified” is not a statement of intent: it is what `validateAppMirror` already checked
—`mirrored_digest` has to be **identical** to `upstream_digest`—. A copy with one byte changed
does not satisfy that equality and does not enter the catalog, so the premise on which everything
above depends is verified by a machine on every load.

The obligation that does remain —AGPL §13 and GPL §6: whoever uses the service can obtain the source— is
met with a new field, `source`, **mandatory** when the license is copyleft and which the interface
shows next to the license. It is mandatory in validation because the alternative was trusting
that whoever adds an AGPL entry remembers.

“Source available” licenses (SSPL, BUSL, Elastic v2, Sustainable Use) stay out, and for the opposite
reason: it is not that their copyleft reaches us, it is that **they expressly forbid offering them as a
managed service**.

**Decision 2 — publishing to the internet is decided by the catalog (`web_port`), never by the customer.**

An entry declares which port it is published on, or does not declare it and then it is only reachable from
inside the project. With `web_port`, the provisioner creates Ingress + cert-manager TLS at
`<app>-<proyecto>.<dominio de apps>`.

A “publish” switch in the dashboard would have been more flexible and it is what Coolify does. It would also
have put the RabbitMQ admin panel on the internet —with the password the
platform has just generated— the day someone ticked the box without knowing that 15672 is a
panel. It is reviewed once per entry, when it is written, and then there is no click to get wrong.

Our own NetworkPolicy (`allow-published-apps`) was also needed: the existing one opens from Traefik
**only** ports 3000 and 8080, so a Nextcloud listening on 80 would have had an Ingress,
an issued certificate and a timeout, with nothing in the logs pointing to the network.

**Decision 3 — apps integrate with each other, and it works in any order.**

An entry declares `integrations`: which OTHER catalog app it connects to, **the URL of the
official documentation that describes it** and which variables it receives. `docs` is mandatory and it is what
separates an integration from a guess: without that anchor, “Sytue connects them on its own” would end up being
Sytue inventing the configuration of someone else's project, and the failure would show up when starting a
container at a customer's site.

The provisioner resolves the integration by LOOKING at what is running in the project (labels
`sytue.eu/app-id` + `sytue.eu/project-id`), not a stored relationship. The project filter is not
optional: the namespace belongs to the TENANT and several projects fit inside it, so without it one project's app
would connect to another's cache and neither party would notice.

And order does not matter, which is the part that was hard: `reconcileServices` **only** provisions what
is in `provisioning`, that is, a `ready` service is never looked at again. With nothing more,
creating the cache AFTER the CMS would never have reached the CMS —no error, no warning, no
connection—. That is why adding or deleting an app **re-enqueues** the sibling apps that integrate with it
(`ReprovisionServices`), and that is why only **one instance of each app per project** is allowed: with
two, “the project's cache” would stop being one and the integration would choose by listing order.

**Consequences.** The catalog can contain complete products, and the customer sees the license, the
source and whether it will have a URL before creating anything. In exchange: each new entry requires checking that its
image starts **non-root and under PodSecurity `restricted`**, and quite a few candidates fall there
—the official Nextcloud image starts as root to adjust permissions, so it will come in through another
image or not at all—. Adding an app is still a pull request with review, and now also with
a UID and port check.

---

<a id="0033"></a>

<a id="0033--sytue-pdf-se-dibuja-en-svg-no-se-rasteriza"></a>

## 0033 — Sytue PDF is drawn in SVG, not rasterized

`accepted` · 2026-08-13 · pdf, docs, frontend

**Context.** Sytue PDF needs a canvas: without seeing the document you cannot annotate,
fill in, or sign in the right place. The obvious way to get it is to rasterize each page to an
image, which is what almost every viewer does. With our invariants that means
writing an entire graphics engine —curves, patterns, antialiasing, compositing and, above
all, interpreting every embedded font format to draw its outlines— because
PDF.js (AGPL), wkhtmltopdf or an external binary cannot be brought in.

**Decision.** Translate the PDF content stream to **SVG** on the server and let the
browser engine rasterize it. The correspondence is almost one to one: `m/l/c/re` are a
`<path>`, `cm` is a `transform` —the PDF matrix `[a b c d e f]` and SVG's `matrix(a b c d e f)`
are literally the same function—, `Do` on an image is an `<image>`, and `sh` is
a `<linearGradient>`. The SVG is inserted into the DOM, not into an `<img>`.

What an image does not give comes for free:

- **text is TEXT**: it can be selected, copied, and read by a screen reader. Highlighting is
  taking the rectangles of the browser's selection, which is what a desktop viewer
  does and which on a bitmap would have to be reinvented, worse;
- **zooming does not lose quality** nor require requesting the page again;
- **a single vector representation serves as the reference** for the six platforms of
  ADR-0024; the Fyne desktop renders it natively and the Capacitor mobile app shows it
  inside its web client;
- the annotation layer **shares the coordinate system** with the document.

**What had to be solved, and it was not obvious.** A composite font with
Identity-H encoding comes with an empty `cmap` table —the PDF never needs it, its codes are glyph
indices— so the browser finds nothing and draws little boxes. That table is **rebuilt**
by inverting /ToUnicode, and with that an InDesign or LaTeX document is shown with ITS typeface
and can also be copied. When there is no way —old Type1, standalone CFF— it is replaced with
a generic family and `textLength` keeps the line square.

**Ruled out.** **Rasterizing in Go** (the graphics engine and the font engine, tens of thousands
of lines, and the result is not selectable); **PDF.js** (AGPL, incompatible with the
permissive clean-room of `security.md#4`); **an external binary** (breaks the zero-dependency
invariant and adds an attack surface on the path of an outside file); **Canvas
on the client** (it would need the same interpreter but in JavaScript, and would lose text selection
just like an image).

**Cost / follow-up.** What SVG cannot express degrades to something visible and never to a silent
gap: tiling patterns are translated to `<pattern>`, mesh shadings are approximated
by their average color, and CCITT, JBIG2 and JPEG 2000 —which no browser decodes— leave a
gray box in their place. The SVG of a complex page weighs hundreds of kilobytes, so
it is served in batches and with a cap of 24 MiB per page. Our own CCITT decoder is the
clearest debt: it is the format of almost everything scanned, and today those pages do not show.

**One MORE output, the rasterized page (2026-09-22).** This decision is NOT undone: without
`formato`, or with `formato=svg`, `op: render` still delivers the SVG with `<text>`, and the web
still uses it to select, search and read aloud. What is added is
`formato=png` for the client that has no SVG engine with text behind it. That is the
Fyne desktop: all SVG goes through `oksvg`, which does not implement `<text>` —measured at 120×120,
a `<rect>` paints 10,000 pixels and a `<text>` paints 0—, so native CertiPDF showed the
paper and the lines of an invoice without a single figure.

- **It is not the engine that was ruled out here.** The interpreter, the colors, the typefaces, the
  images and the clipping already existed: the SAME traversal of the stream also paints into a
  bitmap (`pdfraster.go`, under a thousand lines) with `x/image/vector`, and the glyphs come from
  `x/image/font/sfnt` over the same typeface the SVG embeds. `x/image` was already
  in `go.mod`: zero new dependencies. In PNG mode the SVG is not built.
- **What the PNG does not do and the SVG does:** even-odd rule (`f*` comes out as non-zero), blend
  modes, tilings (not painted) and shadings (their average color). Standalone Type1 and CFF, which
  `sfnt` does not read, are drawn with the Go Fonts, each glyph in its place and with the PDF's advance.
- **Cost, measured on the front node** (ARM64, not the production machine, with load 7-68 on
  6 cores; process CPU, median of 5, two batches that agree; A4 at ×2 ≈ 1.9 Mpx;
  seven real documents; `pdfraster_medida_test.go`): drawing 13-183 ms, encoding the
  PNG 21-98 ms with `BestSpeed` (30-142 with the default compression, which was 40 to
  85 % of the page's CPU, in exchange for up to 3 % fewer bytes: not worth it), and
  the SVG of the same page 1-92 ms. 10-35 MiB are reserved per page (the SVG, 0.6-24),
  of which 7.4 are the RGBA bitmap itself. It weighs 45-710 KiB, on the order of the SVG
  (64-1,182 KiB).
- **Cache and size.** The server stores NOTHING: the promise of `/pdf/*` is that the file
  is processed in memory and returned (`no-store`), and a PNG of a contract is still the
  contract. The cache is the client's, by (document, page, scale). The client requests the
  EXACT scale of its screen (zoom × HiDPI factor) so as not to rescale, and only the window
  being viewed: batches of 8 at most, not the 20 of the SVG. Cap of 16 Mpx per page
  (64 MiB, an A4 at ×5.6), checked before allocating.
- **Fidelity**, against poppler as the yardstick (it is not used in the service): on pages
  with embedded TrueType or CID typefaces, between 0.3 and
  5.8 % of the pixels differ by more than 64 levels —the antialiasing of text edges—; with substituted Type1,
  10 %, and it is readable.
- **What came out while measuring, in the SHARED interpreter:** `Q` restored the state of the
  PREVIOUS `q` and not its own. With a matrix set before the `q` —Google Slides opens all
  its pages that way— everything after the first `Q` fell off the sheet, **also in the web
  SVG**. Fixed. And one remains unfixed: the JPEG 2000 decoder returns
  the colors swapped (green and magenta) in archive.org scans, in both
  outputs equally, and takes from 1.2 s (x86) to 15 s (the front node) and allocates 1.7 GiB per page.

---

<a id="0034"></a>

<a id="0034--ocr-tesseract-autoalojado-en-un-servicio-propio-la-capa-de-texto-la-escribe-sytue-pdf"></a>

## 0034 — OCR: self-hosted Tesseract in our own service; the text layer is written by Sytue PDF

`accepted` · 2026-08-13 · pdf, ocr, infra · **replaces the first draft from the same day**

**Context.** OCR is the last piece of wave P8 and the only one in all of Sytue PDF with no obvious
path in the standard library. Recognizing characters is a computer vision
problem: it is not “a few more tables”, it is a model.

**The observation that orders the decision:** “OCR” is TWO things that get confused because
they arrive together.

1. **Recognizing**: from pixels to characters with their position. That is the model.
2. **Writing the layer**: putting that text into the PDF as INVISIBLE text (render mode 3)
   exactly on top of where it is drawn, so that the document can be searched,
   selected and copied without changing a single pixel of what is seen.

Only the first one needs a model. **The second is PDF formatting and we already know how to do it.**

**Founder's decision (2026-08-13), which replaces the first draft of this ADR:** the
engine has to be **free software and self-hostable**. No calling an AI provider,
not even a sovereign one. With that, the business decision this ADR left open **disappears**: there
is no provider to choose and no per-page cost to negotiate.

**Decision.**

- **The engine is [Tesseract 5](https://github.com/tesseract-ocr/tesseract), Apache-2.0**,
  inside the image. A permissive license —better than what the founder asked for, which allowed
  GPL—, more than a hundred languages and the de facto standard for twenty years.
- **It lives in its own service, `services/ocr`** (Go, port 8096, ClusterIP **without
  ingress**). It is the Tesseract binary with an HTTP layer in front: an image goes in, words
  with their box come out. It knows nothing about PDF.
- **The text layer is written by Sytue Docs**, with the in-house engine and nothing new
  (`CapaDeTextoPDF` in `pdfocr.go`). Docs also does **the coordinate conversion**, which
  is what translates the recognizer's pixels to PDF space (`ImagenColocada.RectDePDF`).
- **The image that is recognized comes from the PDF ITSELF**, not from a rasterization. In a scanned
  document the page **is** an embedded image, so it is extracted with its placement
  matrix (`ImagenDeEscaneo`) and sent as is. No loss of quality, no round trip to the
  client and no rasterizer — which is what [ADR-0033](#0033) decided not to write.
- **The credential is the user's JWT**, which Docs forwards. No secret shared between
  services and no trust in the network: a recognition is attributed to whoever requested it.
- **The endpoint that WRITES the layer still accepts recognized text wherever it comes from.**
  It does not ask to know who recognized it, so a scan processed elsewhere can be incorporated.

**Why a service and not a library.** Linking `libtesseract` with cgo would put C into a
binary that today is static on distroless, and the ~100 MB of language data would travel in
**every Docs replica** for a feature used by one request in a thousand. Besides, OCR is
pure CPU in bursts: as a service it scales on its own, without dragging the editor along.

**The price, stated here so it is not discovered late.** It is the **only image in the suite that
is not distroless**: Tesseract needs a package manager that distroless does not have. This is
compensated with a `debian:bookworm-slim` base pinned by digest, `--no-install-recommends`, no
apt caches, a non-root process with the same uid 65532 as the rest,
`readOnlyRootFilesystem` and each page's temporary file in an **in-memory** `emptyDir` —so
that a scanned payslip never touches the node's disk—. The `NetworkPolicy` only
allows egress to Sytue ID (JWKS) and Cockpit: **if outbound traffic ever appeared
from this pod, it would be an incident, not a missing configuration.**

**Ruled out.**

- **An AI provider (even a sovereign one)**, which was the previous decision: the founder
  revoked it. It added a per-page cost, an external dependency and a pending business
  decision, in exchange for nothing that self-hosted Tesseract does not provide.
- **Rasterizing on the client and uploading PNGs**, which the first draft proposed: it works,
  but for a scan it is worse —the image is already inside the PDF— and it makes batch processing of
  a three-hundred-page case file unworkable.
- **Tesseract compiled to WASM in the browser** (tesseract.js, Apache-2.0). It is tempting
  because it adds no system and the document does not leave the device. It is ruled out because of
  batches and mobile: downloading ~15 MB of model and recognizing three hundred pages in a
  tab is not a product. It is noted as a path if a “serverless” mode is ever
  wanted.
- **Our own engine.** That is a research project, not a wave.
- **Sending the PDF to a third-party OCR service**: it is exactly what Sytue exists to
  avoid; that is where payslips end up.

**Consequences.** The tool is offered **only if there is a service behind it**: `/pdf/ocr`
returns `disponible` and the interface hides the button when it is false, with the same discipline
as the AI assistant — but unlike that one, here the chart **does** exist and `ocr.enabled`
defaults to `true`, so in a normal deployment it is on. The installed languages are
the five product ones (`spa`, `eng`, `fra`, `deu`, `ita`) plus `osd` to detect
orientation; requesting one that is not present is **rejected** instead of falling back to English, because recognizing
a Spanish document with the English model does not give an error: it gives plausible but wrong text that
nobody reviews. And a nuance worth remembering: a scanned PDF that ALREADY comes with an OCR layer
can be searched and copied in Sytue PDF without doing anything, because the render keeps the invisible text
instead of discarding it.

<a id="0035"></a>

<a id="0035--sytue-ai-dos-modelos-en-scaleway-tope-blando-por-usuario-y-ocr-por-modelo-de-visión"></a>

## 0035 — Sytue AI: two models on Scaleway, soft per-user cap and OCR via a vision model

`accepted` · 2026-09-09 · ai, infra, pdf, fotos · **revises the OCR engine of [0034](#0034)**

**Context.** `services/ai` had been written for months and not deployed: `POST /api/v1/assist`,
anti-injection defense, rate limiting and tests. What was missing was not code, it was **choosing a
provider**, and that also blocked the editor panel, which hides itself when the BFF does not
see `SYTUE_AI_URL`.

**What was measured, which is what orders everything else.** Against Scaleway Generative APIs
(fr-par), on 2026-09-09, with three repetitions per task and no output cap, on four
real suite tasks (Mail reply, Calcus formula, invoice extraction from
certiDocs, Notepads headline) plus the Photos development recipe:

| model                 | €/10,000  | latency   | formula          | development                    |
| --------------------- | --------- | --------- | ---------------- | ------------------------------ |
| mistral-small-3.2-24b | **€0.59** | **0.6 s** | wrong 3 of 3     | +1 stop (double)               |
| pixtral-12b           | €1.15     | 0.5 s     | wrong 3 of 3     | +2 stops (blows out the photo) |
| gpt-oss-120b          | €1.71     | 2.8 s     | right 3 of 3     | +1/+0.5/+1                     |
| gemma-4-26b           | €5.66     | 14.0 s    | **right 3 of 3** | **+0.5/+0.1/+0.3**             |

**Decision.**

- **Two models, and the mode is chosen by the caller** (`"modo":"calidad"`), not by the service
  guessing from the action: an inline panel knows its user is watching the cursor and
  a background job knows nobody is waiting. `mistral-small-3.2-24b` handles the
  interactive; `gemma-4-26b` what can wait and has to come out right.
- **`reasoning_effort: high` on the quality model**, which is counterintuitive and that is why it is
  written down: it comes out cheaper (€5.43/10,000), faster (8.1 s) and more accurate (9 of 9) than the
  unset default. `low` and `medium` ramble. And `none` is EIGHT times cheaper with the same
  accuracy **on text**, but it misses the exposure when analyzing a photo: that is why the effort
  can be requested per request and is not a global setting.
- **Soft cap of €1.50 per user per month on the expensive model.** When it is exhausted it is NOT cut off:
  the request is served with the fast model and the response says so (`degradado: true`). The
  counter lives in the shared cache, with the month inside the key so that it resets
  on its own. It is soft in three accepted ways: it is checked before and charged after —the
  request that crosses the cap is served in full—, a cache flush gives away the month, and without a cache
  there is no cap. It degrades OPEN on purpose: being left without an assistant because of an optional piece
  is worse than overspending, and the service warns about it on startup.
- **Whisper-large-v3 for audio** (€0.003/minute: a one-hour meeting, 18 cents) and
  **qwen3-embedding-8b for vectors** (€0.10/million). Neither goes through the budget: they are not
  billed per output token and their spending does not spike on its own.

**The OCR engine: this revises 0034, and it is worth saying clearly.** 0034 decided that the
engine “has to be free software and self-hostable. No calling an AI provider, not even a
sovereign one”. **The founder revised it on 2026-09-09**: OCR is now done with the vision
model —Gemma 4 while there is budget, Mistral Small afterwards—, with an explicit condition
for going back: **if the quality is not there, we return to Tesseract**. The `ocr` chart and the service
are NOT retired, precisely so that going back means changing who calls and not rebuilding anything.

What was measured the same day on a clean synthetic invoice: all three transcribe the text
in full and without errors —Gemma with `esfuerzo: none` in 0.8 s and 0.011 cents, Gemma with `high` in
2.3 s and 0.025, Mistral in 0.7 s and 0.006—. For OCR, `none` is right: half the price
and three times faster with identical output. **What that test does NOT cover** is the case that
truly decides: a skewed scan, with shadows or at low resolution. That is where Tesseract, which
has spent twenty years on that problem, may keep winning. The return condition in the previous
paragraph exists for that reason.

**What Scaleway does NOT have, and it must be known before promising anything.** There is no image
model at all: neither in Generative APIs (15 models) nor in Managed Inference (57). The task
labels are `chat, instruct, code, vision, reasoning, embeddings, audio_transcription`, and
`/v1/images/generations`, `/v1/images/edits` and `/v1/videos/generations` respond 404. `vision`
is INPUT image. That is why Sytue Photos' AI **does not edit pixels: it writes the recipe** that
`services/syphotos/internal/service/receta.go` already knows how to apply, with the same formulas in Go and
in the browser. Real editing —removing an object, filling in background— would require a
diffusion model on our own GPU, paid per hour switched on and not per use: that is another decision and it has not
been made.

**Consequences.**

- `ai` chart in the umbrella, with `global.ai.enabled` as the ONLY switch: both the service chart
  and the frontend chart look at it, and with one per subchart they could contradict each other.
- The provider key goes in a Secret with `optional: true`, and it is **the secret key alone, not
  the pair**: Scaleway issues API keys as access key + secret key, but Generative APIs only
  accepts the secret key as a Bearer —checked on 2026-09-09: with the access key it returns 403, and
  with both together too—. It is the convention the repo already follows for Scaleway's REST
  APIs. Without it the service starts
  healthy and responds 503 `ai_not_configured`, which is what the panel already knows how to read to hide itself.
  A mandatory secret would leave the pod in `CreateContainerConfigError`, which looks
  nothing like “AI needs to be configured”.
- The output cap of the quality model is four times that of the fast one and it is not slack: the
  reasoning tokens come out that way. With 1024, Gemma returns an **empty** response, which
  arrives as a 502 with no visible cause. That is why `finish_reason: "length"` has its own error
  (`ai_truncated`) and is checked BEFORE the empty case.

<a id="0036"></a>

<a id="0036--cloudflare-fuera-el-portal-entra-por-pangolin-y-el-dns-lo-sirve-scaleway"></a>

## 0036 — Cloudflare out: the portal comes in through Pangolin and DNS is served by Scaleway

`accepted` · 2026-09-10 · infra, dns, mail, certificates

**Context.** The `sytue.eu` portal came in through **Cloudflare Tunnel** and its DNS zone lived in
Cloudflare. Five things hung from there that do not resemble each other and that had to be moved at
the same time or not at all: web traffic ingress, the DNS-01 challenge for certificates, the DDNS that
chases the home IP, the guardian that restores the MX records and the wildcard issued by the front node.

**The decision.** No piece of the installation talks to Cloudflare anymore. This is what remains:

| What Cloudflare did               | Who does it now                                                                            |
| --------------------------------- | ------------------------------------------------------------------------------------------ |
| Ingress tunnel to the portal      | **Pangolin** (Traefik + WireGuard) on the media machine, with the `newt` agent on the node |
| Authoritative DNS zone            | **Scaleway DNS** (`ns0`/`ns1.dom.scw.cloud`)                                               |
| DDNS for the `mx` record          | `bin/ddns.sh`, against the Scaleway API                                                    |
| MX, SPF, DMARC and TLS-RPT        | `bin/mx-dns.sh`, against the Scaleway API                                                  |
| cert-manager DNS-01 challenge     | `acme.scaleway.com` webhook + `ClusterIssuer` with `ACME_SOLVER=dns01-scaleway`            |
| Front node `*.<dominio>` wildcard | Front node Traefik with `provider: scaleway`                                               |

**What was learned by moving it, which is what you do not see in a diagram.**

**The DNS provider is discovered, not configured.** A variable says what someone believed the
day they wrote it. On migration day, the DDNS kept updating the old zone every
five minutes **without a single error** while the right one stayed with the previous IP. Now
`ns_autoritativos()` asks with `dig +trace`, which descends from the root: no cache, no validation,
and not the opinion of a resolver that turns out to be authoritative for the zone just abandoned
—`1.1.1.1` kept answering the old NS records hours after the delegation changed—.

**The Scaleway DNS API has three traps, and all three answer `200`:**

1. the `name` is the **relative label** and the apex is the empty string; with the full name,
   queries return zero records as if they did not exist, and writes land on the
   zone apex;
2. a target **without a trailing dot is relative**: an MX pointing to `mx.sytue.eu` is stored as
   `mx.sytue.eu.sytue.eu.`, which does not resolve;
3. the **MX priority travels inside `data`** (`10 mx.sytue.eu.`) as well as in its own field, so
   comparing against the bare target makes it look as if there is a foreign MX.

None of the three gives an error. That is why the two scripts that write DNS **check what
the API returns**, and `mx-dns.sh` has `--dry`: a script that touches the mail DNS has to
be able to be rehearsed.

**A red that means nothing teaches you not to look at reds.** While a script did not know how to
write to the new provider, it left its timer red every fifteen minutes. We preferred that it
exit with 0 while warning, and that the one who really keeps watch be `bin/verify.sh` against public DNS
—which is what whoever delivers the mail sees—.

**Two details of the front node that were fine-tuned at the end (2026-09-10).** The
wildcard challenge asks for the TXT from **the zone's authoritative servers** —`ns0`/`ns1.dom.scw.cloud`—
and not from a public resolver: lego only uses them to see whether its own challenge is already
published, so asking at the source avoids waiting for other people's caches and does not
depend on third parties. And the container receives **`SCALEWAY_PROJECT_ID`** in addition to the
secret key: for lego it is optional —it only requires the key—, but the zone is looked up
within a project, and without it it goes to the credential's default one; the day that
key serves two projects, the challenge TXT would be published in the wrong one.

**What did NOT change, and not by oversight.** The portal's custom domain wizard
(`apps/frontend/apps/portal`) still offers Cloudflare: there the token is **the customer's**, for
their zone, and it has nothing to do with ours.

**When to reconsider it.** If the front node stops being Pangolin or the zone moves again, what
has to be reviewed is the whole table above: there are six pieces that depend on the first two
rows, and moving them one at a time is how you end up writing to a zone that is no longer in charge.

<a id="0037"></a>

<a id="0037--la-plomería-compartida-se-comparte-de-verdad-catálogo-guardianes-y-reglas-de-eficiencia"></a>

## 0037 — Shared plumbing is truly shared: catalog, guardians and efficiency rules

`accepted` · 2026-09-12 · code, maintenance, performance

**Context.** Every product in this suite was created by copying the previous one. It is the fastest
way to get started and the most expensive to maintain, because what is copied **diverges on its own** and does so
where nobody is looking. This is not a theoretical concern; everything below is measured in this repo:

| What was copied                                                                     | How it had diverged                                                                                           |
| ----------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| The compliance audit endpoint, in Mail and Drive                                    | ~490 identical lines except 36, and one validation answered **in Spanish in one and in English in the other** |
| The `middleware.ts` of twelve surfaces, with a “copy this file as is” in its header | When measured, only **five of twelve** were still byte-for-byte identical                                     |
| The access token getter, in three apps                                              | **Different expiry margins** (60 s versus 30 s): that is how an app sends already-expired tokens              |
| The seven `/auth/*` endpoints, in twelve apps                                       | 82 files and 927 lines; the logout GET written in two ways and the `/auth/token` JSON in another two          |

**The decision.** The plumbing lives once, with its written catalog and with tests that fail when
someone copies it again.

1. **Normative catalog** in [`CONVENTIONS.md` §13.2](CONVENTIONS.md#13-2-lo-que-ya-existe): the
   22 packages in `platform/pkg/` and the 10 in `apps/frontend/packages/`, each with what it is
   for. Before writing plumbing you look there; if it exists and does not fit, it is **extended**, not
   copied.
2. **Guardians that compare by equality**, not by membership
   (`plomeria-compartida.test.ts`, `rutas-auth-reexportan.test.ts`). That way they also fail when an
   exception is deduplicated and nobody deletes its entry: an allowlist nobody prunes stops
   protecting within two refactors. `rutas-auth-reexportan` already went red for exactly that reason, when
   unifying the `/auth/token` of Docs, Drive and Portal.
3. **Efficiency rules** in [§13.3](CONVENTIONS.md#13-3-eficiencia), each one born from a
   concrete failure, not from a general principle.

**What is NOT unified, which is half the decision.** Duplication is not the same as
deliberate variation, and confusing them produces worse abstractions than copy-paste. Three
candidates were measured and **all three were deliberately left separate**:

- `ValidName` exists in four services with three implementations, and all three are correct: 128
  bytes in Calendar/Contacts, 255 in Drive because they are file names, and 100 runes in Rooms,
  which validates **channel names** and therefore does not reject `/` or `..`. Unifying them would require a
  length parameter and a “safe for paths” flag.
- `googleReason`/`graphCode` differ between Calendar, Contacts and Drive because each maps the
  HTTP statuses it **actually emits**: Drive those of uploads (413/415/507), the others the 412 of
  conditional requests. It was checked service by service that each omission fits.
- The **Sytue ID** audit listing does not go into the common package: it pushes filters and keyset into
  a store query instead of filtering in memory, which is a different architecture.

The criterion: if the difference is one of **domain** (what a name means here), keep them
separate; if it is one of **accident** (a reflowed comment, a translated message, a number that
someone touched in one place), it is duplication.

**What was learned, and it is what does not show in the diff.**

**Defining something does not apply it.** The `sytue-compress` compression middleware had been **deployed
for seven days** in the cluster, well written, with `zstd`/`br`/`gzip` and `text/event-stream` excluded
so as not to buffer SSE — and **not one of the 49 ingresses referenced it**. The platform
was serving 345 kB uncompressed. A manifest can show a perfectly formed object that is
scenery. Now a render gate requires every Middleware with `compress` to appear in the
argument of the Traefik entrypoint.

**Optimizing what does not run is wasted work that looks like progress.** A session refresh `setInterval`
was annotated as pending a fix —it lacked the visibility
pause—. When going to add it: **nobody called it**, and the real refresh is done by the server
on demand, which is strictly better. It was deleted instead of fixed.

**The bottleneck is not where it is assumed to be.** We were looking for 700 ms of main thread in the portal. Measured:
1146 tasks and **none exceeded 50 ms**, with TBT 0 and zero unused CSS or JS. What was really there was
that nothing was compressed. And the seven API calls “on the critical chain” turned out **not to be
on the LCP path**: their breakdown is TTFB and render delay, with no resource load phase.

**Measured consequences.**

|                                       |                                                                                                |
| ------------------------------------- | ---------------------------------------------------------------------------------------------- |
| Audit: two files of 247 and 245 lines | 57 and 56, with the logic in `platform/pkg/auditlog` and **with tests, which it did not have** |
| `/auth/token` and its getter          | One implementation for all twelve surfaces (previously three by hand)                          |
| Resolving a WebDAV path               | From **six transactions to two**, and it no longer grows with depth                            |
| Adding 200 photos to an album         | From 200 statements with 200 `max(position)` to **one**                                        |
| Portal, bytes transferred             | 345,712 → **104,978 B**; LCP 1,792 → 1,172 ms; Performance 100                                 |

**When to reconsider it.** The §13.2 catalog goes stale on its own: it is generated by the folders, not by an
agreement. If it does not match when looking at the repo, the repo rules. And the 136 store queries without `LIMIT`
that remain noted **were deliberately not prioritized**: the pilot databases weigh between 8 and 18 MB
and choosing which one to fix would be guessing. They will be reviewed when there is real volume, by measuring
`pg_stat_user_tables`, not before.

<a id="0038"></a>

<a id="0038--cuentas-personales-registro-gratuito-con-5-gb-y-plan-de-1-tb-por-5-eur-al-mes"></a>

## 0038 — Personal accounts: free sign-up with 5 GB and a 1 TB plan for 5 EUR a month

`accepted` · 2026-09-13 · product, identity, quotas, billing

**Context.** Until now nobody could get into Sytue One on their own: accounts were born by
invitation from an organization or through an external IdP, and the public website said «Empezar gratis —
sin tarjeta» (“Start for free — no card”) on a button that led to a login screen. We want a single person to be able to
sign up, get 5 GB free for all products and, if they need more, pay 5 EUR a month for
1 TB. Without their own organization —their organization is their email—, and therefore without Sytue Admin or SyERP.

**The decision.** A personal account IS a one-person organization, in the usual data
model —guided onboarding had already decided it that way (mig 0027)—, distinguished by two new columns
(mig 0063):

| Column                                                       | What it decides                                                                                                                                                     |
| ------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `organizations.kind` = `personal`                            | That there is no Sytue Admin or SyERP. **It cannot rely on the role**: the person is `owner` of their own organization, and by role they would get into everything. |
| `org_subscriptions.plan` = `personal_free` \| `personal_1tb` | The quota. 5 GiB of Drive and 1 GiB of mailbox free; 1 TiB and 25 GiB when paying.                                                                                  |

And five pieces come out of that, each in the place that already existed for its purpose:

1. **`/signup` in Sytue ID.** It creates the person and their personal organization in one transaction, with the
   free contract and the guided onboarding already done. The slug is derived from the email
   (`domain.PersonalSlug`: `p-` + digest), because an email is not a valid slug and the slug is a
   subdomain and a namespace. The `p-` prefix is reserved: an organization cannot take it.
2. **The only defense is email verification.** You cannot sign in until you prove
   control of the mailbox. Without that anyone could open accounts —with their storage— in the name of other people's
   addresses. And sign-up does not reveal whether an address already exists: the response is the same.
3. **The plan travels in the token** (`org_kind`, `plan`, `storage_bytes`, `mailbox_bytes`) and each
   service applies it without asking: Drive sets its quota, Mail the mailbox one, SyERP and Sytue Admin
   reject personal ones (`authn.RequireOrgKind`). A token without the claims is read as an
   organization: it predates the distinction, and what existed before were organizations.
4. **A personal account is never locked out.** The free one does not pay, and the 1 TB one that stops paying
   **becomes free again**: what changes is the quota, not the door. Closing someone's session
   over a bounced payment would be closing their own email.
5. **Billing reuses Mollie as is.** A personal checkout sets plan and amount on the server
   —the seats the browser sends are ignored— and the webhook requires that what is charged be
   exactly the price of the contracted plan; a free plan never charges.

**What was decided NOT to do, and why.**

- **A single counter for “all the apps”.** 5 GB _in total_ was requested. Today there are **two counters**:
  Drive —where files, photos (their originals) and documents live, so one figure covers them— and
  the Mail mailbox, which keeps its own. A single counter would require a ledger across services
  that does not exist, and the alternative —Mail reserving against Drive on every message— couples two
  services for something that is solved by declaring the split in one place (`platform/pkg/commerce`).
  If the single counter is ever needed, the event bus is already there.
- **Provisioning storage “on Scaleway” on purchase.** That is not how the system works: the
  object storage is one endpoint per deployment with a prefix per tenant, not a bucket per person.
  What does exist is the deployment's capacity: **the blobs of the `p-*` tenants go from the
  first day to their own bucket in Scaleway One Zone** (second `objectstore` backend,
  `S3_PERSONAL_*`), which grows with usage and costs nothing when empty, while organizations stay on
  the pilot's disk. The measured rates and the details are in
  [`roadmap.md`](roadmap.md#almacenamiento-de-cuentas-personales-en-scaleway). With those numbers
  the 1 TB plan for 5 EUR is **below cost if it fills up** (8.22 EUR/month in the
  cheap class): it lives on average usage staying far from the ceiling.
- **DNS per personal account.** Each organization gets `<slug>.<dominio>`; a personal one does not
  need it —it comes in through `one.<dominio>`— and publishing a record for each free sign-up would be
  giving the zone away. Consequence: in the pilot there is no MX for them, and therefore they do not receive incoming
  mail. It is the same limitation any pilot organization without
  `tenantDNSAddress` has today.

**What was learned by measuring before writing.** Three things seemed necessary and already existed: the
“individual” path of guided onboarding, email verification with its signed token, and the
payment gateway with its webhook. And a discrepancy that is not part of this decision but is worth
noting: the website says “€5 per user per month” for organizations and `commerce` charges 600.

**Consequences.** Three behavioral migrations in `RequirePaidSubscription`: personal
accounts always have the right of use; the unit price allows 0 and 500; and `Entitled` is no longer
just “paid and with seats”. In the pilot there is no Mollie configured, so the 1 TB plan
responds “payments not available” until it is configured —the free account works fully—.

And since 2026-09-13 the move to 1 TB is **closed on all installations**, not only where
Mollie is missing: `commerce.PersonalUpgradeAvailable = false` makes Sytue ID reject the checkout of
a personal account (503), the contract status say so (`upgrade_available`) and the Account app
not offer the button; www announces the price as “coming soon”. Free sign-up
ships before billing; opening it means changing that constant when it is decided to sell.

**Final prices (2026-09-13, same afternoon):** three plans and the same products in all three —
`personal_free` (one person, 5 GB), `standard` (5 EUR per user per month, 200 GB per user) and
`pro` (20 EUR per user per month, 1 TB per user)—, for organizations (per seat) and for
individuals. The 6 EUR seat with 500 GB disappears; what was contracted moves to `standard` (mig
0063). All plans allow connecting your own S3 storage (Drive, `external_storage`), which
does not count towards the quota. And two nuances about the personal account: **it does get into Sytue Admin**, only
to integrate applications and migrate data (`requireOrgKind` closes users, groups, roles,
domains and invitations), and **its mailbox hangs off the parent domain** (`nombre@sytue.eu`), which is
administered in platform.<dominio>: a subdomain with its hash would be an address nobody would know how
to type. And all new storage —of any tenant— goes to Scaleway One Zone, with
previous versions in Glacier as a copy; only the tenants listed in `S3_LOCAL_TENANTS`
stay on the pilot's disk.

**Final names and conditions (2026-09-14):** the 5 EUR one is called **Pro** and the 20 EUR one
**Enterprise**, which includes **Premium Support**; **educational institutions** get Pro at
50 % (`organizations.education`, granted by the platform with a reason and an audit trail; the checkout and the
payment check use `commerce.Plan.UnitPriceCents(education)`); and the **lifetime committed
price increase** is 0.50 EUR per user every four years, and nothing more. Free
sign-up is protected by Sytue's own ALTCHA captcha (`services/id/internal/captcha`,
proof of work without third parties or cookies) and the per-IP limiter. **Connected storage**
supports an S3 bucket, a WebDAV server (Nextcloud/ownCloud, the Internxt CLI), **Google Drive**
and **OneDrive** via OAuth consent in the integrations service, and **the customer's NAS through
the Sytue Cloud tunnel**: Drive creates the site, the resource and the target in the
platform's proxy through its integration API, and the customer only runs a command on their NAS —they never see the
proxy—; the target is restricted to their network (private IP or local name) so that the tunnel cannot serve
an arbitrary site under our certificate. All classes require accepting, on each connection, the
liability notice: backups and recovery of what lives in a connected
store are the customer's (you), and Sytue® is released from all liability for its loss.
Proton Drive has no public API (end-to-end encryption with its own protocol) and will not be
announced until it has one.

<a id="0039"></a>

<a id="0039--aviso-a-móvil-android-no-sale-de-la-ue-ios-sale-con-lo-mínimo-que-existe"></a>

## 0039 — Mobile notifications: Android does not leave the EU, iOS leaves with the minimum that exists

`accepted` · 2026-09-14 · push, mobile, privacy, gdpr, sovereignty

**Context.** The suite already sends notifications with the browser closed via **Web Push** (RFC 8030/8291,
`services/rooms`): the content is end-to-end encrypted and the vendor's push service
—Google, Mozilla or Apple— cannot read it, but it **does see the endpoint** (a
pseudonymous device identifier) **and when** each notification arrives. None of those three is
in the EU. When bringing the suite to native apps we have to decide where the notification comes in through, and the
answer is not the same on the two platforms.

**Android: no third party is needed.** The app can keep the **hub SSE
stream** that the web shell already consumes (`NotificationCenter.tsx` with `EventSource`) open from a
foreground service. The notification is born in our cluster and dies on the device: no FCM, no
Google Play Services, no identifier outside the EU. It also works on phones without
Google services, where FCM simply does not exist.

**UnifiedPush is left for later, and we do not need it.** UnifiedPush solves _several_ apps
sharing _one_ connection maintained by a distributor. Since here we only notify
ourselves, a distributor adds a piece —and a push server to maintain— without removing
any. If sharing the connection is ever of interest, the current sender **already works**: UnifiedPush
3 speaks Web Push, and `validatePushEndpoint` has no vendor allowlist, it only requires
`https` and the anti-SSRF barrier. The endpoint would be registered and that's it.

**iOS: there is no alternative, so we decide what is told.** The system kills the connection when
going to the background and **APNs is the only way** to wake an app; Apple has no
European region. What is chosen is the content, and the minimum that exists is chosen:

- The notification is a **constant**, `{"aps":{"content-available":1}}`, with
  `apns-push-type: background`. No title, no text, no author, no channel, no counter.
- The app wakes up and **asks our servers for the message**, which are in the EU, and draws
  the notification itself.
- Apple can know that that device received something and when. **Not what, nor from whom.**
- That the body is a constant and not something composed is deliberate: it prevents anyone from
  adding a field “just for debugging” without seeing what they are publishing. It is pinned by the test
  `TestAPNSNoMandaNadaMasQueElDespertar`.

**Consequence for what we promise.** “No personal data leaves the EU” (§1.2 of
`security.md`) is no longer true without nuance as soon as there are notifications with the app closed, and not
only on iOS: the **Web Push already in production** shows the endpoint and the timing to
Google, Mozilla or Apple. The claim is corrected instead of being left as is; a legal text that is not
met is worse than a written exception. Whoever does not want even that can turn off notifications:
`POST /me/push/apns/forget` and `POST /me/push/unsubscribe` delete the device.

**Alternatives ruled out.** _FCM on Android_ (identifier and metadata to Google without
need, and it does not work without Play Services). _Content push on iOS_ with the text in the notification
(we would be giving it to Apple in exchange for saving a request). _Background refresh_ without APNs
(iOS decides it, it takes from minutes to hours: it is useless for a call or a DM).

**Amendment (2026-11-06): a single device registry, in the Hub, to wake the
widgets** (piece 5 of [ADR 0078](#0078); written on 2026-11-07). The owner decided that,
with the app closed, “the app wakes up in the background” so that its widget shows what is happening
now. Periodic refresh (piece 4) is decided by iOS and can take hours; for an
event created on the web to reach the widget within minutes, the only path on iOS is still
APNs. Until now **no client registered a token** (Rooms' `push_apns` was empty)
and the server did not know what devices anyone had.

1. **One registry, in the Hub** (`aparatos_apns`, mig. 0010): `tenant, subject, bundle, token,
creado, visto, ultimo_push, pendiente`, with key `(tenant, bundle, token)` —APNs gives one
   token per app and device—, deny-by-default RLS and the merge policy. **Rooms loses
   `push_apns`** (mig. 0062; there were no rows to move) and wakes, with the Hub's change
   signal and without a new route, whoever has their Rooms widget counter change
   (`api.PendientesRooms`): channel members with the default level, and with
   “mentions” those mentioned (or everyone, with @channel); muted, nobody. **They are not the
   Web Push recipients** (those mentioned and DMs): that one is an alert, and this is an empty
   notification so the counter does not fall behind; that is also why they do not look at the notification pause
   or blocks (the unread of a blocked person also counts), and what is written by an
   integration or a system card wakes even if it does not notify. At most 50 per
   message, those mentioned first: in a larger channel, the rest wait for the periodic
   refresh (revision of 2026-11-07; previously it followed Web Push and a message without a mention in
   a channel did not move the widget). The sender **moves** to `platform/pkg/apns`,
   with the `apns-topic` on each send; `TestAPNSNoMandaNadaMasQueElDespertar` moves with it
   and now also checks that no extra `apns-*` header goes out.
2. **Routes.** `POST /api/v1/me/push/apns {bundle, token}` registers (bundle from the closed
   list of the fifteen apps, hexadecimal token; a delegated token does not register);
   **`POST /api/v1/me/push/apns/forget`** deletes: with `{token}`, that one; **without a body, all the
   person's devices** in that organization. A delegated token does not forget either (403):
   without a body it would delete all the iPhones of the represented person. Rooms' homonymous routes redirect
   **308** to the Hub until they are removed, and they only say where it is now: the Hub is another host,
   `Authorization` does not follow the redirect (net/http and fetch strip it) and a `forget`
   followed without a token gets a 401. Whoever calls Rooms repeats the call to the Hub with their
   token; the apps already call the Hub, and the promise above —“`forget` deletes the device”—
   holds at the Hub URL. And account deletion
   (ADR 0054) takes `aparatos_apns` with it.
3. **What wakes.** The change signal (`datos`, `{"producto"}` from a closed list) that the
   Hub already receives from every write: the replica that handles it looks up the person's devices
   with the app of that product (and One, `eu.sytue.one`, with what appears in its widget:
   `mail` and `calendar` for their counters and agenda, and what fills the Hub's recent
   activity —Drive and what lives in it, Docus, Calcus, Diapos, certiDocs and Photos, plus
   Bases, ERP and Notes—; Contacts, Rooms and Cast do not produce recent activity and do not wake it).
   **At most one wake every 5 min per device**, reserved in the database and not in
   memory —the Hub runs with two replicas—: a change marks `pendiente` and whoever gets
   the row with `UPDATE … WHERE ultimo_push < now() - 5 min RETURNING` sends; what falls inside the
   window is picked up by a sweep every minute with the same reservation (`SKIP LOCKED`). It is pinned by a
   test with two pools against the same PostgreSQL. `apns-expiration` at 1 h, the usual
   body, and a 410 or `BadDeviceToken` deletes the device. Someone known only by their email
   (a Calendar guest) is not woken: registration is by subject.
4. **What Apple sees, in addition to the above: which app of the suite has something new** on that
   device (the `apns-topic`), and therefore, approximately, when that person's calendar or
   mail changed. It still does not see what, from whom, or how much.
5. **Only if the person turns it on.** «Actualizar los widgets con la app cerrada» (“Update widgets with the app closed”) is
   **off by default**, lives in each app's account menu (on iOS there is no tray
   to put the `ElementoBloqueo` one in) and only appears in builds with
   `SYTUE_DESPERTAR_APPLE = YES`; its help text says what Apple sees. The app only
   registers with the switch on, one of its widgets placed, a session and without the suite
   lock; and it **forgets** (`forget` with its token) on sign-out —before revoking the session—, when
   setting the lock, when turning off the switch, before switching organization (the rows
   belong to each one and `forget` deletes in the token's one; on signing in again it registers in the
   new one) and when removing its last widget. The first four apply to all fifteen: the app in which they happen forgets the tokens of all of them, which each one stores in
   `despertar-<bundle>` of the **suite's keychain group** (in each app's private one, the
   one signing out only saw its own); the list is the closed one of the fifteen and not the
   store's index, which belongs to each app. With a session (turning off, the lock), a token the Hub has not
   forgotten —no network— stays and the app retries on return; on sign-out it is deleted anyway. **Signing out
   turns off the switch**: it is the consent of whoever turned it on, not of the device, and it is
   stored with their fingerprint (`ajustes.DespertarDe`): with another person signed in it counts as off
   even if the previous session expired without signing out. A registration still in flight when
   the forget arrives is cut off and its token is forgotten too. A device that does not
   register again within 30 days is purged. **What remains open:** if the session ends without signing out
   (it expires or is revoked) and another person signs in, the previous person's rows remain in the Hub —the
   forget is by subject and the new session cannot delete someone else's—, and the device
   wakes with the previous person's changes until the purge, until she signs out on another
   device (the bodiless `forget`) or until Apple considers the token expired. What it reads on
   waking belongs to the new session; what leaks is when the other person's data changes
   (security.md#apns-despertar).
6. **The EU baseline grows, and this is the decision that documents it**
   (`compliance/supply-chain/README.md`: «solo puede disminuir … sin una decisión
   documentada» (“it can only decrease … without a documented decision”)). `api.push.apple.com` (and its `sandbox`) go into `providers.tsv` and
   `blocked-baseline.tsv`, and `platform/pkg/apns` into `runtimeProviderSource`. **It is not a
   new provider**: it is the same sending that this ADR already accepted, which until now lived in
   Rooms without the gate looking at it.

What stays the same: the constant body, `background` and priority 5; Android without third parties (no FCM
and no UnifiedPush); session tokens, only in the app; macOS does not apply (`content-available`
does not launch a closed app). **Status**: the server is done and without Apple credentials it does
not wake anyone; in the app, registration and forgetting are behind patch `0017` of the fork and
`SYTUE_DESPERTAR_APPLE = YES` since 2026-11-07 (the owner's decision, ahead of their steps at
Apple and in the Hub). What is missing is up to the owner:
[runbook](operations.md#despertar-widgets-ios). Security: [§1.2](security.md#apns-despertar).

---

<a id="0040"></a>

<a id="0040--usuario-global-y-conversaciones-con-personas-de-otras-organizaciones"></a>

## 0040 — Global @username and conversations with people from other organizations

`accepted` · 2026-09-14 · identity, rooms, multi-tenancy, privacy · **revises the sign-up of [0038](#0038)**

**Context.** Rooms only talked within one organization: each row with its `tenant` and its RLS,
and the gates requiring the team. And self-service sign-up asked for an outside email that had to be
verified before getting in. Two things were requested: that every user have an **@username** by
which someone from another organization can find them —and that sign-up ask for that @username instead of an
email, because it is the front part of their address: **@ana is ana@sytue.eu**—, and being able to **open
channels, direct messages and meetings with outside people** by typing only their email or their
@username.

**Decision: the @username (Sytue ID).**

1. **`users.username`, global and unique case-insensitively** (mig 0065). It shares a namespace
   with the addresses of the root domain, so its rules are those of a mailbox local part
   (`domain.ValidUsername` is a fixed point of address normalization), the reserved ones
   are those of `platform/pkg/mailaddr` and **an @username whose address in the root
   domain belongs to someone else is not assigned**.
2. **Sign-up asks for @username, name and password.** The account is born with `usuario@<dominio>` as the
   sign-in email and as the exact mailbox (`store.ReserveMailbox`; if it is taken, nothing is
   created), **verified**, and gets in immediately. The verification of 0038 existed because the
   address was SOMEONE ELSE'S; this one is ours. And saying “that @username is not available” leaks
   nothing: an @username is public by definition. What protects sign-up is still the captcha and
   the per-IP limiter. Old accounts with an unverified email remain as they were.
3. **Whoever does not choose one gets one** —invitation, SCIM, SSO—: `nombre.apellidos`, and
   `nombre.apellidos2` if it is taken. It is set by `store.CreateUser`, which is the only path that
   all six sign-up flows go through. The column is born nullable so as not to break sign-ups from pods of the
   previous binary during deployment; those and all prior accounts get one from a
   sweep (`AsignarUsernamesPendientes`) with the same rule, and **root-domain addresses
   go first**. The `NOT NULL` is the contract step of the next version.
4. **You sign in with the email or with the @username** (with or without the at sign).
5. **`POST /api/v1/people:resolve`** finds someone from any organization, with four
   barriers: exact (no prefixes or listings), the minimum (identifier, name and @username;
   **never** the sign-in email —it may be a Gmail— nor the organization), per person and capped,
   and only active accounts. It is what `directory:resolve` deliberately does not do.

**Decision: conversations with outside people (Rooms).**

1. **The conversation lives in the organization of whoever opens it**, with its rows, its RLS and its
   retention. The outside person is one more row in `channel_members` with **`externo`** (mig
   0036), and that flag replaces the team requirement in the read and write gates,
   **only in that channel**. All the gates go through `requireChannelAccess` and
   `requireWriteAccess`: the change is in two functions.
2. **Their name is given by Sytue ID, not by the browser** of whoever adds them: it is how everyone will see them. Rooms
   asks with the caller's token, so Sytue ID's per-person limit applies.
3. **Their token carries THEIR organization.** The routes of a specific channel, message or room
   switch to the host organization when they are there as an external (`chathttp.EnAnfitrion`), and
   **without the roles of their token**: an owner of their own organization is nothing in the host one, and with the
   roles in place the video would give them moderation and recordings. Real time subscribes to each
   channel in their organization's room. The only cross-organization query is “in which
   organizations is this person an external?”, and it goes through a maintenance policy that is
   **read-only and only sees the `externo` rows**.
4. **A group of people has a single conversation**, whoever opens it: a DM's slug
   is that of the set and does not depend on the organization, so before creating it a search is made where the
   opener is already an external.
5. **Meetings.** The call of those conversations —DM, group or channel— is the meeting with outside
   people, with the same waiting room and the same lock. A standalone meeting of the
   host organization is **not** opened to anyone from outside just because they are in one of its DMs.

**Revised on 2026-10-16 (owner's decision: «Que acepte primero» (“Have them accept first”)).** The outside person no
longer gets in when added: they receive an invitation (`invitaciones_canal`, Rooms mig 0041, a table
separate from `channel_members`) and they do not read, write or receive anything until they accept it; they can
reject it. It applies to channels, DMs and groups, and opening the DM themselves means accepting it. Whoever was
already inside before 0041 stays inside.

**What was not done, and why** (pending in
[`roadmap.md`](roadmap.md#conversaciones-con-personas-de-otras-organizaciones-lo-que-falta)).

- **Attachments** in those conversations: the file belongs to the host's Drive and Drive resolves
  by the token's tenant.
- **Push notifications** to the outside person: their subscriptions are in their organization.
- **“Saved”, “Threads”, drafts and scheduled messages** of those conversations do not appear in their
  lists, which are read per organization.
- **Recovering the password** of an account created by @username: the link goes to its own mailbox.
  Solved in [0041](#0041).
- **Changing the @username**, and an **organization policy** to close contact with outside people.

**Ruled out.** _Also asking for an external email_ (it brought back verification and enumeration).
_A “shared” tenant for cross-organization conversations_ (it would have no owner to
decide retention or eDiscovery). _Copying the conversation into each organization_ (edits,
reactions and threads would diverge). _Switching organization via a header_ (with a single external
row, whoever had it would get into the host's standalone meetings, which admit
anyone from the organization). _Giving everyone their @username as an alias in the root domain_ (it is a mail
decision for each organization, not a side effect of having a name).

<a id="0041"></a>

<a id="0041--móvil-verificado-por-sms-en-el-alta-y-cuatro-identificadores-de-acceso"></a>

## 0041 — Mobile number verified by SMS at sign-up and four sign-in identifiers

`accepted` · 2026-09-15 · identity, privacy, third parties, ux · **revises the sign-up of [0040](#0040)**

**Context.** With [0040](#0040) sign-up asked for @username, name and password, and the account
was born immediately: nothing tied an account to a person, and whoever lost the password of an account
created by @username had no way to recover it —the link went to its own mailbox—. It was requested
that sign-up **require a mobile number verified with a one-time code by SMS**, that the **recovery
email** be optional and set in Sytue ID, and that it be possible to **sign in with the @username,
the Sytue address, the recovery email or the mobile number**, always with the same password.

**Decision.**

1. **The account is not born without the code.** Sign-up stores what was filled in in `signup_phone_codes` (mig
   0066; the password already in Argon2id) and sends a six-digit code. Ten minutes, five
   attempts —spent before comparing, in the same statement that checks the cap—, resend
   after one minute, and the code hash bound to its sign-up. The SMS carries the line
   `@<emisor> #<código>` so that Android and iOS autofill it only in Sytue ID.
2. **That a mobile number already has an account is not said in the form**, which would be telling anyone
   whose number it is: the SMS is sent anyway and it is said after the code, to whoever has proven that
   the number is theirs. With the recovery email, the same: it is said when opening the link.
3. **SMS cost money and are the gateway to toll fraud.** In addition to the captcha and the
   form limiter, two shared limiters: 3 SMS in a row per number (then one
   every 20 minutes) and 10 per IP (then one every 5).
4. **Sweego sends the SMS** (`api.sweego.io/send`, transactional SMS). It is operated by Mindbaz SAS
   (Lille) with hosting in France (OVHcloud and Scaleway); it receives only the number and the code,
   and the privacy notice names it. Scaleway has no SMS API. Compared with Brevo, which was already
   approved for the mail relay, it comes out 20 % cheaper: €0.035 per SMS to Spain
   (Portugal €0.0225, France €0.040, Germany €0.078; excluding VAT, no fee), about €42 a month
   with a thousand sign-ups. Our own eSIM in a phone is cheaper from ~300 SMS a month onward, but
   the “unlimited SMS” of the plans is for person-to-person use, does not cover foreign numbers and
   leaves sign-up in the hands of a phone that is switched on.
5. **No key, no sign-up.** `SYTUE_ID_SMS_SWEEGO_API_KEY` (secret `sytue-id`, key
   `sms-sweego-api-key`; in the pilot, `SMS_SWEEGO_API_KEY` in `.secrets.env`) is optional to
   start: without it, in production `/signup` responds that sign-up is not available and the rest
   of the IdP stays the same. In development the SMS is written to the log.
6. **Four identifiers, one password.** `userByLogin` resolves the mobile number (E.164; spaces and
   the 00 prefix are allowed, never a number without a country prefix), the @username, any
   Sytue address of the person —mailbox or sign-in email— and the recovery email, in that
   order: a mailbox address beats another account's recovery email. The same applies to
   passkeys and “forgot my password”, which sends the link to the sign-in email **and** to the
   recovery one.
7. **The recovery email asks for the password and is confirmed by link** (`PUT
/api/v1/me/recovery-email`, the «Acceso y recuperación» (“Access and recovery”) card in My account). With a stolen
   session, setting one's own email and requesting a reset would mean taking over the account. An
   address in the suite's domain or the sign-in email is not allowed; on confirming it, an address that already
   gives access to another account is rejected. Verification links now carry their purpose.
8. **Password sign-in carries the same ALTCHA captcha as sign-up** (`LOGIN_CAPTCHA_ENABLED`,
   on by default; challenge at `/login/captcha`). It is solved only when pressing «Continuar» (“Continue”) —the
   checkbox is no longer mandatory in either of the two— and the server checks it before looking at
   the account, so a submission without a challenge does not spend an Argon2id or anyone's attempt quota.
   Passkeys do not carry it: there is nothing to guess.
9. **Sign-up is a three-step flow with its progress bar**: your details (a sentence under
   each field that says what it is for), the mobile code and **«Todo listo»** (“All set”), which summarizes @username,
   email and mobile number, reminds you that you can sign in with any of them and takes you to **Sytue Hub** —or to the
   app that asked to sign in, if the sign-up started in one—.

**What was not done, and why** (pending in
[`roadmap.md`](roadmap.md#conversaciones-con-personas-de-otras-organizaciones-lo-que-falta)).
Changing the mobile number from My account (requires verifying the new number again), asking older
accounts for a mobile number, and a global cap of SMS per hour.

**Ruled out.** _Guessing the country prefix_ from the language or the IP (it would sign up someone
else's number). _Saying in the form that the mobile number already has an account_ (phone enumeration).
_Refusing to start without an SMS key_, as with SMTP (on deployment day it would bring down the entire IdP
for a feature that can turn itself off). _Recovering the password by SMS_ (it was not requested,
and SMS is the easiest factor to hijack with a SIM swap). _One question per screen_
at sign-up (it would force carrying the password from one screen to another within the page or storing
state per step; three steps give the same clarity).

<a id="0042"></a>

<a id="0042--españa-sin-gestoría-el-banco-entra-por-norma-43-y-presentar-por-ti-espera-al-certificado-y-a-verifactu"></a>

## 0042 — Spain without an accounting agency: the bank comes in via Norma 43 and filing on your behalf waits for the certificate and VeriFactu

`accepted` · 2026-09-15 · erp, tax, banking, ux · full design in [`architecture.md` §7.5](architecture.md#75-españa-sin-gestoría-banco-obligaciones-guiadas-y-presentación)

**Context.** It was requested that a business owner with no knowledge of taxes, accounting or
IT be able to handle all their obligations from Sytue ERP: guided step by step, with weekly
reminders, with their own date four business days before the one given by the administration, with
each bank transaction turned into a task and matched automatically whenever possible, integrated with
Spanish banks and administrations, with the company's certificate and filing “whatever
there is” on the deadline. And that it be designed for business owners, not for accounting agencies.

**Decision.**

1. **The bank comes in via Norma 43, not via PSD2.** All Spanish banks provide it from
   business banking and reading it requires no contract or license; PSD2 requires a provider with an
   account information license, with cost, a contract and data going out to a third party. Matching is the
   same for both: when a provider is chosen, its transactions come in through it.
2. **Each transaction is a task until it has an invoice or a reason**, and it is matched automatically only with the criteria
   of a prudent accountant: exact amount, reasonable date, evidence in the description and no
   equally likely alternative, and only against confirmed invoices. A misallocated payment is
   worse than a question. Everything is undone with a reversed entry, never by deleting.
3. **The date shown is “your date”**: four business days before the last valid day, which
   is the direct-debit date when the payment can be direct-debited. Holidays come from the annual
   BOE resolution; the legal deadline is moved only by national holidays and the margin also counts
   regional ones, which is the safe side in both cases.
4. **Weekly reminder through an imported calendar**, not a subscribed one: Google always discards the
   alarms of a subscribed calendar and Apple does so by default. Email or push notification
   waits until the ERP has a service identity.
5. **Filing on your behalf stays in phase 2, behind VeriFactu and the certificate.** Filing requires the
   consent of the declarant, what is filed cannot be cancelled, and each form's file has to be
   validated against the Tax Agency before sending it; doing it without that would be filing
   blindly on someone's behalf. VeriFactu goes first because it is mandatory for the product itself
   from January 1, 2027.

**Consequences.** The ERP already computes form 130 like the form does (accumulated minus what was paid) and knows
what each obligation is missing. The Spain data grows: work calendar, territories, guides and
form 184. A Basque or Navarrese company sees Social Security and the Registry, not its taxes, and the
screen tells it why.

**What was not done, and why.** Certificate, direct filing and DEHú (phases 2 and 3 in
§7.5); the PSD2 provider (a contract and privacy decision); attaching the invoice PDF
(the catalog has no file type); VAT rates per line (needed for the filed 303,
not for the computed one); and separating, in the Treasury charge, the self-employed person's contribution from
that of their employees (today it all goes to 642).

**Ruled out.** _webcal subscription_ with a capability token (a public route, a new ingress and, even
so, no alarms). _Matching by amount alone_ (two identical receipts in the same month). _Filing without
a registered mandate_. _Counting regional holidays to move the legal deadline_ (if the
administration does not apply them, the filing would be late).

<a id="0043"></a>

<a id="0043--el-erp-es-de-toda-la-organización-permisos-por-módulo-tareas-y-documentos-y-sytue-ai-que-crea-con-confirmación"></a>

## 0043 — The ERP belongs to the whole organization: per-module permissions, tasks and documents, and Sytue AI that creates with confirmation

`accepted` · 2026-09-15 · erp, ai, permissions, mail · design in [`architecture.md` §7.6](architecture.md#76-el-erp-de-toda-la-organización-permisos-tareas-documentos-y-sytue-ai-que-crea)

**Context.** It was requested that Sytue AI create records in any module, several at once and with
everything related (a €180 Claude Code expense creates the supplier Anthropic if it does not exist),
consulting the internet for the data, and that it leave the task of uploading the PDF due today; that
on each bank charge it say where the invoice is downloaded from or whom to ask for it and with which
email; that the ERP belong to the whole organization, with Odoo-style per-module permissions from
Sytue Admin; that when uploading an invoice or a document it say what it has to include; that
Obligations always be active after the wizard; and that the ten-day notice arrive by
email to whoever has permission in Obligations.

**Decision.**

1. **Propose and confirm, in two steps.** The AI proposes a plan; the service validates it,
   completes it with what already exists and checks it; the person sees it and creates it in one transaction.
   Storing whatever the model answers would put data nobody has looked at into the ledger.
2. **The internet means checks, not browsing.** VIES for the VAT number, HEAD/GET for pages and
   MX for emails, from the service and with an SSRF barrier. The model does not search the web: Sytue AI
   does not browse, and a search engine would be a new third party with the organization's queries. What the
   model knows from memory is checked and what is not confirmed is left empty.
3. **Permissions live in the ERP and are edited in Sytue Admin.** The ERP is what applies them on each
   request and the modules are from its catalog. Four levels like Odoo's access groups,
   a default per module and only what differs is stored.
4. **Tasks and documents in their own tables**, not as catalog fields: they hang from any
   record and also from obligations, which are not records, and uploading the document closes the
   task. The files go in Postgres with a 10 MiB cap: an invoice weighs hundreds of kilobytes and
   that way they inherit RLS and backup without another store or another permission.
5. **Mail goes out through our own MSA with a service mailbox**, from a scheduler inside the
   service that claims each send in the database: once per obligation and period even if
   there are several replicas.

**Consequences.** A new member no longer sees the accounting or the payroll until they are given
access; owner and admin notice nothing. The ERP needs a service mailbox to send notices and, without it,
it does not send them. VIES receives the VAT numbers of the European suppliers the AI proposes.

**What was not done, and why.** Row-level rules (Odoo's “record rules”: seeing only your own
within a module), per-record change history and followers, OCR of the uploaded PDF to
fill in the invoice, and reading incoming invoice email. They come later in the parity with Odoo.

**Ruled out.** _Directly creating whatever the model answers_. _A web search engine for the AI_
(a new third party with no way to check what it returns). _Storing the PDFs in Drive or S3_ (another
store and another permission for small files that belong to the organization, not to a person).
_Permissions as Sytue ID scopes_ (the ERP would have to ask ID on each request about
modules ID does not know). _Push notification instead of email_ (email was requested, and push only
reaches whoever has the app installed).

<a id="0044"></a>

<a id="0044--verifactu-dentro-del-erp-la-base-garantiza-la-cadena-y-el-envío-sale-con-un-certificado-del-servicio"></a>

## 0044 — VERI\*FACTU inside the ERP: the database guarantees the chain and submission goes out with a service certificate

`accepted` · 2026-09-15 · erp, tax, verifactu · design in [`architecture.md` §7.7](architecture.md#77-verifactu-cada-factura-registrada-en-la-agencia-tributaria-al-emitirla)

**Context.** From 1-1-2027 (companies) and 1-7-2027 (everyone else), every program with which a
Spanish business owner invoices has to comply with the Regulation on invoicing computer
systems, and whoever produces it is liable for up to €150,000 per year and type of program if not.
It was requested to move forward on everything needed with the VERI\*FACTU mode.

**Decision.**

1. **VERI\*FACTU only**, without the mode that keeps and signs records without sending them: the
   product is in the cloud, the Tax Agency keeps the copy that serves as proof, and without record signing
   or an event log there is less that can fail.
2. **The record is generated in the transaction that issues the invoice** and **the database prevents what cannot
   be undone**: immutable records, a chain without forks and an issued invoice without
   retouching, with triggers and constraints in the migration. That way a new path that issues an
   invoice without going through `emitirVerifactu` fails instead of leaving a gap.
3. **One chain and one installation number per organization**, because each one is a different
   obligated party (AEAT FAQ for developers, section 4): `TipoUsoPosibleMultiOT=S` and
   `IndicadorMultiplesOT=N`.
4. **Submission goes out from syerp with mutual TLS and a service certificate**, not through the
   Integration Gateway: it is SOAP with a client certificate to a public body, and taking the
   certificate to the gateway would be another place to store it. Flow control is kept by the database
   so that two replicas do not skip the wait.
5. **The invoice says what is missing before it is issued** (VAT number, current VAT rate, VAT-exempt regime):
   what the AEAT would reject later forces a correction, and a business owner without knowledge does not know
   what a “registration after rejection” is.

**Consequences.** A confirmed invoice is no longer edited or deleted: it is corrected with a
corrective invoice or, if the AEAT rejected it, it is corrected and sent again. Contacts need a
VAT number to be invoiced. The ERP has a printable invoice for the first time.

**What is missing, and it is up to the owner.** The **certificate**: Sytue's as a social collaborator (an agreement
with the AEAT, which allows filing on behalf of customers without power of attorney one by one) or as the attorney-in-fact
of each customer; without it nothing is sent. **Signing the responsible declaration** for each
version (`verifactu.DeclaracionFecha`), without which production cannot be activated.

**What was not done, and why.** Simplified invoice (F2): the model requires a customer. IGIC and
IPSI: the Canary Islands, Ceuta and Melilla stay out until invoices have their tax. Several
lines with different rates: the ERP invoice has one base and one VAT. Corrective invoice by
substitution and invoices issued by third parties. Querying what was submitted at the AEAT.

**Ruled out.** _Generating the record afterwards, in the background_ (the rule requires it at issuance and the
invoice could go out without it). _Storing each customer's certificate_ (a business owner without
knowledge does not know how to export it, and holding them is an unnecessary risk). _Routing the
submission through the Integration Gateway_. _Allowing an accepted invoice to be edited and simply corrected_ (the
rule only allows correcting what does not require a corrective invoice, and telling them apart is not up to whoever
invoices).

<a id="0045"></a>

<a id="0045--una-sola-interfaz-en-gio-para-web-escritorio-y-móvil-sytuecom-en-gio-sytueeu-en-la-actual"></a>

## 0045 — A single Gio interface for web, desktop and mobile: sytue.com in Gio, sytue.eu in the current one

`superseded` → [0048](#0048) on desktop and mobile · 2026-09-20 · frontend, desktop, mobile, gio · code in `apps/gio/`

> **Superseded on 2026-09-21 by [0048](#0048)**, which returns desktop and mobile to Fyne with
> our own widgets and leaves the web on Astro + Preact. `apps/gio` **is not deleted**: today it serves
> `sytue.com` in production and stays until the owner decides what that domain publishes. What is
> below is kept because it explains where `apps/gio/` and the file layout come from.
>
> **Replaced by Fyne and removed from the repository on 2026-09-23**: the Gio suite, its
> web server and the Wails desktop that opened it leave the tree (`sytue.com` is now a 301
> to sytue.eu). The copy of the code remains in `gio-archivado/`, outside git.

**Context.** It was requested to migrate the entire suite —the fourteen Astro + Preact apps in `apps/frontend/`
and the Fyne desktop in `apps/desktop/shell/gui/`— to [Gio](https://gioui.org), a single Go codebase
that compiles to WebAssembly, desktop and mobile, “pixel for pixel” and without a webview. Measured before
starting: it is **about 440,000 lines** of application plus 36,000 of shared packages. It is not a
task; it is a work program, and this ADR writes down where it goes, in what order and with what
rules.

**Decision.** The owner made it on 2026-09-20, with the four costs below in view:

- **Gio is the destination in all three places** —web, desktop and mobile—, with **a single codebase**. Not
  “Gio on native and Astro on the web”: the reason for doing it is to stop maintaining two interfaces.
- **First, coexist in the repository.** `apps/gio/` lives next to `apps/frontend/` and
  `apps/desktop/`, and nothing that exists is deleted. While it lasts, the web is the source of tokens,
  catalogs and rules, and Gio reads it or compares against it.
- **And when finished, coexist in production, by domain.** When the phases are complete,
  **`sytue.com` publishes the entire Gio version and `sytue.eu` stays with the current one** (Astro + Preact
  - Go). There is no handover that deletes anything: they are two fronts on the same services and the same
    data, and [0004](#0004) remains in force for `sytue.eu`.

  > **This stopped being true on 2026-09-21.** The Gio port was cancelled —desktop and mobile go
  > to Fyne, the web stays on Astro + Preact + Go— and `sytue.com` now serves **the Astro suite**,
  > with a second set of images deployed on the front node: [ADR 0049](#0049). The rest of
  > this ADR (the file layout, `apps/gio/`) is kept because it explains where what
  > remains in the repository comes from.

- **Fyne is replaced, not deleted abruptly.** Today it is the Windows and Linux client, and only
  `shell.Run` calls it (`gui.RunGUI`). From 2026-09-20 it is **frozen** —neither extended nor
  fixed— and it disappears in the SAME PR that puts the Gio shell at that point, so that there is no
  release without a desktop. The publishing pipeline, `bundles.json` and the downloads are not touched.
  The declarative screen contract (`packages/ui/src/native/`, the `NodoUI` tree) is kept:
  what changes is who paints it.
- **It is not a tracing: it is ported and improved.** Whatever the port finds improvable —functionality,
  interface, experience or design— is implemented along the way, and is noted in
  [`roadmap.md` §Gio](roadmap.md#gio-mejoras-identificadas-durante-el-puerto) with its decision:
  what was changed, whether it goes only in Gio or also in the current suite, and why. An unnoted
  improvement is a divergence between the two domains that nobody will be able to explain. Where NOTHING
  is touched is in what has to give the same result: formulas, plurals, dates, permissions.
- `ci-gio.yml` watches the module —native and WASM build, vet, tests with none skipped, lint and
  up-to-date catalogs—, because `go build ./...` at the root does not enter a separate module.

**The rules it is built with (phase 1, already done, without an interface).**

1. **The sources rule, not the brief.** The brief brought values that are not in the code and
   what exists was ported: the standard curve is `(0.2, 0, 0, 1)`, `fade-in` is opacity only,
   `drawer-in` enters from −100 % without touching opacity, the skeleton pulse is 1 → 0.55 → 1,
   `accent-700` is `#224656`, there are 26 languages and not 24, the breakpoints are Tailwind's (640/768/1024)
   and stable Gio is v0.10.2. It also asked for APIs that are Fyne's and not Gio's
   (`canvas.Rectangle`, `app.Settings().ThemeVariant()`).
2. **What can be read is not copied** ([0037](#0037)). The colors come from the SAME stylesheet as
   the web: `scripts/sync-branding.sh` leaves a copy of `branding/tokens/colors.css` and
   `internal/theme` interprets it at startup, with the dark theme as a reassignment over `:root`.
   What did have to be written by hand (scale, radii, shadows, keyframes, cache rules,
   catalog keys) has a test that looks for each value, literally, in its web source.
3. **Where the web relies on the browser, the browser is the reference.** Plurals and number
   formatting are compared against vectors taken from the real `Intl`: `golang.org/x/text` carries CLDR 32
   and no longer answers the same (Maltese, Icelandic, “1234” without a dot in Spanish, U+202F in French).
4. **One session and one expiry margin.** Native: Authorization Code + PKCE to loopback with the
   `sytue-desktop` client from migration 0069, and the refresh token in the system keychain. Web:
   **the token does not go down to the browser** —not even into memory—; the BFF stores it and sets it itself (see
   item 3). The brief asked for `sessionStorage` and a `sytue://` redirect that the IdP does not have
   registered; neither was done.
5. **What exists is ported.** `@sytue/offline` has no mutation queue; what was ported is the store
   per owner, connectivity and the “network first” cache with its privacy rules.

**Two measured findings, which hold even if this does not continue.**

- **On the web 1 rem is 14 px, not 16.** `theme.css` sets `html { font-size: var(--text-base) }`
  and that rescales everything declared in rem: Tailwind typography and spacing come out at 87.5 % of
  what the stylesheet comments say. Measured with Playwright against `www.sytue.eu`: `html`
  14 px and the `--text-4xl` `h1` at 42 px, not 48. `text-sm` shows at 11.4 px. Gio ports what
  is seen (`theme.Rem = 14`); if it turns out to have been an accident, it is one line on each side.
- **The Fyne desktop's `Figtree-*.ttf` files are GitHub HTML pages**, not fonts
  (`750188b16`: the `blob` URL was downloaded). The binary carries 1.3 MB of HTML embedded as a typeface.

**What the decision forces us to build, and was not in the brief.** They are not reasons not to
do it —it is already decided—; they are the five items that weigh most, so that none appears as a
surprise.

1. **An editor.** `gioui.org/x/richtext` paints styled text; **it does not edit**. Docs, Sheets,
   Slides, PDF and Notes are 90,000 lines on top of `contenteditable`. In Gio they are not ported: a
   text editing and layout engine is written. It is the biggest item of the program.
2. **Web accessibility and SEO.** Gio in WASM paints into a `<canvas>`: no screen reader,
   no browser selection or search, limited IME and nothing to index. The web in Gio
   needs a semantic mirror in the DOM (Gio's `io/semantic` tree, which on desktop already
   feeds screen readers) and `www` needs HTML that a search engine can read. Until it exists and
   is measured, those surfaces are not handed over: Directive (EU) 2019/882 is binding since June 2025.
3. **The BFF.** The Astro apps are not only interface: they keep the session behind the httpOnly cookie. The
   Gio one is `services/gioweb`, **done** ([architecture](architecture.md#gioweb)): it serves the
   WebAssembly and stores the SEALED session (AES-256-GCM) in an httpOnly `__Host-` cookie. It goes one
   step further than the Astro one, which hands the token to the page via `/auth/token`: here the API
   is requested from the same origin (`/svc/<producto>/api/…`) and the `Authorization` is set by the server, so
   no script on the page can reach a token and the services do not need new CORS.
4. **Login on mobile.** iOS and Android cannot return to a loopback; they require an OIDC client
   with its own redirect, that is, a migration in `services/id`.

5. **`sytue.com` as a second front.** Today `sytue.com` responds 301 to `sytue.eu`
   (`sytue-com-redirect`, `bc3000c15`): on publishing day it has to be removed. The suite's session cookie
   is for `.sytue.eu` and does not travel to `.sytue.com`, so the Gio front needs its
   own BFF and its OIDC client with a redirect at `sytue.com`. **Done, except for the handover:** ONE
   confidential client for the whole suite (`sytue-gio`, migration 0070, with its audience in
   `firstPartyAudiences` —without that all services answer 401, which is what happened to
   Photos—); ONE canonical host, because the `__Host-` cookie is for a single host and the `redirect_uri`
   is literal (the aliases get a 308 towards it); the services do not accept any new origin,
   because the browser only talks to `gioweb`; and `sytue.com` is already in the inventory.
   **While the port is not complete it lives at `gio.sytue.eu`**, which comes in through the front node's
   wildcard without touching anything, and `sytue.com` keeps its 301. The handover is changing `GIO_HOST` and the
   front node: [runbook](operations.md#gio-el-segundo-frente).

This **replaces** what was decided on 2026-09-13 (PR #429), which understood “native” as what surrounds
the window of a web app: with Gio the entire interface is native.

**Consequences.** `apps/gio/` is a separate module, inside osv-scanner and govulncheck. Nothing in
the current web changes because of this, except the defects the port uncovers in it, which are also fixed
there.

**What is left of phase 1.** The formula engine: 11,400 lines and about 360 functions, with 1,877
assertions in its tests. Porting it “almost the same” is worse than not porting it —a spreadsheet that
diverges in the fifteenth decimal place does not warn you—, so it goes with the same method as `Intl`: vectors
generated by the TypeScript engine and the Go port compared against them, file by file.
Also date formatting (there is no date CLDR in `x/text`) and Inter for Greek and Bulgarian.

**Ruled out.** _bbolt for the offline store_ (it locks its file to one process and the suite
is several binaries at once). _A typed mirror of the colors_ (that is what `tokens.ts` and
Fyne's `theme.go` already are, and both have to be remembered to be touched). _`x/text/message` for
numbers_ (CLDR 32). _Storing the access token_ (it lasts minutes and with it the entry does not fit in the
Windows Credential Manager).

<a id="0046"></a>

<a id="0046--los-medios-de-rooms-no-se-escriben-en-go-el-transporte-sí-los-códecs-y-la-captura-no"></a>

## 0046 — Rooms media is not written in Go: transport yes, codecs and capture no

`accepted` · 2026-09-21 · rooms, gio, mobile, desktop, media · bounds [0045](#0045), does not
change it · guardian in `apps/gio/internal/medios/vp8_test.go`

**Context.** [0045](#0045) mandates that the interface be Gio in all three places, without a webview. Today
Rooms video calls are done by the browser: `apps/mobile/rooms` (Capacitor) and
`apps/desktop/rooms` (Wails) have **not a single line** of `livekit|webrtc|getUserMedia|RTCPeer`;
what talks to the SFU is `livekit-client` inside the webview. It was requested to evaluate —and if it worked out,
write— a native WebRTC/LiveKit client in Go with camera, microphone and screen capture, to
remove that webview. This is what was measured before writing anything.

**What was measured** (2026-09-21, pilot: Ryzen 5 5600X, Go 1.27.1; LiveKit SFU v1.13.7, the repo's pin).

_1. The transport works, and it is cheap._ `github.com/livekit/server-sdk-go/v2` v2.18.1 on top of
`pion/webrtc` v4 connects to a real LiveKit: signaling, ICE, DTLS and SRTP, and receives the others'
tracks. With a publisher writing already-encoded VP8 samples at 30 fps, the subscriber spends:

| participants | tracks | CPU (1 core) | RSS    | received  |
| ------------ | ------ | ------------ | ------ | --------- |
| 1            | 2      | 1.1 %        | 38 MiB | 1.6 Mbps  |
| 4            | 8      | 3.4 %        | 39 MiB | 6.4 Mbps  |
| 9            | 18     | 8.6 %        | 41 MiB | 14.5 Mbps |
| 16           | 32     | 14.6 %       | 42 MiB | 25.7 Mbps |

Linear, ~0.9 % of a core per participant and flat memory. Publishing costs ~1.6 % CPU and
40-60 MiB per room connection. The minimal binary —no interface and no capture— is **26.5 MiB**
with `-s -w`, and pulls in **193 modules**. It compiles with CGO disabled for Linux, macOS and Windows;
**Android needs `-ldflags=-checklinkname=0`** because `wlynxg/anet` v0.0.5 (via `pion/ice`)
links via `//go:linkname` to `net.zoneCache`, which Go 1.27 no longer allows; `js/wasm` does not compile
(`e2ee` package).

_2. There is not a single pure-Go video codec, not even for decoding._ `golang.org/x/image/vp8` is
the only VP8 decoder in Go, and **it rejects every frame that is not a keyframe**: “Golden / AltRef
frames are not implemented”, with the upstream comment “only used for video, not still
images”. It is the WebP decoder. In a video call one in sixty frames is a
keyframe, so it decodes nothing. `pion/opus` v0.1.0 only decodes: there is no Opus encoder
either. Pion itself says so: it carries packetizers, not codecs, and its `getUserMedia` “requires Cgo”.

_3. With cgo there is performance to spare; the problem is not speed._ libvpx decodes 720p in
**0.74 ms** per frame and 360p in **0.23 ms** (one thread). Nine participants at 360p30 are
6.2 % of a core. What is missing is not power: it is that this code is in C and has to be cross-compiled for
five platforms.

_4. Capture is not written for the platforms that matter._ Gio v0.10.2 **has nothing for
media** —no audio, no camera, no screen— although it does expose the JNI bridge on Android.
`pion/mediadevices` v0.10.0 covers camera on Linux/macOS/Windows, microphone via miniaudio and
screen **only on X11**; all with cgo, and **without a single Android or iOS driver** (with
`GOOS=android` it mistakenly selects the X11 file and does not compile). What would have to be written:

|               | Android         | iOS          | macOS            | Linux           | Windows       |
| ------------- | --------------- | ------------ | ---------------- | --------------- | ------------- |
| camera        | Camera2 / JNI   | AVFoundation | mediadevices     | v4l2            | mediadevices  |
| microphone    | AudioRecord     | AudioUnit    | miniaudio        | miniaudio       | miniaudio     |
| speaker       | AAudio          | AudioUnit    | to be written    | to be written   | to be written |
| screen        | MediaProjection | ReplayKit    | ScreenCaptureKit | PipeWire+portal | DXGI          |
| encode V      | MediaCodec      | VideoToolbox | libvpx           | libvpx          | libvpx        |
| decode V      | MediaCodec      | VideoToolbox | libvpx           | libvpx          | libvpx        |
| Opus          | libopus         | libopus      | libopus          | libopus         | libopus       |
| echo (AEC/NS) | libwebrtc APM   | same         | same             | same            | same          |

Forty cells; `mediadevices` covers eight, and none of the sixteen for mobile. The echo
row is the one most often forgotten: without echo cancellation a speakerphone call is unusable, and the
only serious implementation is libwebrtc's APM, which is C++.

_5. Nobody does it this way, not even LiveKit._ Its Rust client SDK does not implement media: it depends on
`libwebrtc` (via `webrtc-sys`), and the Flutter, Swift and Kotlin ones sit on top of it too. The
Go one is called **server**-sdk-go precisely because it is intended for server agents, which do not
capture anything.

**Decision.** **A media client is not written in Go.** The brief was “evaluate and, if the
measurement supports it, implement”; it does not support it. We stop here, with the measurement written down, and
Rooms keeps making calls as it does today. This **bounds** [0045](#0045): the Rooms interface
is ported to Gio like the rest of the suite; the **call** is not, and it is nobody's debt.

**If it ever has to be done, the path is not pure Go.** It is cgo against libwebrtc —via
`livekit-ffi`, which is what the Rust SDK already uses—, accepting what that brings: a native
library per architecture, a cross-compilation toolchain per platform, and Android's Java classes
coexisting with the Gio activity. It is a brief of its own, not a finishing touch to this one.

**And the condition any future implementation has to meet.** Camera,
microphone or screen capture **only with continuous and visible consent**, permission requested at the moment
of use and not before, explaining function, data, duration and impact, and refusal degrading
safely —joining the call without a camera, not being left out—. It is what the web's pre-join
screen (`PreJoin`) already does: it publishes **exactly** what the user chose, instead of
calling `enableCameraAndMicrophone()`.

**When to reconsider it, and who gives notice.** `apps/gio/internal/medios/vp8_test.go` decodes a
real VP8 interframe with `golang.org/x/image/vp8` and checks that it fails for the measured reason.
The day upstream implements Golden/AltRef, that test goes red: it is the signal to reopen
this, not to delete the test. The other thing that would reopen it is a pure-Go Opus or VP8/AV1 encoder
appearing, or Android/iOS drivers in `pion/mediadevices`.

**Ruled out.** _Implementing reception only_ (without a decoder nothing is painted either, and half a
call is not a call). _`pion/mediadevices` as is_ (it has no mobile, and its screen is
X11: not Wayland, not macOS, not Windows). _Keeping the webview inside Gio_ (the brief
forbids it, and rightly so: it would be the third interface).

**Amendment of 2026-09-25 — it is reopened, and through the door this ADR itself left open.** The owner
decided to bring audio and video to the native Rooms app (now Fyne, [0048](#0048)) on desktop and
on mobile. Two of the “when to reconsider it” conditions are already met: there is a **pure-Go Opus
encoder** (`github.com/tphakala/go-opus` v1.1.0, BSD-3, a transliteration of libopus;
it encodes only CELT, which at 32 kbps mono is enough for voice, and decodes all three modes) and **pure-Go VP8
with interframes** (`github.com/thesyncim/govpx`, BSD-3, encoder and decoder;
by its own measurement, 1.8 ms per 720p frame when decoding). The codec row stops being C.

What remains written as follows:

- **Transport**: `livekit/server-sdk-go/v2` v2.18.1 on top of `pion/webrtc` v4.2.21, as measured
  above. `livekit/protocol` stays at v1.49.0 because v1.52 breaks the SDK build.
  Android still requires `-ldflags=-checklinkname=0` (anet); `empaquetar.sh` sets it via GOFLAGS.
- **Microphone and speaker**: miniaudio via `gen2brain/malgo` (cgo with no libraries to link),
  which covers all five platforms —CoreAudio, WASAPI, PulseAudio/ALSA, AAudio/OpenSL ES—. It is the
  only piece with C, and it is kept separate (`apps/fyne/llamada/altavoz`) so that the rest can be tested without a
  sound card.
- **The token and the SFU are the web's**: the service's `POST /rooms/{id}/join` (and
  `from-source`/`by-source` + `knock` for channels), without its own endpoint. Media goes to the suite's SFU,
  not to third parties.
- **Guardian**: `apps/fyne/llamada/sfu_test.go` brings up the same `livekit-server` that is deployed
  and checks that one person's voice reaches the other as a tone, that their camera arrives as an image
  and that hanging up frees the slot immediately. It was seen red by removing each of the three things.

**Second amendment, 2026-09-26: capture too.** The camera and the screen are read with each
system's API (AVFoundation and ScreenCaptureKit, Media Foundation and GDI, V4L2 and X11,
Camera2), in `apps/fyne/llamada/camara`, and echo is removed by the system: VoiceProcessingIO on macOS
and iOS and the voice capture DSP on Windows. It is the first row of the table above that gets
written; the mobile screen one is still missing. What has been tested and where, in the roadmap.

**What is still missing, and why** (the details, in the [roadmap](./roadmap.md#llamadas-nativas)):
**camera and screen capture** —the “camera” and “screen” rows of the table are still
per platform and have no Go code—; **echo cancellation** on desktop —the system's is requested
where possible (AAudio “voice communication”, iOS “play and record” category), and
on desktop there is none: with speakers, use headphones—. Receiving the others' camera and screen
does work, because it only requires decoding.

**The consent condition still stands**: the waiting room publishes only what was chosen, the
microphone permission is requested when opening it (Android) or when opening capture (macOS and iOS, with
`NSMicrophoneUsageDescription`), and without permission you join to listen. The `vp8_test.go` test that was
cited above went away with Gio; its role is now played by `sfu_test.go`.

---

<a id="0047"></a>

<a id="0047--los-avisos-de-android-salen-del-sse-del-hub-y-el-empaquetador-va-parcheado-hasta-que-gogio-lo-acepte"></a>

## 0047 — Android notifications come from the Hub SSE, and the packager is patched until gogio accepts it

`accepted` · 2026-09-21 · mobile, gio, notifications, packaging, sovereignty · **implements
[0039](#0039)** for Android

**Context.** [0039](#0039) already decided where a notification comes in on Android: the **Hub SSE
stream** that the web consumes with `EventSource`, sustained by a **foreground service**.
No FCM, no Play Services and no identifier outside the EU. What was missing was being able to
do it, and what prevented it was not our code but **gogio**: it writes the
`AndroidManifest.xml` itself from a closed template where there is room for neither `POST_NOTIFICATIONS`
—mandatory since `targetSdk 33`, and the APKs ship with 35— nor a `<service>`.

Meanwhile, `@capacitor/push-notifications` in `apps/mobile/rooms` was **the only place in
the whole suite where a message notifies with the app closed**, and that is why that wrapper could not be
turned off.

**Decision 1: a patch, not a fork, and with generic slots.** `gogio-avisos.patch` is **twelve
lines** spread across three gogio files, and it does not add a flag per case but **two
slots**: `-permissions` (extra permissions as is) and `-appmanifest` (literal XML inside
`<application>`). That way gogio does not need another flag next time, which is how this
can be sent upstream instead of being dragged along.

And **there is no fork to maintain**: nothing to clone or host. `gogio-sytue.sh` takes from the
module cache the EXACT version pinned by `versions.env`, applies the patch to it in a temporary directory and
compiles. Upgrading is changing that number; if the patch stops applying, the script **fails**
and that is precisely the warning that gogio has touched that area. The day they accept it, the
script and the `.patch` are deleted and we go back to plain `gogio`.

**Dropping to `-targetsdk 32`** was rejected, which would unblock it today without touching anything: the Play Store requires
≥ 34, so it would be trading a visible blocker for one that shows up on publishing day.

**Decision 2: the native layer is presentation only, and it is measured.** The owner's criterion is to limit
platform-specific code as much as possible. Here that is split like this:

- **In Go, once for the fourteen apps and the three platforms** (`apps/gio/apps/avisos`): the
  SSE client and its reconnection with increasing backoff, what deserves an interruption, discarding
  duplicates, the texts —the Hub's, already translated—, where each notification leads and the
  badge number.
- **In Java, forty lines without a single decision** (`internal/sistema/Avisos.java`):
  “show this”, “set this number”, and the foreground service wrapper. It receives the
  permanent notification already built, as a `Parcelable` in the `Intent`.

The proof that it is well split is that **the iOS layer can be just as thin**:
`sistema.Avisar`, `Callar` and `PedirAvisos` are the same three functions, with
`UNUserNotificationCenter` behind them and without repeating any rule.

**Decision 3: `specialUse` and not `dataSync`.** Since Android 15 a foreground service of
type `dataSync` has a **cap of six hours a day** —the system calls `onTimeout()`—, and a
notification connection that shuts down in the middle of the afternoon is not a notification connection. `specialUse` has no
cap; in exchange Google Play asks for a justification, which is declared in the manifest itself
(`PROPERTY_SPECIAL_USE_FGS_SUBTYPE`) and is the real one.

**Permissions, of which there are three and only three.** `POST_NOTIFICATIONS`, `FOREGROUND_SERVICE` and
`FOREGROUND_SERVICE_SPECIAL_USE`, justified one by one —function, data, duration, impact and
alternative, as brief v4 §5 requires— in `permisosDelManifiesto`. The notification one **is requested in
context**: when opening the bell for the first time, not at startup; and if the person says no, the
bell keeps counting and marking.

**What is verified and what is not, stated as it is.** The APK is built on the pilot and
**verified in the artifact** with `aapt2`: the manifest declares the three permissions and no
others, the `<service eu.sytue.gio.Avisos>` with `foregroundServiceType` `specialUse`, and the class is
in `classes.dex`. **It has not been seen running on a phone**: there is no device or emulator (the
pilot has no `/dev/kvm`). The guardians cover what can be seen going red without a phone
—that the service name and the JNI signature do not diverge between the XML, the Java and the Go, and that the
permissions do not grow— because those three loose strings are what the compiler does not relate and what
would fail only on the device.

**Ruled out.** _A versioned `.jar` in the repository_ (`empaquetar-movil.sh` compiles the `.java`
with the `javac` that gogio already requires, and gogio picks up the `*.jar` from each Go package's directory:
one less slot to patch). _Patching gogio to compile our `.java` files_ (for the same reason:
it is solved from our side). _`START_STICKY` on the service_ (reviving it without the Go that
feeds it is a permanent notification in front of a connection that does not exist). _Requesting
`POST_NOTIFICATIONS` at startup_ (the context-free dialog that v4 §5 does not want).

**Addendum (2026-09-27, Fyne).** With Gio retired, the service is
`eu.sytue.nativo.Avisos` and only the Hub's manifest declares it (`empaquetar.sh`), which notifies
for all thirteen. The Fyne CLI does not know how to write `foregroundServiceType` either: it is added by
`android-binario.py` to the compiled manifest and `verificar_aab` requires it in the AAB. The
Hub's permissions are those three plus `USE_BIOMETRIC`, which all thirteen carry for BiometricPrompt
(the session lock and the Notepads notebooks).

<a id="0048"></a>

<a id="0048--la-interfaz-nativa-se-rehace-en-fyne-con-widgets-propios-y-lo-que-no-es-del-toolkit-sale-a-appsnucleo"></a>

## 0048 — The native interface is rebuilt in Fyne with our own widgets, and what is not part of the toolkit moves to `apps/nucleo`

`accepted` · 2026-09-21 · desktop, mobile, fyne, parity · supersedes [0045](#0045) on desktop and mobile; the web stays with [0004](#0004) · code in `apps/fyne/` and `apps/nucleo/`

**Context.** [0045](#0045) decided on a single Gio interface for web, desktop and mobile.
A month later the owner changes the destination: **Fyne v2.8.1** (latest stable, 2026-08-26) for
**desktop and mobile**, with **our own theme, widgets and layout** until reaching **1:1 parity at the
pixel level** with the web apps. The web does **not** come in: `sytue.eu` stays Astro + Preact

- Go, and therefore **there is no WebAssembly target to sustain**, which was one of the costs of

45.

It is the second toolkit change in a month (Fyne → Gio → Fyne). That, and not the choice itself, is
the fact that governs this decision.

**Decision.**

- **What is not part of the toolkit moves out first, to a module with a neutral name.** `apps/nucleo`
  gathers the ~19,300 lines that do not touch any toolkit: the HTTP/WS client for the services
  (`api`), the language catalog (`i18n`), OIDC session and keychain (`auth`), system
  capabilities (`sistema`), file dialogs (`ficheros`), formula engine (`formula`), HTML to
  text (`htmltext`), offline queue (`offline`) and `webref`. The **color
  tokens** also go —the `colors.css` stylesheet and the three hundred lines that interpret it— and the **five
  Figtree .ttf files**. They were under `apps/gio/internal/`, where Fyne cannot import them; the
  alternative was copying them, and this repository already knows what that costs
  ([0037](#0037)). The name does not carry the toolkit **on purpose**.
- **The interface is composed from primitives, not configured.** `apps/fyne/ui` does not use a single
  stock Fyne widget: each control is a `fyne.Widget` shell with its own
  `WidgetRenderer`, and inside only `canvas.Rectangle`, `canvas.Text`, `canvas.Image` and
  other widgets of ours. The layouts (`ui.Fila`, `ui.Columna`) are also our own, because the
  stock ones use `theme.Padding()` as the gap and that number is moved by the system's font size
  setting. The complete pattern, with its six rules and the measured why of each, is in
  the package comment of `apps/fyne/ui/ui.go`.
- **The Fyne theme exists for two things and nothing more**: the FONT with which
  `canvas.Text` rasterizes and the canvas background. Widget colors are requested by token
  (`tema.Color("border-strong", v)`), never through `fyne.Theme`: its map has forty names and
  the stylesheet a hundred and fifty-odd, and moreover `theme.Size()` is multiplied by the user's font
  scale, with which the geometry would stop being ours.
- **Gio is not deleted.** `sytue.com` serves the Gio suite today and cannot be left with nothing. The
  Gio port is **cancelled** —no apps are added— but the `apps/gio` tree stays
  compiling and deployable until the owner decides what that domain serves.
  [0045](#0045) is superseded on desktop and mobile, and suspended for the web.

**What is kept and does not have to be figured out again.** The numbers measured against production for the
six audited apps (Hub, Mail, Drive, Calendar, Notepads, Contacts), the 120 dialogs with their
size, and the inspector's color vectors. **They do not depend on the toolkit** and they are the most
expensive asset in this work.

**How far the pixel goes, measured on 2026-09-21 against Fyne v2.8.1.** This is what has to be
known before porting sixteen apps, and it is not repeated in each PR:

1. **Color: exact.** Fyne composites alpha in **sRGB**, like the browser —the bug Gio had,
   compositing in linear and drawing all borders at half strength, **does not exist** here—.
   What remains is the quantization of alpha to one byte: `rgba(ink, .12)` over white gives (228,231,232)
   versus production's (227,230,232). **±1 level out of 255**, so a screenshot gate
   needs that tolerance; byte-for-byte will never happen. Borders are stored flattened
   and opaque in `apps/nucleo/tokens` and then the pixel is exact.
2. **Geometry: exact from ×2 for half pixels.** Fyne quantizes each coordinate
   to the device pixel and **does not antialias straight edges** (rounded corners it does).
   At ×1 an `h-9` (31.5) takes 32 rows; at ×2 it is exactly 63. QUARTER pixels —19.25 of
   line height, 15.75— are not exact even at ×2: ×4 would be needed. The error is always positive,
   so **it accumulates when stacking boxes**. That is why the scale is declared with its decimals and
   **it is not rounded in our code**: rounding freezes at ×1 an error that the screen
   fixes on its own.
3. **The border cannot be `StrokeWidth`.** Fyne paints it centered on the outline: at ×1 it spreads
   its coverage over two rows and comes out at half strength. `ui.Caja` paints it as two filled
   rectangles, which works at any scale and also gives the stylesheet's `box-sizing: border-box`.
4. **Typography: it was the only ceiling that did not depend on us, and it has been removed.** See
   [“The shaper is patched”](#0048-texto) below. `github.com/go-text/typesetting`
   scaled the font to `ceil(size)` before placing a glyph, so with the 14 rem
   —seven of the ten steps are fractional— `text-base` (12.25) was shaped as 13:
   **+6.12 %**. Today the width of a text is within **≤0.17 px** of what the browser gives.
5. **The line box has to be imposed.** `canvas.Text.MinSize().Height` returns the box of
   the FONT, 1.2 em: for `text-base` that is 14.70, not the CSS box of 19.25 that is the one that takes
   space in the web layout. (Before the shaper patch it gave 15.59, because that 1.2 em
   came from the ppem rounded to 13; today it is exactly 1.2 × 12.25, but it is still not the CSS
   box.) `ui.Medir` returns the CSS box and `ui.ColocarTexto` centers the glyph within it.

**Consequence, and it is the underlying decision:** **the achievable parity is one device
pixel in geometry and color, and two tenths of a pixel in the width of a text**. Both
are as good as the browser itself.

<a id="0048-texto"></a>

<a id="el-conformador-va-parcheado-ppem-exacto-y-no-ceil-2026-09-21"></a>

### The shaper is patched: exact ppem and not `ceil` (2026-09-21)

**What was wrong.** `github.com/go-text/typesetting` scaled the font to `ceil(size)`
before asking harfbuzz for the advances (`shaping/shaping.go`), so **no fractional
size existed**: the next integer was shaped. With `1 rem = 14 px` almost the whole
scale of the suite is fractional and **every label of the native interface came out wider**.
Measured on the pilot with the software painter, `Guardar cambios` in Figtree Regular:

| step        | CSS px | before | after  | error it carried              |
| ----------- | ------ | ------ | ------ | ----------------------------- |
| `text-2xs`  | 9.625  | 76.08  | 73.25  | +3.86 %                       |
| `text-xs`   | 10.5   | 83.75  | 79.97  | +4.73 %                       |
| `text-sm`   | 11.375 | 91.39  | 86.59  | +5.54 %                       |
| `text-base` | 12.25  | 98.97  | 93.25  | **+6.13 %**                   |
| root        | 14     | 106.59 | 106.59 | 0 (integer: never touched it) |
| `text-lg`   | 15.75  | 121.78 | 119.92 | +1.55 %                       |
| `text-xl`   | 19.25  | 152.25 | 146.52 | +3.91 %                       |
| `text-3xl`  | 31.5   | 243.58 | 239.78 | +1.58 %                       |

**And a HiDPI screen did not fix it**, as the ADR and the three notes in the code said until
this date. Fyne shapes in **CSS px** and multiplies by the canvas scale **afterwards**
(`internal/painter/font.go`), so the measured ink extent shrinks the same at ×1
(−6.06 %) as at ×2 (−5.61 %). The belief that the error “is halved at ×2” was false and
was written in four places.

**Decision: it is applied, with a hosted fork.** One line:
`font.XScale = int32(input.Size) << (scaleShift - 6)`. `input.Size` is already a `fixed.Int26_6`
—the size times 64— and harfbuzz scales by `XScale/upem`, so passing it raw requests the exact
size without losing the 1/64 px precision that the shift by `scaleShift` exists to
preserve.

**Against the browser, which is the reference**: 54 measurements (six real labels × nine steps,
Figtree Regular) against `getBoundingClientRect()` of production's variable Figtree, with
Chromium hinting turned off. Mean error **from +3.30 % to +0.03 %**; the worst case, **from 8.33 px to
0.17 px**.

**Why a fork and not a `.patch` like gogio's ([0047](#0047)).** gogio is a
**tool** invoked by a script of ours, and that script can patch a temporary copy.
`typesetting` is a **library** that goes into every `go build`, every `go test` and every IDE: there
is no single place everything goes through. A `replace` pointing to a generated directory would leave a fresh
clone not compiling until someone ran a `make`. So there is a fork, and the price
is paid in the policy below.

**Fork update policy.**

- It lives at `github.com/thinkddr/typesetting`. The **`main`** branch is the untouched mirror of
  upstream and **`sytue`** is `v0.3.4` + the patch. The `replace` points to a **tag**
  (`v0.3.4-sytue.1`), never to a branch.
- **Rebasing** = bringing upstream into `main`, `git rebase <tag nuevo> sytue`, tag `vX.Y.Z-sytue.1` and
  changing the `replace`. The diff against upstream is **two files and 27 lines** —the patch and the
  `module`, which changes because a `replace` requires the target to declare the path by which it is
  required—, so a rebase that does not apply cleanly is the signal that upstream has touched that
  area, not a conflict to resolve by eye.
- **Who checks it:** whoever upgrades `fyne.io/fyne/v2`. Fyne pins the `typesetting` version in
  its go.mod; if it goes up, the fork is rebased onto that same version in the same PR. The
  version gate in `AGENTS.md` §1 already forces looking at it.
- **What gives warning:** `ui.TestElConformadorNoRedondea`, which compares the shaped width against the
  proportion from a 64 px pattern and fails if someone removes the `replace` or if a rebase
  loses the patch. Seen red in both directions on 2026-09-21.
- **Upstream:** the patch is written to be sent to `go-text/typesetting` as is, with
  its comment in English. As long as they do not accept it, the fork is rebased; the day they do, the
  `replace` and the fork are deleted.

**Sovereignty: the fork is PRIVATE on purpose.** Publishing it would send its path to `proxy.golang.org`
and to `sum.golang.org` as soon as someone resolved it, which is exactly the leak that
`compliance/supply-chain/README.md` §A considers closed for the `git.sytue.eu/...` modules. Since
`git.gay` is already inventoried in `providers.tsv` as a temporary exception until 2026-12-31,
**no new row in `blocked-baseline.tsv` is needed**. The price is that whoever compiles
`apps/fyne` needs `GOPRIVATE=git.gay/*` and git.gay credentials —the pilot and CI already
have them—, and without that `go mod download` fails at the proxy, not silently.

<a id="url-entrante-fork-temporal-de-fyne-v281-2026-09-22"></a>

### Incoming URL: temporary fork of Fyne v2.8.1 (2026-09-22)

Fyne v2.8.1 only exposed `App.OpenURL`, which opens a URL **outward**. There was no
contract or bridge for the URL the system delivers when returning from the browser, opening
another app by its scheme or starting a closed application. That made it impossible
to finish the RFC 8252 mobile login even though `auth.OIDC.LoginEsquema` and `Vuelta` already
existed: the callback had nowhere to come in.

The public dependency `fyne.io/fyne/v2` is kept, at the stable version declared in `apps/fyne/go.mod`, but it is replaced by
the public tag of `github.com/thinkddr/fyne/v2` pinned in `apps/fyne/go.mod` (the history is in `apps/fyne/upstream/README.md`). It keeps upstream's BSD-3-Clause license and the fork's modifications are distributed under Apache-2.0. It adds the
optional `fyne.URLHandler` interface, without widening `fyne.App` or breaking third-party
implementations. The URL is queued until the handler is registered and is delivered on the UI thread.
Android covers the initial intent and `onNewIntent`; iOS, the launch URL and
`application:openURL:options:`; macOS, the `kAEGetURL` Apple Event; Linux and Windows,
the argument the binary starts with.

The original upstream tags are preserved; the fork's `main` contains Sytue's modifications.
When updating Fyne: integrate the new stable tag, run the fork's
tests in CI, create `vX.Y.Z-sytue.N` and change the `replace` in the same change. The
patch will be proposed upstream; when they accept it, the fork and the
`replace` are removed, a parallel API is not maintained.

**And a third fork, of GLFW (2026-09-23).** `apps/fyne/go.mod` also replaces
`github.com/go-gl/glfw/v3.4/glfw` with the tag of `github.com/thinkddr/glfw/v3.4/glfw` pinned in `apps/fyne/go.mod`
(private, under upstream's licenses: BSD-3-Clause for the Go binding and zlib for GLFW), on top of
the go-gl base required by the current Fyne version. GLFW only accepts dropped files, and
its X11 XDnD and its Wayland `wl_data_device` are not visible from outside: the patch adds
dropped text and links (`glfwSetDropTextCallback`, `apps/fyne/soltar.AlSoltar`) and,
since `.3`, notification of files dragged over (`glfwSetDragCallback`,
`soltar.AlArrastrar`). The same rules as the Fyne one: a branch with the patch,
a tag per version and the `replace` in the same change; what each
version carries, how it was tested and how to remove it, in `apps/fyne/upstream/README.md`.

**The upstream submission is prepared (2026-09-22)**, in
[`apps/fyne/upstream/`](../apps/fyne/upstream/README.md): the exported patch (13
files, 235 lines added and none deleted, with its test) and the PR text in
English. It remains to open it, and that requires the owner's personal GitHub account. The live issue
requesting it is `fyne-io/fyne#1847`; there is no open PR that solves it.

**And the other end: who RECEIVES the link.** The sender (`apps/nucleo/api/abrir.go`)
had everything and there was no receiver: the link would have arrived at a blank window.
`api.LeerApertura` takes the link apart —it is the inverse of `RutaDeApertura`, tied to it by a
round trip— and `sesion.Suite.AlAbrirFichero` delivers it already taken apart to the app that owns the
format, which loads it in the background. If it cannot —no session, the file is no longer there,
a format it does not understand— the link **goes on to the same page on the web**, which is
what Drive already did with the app not installed: never a blank window.

Docus, Calcus and Diapos open the file as `/open-from-drive` opens it, but WITHOUT creating a
document (`api.ImportarDeDrive`): it is fetched from Drive by id, `POST /api/v1/import`
converts it to the web's model —HTML, the sheet's JSON or the presentation's— and it is
shown **read-only**, because it is a conversion and not a Docs document, and whatever
was written on top would have nowhere to be saved. Photos does what `/abrir` does: it indexes (it is
idempotent and copies nothing) and shows the large variant in the viewer. CertiPDF keeps
the web fallback until its screen talks to the service.

While wiring it up, two things came out that were not part of the fork and that would have made none of
this work: the schemes of those four binaries were named after their **brand**
(`sytue-docus`) and not after the **product key** (`sytue-docs`), which is what the
other apps emit, what `PRODUCT_SCHEMES` declares on the web and what Sytue ID has
registered (migration 0071) — the symptom would have been “the native app is not installed”;
and on mobile the session requested the token with the desktop's `client_id`. The two
guardians are in `apps/fyne/sesion`.

**An open app receives the link; another one is not opened.** On macOS, iOS and Android the
system does it —on Android, with `launchMode="singleTask"`, which `empaquetar.sh` already sets: without it
the return from the browser creates another activity and `onNewIntent` is not called—. On Linux and
Windows opening a scheme means running the program again, so `sesion` opens a
local socket per application and the second copy passes the URL to the first and exits. And
the link that STARTS the app is held until `main` registers its receiver: Fyne
dispatches it inside `sesion.Abrir`, before anyone exists to give it to.

**Hinting does not move, and it has been checked.** `go-text/render` scales the glyph outline by the
size in **floating point** and rasterizes it with `rasterx`, which **does not snap to the grid**: the shaper's
ppem only governs the advances. Also checked by eye, comparing the software painter's PNGs
at ×1 and ×2 with and without the patch: same stroke weight, same
antialiasing, only tighter tracking.

**And the web's variable Figtree versus the native static instances: they are the same
font.** It was the remaining open risk —the web mounts `@fontsource-variable/figtree@5.3.0` and
`apps/nucleo/fuentes` embeds five static .ttf files— and nobody had measured it. Measured now, at
**640 px** so that neither the browser's rounding nor harfbuzz's counts, over 32
combinations of eight strings by the four weights (400/500/600/700): the maximum deviation is
**0.0098 %**, that is **0.7 px over 7,098**. It is noise. **There is no need to embed the variable font**, and the
difference seen at small sizes was the quantization of advances to whole pixels that
Chromium does on Linux, not the font.

**Accepted cost.**

- **The presentation layer is rebuilt for the third time.** What survives is
  `apps/nucleo` and the measured numbers; what is rebuilt are the ~4,400 lines of
  `apps/gio/internal/ui` and each app's screens.
- **`gioui.org` stays in the core's go.mod** because of two `//go:build android` files (the
  JNI bridge that requires `app.JavaVM()` and `app.AppContext()`). It is not compiled for any desktop
  target. It is removed with about twenty lines the day Android is packaged in Fyne, which is
  when there will be something to test it with.
- **The pixel guardians run at ×2**, which is not necessarily the scale at which it is deployed:
  `SetScale` only exists on the software canvas; in the real binary the scale
  is decided by the screen and `FYNE_SCALE`. At ×2 the widget is measured; at ×1 the rounding would be measured.

**Ruled out.** _Staying with Gio_ (the owner's decision; and the text `ceil` is the same, so
nothing would have been gained in the ceiling that matters). _Configuring Fyne's stock widgets_ (its
theme scales the geometry with the system font: parity ends before it begins).
_Copying the eight packages into `apps/fyne`_ ([0037](#0037), and we already know how that ends). _Rounding the
scale to integers so that it fits at ×1_ (freezes the error where the screen would fix it).
<a id="0049"></a>

<a id="0049--sytuecom-sirve-la-suite-astro-con-un-segundo-juego-de-imágenes-no-la-de-gio"></a>

## 0049 — `sytue.com` serves the Astro suite with a second set of images, not the Gio one

`accepted` · 2026-09-21 · frontend, deployment, oidc · supersedes [0045](#0045) on the web · code in `deploy/sytue-com/`

**Context.** [0045](#0045) split the two domains: `sytue.com` would publish the Gio suite
(WebAssembly) and `sytue.eu` would stay with Astro + Preact. The Gio port **is cancelled** —the
desktop and mobile interface is migrated to Fyne and the web remains Astro + Preact + Go—, so
`sytue.com` is left without a destination. The owner's decision is that it **serves the suite**, not that it redirects
to `sytue.eu`.

**The trap, and it is the center of everything.** The Astro frontends **bake the public URLs in at
build time**: `scripts/publicar-imagen-frontend.sh` passes them `PUBLIC_OIDC_ISSUER`, `PUBLIC_HUB_URL`,
`PUBLIC_SYTUE_DRIVE_BASE_URL` and three more, all derived from `DOMINIO`. Serving those same
images on `sytue.com` gives a suite that **is seen on `.com` and talks to `.eu`**: login and cookies
crossing domains.

**Decision.** The owner made it on 2026-09-21: **a second set of the thirteen Astro images
built with `DOMINIO=sytue.com` and deployed on the front node** (`sytue-rooms-media`), not on the
pilot. **The two fronts share Sytue ID, services and database**; the only thing duplicated
is the frontend.

**What it took to measure, and what the measurement changed.**

- **Baking hardly exists.** Of the suite's ~30 reads of `PUBLIC_*`, **all but
  three** go through `serverEnv()`, which merges the build environment with `process.env` **letting
  the latter win**; the chart already injects each one per container. The three that do not:
  `apps/www/src/lib/downloads.ts` (SSG, and they are not suite domains),
  `packages/config/src/health-endpoint.ts` (version and commit, which ARE the image's) and
  `packages/config/src/tokens.ts`. That last one is the only one that ties the image to a domain: from it
  comes `suiteApex` and with it the links between apps. In other words, the second set exists because of **one
  file**.
- **`PUBLIC_OIDC_ISSUER` and `SYTUE_OIDC_ISSUER` are DIFFERENT variables**, and from that comes the piece
  that makes all this viable. The first is baked in and only feeds `suiteApex`; the second is a
  runtime one and is what the BFF uses. So the `.com` front bakes its links on `.com` and at the
  same time signs in against the **single** Sytue ID, `id.sytue.eu`. Verified on 2026-09-21 with the
  real image: `GET /one/auth/login` with `Host: one.sytue.com` responds `302` to
  `https://id.sytue.eu/oauth2/authorize?…&redirect_uri=https%3A%2F%2Fone.sytue.com%2Fone%2Fauth%2Fcallback`.
- **An IdP issues one `iss`.** There is no “`.com` Sytue ID”: whoever comes in through `.com` signs in at
  `id.sytue.eu` and comes back. `id.sytue.com` and `hub.sytue.com` do exist as hosts —the baked
  `appDomains` names them— and are routed to the pilot by rewriting the `Host`.
- **Cookies need nothing.** They are `__Host-`: host-only and without `Domain`, and
  `resolveOidcConfig` **throws** if someone defines `SYTUE_COOKIE_DOMAIN`. Each host has its own,
  so `.com` does not depend on a `.eu` one being valid. The consequence, written down so that nobody
  discovers it on the fly: **the session is not shared between the two fronts**; signing in on `.com`
  is a login of its own that the IdP's SSO resolves without asking for the password again.
- **The cost of the second set is much lower than feared.** In the registry, the second variant
  of an app adds **~7 MB** and not 647: all layers except `dist` are the same (measured on
  the pilot with two builds of the portal). On the runner, the `.com` set is the **native** one —the
  runners are ARM64 and so is the front node—, so it is built **without** `bun install --cpu` or
  `crane rebase`, the opposite of the `.eu` one, which is the cross-compiled one. Reference measured in v3.3.9: 13
  frontends in 30 min 49 s of runner time, 2 min 22 s each, 79 % of the total publication.
- **And the front node can take it.** An SSR container uses 19-46 MiB at rest and 64 MiB after 260
  requests; the fourteen in production add up to 552 MiB and 17 millicores among all of them. Thirteen more fit
  in the front node's 21 GB free without getting close to Pangolin, the SFU or the three runners.

**The cost that does have to be accepted: SSR latency.** The front node's BFFs cannot use
cluster names, so they call the backend via `https://<producto>.sytue.eu`. Measured from the
front node on 2026-09-21: **33 ms with a reused connection** and **116 ms cold**, of which 85 are
the TLS handshake. **There is no shortcut**: `id.sytue.eu` already resolves to that same machine (ping 0.06 ms), so
those 33 ms are the Pangolin tunnel to the pilot — the physical Paris↔Spain trip, not the
proxy. A page with **chained** calls adds them up; when any of them becomes noticeable, what has to be
removed is the chain, not the hop.

**The only thing that had to change in the code** was adding `sytue.com` to `SYTUE_ALLOWED_DOMAINS`
(`@sytue/config/astro-security`). Without that the symptom is not a clear error: Astro stops believing
the `Host`, `Astro.url` falls back to `http://localhost:<puerto>`, the BFF jumps to the canonical host —taking
the person out of the domain— and `checkOrigin` rejects every POST, logout and session refresh included.
None of that fails at deploy time: it fails at sign-in.

**Guardians** (both seen red by hand before accepting them):

- `apps/frontend/packages/config/src/dominio-horneado.test.ts` compares **by equality** the three
  files that may read `import.meta.env.PUBLIC_*` without `serverEnv`. A fourth one stops it, because
  each new read inflates the price of having two sets of images.
- `TestRedirectUrisDeSytueComCubrenElCompose` (in `make migrate-check`) ties the
  `SYTUE_OIDC_REDIRECT_URI`s announced by `deploy/sytue-com/docker-compose.yml` to those registered in
  Sytue ID. It is the failure that does not look like a failure: the container starts, `/healthz` answers, and
  the only thing that does not work is signing in, only via `.com`.

**Power-on order, and the way back.** First migration 0094, then the containers,
and the Traefik block last: the other way round, `sytue.com` stops serving Gio before the new one
can sign in. **Gio is not turned off**; its router stays in place with the same priority, so
removing the `sytue-com` block returns the domain to Gio immediately. Details in
`deploy/sytue-com/README.md`.

**Ruled out.** _Redirecting `sytue.com` to `sytue.eu`_ (the brief says serve, not redirect).
_Resolving the public URLs at runtime from the `Host` header_: it was actually implemented and
verified —a host allowlist in the BFF and links between apps on the same origin— and it was ruled out when
the owner decided on the two-image route. It works for any future domain without publishing
anything, but it requires links between apps to be relative, and that changes the visible URLs of the whole
suite on `sytue.eu`, which is the parity reference. It remains measured in case a third domain makes it
worthwhile: with absolute links, going from Mail to Drive costs **four redirects and a full
round trip through the IdP**, because the cookies are host-only. _A single host on `.com` serving the
suite by prefixes_, like the organization host: it works, but it requires the same as above.
<a id="0050"></a>

<a id="0050--un-solo-inicio-de-sesión-para-las-trece-apps-nativas-llavero-para-el-secreto-carpeta-compartida-para-las-preferencias-servidor-para-la-presencia"></a>

## 0050 — A single sign-in for the thirteen native apps: keychain for the secret, shared folder for preferences, server for presence

`accepted` · 2026-09-22 · desktop, mobile, oidc, fyne · code in `apps/nucleo/ajustes`, `apps/nucleo/auth`, `apps/fyne/sesion`

**Context.** The installed suite is **thirteen binaries**, one per product ([0048](#0048) §separation
of apps). The owner asks for the session to be **cross-cutting**: whoever signs in to Sytue Mail does not sign in
again to Drive. And with it, the **appearance**, the **language** and the **presence**. Four things that
the person sees as one —“my account”— and thirteen processes that have to agree.

**Four things, three places.** The temptation is a single store. It is a mistake, and each one fails in
a different way:

|                             | where                         | why there and not somewhere else                                                                                                                                                         |
| --------------------------- | ----------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Session** (refresh token) | system keychain               | it is a secret: Keychain on macOS and iOS, Secret Service on Linux, Credential Manager on Windows, Keystore on Android                                                                   |
| **Appearance**              | `<config>/sytue/ajustes.json` | it is not a secret, and on macOS the keychain **may ask for authorization**: a startup that asks about the theme is absurd. And if the keychain is not there, the app has to open anyway |
| **Language**                | same                          | same                                                                                                                                                                                     |
| **Presence**                | the server (`api.Hub`)        | **others see it**. A local copy would be a second truth that goes stale as soon as it is changed from the web or from the phone                                                          |

**`fyne.Preferences` does not work, and it is not an opinion.** In `fyne.io/fyne/v2@v2.8.1`, the store is
`filepath.Join(app.RootConfigDir(), a.UniqueID())` (`app/preferences_other.go:18`): one folder
**per `ID` in `FyneApp.toml`**. Thirteen apps, thirteen `preferences.json`, thirteen themes. On Android and iOS
it is a single folder (`preferences_android.go:19`, “we are in a sandbox”), but there there is also
a single app installed, so it does not share anything between products either. And it is **plain-text JSON**, so
for the secret it is ruled out twice over.

**The shared file says WHAT has changed, it never stores what lives somewhere else.** It is the rule
that orders the split: of the session there is only a **marker** —a number in nanoseconds, which only
serves to be different from the previous one— and whoever sees it asks the keychain again. A guardian
walks the entire folder and fails if anything that smells like a credential appears
(`TestLaMarcaDeSesionNoLlevaLaSesion`).

**The inter-process notification is ONE: a two-second poll** (`ajustes.Vigilar`). Two reads of
less than a hundred bytes, no new dependency and no per-platform implementation; `inotify` /
`FSEvents` / `ReadDirectoryChangesW` would have been three. What each thing does with the notification:

- **Sign-out: immediately**, in all thirteen. It does not admit “on the next startup”.
- **Theme: live.** Changing it in Mail changes it in Drive without reopening anything.
- **Language: on the next startup.** Screens receive their translator when built, and rebuilding them
  entirely would be reopening the app from the inside. It is what any desktop does, and it is stated here
  instead of being left half-done.
- **Presence: does not go through the file.** The Hub is asked in the same heartbeat that keeps it
  alive, because **we also need to find out about what is changed from the web or the phone**, and for
  that the file is useless. Adding a marker to it would be a second mechanism for a case the
  first one already covers.

**Fyne does not let you force the theme variant, and that changes the design.**
`settings.ThemeVariant()` returns `s.variant`, which comes from the operating system, and `SetTheme` **does not
touch it** (`app/settings.go:75-86`). `theme.LightTheme()`/`DarkTheme()` would not work either: the suite's widgets
do not request color through `fyne.Theme` ([0048](#0048) §theme). The choice is resolved in
`tema.De`, through which **the 243 points in the tree** that ask for the variant already went, instead of
changing all 243.

**The OIDC flow is RFC 8252's, and it was already there.** Authorization Code + PKCE against a
**public** client —an installed binary cannot keep a secret—, with the **system** browser and
not a view inside the app (§8.1): that way the domain and the padlock are visible, and any existing session
with Sytue ID is reused. The return from the browser is the only thing that changes between platforms: on
desktop, an ephemeral server at `127.0.0.1:8921` (§7.3, `sytue-desktop` client, migration 0069);
on mobile, each app's own scheme (§7.1, `sytue-mobile` client, migration 0071, with the
fourteen schemes registered one by one). **No new migration is needed**: both clients and their
`redirect_uri`s are already registered.

**The refresh race, which was the real risk.** Sytue ID **rotates** the refresh token on
each use and, if presented with a spent one, **revokes the entire family**. With thirteen processes and a single
keychain entry, two apps that expire at the same time read the same token, both present it and the
second one locks out all thirteen. Looking at what is stored before refreshing is not enough: two must be prevented
from looking at the same time. `Compartida` wraps **read‑refresh‑store** in an **inter-process** lock.

The lock is a file created with `O_EXCL` and **not `flock`**: `O_EXCL` is the same call on all
five targets —including Windows and Android— and `flock` would have been one file per platform in
`golang.org/x/sys`. The price is that a process that dies leaves it in place, and that is why **it expires after
ten seconds**. The guardian is **thirteen real processes**, not goroutines: without a lock, «el
contador acabó en 1 y tendría que ser 13» (“the counter ended at 1 and should be 13”).

**Ruled out.** _A 0600 file with the refresh token_ (it is what the old desktop did; a
plaintext secret is not a keychain, and on a shared computer any process of the same user can
read it). _`fyne.Preferences` for the session_ (plain text) _and for preferences_ (per app, which is
exactly the opposite of what is asked). _A single store for the four things_ (the keychain asking for
permission at startup to know the theme). _Storing presence locally_ (a second truth).
_`singleflight` instead of the lock_ (it solves the race within one process, and the problem is
among thirteen). _Notifying with `fsnotify`_ (a dependency and three implementations to save two
hundred-byte reads every two seconds; it will be reconsidered if it is ever measured to be a nuisance).

<a id="0051"></a>

<a id="0051--firma-con-el-certificado-del-sistema-en-dos-pasos-con-sello-cualificado-de-izenpe-y-pedir-firma"></a>

## 0051 — Signing with the system certificate, in two steps, with a qualified Izenpe timestamp, and “Request signature”

`accepted` · 2026-09-26 · docs, signature, eIDAS, security · **develops [0030](#0030) §2 (c)
and replaces its §4 regarding the timestamping authority** · code in
`services/docs/internal/domain/pdfsigndospasos.go`, `…/http/pdffirmasistema.go`,
`…/http/pdfpedirfirma.go`, `apps/nucleo/api/docs_pdf_firma.go`; docs mig. `0007` and `0011`

**Context.** The signing certificate of most people in Europe is not in a file:
it is in the macOS or iOS Keychain, in the Windows store, in the Android KeyChain, on a
card or in the electronic ID card, and **its private key cannot be taken out of there** —on a card,
physically—. Until now CertiPDF signed with a `.p12` uploaded to the engine, and for everything else
it referred to the country's official application. The owner decided (2026-09-24): sign with the
system certificate, in two steps, against **all** stores, and with a **qualified** EU timestamp
(PAdES-B-T). And (2026-09-25) that it be possible to **request a signature from another person**.

**Decision.**

1. **Two steps; the key does not leave the device.** The service prepares (`POST /api/v1/pdf/sign/prepare`):
   the PDF with the gap and the `/ByteRange` written, the signed attributes (content-type,
   message-digest and signing-certificate-v2, **without** signing-time: PAdES forbids it) and their
   SHA-256 digest, plus the `DigestInfo` already wrapped for the “raw” functions (CKM_RSA_PKCS,
   NONEwithRSA). The app asks the SYSTEM to sign that digest with the chosen key —a call to
   its signing function, without writing a single byte of ASN.1— and returns the signature
   (`POST /api/v1/pdf/sign/complete`). The service builds the CMS and embeds it. The engine is ONE: the
   two steps use the same pieces as signing with a `.p12`, with the same PAdES profile
   (`/SubFilter /ETSI.CAdES.detached`).
2. **The binding lives in the database, not in the client** (mig. 0007). The preparation belongs to ONE person
   of ONE organization (RLS), is valid for **ten minutes**, is reserved while it is completed (two round trips at
   the same time do not produce two signatures) and **is spent** on completion. What is stored is the digest of the prepared PDF
   —the app returns the PDF: 25 MiB are not parked in the database—, the attributes and the chain. On
   completion it requires: the same person, **the same document byte for byte**, the attributes digesting
   THAT document and naming THAT certificate, and a signature of THAT digest with THAT key. A client
   that lies does not get a signature. At most five open preparations per person.
3. **Everything that can fail after the PIN is checked before.** The certificate has to be
   valid, be a signing one (digitalSignature or contentCommitment, the DNIe's), RSA ≥ 2048 or ECDSA
   P-256/384/521; the chain the app sends has to be **in order** (each certificate issued
   by the next) and **fit** in the gap together with the timestamp. Finding out afterwards throws away the work
   of whoever already entered the PIN on their card.
4. **What the person sees BEFORE signing**, from the service and not the app: who signs (the
   certificate's CN; the name the app sends does not count), who issued it, until when it is valid, the level
   (“may be qualified” only if the certificate declares QcCompliance **and** QcSSCD and comes with its
   issuer's chain; if not, “advanced”: the EU trusted list is not consulted here, so
   less is promised, [0030](#0030) §1), how many pages and which authority will timestamp.
5. **Qualified Izenpe timestamp, and without a timestamp nothing is signed silently.** The authority becomes
   **Izenpe** (`http://tsa.izenpe.com`), measured on 2026-09-24 against the EU trusted list
   as a QUALIFIED timestamping service in force; the free Certum of [0030](#0030) §4 is **not**
   qualified. The token is authenticated against the «ROOT CA QC IZENPE» root pinned in the chart
   (`SELLO_ANCLAS`, mandatory with `SELLO_URL`), it has to declare SHA-256, stamp the digest of the
   signature value and carry the request's nonce. **This changes the rule of 0030 §4 for the
   two steps:** if the authority does not respond, the service answers `pdf_timestamp_unavailable`, the
   preparation stays alive, and **the same signature** is valid for retrying or for continuing without a timestamp by
   explicit decision (`sin_sello=1`): the timestamp is not part of what is signed, so there is no need to ask
   for the PIN again. Only a digest travels to Izenpe: it is not a sub-processor ([`security.md` §11.6](security.md#116-sello-de-tiempo-izenpe)).
6. **The stores, one per platform, underneath and thin** ([0024](#0024), the interface is still
   Fyne): PKCS#11 (cards and DNIe) on the three desktops; Keychain on macOS and iOS
   (Security.framework); Windows store via CNG; Android KeyChain with our own Java selector
   class in the APK, alongside those of #983 (decided by the owner). ECDSA comes out in DER from the
   Keychain and from Android and in r‖s from CNG and PKCS#11: the app says which (`formato=p1363`) and the service
   normalizes it.
7. **“Request signature”** (mig. 0011). From CertiPDF, to another person **in the same organization**, by
   their email (resolved by Sytue ID with `directory:resolve` and the requester's session; the browser
   never chooses a `sub`). The PDF is not copied: it lives in the organization's drive and the request
   stores its id and its path, which has to end in that file's name. The recipient gets
   a `docs.signature.requested` notification with origin `pdf` and an email from avisos@, both with the
   link to the PDF (`/docs/pdf?path=`); on opening it, a banner tells them who is asking and for what, with
   «Rechazar» (“Decline”) and «Firmar» (“Sign”). They sign however they always sign (system, `.p12` or handwritten signature), the signed copy
   stays in Drive and **is delivered automatically**: the service downloads the original and the signed one with the signer's
   session and requires more signatures than the original and that the last one cover the whole document. Then
   the requester receives `docs.signature.completed` with the link to the signed one (or
   `docs.signature.declined`). One pending per file and person; at most thirty requests per hour
   per person.

**Consequences.**

- Signing with the system certificate does not turn Sytue into a custodian of anything: through the
  service pass a digest, a signature and a public chain. The sole control of art. 26.c
  stays where it was, on the person's device or card.
- With Izenpe, the date of each timestamped signature has the presumption of art. 41.2. Its policy gives a
  number of free timestamps per month and the number is not published: if it runs out, a contract is signed **without changing
  code** (credential via Secret).
- “Signed”, in a request, means “the recipient delivered a PDF with a new signature that
  covers it entirely”; **who** signed is stated by the certificate inside the PDF and CertiPDF shows it on
  opening it. The two things are not confused in the interface.

**Alternatives ruled out.**

- **A single step on the client** (the app builds the CMS and uploads the signed PDF). Ruled out: the
  cryptographic format would be written six times, once per platform, and the service could not
  check anything it receives.
- **Storing the prepared PDF on the server** instead of its digest: 25 MiB per preparation in the
  database or in a bucket, to save an upload the app has already done.
- **Signing without a timestamp when the authority is down**, as in [0030](#0030) §4. In two steps the person
  is right there and has just entered the PIN: they are asked, it is not decided for them.
- **Requesting a signature from any address**, including outside ones. The PDF lives in the
  organization's drive and whoever is not part of it cannot open it; doing so requires sharing it first through Drive.
  Left for when it is decided how (DECISION noted in the brief's pending items).
  **Decided on 26/09: [0057](#0057)** (first it is shared through Drive, then the signature is requested).

<a id="0052"></a>

<a id="0052--canjear-un-enlace-de-docs-también-desde-otra-organización"></a>

## 0052 — Redeeming a Docs link, also from another organization

`accepted` · 2026-09-26 · docs, multi-tenancy, sharing, security · **applies [0040](#0040)** · code in `services/docs/internal/http/canje.go`, docs mig. `0009`

**Context.** The public view `/s/{token}` (#1042) shows the document to anyone with the
link, read-only. An **editor** or **commenter** link promised more, and «Abrir en
el editor» (“Open in the editor”) only opened for whoever already had access: their organization or an invitation. The owner decided
that the link be **redeemed** for a permission of whoever opens it with their session, **also if they are from another
organization**. A document lives in its owner's organization, with its RLS by `tenant`, and the
token of whoever comes from outside carries THEIR organization.

**Decision.**

1. **Redeeming is a POST with a session** (`POST /api/v1/share/redeem`, the token in the BODY so
   that it does not end up in the request log), not opening a URL: it writes a permission. On the web it is triggered by the `/s/{token}` button with a form
   (`/s/{token}/abrir`), with the origin checked by Astro and the session cookie, which does not travel
   in a POST from another site. Without a session, to login and back to the public view.
2. **Only editor or commenter links are redeemed.** A reader one adds nothing to `/s/`.
3. **What is redeemed hangs off the link** (`doc_permissions.via_link`): reading permissions only
   counts those from links that are not revoked and not expired, so **revoking the link also removes what was
   redeemed**, and deleting it deletes it (`ON DELETE CASCADE`). A DIRECT permission the person already
   had is not touched: it is neither downgraded nor made dependent on the link (the answer is the one they have).
   The other way round too: what the owner grants by hand —or on accepting an edit request— stops
   hanging off the link. If they can already do the same or more, nothing is written. **Removing someone who
   came in through a link BLOCKS them** from that time onward (`doc_permissions.bloqueado_desde`, mig.
   0010): no link that already existed is redeemed for them —not even another live one for the same
   document—; one created afterwards by the owner, yes. Which permission counts is stated by a single function,
   `docs_permiso_vivo` (link not revoked or expired, and not blocked).
4. **The document stays in the host organization**, like the conversations of 0040. Whoever
   comes from outside is a row in `doc_permissions` with **`externo`**, written in the host.
5. **Their requests pass to the host WITHOUT anything from their token except the `sub`.** The
   `enAnfitrion` middleware of `/documents/{id}` only acts if the document is NOT in the token's organization
   and the person is an external in its owner's organization; then the request continues in that
   organization and `subjectFrom` reduces it to their `sub`: neither their groups (their outside roles are
   nothing here) nor their organization (a permission “for the whole organization X” does not apply to them).
6. **Cross-organization reads** are two `SECURITY DEFINER`s: `docs_anfitrion(documento,
sub)` returns the host's slug only if that person has a live `externo` permission there
   on that document, and `docs_de_fuera(sub, filtros, cursor)` the metadata of the list
   of those documents, so that **they appear in their organization's list** mixed by date.
   Redemption resolves the link with `docs_canjear_enlace`, the same surface as
   `docs_resolve_share` plus the link id. That a document belongs to the token's organization
   is remembered for five minutes, so as not to pay one more read on each request.
7. **Removing access cuts the live channel.** Revoking the link or removing the person closes
   their WebSocket (the `PublishRole` with an empty role closes it). And what does not notify on removal —a
   link that expires, a group or whole-organization permission that is withdrawn— too:
   each connection asks again for its role every minute. And it is closed when its token expires:
   on reconnecting, the new one says what is valid NOW (whoever left the organization no longer carries its
   `tenant`). If they can still get in another way, on reconnecting they are authorized again.
8. **In the owner's share dialog, the outside person appears with their name**: the directory
   of their organization does not know them, and `people:resolve` (0040) is asked, which gives the name or
   @username and never their email or their organization.

**Security review** (26/09, before merging, and again when resolving what it left). No
escalations or oracles between organizations. Everything it found is resolved: what is granted by
hand does not detach from a redeemed link; the list does not count what was redeemed from a revoked link;
redemption reports the real role when it does not write; removing someone blocks them for that link; the
live channel is cut on removing access; and **the request log of the ENTIRE suite records the
route pattern and not the URL** (`httpx.RequestLogger`, with a guardian that fails if a token
appears in the log), and **the Traefik access log does not record the path** (a guardian over every
Traefik configuration in the repository that enables it, and over every nginx: `access_log off`). In
production neither the front node's Traefik nor the cluster's nor the front node's nginx write an access
log, and the Astro surfaces do not log requests. Cross-organization
notifications are [0053](#0053).

**Alternatives ruled out.** _Copying the document to the opener's organization_ (two
truths that diverge at the first change). _Moving the person to the host organization_ (it would
give them everything their token does not say). _An ephemeral session with the link's token_ (the WS, the
comments and the ops need an identity, and the link does not have one). _Making redemption not depend
on the link_ (revoking the link would leave everyone who opened it inside).

<a id="0053"></a>

<a id="0053--un-aviso-es-de-la-persona-la-campana-enseña-los-de-todas-sus-organizaciones"></a>

## 0053 — A notification belongs to the person: the bell shows those from all their organizations

`accepted` · 2026-09-26 · hub, notifications, multi-tenancy, security · **completes [0040](#0040) and [0052](#0052)** · code in `services/hub/internal/store/notifications.go`, `services/hub/internal/notify/hub.go`, hub mig. `0005`

**Context.** With 0040 a person converses in Rooms with another organization, and with 0052 they comment on
or edit a document from another organization. The notifications of that activity —a mention, a
comment— are emitted by the service in the HOST organization, because that is where the resource lives.
And the Hub bell listed by the token's organization: the guest never saw them. The
owner authorized it on 26/09 “with review”.

**Decision.**

1. **A notification belongs to its recipient, not to the organization.** The `sub` is issued by Sytue ID and is unique
   across the whole suite; the address in their organization (`mailbox`) is assigned by Sytue ID. Whoever receives
   a notification sees it and marks it as read from the session of ANY of their organizations.
2. **The Hub's RLS says so with its own narrow policy** (`notifications_destinatario_lee`
   and `_marca`, PERMISSIVE, only `SELECT` and `UPDATE`): a row is visible if its recipient is
   one of the mailboxes in `app.destinatarios`. Creating notifications is still done in the
   sender's organization and the purge is still the maintenance one; per-organization isolation of everything
   else (presence, preferences) does not change.
3. **Across organizations only PERSONAL mailboxes count, and they come from the token**
   (`domain.BuzonesPersonales`): the `sub`, which Sytue ID issues as unique, and their primary
   addresses in ALL their active organizations (`mailbox` and the `mailboxes` claim), which Sytue
   ID assigns in a verified domain, are not repeated across organizations and **are derived on each
   token issuance**: whoever leaves an organization stops having that address in the
   next one. The claim only goes to the suite surfaces that render the bell (a closed
   list, `clientesConCampana`, with a guardian that cross-checks it with the deployments; the forge or the
   gateway, even though they are global, do not) and never in a delegated token: the domain of each address says which other organization they are in, and that is not
   told to a third-party app or to whoever acts on behalf of someone else (0040). Up to ten: the token
   travels in the session cookie. And a **delegated** token (`act`) opens none of this in the Hub. **Not the sign-in
   email, even if it comes with `email_verified`**: that claim is set to true by sign-ups done by an
   administration, SAML or a federation, without anyone proving that the address is theirs, and with it
   whoever administers one organization would read in all the others the notifications to that address (it was
   found by the security review). Notifications to the sign-in email are still seen, as
   before, in the organization that sent them. Without a `sub` there are no mailboxes and the policy shows
   nothing. The separator (`\x1f`) cannot appear in a mailbox, and one that carried it is discarded.
4. **Queries still filter by recipient** in addition to RLS: they are two locks, and the
   tests are seen red by removing each one.
5. **Real time, per person.** The SSE dispatch delivers a notification to a personal mailbox in all
   of that person's sessions, whatever their organization; one to their sign-in email, only in
   the organization that sent it. STATE events (the avatar, the
   read-only banner that gets unlocked) remain only in the organization where they happen.

**What is shown.** A notification from another organization carries its title, the name of whoever did it and the
name of the resource: exactly what that sender decided to send to that person. The link leads to the
resource in the host organization, where 0040 and 0052 already decide whether they can open it.

6. **An outside notification is seen as such.** The bell (web, Hub tray and Fyne) says which
   organization it comes from when it is not the session's (`organization`, which the Hub fills in only
   then). And **an organization cannot flood anyone's bell**: at most 200 notifications
   per hour to the same recipient (`notify.TopeAvisosPorHora`); past the cap, 429.
7. **The only thing that changes about a notification is whether it is read.** The UPDATE policy is per row; a trigger
   (`notifications_solo_se_marca`, mig. 0006) makes it per column: outside an organization
   merge, changing anything else fails.
8. **The rest of the person's things are also theirs.** Their mute rules apply to what reaches them from
   any organization (it is enough to mute it in one); “Your data” includes their notifications from all of them.
9. **An address that stops being theirs stops opening anything for them.** When Sytue ID withdraws an
   address, or its holder leaves the organization, the Hub deletes its notifications in all
   organizations and records it with the time (`buzones_olvidados`, `/internal/notifications/forget`):
   a token issued BEFORE —that of whoever had it lives a few minutes— no longer uses it, and whoever
   receives it inherits nothing. Sytue ID asks the Hub for this BEFORE releasing the address, and if the Hub
   does not answer the withdrawal is retried in the next sweep.
10. **The cap is per type** (organization, recipient, origin and type): whoever exhausts one does not mute
    the others. And there is a total one, of 1000 per hour, so that varying the type does not get around it. And to ANOTHER person, `POST /notifications` answers 202 with no body, whether delivered or not:
    returning the notification —or not returning it when they have it muted— would reveal what they have muted.

**Security review** (26/09, before merging, and again when resolving what it left). No leaks
through the database: the variable comes only from the token, lives in the transaction and the policy does not widen
INSERT or DELETE. It found the `email_verified` gap (point 3) and what points 6
to 10 resolve. The third pass ruled out a table of remembered mailboxes (it left whoever left an
organization reading their notifications) in favor of the `mailboxes` claim, and the last two limited it to the
suite surfaces and to non-delegated tokens and added the total cap. Nothing is left pending.

**Tests (seen red).** `TestAvisosDeLaPersona` (real Postgres): the recipient sees and marks
hers from her organization, and the one to her sign-in email does not cross over to her; it is not read, counted, or marked one by one or all at once
by another person of the same organization, nor another from another organization, nor a token without mailboxes.
Red when removing the recipient filter from the store, and when also removing it from the policy.
`TestDispatchPorPersona` (red with per-organization dispatch) and
`TestBuzonesPersonalesSinElCorreoDeEntrar`. `TestAvisosRevision` (real Postgres), red
when removing each thing: the trigger, the cap, the person's rules, forgetting the mailbox (and that a
previous token does not use it) and the mailboxes of their other organizations.

**Alternatives ruled out.** _Emitting the notification in the recipient's home organization_ (the
sender does not know it: 0040's `people:resolve` does not reveal it, on purpose). _A copy of the notification
per organization_ (two to mark as read, and the sender would have to know where each one is).
_A SECURITY DEFINER function_ (it would have repeated in SQL the recipient filter that the policy
already expresses, with more surface).

<a id="0054"></a>

<a id="0054--borrar-mi-cuenta-la-persona-se-da-de-baja-sola-con-30-días-para-deshacerlo-y-se-borra-lo-suyo-y-no-lo-de-su-organización"></a>

## 0054 — “Delete my account”: the person unsubscribes on their own, with 30 days to undo it, and their own data is deleted but not their organization's

`accepted` · 2026-09-26 · id, gdpr, google-play, security · code in `services/id/internal/http/account_delete.go`, `services/id/internal/store/borrado.go`, `services/id/internal/merge/personas.go`, id mig. `0098`

**Context.** Google Play rejected the data safety section of the thirteen apps: it requires declaring
how accounts are created and, if they are created, a web page where deletion can be requested. The account is created in
Sytue ID (username, password and SMS) and nobody could delete it themselves: only the removal
from Sytue Admin (mig. 0092) and the SCIM one existed. The owner asked for it on 26/09.

**Decision.**

1. **A Go page with a stable URL**, `https://id.<dominio>/account/delete`, and not in the
   “My account” app: reauthentication is done by the IdP with its cookie, which the app does not see. The app links
   to it from its «Borrar mi cuenta» (“Delete my account”) card; Fyne gets there through “My account”, which opens that same
   app. It goes in `id.ingress.rutasDeCuenta`, and the Ingress test watches it.
2. **Strong confirmation.** The account (@username or email) is typed in and the person proves they are its owner
   now: the password, or an identification less than five minutes old (`frescuraStepUp`). With a
   second factor, the password alone is not enough —stealing it would be enough to delete an account that requires
   two to sign in—: you have to sign in again with both or with a passkey
   (`/login/?return_to=/account/delete`). The POST checks the origin and is rate-limited like login.
3. **An organization cannot be left without anyone to administer it.** If they are its only
   active owner, or they administer it and there is no other active owner or administrator, it is
   blocked and the person is told which ones. The check goes inside the transaction, with the organizations
   locked `FOR UPDATE`, so that two co-owners leaving at the same time do not cross. The
   personal account does not count: it belongs to the person.
4. **Grace period: that of the removal that already existed**, 30 days (`retencionBaja`, mig. 0092). The
   self-removal is the same removal with `closed_by` = the account itself, which can only be written
   that way: Sytue Admin does not let you remove yourself and SCIM does not write `closed_by`. **An
   administrator cannot undo it** —it is the erasure the person asked for—: `RestoreUser` and
   the listing of recoverable removals exclude it, and only privacy can undo it at their request
   (privacy@sytue.eu).
5. **What serves to sign in is deleted on confirmation**, in the same transaction, not when the
   window expires: sessions, refresh tokens, codes, password, passkeys, TOTP, recovery codes,
   app passwords, devices, links with external IdPs, live elevations and the photo
   (also the one in the Contacts cards). Restoring returns the account, not access: whoever
   comes back sets a new password through recovery.
6. **What other services store about THAT person and not about their organization** —Hub notifications and
   presence; Rooms push, preferences, drafts, saved items, read state, followed threads and
   categories; Drive favorites— is deleted in the next sweep (every ten
   minutes), through the same path as the merge: same credentials, `InMergeTx`. The list is
   closed (`merge.DatosDeLaPersona`) and a test checks that each table exists with its
   `subject` and its merge policy.
7. **What belongs to the organization stays in the organization**: mail, files (also “My drive”,
   which belongs to the tenant), events, messages in channels. Whoever administers it decides. The
   invoices, the legal period; the security log, twelve months.
8. **Once the window has expired, the record is anonymized** (email, @username, name, mobile number, recovery
   email, attributes) and its memberships are removed. The row is not deleted: it is referenced by
   tables without cascade, and its mailbox —which belongs to the organization— keeps occupying the address so
   that nobody takes it and receives what people keep sending to it. A purged account never
   receives an @username again.
9. **The content of a personal account** (its mail, its files, its photos) stops being
   accessible on confirmation and its deletion, when the window expires, **is manual**: it lives in ten databases and
   in the object store and there is no tenant deletion yet. The sweep reports it at
   error level, which fires the alert, and the one-month period of GDPR art. 12.3 runs. It is in `roadmap.md`.

**Security review (2026-09-26), everything resolved before merging.** An account that is no longer
active stops working instantly even with a live access token: `requireAccessToken` and
introspection look at its status, and so does Mail's app password verification
(previously a removal from Sytue Admin or SCIM still opened the mailbox over IMAP). The owner count
(`CountOrgOwners`) counts only active ones and locks the organization, so a
Sytue Admin removal and a self-removal do not cross. A new removal clears the mark of data already
deleted; the purge repeats the credential deletion; deletion in other services waits for
the access token to expire (the services validate the JWT on their own), goes in random order so
that a failing account does not hide the others, and its failures go at error level. A failed password
consumes the same per-account quota as login. The last platform administrator cannot
leave, and the removal takes away platform administration. With a second factor, a passkey without
user verification is not enough. The purged personal account keeps its id as its name. In the second and third rounds of
the review: the owner and administrator counts count the active ones **plus the affected
account** (otherwise a suspended owner could not be removed when there was another active one), the
lock is `FOR NO KEY UPDATE` so as not to slow down sign-ins, platform administration
is locked too, and if the account status cannot be read the token is not valid.
The third round found nothing.

**Alternatives ruled out.** _The page in the “My account” app_ (it does not see the IdP cookie and
a challenge over the API would have to be invented). _Deleting the `users` row_ (it would fail with the references
without cascade and would free up the email address). _A deletion endpoint per service_ (ten
secrets and ten deployments to delete some rows that are already reachable as in the merge).
_Waiting for the 30 days to delete the credentials_ (an account whose holder has asked for deletion
must not be usable that month). _Letting the password always be enough_ (it downgrades the second factor).

<a id="0055"></a>

<a id="0055--la-url-canónica-de-cada-producto-es-su-host-de-producto-la-organización-va-al-path-no-al-subdominio"></a>

## 0055 — The canonical URL of each product is its product host; the organization goes in the PATH, not the subdomain

`accepted` · 2026-09-26 · web, id, deployment · code in id mig. `0099`, `deploy/helm/charts/sytue-frontend/templates/deployment.yaml`, `values.yaml` (`orgHost`), guardian `platform/pkg/migcheck/migcheck_test.go`

**Context.** You go into a product and the login goes in circles: `rooms.sytue.eu` → 302 to
`corporate.sytue.eu/rooms` (the host of the canonical `redirect_uri`) → IdP → back. The chain is
caused by the organization canonical URL (mig. 0054): the BFF ALWAYS starts the OIDC flow on the host
of the callback (`bff.ts`), and the session cookie is born there. With more interactions BETWEEN
organizations and with externals (meeting links, shares, the redemption of 0052), the
model that scales is the PRODUCT host —which links between apps (`appUrl`), notifications
(`hubnotify`) and the native apps (`api.ServiceURL`) already use, all three without hops—, and the place for
the organization is the PATH within the product, not the subdomain: `rooms.sytue.eu/rooms/<org>/…`
makes the org shareable by link and leaves subdomains for products and for the MAIL
domain.

**Decision.**

1. **Canonical = product host with prefix**: `https://mail.sytue.eu/mail/auth/callback`
   (mig. 0099, thirteen clients; `platform` without prefix, `account` stays global at
   `id.<dominio>/account` from 0097). The chart composes `PUBLIC_URL` and
   `SYTUE_OIDC_REDIRECT_URI` with the app's subdomain; wherever the browser comes in, the
   login no longer jumps hosts. NO BFF changes: the jump to `SYTUE_OIDC_REDIRECT_URI` was the
   mechanism, not the problem.
2. **The organization host stays in transition, it is not retired.** It keeps serving the SAME
   prefix (Astro's `base` is baked in) and its callback stays registered (0054 and siblings): a
   login started there works, with its 302 to the product. The decision of 2026-08-14 («las dos
   direcciones válidas», “both addresses valid”) still stands; what changes is WHICH one is announced. When phase 2 puts
   the organization as a PATH segment, the wildcard becomes a 301 and is retired; the subdomain
   survives as a mail domain.
3. **The guardian requires the canonical URL.** `TestRedirectUrisDeLosHostsDeSuiteCubrenElChart`
   NOW ALSO checks `https://<subdomain>.<dominio><path>/auth/callback` for each app
   in the chart, seen red by removing a row from 0099. A `redirect_uri` that does not match does not give a
   deployment error — it gives a login that fails only on that host.
4. **The org in the path is PHASE 2, app by app**: routes under `[org]/` after the app prefix,
   org-aware `r()` and `appUrl` helpers, notifications and native deep links with the org, and entry without an org
   redirected to the active one. The session does not change: the active organization travels signed in the
   token (`tenant` claim) and no service reads it from the host today.

**Why this and not something else.** _A second set of images as for `sytue.com`_ (0049) is not
needed: the domain does not change, only the host each already-deployed image announces, and the only
file that ties image to domain (`tokens.ts`) is not touched. _Resolving URLs by Host header_
was ruled out by 0049 because it forces relative links; here nothing is resolved at runtime: the
canonical URL is one more chart constant. _Shared-domain cookies_ to avoid repeating login
between products would break the deliberate isolation of `__Host-` (`cookies.ts`, risk R6) and it is not
needed: SSO is provided by the IdP. _Registering only the product canonical URL_ and deleting the
organization one would leave whoever comes in through the old host without login until phase 2 finishes.

**Measured precedents.** Dev has logged in via the product host since 0002 (root form), and the
`sytue.com` front (0049/0094) logs in via the product host WITH PREFIX —exactly the form of this
migration— since it was set up.

<a id="0056"></a>

<a id="0056--la-organización-como-segmento-de-path-dentro-de-cada-producto"></a>

## 0056 — The organization as a PATH segment within each product

`accepted` · 2026-09-26 · web, native, notifications · code in `apps/frontend/packages/config/src/organizacion.ts`, `astro-middleware.ts` (`middlewareDeOrganizacion`), mig. none, guardian `middleware-org.test.ts`

**Context.** ADR 0055 left the canonical URL on the product host and announced the direction: with more
interactions BETWEEN organizations and with externals (meetings, shares, notifications, redemption),
links have to say WHICH ORGANIZATION they point to, and the place for the org is the PATH
(`rooms.sytue.eu/rooms/<org>/channels/x`), not the subdomain. The session does not change: the active org
travels in the token (`tenant` claim) and no service reads it from the host.

**Decision.**

1. **Pages do NOT move.** An `[org]` in each app's router would be a walk through
   thirteen apps and hundreds of files with a single typographic benefit. What changes is the
   shared middleware (`middlewareDeOrganizacion`, composed BEFORE the language one): a URL
   with an org is served internally (rewrite to the real route, which keeps its prefix) with the org in
   `locals.org`; a URL without an org with a session is redirected to the canonical one with a LOCAL 302 (same
   host, no IdP); without a session it passes, because the guard sends to login and on return the 302 kicks in
   by itself.
2. **Idempotent by construction, not by faith.** The rewrite carries the `?__org=<org>` marker: if the
   runtime re-runs the middleware on the rewritten request, the second pass sees the marker and
   serves. It does not depend on how the runtime re-runs it (or not). The 301 that closes the transition of the
   organization host is prepared behind `orgHost.retirarComodin` (frontends chart), off
   by default: it is the final phase and the operator turns it on in their closing deployment.
3. **The first segment after the prefix is ALWAYS the org, by construction.** Distinguishing
   “org or route?” with a list of routes is asking for a new page to break the parsing
   silently. URLs without an org are transitional; the exceptions (`api`, `auth`, assets,
   `healthz`, and whatever each app declares: Docs' `s`, Drive's `public`, Photos' `p`) pass
   without an org.
4. **THE PATH ORG DOES NOT AUTHORIZE.** The token (`tenant` claim) and the service authorize. If the path
   org differs from the session's it is NOT blindly redirected: an external with a link from another
   organization has to land where it is — redemption (0052), Rooms externals (0040)
   and shares resolve it. Redirecting them to THEIR org would break their link.
5. **Links between apps, notifications and native apps carry the org when they know it**
   (`appUrl(key, path, org)`, `hubnotify`, `api.AppURL`); those that do not carry it keep
   working with the 302 of point 1. The address bar converges to the form with the org.
6. **Operational switch:** `SYTUE_ORG_PATH=off` turns off the entire behavior without
   deploying new code.

**Why this and not something else.** _Moving the pages under `[org]/`_: hundreds of files and parity
guardians for the same URL result. _Org by query_: it is not a path, it is not shared the same way
and the ADR asked for it as a segment. _Cookie per org for simultaneous multi-org_: that is another wave (the
session carries ONE active org; two tabs of different orgs share a session and the middleware
gate does not simulate it). _Redirecting on path org ≠ session mismatch_: breaks the
external's link (point 4).

**Scope of this wave.** The twelve product surfaces (not `www`, not `platform`, not `account`,
which are global). Retiring the `<org>.sytue.eu` wildcard as a 301 and removing the organizations' DNS
publication are left for the deployment that closes the transition.

<a id="0057"></a>

<a id="0057--pedir-firma-a-alguien-de-otra-organización-el-pdf-sale-por-un-enlace-de-drive"></a>

## 0057 — Requesting a signature from someone in another organization: the PDF goes out through a Drive link

`accepted` · 2026-09-27 · docs, signature, multi-tenancy, sharing, security · **completes [0051](#0051) §7** (which left it ruled out “until it is decided how”) · code in `services/docs/internal/http/pdffirmaexterna.go`, `…/pdfpedirfirma.go`, `services/docs/internal/drive/cliente.go`, `platform/pkg/peticiones/avisos.go` (`DeFuera`), docs mig. `0012`

**Context.** “Request signature” from [0051](#0051) only reached people in the same organization: the
PDF lives in the organization's drive and whoever is not part of it cannot open it. The owner decided
on 26/09 that it should be possible to request outside too: **first the PDF is shared through Drive, reusing what
Drive already has, and then the signature is requested**; the interface detects that the person is from outside,
explains the step and does it with confirmation. Without a new permission system. The only thing Drive
has today for something to leave the organization is the **read-only link** (`kind=link`):
shares with people and groups are resolved by the token's organization.

**Decision.**

1. **Who is from outside is said by Sytue ID.** By email, first `directory:resolve` in the
   requester's organization, as always; if they are not there, or if an @username is typed,
   [0040](#0040)'s `people:resolve` (`peticiones.DeFuera`): their `sub` and their name, never their
   email or their organization. And if that `sub` turns out to belong to the organization (a colleague's
   @username), they are from inside. By `sub` only requests inside are made. Before looking anyone up it is checked
   that the PDF belongs to the requester.
2. **Nothing without confirmation, and of whom.** With someone from outside, the service answers 409
   `pdf_signature_recipient_external` with their name and a `confirm`, and creates nothing. The interface
   (web and Fyne) explains it —a read-only Drive link that expires in 30 days, that nobody
   sees, that leaves a copy in the other person's drive, visible to their organization— and on
   confirming sends the same request with that `confirm`: if it no longer resolves to the same person, it asks
   again.
3. **The link is Drive's and the requester creates it.** The service calls Drive's `POST /api/v1/shares`
   with the requester's session: their permission on the file and **Drive's label
   gate** (a label with `BlockExternalShare` blocks it here too:
   `pdf_signature_share_blocked`). The link appears in the file's share list in
   Drive, where it can be revoked. The caps are checked before creating it.
4. **Nobody sees the token.** It is stored **encrypted** (`enlace`, with the service key,
   `SYTUE_DOCS_SECRET_ENC_KEY`, and **bound to its request** as AEAD associated data: it is not valid in
   another row or for anything else; without a key it is not offered) and only this service uses it: when the
   signer opens the request (`POST …/{id}/open`), it downloads the PDF through the link and leaves a copy in THEIR
   drive (“Sytue Docs” folder); if they delete it, opening again fetches another. It does not travel in the
   notification, nor in an email, nor to any browser, nor to traces (`telemetry.Middleware` names
   the span by the route pattern). It is deleted from the row when the request closes without being signed and when
   the signed one is fetched; the original's link is revoked by the requester's session as soon as the
   request stops being pending (on cancelling, on fetching the signed one, or the next time they
   list theirs, which is the only thing that can do it after a rejection), and expiry is the safety net.
5. **The request lives in the requester's organization**, like the document of [0052](#0052):
   the signer is a row with `externo`. The routes of a request (`/signature-requests/{id}/…`)
   pass to that organization for its outside recipient (`peticionEnAnfitrion`), **with nothing from
   their token except the `sub`**, and every statement still requires recipient or requester. The
   cross-organization reads are three `SECURITY DEFINER`s: `docs_firma_anfitrion(id, sub)` (only
   the slug, only to its outside recipient), `docs_firmas_de_fuera(sub, tenant)` for their list,
   **without the original's path or its id** (they would say how another organization organizes its folders), and
   `docs_firmas_pendientes_para(sub)` (a number, for the cap).
6. **What is signed is THE document.** The request stores the digest (SHA-256) of the original when requesting.
   The copy the signer fetches has to have that digest —if the original changed, it is not
   fetched—, and on delivery, their copy too: if they touched it, they would be signing another document. What is delivered has
   to have more signatures, the last one covering it entirely ([0051](#0051) §7) and, in addition, **the same
   pages with the same text** as the original. Signing rewrites the file the first time, so
   it cannot be required to start with the original; what does not change is what is read, and without
   this any other signed PDF could be delivered. This check also applies to
   requests from inside. In a scan without a text layer it only binds the number of pages.
7. **Delivery comes back the same way.** On delivery, the service creates with the signer's session a
   link to the signed copy (7 days), encrypted the same way, and stores the digest of what was checked. The
   requester fetches it to THEIR drive (`POST …/{id}/fetch-signed`), with the original's name and
   «(firmado)» (“(signed)”), and it has to be exactly what was delivered: if the file changed afterwards in the other
   drive, it is not fetched. Until then the requester sees neither the path nor the id of the signed one.
8. **Notifications, yes; email, no; and never anyone's email.** The Hub notification reaches the person in
   any of their organizations ([0053](#0053)) and leads to CertiPDF in THEIRS (`/docs/pdf` without
   organization or path: the front page, where the request is). No email: Sytue ID does not give theirs,
   and sending from avisos@ to whatever address anyone types would be lending out the sender. Each
   side is named by their name (or what the requester typed), never by their token's email.
9. **Nobody floods another person's list.** In addition to the requester's 30 per hour, someone from
   outside can have at most 5 pending from the same person and 20 from all organizations
   (`pdf_signature_request_recipient_busy`), counted under a lock. “Open” and “fetch” go one
   at a time per request (`ocupada`, which expires after two minutes) and with a per-person cap, because
   each round trip downloads and uploads up to 25 MiB.

**Consequences.**

- The signer's copy stays in THEIR organization's drive, which in Drive is a space for
  the whole organization: the interface says so before confirming and in the row on the front page.
- This does not open any new way of getting files out: it is the same as Drive's “Share” button,
  with the token stored instead of shown. In Drive anyone in the organization can
  create a link to a file they see; if an organization wants to limit that, it is a Drive
  policy, not a policy of this.
- The link to the signed copy lives in the signer's drive and only that
  person can revoke it (from Drive); its token is deleted on fetching it and expires after 7 days.
- The service key is a single one, like the connectors' one: if it is changed, pending outside requests
  can no longer be opened (`pdf_signature_request_link_gone`) and have to be requested
  again. A down migration of 0012 deletes the outside rows; their links remain until they expire
  with a token nobody has anymore.

**Alternatives ruled out.** _Sharing in Drive with the person (`kind=user`) across
organizations_: Drive resolves everything by the token's organization; opening that up is the new permission
system the owner ruled out. _Sending the link in the notification or by email_: it would be a
bearer secret in the Hub, in a mailbox and in the history. _Storing the PDF in this service_:
25 MiB in the database per request, the same thing [0051](#0051) ruled out. _Requiring what is signed
to start with the original_: the first signature rewrites the file.

**Security review** (27/09, before merging, independent). No high findings. Everything
it found is resolved: what was signed was not bound to the original (digests and same text, point 6);
nothing limited how many requests someone from outside receives (point 9); a colleague's @username
came out as from outside (point 1); the token's email reached the other organization and an
email was attempted with the wrong session (point 8); links not revoked after a rejection or not fetched (point
4); the trace span with the full URL, which would carry a public link's token (point 4; the
middleware was not mounted in any service, but now it cannot be); the signed file's name was set by
the other organization (point 7); a deleted copy left the request with no way out and two simultaneous “open”s
left two copies (points 4 and 9); the encrypted token was not bound to its row (point 4); the caps
were not atomic and the link was created before checking them (points 3 and 9); the
person was looked up before checking the file and the confirmation did not say who (points 1 and 2); the
migration did not close `copia_ruta` or the tokens of a closed request (constraints in 0012);
and a 401 from Drive was counted as “label that does not allow sharing” (`drive.ErrProhibido`). The two
product matters it left for the owner —that someone can **block** whoever requests signatures from them and
that an organization can **close** those arriving from outside— were decided on 27/09: [0059](#0059).

**Tests (seen red).** `TestPedirFirmaAFuera`, `TestCancelarAFueraQuitaElEnlace`,
`TestPeticionEnAnfitrion` and `TestElTokenNoValeEnOtraPeticion` (service); `TestPeticionesDeFirmaAFuera`
and `TestNadieLlenaLaListaDeAlguienDeFuera` (real Postgres); `TestElSpanNoLlevaLaURL`
(telemetry); and those of the web, the SDK and Fyne. Each guardian was seen red by removing what it watches.

<a id="0058"></a>

<a id="0058--el-ci-corre-en-github-actions-los-workflows-se-portan-sin-cambiar-lo-que-comprueban"></a>

## 0058 — CI runs on GitHub Actions; workflows are ported without changing what they check

`accepted` (points 1 and 5, replaced by [0060](#0060)) · 2026-09-27 · CI, operations · code in `.github/workflows/`, `.github/actions/`, `.github/RUNNERS.md`, `deploy/sytue-git/bin/{ci-check,publicar-asset}.sh`, guardians `deploy/production/tests/release-workflow-test.sh`, `.github/tests/ci-sin-azar-test.sh`, `scripts/check-toolchain-versions.sh`

**Context.** The repository's origin moves from git.gay (Forgejo) to
`github.com/thinkddr/sytue-one`. The twenty-five workflows in `.forgejo/workflows/` were already
written in GitHub Actions syntax —`github.*` context, `GITHUB_TOKEN`,
`hashFiles`— because Forgejo copies it; what tied them to the forge was the environment (three
of our own runners on the front node, with a filtered Docker and the Go cache mounted from the
host), the plumbing that avoided reading other repositories (cloning with `git` by hand,
a vendored `upload-artifact`, our own script against the runner's cache server) and
a dozen calls to `/api/v1`.

**Decision.** Port all twenty-five as they are —the same gates, the same
`timeout-minutes`, the same secrets— changing only what GitHub does differently:

1. **GitHub-hosted runners** (`ubuntu-24.04-arm`) and not our three own runners
   re-registered: registering GitHub's `actions-runner` on a host that also serves
   production is an infrastructure change that cannot be tested from the
   repository, and the hosted ones require none: the repository is private, so
   they spend the plan's minutes (and are billed once they run out), which is the price of not
   operating machines. Each job runs in
   the SAME container by digest that the Forgejo runners provided (`node:24-bookworm`),
   so that the plumbing that installs tools as root keeps working without touching it.
   ARM64 and not amd64 because it is the architecture on which everything there is was measured.
2. **Official actions pinned by SHA** (`checkout`, `upload-artifact`, `cache`) instead of
   the manual cloning, the vendored copy and the cache script: the reason to
   avoid them (the forge did not allow reading other repositories) no longer exists, and the
   toolchain gate requires every `uses:` to carry the SHA from `versions.env`.
3. **GitHub API where the forge spoke something else:** check runs instead of commit
   statuses to decide whether a commit is green, `git/refs` to mark a version
   as deployable, `upload_url` and `releases/assets/<id>` for the assets, `pull_requests`
   from the runs listing for the canceller, Renovate in `github` mode.
4. **Minimal `permissions:` per workflow**, which Forgejo ignored and GitHub enforces.
5. **What stays on git.gay stays there and is named:** the private Fyne forks
   (`SYTUE_FORKS_SERVER`, `SYTUE_GIT_FORKS_TOKEN`) and the suite's image registry
   (`DESTINO`, with `vars.SYTUE_REGISTRY` to move it). Changing the registry is another
   decision, a deployment one, not a CI one.

**Alternatives ruled out.** _Our own runners on GitHub_ (the front node re-registered): it is
the documented way back in `RUNNERS.md`, not the starting point. _amd64 `ubuntu-24.04`_:
same machine and same price; it is ruled out only so as not to change two things at
once (the forge and the measured architecture). _Rewriting the workflows “the GitHub way”_ (setup-go,
setup-node, buildx): it would throw away the sha256 version gates and the digest image gates
that exist so that CI does not change on its own.

**Consequences.** The Go cache (~9 GB of modules) competes with the 10 GB per-repository cap
of `actions/cache`; if it evicts, only the build cache is cached. `schedule:`
works again (on git.gay it never fired) and the pilot timer that stood in for it
is no longer needed. In the supply-chain inventory `github.com` and `api.github.com` remain
as a `temporary-exception` renewed until 2027-03-27: the forge is outside the EU and the
gate forces deciding it again every six months, which is the honest thing —`pinned-artifact`
would say nothing of ours leaves, and the code and the issues do leave—. The secrets are
registered in GitHub with the same
names; the packages one and the forks one remain git.gay's. `desplegar-forja.yml`
and `espejo-herramientas.yml` (Sytue Git, retired) and the workflows against the Scaleway
registry (being decommissioned) are ported by hand and without an automatic trigger, pending their formal removal.

<a id="0059"></a>

<a id="0059--peticiones-de-firma-de-fuera-quien-recibe-bloquea-y-su-organización-decide-sin-que-quien-pide-se-entere"></a>

## 0059 — Signature requests from outside: the recipient blocks and their organization decides, without the requester finding out

`accepted` · 2026-09-27 · docs, signature, multi-tenancy, privacy, security · **completes [0057](#0057)** · code in `services/docs/internal/http/pdffirmabloqueos.go`, `services/id/internal/http/internal_organizaciones.go`, `apps/frontend/apps/console/src/components/ExternalSignaturesPanel.tsx`, docs mig. `0013`

**Context.** With [0057](#0057) anyone who knows an @username can request a signature from that
person from another organization, and opening the request leaves a copy in THEIR
organization's drive. The 0057 security review left two decisions to the owner, and on 27/09 they made them:
**yes** to a person being able to block whoever requests signatures from them from outside —from the received request, and
it is removed from their settings—, and **yes** to a setting in Sytue Admin so that the organization decides
what it does with those arriving from outside: open (default), closed or only from trusted
organizations. **The requester does not find out**: the request is “sent”, but it does not arrive.

**Decision.**

1. **Not arriving is `silenciada`.** An outside request that must not arrive is created just like
   any other —the same response, the same Drive link, they see it in their list, it counts towards their
   caps and they can cancel it— but it does not arrive: no notification, out of the signer's list, without
   host organization and without being able to be opened, signed or rejected (the signer's
   statements require `NOT silenciada`, in addition to the middleware). Telling the requester that it did not arrive would
   reveal who has blocked them or which organization a person is in, which
   `people:resolve` keeps quiet on purpose ([0040](#0040)). For the same reason:
   - **it is always decided the same way**: first Sytue ID, then the policy and then the block, without
     shortcuts —a block that cut off earlier would be noticeable in the timing and in the 503 of a Sytue ID that is down—;
   - **the recipient's cap of 20 pending from outside no longer returns 429**: whatever exceeds it
     is created silenced. With a 429 the requester could fill that cap from other accounts and count
     which of their requests arrived. The caps that count the requester's things (per hour and per
     pair) remain errors: they could count them themselves;
   - silenced ones do not count towards that cap, and the block is checked again INSIDE the INSERT: one
     that arrives between the check and the creation also silences.
2. **The block belongs to the person**, not to an organization: it applies from any of theirs and
   against the requester from any of theirs (`pdf_firma_bloqueos`, with `subject` = the blocker
   and a `tenant` that only says where it was done from). Nobody reads it directly: everything goes
   through `SECURITY DEFINER` functions that receive the `sub` of the blocker's token. Blocking
   silences what was already pending from that person in all organizations; unblocking does not
   revive what was silenced. Up to 500 per person, under a lock.
3. **Where it is done.** In the banner of the received request (web and Fyne), with confirmation in the
   same banner and in any state of the request —also after rejecting or signing it—; it is
   removed on CertiPDF's front page, in «Personas bloqueadas» (“Blocked people”), which is where their requests are:
   CertiPDF has no other settings screen and the list travels with the requests. Blocks
   go into “Your data” (`signature_blocks`) and “Delete my account” takes the person's ones with it
   (`DatosDeLaPersona`, [0054](#0054)).
4. **The policy belongs to the organization** (`pdf_firma_politica`, with its RLS and its
   `_platform_merge`); it is saved by whoever administers (`owner`/`admin`, not delegable) from «Firmas de
   fuera» (“Outside signatures”) in Sytue Admin, and it is applied at two moments:
   - **When requesting**, the service asks Sytue ID which active organizations the signer is in
     (`POST /internal/people/orgs`, only from inside and with its OWN secret,
     `sytue-id-orgs-persona`, shared only with Docs; without it nothing is requested outside) and it arrives if
     **any** admits it —open or without a row, or trusted with the requester's organization in
     its list— or if the person has a **personal account**, where they decide with their blocks. The
     personal account does not travel by its name (it is derived from its address): Sytue ID only says that there
     is one. If Sytue ID does not answer, nothing is created: letting it arrive would skip the policy.
   - **When viewing, opening and signing**: the signer's list only shows what the
     organization of THEIR session admits, and opening or signing from one that does not admit it is
     `pdf_signature_org_closed` —it would leave files in that organization's drive—. Rejecting and
     blocking are always possible.
5. **Trusted organizations** are written by the name in their address (the `acme` in
   `…/docs/acme/`), up to a hundred, excluding one's own. Sytue ID organizations are not deleted (they become
   `deleted`) and their name is not reused, so nobody inherits the trust. In a merge
   (and in a rename, which uses the same engine) the source's policy is discarded if the destination
   has its own, and whoever trusted the source now trusts the destination, without duplicates or
   trusting itself (Docs' `platform_merge_prepare`, only within a merge).
6. **The functions that cross organizations, also in production.** In production the application
   and the migrations use the SAME role, owner of everything and without BYPASSRLS; with FORCE ROW LEVEL
   SECURITY, a `SECURITY DEFINER` on its own does not skip RLS (Drive found this in its 0003), and
   in the tests a superuser owner masked it. It was like that in ALL of Docs' functions: a document's public
   link, cross-organization redemption ([0052](#0052)) and outside requests
   ([0057](#0057)). All of them now carry `SET app.docs_definer = 'on'` —valid while they run and
   restored automatically on exit— and each table they read has an additive policy that only opens with
   that value. Docs' integration tests put those functions in the hands of the application
   role, as in production, so that it is not masked again.

**Consequences.**

- The requester sees “pending” forever on a request that did not arrive; they can cancel it. One
  silenced by the policy stays that way even if the organization opens later.
- If an organization closes, outside requests —including those already signed or rejected—
  stop being visible from its sessions; from a personal account they are still visible.
- A notification that arrived before the block stays in the bell and leads to CertiPDF's front page,
  where the request no longer is.
- The block list stores the name the person saw in the request: it is THEIR record of
  whom they blocked, and it goes away with them.
- Drive does not record when a link is used, so the link of a silenced request that nobody uses
  says nothing that an ignored request does not say.
- Rolling back migration 0013 cancels the silenced ones: whoever requested them will see them as “cancelled”.

**Alternatives ruled out.** _Telling the requester “blocked” or “not admitted”_: it would leak the
block and the person's organizations. _Not creating the Drive link for a silenced one_: the
file's share list in Drive would give away the difference. _Applying the policy only when
viewing_: the notification would already have arrived. _Storing blocks per organization_: blocking in one would not
work in the others. _Not counting the personal account_: someone could receive nothing in their own
because they were also in a closed organization. _Reusing the internal directory secret_:
it belongs to a maintenance task that is being retired.

**Security review** (27/09, before merging, independent). No high findings; everything
it found is resolved: the recipient's cap served as an oracle (point 1); the block cut off
before asking Sytue ID (point 1); the functions failed open if the owner does not skip
RLS, and it turned out ALL of Docs' were like that (point 6); the directory secret was moving to a
permanent use and the personal account traveled by its name (point 4); the “closed” text
promised too much with personal accounts; signing did not look at the policy (point 4); a block
between the check and the INSERT (point 1); blocks as personal data (point 3); blocking was only
possible from a pending request (point 3); the rollback and its `app.merge`; the block cap without a
lock and the “too many” when blocking oneself; duplicates in the trust list after a merge; the
reuse of organization names (point 5); an orphan copy if the request disappears
while being fetched (it is deleted); the console section shown to whoever could not use it (now only
owner/admin); and Sytue ID's response read without a cap. And the Fyne catalogs, regenerated.

<a id="0060"></a>

<a id="0060--runners-propios-en-el-frontal-imágenes-en-ghcrio-y-forks-en-github"></a>

## 0060 — Our own runners on the front node, images on ghcr.io and forks on GitHub

`accepted` · 2026-09-27 · CI, deployment · replaces points 1 and 5 of [0058](#0058) · code in `deploy/rooms-sfu/{instalar-runners-github.sh,runners/sytue-github-runner.service,drenar-y-reiniciar.sh,podar-ci.sh}`, `.github/workflows/`, `.github/actions/toolchain/forks.sh`, `apps/fyne/go.mod`, `deploy/helm/bin/{actualizar-suite,configurar-git-root,common}.sh`

**Context.** 0058 ported CI to GitHub with two things pending a decision and one
contradiction: the jobs went to GitHub-HOSTED runners (`ubuntu-24.04-arm`), which in
a private repository charge per minute and contradict the owner's decision of
2026-09-23 («el CI va entero en el frontal», “CI runs entirely on the front node”), and it left the image
registry and the Fyne forks on git.gay. On 27/09 the owner decided that git.gay stops being used for
everything and that the forks move to `github.com/thinkddr`, private.

**Decision.**

1. **Runners:** GitHub's `actions/runner` (ARM64) on the front node, `sytue-global`, `sytue-second`, `sytue-third` and, since 2026-09-28, `sytue-fourth` (the owner approved the fourth: three fell short for a full round),
   with the `sytue-arm64` label, the `sytue-runner` user and the usual rootless CI
   Docker. Each job in a container of that Docker (`container: &ci` in the
   workflow, with the filtered proxy as a socket and the host's Go cache mounted), which
   is what forgejo-runner did with its `config.yml`. Registered once with a one-hour
   token; no GitHub token remains on the host.
2. **Registry:** `ghcr.io/<dueño>`, published with `GITHUB_TOKEN` (`packages: write`).
   Private packages, linked to the repository.
3. **Forks:** `thinkddr/{fyne,typesetting,glfw}`, private, with all their branches and tags;
   they are read by `SYTUE_GIT_FORKS_TOKEN` (fine-grained, read-only on the three). The
   typesetting one declared its git.gay path in `module`, so it carries a new tag
   (`v0.3.4-sytue.2`) that only changes that line.
4. **Profile:** each release publishes `completo` (the pilot turns on Cloud), and the
   updater accepts a `completo` marker also on a server without Cloud, because it
   carries all the `piloto` images.
5. **Pilot:** same mechanism (`desplegable/…` marker on the remote, images from the
   registry), with the remote over SSH to GitHub and the same deployment key, registered
   as a read-only Deploy key.

**Alternatives ruled out.** _Hosted runners_: paid for private repositories, and the owner had already
decided the opposite. _The GitHub runner inside a container per job
(ephemeral)_: more faithful, but it requires a repository administration token stored on
the host to register on each job; the runner's own per-job container gives the
same isolation without that credential. _Public images on ghcr.io_: they save the
pilot's credential, but they publish the binaries of a private product; that is the
owner's decision.

**Consequences.** The GitHub runner always mounts `/var/run/docker.sock` in the
job container and has no option to avoid it; in the rootless Docker that path is
the production socket, inaccessible from the CI's user space (measured), and the
proxy goes through `DOCKER_HOST`. When stopping, the GitHub runner cancels its job (it does not wait),
so `drenar-y-reiniciar.sh` waits until there is no `Runner.Worker`. The front node's
pruning recognizes GitHub jobs by their `github_network_*` network. A workflow with only
`workflow_dispatch` cannot be dispatched if it is not on the default branch, which is
why it has to be `main` (it has been since 2026-09-28). The Forgejo runners were removed
from the front node the same day, 27/09 (`retirar-runners-forgejo.sh`; `instalar-runners.sh` brings them
back). **git.gay is archived, not deleted** (owner, 2026-09-28): the repository goes read-only
and its runner registrations are removed, but only once the first v3.7.x is in
production deployed from GitHub. The steps, in
[operations §2.1e](operations.md#21e-archivar-git-gay).

<a id="0061"></a>

<a id="0061--sytue-cast-vídeo-bajo-demanda-y-directo-con-portada-pública-y-cuota-de-drive"></a>

## 0061 — Sytue Cast: on-demand and live video, with a public front page and Drive quota

`accepted` · 2026-09-28 · product, video, live, multi-tenancy, storage · owner's decisions of 27/09 · code in `services/cast/**` (cast mig. `0001`–`0003`), `platform/pkg/{livekit,personas}`, `platform/pkg/objectstore/rango.go`, `deploy/helm/charts/cast`, id mig. `0100`, `apps/frontend/apps/cast`, `apps/fyne/{cmd,apps}/cast`

**Context.** The suite had no video. Rooms recordings ended up in Drive as
loose files, without a player, and whoever wanted to publish a video or stream from OBS had
to go to a provider outside the EU. The owner asked for an app “like Vimeo”: upload,
publish and watch, stream live, with a public front page and a channel per organization, and the bar
“100 % functional and beautiful”.

**Decision.**

1. **One service, two processes.** `services/cast` (:8099) serves the API, `/media` and the chat.
   The same binary, with the `transcoder` subcommand, takes jobs from `transcode_jobs` and runs
   `ffmpeg` in a second image (debian-slim). The queue is Postgres: `FOR UPDATE SKIP LOCKED`
   in a maintenance transaction (it crosses organizations), a single active job per video
   and class (unique partial index), three retries with waits of 30 s, 2 min and 8 min, and
   `running` jobs older than three hours go back to the queue, because the replica that had them
   died. Finishing a job and moving the video happen in the same transaction.
2. **Derivatives:** H.264 HLS (fMP4, 4 s segments) in 1080p (if the original allows it), 720p and
   360p; VP9/Opus 720p WebM for Fyne; poster. The original is kept. _Extended on
   2026-09-28:_ both ladders carry the same qualities, those chosen by the uploader
   (`qualities` on creation and `PUT /videos/{id}/qualities`); the WebM goes one per quality at
   `/media/…/webm/<q>p.webm`, and `webm/720p.webm` remains valid; the video exposes
   `qualities`, `hls_qualities` and `webm_qualities`, play `webm_qualities`, and
   `GET /videos:estimate` gives how much each one will take (cast migration `0007`). Earlier videos
   get the whole ladder (owner, 28/09: all of them), little by little from the sweep
   (`CAST_SWEEP_BACKFILL_QUALITIES`).
3. **Four visibilities:** `public` (front page and channel, indexable), `unlisted` (with link,
   embeddable), `org` and `people` (people by @username, from any organization, with the
   externals criterion of [0040](#0040)). The whole access decision lives in
   `internal/acceso`. The transaction always runs in the **video's** tenant, not the token's.
4. **`/media` with its own token:** HMAC-SHA256 over `vídeo|alcance|caducidad`, 6 h. The scope
   (`ver` or `original`) prevents a playback link from downloading the original. This point
   departs from the initial contract, which only signed video and expiry.
5. **Live:** LiveKit Ingress (RTMP for OBS at `cast-ingest.<dominio>:1935`) and Egress alongside
   the Rooms SFU. The LiveKit package moves from Rooms to `platform/pkg/livekit`. The room is
   `cast-<tenant>-<vídeo>`. The recording comes back as on-demand video through the same queue. The
   egress is **ParticipantEgress** and not TrackComposite: it records the broadcaster with their audio without setting up
   a web template.
6. **Quota shared with Drive:** Cast asks Drive through the internal door before accepting
   an upload (507 with `available_bytes` if it does not fit) and reports usage to it. Cap of 8 GiB per
   video. If Drive answers that there is no room, or rejects the query, nothing is uploaded. If Drive
   does not answer (network, timeout or 5xx), the upload is accepted and logged: a
   Drive outage cannot bring Cast down.
7. **Anonymous access** (front page, channel, public page and public comments) is in
   `/api/v1/public/*`, which is what the ingress publishes. The unprefixed `/public/*` is only served
   inside the cluster, to the BFF, with its own per-IP limiter.
8. **People by @username:** the `people:resolve` client that Rooms had moves to
   `platform/pkg/personas`, shared by both.

**Confirmed by the owner on 28/09**, as implemented: the “live” notification
only reaches the video's people list; the improvised title of an untitled live stream is
in Spanish; `usageInDrive` counts the bytes of the original plus the derivatives; the bytes of a
live stream recording count the same as an upload; `poster:frame` replaces the custom
poster; and the audio is muxed into each HLS variant.

**Drive offers “Upload to Cast”** (owner, 28/09) on its video files. It is an explicit COPY
and not an “open in place”, like those of Photos or CertiPDF: the video needs its
transcoding and its derivatives in Cast, and the Drive file stays where it is. The copy
counts against the same plan quota. It ships on the web and in the native app at the same time, like every Cast
feature: 1:1 parity between web and Fyne is mandatory, and the only exception is embedding.

**Decided by the owner on 28/09** (the web as delivered):

1. A live stream's chat can only be read with a session; without one, the page shows «Entra para
   participar» (“Sign in to take part”). It is the same requirement the service sets: the WebSocket requires a token.
2. What is uploaded is born with `org` visibility. Making it public is an explicit step by whoever
   uploads it.
3. Cast is listed as `disponible` in the catalog since the web is complete, without waiting for
   the native app (`nativa: false` until it arrives).

**Extended on 2026-10-03: “Present with Cast” from Sytue Diapos** (an Office roadmap item,
`diapos-cast`; the most conservative option by default, pending the owner). A WebRTC live stream can
carry a presentation:

1. **Who broadcasts, where.** Diapos schedules the live stream in Cast (`POST /lives` with `source=webrtc`,
   `org` visibility) from its BFF (`/api/cast-presentar`, with the presenter's token) and opens
   Cast's broadcast page with `?live=<id>`, where the screen is shared as in any browser live stream.
   The recording comes back as on-demand video through the same queue.
2. **What travels: only the position** (number, total and title of the slide, and the reference
   `docs:<id>`). Neither the content nor the speaker notes leave Diapos: the notes stay in the
   presenter's moderator view. The BFF rebuilds each field and the service rejects what it
   does not know.
3. **API** (`services/cast/internal/diapositivas`, migration `0008`): `PUT /lives/{id}/slide`
   —only the live stream's owner, scheduled or on air— and `GET /videos/{id}/slides` (and
   `/public/…`) for whoever can see the video through the same `acceso` as playback. Each change
   is a chapter (`slide_marks`, up to 2000, with the ms since the live stream started); the
   presentation (`live_presentations`) stores who launched it and when, and the first slide
   leaves a line in the log. Cast has no compliance audit log (the one in
   `platform/pkg/auditlog` is Mail's and Drive's): that row and that line are the trace.
4. **Viewers** see «Diapositiva n de N · título» (“Slide n of N · title”) under the live stream (3 s polling, without a session if it is
   public) and, in the recording, the list of chapters to jump to each slide.

Web only: parity with the native app (Fyne) is pending.

**Consequences.** There is one more database, one more bucket and two more Deployments. The transcoder
is the only pod in the suite with `ffmpeg`, so it is the one to watch on every ffmpeg
CVE. The Drive quota becomes the whole suite's, and the day another product stores
files it will have to come in through the same door. The number of this ADR was reserved as
0057 in the work contract, but it was assigned to another decision the same day. The code's
references point here.

<a id="0062"></a>

<a id="0062--sytue-web-usa-un-ir-validado-y-publicación-estática-inmutable"></a>

## 0062 — Sytue Web uses a validated IR and immutable static publishing

`accepted` · 2026-09-30 · Sytue Web, security, performance

The editor, the AI and the exporters exchange the same versioned IR. Publishing validates
the IR, blocks demo content and incomplete legal data, and writes a versioned static
output; a preview is another artifact with a token and an expiry. That way no customer
content is executed on publishing and a draft is never mixed with the live version.

<a id="0063"></a>

<a id="0063--los-dominios-propios-se-verifican-antes-de-entrar-al-borde"></a>

## 0063 — Custom domains are verified before entering the edge

`accepted` · 2026-09-30 · Sytue Web, DNS, security

Each domain requires a control TXT record and a CNAME/A target indicated by Sytue Web. Only
verified domains can pass through `/_borde/permitido`; the apex and `www` are kept as a pair to
prevent an unverified variant from capturing traffic.

<a id="0064"></a>

<a id="0064--la-analítica-de-sytue-web-es-de-primera-parte-y-visitantes-es-una-estimación-diaria"></a>

## 0064 — Sytue Web analytics is first-party and visitors is a daily estimate

`accepted` · 2026-09-30 · Analytics, privacy

Sytue Web registers its hostname internally in Analytics and serves the collector under `/_sytue/`.
The dashboard receives only aggregates and must call visitors a “daily estimate”: the salt rotation breaks
correlation across days and no period uniques are promised.

<a id="0065"></a>

<a id="0065--el-código-exportado-consume-el-contrato-público-generado"></a>

## 0065 — Exported code consumes the generated public contract

`accepted` · 2026-09-30 · Sytue Web, export, interoperability

React, Next, Astro and native clients are generated from the public routes table. Forms and
checkout solve ALTCHA through that client; URLs, fields and secrets are not duplicated in a
target template.

<a id="0066"></a>

<a id="0066--el-ci-construye-con-un-buildkit-sin-root-y-propio-no-con-el-builder-clásico-por-el-proxy-de-docker"></a>

## 0066 — CI builds with our own rootless BuildKit, not with the classic builder through the Docker proxy

`accepted` · 2026-09-28 · CI, security, disk · owner's decision of 28/09 («pasar a BuildKit», “move to BuildKit”) · code in `deploy/rooms-sfu/docker-ci/{sytue-buildkit-ci,sytue-buildkit-ci-puente,sytue-docker-ci-proxy,sytue-docker-ci-proxy-unix}.service`, `deploy/rooms-sfu/docker-ci/buildkitd.toml`, `deploy/rooms-sfu/{instalar-docker-ci,podar-ci}.sh`, `scripts/{construir-imagen,publicar-imagen-go,publicar-imagen-frontend}.sh`, `.github/workflows/{imagenes-produccion,security,imagenes-rama}.yml`, `.github/actions/toolchain/accion.sh`

**Context.** The front node's jobs talk to Docker through a closed-list HTTP proxy
(tecnativa/HAProxy): no containers, no exec, no volumes. BuildKit opens `/session` with
an h2c _upgrade_ that HAProxy cannot relay without turning the filter into a tunnel, so
builds used the classic builder (`DOCKER_BUILDKIT=0`). The classic one has no
leases: its cache is dangling images, and the host pruning (`podar-ci.sh`, #96) could
take away from a running build a dangling image older than 2 h that it was reusing. It was an
accepted risk in exchange for not filling the disk.

**Options, measured on the front node.**

1. **Opening `/session` in the proxy.** tecnativa v0.5 has no such option: the HAProxy configuration would have to be
   written by hand and the h2c relayed raw. What goes through
   that tunnel is no longer seen by the filter: it is the entire BuildKit API inside Docker, with the
   permissions of the CI Docker (Docker's grants `network.host` by default). And the
   cache would still be that of the daemon it shares with the jobs' containers. It is
   ruled out: the filter stops filtering precisely where it can do the most.
2. **One `buildkitd` per job, inside the container** (what `imagenes-rama` and
   `release.yml` did on the old runners). The job container does not ship `buildkitd` and cannot
   mount its layers without taking away the seccomp profile from ALL jobs —any branch
   could mount—, and every job would start with an empty cache. It is ruled out.
3. **A shared rootless `buildkitd`, with its own socket.** This is the chosen one. `docker
buildx --driver remote` and `buildctl` talk to it the same way; `buildctl` is used, which is a
   static binary with its sha256 in `versions.env`, because the Debian `docker.io` that
   the jobs' image installs does not ship `buildx`.

**Decision.**

1. **`sytue-buildkit-ci.service`:** `buildkitd` v0.33.0 under `rootlesskit`, as the user
   **`sytue-buildkit`** (its own, no shell, with its subordinate range 265536–331071), with
   `--net=slirp4netns --disable-host-loopback` and the data in `/srv/ci/buildkit` (the CI
   volume, 0700). `overlayfs` snapshotter in user space, `process-mode sandbox`.
   No insecure entitlements (`insecure-entitlements = []`: neither `security.insecure` nor
   `network.host`).
2. **The jobs' socket:** the daemon's belongs to `sytue-buildkit` with 0660 and the jobs do not
   open it. It is offered by `sytue-buildkit-ci-puente.service`, a `socat` running as
   `sytue-runner` with the `sytue-buildkit` group as its only extra, at
   `/srv/ci/run/buildkitd.sock`; each workflow that builds mounts it at
   `/run/sytue-ci/buildkitd.sock` and announces `BUILDKIT_HOST`. Nothing running as root touches the jobs:
   the Docker proxy bridge (`sytue-docker-ci-proxy-unix`), which ran as root, also moves
   to `sytue-runner`.
3. **The Docker proxy closes `/build`** (`BUILD=0`): the classic builder is no longer within
   reach of any job. That is what makes the accepted pruning risk disappear by
   construction and not by discipline.
4. **`scripts/construir-imagen.sh`** translates the `docker build` form to `buildctl`
   (`--push` uploads, `--tar` exports in `docker save` format for trivy). Docker media types
   (`oci-mediatypes=false`) and no attestations, so that `crane append/rebase/index` and the
   pilot's updater do not notice the change. The image does not go through any daemon: gone are
   the local tags, `soltar_*` and the «Borrar las imágenes de este turno» (“Delete this turn's images”) step.
5. **The cache collects itself** (`buildkitd.toml`: 80 GB at most, 40 % of the volume
   always free, 10 GB that are not touched) and the host pruning lowers it with
   `buildctl prune --keep-duration <2 h con jobs, 30 min sin ellos o en emergencia>
--free-storage <objetivo>`, which does not touch what a build has in use. No job prunes.
6. **Multi-architecture, same as before:** Go services are cross-compiled with `go build` and
   assembled with `crane`; Analytics builds its Dockerfile natively (`-base-arm64`) and
   puts the amd64 binaries on top (#86); a runtime stage with `RUN` (the Cast
   transcoder's `ffmpeg`, #103) is built for its architecture with `--platform` and the
   base pinned to that variant's digest, emulating only its `RUN`s with the front node's
   `qemu-user-static`; the frontends cross-build with `bun install --cpu x64` and
   `crane rebase`.

**What a job can do, before and after.**

|                                             | Classic builder through the proxy                                                        | CI BuildKit                                                                                       |
| ------------------------------------------- | ---------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- |
| Docker API                                  | `/build`, `/images`, `/auth`                                                             | `/images`, `/auth` (no `/build`)                                                                  |
| A `RUN` that escapes its container lands in | the user space of `sytue-runner`, owner of the CI Docker and of the runners' credentials | that of `sytue-buildkit`, which owns nothing else                                                 |
| Network of a `RUN`                          | the CI Docker's slirp4netns                                                              | its own slirp4netns, without the front node's `127.0.0.1` (measured: `wget 127.0.0.1:2377` fails) |
| Pruning everyone's cache                    | `POST /build/prune`                                                                      | `buildctl prune`, with leases; forbidden by the `ci-sin-azar` gate                                |
| Root processes that talk to it              | the proxy's `socat`                                                                      | none                                                                                              |

**Measured on the front node on 28/09** (`imagenes-produccion` workflow dispatched on a canary
branch that is not merged, with three to five images and without marking anything as deployable;
checked from the inside: `app/*`, `bun`, `ffmpeg` and `ffprobe` are ELF binaries of their
architecture, and `sharp` is the x64 one):

| Image                                                        | Classic, warm cache (v3.8.2) | BuildKit, empty cache | BuildKit, warm cache  |
| ------------------------------------------------------------ | ---------------------------- | --------------------- | --------------------- |
| `sytue-id` (crane, no Dockerfile)                            | 22 s                         | 20 s                  | 17–20 s               |
| `sytue-analytics` (native base + amd64 binaries)             | 2 min 04 s                   | 1 min 28 s            | 1 min 13 s–1 min 18 s |
| `sytue-frontend-portal` (cross-built amd64 + `crane rebase`) | 2 min 32 s                   | 1 min 17 s            | 1 min 03 s–1 min 15 s |
| `sytue-frontend-portal-com` (native arm64)                   | 2 min 48 s                   | 53 s                  | 21–27 s               |
| `sytue-cast-transcoder` (emulated `RUN apt-get`, #103)       | —                            | 5 min 32 s            | —                     |

The first frontend of each turn with the classic builder took ~9 min 30 s (v3.7.12, three
turns at once); with BuildKit the `deps` and `prod-deps` stages run in parallel and the
cache is shared. **Disk:** the entire v3.8.2 publication (classic) took `/srv/ci` from 50
to 75 GB and left 101 images in the daemon, 29 GB of them dangling, which only pruning collects;
with BuildKit the daemon does not grow with builds and the BuildKit cache, after four
canary publications (five images), took up 9.8 GB, with a ceiling of 80 GB and its
own collection.

**The first release with BuildKit, v3.8.5** (28/09): the three turns of
`imagenes-produccion` published the 35 images in 8 min 35 s (with the classic one, 43 min in
v3.7.12 with the three turns at once, and 48 in v3.8.2), `cerrar` checked the
architecture of all of them and marked them deployable, and the three `trivy image` jobs of `security`
built and scanned with `--tar`. The CI Docker daemon did not gain a single image; the
BuildKit cache ended at 19 GB. That same day, with the four runners drained
(`sytue-drenar-y-reiniciar --parar`), `instalar-docker-ci.sh proxy` left the proxy without
`/build` (`POST /build` → 403) and its bridge running as `sytue-runner`.

**Consequences.** A tag cut BEFORE this change cannot rebuild its
images (its `imagenes-produccion` uses `docker build` and the proxy no longer opens `/build`): a
new version is cut. The jobs mount the bridges' sockets by inode: restarting a
bridge leaves them without Docker or without BuildKit, so it is only done with no jobs running
(`instalar-docker-ci.sh proxy` refuses if there are any). Neither of the two bridges depends
with `Requires=` on what is behind it (the Docker proxy one did and moves to `Wants=`),
so the proxy and the daemon can be restarted with jobs running. `imagenes-rama` and `release.yml` publish to the Scaleway registry, which
is being decommissioned: `imagenes-rama` already goes through this BuildKit, but, like `release.yml`, it builds the
runner's architecture (arm64) and the Scaleway nodes are amd64; it remains a question for the
owner whether they are ported to the multi-architecture publishers or retired along with Scaleway.
They are watched by `deploy/production/tests/buildkit-ci-test.sh` (rootless daemon, no loopback
or insecure entitlements, rootless bridge, proxy without `/build`, workflows that mount the socket),
`.github/tests/ci-sin-azar-test.sh` (neither the classic builder nor pruning of the shared one in CI),
`podar-ci-test.sh` and the publishers' tests, all seen red by sabotaging what they
watch.

<a id="0067"></a>

<a id="0067--el-catálogo-de-plantillas-de-sytue-web-crece-por-sector-se-genera-desde-código-y-lo-que-hay-que-cambiar-impide-publicar"></a>

## 0067 — The Sytue Web template catalog grows by sector, is generated from code, and what has to be changed blocks publishing

`accepted` · 2026-10-02 · Sytue Web, templates, gallery · owner's brief («cuantas más plantillas mejor, para los negocios pequeños y medianos de dueños con pocos conocimientos informáticos», “the more templates the better, for small and medium businesses whose owners have little IT knowledge”) · code in `scripts/pages-plantillas/`, `services/pages/internal/plantillas/` and `services/pages/internal/domain/legal.go`

- **One template per sector**, not cosmetic variants: they are written thinking of what the customer of that trade wants to see
  (menu, rates, emergencies, steps, opening hours, how to get there, appointment or quote) and with real, plain texts in es and en. 14
  categories for the gallery (`belleza`, `comida`, `comercio`, `oficios`, `motor`, `profesionales`, `salud`, `deporte`,
  `educacion`, `creativos`, `turismo`, `local`, `comunidad`, `web`). Each template carries `category` and `keywords` (commonly used
  synonyms per language) for the search box.
- **Languages: es + en**, which is the only thing `IDIOMAS_VISITANTE` allows. The other 24 languages are added with the
  translation piece, just like for the initial ten.
- **The JSON files are generated**: `lib.py` (pieces), `sectores/<clave>.py` (content), `looks.py` (AA palettes) and `generar.py`. The
  generated JSON is authoritative and is validated by `catalogo_test.go`; `vista-previa.mjs` renders the IR according to §4.2 to review the design.
- **Nothing made up that gets published by mistake**: no remote or stock images (placeholders only), no made-up reviews, figures,
  awards or certifications. Name, phone, email, address, map pin, «Tu ciudad» (“Your city”), professional association
  or registration numbers (`00-0000`) and every “example” note carry a `domain.TextosDemo` marker, and `live` publishing
  refuses with `demo_content` while any of them remains on the site. The test requires every visible text that gives instructions to be
  covered by a marker, and forbids marketing jargon and promises without data.
- **Gallery**: a search box that understands «pelu» or «barbero» (without accents, plurals, es and en), groups by category and preview in an
  isolated iframe. No preselected template.

<a id="0068"></a>

<a id="0068--sytue-bases-tablas-relacionales-colaborativas-como-producto-propio-con-acl-por-base-y-conector-de-solo-lectura"></a>

## 0068 — Sytue Bases: collaborative relational tables as a product of its own, with per-base ACL and a read-only connector

`accepted` · 2026-10-04 · Bases, permissions, interoperability · owner's decision (2026-10-01: «lanzarlo tras los P0», “launch it after the P0s”) · P1 row «Tablas relacionales» (“Relational tables”) of the office roadmap · code in `services/bases/` (Go API, :8102) and, in the following pieces, `apps/frontend/apps/bases/` (web, :4337)

- **Why a product and not a Calcus sheet.** A relational table needs what a sheet does not have: a schema with types that
  the server enforces, relationships with referential integrity, its own permissions and auditing, and views that are not a
  grid. Hiding it inside Calcus would mix the cell model with the record model. Calcus is a CONSUMER (piece D).
- **Model.** base → tables → typed fields → records. Types: text, number, date, checkbox, single and multiple select and
  link to a record of another table in the SAME base. A field's type does not change after creation (the data written would stop
  complying with it). Values go in `base_records.data` (jsonb by field id); links, in `base_links`, which is what
  allows Postgres to guarantee integrity.
- **Referential integrity in the database, not only in the code.** Foreign keys are COMPOSITE `(tenant, id)`:
  a foreign key check is done by the owner and skips RLS, so without the tenant in the key a foreign id
  would slip in a link between organizations. Those of the links also carry the table (`to_table_id` ↔ `link_table_id`), so
  that not even a link to a record of another table is possible. Deletion is safe: a record or a table pointed to by a
  link is not deleted (409); the owner can force deleting a record, which removes its links; deleting the entire base does
  drag everything along (deferred keys, see the migration).
- **ACL consistent with Docs**: owner > editor > commenter > reader, per base and optionally per table (the table
  entry ADDS to the base one, never subtracts; deny-by-default). The owner is the `owner_sub` and is not granted by ACL. The
  principals are a user or the WHOLE organization; there are no groups (the token does not carry them) and nothing is shared with other
  organizations. Editor: records and views. Owner: schema, import, permissions, audit, delete. The matrix is in
  `docs/security.md#bases-acl` and the test `TestMatrizACL`. The decision is ALWAYS made on the server, within the same
  transaction as the operation (one request, one transaction): there is no gap between checking and writing. Without access it is 404
  (it is not revealed that it exists); with insufficient access, 403.
- **Views**: grid, Kanban (by a select field) and form. Their configuration (filters, sort, grouping) is validated
  against the real fields when saved and is NEVER concatenated into SQL: field ids and values travel as parameters, the
  operators are a closed list per type and the sort direction is one of two constants (`TestConstruirConsultaNoConcatenaEntrada`).
  `contains` uses `position()`, not `LIKE`: wildcards mean nothing.
- **Audit** of every operation (who, which record and field, before and after capped at 1 KiB), also of import and
  export. Without foreign keys: it survives the deletion of what it audits. Only the owner reads it.
- **CSV import/export.** Importing creates ONE new table (schema → owner) with types inferred per column (number,
  ISO date, checkbox or text; `+34…`, `0x10` or `1_000` are text). Limits: 5 MiB, 10,000 rows, 50 columns, 10,000 characters
  per cell, UTF-8 without NUL, all or nothing. A value starting with `=` goes in as text: the defense against “formula
  injection” is on EXPORT (a `'` is prepended to texts starting with `= + - @`, TAB or CR; numbers are not touched).
- **Limits** (each one protects a query or a body): 200 bases per organization, 50 tables, 100 fields, 1,000,000
  records per table (it was 50,000 until bases-2), 50 views, 200 permissions; pages of 500 at most; every query carries
  LIMIT and none OFFSET.
- **Cursor pagination (bases-2, piece 1).** Records are paginated by cursor (keyset): `GET /tables/{id}/records`
  returns `next_cursor` and accepts it in `?cursor=`; `offset` no longer exists. The cursor is opaque (base64url of a JSON) and carries
  the value of each key of the view's sort in the last record delivered plus its `created_at` and its id, which always
  break ties. **It is not signed because it grants nothing**: authorization is that of each request, and whoever forges it only chooses
  where to continue reading a table they can already read; its values are validated in Go before reaching Postgres (422, never
  a 500) and travel as parameters (`TestCursorHostil`). It is **stable with the view's sort**: it carries the signature of that sort
  and, if the view changes its sort between two pages, it is rejected and the client starts over; deleting the record where it
  stopped neither repeats nor skips anything (`TestCursorEstableAnteCambios`), which is what an OFFSET does not guarantee. Texts sort
  by their first 128 characters: that way the key fits in a cursor that travels in the URL (a text allows 10,000); those that tie
  there are broken by age. The audit already went by descending id (`?before=`), which is a cursor, and a
  table's views are 50 at most and are listed at once.
- **Indexes and measurements** (200,000 records, the agents' Postgres, best of 5, `TestMedirPaginacion` with `BASES_MEDIR=1`):
  in the default sort the page comes out of `base_records_por_tabla (tenant, table_id, created_at, id)` without sorting anything —the
  first one, 1 ms of query; a deep one goes from 92 ms (OFFSET) to 3.6 ms for the whole operation (ACL, fields and links
  included)—. In a view SORTED by a field, the deep page drops from 253 ms to 95 ms and costs the same as the first
  (86-100 ms): it is a table scan with top-N, because the key is `data ->> <campo>` with the field as a parameter and an
  expression index per field would require DDL for each field someone sorts by. Noted ceiling: if sorted views of
  large tables become heavy, an expression index created when saving the view. The record ceiling was checked by counting the table
  on each insert (64 ms with 200,000); now it is carried by `base_tables.record_count`, maintained by a per-statement trigger on all
  paths (grid, public form, import, cascades), and the check costs 0.5 ms.
- **Export in chunks and links with their value (bases-2, piece 2).** Exporting no longer loads the table into memory: it walks
  the table by cursor in chunks of 500, each in a short transaction that re-authorizes (no transaction remains
  open while someone downloads slowly, and removing their access cuts the export at the next chunk). It is audited at
  the start, with the table's record count. If a chunk fails with half the file already sent, the connection is cut so
  that the download fails instead of cleanly closing an incomplete file. A link field comes out with the TEXT of the linked
  records (their primary field: the first one with a value, the same one the interface and the connector show) as well as their id: in
  CSV, two columns —`<campo>` with the texts and `<campo> (id)` with the ids—; in JSON, `link_values` next to `links`, in the
  same order. The text of a linked record was written by someone in another table and goes through the same formula
  neutralization as any other (`TestExportarEnlacesConValores`).
- **Formula fields (bases-2, piece 3).** A field of type `formula` stores an expression and the type of its result
  (`number` or `text`, which does not change, just as no field's type changes). **It is evaluated on the server**, with the suite's
  formula engine (`apps/nucleo/formula`, Calcus's; there is no second engine), and the result **is stored in the
  record** like any other value: that is why filters, sort, cursor, export and connector see the same thing without knowing it is
  a formula, and the browser computes nothing. Nobody writes it from outside (422). The language is that of a spreadsheet,
  trimmed down: the only references are to fields, in braces and BY ID —`{campo}` of the same record (a link is worth the
  number of linked records) and `{enlace.campo}`, the values of that field in the linked records, for `SUM`,
  `COUNT`, `MIN`, `MAX`…—; cells, ranges, names and arrays are rejected (`formula.Inspeccionar`, new in the engine, says
  what a formula uses without evaluating it), and only the 37 functions of a closed list are allowed: pure, without clock or randomness (nothing
  volatile), without network, without loops or arrays and none that produces huge texts. Each value enters the engine as a typed
  literal: a text starting with `=` is a text. **Cost bounded by construction**: 1,000 characters, 200 nodes, 20
  formulas per table and 100 values per aggregate. **Cycles**: they are rejected when saving, between fields of a table and going
  around through others (dependency graph of the whole base); if one appeared anyway, the engine leaves it empty. A field
  used by a formula is not deleted (409). An error (`#DIV/0!`, overflow) or a result of another type is left empty.
  **Recalculation**: the fields and links of the record itself, in the same write; when a LINKED record changes, those
  that link to it and those that depend on them are recalculated, in the same transaction as long as they fit in a budget of
  500 records; if they do not fit —or when creating or changing a formula in a large table— the table is noted in `base_recalc` and a
  service loop walks it in chunks of 500 (`SKIP LOCKED` across replicas); meanwhile the page says so
  (`recalculating`). A recalculation does not bump `rev` nor is it audited: it is nobody's edit. Defining or changing a formula is
  schema (owner). Like a link's label, an aggregate shows whoever reads a table a piece of data derived from another one
  they may not be able to read: the owner decides that when defining it. Noted ceilings: the interface creates formulas but does not
  edit them (the API does, `PATCH /fields/{id}`), there is no date-typed result and the error code is not stored.
- **Attachments (bases-2, piece 4).** A field of type `attachment` stores files. The bytes go to the suite's object
  store (`platform/pkg/objectstore`, bucket `bases-blobs`, the same `S3_*` variables and the same provider encryption at
  rest as Drive or Docs; there is no encryption of our own) and in Postgres the record remains (`base_attachments`: name, type, size,
  SHA-256). **Nobody chooses the object key**: it is `bases/<base>/<adjunto>` under the organization's prefix. **The
  type is determined by the content** (`http.DetectContentType` over the bytes; the uploader's `Content-Type` header is not even
  looked at) and nothing a browser or a system would execute gets in: HTML, SVG, scripts, executables (by content and by
  extension), nor a file whose content is not what its name promises (`foto.png` that is a PDF). **Limits**: 20 MiB per
  file, 20 files per record and field, 2 GiB per base (quota), two concurrent uploads per replica (the file is read
  entirely into memory). **The download is served by the service**, not a signed URL: permission to read that record is checked
  on each download, the attachment has to belong to THAT record (knowing its id is not enough), and it always comes out as a download
  (`Content-Disposition: attachment`, `nosniff`, CSP `sandbox`). Uploading and deleting is for editors; reading, for readers. PNG,
  JPEG and GIF images of up to 8 Mpx get a thumbnail (`platform/pkg/miniatura`, with its anti-bomb barrier). **Safe deletion**:
  the record cascades with its record, field, table or base, and a trigger notes the key in `base_attachment_purge`, from
  where a service loop deletes the object and its thumbnail; no deletion path has to remember the store. Uploading
  is three steps (reserve the key in that queue with an hour's margin, upload, confirm with the record locked), so
  a half-done upload leaves neither a record without an object nor an object without an owner. In the export the name of the files comes out
  (neutralized like any text); no filtering or sorting by an attachments field, and a public form does not
  accept them. Without a configured store the service starts and attachments respond 503. Noted ceilings: no antivirus, no
  deduplication by digest, and after an organization merge the object stays under the prefix with which it was uploaded
  (`store_tenant`).
- **Windowed grid.** The web keeps at most three pages of 100 (`lib/ventanas.ts`) and remembers the cursor with which it
  requested each one: «Cargar más» (“Load more”) adds the next one and drops the first; «Cargar anteriores» (“Load previous”) requests the previous one again with its
  cursor. With buttons and not infinite scrolling: they work equally with keyboard and screen reader, and the loaded range is announced.
- **Board and form (piece C).** The Kanban board groups by a select field; moving a card means writing that
  field, with the card's «Mover a» (“Move to”) selector (it works equally with mouse, keyboard, screen reader and mobile; no drag and drop). The form is a view
  with the requested fields and, to collect customer data, **revocable public links**: a 32-byte token of which
  only the hash is stored, a submission quota per link, optional expiry and an ALTCHA proof of work (`platform/pkg/altcha`,
  without cookies or third parties) bound to the link. Only the owner publishes and revokes. A public form never fills in links
  between tables or fields the view does not include (they are rejected, not ignored), and the record is audited with the actor
  `form:<enlace>`.
- **Read-only connector to Calcus (piece D).** The owner grants a CONNECTION to a table or to one of its
  views: a token (`sbc_…`, of which only the hash is stored) that always expires (90 days by default, 365 at most), can
  be rotated (bumps `version`) and revoked. Calcus stores it encrypted as the credential of a `bases` source of its external
  data and sends it to a FIXED root (service config `BASES_URL`, never from the user). Sytue Bases checks it against
  the database on EVERY read —there is no cache—, so revoking cuts it off immediately. There is no write route for a connector
  token. Each read returns the PROVENANCE (base, table, view, who granted, when, version), which Calcus shows and
  stores with the table, and it is audited (`connector.read`, actor `conn:<id>`). Drive, ERP and Pages as consumers are left
  out: the same token and the same route would work for them, but each one needs its destination screen.
- **Accessibility audited in CI (bases-2, piece A).** `test/e2e/accesibilidad-bases.mjs` runs axe-core (WCAG 2.2 AA),
  keyboard, screen reader, 200 % zoom and touch targets at 320, 390 and 1280 px, in light and dark, over the
  component benches (`/bases/dev/rejilla`, `/bases/dev/vistas`) and over the FULL SCREENS (`/bases/dev/app`: the real islands with
  the frame and the API answered by the test): list of bases, open base, board and sharing. It runs in the
  `accesibilidad-office` job of `ci-frontend.yml`. A new screen is added with one line in `PANTALLAS`. The touch bar is
  44 × 44 px with a coarse pointer (rule in `styles/app.css`); a grid cell is content and stays at 24. The header
  and the sidebar belong to the common frame and are not measured here.
- **Connectors to Drive, SyERP and Sytue Web (bases-2, piece B).** A connection belongs to ONE consumer (`consumer`:
  `calcus`, `drive`, `erp`, `web`; migration `0020`), and the three new ones require scoping it to a VIEW: what leaves Bases
  towards a file, a report or a public site is what the owner left in a view, never the whole table. The
  consumer identifies itself when reading (`X-Sytue-Consumer`) and says which organization it reads for (`X-Sytue-Tenant`): if they do not match
  the connection, the read is the same 401 as a revoked token and is audited as `connector.denied`. It is not service
  authentication —the route does not leave the cluster and the credential is still the token—: it is what prevents the token granted to
  a report from publishing a site, or from being used from another organization (Bases is not shared across organizations). Calcus
  keeps what it had (whole table, without an organization header) because its connections predate this. The client is ONE,
  `platform/pkg/basesconn` (request to a fixed root, token format checked before sending anything, caps, provenance that has to name the
  consumer, and CSV with the formula defense): the consumers import it. Tests:
  `TestConectorPorConsumidorVistaYOrganizacion` (a token does not read another view or another base, nor is it valid for another consumer or
  organization; revoking cuts it off at once) and `TestConectorPorHTTPExigeConsumidorYOrganizacion`.
- **SyERP consumes a view as a report source.** Administration stores the token with `POST /conexiones/bases`
  (sealed with `keycrypto`, like the other `conexiones` secrets); the server READS with it before storing it —that way it knows it
  is for SyERP and for this organization— and notes the columns and the provenance. The `bases` source only exists for
  owner/admin: the token belongs to the organization, not to the person, so Bases cannot decide for each reader and
  SyERP decides it with the same criterion as for storing it. Each report reads again (no cache: revoking in Bases gives “not available”
  on the next read) and the table carries the provenance of THAT read, which the interface shows under the table. Ceiling: one
  view per organization (the key of `conexiones` is the provider) and no screen to paste the token (today, via API).
  `TestFuenteBasesDeLosInformes`.
- **Sytue Drive exports a view to a folder.** `POST /folders/{id}/imports/bases {token}` («Nuevo → Vista de Sytue
  Bases», “New → Sytue Bases view”): Drive PULLS from Bases with the token of a connection granted to Sytue Drive, writes a CSV (with the formula
  defense of `basesconn.CSV`; name sanitized from table and view) through the path of any upload —quota,
  deduplication, versions— and records the PROVENANCE per version in `drive_file_provenance` (migration `0029`, forced RLS,
  composite foreign key `(tenant, file_id)`), which is read with `GET /files/{id}/provenance`. Pull and not push: that way the
  provenance is stated by Bases in its response and not by a field the caller could invent, and Bases does not gain an outbound
  dependency. The token is used once and is NOT stored: refreshing means exporting again (a new version of the same file), and revoking
  the connection prevents doing it without touching what was already exported. Ceiling: only “My drive” (team drives and shared
  folders have their own upload path) and the provenance is not yet shown in the file details panel.
  `TestExportarVistaDeBases`.
- **Sytue Web lists a view's records in a section of the site, as a SNAPSHOT.** «Insertar → Tabla desde
  Sytue Bases» (“Insert → Table from Sytue Bases”) asks for the token of a connection granted to Sytue Web and `POST /sites/{id}/bases/table {token}` returns the view
  as the body of a regular `table` node, which goes into the draft and is rendered and validated like any other table (each
  cell goes through the renderer's escaping). There is no new node in the IR, no query when serving the site, no stored token:
  what is published is exactly what the editor saw. Only owner/admin (the same people who publish): what goes in ends up on a
  public page. A view that does not fit (50 rows × 8 columns) is rejected entirely, not trimmed. The provenance stays in the
  node's name (only visible in the editor) and in the base's audit. **Revoking the connection does not withdraw what was already published**
  —it is site content—: it prevents fetching it again. Ceiling: the data is refreshed by inserting the table again; a linked
  section that updates on publishing would require a new type in the IR (TS and its Go mirror), the token sealed per site and
  deciding what a publication does when the connection is no longer valid. `TestTablaDesdeBasesExigeAdminYTokenDeConexion`,
  `TestATablaDeSitio`.
- **Web only** (`nativa: false` in the parity): a conservative decision until the owner says otherwise; it is a grid
  screen that in Fyne would cost a piece of its own. **Superseded on 2026-10-05**: the piece was built
  (`apps/fyne/apps/bases`), the app is registered (`apps/fyne/cmd/bases`, `nativa: true`, scheme `sytue-bases`, Sytue ID
  migration 0109 for the login return on mobile) and it reaches the API through the Ingress of the last point of this ADR.
  What the web has and the native app does not (import, attachments, comments, connections, scripts, real time) is still
  debt, and on the phone it has only been seen at 360 width against the fake service.
- **Out of scope for the first delivery** (and why): ACL groups, transferring ownership, comments per
  record (the “commenter” role reads just like the reader until they exist). Cursor pagination arrived in bases-2.
- **On by default (2026-10-04, owner's decision).** It was delivered off and in `desarrollo`; since that date the
  catalog lists it as `disponible`, the chart deploys it (`bases.enabled: true`), the Hub announces it and the pilot brings it up
  unless `BASES_ENABLED=false`. Turned off it still leaves not a single object. Its two secrets are filled in by `generate-secrets.sh`,
  which `deploy.sh` runs before touching anything. **Production and staging (Scaleway) keep it off** in their overlays:
  their scripts do not yet create the Bases secrets and its DNS and certificate do not exist, and turning it on blindly would leave the pod
  unable to start; it is a pending decision for the owner, with the steps in
  [operations.md](operations.md#bases-encendido).
- **Adversarial review (piece F):** six findings fixed and eighteen mutations seen red, in
  [security.md](security.md#bases-adversarial).
- **Comments per record (bases-2, collaboration A · 2026-10-04).** Each record has a ONE-level comment
  thread (root and replies), in `base_comments` with the same rules as the rest: forced RLS, composite keys
  `(tenant, id)` and, in the reply, also the record —a reply cannot hang from a thread of another record or of
  another organization even if the code got it wrong—. With them the **commenter is finally distinguished from the reader**: they read and
  comment (`AccComentar`), they do not edit. **Deleting** follows the Docs rule (PR #353): whoever wrote it, while they can still
  comment, or whoever moderates (editor or owner); a deleted root that has replies leaves a tombstone without text or author,
  so as not to take down what others wrote, and it disappears with its last reply. Plain text of 1 to 2,000
  characters, 500 comments per record. **Mentions** in the suite's form (`@ana@empresa.eu`,
  `hubnotify.Mencionados`). **Notifications through the Hub** (`bases.comment`, `bases.mention`) to the base's owner and, on a
  reply, to those writing in that thread, never to the commenter; and **only to whoever at that instant can read the
  record**: the thread participants are filtered in the same transaction that saves the comment, and a mention is resolved in Sytue
  ID with the writer's session (it only answers for people in their organization) and checked against the ACL before
  notifying —the notification carries a snippet of the text, and sending it to someone who cannot see the record would be showing it to them—. **Audit**:
  `comment.create` and `comment.delete` with the record and the ids, WITHOUT the text (a deleted comment does not survive in the
  trail). The service now needs `HUB_BASE_URL`, `HUB_INTERNAL_SECRET` (mandatory outside development, from the
  `sytue-hub-internal` Secret), `ID_BASE_URL` and `PUBLIC_URL`. On the web, a side panel (the system `Modal` on the `right`) that
  opens from the row or the card and from the notification link (`?table=&record=`).
- **Groups as ACL principals (bases-2, collaboration B · 2026-10-05).** An entry can belong to a **Sytue ID
  group** (`principal_kind = 'group'`), as in Docs and Drive. Where membership comes from was investigated: **the token already
  carries it** —Sytue ID issues in the access token the `groups` claim with the names of the person's groups in the
  active organization, which is what Drive uses for sharing by group—, so **no server-to-server query is needed
  and Sytue ID does not have to be touched**: `authn.Claims.Groups` reads it (previously Drive re-read the payload on its own) and Bases passes it to
  `domain.ResolverRol`. The “short-expiry cache” is the lifetime of the token itself: joining or leaving a group takes effect
  when it is renewed, in minutes, and the live channel (piece C) closes when its token expires. Rules: the group is stored by its
  NAME, which is what the token carries (a known limitation noted for the owner: renaming a group in Sytue ID leaves the
  entry orphaned, and a new group with the old name would inherit it; moving to identifiers requires the token to
  emit them); it only applies within its organization (the row carries `tenant` and RLS); the classes are not confused (a group
  named like a `sub` is not that person); an unknown class does not apply; and at most 200 groups from the token go into the
  query. A comment notification to someone who only has access through a group does not go out (the service does not know the groups of a
  third party): deny-by-default for notifications too.
- **Granting to a person by their address.** `POST …/permissions` with `email` resolves the address in the
  Sytue ID directory with the granter's session (it only answers for people in THEIR organization) and stores what they are called
  (`principal_label`) for the list. Whoever does not manage permissions never gets to query the directory.
- **Transferring ownership.** The owner can give it to **another person in their organization**, identified ONLY by their
  address —the service never accepts a `sub` from the client: that the target belongs to the organization is stated by Sytue ID—. Whoever
  had it stays as **editor** (a conservative decision: they do not lose their work all at once, but they stop deciding schema,
  permissions, connectors and deletion; the new owner can remove them). The base's row is locked, so two
  simultaneous transfers do not step on each other. It stays in the audit (`base.transfer`, before and after) and the new owner
  receives a notification through the Hub (`bases.transferred`). On the web it asks for confirmation with the base's name and the target's
  address.
- **Sharing with other organizations** (building on [0040](#0040) and [0052](#0052)). **The base lives in the host
  organization**, with its rows, its RLS, its audit and its limits; the outside person is one more ACL entry, of class
  **`external`**, with their `sub`. The owner invites them by their **@username or their address**, which the service resolves with
  `people:resolve` (it gives the `sub` and what they are called; never their email or their organization); a `sub` pasted by the client is not
  accepted, and if the person turns out to be from the organization itself they are not “from outside” (`is_member`). **Bounded role**: reader,
  commenter or editor, on the base or on a table; never owner.
  - **Their token carries THEIR organization**, so which organization serves the request is decided by the service and **never
    by the client** (no header, no query, no body; 0040 ruled out the header): `enAnfitrion` looks at the route's resource
    (base, table, record, view or comment) and, only if it does NOT exist in the token's organization and does in one where the
    person is invited, the request continues in the host with `Actor.Externo`.
  - **There they are nothing more than their `sub`**: `ResolverRol` only counts the `external` entries in their name. Not “the whole
    organization”, nor a host group named like one of theirs, nor a person entry with the same `sub`, nor
    ownership even if the `sub` matches. Resolving the host grants nothing: the base's ACL is decided afterwards, in the
    operation's transaction, so **they see nothing else of that organization** (another base open “to the whole organization”
    gives them 404).
  - **The only cross-organization read** is “in which organizations is this person invited?”, through a
    read-only policy (`base_permissions_externo`) that only shows the `external` entries of the person declared in the
    transaction. It is remembered for 30 s per replica; that does not delay a **revocation** —the ACL is checked on every operation and
    removing the entry cuts off immediately—, only a freshly made invitation takes that long to be openable.
  - **In their list** the base appears after their own, with the host (`host_org`). **Audited** in the host: the
    invitation, the revocation and everything the guest does, with their `sub`.
  - **What was not done, and why.** Hub notifications to the outside person (their bell is in their organization, as in 0040) and the names of the host's people in comments when someone from outside reads (the directory answers
    per organization: they appear as «Alguien», “Someone”). An organization policy to close sharing with outside people is left for the
    owner.
- **Real-time co-editing (bases-2, collaboration C · 2026-10-05).** Other people's changes are seen instantly
  in the grid and on the board, and who is in each cell. **The pattern is Docs'** (PR #284-#305 and #369): versioned
  contract, ordered changes with replay on reconnect, per-cell operations with identifier and idempotency, presence
  and service authorization. **The transport is not**, and that is why no plumbing has been extracted from `services/docs/internal/collab`
  to `platform/pkg`: what is there is a WebSocket with in-memory rooms distributed through DragonflyDB and typed to CRDT
  operations, and **the Bases API had no Ingress** (it was born that way; since 2026-10-05 only `/api/v1` goes out, see the last point
  of this ADR), so a browser could not open a WebSocket to it. What was common and did exist is imported:
  `httpx.AbrirSSE` and the `Idempotency-Key` that the BFF proxy already lets through.
  - **Change stream over SSE through the BFF** (`GET /bases/{id}/events`, contract `v=1`; any other version, 400). The change
    log is **the audit itself**, which every write already fills in within its transaction: there is no second path that
    someone could forget. To follow it **without gaps** each row carries the transaction that wrote it (`xact`) and only
    those of transactions older than the oldest still in progress are delivered, in `(xact, id)` order: a slow transaction that
    commits late does not fall behind the cursor (`TestFlujoDeCambiosSinHuecosConTransaccionesLentas`). It works behind
    PgBouncer —LISTEN/NOTIFY does not— and the database is the only relay between replicas. Latency: one tick (1 s). **Known
    limit**: that horizon is for the ENTIRE Postgres instance, so a write transaction left open
    for a long time in any database of the instance (a migration that rewrites a table, a bulk load) DELAYS
    delivery while it lasts; it loses or reorders nothing, and editing keeps working through the API. If it ever became a nuisance, the
    path is a per-base sequence assigned under a lock, at the cost of serializing each base's writes.
  - **No data travels through the stream**: only what changed (action, table, record, field, who and which operation). The
    client requests that record again through the API (`GET /records/{id}`), which authorizes the read. Permission changes
    come out as `access`, without who or to whom.
  - **Authorization on every batch**: each tick resolves the listener's ACL inside the reading transaction, filters each
    change by the table they can read and, if they no longer have access, says goodbye (`bye access`) and closes: **a revoked permission
    cuts the open stream** on the next tick. Nor does it outlive its token (`bye token`; the browser reconnects with a
    new one, which says what is valid now).
  - **Reconnection with replay**: each event carries its cursor as `id`; `EventSource` reconnects on its own and sends `Last-Event-ID`.
    Whoever falls more than 500 changes behind receives `resync` and reloads.
  - **Per-cell operations** (`PUT /records/{rid}/cells/{fid}` with `Idempotency-Key`): they do not ask for the record's version,
    so two people in different cells of the same record do not get in each other's way; in the same cell **the last
    writer wins and is warned** if they overwrite a value they had not seen (`expected` → `conflict`). The identifier stays in the
    trail with a unique index: repeating the operation after a disconnect does not apply it twice (`replayed`), nor at once from eight
    places. Version-based editing (`PATCH /records/{id}` with `rev`) still exists for whoever edits an entire record.
  - **Presence** (`PUT /bases/{id}/presence`): the tab says which cell it is in; **who it is is set by the service** (from the
    token), the cell has to belong to a table they can read, and nobody uses another person's tab. It is stored in the
    database with a 30 s expiry (so all replicas see it) and each person sees the others' cell only in the tables they
    read; in those they cannot, they know the person is in the base and nothing more.
  - **Limits**: 8 streams per person and 200 per replica (each stream is a short transaction per tick), 100 present
    tabs per base, 500 changes per batch. Ceiling noted in the code: one room per base with a single poll if a base
    gathers many tabs.
- **User scripts (bases-2, automation · 2026-10-05; PR #432, #437 and #439).** On top of the common isolated runtime of
  [0069](#0069): the server does NOT execute code; the script runs in the browser of whoever launches it and everything it does is
  an ordinary request to this API with THEIR session. **There is no script credential or path of its own**, so a script
  can never do more than whoever runs it, without that having to be proven operation by operation.
  - **It hangs from a base.** `base_scripts`, `base_script_versions` and `base_script_runs` (migration `0030`), with the usual
    rules (forced RLS, composite and deferred keys). The run log carries no keys: it survives the
    deletion of the script and the base. It is not written to `base_audit` —that is the live stream's change log and a
    script is not a data change—; what the script writes does stay there, in the name of whoever ran it.
  - **Immutable versions and approval.** A version is `(código, SHA-256, manifiesto)`; editing creates another, as a draft,
    which does not inherit the approval (a trigger also prevents it in SQL). A script is `private` (only its author; it does not exist
    for anyone else, not even for the base's owner) or `shared` (run by whoever has access to the base, if the
    version is approved). **The base's owner approves**, with the hash of the code she reviewed, and she can approve her
    own (the owner's decision for Docs, as is). Not an organization administrator: in Bases they have no implicit
    access to bases, and giving it to them for review would open a new read; the owner already sees everything in hers.
  - **`sytue.v1` surface, short and closed**: `tables.list`, `fields.list`, `views.list`, `records.list`,
    `records.get`, `records.create`, `records.update` and `records.delete` (never forced). Out, on purpose: schema,
    permissions, sharing, connectors, forms, attachments, comments, export, import and the audit. The list lives
    in `domain.OperacionesDeScript` and in its web mirror, tied together by a test.
  - **Manifest** (`v: 1`): operations, tables of the base (empty = those the person can read), `write`, `max_ops` (up to
    500; here each operation is a server transaction) and `max_ms` (up to 30 s). A manifest that declares a
    writing operation without `write` is rejected entirely. No unattended execution: Bases has no triggers.
  - **Confined to its base.** The server does not distinguish a request from a script from one from the person, so the
    confinement is set by the web host: every table has to belong to the script's base and every record is fetched first
    and has to belong to an allowed table. Without that, an approved script could copy into its base another one that whoever runs it
    has access to and its author does not. The reasoning, in [security.md](security.md#bases-scripts).
  - **Fixed quotas and log**: the same as Docs (60 runs per person per hour, 600 per organization, 10 minutes
    reserved), under a lock. Each run records who, which version with its hash, result, duration, operations and a
    short code; denied attempts too, without spending quota. The owner reads it.
  - **Who**: anyone with a role on the base creates scripts and runs their own or the approved ones, a reader too (the
    server denies them those that write). Left out are whoever comes from another organization and whoever only has access to some table.
  - **On the web**: a “Scripts” button on the base and a panel with list, code, manifest, versions, propose, approve or
    reject, run with output, quota and log; consent is requested before the first run and
    after any manifest change. In all 26 languages. It is included in the Bases accessibility audit.
  - **Noted ceilings**: the code is edited in a text area without line numbers (the editor with line numbers belongs to Docs;
    move it to `@sytue/ui` if both need it); the manifest is written as JSON; a script cannot be renamed; reading
    is 100 records per page and 256 KiB per response; and there are no triggers (on saving a record, on a
    schedule): they would require unattended execution and another decision about who answers for what runs on its own. No Fyne app.
- **The API goes out through an Ingress, limited to `/api/v1` (2026-10-05, owner's decision; REVERSES the “no Ingress” of this
  ADR).** Until this date the API had no Ingress and only suite pods could reach it. It is reversed because **a native
  app has no way in**: the Fyne one already exists (`apps/fyne/apps/bases`) and talks to the API with the
  person's token, like Drive's and the rest, but the only path was the web BFF, which only accepts the session cookie.
  Having the BFF also accept the token was ruled out (two forms of session in a piece designed for one, and a path
  different from that of the other services with a native app), as was leaving Bases web-only. **What goes out**: `bases.<dominio>/api/v1`
  and nothing else; the route is written in the chart template (`deploy/helm/charts/bases/templates/ingress.yaml`), not in the
  values, because all of `/api/v1` requires a Sytue ID token and the rest served on the same port carries no session. **What
  stays inside**: the public forms and the read-only connector (`/_bases/v1/…`), which come in through the BFF and through the
  consumers, and the probes. It shares host and certificate with the web: `/api/v1` is more specific than the BFF's `/api`,
  which has no routes of its own there. **CORS is not opened**: a native app does not need it and the web talks to its BFF. What
  changes when receiving direct traffic, what was reviewed and what was fixed (upload, download and export timeouts),
  in [security.md](security.md#bases-api-publica); it is watched by `helm-render-test.sh`, which looks at all paths to the Service
  and not only the chart's Ingress. Production and staging still have Bases off: nothing changes there.

<a id="0069"></a>

<a id="0069--los-scripts-de-usuario-corren-aislados-en-el-navegador-contra-una-api-versionada-que-se-traduce-al-registro-común-de-comandos"></a>

## 0069 — User scripts run isolated in the browser, against a versioned API that is translated to the common command registry

`accepted` · 2026-10-04 · Docus, Calcus, Diapos and, since 2026-10-05, Bases · ROADMAP-OFFICE `automatizacion-1` · code in `services/docs/internal/{domain,store,http}/scripts*.go`, migration `0022`, `platform/pkg/guiones` and, on the client, `apps/frontend/packages/scripts` (the common runtime) and the scripts module of `apps/frontend/apps/docs/src/editor/`

- **What already existed is not a sandbox and is not advertised as one.** The macro recorder (`macros.ts`) validates steps against the
  command registry and nothing else; it runs what the user recorded with their own permissions. Scripts are something else and are
  built ON TOP of the same registry (`comandos.json`): it is the API surface, not a second one.
- **No ambient authority.** The user's code runs in an `<iframe sandbox="allow-scripts">` WITHOUT `allow-same-origin`
  (opaque origin: no cookies, no storage, no host DOM), with CSP `default-src 'none'` on the iframe's page (no network,
  no outside `importScripts`, no `connect-src`) and, inside, a Worker that is killed (`terminate`) when time runs out. Everything
  that crosses is `postMessage` with fixed-shape messages, validated and size-bounded. The Worker does not inherit the
  iframe's origin: the iframe has none, and neither does a `blob:` Worker of the iframe; fetch, XHR and WebSocket die from the CSP as well as from
  the opaque origin.
- **The API is `sytue.v1.*`**, versioned and stable. The script asks; the HOST translates each call to a command of the
  registry and validates it with the macro validators (existing command, valid for the document type, current
  permission, allowed value) and, in addition, against the manifest. What the host dispatches goes through the usual `dispatch`
  and ends up in `/ops`, where the server re-authorizes ACL and protection on every write: **a script is never a way to
  bypass protection**; if the editor does not allow it to the person, the script does not either.
- **Manifest per script** (JSON `v: 1`): allowed commands, read families, write yes/no, scope (sheets and
  ranges), `max_ops`, `max_ms`, and whether it can run without interaction. Consent is requested before the first run and
  each manifest change requests it again. The server stores the canonical manifest (sorted lists, default bounds)
  and that is what runs; the one shown to the person is that one.
- **Immutable versions and review.** A version is `(código, hash SHA-256, manifiesto)` and is not rewritten (a trigger also
  prevents it in SQL). Editing creates a new draft version. An organization script is run by other people
  only if an administrator approved THAT version, and the approval carries the hash of the reviewed code; the server rejects it if it does not
  match. The author can test their drafts. Another person's private scripts do not exist for anyone (404),
  administrators included.
- **Who runs what.** The server authorizes each run (`POST …/runs`): runnable version, the role on the document that the
  manifest requires (editor if it writes), document type and the script's document, and quota. It returns the authorized code and manifest,
  which are exactly what the client runs, and a run id that the client closes with the result.
- **Rolling-hour quotas**: 60 runs per person, 600 per organization and 10 minutes of reserved time per person
  (the manifest's maximum counts until it is closed with the measured one: opening and abandoning is not free). They are checked and
  reserved in the same transaction, serialized per organization.
- **Audit without content**: who, which script and version (with the hash), which document, result, duration, operations and
  a short error code. Attempts the server denies are also recorded and do not spend quota. The results that
  close the run are reported by the client (the server does not see the code run): they are diagnostics, not proof; what is
  proof is that it was authorized and which version.
- **`sytue.v1` surface** (piece B, `apps/frontend/apps/docs/src/editor/scripts/`): `run(comando, valor?)`,
  `select(hoja, rango)`, `read.cell|range|text|slides|selection|title()` and `log(...)`. Nothing else crosses the boundary: the
  protocol (`protocolo.ts`) allows five message types with exact keys, and an object with `__proto__`, `constructor` or
  an extra key is rejected. A denied call ABORTS the run (`denied` with the reason): a script that tries what
  it cannot do does not carry on as if nothing happened. The limits are enforced by the host (`anfitrion.ts`), not the sandbox: time
  (`timeout`), operations, messages per second, unanswered calls, message size, output size and returned data size
  (`limit`). The memory of a Worker from another origin cannot be measured from outside: time bounds it.
- **Scope**: cell reads and `select` have to fall within the manifest's sheets and ranges; in Calcus, an
  editing command also requires the current selection to be within them. In Docus and Diapos the scope does not bound writing
  (there are no ranges): the command list and `write` bound it.
- **Measured in real Chromium** (`test/e2e/scripts-sandbox.mjs`, 33 cases plus an impostor frame, in the agents' container):
  fetch, XHR, WebSocket, `importScripts`, `import()`, `eval`, cookies, storage, `parent`/`top`, infinite loop, memory,
  message spam, forged messages and commands without permission fail with their code and a canary server receives no
  request. Seen red by disabling each defense by hand: without the CSP the canary receives three requests (the prototype's
  `fetch`, `importScripts` and `import()`: the prelude alone is not enough); with `allow-same-origin` nothing starts (the host
  only accepts messages from origin `null`); without the host's clock, the memory loop ran for 677 s.
- **Interface** (piece C): a «Scripts» group in the View tab, next to «Macros» (`scriptsManage` command of the common
  registry). The panel (`dialogs/ScriptsDialog.tsx`) lists, edits (textarea with line numbers, no dependencies), saves versions,
  changes the scope, proposes, approves or rejects, runs and shows the output, the hour's quota and the paginated log
  (the organization's one to an administrator; the document's one to its owner). «Ejecutar» (“Run”) runs the SAVED version —it is
  disabled with unsaved changes— and what runs is the code and manifest that the server returns on authorization; if that
  manifest is not the consented one, it does not run and is recorded (`manifiesto_cambio`). Consent is remembered per
  (script, hash of the canonical manifest) in the person's browser: it is a question, not the security boundary.
  The BFF (`pages/api/scripts.ts`) forwards through a CLOSED table of routes (`lib/scriptsRuta.ts`).
- **The scripts' command list is SHORTER than the macros' one** (owner's decision, 2026-10-04). The common
  registry marks with `script: true` what a script can use, and what does not carry the mark is not opened by oversight: inside,
  editing content and formatting, cells, rows and columns, sorting and filtering, tables, inserting a link (it goes through sanitizing) and
  pasting; outside, AI (`aifill`, `iafill`, `aiPanel`: it spends quota and sends data to a model), what opens dialogs or managers
  (`scriptsManage`, macros, names, comments, mail merge), what leaves the document (sharing,
  presenting, external data, images and media), permissions, protection, versions, templates, branding and undo/redo. It is
  enforced in three places: the server's manifest validator (`domain.ParseManifiesto`, with the embedded copy of the registry
  watched by `sync-office-catalog.sh`) on save, the client's, and the host's on dispatch (`comando_vetado`). The
  server validates the manifest again on EVERY run: a version saved earlier with a command that is no longer allowed
  is not deleted or touched, but it does not run (400, audited as `manifest_not_allowed`) until a corrected one is saved.
- **Out of scope**: Fyne (a native runtime would be a different design), ERP (the API allows more surfaces, but it is not
  connected here), network access from scripts and third-party libraries. Bases was connected later: see the next point.
- **The runtime is common and each product contributes its surface (2026-10-05, PR #432).** To give scripts to Sytue Bases the
  sandbox was not copied: the iframe, the CSP, the Worker, the protocol and the host loop moved to
  `apps/frontend/packages/scripts` (`@sytue/scripts`), PARAMETERIZED. A product declares its surface as data
  (`{ op, min, max, json? }`) and the package generates `sytue.v1`; the product does not write code that runs inside the sandbox.
  The loop (`ejecutarAislado`) receives the closed list of operations, the bounds of that run and an `atender` that only
  answers value, rejection or limit; time, operations, rate, size, output, message shape and sanitizing of what is
  returned stay in the package, out of the product's reach. Docs keeps its surface and its translator
  (`traducirLlamada`) and its three suites pass without touching a line (56 tests), plus the 35 Chromium ones. `json` lets
  an object travel as a size-bounded JSON string (a Bases record) without widening the protocol. The server
  rules that do not belong to one product —quotas, states, who runs which, hash— are in `platform/pkg/guiones`, which
  `services/docs` and `services/bases` import. The Bases surface and server, in [0068](#0068).
- **For the owner**: an administrator can approve their own script (in a one-person organization there would be no
  other way out); who approved stays in the audit. A second administrator can be required if preferred.

<a id="0070"></a>

<a id="0070--la-presencia-de-docs-se-integra-con-rooms-la-sala-de-un-documento-nace-del-documento-y-su-acl-manda"></a>

## 0070 — Docs presence integrates with Rooms: a document's room is born from the document and its ACL rules

`accepted` · 2026-10-04 · Docus, Calcus, Diapos and Sytue Rooms · owner's decision · code in `services/rooms/internal/media/service/documento.go`, `apps/frontend/apps/docs/src/pages/api/llamada.ts` and `apps/frontend/apps/docs/src/editor/layout/EditorHeader.tsx`

- **What is seen.** The editor header already showed who is in the document (`/api/presence`, `state/cursors.ts`, an
  `aria-live` announcement). Now the avatars carry the name when it is known, and if Rooms is available «Llamar desde el
  documento» (“Call from the document”) appears (or «Unirse a la llamada (n)», “Join the call (n)”, if there is already one). Whoever is in the call is marked with a ring and with their
  accessible name («Ana, en la llamada», “Ana, in the call”). The call opens in Rooms' own window (`/pip?documento=…`), so the
  document is not left.
- **What travels to Rooms: only the document's id and title.** It is one more room origin (`kind: document`, alongside
  `calendar` and `chat`), with `ref_id` = the document's UUID. Never the content or the list of people with access. The title is
  requested by the BFF from Docs with the caller's token, it is not sent by the browser.
- **The document's ACL rules, on every request.** Rooms does not replicate who can see the document: when creating the room,
  viewing it (`by-source`, `GET /rooms/{id}`), joining, knocking and listing participants it presents the person's token
  to Docs' `GET /api/v1/documents/{id}`. Docs applies its own role resolution (user, groups, organization).
  The same applies to the creator, to a tenant administrator and to whoever already took part: removing someone from the document
  closes the call for them on the next request. It fails closed: without `DOCS_URL`, without a token, with Docs down or with a 5xx, it is denied.
  Document rooms do not appear in `GET /rooms` (their title is the document's).
- **No hard coupling.** The BFF probes Rooms with a 2.5 s timeout; if it is not deployed, does not resolve or answers 5xx,
  it responds `available: false` and the header renders nothing, with no error. Docs does not depend on Rooms to open or save.
- **Integration through what already exists.** It reuses Rooms' `from-source` and `by-source` and its `meet.room.created` event, which now
  carries `created_by`; Rooms' call fan-out ignores origins that are not channels. There is no new channel in the Hub.
- **Audit of who created the room.** The room stores `created_by` and the origin (`document`, id, title) and the creation event
  publishes it. Docs' «Actividad» (“Activity”) panel remains the document's operations one; they are not mixed.
- **Ceiling.** Polling for who is in the call is every 15 s with the tab visible, not via WebSocket; the button is hidden
  below 768 px, where the header does not fit. A Hub notification to the people with access when someone starts a call
  is not done: it would require resolving the list of people from Docs, precisely what this design avoids copying.

<a id="cursores-de-hoja-anclados-por-identidad"></a>

### Sheet cursors anchored by identity

`SheetData` gains `rowIds` and `colIds` (`{ id }[]`, with `rowBase`/`colBase`): identity of rows and columns, implicit
(`t0`, `t1`…) until someone inserts or deletes. The sheet cursor also travels with `sheetId`, `rows` and `cols`; the receiver
resolves them against its model and, if any is missing (older client, deleted row), uses the usual indices, so protocol
v1 does not change. They are stored as a list with `id` so that `fusionar3` preserves two concurrent insertions.
Tests: `apps/frontend/apps/docs/test/sheets-anclas.test.ts`.

<a id="0071"></a>

<a id="0071--el-color-de-la-suite-es-elegible-petróleo-por-defecto-y-azul-cobalto-como-eje-aparte-del-modo"></a>

## 0071 — The suite color is selectable: petroleum by default and cobalt blue, as an axis separate from the mode

`accepted` · 2026-10-04 · the whole suite (web and native) · owner's decision · code in `branding/tokens/colors.css`, `branding/tokens/appearance.css`, `apps/frontend/packages/config/src/contraste-paleta.test.ts`, `apps/frontend/packages/ui/src/theme/`, `services/id/internal/http/admin_branding.go`

- **What is decided.** The suite stops having a single brand color. It offers two, **petroleum** and **cobalt blue**; whoever has not
  chosen anything sees petroleum. Emerald, which was the only brand color for a day, disappears as an option. Each
  person chooses, with an organization default color set by its administrator.
- **Why two and why these.** The owner saw the suite in emerald, orange, terracotta, blue and petroleum. Petroleum is the
  default because the action no longer looks like the green of “success”: a button and a “Signed” badge no longer share a
  color. Cobalt is the lively alternative, and it is where the amber of “you are here” shows best.
- **Color is an axis separate from the mode.** `data-color` on the root, next to `data-theme`: two colors by three modes (light,
  translucent, dark), plus the system dark. Petroleum is the value without the attribute, so a page that knows nothing
  about color renders correctly. It is not a fourth and fifth theme: mixing it with the mode would have produced six complete blocks instead of
  four for petroleum plus what changes in cobalt.
- **Color also tints the neutrals.** Canvas, lines and text carry a touch of the hue, and the derived ones (the
  `neutral-*` scale, `bg-subtle`, `chrome-hover`) are derived the same way for each color. Status, product and
  person colors do not change. “Info” remains blue in both.
- **How it does not break.** `contraste-paleta.test.ts` measures the pairs the interface renders in both colors by the four
  modes, applying the blocks like the cascade, and checks that each cobalt mode repeats what its light block declares:
  `[data-color]` and `[data-theme]` have the same specificity and at equal specificity the later one wins.
- **Where the choice lives.** In the same place as the mode one, without separate plumbing: a cosmetic cookie on the apex
  (`sytue-color`) plus local storage, read by the same reader and applied by the same first-paint
  script (`packages/ui/src/theme/init.ts`), which is the one the IdP and the self-contained pages also carry. It is not
  account data: like the mode, it says nothing about who the person is. Following the organization means having NO choice: the
  selector option that says so clears the cookie and the storage, and the usual order does the rest. It is done once,
  in `useEje`, for the color and for the shape.
- **The organization's color.** Column `organizations.color` (Sytue ID migration 0107), which the administrator sets
  in Sytue Admin (`PUT /api/v1/admin/org/color`). It travels in the ID token as `org_color` —only if it is not the house one— and the
  BFF leaves it next to the session in a readable cookie, `sytue-color-org`, because the first-paint script does not see the
  session. Order: the person, then their organization, then petroleum. Changing it does not invalidate sessions: it arrives with each person's next
  token.
- **The color list lives in three places and two tests tie it together.** The token stylesheet (one `[data-color]` block per
  color), `THEME_COLORS` on the web and `domain.OrgColors` in Sytue ID. The database only guarantees the shape of the value, so that
  adding a color does not require rewriting a `CHECK`. `init.test.ts` and `TestLosColoresDeOrganizacionExistenEnLaHoja` fail if
  one of the sides falls behind.
- **What is left out of this decision.** The native apps today read only the petroleum blocks of the stylesheet: their token
  reader does not yet have the color dimension, so neither the choice nor the organization's color reaches them.

<a id="0072"></a>

<a id="0072--color-de-cada-app-tercera-opción-del-eje-de-color-carril-y-acción-en-el-color-de-producto"></a>

## 0072 — “Each app's color”: a third option of the color axis, rail and action in the product color

`accepted` · 2026-10-05 · web (native apps, once they read the color axis) · owner's decision · code in `branding/tokens/colors.css`, `branding/tokens/appearance.css`, `apps/frontend/packages/ui/src/theme/init.ts`, `apps/frontend/packages/config/src/contraste-paleta.test.ts`

- **What is decided.** The color axis of [0071](#0071) gains a third value, `app`: each app paints the rail and the action
  —primary button, link, selection, focus— with its product color (`--sytue-product-<app>`). It is one more option of the
  same selector, for the person and as the organization's default color. The neutrals remain the house ones.
- **The values come from the stylesheet, not from a computation on the client.** Each app contributes an eleven-step ramp derived from
  its product color, and the per-mode assignment is written once with `var()`: 600 carries white text (4.7:1), 700
  is the rail and the link (6.6:1 with white), 400 the action in dark mode with dark text (4.7:1) and 300 the link in
  dark mode (6:1). That is 13 lines per app instead of a whole theme per app and mode. `contraste-paleta.test.ts` measures each app in
  the four modes with those thresholds.
- **How the stylesheet knows which app it is.** `data-app` on the root. On the web it is set by the first-paint script with the prefix
  under which the app is served, which was already published before it: no layout changes and there is no flicker. Where the prefix
  is not the product key (`/admin`, `/web`) the stylesheet carries the alias, and a guardian reads each app's `base` and fails if
  one is left without a color when its prefix changes. A surface served at the root publishes the empty prefix and says so with the
  first label of its host: that is how Sytue Admin takes its color also on `platform.<dominio>`.
- **Each editor, its own color.** Docus, Calcus, Diapos and CertiPDF share `/docs`, so there the prefix does not say the
  product. Their layout —and the public view of a shared link— sets `data-app` on the `<html>` from the server, with
  the product of the document or of the list being shown, and the first-paint script respects the attribute it already
  finds: no flicker. When opening another document without reloading it is changed by `aplicarMarca`, which is what already changed the
  tab icon. With this, PDF and Diapos tinting only the rail takes effect on the web too.
- **The house surfaces stay petroleum.** The Hub and My account carry no ramp: they are the suite's frame, and
  the Hub shows the tiles of all apps at the same time. With `data-color='app'` and no ramp of their own the assignment gives the
  petroleum values. Neither do the IdP nor the self-contained pages, which do not say which app they are.
- **The amber of “you are here” works in all of them.** With the rail at 6.6:1 against white, amber reaches 3:1 on the active
  item in all 19 apps and in the four modes; the white fallback was not needed.
- **Five apps only tint the rail** (owner's decisions, 2026-10-05): Mail, PDF, Cast, Diapos and ERP. The color
  stays in the rail and the frame; primary button, link, selection and focus go in the house primary. In the stylesheet those
  apps declare the rail ramp (`--sytue-rail-*`) and not the primary one, so the web and the native apps pick it up without more
  code.
- **The rule: the apps whose color gets confused with the danger color in badges, links and focus.** Not in the button, which is what
  misleads least. With the action in red, in Mail a field's focus ring looked like a validation error, «Cerrar
  sesión» (“Sign out”) could no longer be told apart from a link and the primary button looked destructive. In Diapos and ERP, which are orange,
  the primary button and the danger one can be told apart side by side, but the badge in the app's hue and the danger one are both
  a pinkish pill with reddish text, the focused field and the field with an error are separated more by the ring's thickness than by
  color, and the link sits too close to the danger text.
- **It is a decided list, not a formula, and it is stated as such.** No measurement separates the ones that get confused from the one that does not:

  | App       | Hue offset (OKLCH) from `danger` | ΔE2000 with `danger` | Decision         |
  | --------- | -------------------------------- | -------------------- | ---------------- |
  | PDF       | −1.9°                            | 4.0                  | rail only        |
  | Mail      | −11.9°                           | 9.8                  | rail only        |
  | Cast      | −15.5°                           | 15.5                 | rail only        |
  | Diapos    | +16.4°                           | 11.0                 | rail only        |
  | Bases     | +17.8°                           | 13.9                 | rail and action  |
  | ERP       | +19.8°                           | 14.2                 | rail only        |
  | Sytue Web | −37.6°                           | 24.9                 | outside the band |

  Bases sits between Diapos and ERP by hue and closer to `danger` than Cast by distance, and even so it does not get confused: it is a
  chocolate brown, darker and duller than bright red. What decides is looking at it: the sample from the screenshot harness (a
  delete dialog with the primary and danger buttons together, a focused field, another with an error, and badges, in light
  and dark), which is the one the owner saw for the six apps in the table.

- **What is measured is who has to go through that decision.** An app whose color falls within 26° of the hue of
  `--sytue-danger` is in the band. `contraste-paleta.test.ts` carries the two lists —rail only, and in the band with
  action— and fails if an app with a block falls in the band without being listed in one: it does not decide on its own, it asks for the decision.

<a id="0073"></a>

<a id="0073--rooms-denunciar-y-bloquear-con-el-bloqueo-aplicado-en-el-servidor-y-la-denuncia-en-la-organización-anfitriona"></a>

## 0073 — Rooms: reporting and blocking, with blocking enforced on the server and the report in the host organization

`accepted` · 2026-10-06 · service, web and native · owner's decision («añade denuncia y bloqueo para rooms y cast y lo marcamos como que existe», “add reporting and blocking for rooms and cast and we mark it as existing”) · code in `services/rooms/internal/store/moderacion.go`, `services/rooms/internal/http/moderacion.go`, `apps/frontend/apps/rooms/src/components/Moderation.tsx`, `apps/fyne/apps/rooms/moderacion.go` · migration `services/rooms/db/migrations/0038_denuncias_y_bloqueos.up.sql`

- **Why now.** In the content rating, Google Play asks whether the app allows blocking and reporting users
  and content, and its user-generated content policy requires it when the main content comes from users. Rooms
  had neither: the only thing that existed was that the author or a channel administrator could delete a
  message, and that an administrator could kick someone out.
- **Reporting.** You report a message (and with it its author) or a person in a channel you share with them. The
  reason comes from a closed list —harassment, sexual content, violence, spam, other— and allows a note of up to 500
  characters. The report **belongs to the organization hosting the conversation** ([0040](#0040)): whoever comes from outside
  reports there, and that is where it is moderated.
- **Who moderates.** The administrators of the **team** hosting the channel —who could already delete someone else's message and
  kick people out— and the administrators of the **organization** (`owner` or `admin` role in the token). Team ones are notified
  through the Hub (`rooms.report`), without saying who reports or whom; organization ones see it when they open the queue, because
  Rooms does not know who they are until they come in. The queue offers three actions and a report is closed with one: dismiss,
  delete the message, kick out of the channel (the latter not in a direct message).
- **What is not revealed.** Nobody sees a report against themselves, whatever they administer. And who reported does not appear
  in any API response, not even to whoever moderates: it is not needed to act, and it is what prevents it from reaching the
  reported person. It is in the database.
- **And the operator.** Since 2026-10-12 reports also reach the platform operator's queue, which does see
  those against whoever administers ([0075](#0075)).
- **So that it cannot be used for harassment.** A person reports each message once, and another person in a channel once while
  that report remains open; repeating it returns the existing one and notifies nobody. There is a per-person limit on the route and a
  cap of 20 reports in 24 hours counted in the database, which holds with several replicas.
- **Blocking belongs to the person, not the organization.** The Sytue ID `sub` is the same in all organizations, so
  the block also applies in the conversation two people share in another one. That is why `user_blocks` is read and
  written through the maintenance policy, always by the verified token's `sub`.
- **What a block does, on the server.** A direct message between two people with a block in either
  direction does not accept new messages —nor scheduled ones nor a call card—: the service answers 403 with
  the same text to both, without saying who blocked. And an author's notifications (Web Push, APNs, Hub) do not reach whoever has
  blocked them, in any channel. In shared channels and groups the message still arrives and the client shows it
  **collapsed**, with a button to read it: a work channel cannot have gaps that only one person sees.
- **It is rejected, not hidden.** The alternative in DMs was to accept the message and not show it to whoever blocked. It is
  ruled out: it would lie to the writer about delivery and leave in the organization's retention messages that nobody
  saw.
- **Blocking deletes nothing.** The conversation stays where it was, with the retention of the organization hosting it. The
  block is personal data: it is deleted with their account (`services/id/internal/merge`). Reports are not: they belong to the
  organization.
- **The trail.** The report's row says who, whom, about what and when, and who resolved it, how and when; it is
  resolved only once and deleted twelve months after being resolved ([0075](#0075)). There is also one log line per report and per resolution. Neither the message text
  nor the note goes to the log, and the message text is not copied into the report: the queue reads it live, so an already
  deleted message does not reappear there.
- **What is left out.** There is no organization-level block (an organization banning an outside person), nor
  an appeal by the reported person, nor automatic content detection. A blocked person still sees the messages of whoever
  blocked them in shared channels: the block protects whoever blocks, it does not hide anyone from a work channel.

<a id="0074"></a>

<a id="0074--cast-denunciar-y-bloquear-con-la-denuncia-abierta-a-quien-mira-sin-cuenta-y-dos-bloqueos-aplicados-en-el-servidor"></a>

## 0074 — Cast: reporting and blocking, with reporting open to anonymous viewers and two blocks enforced on the server

`accepted` · 2026-10-06 · service, web and native · owner's decision («añade denuncia y bloqueo para rooms y cast y lo marcamos como que existe», “add reporting and blocking for rooms and cast and we mark it as existing”) · code in `services/cast/internal/social/denuncias.go`, `services/cast/internal/social/bloqueos.go`, `services/cast/internal/http/moderacion.go`, `apps/frontend/apps/cast/src/components/Moderacion.tsx`, `apps/fyne/apps/cast/moderacion.go` · migration `services/cast/db/migrations/0009_denuncias_bloqueos.up.sql` · sibling of [0073](#0073) (Rooms)

- **Why now.** The same as in [0073](#0073): Google Play requires it of an app whose content comes from its users. In
  Cast it weighs more, because it has a **public front page**: people without an account see it. The only thing that existed was that the author, the
  video's owner or whoever administers the organization could delete a comment or a chat message.
- **What is reported.** A video or live stream, a comment, a chat message, a person and a channel. The reason comes from a
  closed list —harassment, sexual content or nudity, violence, spam or deception, copyright, other— and allows a text of
  up to 1000 characters. A snapshot of what was reported is stored (whose it is and a 300-character excerpt): the chat is
  removed after 30 days and a comment can be deleted, and the queue has to keep saying what was reported. In this it
  departs from Rooms, which reads the message live.
- **Only what appears in the video.** A comment or a message is reported within ITS video, and a person only if they
  appear in it (they publish it or have written in it). That way you cannot report or block an arbitrary `sub`, nor probe
  which ones exist.
- **Without an account too.** Whoever watches the public front page can report what they see without an account: public or unlisted videos,
  their comments, whoever publishes them and the channel. Not the chat, which cannot be read without a session. It goes with an ALTCHA proof of work
  (`platform/pkg/altcha`, the one from Bases' public forms), with a per-IP cap shared across replicas, and without
  storing the IP: only an HMAC so as not to count the same one twice. The alternative —requiring an account— left without a door
  precisely those whom Google asks to have one.
- **Whose report it is and who acts.** It belongs to the organization that owns the **content**, not to the reporter's. It is
  seen and resolved by whoever administers that organization (`owner` or `admin` role in the token, which is only valid in their
  organization). The queue offers: dismiss; delete the comment or the message; remove the video from the public front page (it becomes
  unlisted); unpublish it (it becomes organization-only); and block the person throughout the organization. Acting closes
  all open reports about the same thing; dismissing, only that one.
- **Who is notified.** Through the Hub (`cast.report`), those who administer the organization **and whom Cast has seen**: the Hub
  delivers per person and Cast does not have the member list, so it records each administrator when they open Cast. If what is
  reported belongs to another person in someone else's video, also the video's owner, who could already delete it. Never the reported person
  or the reporter, and without saying who it was.
- **What is not revealed.** Who reported does not appear in any API response: neither in the queue nor in the trail, where
  they appear as `account` or `anonymous`. It is in the database (`reports.reporter_sub`). And, as in Rooms, **nobody sees a report
  against themselves**, whatever they administer: not in the queue, not in the open count, not in the trail —each row of
  a report carries `target_sub` in the detail, so that it stays hidden once the report has been purged—, nor to
  resolve it. The first version showed it to whoever administers («en Cast quien publica es la organización», “in Cast the publisher is the organization”); the owner
  changed it on 2026-10-12: if the publisher is whoever administers, the report is seen by the other people who administer and the
  platform operator, in their queue ([0075](#0075)).
- **So that it cannot be used for harassment.** One open report per person and target (unique partial index): repeating it answers the
  same and notifies nobody. Five at once and one every twenty seconds per person; ten at once and one every thirty
  seconds per IP without an account. The response is always an empty 204.
- **Two blocks, both on the server.** The **personal** one: whoever blocks stops receiving that person's comments and chat
  messages (the service does not send them, not over the WebSocket either) and their live notifications; and that person can no longer
  comment and write on the videos of whoever blocked them. The **organization** one, set by whoever
  administers it: that person does not write on any video of the organization. It is what Rooms left out and is needed here,
  because in Cast people from outside write.
- **It is rejected and said.** To whoever is blocked the service answers 403 `blocked` when writing, and the app tells them.
  They are not told who blocked them, but they know they cannot write there: accepting the comment and not showing it would lie to them.
- **Blocking deletes nothing**, and it can be undone: the personal one, in «Denuncias y bloqueos» (“Reports and blocks”); the organization one, by whoever administers,
  on the same screen.
- **The trail.** `moderation_audit`: one row per report, per action and per organization block, in the same
  transaction as what it audits, with who acted. It is read with the filters and CSV of `platform/pkg/auditlog`.
- **Export and delete.** Cast's “Export your data” includes whom the person has blocked and the reports SHE filed,
  from any organization (what, why, when and whether it is still open), without the copy of what was reported or the action
  taken, and never those that others filed against her (policy `reports_de_quien_denuncia`, migration 0011). “Delete my
  account” deletes her blocks through the same path as Rooms' `user_blocks` (`merge.DatosDeLaPersona`); `subject` is a
  generated column over `blocker_sub`, because renaming it would break the previous pods during deployment. The
  blocks others placed on her belong to those people and stay; her reports belong to the content's organization and
  are purged on their schedule ([0075](#0075)).
- **Several replicas.** Cast runs with two or more. Already-used ALTCHA solutions are stored in the database (`altcha_used`,
  migration 0012, with `altcha.EnTabla`, the same form Sytue ID, Bases and Web already had): a solution is valid once
  across all replicas. And what the chat broadcasts on one —a message, a deletion, a newly placed block— reaches
  the connections of the others through PostgreSQL LISTEN/NOTIFY (`Servicio.Relevar`, `events.Notificar`), with its origin
  to discard the echo; before, neither the block nor the messages crossed replicas. The viewer count is still
  per replica. If the NOTIFY fails, the frame stays on its replica, as before.
- **What is left out.** There is no appeal, no automatic detection, and no notification to administrators who have never opened
  Cast. A live stream's viewer count is that of each person's replica, not the total.

<a id="0075"></a>

<a id="0075--cola-de-denuncias-del-operador-de-la-plataforma-las-mismas-filas-el-sí-de-sytue-id-en-cada-petición-y-la-traza-en-su-registro"></a>

## 0075 — The platform operator's report queue: the same rows, Sytue ID's yes on every request and the trail in its log

`accepted` · 2026-10-12 · Cast, Rooms, Sytue ID and platform console (web only) · owner's decision («cola también para Sytue», “a queue for Sytue too”; «un administrador no ve las denuncias contra sí mismo; Sytue sí», “an administrator does not see reports against themselves; Sytue does”) · code in `platform/pkg/operador`, `services/id/internal/http/platform_moderacion.go`, `services/cast/internal/http/moderacion.go`, `services/rooms/internal/store/operador.go`, `apps/frontend/apps/console/src/components/PlatformModeration.tsx` · migrations `services/cast/db/migrations/0010_cola_del_operador.up.sql` and `services/rooms/db/migrations/0039_cola_del_operador.up.sql` · completes [0073](#0073) and [0074](#0074)

- **What.** Cast and Rooms reports reach two queues: that of whoever administers the organization that owns the content and
  that of the platform operator (Sytue), in its console (`platform.<dominio>/moderacion`). There is no copy: they are the same
  rows, so resolving in one closes it in the other. `resolved_by` and `resolved_from` (`org` | `platform`) say who and from
  where, and in Cast the organization's trail (`moderation_audit`) also carries `from: platform`.
- **What the operator sees.** All organizations, including reports against whoever administers each one (which that person
  no longer sees). They see what the report stores or points to, and nothing more: in Cast, the 300-character excerpt, the name of the
  reported person and the reporter's text; in Rooms, the channel, the name and the reported message trimmed to 300
  characters. There is no link to enter the customer's content, and who reported never travels.
- **What they can do.** The same as the organization's queue: in Cast, dismiss, delete the comment or the message,
  remove the video from the front page, unpublish it and block the person in the organization; in Rooms, dismiss, delete the
  message and kick out of the channel. Suspending the organization already exists in the platform console, with its second person, and
  the screen links there. **Suspending a person at the platform level does not exist** and has not been invented: today only
  whoever administers their organization suspends them.
- **Who is an operator is said by Sytue ID, on every request.** The service does not deduce it from a role or a claim: before
  listing or resolving it asks `POST /api/v1/platform/moderation/access` with the caller's token and what they are going to do
  (service, action, organization, report). That route sits behind the same gate as the rest of the platform API
  —allowlist, second factor and live elevation—, writes the log row and answers 204. If it is not 204, the service
  shows nothing; if Sytue ID cannot write the row, it denies. The check and the trail are the same call: there is no
  read without a trace.
- **Only from the platform console.** The services also require the token to be from the `sytue-platform` client: an
  elevated operator browsing Cast's website does not open the all-organizations queue there. And the platform
  console can only be reached through the administrative tunnel.
- **The trail.** In the Sytue ID log, `platform.moderation.<servicio>.<acción>` with the operator, the organization
  from which they act (`from_org`) and the client; with the affected organization, the row also appears in that
  organization's log, which sees that Sytue resolved one of its reports. When listing there is no organization: the row says what was requested.
- **Retention (both queues, Cast and Rooms).** An open report is kept until someone resolves it. Resolved or
  dismissed, it is deleted **twelve months** after its resolution, and with it what it stores of what was reported: in Cast, the
  excerpt, the name and the reporter's text; in Rooms, which does not copy the message, the entire row (who, whom,
  reason, note and resolution). It is done by a daily sweep of all organizations (`PurgarDenuncias`, through the maintenance
  policy). **The trail is not purged**: it carries no content —identifiers, action and who acted— and
  `platform/pkg/auditlog` does not set a period for any trail in the suite; setting one is the owner's decision (proposal:
  24 months, twice the report's, to be able to reconstruct a disputed resolution). The privacy policy says so
  in all 26 languages.
- **Processing on behalf of the customer.** The operator reading and acting on a customer organization's reports is
  data processing on behalf of that customer (reported content, names, the reporter's text). **It has to
  appear in the terms and in the DPA**: reviewing that before deploying is the owner's decision.
- **What is left out.** Hub notifications to the operator (the queue is the source of truth and is checked when elevating), appeal, automatic
  detection and suspension of people at the platform level.

<a id="0076"></a>

<a id="0076--docs-se-separa-en-cuatro-productos-con-app-host-y-client-propios-docsdominiodocs-desaparece-sin-redirecciones"></a>

## 0076 — Docs is split into four products with their own app, host and client; `docs.<dominio>/docs` disappears without redirects

`accepted` · 2026-10-12 · Docus, Calcus, Diapos, CertiPDF, Sytue ID and deployment · owner's decision, through direct questions
(«cada editor es un producto con su nombre, su app nativa y su ficha en las tiendas; la URL tiene que decir lo mismo», “each editor is a product with its name, its native app and its store listing; the URL has to say the same”) ·
code in `apps/frontend/packages/office`, `apps/frontend/apps/{docus,calcus,diapos,certipdf}` · migration
`services/id/db/migrations/0111_clientes_oidc_de_la_ofimatica.up.sql` · runbook in
[operations.md](operations.md#ofimatica-una-app-por-producto) · replaces the decision of 2026-09-09 to serve the four
products under `/docs` with `?kind=`

- **What.** Four products with their own host and prefix, with the organization in the path ([ADR 0056](#0056)):
  `docus.<dominio>/docus` (documents), `calcus.<dominio>/calcus` (spreadsheets), `diapos.<dominio>/diapos` (presentations) and
  `certipdf.<dominio>/certipdf` (PDF). Subroutes are preserved: `docs.sytue.eu/docs/corporate/edit?id=<uuid>` of a spreadsheet
  becomes `calcus.sytue.eu/calcus/corporate/edit?id=<uuid>`, and the PDF studio `certipdf.sytue.eu/certipdf/<org>/pdf?path=…`.
- **Four apps, not one compiled four times.** Each product is an Astro app (`apps/frontend/apps/<producto>`) with
  its `base`, its BFF and its OIDC client (`sytue-docus`, `sytue-calcus`, `sytue-diapos`, `sytue-certipdf`). What is common —the editor,
  CertiPDF, the views, the middleware and the BFF that talks to the service— moves to the `@sytue/office` package and is not copied
  ([ADR 0037](#0037)): each app's pages are thin and say which product they are; the `/api/*` endpoints are injected by the
  `sytueOffice()` integration; the `/auth/*` routes re-export as in the rest of the suite.
- **The Go service is still one.** Documents, spreadsheets and presentations share the model, collaboration, permissions,
  trash and templates, and CertiPDF stores its files in Drive; splitting it would give nothing and would duplicate the data
  migration. It already filters by type (`?kind=`), which is all each app needs. Its API stays at `docs.<dominio>/api/v1`:
  it is used by the installed native apps, Drive (WOPI) and the BFFs, and no person sees it in the address bar.
- **A document in another product's app goes to its own.** If `calcus…/edit?id=` receives a presentation, the server
  sends it to the Diapos editor with the same organization and the same return to Drive: a link built without knowing the type
  —a notification, a search— does not open a presentation in Calcus.
- **`docs.<dominio>/docs` disappears WITHOUT redirects** (chosen over “redirect to the right product”). **What is
  lost:** every saved link in that form —in an email, an old Hub notification, a bookmark, a note or a Rooms
  message— stops working and gives a 404; notifications already issued are not rewritten. Those generated since this change
  come out with the product's host.
- **Sytue ID.** The four clients are born in 0111 without a secret (the deployment sets it), and `docus`, `calcus`, `diapos` and
  `certipdf` are reserved slugs: an organization with that name would collide with the product's host in the wildcard. `docs`
  remains reserved (the API host).

<a id="0077"></a>

<a id="0077--rooms-cifra-en-los-dispositivos-mediante-matrix-los-canales-externos-conservan-la-protección-real-de-su-proveedor"></a>

## 0077 — Rooms encrypts on devices through Matrix; external channels keep their provider's real protection

`accepted` · 2026-10-16 · express decision by the owner in this session · bounds the
language and license restrictions of 0016, 0018 and the removal of AGPL engines

**Problem.** Storing and processing text in the Rooms service prevented offering E2E.
Adding a padlock to the assistant does not change who can read the message. In addition, the
web and Fyne SDKs must exchange messages, recover history and remove devices
without handing their keys to the server.

**Decision.** Native content is encrypted on the clients with the maintained
Olm/Megolm implementations: the Matrix web SDK with Rust/WASM and mautrix-go with GoOlm in
Fyne. Synapse, unmodified, works as a separate transport, with its own database
and credentials; its code is not incorporated into the proprietary product.
The versions are in CONVENTIONS §3 and the manifests pin the image by digest.
The licenses, notices and access to the exact sources of these pieces are preserved.
The MPL exception is limited to mautrix-go and its dependency go.mau.fi/util, without opening
the gate to other copyleft dependencies.

Sytue ID remains the identity authority. Matrix sign-in uses OIDC
and PKCE; passwords, public registration and federation are disabled. The Rooms
service provisions private rooms according to its ACL and keeps opaque references to encrypted
events. The administrative controller remains as a metadata member:
Synapse requires the administrator to stay inside to manage a private room, and
it clears the current state when the last local member leaves. It does not run a
cryptographic client. The SDKs share keys only with devices verified
by SAS or signed by an already verified identity; the controller's fixed MXID
is excluded from verification and from key requests, even if it publishes a
self-signed device. This separation is checked with the real engines;
it does not depend on the account having no devices at deployment time.
New rooms use version 12 and its implicit creator power.

Suspending an account and removing an organization revoke the transport
before confirming the change in ID. Reactivations come out of the shared outbox
after the commit, with monotonic versions and a check of the current state in ID.
Failures do not open a plaintext path. Recovery, device verification,
attachments and drafts are resolved on the client; earlier history is identified
as pre-E2E and is not presented as retroactively encrypted. Native call
keys must travel through authenticated and encrypted Matrix events, never
through the SFU's readable signaling.

**External channels.** The owner authorized full exchange with Slack, Teams and
Google through each account's consent, and asked to use external E2E when there is
a compatible official API. Those conversations are distinguished from native Rooms and
declare their provider's effective protection. A native room is not decrypted and forwarded
because a connection is enabled. The option of sending only notifications/links remains
valid. A video call that requests E2E must check the provider's confirmation;
if it cannot enable it, it fails explicitly without silently lowering the protection.

**Consequences.** An additional transport is operated and two compatible SDKs are maintained.
Before publishing clients it is required to verify web/Fyne exchange, recovery,
removal of members and devices, attachments and media. Architectural approval
does not by itself prove that this validation is finished. European WhatsApp
interoperability is postponed by the owner's decision; the Meta
Business configuration is pending until an authorized administrative session is available.

**Sources.** [Matrix specification](https://spec.matrix.org/latest/client-server-api/),
[Synapse OIDC](https://element-hq.github.io/synapse/latest/openid.html) and
[Teams meeting encryption in Microsoft Graph](https://learn.microsoft.com/en-us/graph/api/application-post-onlinemeetings?tabs=http&view=graph-rest-1.0).

<a id="0078"></a>

<a id="0078--widgets-nativos-contadores-con-caducidad-en-un-app-group-apple-tras-un-interruptor-y-nada-en-windowslinux"></a>

## 0078 — Native widgets: expiring counters in an App Group, Apple behind a switch and nothing on Windows/Linux

`accepted` · 2026-10-21 · PR #967 · completes the shared session of the Fyne apps

**Problem.** The Fyne apps wanted a widget on the home screen (iOS, Android) and on
the macOS desktop, with a shortcut to open and search. A widget is drawn by the system
in another process that has neither the session, nor the network, nor the app's Keychain; whatever it reads
has to be left written for it by the app beforehand, and that cannot become a plaintext copy
of the person's data. Moreover, on Apple sharing a folder between the app and its
extension requires an App Group registered by hand in the Developer Portal: turning it on without that step
breaks the iOS, Mac App Store and Developer ID signing of all fifteen apps at once.

**Decision.**

- **The snapshot format** (`apps/nucleo/widgets`, version 1): one JSON per app,
  `widgets/<bundle>.json` inside the suite's settings folder, with `version`,
  `product`, `generation`, `updated_at`, `expires_at`, `active`, up to twelve `counters`
  `{kind, value}`, up to eight sanitized visual `items` (`title`, `subtitle`, `badge`,
  `color`, `start_at`, `end_at`, `all_day`, `unread`, `done`) and an optional `calendar`
  (`year`, `month`, `busy_days`, `day_colors`). `kind` comes from a closed list (`unread`,
  `notes`, `calendars`, `address_books`…) and `value` is a bounded integer. The native reader
  (`Snapshot.swift`, `WidgetSnapshot.java`, `Widgets.java`) rejects extra fields, nulls,
  files larger than 16 KiB and any value outside that shape.
- **Privacy limits (extended on 2026-11-01 for visual widgets).** The fifteen
  apps publish bounded visual headers (upcoming events and the month's busy days in
  Calendar, sender/subject in Mail, public non-external channels in Rooms, unencrypted notes/tasks
  in Notes, recent files in Drive/Docus/Calcus/Diapos/CertiPDF/Bases/Photos,
  contacts, videos/live streams in Cast, pending activities in ERP and agenda in Hub), never
  message bodies, E2E encryption markers (`enc:v1:`, rejected both when publishing
  and when reading), private/external rooms or arbitrary links. A snapshot **expires**
  (15 min when published, 30 at most) and is ignored if expired, future-dated or from another
  session generation (`sesion.marca`, opaque, never the subject or the email). It is **deleted on
  sign-out or when the suite lock is activated**, and the reader checks the marker and the
  lock (`KeyguardManager.isDeviceLocked()` on Android and `NSFileProtectionComplete` +
  `privacySensitive()` on Apple) even if the deletion fails. The app publishes only what it has already
  requested for its own screen, never an extra request, and captures the session **before**
  requesting it: a late response from another account is not published. The extensions have no
  network or Keychain.
- **Where it lives.** On Android, the APK's own private folder, read by its
  `AppWidgetProvider` (`eu.sytue.nativo.Widgets`). On Apple, the App Group
  **`group.eu.sytue.suite`** (`ajustes/grupo_apple.go`), common to the fifteen apps and their
  fifteen `eu.sytue.<app>.widgets` extensions, which is also where the suite's settings
  move to live while it is on. Tokens stay in the Keychain (`<equipo>.eu.sytue.suite`), never
  in the App Group.
- **Apple, behind a single switch**: `SYTUE_WIDGETS_APPLE` in
  `apps/fyne/xcode/Sytue.xcconfig`, **`NO`** until the owner carries out the steps of
  [«Activar los widgets de Apple»](operations.md#activar-widgets-apple) (“Enable the Apple widgets”). It is read by
  `empaquetar.sh` (`widgets-apple`), `generar.py`, `widgetkit-apple.py` and
  `release-desktop.yml`. Off, each host is signed as before the widgets: no
  extension inside, no App Group, Developer ID without a profile and the Mac App Store with the usual
  `<equipo>.eu.sytue.suite` group. And if at runtime the system does not provide the group
  folder, `ajustes` falls back to the previous one without error. It is in a versioned file and not in a
  repository variable so that **it travels with the tag**: an earlier tag comes out off, and
  turning it on shows up in the diff.
- **Android, on**: it does not depend on anything external and its packaging checks it
  (`android-widgets-test.py`, `verificar_aab`).
- **Windows and Linux, nothing.** The Windows 11 Widgets board requires MSIX packaging, a
  COM provider and Adaptive Cards: another distribution chain for a counter. Linux does not have
  a standard surface (each desktop its own, or none). Without a reader, Go **does not
  write** snapshots: `widgets.Disponible()` is only true on Android and on an Apple host
  that has loaded the WidgetKit bridge, which `empaquetar.sh` links only with the
  switch on.

**Ruled out.** _Reading the data from the extension with its own session_: it would put a refresh
token in a process the system starts whenever it wants and that cannot request
reauthentication. _Titles and upcoming events in the widget_: it is what would be seen with the
phone on the table; it can be reconsidered with an express decision. _A repository
variable as the switch_: it does not travel with the tag and re-dispatching an old release
would turn on the widgets on sources that do not include them.

**Consequences.** Until the switch is turned on, iOS and macOS have no widgets, and the Mac
App Store stays with the old group. Turning it on is a commit (the switch and
`python3 apps/fyne/xcode/generar.py`) after the manual steps; from that tag on,
`release-desktop` fetches the profiles of the apps and their widgets with `apple-perfiles.py`.
No widget has been seen on a device yet; what is missing is in the
[roadmap](roadmap.md#widgets-nativos-lo-que-falta).

**Amendment (2026-11-06): publish after every change.** The owner's request: «los widgets
nativos no se actualizan en tiempo real; al añadir un evento para hoy, no sale en el
widget» (“native widgets do not update in real time; when adding an event for today, it does not show in the widget”). The measured cause was not in any app: `auth.Save` changes `sesion.marca` on
**every** token save, and Sytue ID rotates the refresh token every ~15 min. The view's capture
(`Capture(generación)`) stopped being valid at the first renewal, `PublishDetail` returned
`ErrPrivate` and every app discarded it: with Calendar open for more than 15 min, creating an
event for today published nothing. In addition each screen published what it had in view (the
month being looked at, the open folder), from loose routines that could overwrite each other.

What changes (piece 1 of 6, done):

1. **The app publishes after every change, with its own bounded read.** It replaces “the app
   publishes only what it has already requested for its screen, never an extra request”. Each app provides
   a `Fuente` (`apps/nucleo/widgets/fuentes`, without Fyne) that reads just what its widget needs
   —at most 4 + N requests per round— and a single `widgets.Publicador` decides when:
   a 1.5 s debounce (a burst publishes at the latest after 5 s), one round at a time, and
   **at most one round every 30 s due to writes**, which only count if they come from the
   product's own client (`Suite.ClienteConAviso`; never presence, notifications or preferences). It is
   also triggered by signing in, removing the lock, returning to the foreground, regaining the network, a
   Hub notification for the product, the Mail and Rooms live streams (also on reconnect), a 10-min
   heartbeat with the app alive, and midnight. **When going to the background** whatever the
   debounce or the 30 s window had deferred is published right away (`PublicarPendiente`, 4 s): that is when
   the widget is looked at, and iOS suspends the app seconds later. Screens no longer publish (`TestNadiePublicaWidgetsDesdeLaVista`).
2. **What is published does not depend on the screen.** Calendar reads the current month of all
   calendars, Mail always Inbox, Contacts and Notes the initial address book or notebook,
   Drive the root, the editors their list without search or starred items.
3. **The capture marker is the identity epoch.** `ajustes.MarcarIdentidad` writes
   `sesion.epoca` (128 random bits, salted fingerprints of the issuer, client, subject and
   organization, and the marker it was confirmed with); it only rotates if who changes or on
   sign-out. `PublishDetail` requires the captured epoch and that it be confirmed with the current marker,
   and writes the **current** marker in `generation`: the readers (Swift, Java) do not change. A
   session changed by a build that does not know the epoch fails closed until another
   app adopts and confirms it. It is called by `auth.Save` and `AlmacenSesion.Guardar` (always under
   `CerrojoSesion`, which is why it does not take it itself) and by the suite when adopting a session.
4. **A rejected session deletes that app's widget** (`ClearBundleIf`); a network failure
   keeps what was published, also if a secondary read fails (the quota, the live streams,
   the albums): only a 403/404 publishes without that data. A one-off rejection does not rotate the epoch of all fifteen.

What stays the same: no tokens, network or Keychain in the extension or the receiver; sanitizing and the closed list
of `kind`; deletion on sign-out and on locking; the reader checks marker, expiry and
lock; capture before requesting; nothing on Windows or Linux; `NSFileProtectionComplete`;
15-min expiry (30 at most).

Pending, each piece with its commit: (2) format v2 with the epoch as the marker, 24 h and
“Updated X ago” in the readers (done: next amendment); (3) the server's `datos` signal, also when the
asynchronous jobs of Photos, Cast and CertiPDF finish (done: the change-signal one); (4) background refresh on iOS
without presence or SSE (done in the suite, it is turned on with the fork's tag: the refresh amendment); (5) silent push with a single APNs registry in the Hub (amendment to
[ADR 0039](#0039)); (6) Android without an activity. Details in the
[roadmap](roadmap.md#widgets-nativos-lo-que-falta).

**Amendment (2026-11-07): format 2, one day of life and “Updated X ago”** (piece 2 of 6,
done). The owner asked that what is published not disappear after 15–30 min: the widget shows the
latest data and says when it is from.

1. **Format 2.** `generation` carries the **epoch** (`sesion.epoca`), not the session marker: a
   renewal no longer invalidates either the capture or what was published. The three readers (Go
   `Snapshot.Visible`, `Snapshot.swift`, `WidgetSnapshot.java`) accept **only** version 2 and require
   the same rule: the current epoch; **confirmed with the current `sesion.marca`** (if the last
   session was changed by a build that does not know the epoch —the fifteen apps are updated
   separately—, it is hidden); **no `*.cerrada`** in the folder; no lock; and at most
   24 h between `updated_at` and `expires_at`. The cases are a single file,
   `apps/nucleo/widgets/testdata/contrato.tsv`, run by Go, Java and Swift; a Go test
   also checks that Swift and Java say `86400`, `900` and version 2.
2. **24 h expiry**, and not less: the fifteen widgets show “today's things”, and after a day
   they would show yesterday's today as if it were today's; with something shorter (2 h) the widget would
   empty every night, which is what was asked to avoid. **Acknowledged cost**: if the session is
   revoked from the server and the app is not running, the latest data can remain visible for up to 24 h
   (previously, 30 min). It is bounded by deletion when the session is rejected, the device lock
   and the wake-up of pieces 4–6 ([security](security.md#widgets-24-h)).
   `RenovarTras` becomes 10 min, the Publisher's heartbeat: `updated_at` says when it was
   checked.
3. **“Updated X ago” from 15 min on** (`Snapshot.Rancio`). On iOS and macOS, the
   timeline is precomputed in full with `.atEnd` —now; +15, +30, +45 min and every hour
   from the check; every midnight; the end of each event; the expiry with no data—,
   up to 40 entries and without periodic reloads: the app reloads on publishing. The label is
   `widgets.updatedAgo` with `RelativeDateTimeFormatter`, computed for each
   entry's date. If the read fails because of data protection (`NSFileProtectionComplete` with the
   phone locked), the empty entry is **not** returned: the frame is drawn with the placeholder
   and it is requested again after 5 min. On Android the label is the **absolute time**
   (`widgets.updatedAt`, «Actualizado: 14:05», “Updated: 14:05”), whenever there is data: `RemoteViews` keeps
   the text it was drawn with, and nothing guarantees a repaint on unlock, so “X ago”
   would lie. With the device locked the receiver **does not repaint** the home screen
   (which is not visible), except for a `KEYGUARD` host, which is left blank; and an
   `AlarmManager.RTC` alarm (no wake-up and no permission) repaints at the next change: the end of an
   event, midnight or expiry (after 5 min if it was locked).
4. **Calendar trims its rows in the reader** (`widgets.Vigentes`, the same rule in all
   three): a timed event is valid until it ends; an all-day one is floating
   (UTC midnight) and is valid until the local midnight following its last day. The grid is
   that of the entry's month, with busy days only if the snapshot is from that month. The
   Calendar Fuente uses the same rule: until now it shifted the all-day event by an hour in
   Madrid and also marked the following day as busy.
5. **Android with the shared session**: the broker gives `marca_widgets` =
   SHA-256(«sytue-widgets\0» ‖ its epoch), which only changes with another identity, the lock or
   a revocation; it is the snapshots' epoch and what `Widgets.java` compares
   (`SesionCompartida.marcaWidgets`). The broker's plaintext epoch does not leave: it binds the
   owner's challenges.

Everything else stays as in the previous amendment; in addition, nothing new in the extension or the receiver:
no network, no Keychain, no session. Pending: pieces 3 to 6.

**Amendment (2026-11-07): the server's change signal** (piece 3 of 6, done). With the
app open, what changed outside it —on the web, on another device, by another person, when
a server job finished— did not reach the widget until the 10-min heartbeat.

1. **A new state event, `datos`**, with `{"producto": "<producto>"}` and nothing else: no
   title, id or content. The product is the app's scheme without `sytue-` and comes from a
   closed list (`hubnotify.ProductosDeDatos`, tied to the `FyneApp.toml` files by a test); the
   Hub validates it and rewrites the body, so a sender cannot slip anything else through the
   stream. Like `avatar`, it is not stored nor does it appear in the bell.
2. **Each service emits it on its writes**, with a single middleware
   (`hubnotify.TrasEscribir`, after `RequireAuth`): a 2xx to POST, PUT, PATCH, DELETE or a
   writing WebDAV verb, to whoever writes. What is written at keystroke rate or does not change
   what the widget shows goes without a signal: Mail drafts and settings, a document's operations and
   presence, the preview of a Notepads copy, GIF search and the
   Rooms composer draft (also in the native client, with `SinAviso`). It also goes through CalDAV, CardDAV and WebDAV, which is how the Calendar web
   and phones write (with `MKCALENDAR`), and through Drive's compatibility APIs.
   Mail carries it on every door that changes the mailbox: REST, `POST /jmap` (a batch
   that only reads turns it off with `CambioEn` without products), IMAP and the delivery of a new
   email, apart from the bell notification (it goes out even if Mail is muted).
   `TestCadaServicioMandaLaSenalDeCambios` requires all eleven services to mount it with their
   products (Drive also `pdf`; Docs `docs`, `sheets`, `slides` —the route does not say the
   type; on creation it does, and it goes only to its own— and `pdf`).
3. **And to those who see what was written** (`hubnotify.AvisarTambien`): in Calendar, the owner and each
   share of the calendar being written to, via REST or CalDAV (`QuienesVen`, one
   query, only if there is a signal in progress), and whoever gains or loses a share or sees the
   calendar deleted; in the ERP, the person responsible for an activity that someone else assigns to them or
   closes; in CertiPDF, the
   other party of a signature request (request, sign, reject, cancel); in Notepads and
   Bases, the person something is shared or unshared with.
4. **When asynchronous work finishes**, to the owner of what was processed: the Cast transcoder
   on finishing a job (or on failing with no retry left), the start and end of a live stream,
   the pass that completes the type and size of photos in Photos, and the refresh of a Calendar ICS
   subscription whose feed changed (a 304 does not), to its owner and its shares; the
   non-delivery notice that the Mail queue leaves in the sender's Inbox; what the Drive
   trash does with a document's file (Docs' internal door notifies the widget of its
   type, to the owner), and what an external editor saves via WOPI, which does not carry the Bearer and does not
   go through the middleware (to whoever edits and to the owner). Photos thumbnails
   are generated on request and EXIF is read on indexing, within the write: they
   have no asynchronous end to notify.
5. **Grouped** by (organization, person, product) over 3 s: the first one goes out immediately and
   those within the window, as one when it closes. In memory, per replica: it is a saving, not
   a cap.
6. **The Fyne bar forwards it to the publisher** (`alEstado` → `Suite.AvisoDelHub`), which only
   handles it if the product is its own (the Hub, also `mail` and `calendar`). The one for its own
   product also reaches whoever writes: within 6 s of their own write
   (`VentanaEco`, twice the grouping window) it is almost always their echo and respects the
   30 s minimum (`Publicador.AvisarDelServidor`); outside it, it is read after the debounce. Without
   that, a note's autosave made a round every 3 s. A round that starts after
   everything noted cancels the wait at the end of that window: an isolated write with its
   echo is one round, not two. What cannot be distinguished is someone else's change arriving within
   those 6 s: it waits until the end of the window, at most 30 s.

What stays the same: nothing new in the extension or the receiver; the signal carries no data about the
person; without the app open there is nobody to deliver it to until pieces 4–6, which will use it
to wake up. Contacts receives the Hub secret in its chart: until now it did not notify about
anything.

**Amendment (2026-11-07): the app wakes up on iOS for its widget** (piece 4 of 6, done
in the suite; it is turned on with the fork's tag). With the app closed nobody republished: the
widget showed the latest data, with its age, for up to 24 h. It is the owner's decision
«la app se despierta en segundo plano» (“the app wakes up in the background”), in its periodic part.

1. **A refresh task** (`BGAppRefreshTask`), `<App ID>.refresco`, requested at least
   every 30 min when going to the background and after each one (`sesion.CadaRefrescoEnFondo`):
   when it runs is decided by iOS based on usage. It is declared by `UIBackgroundModes = [fetch]` and
   `BGTaskSchedulerPermittedIdentifiers` in the iOS `Info.plist` of each app with a widget,
   only with `SYTUE_REFRESCO_APPLE = YES`, which requires `SYTUE_WIDGETS_APPLE = YES` and is separate
   because the one registering the task is the fork: with a tag without the patch, the plist would request a
   background mode nobody uses (App Review 2.5.4) (`empaquetar.sh plist_refresco`, from
   `plist_apple`, which writes that plist via Xcode and via the CLI: one source, not two). It is not an
   entitlement: it requires no capabilities or profiles.
2. **The app is woken, not the extension.** The fork's driver (patch `0016`,
   `mobile.BackgroundDriver`: `LaunchedInBackground`, `OnFirstScene`,
   `ProtectedDataAvailable` and `OnBackgroundRefresh`, in English like the rest of the fork) hands the
   task to `Suite.RefrescarWidgetEnFondo`, the single entry point for everything that wakes the app
   for its widget: the same Fuente and the same Publicador, with the time limit of one round (20 s).
   If the system cuts the task short, it is cancelled and reported as failed as soon as the round
   returns, or after 500 ms: completing it with the `ajustes` lock held (a `flock` in the
   App Group) would make iOS kill the app on suspending it (0xdead10cc). The suite requests
   that API through a type assertion on the driver, and not through a fork function: that way it compiles with
   any tag, and without it there is a scene from the start and everything is as before.
3. **Launched without a scene, the app does not act as an app.** iOS runs the entire `main` of
   `cmd/<app>`; until the first scene connects only the widget and the shared-data
   watcher run. Waiting (`Suite.CuandoHayaEscena`) are presence —a wake-up **never**
   publishes “Online”—, system notifications, the URL socket, the SSE and the bar's
   card and the startup loads of the fifteen `cmd`s: the screen, the importer, the
   browser for signing in. The apps' `AlCambiar`s are held and notified once on
   connecting, before what was waiting for the scene; those of the widget still run without a scene. This is
   pinned by `TestLosCmdCarganTrasLaEscena`, which follows the `main` of each `cmd`,
   `TestLanzadaSinEscenaSoloRefrescaElWidget` and `TestLaBarraNoAbreElFlujoSinEscena`.
   **Not when warm either**: the app sent to the background without swiping it away stays in memory, iOS
   resumes it for the task and what was waiting runs again —the overdue heartbeat, the SSE that
   reconnects—. That is why on iOS presence and the bar's SSE follow the **foreground**,
   not the scene (`Suite.PausaEnFondo`): they stop when leaving it and come back with it, and
   on returning a heartbeat is sent immediately. On desktop (background means losing focus) and on Android
   (notifications arrive through that SSE with the app in the background) they do not stop. This is pinned by
   `TestElDespertarEnCalienteNoLate` and `TestLaBarraCierraElFlujoAlFondoEnIOS`.
4. **With the device locked nothing is read.** The App Group is
   `NSFileProtectionComplete` and the app can read neither the session nor whether the suite is
   locked: it answers `Aplazado` and requests another task in 5 min, which iOS does not guarantee. In
   practice **the refresh only happens with the phone unlocked**, in use with another app;
   locked, the widget keeps the latest data and “Updated X ago”. The protection class is not
   lowered: lowering only `widgets/` would not be enough (`ajustes.json`, `sesion.marca` and the Keychain
   also have to be read) and lowering everything weakens the app's encryption at rest.
   It remains the owner's decision. Nothing is read without a session or with the suite
   lock either. And **what could not be read is not deleted**: launched cold with the phone
   locked, the app is “out” because it cannot read the session, not because there is none. Neither does its
   first widget notification delete (`ilegibleAlAbrir`), nor does `ClearIf` delete with an
   unreadable `sesion.epoca` (`ajustes.EpocaLegible`); before, the unreadable epoch came out empty,
   matched the “no session” one and the widgets of all fifteen were wiped. This is pinned by
   `TestLanzadaEnFrioBloqueadaNoBorraLosWidgets` and `TestSinPoderLeerLaEpocaNoSeBorraNada`.
5. **The suite lock is requested when the scene connects, not in `Abrir`.** Without an interface there
   is no Face ID to ask for —it would fail and the app would exit on every refresh—, and with the phone
   locked it cannot even be read whether it is set. So, where the app can be launched without a
   scene, `Abrir` does not verify: with the lock set (or unable to read it) the stored session
   **is not adopted** —neither when the watcher reads it on unlock, nor in the
   refresh, which with the lock set does nothing—, and on connecting the first scene
   verification is requested with whatever is on disk, before the loads and the URLs. Without
   this, `UIBackgroundModes=fetch` opened a lock bypass that did not exist without background
   launches. This is pinned by `TestElBloqueoSeExigeAlConectarAunqueAbrirNoLoLeyera` and
   `TestLanzadaEnFondoNoVerificaHastaLaEscena`.

What stays the same: nothing new in the extension; tokens, only in the app; macOS without refresh (it does not
have `BGAppRefreshTask`); Android unchanged (piece 6). An app closed by hand in the
app switcher receives no tasks until it is opened again. **Status** (2026-11-07):
on. The patch is in the fork (`v2.8.2-sytue.4`; the suite uses `.5`), with its
Objective-C compiled and started in the simulator by the UIKit matrix, and
`SYTUE_REFRESCO_APPLE = YES` ([runbook](operations.md#refresco-widgets-ios)); with an earlier
tag `empaquetar-test.sh` does not allow turning it on. Not tested on an iPhone nor through App
Review. Pending: piece 6.

**Amendment (2026-11-06), piece 5: the silent notification wakes the app through the same
door** (written on 2026-11-07). When something of the product changes, the Hub sends an empty
APNs notification and the app handles it with `RefrescarWidgetEnFondo`, the same entry point as the
periodic refresh: no presence, no SSE, no reading with the phone locked and
answering iOS only once (fork patch `0017`). The device registry, the cap
of one wake-up every 5 min per device, the person's switch (off by default)
and what Apple sees are in the [amendment to ADR 0039](#0039). Everything
above stays the same; the extension gains no network or token. **Status** (2026-11-07): the fork already carries the patch
(`v2.8.2-sytue.5`), and `SYTUE_DESPERTAR_APPLE = YES` since the same day by the
owner's decision, ahead of their steps (key, Push in the App IDs, profiles, Hub Secret): until
they are done, iOS signing fails and the Hub does not wake anyone ([runbook](operations.md#despertar-widgets-ios)).

**Amendment (2026-11-06), piece 6: Android wakes up without Google** (written on
2026-11-07). Without FCM, UnifiedPush or any third party (the EU rule of
`compliance/supply-chain` and [ADR 0039](#0039)):

1. **Go without an activity: it works.** `GoNativeActivity` already loads the library with
   `System.loadLibrary` before creating the activity, and that initializes Go and runs the `init`s
   without `main`. A `JobService` (`eu.sytue.nativo.RefrescoWidgets`) does the same and calls
   a JNI entry point (`apps/fyne/sesion/fondo_android.go`) that provides `RefrescarSinVentana`: it reads
   the stored session like `Abrir` (`leerSesionGuardada`, with the mobile Sytue ID
   client), builds the Fuente from `fuentes.Registrada` (the same one each `cmd` mounts, now
   with `Suite.FuenteWidget`, pinned by `TestLosCmdUsanLaFuenteRegistrada`) and publishes one
   round with a Publicador that stops when done. No `fyne.App`, no Suite, no presence,
   no SSE, no Hub (`TestSinVentanaPublicaConLaSesionGuardadaYNadaMas`). With the app open
   in the same process, it leaves it to its publisher. The door to Java, without an activity, is
   that of the entry point itself (`conRespaldo`: the activity's as soon as it exists). Tested in the
   machine's JVM with a c-shared `.so` built the same way (the real entry point, with
   `sistema` replaced): without `main`, called twice from a worker thread, with a
   return to Java from another goroutine and a Java exception caught as an error. **Not on a
   phone.**
2. **Every hour, and with the change signal.** The periodic job runs every hour with 15 min
   of slack, with network and without low battery; it is enqueued by the widget's receiver
   (`Widgets.onUpdate`) and removed by the last widget (`onDisabled`). With the phone
   locked nothing is read (like the receiver). The change signal arrives through the Hub's SSE, which
   only One keeps open with everything closed (`eu.sytue.nativo.Avisos`): One **relays** it to the
   product's app, which enqueues a one-off job (at most one per minute; if locked,
   it is retried in 5 min).
3. **The relay is not a broadcast with a signature permission**, as the design said: each of
   the fifteen apps is signed with **its own** Play certificate
   (`android/session-signers.json`), a `signature` permission cannot be granted between
   them, and declaring it in all fifteen would make the installations collide
   (`INSTALL_FAILED_DUPLICATE_PERMISSION`). It is a call **with no data** to the app's
   `eu.sytue.nativo.RelevoWidgets` provider (`<paquete>.sytue.widgets`), which checks the
   caller with the same boundary as the shared session (`SesionAndroid.llamante`: the Binder
   UID and each package's pinned certificate) and only accepts One; One, in turn,
   only calls a package from the pinned catalog whose provider is that one and whose signature is
   its own. What passes from one app to another is “something of yours has changed”: no product, no who,
   no what.

What stays the same: the widget receiver without network or session (the app reads); sanitizing, the closed
list of `kind`, the epoch and the lock; nothing with the suite locked. **Status**: turned on
on 2026-11-07 by the owner's decision, ahead of testing on a phone (`FONDO_GO = True` in
`apps/fyne/scripts/android-widgets.py`, pinned by a test); before that date, off
(`FONDO_GO = False`). Off,
the periodic job only repaints the label and the expiry (the same that
`updatePeriodMillis` and the piece 2 RTC alarm already did) and there is no relay. It is turned on in a
reviewed commit after the [runbook](operations.md#widgets-android-fondo) test, on a
phone: here there is no Android SDK or NDK, so the Java has been compiled against the
Android 16 framework (Robolectric's `android-all`) and the C entry point against the
JDK's `jni.h`, but neither the APK nor the Android `.so` has been built.

<a id="0079"></a>

<a id="0079--sytue-notepads-sobre-papel-un-cromo-neutro-elegido-por-la-app"></a>

## 0079 — Sytue Notepads on “paper”: a neutral chrome chosen by the app

`accepted` · 2026-11-05 · branch `claude/epic-brown-cnv5mt` · extends the shape axis of 0072

**Problem.** Notepads is a notebook: it is opened to write a line and read it. The suite's color
frame —Frame or Rail, in the house color, in cobalt or in the app's color—
dominated the screen: a strongly colored sidebar and top bar around a few
lines of text, the open day and the done checkbox in the brand color, and every glyph
of every bar with its accent fill. What people came to read was what was least visible.

**Decision.**

- **A fourth axis, the CHROME, chosen by the app and not by the person.** `data-cromo="papel"` on the
  `<html>` of Notepads (`AppLayout.astro`) and `tokens.ElegirCromo("papel")` when mounting its
  screen in Fyne. A single block in `branding/tokens/appearance.css` reassigns the frame, the
  side menu and the bar to the page background with the text ink, leaves the sheet in
  `surface` with a hairline instead of the shadow and takes “you are here”, the action and the focus to the
  primary. Everything is `var()` of role tokens, so it works in light, dark, system and
  translucent without a block per theme, and the color the person chooses stays on the action.
- **The block beats any rail** by specificity (three attributes) and by being last. The
  native interpreter (`apps/nucleo/tokens`) also stacks it last, after the shape.
- **Single-ink glyphs on paper.** `--sytue-glyph-fill: transparent` on the web and
  `tokens.EsPapel()` in `iconos.acentoDeAhora`. A requested accent, an icon on a colored
  fill or a status ink keep their fill.
- **The notebook** (styles/app.css and its Fyne mirror): each note is a line with its mark
  in a fixed column —a short dash, or a task's checkbox— and the text at 16 px; what is
  done fades to gray; the open day is an ink circle and “today”, the only color in
  the header.

**Ruled out.** _Changing the frame of the whole suite_: the other apps are panels with a lot of
navigation and the color frame organizes them; the decision belongs to one app. _A person's
preference_: it would be a fourth selector in “Appearance” to solve a problem of a single app.
_Hiding the suite menu and drawing our own inside the sheet_: it would duplicate the
navigation, the mobile drawer and its accessibility.

**Consequences.** Notepads looks different from the rest of the suite on purpose; removing the
attribute (and the call in `NuevaPantalla`) returns it to the color frame without touching anything else.
In Rail, the Notepads content shares the menu's background: only the menu's hairline separates
them. This is watched by `TestElCromoPapelEsUnEjeAparte` (native tokens) and
`TestSobrePapelElGlifoEsDeUnaTinta` (icons).

<a id="0080"></a>

<a id="0080--toda-la-suite-sobre-papel-el-cromo-de-notepads-pasa-a-las-quince-apps"></a>

## 0080 — The whole suite on “paper”: the Notepads chrome moves to all fifteen apps

`accepted` · 2026-11-06 · branch `claude/epic-brown-cnv5mt` · extends [0079](#0079)

**Problem.** 0079 left the “paper” chrome only in Notepads and ruled out changing the frame of
the whole suite. Having seen the proof of concept on Drive and Contacts, the owner asked for the same
chrome in all apps (2026-11-06): the color frame weighed as much in a panel as in a
notebook.

**Decision.** `data-cromo="papel"` on the `<html>` of each web app (the `AppLayout`s, the
Office one, the console and the cloud dashboard; not the public website) and
`tema.ElegirCromo(tema.CromoPapel)` in the `main` of each native app, right after
`sesion.Abrir`. The `appearance.css` block and its reading in `apps/nucleo/tokens` do not
change: it is the same axis, now turned on in all of them.

**Why in each `main` and not in `sesion.Abrir`.** The tests that open a session also check
the color chrome (the rail's amber fill, the translucent gradient…), and the
chrome choice is global to the process: done in `sesion.Abrir` it leaked into those tests.

**Consequences.** The color frame still exists and is tested; going back to it in an app means
removing the attribute and the call. Apps that paint their content on `bg` inside the
sheet show it gray inside the sheet's white hairline: they will be moved to `sheet-bg` as they
are touched.

<a id="0081"></a>

<a id="0081--la-clave-de-recuperación-de-matrix-la-custodia-rooms-para-la-sesión-oidc--superseded-por-0082"></a>

## 0081 — The Matrix recovery key is held by Rooms for the OIDC session · superseded by [0082](#0082)

`superseded` · 2026-11-08 · branch `sytue-native`

**Problem.** To set up Rooms end-to-end encryption, each person had
to create a recovery key, store it on their own and type it on every new
device. It is the step where people get stuck or lose the key, and a lost key forces
resetting the encryption and losing the encrypted history. The owner asked (2026-11-08) that the
key be stored tied to the Sytue ID sign-in, without the person managing it.

**Decision.** The Rooms service holds each person's recovery key:

- The `matrix_recovery_keys` table goes by `subject` (the Matrix account `@sytue_<sub>` is a
  single one across all their organizations). It is stored encrypted with the service KEK
  (`SYTUE_ROOMS_KEY_ENCRYPTION_KEY`, `encv1:`) and is only visible in maintenance transactions.
  Without a KEK nothing is stored.
- `GET`/`POST /api/v1/matrix/recovery-key` serve and store the caller's key. The
  identity comes from the OIDC token, and it goes through the same check as the Matrix bootstrap
  (active account and organization). Nobody can request another person's key.
- The browser, when opening Rooms on an unverified device, acts on its own:
  - If there is a held key, it recovers with it.
  - If the account has no encryption, it creates it and holds the new key.
  - If the account already had encryption with a key the service does not know, the usual
    buttons remain. When the person types their key and it works, that key goes into
    custody.
  - Resetting the encryption replaces the held key.
- When the account is deleted, its held key is deleted in the same transaction.

**Consequences.** The encryption stops being end-to-end with respect to the operator. Whoever
controls both the Rooms database and the KEK, or can act as the person before Sytue ID,
can read the key and, with it, the key backup of their rooms. With respect to third parties
nothing changes: Synapse still stores only ciphertext, and the key never goes through Synapse.
If one day we want to go back to the previous model for an organization, it is enough not to call the
custody. Held keys are deleted with `DELETE FROM matrix_recovery_keys`, and whoever already
had the key on a device keeps it.

<a id="0082"></a>

<a id="0082--la-clave-de-recuperación-de-matrix-envuelta-en-el-dispositivo-con-la-contraseña-o-la-passkey"></a>

## 0082 — The Matrix recovery key, wrapped on the device with the password or the passkey

`accepted` · 2026-11-12 · branch `sytue-native` · replaces [0081](#0081)

**Problem.** [0081](#0081) took away from the person the step of creating, storing and typing the recovery
key, but it did so by storing it in Rooms encrypted with the service KEK: whoever
controls the database and the KEK, or can act as the person before Sytue ID, can read it. The owner
(2026-11-12) set two rules at once: «ni crear ni usar clave de recuperación» (“neither create nor use a recovery key”), and «solo el
usuario debe poder acceder a los datos con su OIDC, nadie más» (“only the user must be able to access the data with their OIDC, nobody else”). Moreover 0081 went by
person and not by Matrix account: storage products with an isolated identity
(`@sytue_storage_<product>_<tenant>_<sub>`) have another key, and storing it in the same row
would have overwritten Rooms' one.

**Decision.** The key is still generated on the device and never shown, but what
Rooms stores is a **wrapper**: the key encrypted with a key derived on the
device that the service never sees.

- **Per Matrix account.** Table `matrix_recovery_wrappers` (Rooms migration 0077): the
  Rooms account and legacy storage share a row (empty `tenant`/`product`);
  each isolated account has its own (`tenant` + `product`). Several wrappers per account:
  one for the password and one per passkey, up to 16. Only visible in maintenance.
- **API.** `GET`/`PUT /api/v1/matrix/recovery-wrappers`, with `product` and
  `identity_version=2` to choose the isolated account. The same check as the Matrix and storage
  bootstraps: nobody reads another person's or those of an organization that
  is not theirs, and even if they read them they could not open them. Storing a wrapper with a different
  `key_id` removes those of the previous key (reset) in the same transaction.
- **Password.** Argon2id (64 MiB, 3 passes, 1 thread, 16-byte salt per wrapper) and HKDF
  with the label `sytue-matrix-recovery-wrap-v1/password`. It is asked twice on creation (a
  typo would leave the key unopenable on the other devices) and once on opening.
- **Passkey.** The WebAuthn PRF extension of the Sytue ID passkey (RP ID from
  `WEBAUTHN_RP_ID`), with HKDF and the label `…/passkey`. It is the recommended option. On the web
  it is implemented by the browser client; in Fyne there is not yet a platform WebAuthn path in the
  repository, so the desktop uses the password and the passkey remains
  pending.
- **Format.** AES-256-GCM with the AAD bound to the Matrix account, the type, the storage
  key and the passkey. The specification is in
  [architecture.md](architecture.md#custodia-envuelta-de-la-clave-de-recuperacion) and it is pinned by
  the vectors in `apps/nucleo/recoverywrap/testdata/vectors.json`, which Go and the web check.
- **Flows, with no key in sight.** When creating the encryption, the key is wrapped and stored
  **before** publishing it; if the service does not accept it, nothing is published and the retry uses
  the same key (the half-done setup lives in the system keychain). A new device
  asks for the password once per account and opens it. An account with a key the
  service does not know only offers verifying the device from another one; once verified, if the
  device has the key (storage accounts keep it), it asks for the password
  to wrap it. Resetting (Rooms only; storage still has
  `ErrResetUnsupported`) wraps the new key before publishing the new identity.
- **Password change in Sytue ID.** The old wrapper still opens with the previous
  password. If the person fails and then gets it right (the normal case after a change), they are asked for the
  current one to wrap again; the new wrapper replaces the password one.
- **Transition from 0081.** `POST /matrix/recovery-key` responds 410: no new key
  comes in in the clear. `GET` still serves any existing KEK copy, only for this: the
  client uses it once to recover the device without asking for anything and asks for the password
  to wrap it; when the first wrapper of the Rooms account is stored, the KEK copy is
  deleted in the same transaction. Copies that nobody has wrapped are **not** deleted automatically,
  because that person would lose their data. They will be removed with a later migration when
  `SELECT count(*) FROM matrix_recovery_keys` reaches zero or after a dated notice; until
  then 0081 remains valid for those rows, and only for them.

**Consequences.** The operator can no longer read the new keys: the database only has
ciphertext and public parameters, and neither the KEK nor Sytue ID opens a wrapper. The password
has an honest caveat: it is the same one typed when signing in to Sytue ID, so
a malicious operator who captures sign-ins could derive it. The passkey does not have
that weak point, and that is why it is the recommended one. A wrapper can be attacked offline with
the password: Argon2id at 64 MiB makes it expensive, but a weak password is still weak.
Whoever forgets the password and has no other verified device or passkey can only
reset (Rooms) and loses the encrypted history, as before 0081.

<a id="0086"></a>

<a id="0086--bóveda-post-cuántica-directa-age-mlkem768-x25519-v1-en-el-almacenamiento-de-la-suite-sin-synapsematrix"></a>

## 0086 — Direct post-quantum vault (`age-MLKEM768-X25519-v1`) for the suite's storage, without Synapse/Matrix

`accepted` · 2026-11-19 · security, e2ee, post-quantum, storage · builds on 0084 and 0085 · code in `apps/nucleo/recoverywrap/boveda_pq.go`, `services/id/internal/http/e2e/{comun,bridge}.js`, `services/drive/db/migrations/0040_pq_vault.{up,down}.sql`, `services/drive/internal/{store,service,http}/*pq_vault*`, `apps/frontend/packages/auth/src/server/pq-vault.ts`, `apps/frontend/packages/offline/src/vault/`, `apps/frontend/packages/shell/src/matrix/PqVaultGate.tsx`, `apps/frontend/apps/drive/src/{client/private-drive.ts,components/PrivateDriveGate.tsx}`

**Context.** Using Synapse/Matrix (isolated `v2` accounts, cross-signing, SSSS secret storage,
SAS/device verification and `matrix-js-sdk` + RustCrypto WASM) to encrypt the private storage
of the suite's apps introduced a fragile chain of state —in particular, “set up device / sign
in again with Sytue ID” loops for already existing accounts and a disproportionate operational
and memory load— without bringing any multi-user messaging value to apps that only store the
user's own encrypted records and files. At the same time, the suite already had the hybrid
post-quantum cryptographic core `age-MLKEM768-X25519-v1` (NIST FIPS 203 ML-KEM-768 + X25519 +
ChaCha20-Poly1305 via `filippo.io/age`, compiled to Go/Wasm in `apps/nucleo/cmd/pqc-wasm`) and
the 256-bit account unlock key (AUK) held non-extractably in the Sytue ID bridge (0084, 0085).

**Decision.**

1. **Direct hybrid post-quantum encryption without Synapse.** Each storage product of the
   suite opens its own **direct PQ vault** per `(tenant, owner_sub, product)`:
   - **Record encryption:** profile `age-MLKEM768-X25519-v1` (ML-KEM-768 + X25519 +
     ChaCha20-Poly1305) run in the shared Go/Wasm core (`pqc-wasm`), without implementing
     primitives in JavaScript or loading `matrix-js-sdk` or RustCrypto WASM.
   - **Owner authentication:** each record includes an HMAC-SHA256 proof
     (`org.sytue.storage.auth.v1`) computed with a 256-bit `owner_key` per vault, imported
     as a non-extractable `CryptoKey` in WebCrypto.
   - **Wrapper in the Sytue ID bridge (`v2`):** the private identity `AGE-SECRET-KEY-PQ-1…` and
     the `owner_key` are sealed in `id.<domain>` with AES-256-GCM and a key derived from the AUK
     through HKDF-SHA256 with its own domain separation (`sytue-pq-vault-wrap-v1`, distinct from
     the Matrix wrapper) and canonical AAD
     `sytue-pq-vault-wrap-v1\0<tenant>\0<product>\0<sub>\0<key_id>`. Session records from
     before this decision derive `vaultWrap` lazily and non-extractably without forcing a new
     sign-in. The shared vectors live in `apps/nucleo/recoverywrap/testdata/boveda_pq.json` and
     are checked in Go and in the bridge scripts.
2. **Direct store in PostgreSQL (`services/drive`, migration `0040_pq_vault`).** The tables
   `pq_vaults`, `pq_vault_keys` and `pq_vault_records` store only encrypted wrappers and
   `org.sytue.storage_pq` envelopes, under `ENABLE` + `FORCE ROW LEVEL SECURITY` by
   `(tenant, owner_sub)` and registered with `sytue_scope_admission_register`. The service rejects any
   record without `org.sytue.storage_pq` and validates the wrapper through
   `recoverywrap.PQVaultWrapper.Validate()` before persisting it. The apps' BFFs re-export the
   single plumbing `rutaBovedaPq` from `@sytue/auth/server` at `/api/pq-vault/[...ruta].ts`.
3. **Phase 1 scope (13 migrated apps) and exceptions.**
   - **Migrated to `PqVaultGate` / `PqVault`:** `drive` (full encrypted repository with S3 blocks
     at `/api/v1/cipher/blobs/drive/*` and PQ records at `/api/v1/pq-vault/drive/*`) and the 12
     apps protected by `MatrixStorageGate` (`bases`, `calcus`, `calendar`, `cast`, `certipdf`,
     `contacts`, `diapos`, `docus`, `erp`, `notes`, `photos`, `portal`/`hub`).
   - **Sytue Rooms (permanent exception):** keeps Synapse/Matrix and Megolm/cross-signing for
     real-time multi-user chat and federation/verification between people ([0077](#0077)).
   - **Phase 2 (pending):** migrate Sytue Mail (it requires adapting the encrypted SMTP ingress
     authority flow towards a PQ recipient) and the native `apps/fyne` clients (which already
     share `apps/nucleo/pqc` and `apps/nucleo/recoverywrap`).

**Consequences.**

- The 13 storage apps stop depending on Synapse, Matrix SSO, SSSS and device verification to
  open the user's data: signing in with Sytue ID is enough to unlock or initialize the
  `age-MLKEM768-X25519-v1` vault in a single step.
- End-to-end protection resistant to _harvest-now, decrypt-later_ attacks (ML-KEM-768 + X25519):
  neither `services/drive` nor Sytue ID can decrypt the records or the private keys.
- If a person loses their previous Sytue ID AUK, `PqVaultGate` offers an explicit reset with
  confirmation (`PqVault.reset`) that generates a new PQ identity without getting stuck in a loop.

**Fix after v3.22.0 (whoever was already encrypting could not get in).** Three faults added up:

- The vault operations (`sytue-e2e-vault-unwrap`/`-wrap`) did not carry to the bridge the
  `handoff` of the return from sign-in, so in Safari —partitioned bridge, no AUK in its
  storage— each return asked for “Sign in” again. Now it travels with all key operations
  (`unlock-bridge.ts`, `keyOperations`).
- A record from before this ADR whose box could not be revealed left the bridge in `unavailable`,
  with no way out; now it answers `signin`. And `wrong` (AUK expired in that browser) also returns
  a one-time handoff key, so that signing in again replaces it.
- `wrong` showed the Matrix text (“verify from another device”), which does not apply here.
  Now it offers **sign in to Sytue ID again** or **empty the encrypted data** with confirmation, and
  the same for any other failure when opening. If the vault has not a single record —the twelve apps
  that go through the gate without storing anything in it—, `PqVault` resets it on its own: there is
  nothing to lose. Emptying really deletes `pq_vault_records` (they used to be left orphaned and, since
  `transaction_id` is unique per product and not per vault, a re-import collided with them).

**One key for the whole account, also when typing it (after v3.22.0).** The unlock key is a
single one, but each app sent people to Sytue ID with `prompt=login`: in a browser that separates
the bridge's storage by origin (Safari) that meant typing the password once per app, and it was felt
as “one encryption per app”. And if the relay ended with no key to hand over (passkey without PRF,
the derivation did not finish), it returned to the app empty-handed and without saying anything: the
app only knew how to ask for “Sign in” again, endlessly.

- The bridge of a web app that brings its ephemeral key first goes through the relay **without typing
  anything** (`entregaSinTeclear`, `desbloqueo.go`). If this browser already has the key in the Sytue
  ID origin, the relay hands it over to the bridge after **one confirmation click in Sytue ID**: a
  script injected into an app can bring the person to that page, but not click for them, which is
  the same boundary that typing the password used to set.
- If it does not have it, the relay itself asks for the typed authorization (`sytue_unlock_typed=1`,
  only once). If there is still no key then, **it says so** and offers to retry with the password or
  to go back without unlocking (`E.relayOutcome`, `relay.js`): it never returns silently.
- Sytue Mail stays on Matrix (Phase 2). Whoever lost their key there has no reset: the storage
  authority (`matrix_storage_authorities`) only allows changing root with the signature of the
  previous root. Emptying it goes through migrating Mail to this vault, not through relaxing that
  authority.

**Native clients (Phase 2, 2026-11-23): pieces done and why no product changes yet.**

Done: the native vault opens and seals its wrapper with the unlock key that the device session
already has, without a bridge (`apps/nucleo/storage/pqvault`). It uses the same envelope, the same
owner proof and the same emptying as `pq-vault.ts`, with a shared vector
(`recoverywrap/testdata/boveda_pq_registro.json`). It satisfies `storage.RecordStore`; with
`storage/vaultfiles`, which mounts Matrix's file layer on it (`matrix.StorageFiles`, the same
code, without touching `Client`), it is a complete `records.Crypto` and `drive.Crypto`. The Fyne gate
is `cifrado.PuertaBoveda`: sign in again, retry or empty with a confirmation checkbox.

**Decision: native products stay on Matrix until three things are resolved.** None is switched
behind the gate, not even the records-only ones. Moving them now would be prudence backwards:

1. **On the web, the twelve records apps store nothing in the vault.** The gate only lets people
   through; their data goes through the server API. Natively, calendar, contacts, notes, erp, bases,
   cast, hub and the office apps store their encrypted records in Matrix (`storage/records`). Moving
   them to the vault would not make web and native read the same thing: it would create a third copy
   that nobody else reads. First it has to be decided where that encrypted data lives for both
   platforms.
2. **There is no reliable migration from Matrix to the vault.** `ImportLegacySnapshot` imports from
   the server source (`legacy` → `frozen` → `private`), not from Matrix. If the source is already
   `private` because the data was copied to Matrix, there is nothing to re-import. And a new vault
   cannot start empty on a `private` source without changing the engine: it has no receipt and stays
   in `ErrNotReady`. That same gap prevents “empty and start in the vault” for whoever cannot open
   Matrix.
3. **The device journal is shared.** Import and upload intents live in the same `offline.Store`
   and namespace (`<product>-private-v1`, `PendingUploadsNamespace`) for both backends, with their
   destination inside. A Matrix intent blocks the vault's and vice versa. The vault needs its own
   journal, and signing out has to delete it as it deletes the current one.

Drive (and with it photos, pdf and docs attachments) is the case that does converge: the web
already stores Drive in the vault (`private-drive.ts`). It lacks points 2 and 3. Proposed rule for
choosing the backend without asking for any key, only with what the server says:

- if the product's vault has records, the vault;
- if not, and the source is still `legacy`, the vault, importing as the web does;
- if not, Matrix, until there is a migration.

Mail and Rooms stay out: Mail because of delivery 4 of [0087](#0087), Rooms because of [0077](#0077).

**Owner's decision (2026-11-23): bring the native apps in line with the web.** It resolves point 1:
the native apps' encrypted records are not moved to the vault; the apps read and write through the
server API, like the web. Drive, photos, pdf and docs attachments will go to the direct vault.

Done, for the records (calendar, contacts, notes, erp, bases, cast, hub, docus, calcus and
diapos): `cifrado.AplicacionRegistros` chooses **per account** with the mode of that product's
server source (`GET /private-storage/migration`, without asking for any key).

- **`legacy`** → the view is mounted on the service API with the Sytue ID session, bound to that
  account (`sesionDeCuenta`: each request captures its authorization intent and does not go out
  with another account's token). The Matrix app does not start: its `CanStart` is set by the host.
  This is what mattered most: until now, opening a native app with the source in `legacy` started
  the migration to Matrix, **froze the source** and that person's web could no longer write.
- **Any other mode** (`building`, `frozen`, `private`) → Matrix, as until now: their data is there
  and the server source no longer accepts writes.
- Without being able to read the mode, neither one nor the other: “Retry”.
- Contacts reads the mode with `PrivateStorageMode`, which does not reject shared address books (in
  the API they do not matter; `PrivateStorageState` rejects them because it is the migration's gate).

**Why there is no import from Matrix into the API.** There is no reliable path: the source's shared
triggers (`sytue_private_storage_state_scope_guard`, `platform/pkg/pg/private_storage_v1.sql`) do not
let a `frozen` or `private` source go back to `legacy`, and there is no server write that receives
the Matrix records. So whoever opens Matrix keeps seeing their data through Matrix until that exists.
Whoever **cannot** open Matrix and has the source frozen still has no way out: the Matrix gate stays
as it was. The identified path is a “release the source” on the server: a new version of those
triggers that allows `frozen`/`private` → `legacy` only through that operation, with the
reauthentication of the emptying of [0087](#0087). It returns the server data as it was when frozen
and loses only what was written in Matrix, which is what that person can no longer open; the “empty
the encrypted data” checkbox says so. Done: see “Release the source” below.

**Drive, photos, pdf and attachments.** Points 2 and 3 above and their host were missing. Drive,
photos and pdf already choose vault or Matrix per account: see “Native Drive on the vault” below,
with what is still missing.

**“Release the source” (2026-11-23): the way out for whoever cannot open Matrix.** Done for the
eight records services (calendar, contacts, notes, erp, bases, cast, hub and docs with docs, sheets
and slides).

- **v2 triggers** (`platform/pkg/pg/private_storage_release_v1.sql`, marker
  `sytue:private-storage-release-v1`). The state row guard keeps all the v1 rules and adds a single
  return from `frozen`/`private` to `legacy`: the one from `sytue_private_storage_release`, with the
  epoch the person saw (another epoch is `PST03`, `private_storage_release_stale`), in its owner
  transaction and never under maintenance or merge. The transition is bound by a transaction GUC to
  `(table, tenant, owner, epoch)`; it is not an authorization boundary (the app role could set it,
  like `app.maintenance`): it prevents a generic `UPDATE` from reopening a source. Each service swaps
  its guard for v2 with `sytue_private_storage_release_install` in its own migration, without new
  tables.
- **What it returns.** The domain tables are not touched while the source is frozen (the write guard
  rejects them), so `legacy` again is the server data as it was when frozen. What was written later
  in Matrix is lost (and what Calendar's or Contacts' encrypted ingest left for that epoch): it stays
  in Matrix, but nobody reads it any more. The old epoch's snapshot stays, immutable; a later
  migration uses another epoch.
- **Audit** in the same transaction: the state row gains `releases`, a list that only grows this way
  (epoch, mode, rows, `released_at`, `auth_time`, `amr`). In addition, a line in the service log and
  a bell notification (`security.storage.released`, pinned like the security ones; One's is created by
  the Hub itself).
- **Route**: `POST /private-storage/migration/release` (`platform/pkg/soltarfuente`), with the scope
  header, without delegation, with `{"epoch", "discard_private_writes": true}` in the body —the
  confirmation is also required on the server— and an identification less than five minutes old,
  with a second factor if the account has one (`authn.IdentificacionReciente`, the same rule as the
  emptying of [0087](#0087)). If missing, 401 `insufficient_user_authentication` with `max_age=300`
  (RFC 9470). The services do not know whether the account has a second factor: Sytue ID now issues
  it in the access token (`mfa_enabled`); a token without the claim counts as “with a second factor”.
- **Web.** These apps never open Matrix on the web, so “the key does not open” is always the case:
  `MatrixStorageGate` paints, outside the vault gate, a notice when the source is not `legacy` and
  offers “Empty and go back to the server data” with a checkbox. If the service asks for
  reauthentication, it stores the already confirmed operation (account, product and epoch, ten
  minutes), sends the person to `/auth/login?reauth=1` (`max_age=0`) and repeats it once on return.
  The common BFF only lets `POST migration/release` through, never in Drive or Photos.
- **Native.** The Matrix panel of the records apps (`AplicacionRegistros`) offers the same way out
  when the key does not open (`Panel.ConfirmarSoltarFuente`). If the service asks for
  reauthentication, `Suite.Reidentificar` opens Sytue ID with `prompt=login` and `max_age=0` and it is
  repeated for the same account; afterwards the app drops Matrix and mounts the view on the API.

Seen while doing it and fixed afterwards: `hub_freeze_private_storage` read `notification_preferences`,
which does not exist, and One's `Begin` failed (hub 0013 redefines it on `notification_prefs.kind`).
ERP (syerp 0105) and Bases (0036) now deny in the database writing what the freeze captures while the
source is not `legacy` (409 in the API). In Bases, a shared base does not enter the snapshot and stays
editable: locking it would leave it read-only without an encrypted copy. Hub was first left without a
guard on purpose: its server inbox was still alive with the private source, and a guard would have
silently dropped the notifications other services send, security ones included. Guarding it first
required a private notifications ingress; it is done (below, “One's private notifications
ingress”). The migrations of calendar, contacts, notes, erp, cast and docs require PostgreSQL 17, so
their routes were tested only at unit level; the shared SQL and those of bases, hub and erp (with a
substitute collation), against PostgreSQL 16.

**One's private notifications ingress (2026-11-23): done, with the guard in place.**
Hub 0014 installs in its database the ingress shared with Mail and Calendar (`pg.WithPrivateIngressDDL`,
with its single type `hub.receive`) and 0015 puts the write guard on `notifications` and
`notification_prefs`. The order is the one that was needed: no notification reaches the guard any more.

- **Where a notification goes.** `notify.Service.Create`, inside its transaction and after the
  caps, reads the person's state row `FOR SHARE` (the freeze takes it `FOR UPDATE`; there is no race
  between deciding and writing). In `legacy`, to `notifications` as always. If not,
  `store.AdmitirAvisoPrivado`: its copy is stored in `hub_private_notices` and, if the person has
  already enrolled an ingress key for that epoch, it is encrypted for it (`age`, inline in
  `private_ingress_bodies`, the same sealing as Calendar; the body goes empty and the whole
  notification in the encrypted metadata). The notification does not travel through NOTIFY, only a
  `private_ingress` state event without data.
- **No notification is lost.** The copy exists from the same transaction that accepts the notification:
  - with no key yet, it is the queue; any operation of that person's ingress (including enrolling
    the key) encrypts what was waiting, and if encrypting fails, only its savepoint is undone and
    the notification stays in the copy;
  - encrypted but unconfirmed, it is the reserve: **releasing the source** returns to
    `notifications`, in the same transaction and with its id and date, everything no device
    confirmed. What was encrypted for a key that person no longer opens is not left unread;
  - when a device confirms its delivery (ACK verified by Rooms), the copy is deleted in that same
    transaction: from then on the notification only exists encrypted in its private store.
    Security ones (`security.*`, also `security.storage.released`) follow this same path
    without exception. The “you released the source” one is created by the Hub after releasing it,
    already in `legacy`. Accepted price: until the ACK the server stores the notification in clear
    text, as in `legacy`.
- **Caps.** The per-person, per-organization and per-type ones also count the copies of the last hour.
- **Guard (0015).** The same rules as the previous attempt, which was discarded for dropping
  notifications: a `subject` that cannot have a source (`email:…`, empty or longer than 1024 bytes)
  does not create a state row; a DELETE with `app.merge='on'` (account deletion) passes, the merge
  does not; the purge and “mark all” skip what is frozen (`hub_private_storage_frozen`); marking a
  frozen notification and changing preferences give 409 `private_storage_frozen`. The historical
  rebuild (already closed) answers `congelado`.
- **Account deletion.** `hub_private_notices` is in `merge.DatosDeLaPersona`. What is encrypted in
  the shared queue is retained, as in Mail and Calendar.
- **Routes** `/private-storage/ingress/*` (the shared ones), on the Rooms authority
  (`ROOMS_INTERNAL_SECRET`; without it, 503 `private_ingress_required` and notifications wait in the
  copy). On the device, `apps/nucleo/hub.IngressExecutor` stores the decrypted notification in the
  private store only once (its id is the delivery's), recognizes it on retry without restarting its
  read and returns the commit of the claim's room.
- **Devices.** `hub.Receiver` (`apps/nucleo/hub/receiver.go`) is the ingress on the device, like
  Mail's: enrollment of the epoch's ingress key (`ingress.NewEnrollment`) and a worker
  (`ingress.New` + `hub.IngressExecutor` + `ingress.NewHTTPSource` against the Hub's
  `/private-storage/ingress/*`) that claims, stores and confirms. With the source in `legacy` it does
  nothing. The Fyne Hub (`apps/fyne/cmd/hub`) first asks for Rooms' consent (like Mail), puts the
  receiver among the retained sources of the records plan —its journal is drained on unmount and
  is withdrawn or retained with the session— and starts it with the view: it reloads the inbox when
  storing a notification and stops on unmount, sign-out or account switch. If the ingress fails,
  the notifications already stored open anyway and what is pending stays in the Hub. `hub` enters
  the single list of ingress products (`storage/ingress.Product`), and its enrollment journals are
  deleted on sign-out like Calendar's.
- **Web bell.** The web does not open One's private store, so the API's list and counter join
  `notifications` with the copies not yet confirmed (`private: true`): with the source frozen the
  bell still sees the new notifications, and the data-less `private_ingress` event reloads it.
  Decision: they can be marked as read, in the copy (`read_at`), which is only the server's view: it
  does not travel encrypted nor change what the device stores, and it comes back with the
  notification if the source is released. “Mark all” marks its own and the copies and leaves what is
  frozen as it is (it used to give 409). A notification that a device already confirmed is no longer
  seen on the web: from then on it only exists encrypted in its private store. Frozen notifications
  still give 409 when marked.
- **Deployment.** The Hub's `ROOMS_INTERNAL_SECRET` in Compose and in the deployment configuration
  (`sytue-hub`, key `rooms-internal-secret`, optional, like Mail and Calendar); the deployment
  requires it.

The enrollment and receipt of `hub` against the real Rooms authority is still not tested end to end
(the tests use an authority that accepts any receipt and a worker with a fake source). Until a device
confirms, what belongs to a person with the source frozen waits in the Hub, is seen in the web bell
and goes back to `notifications` if they release the source: it is never lost.

**Native Drive on the vault (2026-11-23): done for whoever the web already migrated.** Native Drive,
Photos (its originals) and PDF read and write in the same vault as the web when the web has already
taken that person's Drive there; if not, they stay on Matrix without any change. Native import and
the move from Matrix to the vault came later (see below). Code in
`apps/nucleo/storage/pqvault/existe.go`, `apps/fyne/sesion/diario_boveda*.go`,
`apps/fyne/cifrado/{drive_boveda,aplicacion_drive}.go` and the hosts of `cmd/drive`,
`cmd/photos` and `cmd/certipdf`.

- **Choice per account, without asking for any key** (`cifrado.ElegirAlmacenDrive`), in this order:
  pending Drive work of that account in some Matrix journal on the device → Matrix; pending work in
  the vault journal → vault; the `drive` vault stores records (`pqvault.HasRecords`: it only reads
  the key state and one page of encrypted records, it never creates the vault) → vault; otherwise →
  Matrix. Without being able to count or ask: neither one nor the other, “Retry”. The Matrix app only
  starts for the account sent to Matrix (its `CanStart` is set by the host, as in
  `AplicacionRegistros`).
- **The journals are never mixed.** The vault has its own device journal, encrypted and separate, in
  `<settings>/private-work/<app>/pq-vault` (`Suite.DiarioBoveda`, with the same work token as the
  Matrix one): same namespaces, another store, so an intent of one backend neither blocks nor resumes
  the other's. An upload pending in the Matrix journal **is finished in Matrix**: while it exists,
  that account stays on Matrix (which resumes it on mount) and nobody touches its intent; with the
  Matrix journal empty, the account moves to the vault.
- **Sign-out.** The vault journals enter `directoriosAlmacenPrivado`: their pending work blocks
  sign-out like Matrix's, the explicit discard deletes it only for the account signing out, and the
  sign-out cleanup removes everything that is not durable work, the same as with Matrix's.
  `PendientesAlmacenamiento` also counts their uploads.
- **Unlock with Sytue ID** through `PuertaBoveda` (the unlock key of the device session; sign in
  again, retry or empty with a checkbox), without a new password or a loop. Its server is bound to
  the account (`sesionBovedaDeCuenta`): a vault request after switching accounts fails instead of
  carrying the other person's token.
- **Repository:** the usual one (`apps/nucleo/drive`) on `vaultfiles.Store` and the vault journal,
  with the same cycle (`RepositoryCycle`) and the same work registry as Matrix, so sign-out drains
  it. It opens only if the vault has the migration receipt of the server's epoch (the same
  `drive_migration_<sha>` object that the web's `PrivateDriveRepository.migrate` leaves); if not,
  `ErrNotReady` and nothing is mounted (except the import below).

**Importing from native and moving Matrix to the vault (2026-11-23): done.** What was left in
points 1 and 2 of the previous list. Code in `apps/nucleo/drive/{direct_import,vault_migration}.go`,
`apps/nucleo/matrix/storage_file_copy.go`, `apps/fyne/cifrado/{drive_boveda,drive_migracion}.go` and
`cmd/drive`; end-to-end tests in `apps/nucleo/storage/vaultfiles/drive_migracion_test.go`
(on `pqvaulttest`, with a fake server source and another vault as the copy's origin).

- **The choice rule gains two steps** and ends up like this: pending work in Matrix → Matrix; an
  unfinished copy from Matrix to the vault (`drive-vault-migration-v1` in a vault journal) →
  Matrix, where the whole data still is and from where it is resumed; pending work in the vault →
  vault; the vault with records → vault; **the server source in `legacy` → vault**, which imports
  it; otherwise → Matrix. Matrix no longer freezes anyone's source: with the source in `legacy` the
  vault always wins (also in Photos and PDF).
- **Native import** (`drive.Config.DirectImport`, set only by the vault host; Matrix still does
  not import onto an isolated identity). It is the same code that imported into Matrix and the same
  result as the web: it freezes with its own epoch, whose intent (`begin_`) is written to the journal
  **before** freezing; it copies each row with its intent and fixed transactions (a row already
  published is reread and confirmed, not rewritten), checks the inventory against what the server
  said and publishes the receipt at the end. An interruption leaves the epoch in the journal and the
  next opening resumes it; an opening with the receipt already in place does not read the source
  again. **An epoch frozen by someone else** (the web, another device or Matrix) is not imported from
  here: `ErrNotReady`, as before. That way a half-done copy of something else is never mixed with the
  server source.
- **Copy from Matrix to the vault** (`drive.Repository.MigrateToVault`), with confirmation: the Drive
  app, with the Matrix Drive open, offers “Move to the vault” in a notice that does not go away on its
  own and asks for a checkbox (`cifrado.PasoBoveda`, keys `vault.move*` in the 26 languages). It uses
  the identity cutover engine (`storage/transfer`): it copies each catalog revision with its links,
  each file re-encrypted in authenticated blocks without a clear-text temporary
  (`Client.MigrateStorageFileContentInto`) and checked against the original's commitment, rereads
  everything from the destination, downloads every copied file in full and publishes **the receipt
  last**. Until then the vault has no receipt and nobody opens it. **Matrix is not touched**, neither
  during nor after: its data stays there. Matrix cannot be frozen (the power fence belongs to the
  identity cutover controller), so its “fence” is a fingerprint of each room's authenticated catalog:
  if someone writes to Matrix during the copy, it is not accepted. With Matrix work pending on the
  device, it does not start.
- **Resume or repeat.** The plan and the mappings live in the vault journal: an interruption resumes
  where it stopped. If Matrix changed in between, the plan is no longer valid; the vault, which
  without a receipt only holds the copy, is emptied (`pqvault.Reset`, only as it was seen and with the
  copy's mark: see “The copy without races”) and it starts again. A vault **with** a receipt is never
  emptied: if the web migrated in the meantime, the vault wins (`ErrVaultInUse`). When finished, the
  map is archived separately (`drive-vault-migration-history-v1`, which does not count as pending)
  and the app chooses again: it opens the vault with its receipt, as if the web had migrated it.
- **Whoever cannot open Matrix** has in Drive's Matrix panel the way out of the records apps,
  “Empty and go back to the server data”, without reauthenticating: it records in the vault journal
  that it adopts the frozen epoch (`drive.AdoptRetainedSource`) and the vault imports it from the
  copy the server kept. What was written afterwards in Matrix stays there, unread; the server does
  not change (it does not work for a source already in `private`).

What is **still not done**, and why:

1. **Stuck Matrix uploads.** If the web already migrated to the vault and an upload is left pending
   in the Matrix journal that Matrix can no longer finish (its engine has no receipt for the epoch
   the web froze), the account stays on Matrix: the intent is kept, counts as pending on sign-out
   and is only lost with the explicit discard. The copy does not start with Matrix work pending
   either. Unchanged.
2. ~~**The web during a copy.**~~ Done: see “The copy without races” below. What remains is that, if
   the person **cancels** the copy from the web (or the mark expires because the device stopped),
   the web imports on top as before: either it collides with what was copied and fails without
   accepting anything (“Empty” with confirmation), or it leaves the vault with the server's copy and
   its receipt, and what was written in Matrix after freezing is not seen in the vault. Nothing is
   lost: it stays in Matrix.
3. **Writes to Matrix after copying**, from another device that has not chosen again yet, stay in
   Matrix. Without freezing Matrix there is no way to prevent them.
4. ~~**A journal lost halfway through a copy.**~~ Done: see “The copy without races” below. What
   remains is that, with the journal lost, the old mark (of an identifier that is no longer in any
   journal) has to expire —five minutes without renewal— before the copy can be repeated, or be
   cancelled from the web. The device is not identified (the mark's label is only
   `native-<system>`), so one's own mark is not “taken back”.
5. **Photos records:** see “Photos records and Docs attachments” below.
6. **Docs attachments:** ditto.
7. In the vault there is no ingress consent nor Matrix publication context
   (`BindPublicationContext` is null: the vault receives nothing from outside) nor live event
   notices (`AlEvento`): the view is refreshed on request.
8. On Android, Photos and PDF still have no private Drive (the consumers have no shared journal
   there, same as with Matrix); the Drive app does choose. The offer to copy is only in the Drive
   app; Photos and PDF follow what is chosen.

**The copy without races (2026-11-23): done.** Three gaps in the copy from Matrix to the vault.
Code in `services/drive/internal/store/pq_vault.go` (migration `0041_pq_vault_copy`),
`apps/nucleo/drive/{vault_copy,vault_migration}.go`, `apps/nucleo/storage/pqvault/vault.go`,
`apps/fyne/cifrado/{drive_migracion,drive_boveda}.go`, `packages/offline/src/vault/pq-vault.ts`,
`apps/drive/src/client/private-drive.ts` and `packages/shell/src/matrix/PqVaultGate.tsx`.

- **(a) Emptying is compare-and-swap on the server.** The vault gains `version`, which goes up with
  each new record and with each emptying under its row's lock (publishing takes `FOR NO KEY UPDATE`,
  emptying `FOR UPDATE`: a publication in flight either finishes first and is seen, or waits).
  `PUT keys` with `reset` requires `expect: {vault_id, version}` —without it, 400— and only empties
  if the vault is still like that; if not, 409 `pq_vault_changed` and nothing is deleted. The client
  sends **what it saw when deciding**: the native copy, what it read with its mark already set and
  before rereading the catalog (`VaultNotEmptyError.Expect`, which the app passes to
  `pqvault.Config.Expect`); the web, the vault it saw when the key did not open it
  (`PqVaultError.expect` → `PqVault.reset({expect})`); the silent reset of an empty vault, what it
  read before checking it was empty. That way the receipt of an import that finishes between “there
  is no receipt” and “empty” is never deleted (`pqvault.ErrChanged`, `PqVaultError('changed')`): the
  next attempt sees it and the copy stops (`ErrVaultInUse`). The server cannot read a receipt —it is
  encrypted—, so “never empty a vault with a receipt” is decided by whoever can read it, and the CAS
  guarantees that they decided over everything that was there.
- **(b) The “copy in progress” mark.** Before reading the vault, `MigrateToVault` sets on the server
  (`PUT /pq-vault/{product}/copy`) a mark with a random identifier stored in the vault journal, the
  label `native-<system>`, the type `matrix-copy`, the server epoch and five minutes of life; it
  renews it every minute while copying. With the mark active, the server only lets whoever brings
  it (`copy_id`) publish and empty; the rest get 409 `pq_vault_copy_in_progress`. The copy writes
  and empties with it (`api.WithPQVaultCopy`), keeps it when emptying and removes it when finishing
  or when seeing that the vault already has its receipt. If someone cancels it, the copy's next
  write is rejected and the copy stops. The web, before importing, looks at the mark: if it is
  active it writes nothing and the gate shows “Another device is moving your Drive to the encrypted
  vault” with **Retry** and **Cancel the copy** (`DELETE copy?id=`, which deletes no record: what
  was copied is left without a receipt and Matrix stays whole).
  When opening with the receipt, an already expired mark is removed. Another native device that
  wants to copy in the meantime writes nothing and shows the same (`PasoOcupado`).
- **(c) An orphaned copy is recognized.** With the mark, what is in a vault without a receipt is
  known to be a copy from Matrix even if the journal was lost: the choice (`ElegirAlmacenDrive`)
  sends to Matrix the account whose vault carries a `matrix-copy` mark (active or expired), where
  the data is and from where “Move to the vault” repeats it: with the old mark expired, the new one
  sees that the vault already had that of a Matrix copy (`VaultNotEmptyError.Copy`) and empties it
  safely (CAS and its mark) before starting. With no plan **and** no previous mark, what is half
  done is something else (a web import) and the copy **no longer empties it on its own**: it fails
  without writing.
- **Compatibility.** The migration only adds columns with a default value. Against an earlier
  server (which rejects unknown fields), emptying with `expect` is 400 and the native copy does not
  start (without `PUT copy`): it fails without writing anything, which is the safe thing, until
  `services/drive` is deployed. Native apps already installed (v3.22 and earlier) empty without
  `expect`: the new server allows it with the previous semantics (empties without comparing), but
  rejects it with 409 `pq_vault_copy_in_progress` if someone else has an active mark, and records it
  in the log (“emptied without expect”) to know when to withdraw it. Current clients always send
  `expect`; their writes without `copy_id`, like those of earlier ones, respect the mark.

**Photos records and Docs attachments (2026-11-23).** What was asked was to move them to the vault
when the person's Drive is already in it. Having looked at the code, neither of the two can be moved
there without breaking the web or risking data; what was done is the safe subset.

_Photos._ The web does not store Photos records (albums, favorites, edits, memories) in the vault:
its gate (`MatrixStorageGate` → `PqVaultGate`) only lets people through and the gallery reads and
writes in syphotos through the API; the originals are uploaded by the BFF to the server Drive
(`/api/drive-subir`) and syphotos indexes them. It is point 1 above, the same as for the records
apps, and its answer is the same:

- **Done:** native Photos uses `AplicacionRegistros` (`cmd/photos/private.go`). With the syphotos
  source in `legacy`, the gallery goes through the API, like the web: records in syphotos, originals
  in the “Photos” folder of the server Drive, opening a Drive file in the editor indexes through the
  API (idempotent). Without Matrix, without `PuertaBoveda` and without freezing the source. Before,
  native Photos always went to Matrix: with an isolated identity and the source in `legacy` it never
  got to open (`ErrNotReady`), and with the old identity it imported and froze the web's source.
  With the source already copied to Matrix (`building`, `frozen`, `private`), through Matrix as
  until now, with its originals in the vault or in Matrix according to `ElegirAlmacenDrive`.
- **Not done: Photos records in the vault.** A copy there would not be read by the web (it stays in
  syphotos), which is exactly the third copy of point 1. And what is already in Matrix cannot be
  moved without risk: the copy (`records.MigratePrivateCatalog` + `storage/transfer`, which already
  verifies inventory, file commitments and receipt) requires the origin to be **frozen** on the
  server (`FrozenStorageBindings`: the controller owning the rooms and the account unable to write).
  For the isolated Matrix identity that freeze does not exist —only the old identity's cutover has
  it, in Rooms—, so another device with Matrix open could keep writing after copying and that would
  stay in Matrix without anyone reading it. Required first: a freeze of the isolated `photos`
  identity in Rooms and reading the encrypted gallery on the web. To decide without races between
  devices there is already a reusable piece: the server accepts Photos' source plan by CAS
  **before** `Begin`, and its link (`SourcePlanRouting.Plan.Link.RoomID`) is the vault's `vault_id`
  or a Matrix room, so it says without keys who kept the import.
- **Uploading photos with Drive in the vault: fixed** (see “Uploading photos with Drive in the
  vault” right below). Before, with the server Drive's source frozen, uploading failed with 409
  `private_storage_migration_required` on the web and natively through the API.
- A source plan accepted in Matrix before `Begin` (source still `legacy`) stays in the Matrix
  journal: it counts as pending on sign-out and is discarded with the checkbox; the server data is
  not touched.

**Uploading photos with Drive in the vault (2026-11-23): done.** The gallery now uploads when the
person's Drive is in the vault, without writing anything in clear text on the server and without
overwriting any photo. What was decided is the minimum that leaks nothing: **the original goes
encrypted to “Photos” in the vault's Drive and does not enter the server index**.

- **Why without an index (on the server).** syphotos indexes by reading the original from the
  server Drive and generates the thumbnails there; with the photo in the vault it cannot read it.
  Giving it an entry with the vault id would give it metadata (date, name, size, EXIF) that it does
  not have today, and even so it would not know how to show it. So the encrypted photos **do not
  appear in the timeline, the albums or the search**: the web lists them separately, in the
  browser. _Superseded_ by “The encrypted gallery” (below): the index exists, but on the client and
  encrypted in the vault; the server still does not have it.
- **Thumbnails: none stored.** No thumbnail is generated or stored (neither in clear text nor
  encrypted): each photo is decrypted in memory when shown and the browser scales it; meanwhile, a
  placeholder with a padlock. Storing encrypted thumbnails in the vault would leave loose files in
  the person's Drive and a second copy to maintain; with the separate gallery and batches of 60 it
  is not needed. Above 64 MB it is not decrypted for the view. _Superseded_ by “The encrypted
  gallery”: with the photos in the timeline they are needed, and they are stored encrypted in the
  vault.
- **Web** (`apps/photos/src/client/{subida,driveCifrado}.ts`, `components/FotosCifradas.tsx`).
  `/api/drive-subir` translates Drive's 409 (only with its code) into 409 `drive_private_vault`;
  the gallery then sends that photo and the following ones to the vault's Drive, with **the same
  repository and the same gate as web Drive**: `createPrivateDriveGate` moved from `apps/drive` to
  `@sytue/offline/vault/drive` (Drive re-exports it) and the sink that assembles a decrypted file
  (`collectPrivateDriveFile`) is shared by both. The gate gains `importar: false`: the gallery
  **never imports or freezes** the server source; it opens the repository only with the migration
  receipt (`initialize`, without reading anything in clear text) and, if Drive is still moving to
  the vault, it says so. `PqVault.open` gains `existing`: the vault is opened only if it already
  exists; it is never created, reset (not even silently with the wrong key), resealed nor does it
  register its signing key from Photos. A name already in “Photos” is uploaded as
  “IMG_0001 (2).jpg” (uploading with the same name would create a new version). The gallery shows
  “In your encrypted Drive” only if the server Drive's source is no longer in `legacy` (checked
  without keys), and opens the vault when the person asks for it or when uploading.
- **Photos BFF for the Drive vault** under `/api/drive-cifrado/api/…`: `pq-vault` (GET and POST,
  without PUT of keys), `matrix/storage/blobs` (GET and PUT, without DELETE) and
  `matrix/storage/migration` (GET only: it never starts a migration), with the scope bound to
  `drive`.
- **Sytue ID bridge** (`bridge.js`): on its own host (`photos.`), Photos can **open**
  (`sytue-e2e-vault-unwrap`) the `drive` vault (`VAULT_READERS`, `productOpenAllowed`); creating it
  or resealing it (`vault-wrap`) is still Drive-only. On an organization host it already could, like
  any app.
- **Native** (`apps/fyne/apps/photos`, `cmd/photos`). The API gallery asks on each upload where to
  upload (`App.DriveDeSubida`): with the server Drive's source in `legacy`, there; if not, to the
  private Drive of the account chosen by `cifrado.AplicacionDrive` (the vault, `vaultfiles.Store`,
  or Matrix), through `galeriaDriveData`. What is uploaded there is not indexed (`ErrEnBoveda`,
  which the backup counts as done) and overwrites no photo. Without the private Drive open, the
  gallery shows its panel to unlock it at the top and says so (`gallery.uploadVaultLocked`); a 409
  from the server's encrypted drive (`api.EsUnidadCifrada`) is counted the same way, not as “could
  not upload”.
- **Messages** in the 26 languages (web Photos catalog, which native embeds):
  `gallery.uploadedEncrypted`, `gallery.uploadVaultSignIn`, `gallery.uploadVaultNotReady`,
  `gallery.uploadVaultLocked`, `gallery.uploadVaultUnavailable` and `gallery.encrypted.*`.

What is **still not done**:

1. ~~**The encrypted photos are not really in the gallery.**~~ Done on the web: see “The encrypted
   gallery” below. Memories, similar, nearby and AI still do not include them, and it is said.
2. ~~**Native does not list them.**~~ Done: its own section in the gallery, with the same index (see
   below; they are not yet mixed with the server timeline natively).
3. ~~**The backup with the app closed still uploads to the server Drive.**~~ Done: it no longer
   insists on the 409 (see below); it uploads encrypted only if the app is alive in the process and
   its encrypted Drive is already open.
4. **What was already in the index** before moving Drive to the vault stays in syphotos with its
   server thumbnails; it is not touched.
5. **Where the device has no private Drive** (Photos on Android, point 8 above) it still cannot
   upload with Drive in the vault; now it says so with its message
   (`gallery.uploadVaultUnavailable`: “upload them from the web”) instead of a generic failure.

**The encrypted gallery (2026-11-23): done, with what is said below.** The photos in the vault's
Drive are first-class in Photos for whoever has their Drive there, without a single photo in clear
text or a single piece of metadata reaching the server. Code in `apps/nucleo/photos/galeriacifrada`
(Go, with the format and its tests), `apps/frontend/apps/photos/src/client/galeriaCifrada.ts`,
`src/lib/{exif,lineaMezclada}.ts`, `src/components/{FotosCifradas,AlbumesCifrados}.tsx` and
`apps/fyne/apps/photos/{galeria_cifrada,editor_cifrado,copia_fondo,copia_app}.go`,
`apps/fyne/cmd/photos`.

- **Where the index lives: encrypted files of the vault's own Drive**, inside “Photos”:
  `.sytue-galeria/indice/<id>.json` (one entry per photo, with the Drive file id: capture date from
  EXIF —extracted on the device—, already oriented dimensions, favorite, albums, the hash of the
  indexed content and the id of its thumbnail), `.sytue-galeria/miniaturas/<id>.jpg` (512 px, made
  on the device) and `.sytue-galeria/albumes/<id>.json` (one record per album). They are encrypted
  with the same key and the same format as the photo; the server only sees opaque blocks, as with
  any Drive file. They are **not** records of another type in the `drive` vault (the Drive catalog,
  web and native, and the copy from Matrix to the vault verify what is there: a foreign type there
  is a needless risk) **nor** a new `photos` vault (that is the one for the Photos records above,
  which is still undecided; creating it would make native believe it already stores records, and
  the Sytue ID bridge does not allow creating it from Photos). So nothing changes in the BFF, the
  bridge or `services/drive` either. The `.sytue-galeria` folder is visible in the Drive app.
- **Nothing is ever lost.** One entry per photo, not a whole index: two devices touching different
  photos do not overwrite each other. Each change (favorite, album) rereads the entry right before
  and applies only its own; if nothing changes, it does not write. Writing an entry again is a new
  version of the same Drive file: the previous one stays in its history. An entry that is missing,
  cannot be read, is not valid or belongs to other content (the photo was edited) stops nothing:
  the photo comes out with what Drive says (name and upload date) and is **rebuilt from the
  original** (a few per opening: 30 on the web, 10 natively), keeping favorite and albums. Nothing
  is deleted from the gallery: no photos, no entries, no thumbnails. Two `.sytue-galeria` created
  at the same time by two devices are read the same way (always the one with the lower id).
- **Web.** Opening the encrypted Drive is still on request (“Show”) or when uploading. With it open,
  the encrypted ones **enter the timeline by date** with the server ones (an encrypted one only
  appears when the server timeline has reached its date, so as not to skip months), with a padlock;
  in **favorites** (the marked ones), in the **search** (in the browser, by name and by date, ahead
  of the server results) and with the year jump. Heart, viewer (decrypted in memory,
  previous/next, download), **encrypted albums** (create, add, remove; on the albums page, a
  separate section) and **editor** (rotation, mirror, exposure, contrast, saturation, temperature,
  with the editor's engine) write only to the vault; saving an edit uploads a **new version** of the
  same file (the original stays in the version history) and rebuilds the entry without losing date,
  favorite or albums. Uploading records each photo in the index right away (if it fails, the photo
  is already safe and gets rebuilt).
- **What needs the server does not include them, and it is said**: Sytue AI (describe, ask, edit
  with AI), similar, nearby, memories, duplicates, sharing, trash and bulk actions. The “In your
  encrypted Drive” strip and the viewer say so (`gallery.encrypted.note`); in selection mode the
  encrypted tiles are not selected and say so on hover (`gallery.encrypted.notSelectable`).
- **Native (parity with the web, 2026-11-23).** The API gallery does the same as the web with the
  same index and the same rules, in Go (`galeriacifrada`): the encrypted ones **enter the timeline
  by date** (`MezclarLinea`, the rule of `lineaMezclada.ts`, with the chosen year and the search),
  with a padlock on the tile, in **favorites** and in the **search** (on the device, by name and by
  month). In the timeline, an encrypted one is an `api.Photo` with the file id prefixed with
  `cifrada:`; the Photo libraries ask the app for their thumbnails, which decrypts them
  (`miniaturasCifradas`), and **never** the server. In selection mode they are not selected and say
  so (`gallery.encrypted.notSelectable`). Opening one shows it in the **full-screen viewer**, like
  the server ones (see “The native viewer”, below), with its favorite, its **encrypted albums**
  (checking adds it, unchecking removes it, and “New encrypted album”) and **Edit**: the usual
  native editor without the Sytue AI tab and with “Save as new version”
  (`gallery.encrypted.{saveVersion,editHint}`), which applies the recipe (rotation, mirror, crop,
  exposure, contrast, saturation, temperature) to the whole photo with the Photos engine
  (`photos/render`, orientation EXIF included) and uploads it as a new version **through
  `ReplaceFile`**: the vault's Drive rejects a version that another device has already changed
  instead of overwriting it (`galeriacifrada.Editar`/`GuardarEdicion`, which return the new file for
  the next edit). The entry is rebuilt without losing the capture date (the edit comes out without
  EXIF and the previous one is kept), the favorite or the albums. The **Albums** page has the
  “Encrypted albums” section (create, open with its photos, compact empty state
  `gallery.encrypted.albumEmpty`); adding and removing write only the photo's entry (`PonerEnAlbum`).
  “Similar” and “nearby” say they do not include them (`gallery.encrypted.note`), as does the “In
  your encrypted Drive” header, which no longer carries the photo strip: it keeps “Show”, the state
  and the compact empty state. It does not ask to unlock the Drive on its own (`App.GaleriaCifrada`
  with `abrir` false): with it closed it offers “Show”. What “Add photos” and the backup with the app
  open upload to the private Drive is recorded in the index from the local copy, without downloading
  the original again. The gallery's hidden Drive facade (`galeriaDriveData`) opens `Download` and
  `ReplaceFile` for that; the rest stays closed. No new text: the web keys were already in the 26
  languages of the catalog that native embeds.
- **The native viewer (2026-11-23).** The full-screen viewer takes what belongs to the photo from its
  **source** (`apps/fyne/apps/photos/visor_fuente.go`): the server one (its export) or the encrypted
  gallery one (`cifrada:`), whose original is decrypted **only in memory**
  (`galeriacifrada.Galeria.{Vista,Imagen,Exportar}`: the 2048 px “large” one, the whole photo at
  real size and the JPEG or PNG export, oriented by its EXIF and without EXIF in what is exported).
  It walks the **mixed timeline** being shown (and, from “Albums”, the open encrypted album), so the
  arrows, Home/End, the slideshow and the “12 / 240” move from one kind of photo to the other.
  **Download** writes the decrypted data only through `App.Guardar` (the system downloads or, on
  mobile, the gallery), without a temporary file or cache; the viewer's Photo library stores the
  “large” one, as with the server ones. Favorite, encrypted albums (a menu with the ones that have it
  checked and “New encrypted album”) and Edit write only to the vault. With an encrypted one, what
  goes through the server **is not offered** (similar, describe, ask, rotate —rotation is done in
  Edit—, trash, map and nearby, system gallery), not even through its shortcuts, and the info panel
  shows the padlock and `gallery.encrypted.note` instead. The dialog that used to show it is
  removed.
- **The backup with the app closed** asks for the server Drive's state (without keys) before
  uploading anything. With the source in `legacy`, as always. If not, **it does not upload to the
  server Drive**, and what it does depends on who can open the vault (decided on 2026-11-23):
  - **The key** to unlock is the Sytue ID one: it goes with the session in the **system's secure
    store** (Keystore on Android, Keychain on iOS; `auth.guardado`), with no copy anywhere else, and
    it is used by the app's vault gate (`cifrado.PuertaBoveda`), which is what knows how to open it
    with its journal, its leases and the account's authorization intent. The background round **does
    not read or store it**: it only asks whether it is there (`App.ClaveEnDispositivo`, which
    immediately deletes the copy the session returns).
  - **With that account's app alive in the process** (Android with the process open, the iOS refresh
    task) and the key on the device, the round asks **that same gate** to open the encrypted Drive
    —what “Show” does—, waits for it to be ready (at most one minute, within the task's time limit)
    and uploads encrypted through it, recording it in the index. If the key does not open, the gate
    stays waiting for the person and the round ends without uploading.
  - **Without the app alive** (Android waking up only the library, without `Suite`) **nothing is
    opened**: opening the vault outside its gate would be a second way of handling its keys, without
    journal or leases, and its guard is not weakened for the backup. Neither with the app alive and
    without the key on the device (one has to sign in to Sytue ID again, and in the background there
    is nobody to ask).
  - In those cases the round ends without counting failures (`ErrCopiaEsperaBoveda`, `SinCambios`),
    but **it no longer waits silently**: if there are photos to upload and the round could run
    (permission, network), it notifies in the system (“Open Sytue Photos to finish the encrypted
    backup”, `backup.encryptedWait{Title,Body}`), **once a day** at most
    (`registroCopia.AvisoBoveda`, which a complete round clears).
  - **With the app open, the backup continues as soon as the encrypted Drive is opened**
    (`App.DriveCifradoAbierto`, from mounting the private Drive in `cmd/photos`): the round that
    stopped because of it, or the one the background backup left waiting, does not wait for the next
    one (15 min).
  - Without being able to ask for the state (no network, earlier server), as before.
- **Messages** in the 26 languages (web Photos catalog, which native embeds):
  `gallery.encrypted.{note,badge,open,notSelectable,actionFailed,saved,albums,newAlbum,saveVersion,editHint,albumEmpty}`
  and, native only, `backup.encryptedWait{Title,Body}`; removed `gallery.encrypted.{hint,more}`. The
  native viewer adds none.
- **Tests.** The format is tested with the same files in Go and in TypeScript
  (`galeriacifrada/testdata`); the web, on the real `PrivateDriveRepository` and its synthetic store
  (nothing in clear text reaches the store: no names, no dates, no album titles). In Go, the order
  of the mixed timeline (`linea_test.go`), the albums and the editing (`edicion_test.go`) and the
  version saved on the real vault (`boveda_test.go`: `vaultfiles` on `pqvaulttest`, the previous
  version stays in the history, nothing in clear text in the blocks and an edit on an old version is
  not saved; viewing and exporting write nothing and leave nothing in clear text); natively, the app
  with the timeline, the tile, the albums and the editor
  (`apps/fyne/apps/photos/galeria_cifrada_test.go`), the viewer (`visor_cifrada_test.go`:
  previous/next along the mixed timeline, download only through `Guardar`, without the server
  features) and the backup (`copia_fondo_boveda_test.go`: it opens through the gate with the key on
  the device, without it it notifies once a day, and with the app open it continues when the
  encrypted Drive is opened).

What is **still not done** in the encrypted gallery, and why:

1. **Native: what is not yet like the web.** Saving an edit closes the editor and goes back to the
   gallery. Encrypted albums cannot be renamed or deleted (not on the web either). Natively with
   Matrix (not through the API) there is no encrypted gallery.
2. **The backup with the app closed does not open the vault without the app alive**, on purpose (see
   above): opening it outside its gate, without `Suite`, would be another way of handling its keys.
   It notifies and waits for the app to be opened. On iOS the notice does not show (the Fyne backup
   one is not used there). On Android, Photos still has no private Drive (point 8 above), so the
   photos wait there.
3. **Performance with many photos**: reading the index means downloading one entry per photo (four
   at a time). A snapshot derived from the index (which would not be a source of truth) would speed
   it up.
4. **No thumbnail** for what the device cannot decode (HEIC in many browsers): those show the
   padlock and are seen when opened if the browser can.
5. **What was already in the server index** before moving Drive to the vault stays there; it is not
   moved to the encrypted gallery.

_Docs._ The “attachments” of a private document are not Drive files: images go inside the content
(dataURL) and natively the snapshots and blobs are binary rows of the records repository itself, so
they follow the records (API with `legacy`, Matrix otherwise). What Docs does write to Drive, from
the **server**, is each document's “home” file (`hogar_en_drive.go`), the signature PDFs and “Save
to Drive”. With Drive in the vault, Drive rejects those writes with 409
`private_storage_migration_required`, and that broke Docs:

- **Done (`services/docs`):** the Drive client distinguishes that 409 by its code
  (`drive.ErrUnidadCifrada`; any other 409 is still what it was). **Creating a document** no longer
  fails with 503: it is created only in Docs and without an anchor, as in a deployment without
  Drive; the server does not leave a clear-text copy in a drive the person has encrypted. **Save to
  Drive** and the signature PDF answer 409 `drive_private_vault`; the editor says so in its language
  (`doc.msg.saveToDriveVault`, 26 languages) instead of “the changes could not be saved”. The
  **trash** took that 409 for “name taken”: now trashing, restoring or checking the frozen copy does
  not apply, and purging still asks about the owner's legal hold, like a document without a file.
- **The Docs file inside the vault: done for “Save to Drive”** (see right below). It is written by
  the client, as was said; the `drive:<id>` anchor is not reused for the vault's new files.

**The Docs file in the vault's Drive (2026-11-23).** With the person's Drive in the vault, web
Docus, Calcus and Diapos store the document's file there, **encrypted in the browser**: no server
receives it in clear text and the Docs one only gets the anchor.

- **Two anchors** (`packages/office/src/editor/state/anclaDrive.ts`): `drive:<uuid>` is a file in
  the server Drive, as always; `vault:<uuid>`, one in the vault's Drive. Nothing on the server
  (Drive's trash, “open from Drive”, `ficheroDe`) treats a `vault:` as its own: it only recognizes
  `drive:`. A `drive:<id>` from before moving to the vault still works if that file was copied:
  `PrivateDriveRepository.migrate` keeps the id (`source_id`).
- **Save to Drive** (`editor/state/driveCifrado.ts`, `useDocument.saveToDrive`). With a `drive:`
  anchor, first the BFF as always; if it answers 409 `drive_private_vault`, or the anchor is already
  `vault:`, or there is no anchor and Drive is in the vault (checked without keys, by the source
  state), the browser exports the document through `/api/export`, opens the vault and uploads:
  - to the anchored file if it is still alive in the vault, with its name, in its folder and in the
    format of its extension: a new version (if it has no round trip, `.pdf` or `.txt`, it is not
    rewritten and it is said: `doc.msg.saveToDriveUnsupported`);
  - if not, to a new file in “Sytue Docs” at the root (the server's folder), in ODF (the format of
    the server's home file) and without overwriting any name (“Minutes (2).odt”).
    The menu offers “Save to Drive” also without an anchor when Drive is in the vault.
- **The anchor is set by the service, not blindly by the client**
  (`PUT /api/v1/documents/{id}/vault-anchor`, `services/docs/internal/http/ancla_boveda.go`, BFF
  `/api/vault-anchor`): only the owner, only `vault:<canonical uuid>`, only from no anchor or from
  another `vault:` —a `drive:` is not replaced: the trash still uses it for the server's frozen
  file—, comparing with the current one (`expect`, 409 `vault_anchor_changed`) and without touching
  `updated_at`. If it cannot be anchored (another tab did it first, no network), what was saved
  stays in the vault and the next save tries again. Whoever is not the owner saves a copy in THEIR
  encrypted Drive and anchors nothing.
- **Same gate as Photos.** The consumer opening (`openConsumerPrivateDrive`: the vault only if it
  exists, the repository only with the receipt, `importar: false`), the failure reasons and the
  folder and name helpers moved to `@sytue/offline/vault/drive`; Photos uses them the same way.
  Office BFF for the Drive vault under `/api/drive-cifrado/api/…`: `pq-vault` (GET and POST, without
  PUT of keys), `matrix/storage/blobs` (GET and PUT, without DELETE) and `matrix/storage/migration`
  (GET only). The Sytue ID bridge allows **opening** the `drive` vault from `docus.`, `calcus.` and
  `diapos.` (`VAULT_READERS`); creating or resealing it is still Drive-only. CertiPDF also opens it
  since 2026-11-23 (see “Signing on the device” below).
- **Messages** in the 26 languages (office catalog, and the regenerated Fyne one):
  `doc.msg.savedToDriveEncrypted(In)`, `doc.msg.saveToDriveVaultSignIn` (vault locked),
  `doc.msg.saveToDriveVaultNotReady` (Drive still moving to the vault),
  `doc.msg.saveToDriveUnsupported`, and `doc.msg.saveToDriveVault` for any other failure (download
  and upload from Drive).

**Native Docus, Calcus and Diapos (2026-11-23).** They do the same, with the rules in the core
(`apps/nucleo/office/guardardrive`) and not in the view, and they encrypt on **the device**:

- **The same decisions** (`guardardrive.GuardarDocumento`): with a `drive:` and Drive on the server
  (source `legacy`), a new version in that file of the server Drive —what the BFF
  `save-to-drive.ts` did, which native did not have: the entry did not show up—; if not, to the
  vault's Drive: to the anchored file (`drive:` or `vault:`) if it is still alive, with `ReplaceFile`
  tied to the version read (if another device published in the meantime, it fails instead of
  overwriting it), or to a new ODF one in “Sytue Docs” without overwriting names (exact, as the vault
  compares: files and folders). The owner anchors with `api.Docs.AnclarEnBoveda` (the same
  `PUT …/vault-anchor`). The entry shows with an anchor, or without one if Drive is in the vault
  (checked by the source state, without keys).
- **Nothing in clear text to the server:** the Drive mode is checked BEFORE uploading; with the
  vault, the server Drive is not even asked for the file. If Drive moves to the vault between
  checking and uploading, the 409 `private_storage_migration_required` leads to the vault, as on the
  web.
- **No half-done state:** it exports before writing anything; if exporting or uploading fails, no
  new file, version or anchor is left (only, if it was created, the “Sytue Docs” folder). The service
  not allowing the anchor does not undo what was saved.
- **The vault does not open on its own** (`cifrado.DriveOficina`, on `AplicacionDrive` like Photos):
  it is not touched on startup or when opening a document. When pressing “Save to Drive” without the
  vault's Drive open on the device, its gate is shown (`PuertaBoveda`: the device's Sytue ID key,
  sign in again if it does not open) **above the editor** and the person is told to save again
  (`doc.msg.saveToDriveVaultLocked`); once open, the strip disappears. If `ElegirAlmacenDrive` sends
  the account to Matrix (its Drive has not moved to the vault yet), the office apps do not open the
  Matrix Drive —a `vault:` would point to a file the web does not see— and say to finish from Sytue
  Drive (`doc.msg.saveToDriveVaultNotReady`). With no vault Drive possible on the device,
  `doc.msg.saveToDriveVault` (download and upload from Drive).
- **New messages** in the 26 languages: `doc.msg.savedToDriveEncryptedOnDevice` (“it has been
  encrypted on this device”, not “in this browser”) and `doc.msg.saveToDriveVaultLocked`.
- **Tests** (`guardardrive_test.go`) against a real vault (`drive.Repository` on
  `vaultfiles`/`pqvault` with `pqvaulttest`): new file and anchor, new version of the anchored one
  (`vault:` and `drive:` kept), anchored one in the trash, clashing names, format without round trip,
  export and upload failures with nothing half done, and that the blocks server never sees the
  document; the editor hook and the gate, in `editor` and `cifrado`.

What is **still not done**, and why:

1. **The “home” file when creating the document.** It is still created without an anchor; the
   encrypted file is born on the first “Save to Drive”. Writing it on creation would require opening
   the vault (and perhaps asking to sign in again) on every creation, from all the places there are
   (home page, templates, copies, Drive, native), and even then it would not follow the document:
   the server one does not either, it is only rewritten when saving. Hence only explicit saves.
2. **The signature PDFs.** The copy brought from another organization (`open`) and the signed one
   brought back (`fetch-signed`) are server state: it stores their id and their server Drive path,
   checks the copy through `Metadatos`, checks what was delivered against a Drive link, and CertiPDF
   opens and signs only by server Drive path (`/pdf?path=`). A copy in the vault could not be opened,
   signed or delivered from CertiPDF, and the request would be left pointing to a file the server
   does not see. Meanwhile, the 409 `drive_private_vault` is said in its language
   (`pdf.error.driveVault`: “download the PDF”) instead of a generic failure. The person's OWN
   signature is solved: see “Signing on the device” right below.
3. **Native with the Docs data in Matrix.** With the Docs records already in Matrix there is no
   service to anchor to (`AnclarEnBoveda` gives `ErrPrivateDataOnly`), so the entry only shows with
   the service API. And an account whose Drive was decided to go to Matrix stays that way until the
   app is reopened or the account is switched (the office apps do not decide again on their own).
4. **One opening per save (web).** Each “Save to Drive” opens the vault and closes it when finished;
   it does not stay open in the editor. Natively, once unlocked it stays open while the account
   lives in the app, as in Photos, and one has to press again after opening it.
5. **The store test** of `AnclarEnBoveda` (`store_integration_test.go`) needs PostgreSQL 17
   (`pg_c_utf8`); the HTTP layer test and the ACL matrix cover the rules.

**Signing on the device: CertiPDF with Drive in the vault (2026-11-23).** Owner's decision
(“Signing on the device”): with the person's Drive in the vault, **their own signature** of a PDF is
made on the device; requesting signatures from other people or organizations still needs the server
Drive. Nothing reaches any server in clear text: not the PDF, not the certificate, not its password.

- **How CertiPDF signs today, and why the model does not change.** On the web, the `.p12` (or the
  two PEMs) travels with each signature to the BFF (`pdf-aplicar.ts`, op `firmar-certificado`) and
  the Docs service signs with the `apps/nucleo/pdf` engine (`FirmarIncrementalPDF`, appended PAdES
  B-B and an RFC 3161 timestamp if the deployment has an authority); the certificate is not stored
  anywhere. The vault signing key of [0087](#0087) (Ed25519 + ML-DSA-65) **is no use** for this: a
  PDF is signed with an X.509 certificate that a European validator recognizes, not with a key
  anchored in Sytue ID. So it is signed with the SAME certificate of the person, only in the browser.
- **The browser signer** (`packages/office/src/pdf/dispositivo/`), without new dependencies, like
  the Go one: `pkcs12.ts` opens the `.p12` with the same subset as `platform/pkg/pkcs12` (PBES2 with
  PBKDF2 and AES-CBC through WebCrypto; the classic PBE-SHA1-3DES with the RFC 7292 derivation and a
  hand-written 3DES in `des.ts`, checked against the FIPS 81 vector) and leaves the key as a
  **non-extractable** `CryptoKey`; `pdfObjetos.ts` reads the chain of cross-reference tables
  (classic, stream-based and objects inside streams, with FlateDecode and PNG predictor) and
  `pades.ts` appends the signature (§7.5.6) with the same pieces as `pdfsignincr.go`:
  `/SubFilter /ETSI.CAdES.detached`, content-type, message-digest and signing-certificate-v2
  (without signing-time), SHA-256 with RSA PKCS#1 v1.5 or ECDSA, a 24 KiB placeholder, a free
  “Sytue.FirmaN” field, an invisible signature with its empty appearance, `/SigFlags 3` and a table
  of the same type as the original's. Before returning anything it **verifies** what was signed
  (digest and signature with the certificate's key): a broken signature is not saved.
- **With the vault** (`dispositivo/firmarEnBoveda.ts`, CertiPDF home page): if Drive is in the vault
  (checked without keys, `driveEnBoveda`), the home page shows “Sign on this device”. The vault
  **does not open on its own**: when pressed it opens with the Docs and Photos consumer gate
  (`openConsumerPrivateDrive`: the vault only if it exists, the repository only with its receipt,
  `importar: false`; the office BFF `/api/drive-cifrado/…`), the PDFs are listed (the repository's
  recent ones), and when signing the chosen one is downloaded, decrypted in memory, signed and
  uploaded **encrypted** to its same folder as a NEW file “name (signed).pdf” without overwriting
  any name (“… (signed) (2).pdf”), the way the server leaves a copy (`siempreCopia`). The original is
  not touched and the clear-text bytes are wiped when finished.
- **The Sytue ID bridge** allows **opening** (not creating, emptying or resealing) the `drive` vault
  from `certipdf.` (`VAULT_READERS`), as for Photos, Docus, Calcus and Diapos; its test
  (`sytue-id-pages.test.ts`) fails if it is removed.
- **Requesting signatures** stays on the server Drive: the service has to read the PDF to send it
  (or share it by link with another organization) and check against it the signature that comes
  back. The home page explains it (`pdf.device.requests`) next to signing on the device; the 409
  `drive_private_vault` of outside requests still says `pdf.error.driveVault`.
- **Native.** Native CertiPDF already works on the private Drive (the vault via `vaultfiles.Store`
  and `PuertaBoveda`, or Matrix) and signs with a certificate LOCALLY (`api.PDF.localSign`, the same
  engine as the server, without timestamp); now with its test (`docs_pdf_private_sign_test.go`: it
  appends, verifies and does not talk to the service). What was missing was the other half: the
  “Request signature” dialog of a private PDF no longer tries to call the service (it failed with a
  generic error) and says the same as the web (`pdf.device.requests`); and `drive_private_vault` is
  also translated natively (the error map did not match the web's and its test failed).
- **Messages** in the 26 languages (`pdf.device.*`, office catalog and the regenerated Fyne one).
- **Tests.** `firmaLocal.test.ts`: the two `.p12` of `platform/pkg/pkcs12/testdata`, signature on a
  classic table and on streams, second signature, signature on a pyHanko PDF without breaking its
  own, rejections with a reason, and the real vault (`PrivateDriveRepository`): it lists only PDFs,
  the signed one verifies, the original does not change, names are not overwritten and **nothing in
  clear text** in the store's blocks or records. **Third-party** verifiers:
  `firmado-en-el-dispositivo.pdf` joins the corpus of `pdfverificar_corpus_test.go` (with
  `anclas/empresa-pruebas.crt`) and comes out valid alongside the pyHanko signature it already had;
  Poppler (`pdfsig`) also considers the signatures valid.

What it **did not do** in that first delivery —no timestamp, invisible only and without opening the
PDF in the studio— is solved in the second one, right below. Still not done, and why:

1. **It does not certify (`/DocMDP`)** and does not sign a **password-protected** PDF, one
   **certified with P=1** or one whose cross-reference table cannot be read safely: the server would
   fall back to rewriting the whole file, and that is not done here. It is said with its sentence
   instead of writing something dubious.
2. **Without the system certificate** (electronic ID card, smart card, keychain): from the browser
   there is no interface to sign with it. Native does have it (two-step signing, [0051](#0051)), but
   its two steps go through the server with the PDF and are not offered with the private Drive.

**Signing on the device, second delivery: timestamp, visible signature and the studio
(2026-11-23).**

- **Timestamp (PAdES B-T).** The same model as the server (`pdftimestamp.go`): after signing, the
  SHA-256 of the **signature value** is stamped (the SignerInfo's messageImprint, what in Go is
  `huellaDeFirma`) and the token goes as the **unsigned** attribute
  `id-aa-signatureTimeStampToken` (`dispositivo/sello.ts`, `construirCMS`). The authority (URL,
  credentials, anchors: the Docs service's `SELLO_*`) is only reachable by the server, so the
  browser requests it through the BFF `/api/pdf-sello` (`pages/api/pdf-sello.ts`), which validates
  that the body is **32 bytes** and rebuilds it with that single field, to the Docs service
  (`GET`/`POST /api/v1/pdf/timestamp`, `services/docs/internal/http/pdfsello.go`), which calls
  `PedirSello` with the nonce, checks that the token stamps THAT hash and, with anchors, that it is
  signed by the configured authority, with a per-person cap. **The PDF or its digest never travel**:
  a hash of a signed number says nothing about the document. The browser checks again that the token
  stamps its signature before inserting it (`comprobarSello`), and the local verifier
  (`verificarFirmas`) says whether a timestamp covers its signature. If the deployment **has no
  authority**, the dialog says so BEFORE signing (`pdf.device.stamp.off`) and the signature comes out
  B-B; if it has one, it says whose (`pdf.device.stamp.on`). If the authority does not respond, it
  signs anyway without a timestamp —like the server (`construirCMS` in Go)— and the final notice says
  so (`pdf.device.signedNoStamp`); with a timestamp, `pdf.device.signedStamped`.
- **Visible signature**, optional (the invisible one is still the default). Page, one of the studio's
  four positions (`rectFirma`, with the page's real size and its inherited MediaBox:
  `DocumentoPdf.paginas`) and the drawn handwritten signature, if wanted, with the SAME canvas as the
  studio (`LienzoDeFirma.tsx`, now shared; `trazosAPagina` in `firmaPosicion.ts`). The appearance is
  generated here and is the server's (`objetoAparienciaFirma`): light background, golden border, the
  handwritten signature in dark ink, “Signed by …” and the UTC date in Helvetica WinAnsi (what is
  outside WinAnsi comes out as “?”, as in Go). A page that does not exist or a box that overflows is
  reported (`pdf.device.error.page`).
- **The studio with the vault.** `/pdf?path=` checks without keys whether Drive is in the vault
  (`driveEnBoveda`); if it is, it opens `EstudioBoveda.tsx` instead of the usual one: the vault is
  opened WHEN PRESSED with the same gate (`abrirRepositorioCifrado`: `existing: true`,
  `importar: false`), the file is looked up by its path going down folder by folder
  (`abrirPdfDeBoveda`) and decrypted in memory; it is **viewed** with the browser's PDF viewer on a
  local `blob:` (read-only), **downloaded** and **signed** with the same dialog as the home page,
  which leaves the encrypted signed file next to it and offers to open it. What needs the service to
  read the PDF (organize, tools, protect, request signature…) shows **disabled with its note**
  (`pdf.vault.serverOnly`); a path that does not exist, with its `EmptyState`. The home page list
  links each vault PDF to its studio.
- **Messages** in the 26 languages (`pdf.device.stamp.*`, `pdf.device.appearance*`,
  `pdf.device.signedStamped`/`signedNoStamp`, `pdf.device.error.page`, `pdf.device.view`,
  `pdf.vault.*`; removed `pdf.device.note`), and the regenerated Fyne catalog.
- **Tests.** `selloVisible.test.ts`: the authority only receives ONE 32-byte hash which is the SHA-256
  of the signature value (not of the PDF), the token covers it and `verificarFirmas` says so; a token
  for another hash, of another algorithm, unreadable or an authority that is down leave the signature
  valid and WITHOUT a timestamp; cancelling does not sign behind the person's back; the BFF client
  sends only `{resumen}`; the visible appearance (page, box, handwritten signature, text) and the
  page rejections. `EstudioBoveda.test.tsx`, with the real `PrivateDriveRepository`: with the vault,
  `/pdf?path=` is the vault studio and does not open on its own; the server features show disabled;
  the PDF is viewed from a `blob:`; signing visibly with a timestamp leaves “Minutes (signed).pdf”,
  which verifies and carries the timestamp; the BFF only received `{resumen}` and nobody received the
  PDF; and there is nothing in clear text in the store's blocks and records. In Go,
  `pdfsello_test.go`: without an authority (404 `pdf_timestamp_unconfigured`), round trip against a
  fake authority (it receives a TimeStampReq with the hash and a nonce), only SHA-256 digests (the
  rest, 400 and without calling the authority), 401 without a session and 502 for a token for
  another hash. **Third-party verifiers**: `firmado-en-el-dispositivo-visible-con-sello.pdf` (visible with a
  handwritten signature on page 2 and a timestamp from an `openssl ts -reply` authority, root in
  `anclas/raiz-sello-dispositivo.crt`) joins the corpus of `pdfverificar_corpus_test.go` and comes
  out **valid with a trusted timestamp**; Poppler (`pdfsig`) considers the signature valid. It is
  regenerated by `generar-dispositivo.sh` (openssl and bun, with `firmar-en-el-dispositivo.ts`),
  which `generar.sh` also runs and which rewrites identically the first delivery's
  `firmado-en-el-dispositivo.pdf`.

What this second delivery **does not do**, and why:

1. **The viewer is the browser's**, not the studio's SVG canvas: that one is drawn by the service's
   engine, which cannot see the PDF. Some browser may download it instead of showing it (the
   WebViews that Drive already noted); “Download” is always at hand. Annotating, filling in or
   organizing a vault PDF on the web would need the PDF engine in the browser (Go/Wasm), and it is
   still pending; native does it all locally.
2. **No LTV** (`/DSS` with OCSP/CRL), like the server: the timestamp already makes the signature
   survive the certificate's expiry, which is the main thing.
3. **The handwritten signature stored in the account is not offered** when signing on the device: it
   is drawn at the moment (or not drawn). Reading it from Sytue ID would be possible, but it is not
   mixed here with signing a PDF the server does not see.

<a id="0087"></a>

<a id="0087--sytue-mail-en-la-bóveda-directa-clave-de-firma-en-la-bóveda-anclada-por-sytue-id"></a>

## 0087 — Sytue Mail on the direct vault: signing key in the vault, anchored by Sytue ID

`accepted` · 2026-11-22 · security, e2ee, post-quantum, mail · Phase 2 of [0086](#0086) · builds on 0084 and 0085 · delivery 1 code in `apps/nucleo/recoverywrap/firma_boveda.go`, `apps/nucleo/cmd/pqc-wasm/vault_signing_operations.go`, `services/id/internal/http/e2e/bridge.js`

**Context.** The thirteen apps of [0086](#0086) only store the user's own records: changing the
transport was enough for them. Sytue Mail also **receives** encrypted mail from outside, and for
that the server has to know that an ingress key really belongs to that person. Today it knows
through Matrix, in three places:

- the mail module asks the browser for the cross-signing master key (`identity.root`,
  `packages/offline/src/matrix/client.ts`);
- the ingress key enrollments and the work turns (`key.enroll`, `task.claim`, `task.ack`,
  `task.handoff`) are signed by the Matrix device and the server verifies them against Synapse's
  key directory (`apps/nucleo/storage/ingress/matrixproof`);
- the storage authority (`matrix_storage_authorities`, `services/rooms`) only allows changing root
  if the previous root signs it.

That last rule is the one that leaves whoever lost their key with no way out: there is no way to
empty Mail and start again. The direct vault has nothing equivalent to replace it with: its owner
proof is an HMAC, which the server cannot verify.

**Decision.**

1. **Signing key inside the vault's secret.** `PQVaultSecret` gains `signing_key`: a hybrid
   Ed25519 + ML-DSA-65 pair (`crypto/ed25519`, `crypto/mldsa`), generated in the Go/Wasm core
   together with the `age-MLKEM768-X25519-v1` identity and sealed with it by the Sytue ID bridge. A
   signature is valid only if both halves verify. Existing vaults add it on their next opening; apps
   that do not use it ignore it.
2. **Sytue ID is the anchor.** When creating or emptying a vault, the browser registers the public
   part in Sytue ID (`PUT /e2e/vault-keys/{product}`) with the person's session; Sytue ID stores
   `(sub, tenant, product, generation, public key)` and serves it signed to the services. The mail
   server verifies ingress actions against that registry, not against Synapse.
   - **Enrollment:** the first key of a `(sub, tenant, product)` enters with the session.
   - **Change with continuity:** a new key signed by the previous one enters without more.
   - **Emptying:** a new key **without** the previous one's signature requires recent
     reauthentication (`max_age`, and a second factor if the account has one), raises the
     generation, stays in the audit log and notifies the person by email. It is the way out that
     does not exist today.
3. **Mail on `PqVault.storage()`.** The mail module keeps its ports (store, journal, work, ingress
   secrets, control); what changes is who serves them: `identity.root` returns the public signing
   key and ingress actions are signed with it. The Matrix authority of `mail` stays read-only to
   migrate what the person can still open.
4. **Migration per account, without silent loss.** Whoever opens their Matrix Mail moves it to the
   vault on sign-in (the same historical origin mechanism already used by the move from identity 1
   to 2). Whoever cannot open it chooses: sign in to Sytue ID again or empty with confirmation.

**Deliveries.**

1. **Done.** Core: `signing_key` in `recoverywrap` and `pqc-wasm` (`vault.signing.create`,
   `.public`, `.sign`), shared vectors (`testdata/firma_boveda_pq.json`) and bridge (`bridge.js`
   keeps the key when sealing and opening). No app creates or uses it yet.
2. **Done.** Sytue ID: vault key registry (migration `0123_claves_boveda`, table `e2e_vault_keys`,
   one row per generation and a single active one), enrollment, continuity and emptying rules
   (`services/id/internal/http/claves_boveda.go`), audit `e2e.vault_key.<reason>` and email on
   emptying. The person reads and registers with their access token at
   `GET`/`PUT /api/v1/me/vault-keys/{product}`, not with the browser session as point 2 of the
   decision said: the apps talk to Sytue ID through their BFF. The services read the active one at
   `POST /internal/vault-keys/lookup` (its own secret, `VAULT_KEYS_SECRET`; without it the route is
   not mounted). The statement that is signed is composed by the core
   (`recoverywrap.PQVaultKeyStatement`, Wasm operation `vault.signing.enroll`).
3. **Done.** Mail: verification of ingress actions against the registry
   (`apps/nucleo/storage/ingress/vaultproof`), with the Matrix one in parallel; Rooms branches by
   signature type (`services/rooms/internal/store/claves_boveda.go`).
4. **In part, behind a switch that is off.** Web: Mail opens its storage in the vault
   (`@sytue/offline/vault/mail`, `openVaultMailPrivate`) with the same ports of the mail module:
   store `vault.storage()`; ingress secrets as encrypted `content` records of the vault itself,
   write-once, read by their `transaction_id` (Drive gains that query); action signing in the core
   (`vault.ingress.sign`, `vaultproof.SignIntent`: only the action the module asked for, ingress
   only and with that key's root); `identity.root` = the Ed25519 half. Rooms gives `mail` the vault
   authority when there is **no** Matrix authority and Sytue ID has an active key, and such a scope
   no longer allows enrolling a Matrix root (`autoridad_boveda.go`; only `mail`: calendar also
   registers a key but receives through Matrix). Everything is behind `SYTUE_MAIL_BOVEDA=activado`
   in the Mail BFF; off, nothing changes. On, the gate looks at the authority: the vault's or none
   (new account) → `PqVaultGate` (sign in again or empty with confirmation); Matrix's or when in
   doubt → the Matrix gate.

   **Decision on the migration:** in this delivery mail is **not** migrated from Matrix to the
   vault. The historical origin (`resolveLegacy`, `registerHistoricalStorageReader`) reads from a
   second Matrix SDK and writes to the new Matrix one; taking it to the vault requires that whoever
   opens it can still open Matrix and a per-scope authority change (from Matrix to vault) that does
   not exist today. Whoever opens their Matrix Mail stays on Matrix, untouched. And whoever
   **cannot** open it but has a Matrix authority still has no way out: the authority handover has
   to require the emptying's reauthentication, and Rooms does not see `auth_time` (pending: that
   Sytue ID certifies it in the registered key, e.g. reason `vaciado` later than the Matrix
   authority).

   **Second batch (blockers for turning the switch on).**

   - _ACK receipt:_ done. If the scope's authority is the vault's, Rooms does not look for a Matrix
     event: it asks Drive (`POST /internal/pq-vault/receipt`, secret `ROOMS_INTERNAL_SECRET` which
     they already share) whether the record is in the person's **current** vault and the destination
     is that of their active key (`storage.PQVaultTarget`). When in any doubt, the ACK is rejected
     and the mail stays in the queue.
   - _Destruction guard:_ `guardMailDestruction` does what Matrix's `destructionGuard` does (pause
     and “pending = 0”, otherwise `locked`) and is exposed as `guardDestruction`. Today nobody calls
     it, because the vault does not destroy anything local with mail open: there is no `forget()`
     (the keys come back from Sytue ID) and emptying is only offered if the vault does not open.
   - _Device fence:_ there is none for the vault, **on purpose**, and a handoff with a vault proof
     fails closed (without an active Matrix authority, `verifyMatrixStorageActionTx` rejects it;
     besides, the fences table requires a Matrix authority row). A vault has a single destination
     per key, shared by all its tabs, and a handoff requires different destinations
     (`ingress.Handoff`). The only possible previous destination is the key from before an
     emptying, and with it its ingress secrets were lost: there is nothing to rescue. Or else it is
     a Matrix device after the authority handover (next point), which is fenced by revoking it in
     Matrix.
   - _Identity cutover for a vault scope:_ it was the underlying blocker (the mail module always ran
     `transfer.Cutover`, which without a cutover row is in `legacy` and requires the historical
     origin, and the initial import was done by that origin). Solved in the third batch.

   **Third batch.**

   - _Vault mode of the mail module:_ done. **The host chooses it**, never the module: `mail.open`
     carries `store: "vault"` (`openVaultMailEngine`), `webMail.OpenStore` passes it to the
     constructor and `application.Config.Vault` applies it. In that mode there is no historical
     origin, no identity cutover, no call to `rooms.cutover.*`; a vault configuration with any
     Matrix piece (origin, files, copier) is rejected. The epoch of the server's retained mail is
     fixed with the vault as destination (`mail.CaptureDirectSource`: the same `transfer.BeginSource`
     and the same server Begin, preceded by the enrollment of that epoch's ingress key) and imported
     directly into the vault (`Repository.ImportDirectSource`, the usual importer: encrypted plan
     in the journal before publishing, receipt last). With the server still in `legacy`, a vault
     with data starts nothing. In addition, the chosen store has to be the authority's (Rooms): the
     vault only opens with a vault authority and **the Matrix path no longer opens a vault scope**
     (before it failed anyway, later, when comparing the root). For whoever opens their mail through
     Matrix nothing else changes: same authority, same cutover, same ports.
   - _Authority handover Matrix → vault:_ done, with the second batch's design.
     - Sytue ID: `PUT /api/v1/me/vault-keys/mail` accepts `handover: true`. It always requires
       recent identification (also on an enrollment), records the reason `traspaso`, leaves
       `e2e.vault_key.traspaso` in the audit log and sends the same notice as an emptying. Only
       `mail`, never together with the previous one's approval, and never hanging from the same key
       that was already there without it (the signed statement does not allow a key to succeed
       itself: the handover comes with a new vault). Repeating it returns the one already done.
       Migration `0124_traspaso_boveda`: generation 1 is `alta` or `traspaso`; `alta` is still only
       generation 1; `traspaso` only in `mail`. Checked against PostgreSQL 16 (up, a down that
       refuses with handovers, and each constraint).
     - Rooms: with a Matrix authority, it asks Sytue ID for the active key (which now carries
       `reason` and `created_at`). If it is a `traspaso` one and **later** than the last change of
       the Matrix authority (`updated_at`), it writes it down once in
       `matrix_storage_vault_handovers` (migration `0081`, immutable, with the Matrix generation of
       that moment) and gives the vault authority. From then on the scope belongs to the vault even
       if the key later changes through continuity or emptying; without Sytue ID there is no answer
       any more (it never goes back to Matrix), and the scope's Matrix proofs (ingress actions and
       fences), their receipts and their root changes are rejected. That way all their Matrix
       devices are fenced at once. Before the handover, Sytue ID down or a key that is not a
       handover one leave the Matrix one, as always; and **a vault proof is no longer valid in a
       Matrix scope without a handover** (before, Rooms would have verified it against Sytue ID; it
       was only saved by mail retaining the Matrix root in the challenge).
     - _The generation does not go down:_ the vault's is counted **on top of** the Matrix one there
       was at the handover: authority = base + generation in Sytue ID (Matrix at 5 → the vault
       starts at 6). Rooms applies the same base when giving the authority and when verifying, so
       the generation retained by mail and the clients only goes up. Without a Matrix authority the
       base is 0 and everything stays as it was.
     - Web: with the switch, Mail's Matrix gate offers “Can't open your mail? Empty it and move it to
       the vault” while the Matrix mail is not open, with confirmation (an “I understand it will be
       lost” checkbox). It opens a **new** vault (`PqVault.reset` with `handover`), and if Sytue ID
       asks to sign in again, the mark survives in the tab's `sessionStorage`. As soon as the
       authority is the vault's, the gate chooses the normal vault and forgets the mark: **it never
       empties twice**.
     - Clock skew: Rooms compares the Sytue ID date with that of its own database. A skew of seconds
       only matters if the person rotates their Matrix root at that very instant, and the handover
       also requires the confirmation checkbox.

   **Fourth batch.**

   - _Rotation of the ingress key of an already enrolled epoch:_ done. It was what left whoever
     hands over without receiving: their epoch had a Matrix ingress plan that the vault does not
     have.
     - Server (`platform/pkg/pg`, fragment `private-ingress-rotation-v1`; mail migrations `0040`
       and calendar `0026`): each plan stores the Rooms authority that authorized it
       (`authority_user`, `authority_generation`, immutable). The authorized enrollment
       (`EnrollAuthorizedPrivateIngress`) accepts a **next** generation plan only if it is
       authorized by an active **vault** authority **later** than that of the current plan: another
       identity (from Matrix to the vault, or a plan from before storing its authority) or a higher
       generation (a vault emptying; the generation never goes down). Matrix and calendar never
       rotate; nor is a generation skipped nor is a revoked plan rotated. The previous plan **is
       neither revoked nor deleted**: what was already encrypted for it stays in the queue, and the
       admission encrypts new mail for the highest active plan (as it already did).
     - Client (`ingress.EnrollmentConfig.Rotate`, which the host sets only in vault mode): if the
       SDK answers that it **does not have** the current plan (`keyring.ErrRecoveryRequired`: the
       vault port returned “not there”, not a network failure), it creates the next plan only once
       (a retry recovers the same candidate) and enrolls it; if the server does not authorize it, it
       fails closed and nothing changes.
     - The vault worker only claims encrypted deliveries for **its** key (`ingress.Config.KeyID`):
       those for the previous key stay in the queue, intact, instead of being claimed and blocked
       here. The Matrix path carries neither `Rotate` nor `KeyID`.
     - What is lost, and the confirmation says so: the mail that was already encrypted for the
       Matrix key. Nobody can open it (its devices were fenced), but it is not deleted.
   - _Latency of the handover query:_ solved. Before the handover, the question to Sytue ID has a
     400 ms deadline and, if it fails or exceeds it, for 30 s Matrix is answered without asking. The
     **outage** is remembered, never a “there is no handover”: with Sytue ID healthy it is asked on
     every read, so a freshly registered handover is seen on the next one (the vault gate reads the
     authority right after registering it; a cache of the “no” would have left it out). After the
     handover there is no shortcut: without Sytue ID there is no authority.

   **The switch is still off.** There is no known blocker in the code any more; it remains to be
   tested end to end, which could not be done here (there is no Synapse nor the stack, and the
   integration tests ran on a local PostgreSQL 16, not the 18 used in CI). Checklist, in an
   ephemeral environment with Sytue ID, Rooms, Drive and Mail (`ID_VAULT_KEYS_SECRET` /
   `VAULT_KEYS_SECRET`, `ROOMS_INTERNAL_SECRET`), the migrations `0124` of Sytue ID, `0081` of
   Rooms, `0040` of mail and `0026` of calendar, the Mail BFF with `SYTUE_MAIL_BOVEDA=activado` and
   the real `mail-wasm`/`pqc-wasm` in the browser:

   1. _New account._ Opening Mail chooses the vault; the server moves to `frozen` with the epoch
      `InitialSourceEpoch`; ingress plan 1 stores the vault authority; the mailbox imported from the
      server appears; an incoming SMTP mail arrives, is claimed, stored in the vault and its ACK is
      accepted by Drive (vault receipt); sending one goes out.
   2. _Matrix account that does open._ With the switch on it stays on Matrix, without offering the
      handover with mail open; it receives and sends the same; its ingress plan does not rotate.
   3. _Handover._ Account with Matrix authority at 5 (with some previous rotation) and an enrolled
      epoch that does not open its mail: confirm “empty and move”; sign in to Sytue ID again if
      asked; check the email notice and the `e2e.vault_key.traspaso` audit; the vault authority at
      6; an action signed by the Matrix device rejected; ingress plan 2 enrolled with the vault
      authority; new mail arriving in the vault; deliveries of plan 1 still in the queue unclaimed;
      the mailbox imported again from the server's retained epoch.
   4. _Same browser that already used Matrix Mail._ The tab's encrypted journal is the same for both
      paths: check that half-done Matrix intents (an unfinished ingress enrollment, claimed
      deliveries) do not prevent opening the vault; today a half-done enrollment with another
      destination fails **closed** (`ErrRevoked`), and then it would have to be withdrawn on
      handover.
   5. _Matrix scheduled sends._ The sends that the Matrix client left registered (controlled by its
      commitment) are not controlled by the vault: check what happens to them and, if needed, cancel
      them on handover.
   6. _Emptying the mail vault._ After an emptying (new key, higher generation), the ingress plan
      rotates again and mail arrives again.
   7. _Sytue ID slow or down._ Before the handover, Matrix Mail opens without waiting more than
      ~400 ms once and then immediately for 30 s; after the handover, the vault does not open.
   8. _Unauthorized rotation._ A second rotation with the same authority, or one requested with a
      Matrix authority, is rejected and the current plan does not change.

5. Withdrawal of the Matrix authority of `mail` and, afterwards, native clients.

**Consequences.**

- Mail opens with the same key as the rest of the suite and gains emptying; Synapse remains only
  for Rooms ([0077](#0077)).
- Sytue ID becomes a trust anchor for incoming mail: whoever controls Sytue ID **and** the person's
  session can register a new key. It is bounded by reauthentication, the generation visible to the
  person and the notice; it is not eliminated. The current authority did prevent it, at the price
  of having no way out.
- An emptying does not decrypt anything earlier: mail encrypted with the lost key stays lost.
- With the key registry configured, each read of the `mail` authority of a Matrix account also asks
  Sytue ID (to see a handover). If Sytue ID does not answer, the Matrix one, as before. A slow answer
  adds at most 400 ms to a read, and after an outage it stops asking for 30 s (fourth batch); a
  handover registered during that pause waits for it to pass.

**Rejected alternative.** Trusting the first key presented, without a registry in Sytue ID: fewer
pieces, but after an emptying nobody could tell the person's key from one put there by whoever
controls the server.
