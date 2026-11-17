<a id="sytue-one--contrato-canónico-single-source-of-truth--v4-sytue-native"></a>

# Sytue One — Canonical contract (single source of truth) · v4 "Sytue Native"

> Source of truth for the whole monorepo. Every name, domain, port, version,
> license, stack or path **must** match what is defined here. Parent brand: **Sytue**.
> Suite: **Sytue One**.

<a id="0-principios-nucleares-en-orden-de-prioridad"></a>

<a id="0-principios-nucleares-en-orden-de-prioridad"></a>

## 0. Core principles (in order of priority)

1. **SINGLE and TOTAL stack: `Go + Astro + Preact + PostgreSQL`.** Every **backend** is written in
   **Go**; every **web surface** in **Astro + Preact** (islands); the **main datastore and
   source of truth** is **PostgreSQL** (state, queues and cache on Postgres by default). The
   **versions** are not quoted here: they live in the table in
   [§3](#3-versiones-y-stack--politica-ultima-estable--cadena-de-suministro), which is their source of
   truth, and the policy is **always the latest stable**. A number repeated in prose ages without
   anyone noticing; this one said “Astro” when the table was already at 7. Large
   blobs (files, attachments, documents) go in **S3 Object Storage** (Scaleway in prod, MinIO in dev).
   - **Valkey (BSD-3-Clause) — OPTIONAL cache/queue allowed.** Where it brings a **measurable
     performance improvement** to the apps (e.g. Chat/presence fan-out, sessions, rate-limiting,
     high-volume queues), **Valkey is allowed** as an acceleration layer. It is not mandatory and
     does **not** replace PostgreSQL as the source of truth; it is introduced only with a performance
     justification. **Never Redis** (non-OSI license). No backend language exceptions except §0.5.
2. **Sytue-native, original products.** **No third-party AGPL
   engine** is deployed (Nextcloud, OnlyOffice, Stalwart, Mattermost, Keycloak are **out**). Each
   product is an **original Sytue work** (a **clean-room** reimplementation on **open
   standards**), licensed **Apache-2.0** (© 2026 Daniel Delgado Ramos (thinkddr)).
   **Express exception by the owner (2026-10-16, [ADR 0077](decisions.md#0077)):**
   Rooms may operate unmodified Synapse as a separate Matrix transport (AGPL),
   and use mautrix-go with its dependency go.mau.fi/util (MPL-2.0) in the Fyne clients. The language and license exception
   is limited to those E2E pieces and does not authorize other engines or copying their code
   into the proprietary product. Message keys remain on the clients.
3. **Design first — and across the WHOLE set.** Modern visual language (sidebar + topbar +
   content) with the Sytue brand. A single aesthetic/component library (`@sytue/ui` +
   `@sytue/shell`) for all surfaces. No ad-hoc UI.
4. **The IdP is the CORE piece.** _Sytue ID_ is **our own Go service** (OIDC/OAuth2/SAML/SCIM);
   identity, SSO, authorization, groups/tenancy, provisioning and presence go through it.
5. **100 % EU sovereignty by origin and operation.** Only Scaleway EU regions and, whenever a
   viable alternative exists, products/providers originating in and ultimately controlled by the EU;
   hosting in Europe does not change the origin. GitHub is the only approved general temporary exception,
   limited to repositories, Actions and Releases during the migration P0 in `roadmap.md`. The
   one-off Bun/Kaniko exception for the internal release `v0.1.6` is closed by scope and
   date in [ADR 0027](./decisions.md#0027); it does not authorize new dependencies or a later release.
6. **Legal compliance (clean-room).** See §12: original reimplementation on open
   standards and **permissive Go libraries** (MIT/BSD/Apache); copying/porting AGPL code is forbidden.
   The third-party application exceptions are **LiveKit** (Apache-2.0, Go), as the
   **WebRTC media SFU**, and the separate Matrix transport approved in §0.2.
7. **Security and privacy by DEFAULT, by DESIGN and HARDENED — across ALL the code (a cross-cutting
   principle that governs §1–§6).** It is a _Definition of Done_ criterion for every PR. Details in
   [`docs/security.md#1-seguridad-y-rgpd`](./security.md#1-seguridad-y-rgpd). Non-negotiable minimums:
   - **Privacy by default (GDPR art. 25):** data minimization (only the necessary data),
     most protective configuration by default, limited purpose and retention with automatic
     deletion, pseudonymization/encryption, native data subject rights (DSR), **no telemetry
     without consent**, data **only in the EU**.
   - **Security by default:** **deny-by-default** authorization and least privilege; multi-tenant
     isolation with **RLS** in PostgreSQL; **TLS 1.3 + HSTS**; `HttpOnly`/`Secure`/`SameSite` cookies;
     strict **CSP** and security headers; **CSRF** protection; input validation/sanitization and
     **parameterized queries** (no dynamic SQL); **rate-limiting**/anti-brute-force;
     **MFA/WebAuthn**; secrets only in a secrets manager (**never** in code/logs); encryption at
     rest and of sensitive fields; immutable **audit trail**; logs without personal data.
   - **Hardened (defense in depth):** threat modeling per service; **OWASP ASVS** as the
     yardstick; **SAST/DAST** + dependency/secret scanning in CI; **SBOM** and artifact signing;
     **rootless/distroless** containers and a restrictive `securityContext`; **deny-by-default
     NetworkPolicies**; **mandatory security audit of our own IdP** before production.
8. **Performance as a TOP PRIORITY in every component; and SEO on the public surfaces.**
   A _Definition of Done_ criterion, with budgets that **break the build** in CI.
   - **Performance by default:** latency SLOs/budgets per service (target p95 API
     < 200 ms, low TTFB); efficient **Go** (pprof, minimal allocations, concurrency, pgx pool);
     optimized **PostgreSQL** (indexes, `EXPLAIN`, _keyset_ pagination, no N+1, materialized
     views); caching where it **measurably** improves things (Postgres or optional **Valkey** §0.1); HTTP/2-3,
     compression (br/gzip), keep-alive; **Astro frontend with islands and minimal JS**, code-splitting,
     lazy loading, AVIF/WebP images, edge/CDN in the EU; green **Core Web Vitals** (LCP/INP/CLS);
     bundle size budget.
   - **SEO (public surfaces: `www`, landing, shareable public content):** **SSR/SSG** with
     Astro, semantic HTML, metadata + **Open Graph/Twitter Cards**, **JSON-LD structured data
     (schema.org)**, `sitemap.xml` + `robots.txt`, canonical URLs, **hreflang** (i18n), CWV as a
     ranking factor, WCAG AA accessibility. **Authenticated** surfaces carry `noindex`
     (they are not indexed); SEO is concentrated on what is public.
   - **Continuous measurement:** Lighthouse/CWV and load testing (**k6**) in CI with budgets; continuous profiling.

<a id="1-marca-y-catalogo-de-productos-todos-sytue-native-stack-go--astropreact--postgresql"></a>

<a id="1-marca-y-catálogo-de-productos-todos-sytue-native-stack-go--astropreact--postgresql"></a>

## 1. Brand and product catalog (all Sytue-native, stack `Go + Astro+Preact + PostgreSQL`)

| Product                      | Description                           | Backend (Go) — standard/protocol                                                                                                                | Frontend                         | Subdomain                                               |
| ---------------------------- | ------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------- | ------------------------------------------------------- |
| **Sytue One**                | Portal/launcher                       | (uses Sytue ID + Hub)                                                                                                                           | Astro+Preact                     | `one.sytue.eu`                                          |
| **Sytue ID**                 | Identity/SSO/MFA/SCIM                 | Go service: **OIDC/OAuth2/SAML/SCIM** (Apache/MIT libs)                                                                                         | Astro+Preact                     | `id.sytue.eu`                                           |
| **Sytue Drive**              | Files/sync                            | Go service: **WebDAV** + our own REST API; PG + S3                                                                                              | Astro+Preact                     | `drive.sytue.eu`                                        |
| **Sytue Docs/Sheets/Slides** | Collaborative office suite            | Go service: document model + **CRDT/OT collab** (WS); PG + S3                                                                                   | Astro+Preact (our own editor)    | `docs.sytue.eu`                                         |
| **Sytue Mail**               | Mail                                  | Go server: **SMTP/IMAP/JMAP**; PG + S3 (blobs)                                                                                                  | Astro+Preact                     | `mail.sytue.eu`, `mx.sytue.eu`                          |
| **Sytue Calendar**           | Calendar                              | Go service: **CalDAV** + REST API; PG                                                                                                           | Astro+Preact                     | `calendar.sytue.eu`                                     |
| **Sytue Contacts**           | Contacts                              | Go service: **CardDAV** + REST API; PG                                                                                                          | Astro+Preact                     | `contacts.sytue.eu`                                     |
| **Sytue Notepads**           | Notes and notebooks                   | Go service: our own model + REST API; client-side E2E encryption; PG                                                                            | Astro+Preact                     | `notes.sytue.eu`                                        |
| **Sytue Rooms**              | Messaging and video calls             | ONE Go service: **WebSocket** + REST API + rooms/signaling; **LiveKit** (media SFU, Apache-2.0) only transports; PG                             | Astro+Preact (+ LiveKit web SDK) | `rooms.sytue.eu` (SFU at `rooms-sfu.`)                  |
| **Sytue Cloud**              | PaaS (Vercel × Supabase × GCloud)     | Go service: control plane (deploy/build/services) + PostgREST; PG + S3                                                                          | Astro+Preact                     | `cloud.sytue.eu`                                        |
| **Sytue Admin**              | Admin / tenants                       | Go service: admin API (on top of Sytue ID + Hub)                                                                                                | Astro+Preact                     | `admin.sytue.eu` (formerly `console.*`, now `301`)      |
| **Sytue Hub**                | Integration (shell backend)           | Go service: search/notifications/presence/intents; PG                                                                                           | —                                | `hub.sytue.eu`                                          |
| **Sytue Analytics**          | Web analytics for the customer's apps | Two Go services: collector (public, no auth) + read API; PG. **No cookies**: HMAC with a destroyed daily salt ([ADR 0026](./decisions.md#0026)) | In `cloud-dashboard`             | **First party**: `/_sytue` under each customer's domain |
| Public website               | Marketing                             | —                                                                                                                                               | Astro+Preact                     | `sytue.eu`,`www.`                                       |

> The canonical address of each product is its subdomain (the “Subdomain” column):
> links between apps, notifications, native apps and login go through it
> (ADR 0055). `<slug>.sytue.eu` keeps serving the suite during the transition.

> **License:** all Sytue product code is **Apache-2.0 (© 2026 Daniel Delgado Ramos (thinkddr))**. The incorporated Go/JS
> **libraries** are **permissive** (MIT/BSD/Apache-2.0) — compatible with the product's
> Apache-2.0, keeping their notices (NOTICE). The exceptions are **LiveKit** (media SFU) and
> **unmodified Synapse**, separate from the proprietary product (§0.2). The two
> MPL modules of the Matrix client keep their sources and notices. No AGPL code is copied into the product.
> Standalone internal tooling has a separate license perimeter (§12): it is not part of the
> product, nor is it exempt from the origin sovereignty policy.

<a id="dominios-de-ejecucion-del-paas-sytue-cloud"></a>

<a id="dominios-de-ejecución-del-paas-sytue-cloud"></a>

### PaaS runtime domains (Sytue Cloud)

- Customers' prod apps: `https://<app>.apps.sytue.eu` · Previews: `https://<proyecto>-pr-<n>.preview.sytue.eu`
- Custom domains: CNAME → `ingress.sytue.eu` + automatic TLS.

<a id="2-dominio-e-infraestructura"></a>

<a id="2-dominio-e-infraestructura"></a>

## 2. Domain and infrastructure

- **Root domain:** `sytue.eu`. **Cloud:** Scaleway (EU only). **Region:** `fr-par` (DR `nl-ams`).
- **Object Storage (S3):** `fr-par`, versioned buckets (drive-blobs, mail-blobs, docs-blobs,
  backups, PaaS build-cache, tfstate). Managed **PostgreSQL** (Scaleway RDB) for all state.
- Resource names: `sytue-<entorno>-<servicio>`. k8s labels: `app.kubernetes.io/part-of=sytue-one`,
  `sytue.eu/env`, `sytue.eu/tenant`, `sytue.eu/stack=go`.

<a id="3-versiones-y-stack--politica-ultima-estable--cadena-de-suministro"></a>

<a id="3-versiones-y-stack--política-última-estable--cadena-de-suministro"></a>

## 3. Versions and stack — "latest stable" POLICY + supply chain

**Policy (mandatory):** we **always use the latest STABLE (GA) version** of every language,
library, component and dependency, for maximum maturity and minimum exposure to CVEs. Versions are **pinned
exactly** (go.mod/go.sum, `bun.lock`, image tags by digest, `terraform.lock.hcl`) for
reproducibility, and **updated continuously and automatically** via **Renovate/Dependabot**.
**Forbidden:** EOL, RC/beta/nightly versions, or versions without security patches. Where the project offers
an **LTS**, the **current LTS** is used (not "Current") to balance novelty and stability.
**Vulnerability-free verification in CI (a gate that breaks the build):** `govulncheck` (Go),
`osv-scanner` (JS), `trivy`/`grype` (images), **CycloneDX/SPDX SBOM** per artifact.

Current versions (audit 2026-11-02; Renovate bumps them when new stable versions come out):

| Layer                                          | Technology                                                                                 | Version (latest stable / floor)                                                         |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------- |
| **Backend**                                    | **Go**                                                                                     | **1.27.4**                                                                              |
| Go Router/HTTP                                 | `net/http` + `chi` (or stdlib)                                                             | latest stable                                                                           |
| Data access                                    | `pgx` + `sqlc`; migrations `goose`/`golang-migrate`                                        | latest stable                                                                           |
| IdP (libs)                                     | `ory/fosite` or `zitadel/oidc` (Apache-2.0) for correct OIDC/OAuth2                        | latest stable                                                                           |
| WebDAV/CalDAV/CardDAV                          | `golang.org/x/net/webdav`, `emersion/go-webdav`, `go-ical`, `go-vcard` (BSD/MIT)           | latest stable                                                                           |
| Mail                                           | `emersion/go-smtp`, `go-imap`, `go-message` (MIT) + our own JMAP                           | latest stable                                                                           |
| Realtime                                       | `coder/websocket` (MIT)                                                                    | latest stable                                                                           |
| **Frontend**                                   | **Astro / Preact**                                                                         | **Astro 7.4.2 / Preact 10.29.8** (Preact 11 is GA; the compatibility policy keeps 10.x) |
| Styles                                         | Tailwind CSS                                                                               | **4.x** (latest)                                                                        |
| FE runtime / build / bundler / package manager | **Bun** (MIT) — runtime, `bun install`, workspaces and bundler; replaces Node.js + pnpm    | **1.4.4**                                                                               |
| Native interface                               | Fyne, with the suite's patches pinned in `apps/fyne/go.mod`                                | **2.8.2** (fork **v2.8.2-sytue.8**)                                                     |
| **Datastore**                                  | **PostgreSQL**                                                                             | **18.x**                                                                                |
| Queues/cache                                   | **PostgreSQL** by default (`riverqueue`/`pgmq`/LISTEN-NOTIFY); optional **Valkey** (§0.1)  | latest stable                                                                           |
| Blobs                                          | S3 Object Storage (Scaleway) / MinIO (dev)                                                 | latest stable                                                                           |
| WebRTC media                                   | **LiveKit** (Apache-2.0, Go) — _Sytue Rooms_                                               | latest stable                                                                           |
| Rooms encrypted transport                      | Unmodified Synapse (AGPL), independent server; exception §0.2                              | **1.163.0**, image pinned by digest in `deploy/helm/charts/matrix/values.yaml`          |
| Matrix web                                     | matrix-js-sdk (Apache-2.0), official Rust/WASM cryptography                                | **45.0.0 / 18.9.0**; minimum lockfile maturation window                                 |
| Native Matrix and public signatures            | mautrix-go and go.mau.fi/util (MPL-2.0); GoOlm on clients, public signatures on the server | **0.32.0 / 0.11.0**, limited exception §0.2                                             |
| Encrypted external input for devices           | age (BSD-3-Clause), maintained format; transitive hpke (BSD-3-Clause)                      | **1.3.2 / 0.4.0**; server only with public recipients, private keys on clients          |
| Web file integrity                             | hash-wasm (MIT), local incremental SHA-256; does not encrypt or use remote services        | **4.12.0**                                                                              |
| Ingress/TLS                                    | Traefik 3 (Go/MIT) + cert-manager (Go/Apache)                                              | latest stable                                                                           |
| Orchestration                                  | Kubernetes/Kapsule · Helm · Kaniko                                                         | Kapsule: latest GA supported by Scaleway; Helm **4.3.1**                                |
| IaC                                            | OpenTofu (MPL-2.0, preferred over BUSL Terraform) · provider scaleway/scaleway             | latest stable                                                                           |

> **Removed from the stack:** Keycloak, Nextcloud, OnlyOffice, Mattermost, Stalwart (third-party AGPL
> engines) and **Redis** (non-OSI license). Their functionality is provided by **Sytue-native Go services** +
> **PostgreSQL**. **Valkey** (BSD) allowed as an **optional** cache/queue for performance (§0.1).
> **Note:** "latest stable" = GA, never beta/RC; it prioritizes **maturity + absence of CVEs**.

<!-- The explicit anchor keeps the old link working even though the title no longer carries versions. -->

<a id="4-frontend-astro-7--preact-1029-ultima-estable--repuntado-a-los-servicios-go"></a>
<a id="4-frontend"></a>

<a id="4-frontend-astro--preact-última-estable--versiones-en-3--repuntado-a-los-servicios-go"></a>

## 4. Frontend (Astro + Preact, latest stable — versions in §3) — repointed to the Go services

- **Bun workspaces** monorepo in `apps/frontend/` (runtime/install/bundler with **Bun**, not Node/pnpm).
  Astro runs on Bun; the BFFs (Astro endpoints) are served with Bun. Tailwind v4 + `@sytue/ui` tokens.
- **Packages:** `@sytue/ui` (design system), `@sytue/shell` (topbar + app switcher + search +
  notifications + presence + account, mounted in every app), `@sytue/auth` (OIDC+PKCE against
  **Sytue ID** + BFF), `@sytue/api-clients` (**SDKs for our own Go services**: id, drive, docs,
  mail, calendar, contacts, notes, rooms, hub, cloud; + LiveKit web SDK for video calls), `@sytue/config`.
- **Apps:** `portal`, `console`, `cloud-dashboard`, `mail`, `drive`, `calendar`, `contacts`,
  `docs`, `notes`, `rooms`, `www`. All of them mount `@sytue/shell` and use `@sytue/ui`.
- **SSO without domain cookies:** each BFF issues a **host-only `__Host-`** httpOnly cookie
  after OIDC+PKCE against Sytue ID. Switching apps is transparent because the IdP keeps its own
  host-only session. Never use `Domain=.sytue.eu`: Cloud hosts untrusted code under that apex.
  No secrets on the client. Light/dark mode + WCAG AA.
- **The client's first render must produce the SAME tree as the server's.** Preact hydrates
  by matching on **position**: if the client paints one child too many or too few, it reuses the
  wrong element and **keeps the SSR attributes** on it. It does not warn in the console, TypeScript
  does not see it and no text assertion catches it — the node exists and even has the right
  label, just inside someone else's box. The topbar's “offline” notice left two
  defects in production at once: an **empty gray box** with border and background for everyone who
  was connected, and —when actually offline— the notice painted **inside the 1px
  separator** and with `aria-hidden`, invisible and silent exactly when it needs to be read.
  Rule of thumb: **everything that depends on the browser starts at the neutral value and is corrected in a
  `useEffect`**, never in the `useState` initializer. Watch out for `navigator`: Node ≥21 defines it
  as a global **without `onLine`**, so `!navigator.onLine` is `true` on the server and the SSR comes out
  believing nobody has a network. Check `onLine === false`, not falsiness.
  Gate: `test/e2e/tests/topbar-hidratacion.spec.ts` measures the boxes in the DOM.

<a id="41-la-marca-de-la-barra-superior-es-la-de-cada-organización"></a>

### 4.1 The top bar brand is each organization's

An organization can upload its logo and it replaces ours in the bar of the
eleven surfaces. Organizations that upload none see ours with **the wedge tinted** in a
color derived from their slug (twelve `--sytue-orgmark-*` tokens, all measured above
3:1 against the badge and against the monogram). Deterministic, never random: a
color that changes between reloads or between apps reads as a bug. Only the
wedge changes; the badge and the monogram are not touched, because what is customized is the accent.

- **Reframing is done by the backend** (`platform/pkg/logofit`, stdlib only). It works out
  which color the margin is and each side trims its own, which yields, without
  programming them separately, the cases “all four edges”, “horizontal only”,
  “vertical only”, “just one” and “none”. Transparency wins when there is any
  —an alpha of zero is the designer saying where there is NO logo— and the count of
  perimeter colors is only the fallback for JPEG, which has no alpha. Two
  guardrails: if no color dominates the edge nothing is cropped (gradients,
  logos over a photo) and never more than 25 % of one side (brands designed as a plate).
- **The byte served is never the byte uploaded.** It is always decoded and re-encoded
  to PNG, even if the input was already perfect: that is what
  allows serving it on a public route without trusting anyone's file. The
  original is not stored. SVG is NOT accepted: it cannot be reframed without rasterizing and
  it is the format with the largest attack surface; the error message explains that it
  must be exported to PNG.
- **Served at scale without hitting the database per paint.** The content hash travels in
  the `org_logo` claim of the ID token; the bar composes `?v=<hash>` on
  `id.sytue.eu/branding/<slug>/logo.png` and the response is `immutable` for a year. When
  the logo changes the hash changes, and with it the URL. No logo: 404 with a short cache.
- The binary lives in `bytea` (`org_branding`, migration 0051) and **not** in
  `platform/pkg/objectstore`, which is the house rule: `services/id` does not
  know it, and wiring all of S3 into the service that guards the JWKS to move 30 KB
  is not worth it. The day ID stores something truly large, that is the moment.

<a id="5-lenguaje-de-diseno--sobrio-y-atractivo-estilo-mattermost--sytue-con-mucho-carino"></a>

<a id="5-lenguaje-de-diseño--sobrio-y-atractivo-estilo-mattermost--sytue-con-mucho-cariño"></a>

## 5. Design language — sober and attractive, Mattermost × Sytue style (with great care)

The graphical interface is a first-order product value: a **SOBER and ATTRACTIVE style like
Mattermost**, polished in every detail. Left sidebar (dark/neutral) + topbar (global shell) +
light content; **full dark mode**; comfortable density of a professional tool; restrained palette
with one accent; typography with a clear hierarchy; soft borders/radii and subtle shadows; **visible
hover/focus/active states** and an accessible focus ring; polished **empty states** (icon + text +
action); discreet CSS microinteractions (150–200 ms). Sytue brand tokens (primary/accents/
neutrals/semantic, 4px scale) — **no loose HEX values**. Canonical components exist once in
`@sytue/ui`; apps do not create variants. Cross-cutting consistency: the whole suite looks and behaves
like **a single application**. WCAG AA. Guide in `docs/design.md#2-sistema-de-diseno` + `branding/`.

Buttons are rectangular, with **4 px** corners (`rounded-control`), in both
forms of the suite, including `www`, account sign-in and the native apps. Icon-only
buttons keep their square proportion and the same radius.

<a id="6-puertos-dev--docker-compose--dominio-dev-sytuelocalhost"></a>

<a id="6-puertos-dev--docker-compose--dominio-dev-sytuelocalhost"></a>

## 6. Ports (dev / docker-compose) — dev domain `*.sytue.localhost`

| Go service (backend)              | Port           |     | Astro frontend (dev) | Port |
| --------------------------------- | -------------- | --- | -------------------- | ---- |
| Sytue ID                          | 8081           |     | portal               | 4321 |
| Sytue Drive                       | 8082 (+WebDAV) |     | console              | 4322 |
| Sytue Docs (+collab WS)           | 8083           |     | cloud-dashboard      | 4323 |
| Sytue Rooms (+WS)                 | 8085           |     | www                  | 4324 |
| Sytue Mail (HTTP/JMAP)            | 8086           |     | mail                 | 4325 |
| Sytue Calendar (+CalDAV)          | 8087           |     | drive                | 4326 |
| Sytue Contacts (+CardDAV)         | 8088           |     | calendar             | 4327 |
| Sytue Notepads                    | 8089           |     | contacts             | 4328 |
| Sytue Cloud API                   | 8090           |     | (free, was docs)     | 4329 |
| Sytue Cloud builder               | 8091           |     | rooms                | 4330 |
| Sytue Hub                         | 8092           |     | notes                | 4331 |
| Sytue Integration Gateway         | 8093           |     |                      |      |
| Sytue Analytics collector         | 8094           |     |                      |      |
| Sytue Analytics API               | 8095           |     |                      |      |
| Sytue OCR (internal, no ingress)  | 8096           |     |                      |      |
| Sytue Cloud controller (internal) | 8103           |     |                      |      |
| Sytue Cast (+chat WS)             | 8099           |     |                      |      |
| Sytue Bases                       | 8102           |     | bases                | 4337 |
|                                   |                |     | docus                | 4338 |
|                                   |                |     | calcus               | 4339 |
|                                   |                |     | diapos               | 4340 |
|                                   |                |     | certipdf             | 4341 |

| Infra                                  | Port                                  |
| -------------------------------------- | ------------------------------------- |
| Traefik HTTP/HTTPS/dashboard           | 80 / 443 / 8080                       |
| PostgreSQL                             | 5432                                  |
| MinIO Object Storage (dev) API/console | 9000 / 9001                           |
| LiveKit (Meet, WebRTC)                 | 7880 (WS) / 7881 (TCP) / 7882 UDP mux |
| Sytue Mail SMTP/IMAP                   | 25 / 465 / 587 / 993                  |

> **No** Redis/Valkey ports or AGPL engine ports (Keycloak/Nextcloud/OnlyOffice/Mattermost/Stalwart).

<a id="7-layout-del-monorepo"></a>

<a id="7-layout-del-monorepo"></a>

## 7. Monorepo layout

```
sytueOne/
├── docs/            CONVENTIONS.md architecture.md decisions.md security.md
│                    operations.md design.md roadmap.md business.md README.md
├── platform/        # Go: módulo único y librerías compartidas — catálogo en §13.2
│   ├── go.mod (module git.sytue.eu/sytue/one)   go.sum
│   └── pkg/{asistente,auditlog,authn,cache,cms,commerce,config,events,httpx,ingressroutes,
│            keycrypto,log,logofit,migcheck,miniatura,objectstore,pdfdoc,pdffuente,pdfsimple,personas,pg,
│            pgtest,ssrfhttp,telemetry,tenancy,validation}
├── services/        # Go: un servicio por backend de producto
│   ├── ai/ analytics/ calendar/ cast/ cloud/ contacts/ docs/ drive/
│   │   hub/ id/ integrations/ mail/ notes/ ocr/ rooms/ syerp/ syphotos/
│   │   └── cada uno: cmd/<svc>/main.go · internal/{http,store,domain,...} · db/migrations · openapi.yaml · Dockerfile
├── apps/frontend/   # Astro + Preact (repuntado a services/*)
│   ├── apps/{account,calendar,cloud-dashboard,console,contacts,docs,drive,erp,
│   │         mail,notes,photos,portal,rooms,www}
│   └── packages/{ai,api-clients,auth,cloud,config,escena,formula,i18n,offline,scripts,shell,ui}
├── apps/fyne/       # Interfaz nativa (escritorio y móvil); su empaquetado, en scripts/
├── apps/nucleo/     # Núcleo Go de los clientes nativos, sin toolkit
├── apps/mobile/     # Capacitor (Rooms; la suite ya no publica)
├── deploy/          compose/ (dev local)   helm/ (prod Kapsule)   helm/ (piloto)
├── infra/terraform/ modules/{network,kapsule,database,object-storage,registry,dns,secrets,loadbalancer,livekit}
│                    environments/{dev,staging,prod}
├── branding/        logo/ icons/ tokens/ brand-guidelines.md   (sin themes de motores de terceros)
└── scripts/
```

> This list goes stale by itself: it is generated by the folders, not by an agreement. If it does not
> match when you look at the repo, the repo wins — and update it here, which is where
> newcomers look for it.

> **Removed** (AGPL engines / TS): `identity/keycloak/`, `apps/hub/` (TS), `apps/paas/` (TS),
> `branding/themes/{keycloak,nextcloud,mattermost,onlyoffice}`. Replaced by `services/*` (Go).

<a id="8-identidad-y-sso-sytue-id--servicio-go-nuclear"></a>

<a id="8-identidad-y-sso-sytue-id--servicio-go-nuclear"></a>

## 8. Identity and SSO (Sytue ID — core Go service)

- **Sytue ID** is our own **Go** service implementing **OIDC/OAuth2** (Authorization Code +
  PKCE, refresh, JWKS, discovery), **SAML** (optional), **SCIM** (provisioning) and **MFA/WebAuthn**,
  on Apache-2.0 libraries (`ory/fosite`/`zitadel/oidc`). Data in **PostgreSQL**.
- **OIDC clients (confidential BFF per surface):** `sytue-one`, `sytue-console`, `sytue-cloud`
  (+ `sytue-cloud-api`), `sytue-mail`, `sytue-drive`, `sytue-calendar`, `sytue-contacts`, `sytue-docs`,
  `sytue-notes`, `sytue-rooms`, `sytue-hub`. There are **no** engine federation clients (there are no engines anymore).
- **Every Go service validates Sytue ID tokens** (JWKS) and applies role/tenant-based authorization.
- **LiveKit (video calls):** the `sytue-rooms` BFF mints the **LiveKit JWT** (API key/secret) after validating OIDC.
- **Groups/roles:** `org/<tenant>`; roles `owner`, `admin`, `member`, `billing`. MFA/WebAuthn
  is mandatory for `admin`/`owner` when administering an organization's identity, and for
  platform administration. Signing in to the apps or to “My account” does not force MFA enrollment
  just for holding those roles: MFA enrollment is voluntary, unless there is an explicit organization
  policy. If the person has already enabled it, login requires their second factor. One sign-up in
  Sytue ID → access to the whole suite (tenancy in §9).

<a id="9-multi-tenant"></a>

## 9. Multi-tenant

**The ACTIVE organization of a session** is the one in the `tenant` claim of the access token, and
ALL of the suite's isolation derives from it. It is chosen when the session is opened (`createSession`, which
checks membership + status + right of use) and changed with
`PUT /api/v1/me/active-organization` (`services/id/internal/http/me_orgs.go`), which applies the same
three fences, moves the live sessions and revokes the refresh tokens —they carry the organization frozen in—.
The token the browser already has does NOT change: the SSO login has to be redone, which with a live
session cookie is a redirect, not a login. Without this endpoint, the active one was the oldest
membership and there was no way to change it: someone who was `member` in the old one and `owner` in the new one got
403 across the whole console.

Organization (tenant) → Workspace → Users. tenant ⇄ Sytue ID group `org/<slug>` ⇄ Kapsule
namespace `tenant-<slug>` (ResourceQuota + NetworkPolicy) ⇄ per-tenant schema/rows in PostgreSQL ⇄
dedicated S3 prefixes. **Sytue Cloud (PaaS):** git push → Kaniko build (rootless Job) → Scaleway
Registry → deploy (Serverless Containers or namespace) → preview/prod URL with TLS. Managed services
(Sytue Postgres/PostgREST, Storage, Functions, Cron) inject credentials into the project's deployments.

<a id="10-seguridad-y-rgpd-detalle-en-security-gdprmd"></a>

<a id="10-seguridad-y-rgpd-detalle-en-security-gdprmd"></a>

## 10. Security and GDPR (details in security-gdpr.md)

EU-only residency. TLS 1.3; encryption at rest (S3/Block + encryption of sensitive columns in Postgres).
Secrets in Scaleway Secret Manager, materialized as Kubernetes `Secret`s
by the production bootstrap (never in git). Mail: SPF+DKIM+DMARC+MTA-STS+TLS-RPT.
Centralized auditing (Sytue ID + each service + Hub). Backups: PG snapshots + S3 versioning.
Compliance: DPA, ROPA, DSR runbook, retention. **Critical auth** (Sytue ID) undergoes a security
audit before production (see `security.md#4-politica-de-reimplementacion-clean-room` and `security.md#1-seguridad-y-rgpd`).

<a id="11-integracion-seamless-sytue-shell--sytue-hub"></a>

<a id="11-integración-seamless-sytue-shell--sytue-hub"></a>

## 11. Seamless integration (Sytue Shell + Sytue Hub)

`@sytue/shell` (front end) provides navigation/app switcher/search/notifications/presence/account to every
app. **Sytue Hub** (Go service, `hub.sytue.eu`): federated search across the services (drive/mail/
calendar/contacts/notes/rooms), unified notifications (SSE/WS), presence, **cross-app intents**
(attach from Drive in Mail, create an event from an email, share in Chat). It authorizes with Sytue ID tokens.

<a id="12-licencias-propiedad-y-politica-clean-room"></a>

<a id="12-licencias-propiedad-y-política-clean-room"></a>

## 12. Licenses, ownership and clean-room policy

- **Sytue product code = Apache-2.0 (© 2026 Daniel Delgado Ramos (thinkddr)).** No copyleft;
- **Clean-room reimplementation:** the products are **original works** built on **open standards**
  (OIDC, WebDAV, CalDAV, CardDAV, SMTP/IMAP/JMAP, OOXML/ODF, ActivityPub/our own) — ideas,
  functionality, _look & feel_ and protocols are **not** copyrightable. Copying,
  translating or porting AGPL code is **forbidden** (it would be a derivative work). Whoever implements **does not consult** the
  source code of the AGPL engines; they work from public RFCs/specs/APIs. Detailed process and log in
  [`docs/security.md#4-politica-de-reimplementacion-clean-room`](./security.md#4-politica-de-reimplementacion-clean-room).
- **Allowed product dependencies:** only **permissive libraries** (MIT/BSD/Apache-2.0/ISC/
  PostgreSQL/MPL-2.0). Notices are kept (NOTICE/attribution). The allowed third-party
  applications are **LiveKit** and **unmodified Synapse**, within the limits of §0.2. Incorporating,
  linking, packaging or redistributing AGPL/GPL or _source-available_ dependencies
  (SSPL/BSL/MSL/Elastic/RSAL) is **forbidden**. License gate in CI (CycloneDX/SPDX SBOM).
- **Operational infrastructure:** any third-party engine that serves the runtime or processes customer
  data is approved case by case in a recorded decision, with license, terms of use,
  isolation, origin and exit plan. It does not inherit the internal tooling permission.
- **Internal engineering tooling:** Git, forges, CI, analyzers and administration tools
  run as independent processes/services and **are not incorporated into the product**. They may use
  GPL/AGPL if they are not linked or packaged with Sytue One, are not offered to customers, are listed in the
  tooling inventory and all their license obligations are met, including modifications and
  source offers where applicable. The product gate proves that their code does not reach
  binaries, images or installers. _Source-available_ remains denied by default and requires an ADR.
- **Sovereignty independent of the license:** the three perimeters —product, infrastructure and
  tooling— also pass the EU origin gate. A tool being FOSS, internal or hosted
  in an EU region does not make its provider/product European.
- **Trademarks:** the user-facing brand is **Sytue**; third-party trademarks are not used. Matrix and
  obligations in [`docs/security.md#5-licencias-propiedad-y-cumplimiento`](./security.md#5-licencias-propiedad-y-cumplimiento).

<a id="13-convenciones-de-codigo"></a>

<a id="13-convenciones-de-código"></a>

## 13. Code conventions

- **Go:** single module `git.sytue.eu/sytue/one`; one binary per service (`services/<svc>/cmd/<svc>`);
  layers `internal/{http,domain,store}`; wrapped errors; `context` on all I/O; `slog` for logs;
  versioned migrations; tests + `go vet`/`golangci-lint`. Proprietary SPDX header per file.
- **Frontend:** strict TypeScript; oxlint + Prettier (ESLint only for `*.astro`, which oxlint
  cannot parse yet); no UI outside `@sytue/ui`.
- **Data:** PostgreSQL as the single truth; migrations per service; multi-tenant via schema/RLS.
  Migrations are **expand → contract** and a CI gate checks it (`platform/pkg/migcheck`,
  inside `go test ./...`): an `.up.sql` with `DROP COLUMN`, `RENAME`, a type change, `SET NOT NULL`,
  `TRUNCATE` or removal of a policy/constraint/function breaks the build unless the file carries
  `-- sytue:destructive-ok <razón>`. Rationale in `operations.md#42-verificar-y-revertir`.
- **IaC:** Terraform `fmt`, reusable modules, state in a Scaleway S3 backend.
- **Before writing plumbing, look at [§13.2](#13-2-lo-que-ya-existe)**: the catalog of what already
  exists in `platform/pkg/` and in `apps/frontend/packages/`, and what is NOT unified and why. The
  efficiency rules are in [§13.3](#13-3-eficiencia) and what test to leave behind in
  [§13.4](#13-4-pruebas). The reasoning behind all of it, in [ADR 0037](decisions.md#0037).
- Product documentation in **English**; identifiers in English; code comments are still mostly in Spanish. Code license: **Apache-2.0, © 2026 Daniel Delgado Ramos (thinkddr)**.

<a id="13-1-idiomas-de-producto"></a>

<a id="131-idiomas-de-producto--normativo"></a>

### 13.1 Product languages — NORMATIVE

**DECIDED 2026-08-11 (Daniel).** The two questions this document left open
have an answer, and both are pending work, not discussion:

1. **The ~444 Spanish `WithMessage(...)` calls in the 8 Go services move to ENGLISH.** The
   platform's default language is English; Spanish is just another translation.
   This is NOT cosmetic: these are messages the interface shows to the user as they are (the
   logo card shows Sytue ID's message untouched), so today a German administrator
   gets Spanish. Path: rewrite the literal in English in the service and, where
   the message is meant for a person and not for a log, move it to the catalog. Do it per
   service, not in one sweep: the frontend i18n gate does not cover Go.

   **Actual count measured on 2026-08-11: 530 messages in ELEVEN services**, not the 444 in
   eight this document claimed. Per service: `id` 165 · `mail` 70 · `drive` 70 ·
   `rooms` 58 · `calendar` 44 · `cloud` 43 · `integrations` 36 · `docs` 18 ·
   `contacts` 13 · `hub` 10 · `ai` 3.

   Suggested order: **`id` first** —it has the most and is the only one whose messages are
   shown on screen as they are today (the logo card)—, and within it `admin_branding.go` is already
   done. Then the ones with their own interface in front.
   Command to measure progress:

   ```sh
   for d in services/*/; do grep -rho 'WithMessage("[^"]*"' "$d" --include='*.go' |
     grep -cE '[áéíóúñ¿¡]|\b(de|la|el|los|las|un|una|que|no|con|para|se)\b'; done
   ```

2. **SVG is accepted for the organization logo — RESOLVED 2026-08-11.** It was the
   format most corporate logos arrive in, and forcing an export to
   PNG was the most expensive concession of the design.

   **It is rasterized in the BROWSER, not on the server**, which is the opposite of what
   this document planned. The original plan —sanitize the SVG in Go and rasterize it there—
   required a new rasterization dependency, and bringing in a dependency is “risk B”
   of this very policy; it also left Sytue ID, which guards the JWKS, parsing
   third-party XML. Converting on the client makes both things disappear: the service
   NEVER receives an SVG, so there is nothing to sanitize and no new surface to
   protect, and the fitting engine receives a bitmap like that of any other
   logo. Zero dependencies.

   It is safe because an SVG loaded through `<img>` runs in image mode: the browser does not
   execute its script or fetch its external resources, per the specification. And if the file
   tried to load something from outside, the canvas would be tainted and `toBlob` would fail,
   so that case rejects itself. See `svgAPng` in `OrgLogoCard.tsx`.

   The API still accepts only raster, and its error message still explains how to
   export: whoever calls the endpoint without going through the console loses nothing.

**The platform speaks 26 languages: the 24 official EU languages —English as the BASE— plus Norwegian Bokmål and
Icelandic, to cover the EEA and Switzerland** (extended on 2026-09-14; until then there were five: English,
Spanish, French, German and Italian). It is not a goal for one wave: it is a condition of “done”.
A feature without all the languages of its app is not finished, and the compiler enforces it
(`AppCatalog<typeof en>`: a new key in English breaks every catalog that lacks it).

- **The 21 new languages are released app by app.** The shared chrome (`@sytue/i18n`), Sytue ID, the
  Cloud SDK, the ERP and the desktop are in all 26. An app moves to 26 when it has its 21
  catalogs (`src/i18n/messages/<cc>.ts`); meanwhile it serves any missing language in English, on
  the server and in the island (`AppCatalogs` in `translate.ts`). The public website already ships in all 26; a
  new language is not published there without its catalog, because it would come out in English under its prefix and
  `hreflang` would announce it as translated.
- **Form of address and gender.** Each language with its own convention, like the hand-written catalogs: informal
  in es, it, pl, sv, da, nb, fi, is, ga, mt and nl; formal in de, fr, pt, cs, sk, hr, sl, bg, lt, lv,
  el, ro, hu and et. And never with the gender of the user or of third parties: an impersonal or nominal form
  instead of an agreeing participle or adjective (“Logowanie powiodło się”, not “Jesteś zalogowany”;
  “Norint išjungti”, not “Norėdami išjungti”).
- **English is the base language.** Strings are born in English in the catalog and the others are
  translations of that original. This is the language of the PRODUCT: the convention above
  —documentation in English (code comments still mostly in Spanish), identifiers in English— does not change.
- **Scope:** everything a user sees. The interface of the 11 apps, transactional emails, API error
  messages, CLI output and also the functions that GENERATE text.
- **Generating functions respect each language's grammar**, they do not translate word by word.
  The reference case is the project name suggester: in Spanish, French and Italian the
  adjective goes after and agrees in gender; in English it goes before and does not agree; in German it goes
  before, there are three genders and the preceding adjective is declined.
- **Never a loose visible string in the code.** It goes to the catalog, with all its languages, in the
  same commit. Whatever a wave cannot translate is reported as explicitly pending.

Rationale: a European sovereign suite that only speaks Spanish cannot be sold outside Spain, and
back-translating the interface costs much more than being born translated.

**Status measured on 2026-08-07.** The eleven interfaces and the IdP screens comply: the sign-in screen
negotiates via `Accept-Language` and ships in all five (`lang="de"` + “Anmelden”, `lang="fr"` +
“Se connecter”). Two gates uphold it: `apps/frontend/scripts/lint-i18n.sh` (a ratchet on the
labels in the code) and `test/e2e/tests/i18n-sin-espanol.spec.ts` (loads each app asking for English).

What does NOT comply are the **API error messages**, which this very section includes in the
scope: **444 Spanish `WithMessage(...)` calls**, distributed as follows —`id` 143, `mail` 67, `drive` 64,
`rooms` 45, `cloud` 42, `calendar` 42, `integrations` 29, `docs` 12—. The contract in
`platform/pkg/httpx/errors.go` already says what should happen: “messages are the English fallback;
the client translates by `Code`”. Today neither of the two happens.

There are two paths and it is best to choose one before touching 444 places:

1. **Meet the contract as is**: switch the fallbacks to English and have the client translate by
   code. Cheap, but the code (`bad_request`) is much less specific than the message
   («client_id desconocido», “unknown client_id”), so the user loses information.
2. **Translate on the server**: `Fail` already receives the `*http.Request`, and the IdP already has a catalog of
   552 keys in five languages with `Accept-Language` negotiation (`services/id/internal/i18n`).
   Extending that pattern keeps the specificity at the cost of one catalog per service.

Meanwhile the damage is limited —the frontend almost never paints the raw `message`—, but the rule
is being broken and it is best to decide before there are 800.

<a id="13-2-lo-que-ya-existe"></a>

<a id="132-antes-de-escribir-código-nuevo-el-catálogo-de-lo-que-ya-existe--normativo"></a>

### 13.2 Before writing new code: the catalog of what already exists — NORMATIVE

Every new product in this suite was created by copying the previous one. It is the fastest
way to start and the most expensive to maintain: what is copied **diverges by itself**, and
it does so where nobody is looking. Measured in this repo, all real:

- the same audit endpoint in Mail and in Drive, ~490 lines identical
  except 36 — and it already answered in **Spanish in one and English in the other**;
- twelve `middleware.ts` files that the reference file asked to be “copied as is”: by the time
  they were measured, only five were still byte-for-byte identical;
- three copies of the same access token getter with **different expiry
  margins** (60 s versus 30 s), which is how an app ends up sending already-expired tokens.

**The rule:** before writing a piece of plumbing, look for it here. If it exists,
it is imported. If it exists and does not fit, **extending it** is discussed, not copying it.

<a id="go-clientes-nativos--appsnucleo"></a>

#### Go, native clients — `apps/nucleo/`

A separate module (`git.sytue.eu/sytue/one/apps/nucleo`): desktop client
dependencies —keyring, D-Bus— do not enter the platform's go.mod.
It is used through a `replace` from `apps/fyne`, and from the root module for `formula` and
`numfmt` (Docs exports spreadsheets with their values): that is why the services'
Dockerfiles also copy `apps/nucleo/go.mod`.

| Package                                   | Purpose                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| ----------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ajustes`                                 | The `<config>/sytue` folder shared by the thirteen apps: theme, language, the flag that the session has changed and the inter-process lock ([ADR 0050](decisions.md#0050))                                                                                                                                                                                                                                                                                                                                  |
| `api`                                     | HTTP/WS client for the suite's services, product by product, `Elegir.Listar` (what the suite picker shows from Drive, Docs, Calendar, Rooms or Notepads, with the person's token and without a BFF), the cross-app links `RutaRedactar`/`LeerRedactar` (open the Mail composer with recipient, subject, body and Drive attachments) and `RutaCompartir`/`LeerCompartir` (the Rooms share screen), and `Fase`: which of its six states a screen is in (loading, empty, error, offline, no permission, ready) |
| `auth`                                    | OIDC session (code + PKCE via loopback on desktop, custom scheme on mobile) and the system keyring. **ONE** session for the thirteen apps                                                                                                                                                                                                                                                                                                                                                                   |
| `buscar`                                  | `Plegar`, `Contiene` and `Rangos`: the SINGLE rule for searching ignoring accents and case (“reunion” finds “reunión”). Mirror of `plegar` in `@sytue/i18n` and of the SQL function `plegar(text)`; all three read `buscar/testdata/casos.json`                                                                                                                                                                                                                                                             |
| `ficheros`                                | The system “Save” dialog, the downloads folder and “show in file manager”                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `formula`                                 | Sytue Sheets formula engine: 451 functions, tied by traces to the web one                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| `fuentes`                                 | The five Figtree .ttf files, raw. Each toolkit converts them to its own format                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `htmltext`                                | HTML to plain text (preview of an email, of a note)                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `i18n`                                    | Catalog of the 26 languages, generated from the web                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `numfmt`                                  | A cell's number format (es-ES presets and masks): the same text in Calcus and in the Docs service's spreadsheet downloads                                                                                                                                                                                                                                                                                                                                                                                   |
| `offline`                                 | Records per account and space, and a shared HTTP cache. On native it encrypts by default with a key from the OS secure store, migrates the previous JSON resumably and allows deleting on exit without decrypting. On its own it does not implement a mutation queue or backend E2E.                                                                                                                                                                                                                        |
| `storage`                                 | Contract for private application records, authenticated loading of complete catalogs and validation of the revision graph: person/organization/product scope, object, revision, role, conflict branches and references to a read-only source. Repositories import `LoadCatalog` and this package's validators; they do not copy the room traversal or the graph reduction. Records travel inside Matrix encryption; this package does not implement cryptography nor migrate the apps' data on its own.     |
| `pdf`                                     | Shared original PDF engine: reading, rendering, editing, forms, annotations and signatures; no toolkit or storage. The backend keeps its API through aliases generated with `go run ./tools/sync-pdf-surface`; authorized clients import the same engine.                                                                                                                                                                                                                                                   |
| `cms`, `pdffuente`, `pdfsimple`, `pdfdoc` | Shared engines for CMS, fonts and basic or accessible PDF writing. `platform/pkg/` keeps the public surfaces through the existing generator; authorized devices use the same engines.                                                                                                                                                                                                                                                                                                                       |
| `office/model`                            | The complete domain of Docus, Calcus and Diapos: CRDT, ACL, protection, import and export, themes, templates, scripts and `Service` over its `Store` interface. The services keep aliases; the core makes it possible to implement private storage without duplicating those rules. Its availability does not by itself activate the migration or the clients.                                                                                                                                              |
| `guiones`, `clave`                        | Common rules for script permissions and quotas, and link password derivation. The services re-export the core's implementations.                                                                                                                                                                                                                                                                                                                                                                            |
| `miniatura`                               | Single raster image engine with pixel and memory bounds, downscaling and encoding. `RenderConContexto` waits its turn before loading the private source and wipes its buffer when done; the services import it through `platform/pkg/miniatura`.                                                                                                                                                                                                                                                            |
| `ingress`                                 | Execution of opaque inputs on authorized devices: encrypted journal, retained authority and generation, revalidation before publishing and confirming, and `RootConsent` for explicit enrollment or recovery of the same root. It uses the domain ports and the maintained SDK; it does not copy signatures, ciphers or HTTP transports.                                                                                                                                                                    |
| `matrix`                                  | Maintained Matrix E2EE client: devices, verification, recovery, attachments and Rooms messages. Private storage rooms have per-product devices and exclude the controller and any other person from their keys.                                                                                                                                                                                                                                                                                             |
| `sistema`                                 | Operating system language and theme                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `externo`                                 | Downloading what is OUTSIDE the suite —the image of an image note, whatever someone pasted as a URL— with the caps a browser applies on its own                                                                                                                                                                                                                                                                                                                                                             |
| `sistema/escritorio`                      | The desktop bits Fyne does not provide: the number on the icon (`Insignia`), whether anything paints the tray (`HayBandeja`), the system notification that takes you to its destination when clicked (`Avisar`, Linux only today) and the suite's global shortcut (`Atajo`)                                                                                                                                                                                                                                 |
| `tokens`                                  | The colors of the `branding/tokens/colors.css` sheet, as `image/color.NRGBA`, `Crudo` (the token as the sheet writes it, without the flattening of borders over `surface`: for the border that sits on the ink chrome), and how they are composed: `SobreSRGB` (one translucent color over another), `Aplanar` (a group opacity, `opacity-40`, against a background) and `ConAlfa` (Tailwind's `/60`)                                                                                                       |
| `webref`                                  | Reading a web file in a test, to compare mirrors                                                                                                                                                                                                                                                                                                                                                                                                                                                            |

<a id="go-interfaz-nativa--appsfyneui"></a>

#### Go, native interface — `apps/fyne/ui`

Device verification and key recovery live in
`apps/fyne/cifrado`: it is the shared panel extracted from Rooms. Each app contributes its
canvas, translation and product bootstrap; the panel keeps the SDK's
life cycle. Rooms adds the verifiable people and the call licenses.

The web equivalent lives in `@sytue/shell/matrix`, in a subpath so that the other
surfaces do not load the encryption SDK just by importing the shell. The app contributes SSO,
bootstrap and preparation/shutdown of its repository; its children are only mounted after
the private preparation. Shutting down waits for tasks to drain before releasing the SDK.
The verification and recovery texts are shared from `@sytue/i18n` and are generated
for Fyne with the existing generators; the Rooms catalog is not copied.

Our own Fyne widgets (§13.6). Each one is the mirror of a piece of
`packages/ui` or of something the web writes the same way in several apps, with its size
tied to production by a guard. **Before writing a widget in an app's package,
look for it here**; the contract is the rules in the package comment of
`ui.go`. Next to it live `tema` (tokens, scale, shadows; the Figtree faces, with
`NegraCursiva` for `<b><i>`, which Fyne does not embolden; `SinSombras`, the theme without the
edge shadow Fyne paints on a scroll and the web does not, and `SinBarras`,
additionally without the scrollbar of strips that slide sideways) and
`iconos` (the
83 from the web and the 18 brand marks, as typed constants).

| Family    | Pieces                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| --------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Base      | `Caja` (the border, which is not a stroke), `Aro` (focus ring; `Abierto()` for the control that does not cover its center with an opaque box), `Sombra`, `BordeSobreCromo` (a sheet border composed over the ink chrome and not over `surface`: the hairline of the bar and of the sidebar), `Texto`/`Medir`/`ColocarTexto`/`Truncar`, `Rotulo`, `Versal`, `Parrafo` (the `<p>` that wraps into lines and is clipped with `line-clamp`), the layouts `Fila`, `Columna`, `Pila` and `Flujo` (the `flex-wrap`; with `Ancho` it declares the height once wrapped)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| Controls  | `Boton` (four variants by four sizes; `Hueco` is the `gap-*` between glyph and label when it is not the size's own), `BotonIcono`, `BotonFlotante` (the `MobileFab`: the main action on the phone, below 768), `Enlace` (the `<a>` with `hover:underline`, or the `<button>` that looks like one), `Tecla` (the `<kbd>`), `Segmentado`, `Deslizador`, `MuestraColor` (the round swatch of the CertiPDF annotation palette)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| Form      | `Entrada` (with the `CampoBusqueda` size for a search box and `Rotulo`, its accessible name), `AreaTexto`, `Selector`, `Eleccion` (checkbox and radio), `TarjetaEleccion` (the `ChoiceCard`: one option of a radio group with its `Icono`), `GrupoCampo` (the `FieldGroup`: label, control, help and error), `Conmutador`, `CampoFecha`/`CampoHora`, `AyudaEnLinea`, `PistaCampo`, `EditorPlano`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| Structure | `Armazon` (with `AlCajon` and `CerrarCajon` for the mobile drawer), `BarraMovil` (the bar under the suite's bar below 768), `Lateral` (with `SeccionLateral` —`AccionesAlPasar` hides its actions until hovered—, `FilaLateral` —`Tachada` for a completed task—, `RamaLateral` —with its `Pliegue`—, `AccionLateral` and `Plegador`), `BarraSuite` (created with `NuevaBarraSuite(t, s, iconos.<Producto>)`: it carries its app's brand mark), `CabeceraApp` (its `Ancho` declares the height when it wraps, like `SectionHeader`), `Pestanas`, `Migas` (with its `EstiloMigas`: font size, hover background, chevron ink and `Iguales`, which paints the current segment like the others), `Pasos` (a wizard's `Stepper`), `BarraDeLote`, `PaginaNoEncontrada` (the native 404: a link to something deleted or to a route the app does not know), `FranjaSoloLectura` (the `ReadOnlyBar`: the read-only strip with its way out —“Edit” if the lock is the person's own, “Request edit access” if they lack the permission—; in the editors it is hooked up by `editor.EngancharPeticionEdicion`) |
| Data      | `Tabla` (also with cells that are objects), `FilaLista`, `FilaDrive`, `Rejilla`, `Tarjeta`, `Avatar` (with `Foto`; `PresenciaDe` translates a Hub presence state into its dot), `SelectorAvatar` (the `AvatarPicker`, with `AvatarPredefinido`: the initials rendered as an image to upload as a photo), `Insignia`, `Contador`, `Medidor`, `Vacio`, `Esqueleto`, `Rueda`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| Overlays  | `Dialogo` (`Recolocar` re-centers it when its content changes; `PieAncho` stretches the footer across the whole panel), `Confirmacion` (the `ConfirmDialog`: an `sm` `Dialogo` with the cancel and confirm footer already assembled; `Ocupado` while it works and `SinConfirmar` while something is missing), `Velo`, `Menu` (with `Ancho` for the one the web fixes; `ElementoMenu.MantieneAbierto` for the item that does not close the menu and `PuntoColor` for a color dot that is data), `SelectorSuite` (the `SuitePicker`: pick a Drive file, a document, an event, a room or a note as a reference), `MostrarPopover` (the `Popover` on an `Emergente`, which has `Recolocar` and `Rotulo`), `Aviso` and `PilaAvisos`, `Pista`                                                                                                                                                                                                                                                                                                                                                            |

**For tests, `ui/recorrido`**: `recorrido.Comprobar(t, w, tope)` clicks everything visible in a
test window —and whatever keeps appearing—, closes each layer that opens with Escape and flags as errors
panics and layers that do not close. Each app has its `TestRecorridoConSesion` against the fake
service of its own tests; a new app brings its own.

**And what stays in its app on purpose**, because the web paints it differently
(§“What is NOT shared”, below): the Mail conversation row is not
`FilaLista`; the ERP rail measures 210 and the suite's 260; the menu row in
Rooms measures 35 and everyone else's 33.25; the editor's color swatch (square) and the
avatar's (with the border inside) are not `MuestraColor`; the ERP filter
pill (`text-xs`, 21 high, `bg-selected`) is not the Hub's (`text-sm`, with a
border and `bg-primary`); the Calendar view switcher (semi-bold `text-sm`,
`h-7`) is not the account menu's `Segmentado` (`text-xs`, 21 high), and neither is
Drive's list/grid switcher, which carries icons and `divide-x`; and the editor
ribbon tab measures 31 and is not in small caps, so it is not `Pestana`; and
Mail's “Attach from Drive” is not the `SelectorSuite` in Drive mode, because on
the web it is not either (`DrivePickerModal.tsx` versus `SuitePicker.tsx`): it carries a
footnote, a session error with “Reload” and the folders in
`font-medium`. Each one explains it in its comment.

And in `apps/fyne/sesion`, besides the session, the appearance, the language and the
presence of the thirteen apps: `Residente` (the app stays in the system
tray and closing the window hides it instead of quitting), `Abierta` (whether another app of
the suite is open in this user session) and `AlNoEncontrar`/`NoEncontrado`
(who shows each app's `PaginaNoEncontrada` and how to request it), and
`Suite.Conexion(producto, privadas...)`, an app's HTTP client with the suite's
“network first” cache and the monitor that knows whether there is a network (`privadas`, the routes
that are not stored). In `apps/fyne/iconos`, `Dibujo.Apagar` is the `opacity-40
grayscale` of the tile of a product the account cannot use. And in `apps/fyne/editor`, what the
three editors share: `AbrirArchivo` (“New” and “Open a file” in the File menu,
which each product hooks up with `Contexto.Nuevo` and `Contexto.Abrir`; its `Importa`
are the extensions no app claims and that the app imports anyway, like
the `accept` of its web home page) and
`Autoguardado` (the web's `saveTimerRef`: it groups changes and saves ONCE
when they stop arriving) `Servicio` (download, save a version, history
and restore, with the web's notices) and `ElegirDelSistema` (the system file
picker, the one behind `<input type="file">`).

#### Go — `platform/pkg/`

| Package         | Purpose                                                                                                                                                                                                          |
| --------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `asistente`     | Calling Sytue AI (`/api/v1/assist`) from a service, with the person's token, and trimming the response JSON                                                                                                      |
| `auditlog`      | Admin reading of the compliance log: filter, keyset, cursor and CSV                                                                                                                                              |
| `authn`         | Validating the Sytue ID JWT (JWKS with caching and rotation)                                                                                                                                                     |
| `cache`         | Client for the ephemeral cache/pub-sub                                                                                                                                                                           |
| `callaccess`    | Access fences for document calls: synchronous closure in Rooms before the ACL commit, monotonic version and shared flushing from Docs and ID                                                                     |
| `clave`         | Resource passwords (Drive and Docs public links): Argon2id PHC hash and constant-time verification                                                                                                               |
| `cms`           | Signing and verifying detached CMS SignedData (RFC 5652)                                                                                                                                                         |
| `commerce`      | Commercial contracts shared between services                                                                                                                                                                     |
| `config`        | Typed configuration from the environment                                                                                                                                                                         |
| `correo`        | Sending a text email through the Sytue Mail MSA (mandatory STARTTLS with credentials)                                                                                                                            |
| `events`        | Event bus on PostgreSQL                                                                                                                                                                                          |
| `guiones`       | Product-independent rules for user scripts (ADR 0069): code bounds, FIXED hourly quotas, states of a version, who runs which, and the hash that backs an approval. Docs and Bases import them                    |
| `httpx`         | API errors (`Error`/`Fail`/`JSON`), cookies and CSRF                                                                                                                                                             |
| `ingressroutes` | Checks the routes a service mounts against the ones it declares                                                                                                                                                  |
| `keycrypto`     | Encryption of sensitive material (private keys, secrets)                                                                                                                                                         |
| `log`           | Structured loggers with `log/slog`                                                                                                                                                                               |
| `logofit`       | Fitting the logo an organization uploads                                                                                                                                                                         |
| `migcheck`      | Expand → contract gate for migrations                                                                                                                                                                            |
| `miniatura`     | Services API for the shared engine in `apps/nucleo/miniatura`                                                                                                                                                    |
| `objectstore`   | S3, Google Drive, OneDrive, Dropbox and WebDAV clients; shared blob contract and SSRF barrier                                                                                                                    |
| `pdfdoc`        | Text PDF with embedded fonts and layout by real metrics; the basis for tagged PDF (PDF/UA)                                                                                                                       |
| `pdffuente`     | Reading TrueType and writing its embedded subset (CIDFontType2, /W, /ToUnicode); Figtree and Liberation Sans OFL                                                                                                 |
| `pdfsimple`     | Text PDF with only the standard library                                                                                                                                                                          |
| `personas`      | Who “@ana” is in Sytue ID (`people:resolve`, ADR 0040), with the asker's token (Rooms and Cast); and `NombresOrgs`, the display name of an outside organization (Hub, Rooms, Calendar)                           |
| `pg`            | PostgreSQL over pgx: pool, `InTenantTx`, per-tenant RLS and versioned expansion of the common resource schema (`WithResourceRelayDDL`)                                                                           |
| `resourcepg`    | Common opaque relay for collaborative resources: exact CAS, durable holds, immutable SDK evidence and ACK reconciliation without reactivating the producer; transaction, routes and outbox injected per product. |
| `pgtest`        | Integration test harness against a real Postgres                                                                                                                                                                 |
| `portabilidad`  | “Export your data”: the delivery each product writes and Hub assembles (tables in JSON, CSV or XML)                                                                                                              |
| `qr`            | QR code as PNG (level M, no dependencies): Sytue ID TOTP enrollment and the VERI\*FACTU tax QR                                                                                                                   |
| `sqlorg`        | The organization goes IN the SQL: a guard that walks a service's SQL literals and fails if a statement touches a table with an organization column without filtering it                                          |
| `ssrfhttp`      | SSRF barrier for outbound requests                                                                                                                                                                               |
| `telemetry`     | Correlated traces, metrics and logs                                                                                                                                                                              |
| `tenancy`       | The tenant context that runs through everything                                                                                                                                                                  |
| `validation`    | Input validation and sanitization, and `CSVSafe`                                                                                                                                                                 |

#### Frontend — `apps/frontend/packages/`

| Package                  | Purpose                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| ------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ui`                     | **All** of the interface. No UI outside it (§13). The read-only strip of any app is `ReadOnlyBar`, with its exit button                                                                                                                                                                                                                                                                                                                                              |
| `shell`                  | The bar, the app switcher, notifications, global search                                                                                                                                                                                                                                                                                                                                                                                                              |
| `auth`                   | BFF (`/server`) and browser (`/client`): session, login, `getAccessToken`                                                                                                                                                                                                                                                                                                                                                                                            |
| `api-clients`            | Typed SDK for the Go services                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `config`                 | Language and security middleware, brand tokens, Tailwind, Astro                                                                                                                                                                                                                                                                                                                                                                                                      |
| `i18n`                   | Language negotiation, shared catalogs and `plegar`/`contiene`/`rangos` (search ignoring accents and case)                                                                                                                                                                                                                                                                                                                                                            |
| `ai`, `cloud`, `formula` | Assistant, PaaS and spreadsheet                                                                                                                                                                                                                                                                                                                                                                                                                                      |
| `offline`                | Per-account store, connectivity and service worker. `@sytue/offline/matrix` shares the Matrix client and Rust/WASM; the app contributes its routes and sessions through an adapter. `@sytue/offline/matrix/types` and `@sytue/config/matrix` describe contracts without the SDK. SSO returns import only `@sytue/offline/matrix/auth`: it strips the token before loading cryptography and binds the session to its scope. Do not copy this engine into another app. |
| `escena`                 | The common vector scene (Docus, Diapos, Sytue Web): nodes, layers, connectors with anchors, freehand strokes, hit-testing, versioned serialization and SVG. Shapes, connector routing and z-order/align/distribute live here, not in the apps                                                                                                                                                                                                                        |
| `scripts`                | The isolated runtime for user scripts ([ADR 0069](decisions.md#0069)): opaque-origin iframe with its CSP, a Worker killed on timeout, a validated message protocol and the host loop that enforces the limits. Each product (Docs, Bases) contributes its `sytue.v1` surface as data and its call translator; the sandbox is NOT copied into the app                                                                                                                 |

<a id="lo-que-no-se-comparte-y-por-qué-importa-distinguirlo"></a>

#### What is NOT shared, and why it matters to tell the difference

Duplication is not the same as **deliberate variation**. Also measured here:
`ValidName` exists in four services with three implementations, and all three are
correct — 128 bytes in Calendar/Contacts, 255 in Drive because they are file
names, and 100 runes in Rooms, which validates **channel names** and therefore does not
reject `/` or `..`. Unifying them would require a length parameter and a
“path-safe” flag: two copies of sixteen lines are a better abstraction than
that.

**How to decide:** if the differences are about the _domain_ (what a name means
here), keep them separate. If they are _accidental_ (a reflowed comment, a
translated message, a number someone touched in one place), it is duplication.

<a id="los-guardianes-que-lo-sostienen"></a>

#### The guards that uphold it

This is not maintained by goodwill, because the next app will be created the same way:
by copying the previous one. There are tests that fail when someone copies again:

- `packages/config/src/plomeria-compartida.test.ts` — searches the SOURCE CODE for
  the markers of copied plumbing (the middleware's `negotiateLocale`, the
  getter's `'/auth/token'`, the BFF mounting, the assistant proxy).
- `packages/auth/src/server/rutas-auth-reexportan.test.ts` — requires every route
  under `/auth/*` to be a re-export of `_rutas.ts`.

Both compare **by equality** against their list of exceptions, not by
membership: that way they ALSO fail when an exception is deduplicated and nobody deletes
its entry. An allowlist nobody prunes stops protecting within two refactors.

> If one of these tests stops you, the usual reason is that a function that does
> that already exists. Adding your file to `excepciones` is the last option, not the first, and
> requires writing down **why** the behavior is genuinely different.

<a id="13-3-eficiencia"></a>

<a id="133-eficiencia-las-reglas-que-salieron-de-medir--normativo"></a>

### 13.3 Efficiency: the rules that came out of measuring — NORMATIVE

These are not general principles: each one comes from a specific failure in this repo.

**1. One request, one transaction.** Each store method opens its own
(`InTenantTx`), so chaining four calls means four `BEGIN`/`COMMIT`s.
Resolving a WebDAV path once cost **six transactions**; today it is two.
If a service method calls the store more than twice in a row, the question
is whether the store should offer a single operation that does them together.

**2. Traverse in SQL, not in Go.** Walking a path segment by segment was one query
per segment, and each one fetched ALL the parent's subfolders to keep
one. A `WITH RECURSIVE` brings it down to a fixed cost, **without growing with
depth**. Before writing a loop that queries, check whether Postgres can
do the whole traversal.

**3. One INSERT, not N.** Adding a selection of two hundred photos to an album was
two hundred statements, and each one carried a `max(position)` over a
table that grew with every insert. `unnest(...) WITH ORDINALITY` + a scalar
subquery (which Postgres resolves once, as an InitPlan) brings it down to
one.

**4. Read before writing in every `Ensure*`.** An `INSERT ... ON CONFLICT DO
UPDATE` **writes even if no value changes**: a dead tuple, WAL and a
lock on the tenant's single row, which all its requests go through at
once. If every request's preamble goes through there, the whole service is
serialized. The signal to check it in a test is `xmin`: if Postgres rewrites the
row, it changes.

**5. Pause polling when the tab is in the background.** `useSondeo` from `@sytue/ui`
does exactly that and queries immediately on return. A bare `setInterval` wastes
battery on the client and requests on the server to show results to nobody.

**6. Text search: `plegar()` and, at volume, an already-folded column.** All
text search ignores accents and case with the same rule: `plegar(text)`
in SQL, `buscar.Plegar`/`Contiene`/`Rangos` in Go and `plegar`/`contiene`/`rangos` from
`@sytue/i18n` on the web (two guards watch it). And a measured finding: with
forced RLS, **Postgres does not use trigram or FTS indexes for the application
role**, because `LIKE`, `ILIKE` and `@@` are not LEAKPROOF and cannot run ahead
of the policy; what narrows it down is the tenant. On 200,000 messages: `body
ILIKE` 205 ms, `plegar(body) LIKE` 440 ms, stored folded column 60 ms. That is why
Rooms stores `body_plegado` and Mail its folded tsvector; a new large table
with text search does the same.

**7. And the one that rules them all: measure before touching.** Twice in a single
day the bottleneck was not where it was assumed to be. We were hunting 700 ms of main thread and
it turned out that **nothing was compressed** —the middleware existed, deployed and without
a single one of the 49 ingresses referencing it—. And a visibility pause was about to be added to a `setInterval`
that **nobody called**. Defining something does not
apply it, and optimizing what does not run is wasted work that looks like
progress.

<a id="13-4-pruebas"></a>

<a id="134-qué-prueba-se-deja-detrás--normativo"></a>

### 13.4 What test to leave behind — NORMATIVE

Non-trivial logic leaves **one** executable check, the smallest one that fails
if the logic breaks. With two conditions that have proven costly in this repo:

- **Test the SQL against real Postgres** (`-tags=integration`, `pgtest`).
  An in-memory double only proves that the double resembles what one _thinks_
  the `WITH RECURSIVE` does.
- **Verify that the guard fails.** A test that has never been seen red does not
  protect anything: it is checked by manually inverting what it guards. The ones in this repo
  were written that way, and one caught a positions regression (`[0 1 3 4]`) that
  does not show on screen because the order still comes out right.

And beware of the **false green**, which has happened here several times:
`grep -c "^FAIL"` gives 0 both when everything passed and when the command did not even run;
`go test -run` with no matches says `ok`; and `printf … | grep -q` with
`set -o pipefail` turns _finding it_ into _not finding it_ because of SIGPIPE.
Always count the ones that passed too.

<a id="135-paridad-web--apps-nativas--normativo-2026-09-17"></a>

### 13.5 Web → native apps parity — NORMATIVE (2026-09-17)

**Every new feature of the web apps is also added to the native desktop
and mobile apps in the SAME wave, or is recorded as planned
debt.** It is not a wish: it is governed by a registry and a test.

- **Registry**: [`apps/frontend/packages/config/src/paridad.ts`](../apps/frontend/packages/config/src/paridad.ts).
  The PR that introduces (or touches) a web feature ADDS or UPDATES it there,
  with its status in `escritorio` and `movil`.
- **Gate**: `paridad.test.ts` fails if anything from the web is left without a status on both
  surfaces, if 'planificado' has no `deuda` pointing to a section that
  EXISTS in the roadmap, or if 'no' is used for something the web has.
- **And the registry is not the only net**: this one is written by hand and that is why
  it falls short —it declares 31 features for 80 web screens—. What the web
  really has is derived from the files in
  [§13.7](#13-7-el-gate-de-paridad).
- **Build and tests**: ci-desktop and ci-mobile run WHEN A RELEASE IS
  CREATED (decision 2026-09-17: the native gates validate what is
  published, not every push). The parity gate, on the other hand, runs on every PR,
  inside ci-frontend.
- **Single aesthetic exception**: the global topBar, which only exists on the
  web; the native chrome replaces it (registry entry `shell-topbar`).
- **Where the native code lives**: `apps/fyne` (interface) on top of `apps/nucleo`
  (everything that is not toolkit-specific) is the SAME base for the desktop builds and
  the mobile ones — one improvement, two surfaces. See §13.6.

The registry's debt is reviewed at the close of every wave; whatever is 'planificado' that
reaches its roadmap phase is raised to 'si' in the same PR that implements it.

<a id="13-6-la-interfaz-nativa-es-fyne-con-todo-propio--normativo-2026-09-21"></a>

<a id="136-la-interfaz-nativa-es-fyne-con-todo-propio--normativo-2026-09-21"></a>

### 13.6 The native interface is Fyne, with EVERYTHING our own — NORMATIVE (2026-09-21)

**Desktop and mobile are built with [Fyne](https://fyne.io), in the version pinned in §3, and OUR OWN widgets,
theme and layouts, up to 1:1 pixel-level parity with the web apps.**
The web is not included: `sytue.eu` remains Astro + Preact + Go (§4), and therefore
**there is no WebAssembly target**. The why, what was measured and the real ceiling are in
[ADR 0048](decisions.md#0048); this section is the normative part.

**There is no limit on how much of our own code.** If the Fyne theme gets in the way, it is
replaced; if a stock `Layout` does not give the exact geometry, one is written; if
the font has to be loaded by hand, it is loaded. **Everything** is part of parity:
geometry, typography, color, icons, spacing, states, focus, animations and
dialogs.

**Two exceptions, and only two.** They are not negligence and there is no need to ask
about them again:

1. **The `TopBar` DOES go into Fyne, trimmed down.** Corrected by the owner on
   2026-09-22; until then this section said it was not replicated, which is the
   opposite.

   Each native app carries, **at the top right**, the **three cross-cutting
   pieces**: the suite-wide **search**, the **notifications** and
   the **account menu** (the avatar). It is not app chrome: it is the suite, and that is why
   they are the same in all thirteen.

   **What it does NOT carry** is the **app switcher** —on native you switch
   apps in the operating system, not inside a window— and that is why the native bar is
   **thinner** than the web one.

   **And it is LIGHT, with the brand corner in the rail's color** (owner,
   2026-10-04: the native apps look the same as the web). With the Rail shape the brand
   color lives in the side menu, so the bar paints the web's `topbar-bg`
   and only its **brand corner** —the app's brand mark and its name— carries the
   rail's `sidebar-bg`, with the menu's width from 768 px: it is the `rail` of
   `TopBar.tsx`. The bar's hairline stops at the corner, so that the rail
   reads from top to bottom without a line cutting it. The piece is `ui.BarraSuite`
   (tokens `fondoBarra` and `fondoEsquina`) and the guards are
   `TestLaEsquinaDeMarcaContinuaElCarril` and `TestElFileteDeLaBarraNoCruzaElCarril`,
   which compare PIXELS of bar, corner and sidebar painted together. It replaces
   the 2026-09-23 rule, which painted the whole bar in the sidebar's color
   when both were the same chrome. **The bell's red dot** stays in
   `danger-text`, which against the ring in the bar's color exceeds 3:1 in all
   three modes (guard `TestElPuntoDeAvisoContrastaConLaBarra`).

   **Inside the rail, each piece asks for its token by name.** On the web everything
   that hangs from `bg-sidebar` gets the chrome family reassigned
   (`appearance.css`); Fyne has no cascade, so a piece that goes inside
   the side menu asks for `sidebar-focus`, `sidebar-indicator`, `sidebar-border`,
   `sidebar-hover-bg` or `sidebar-primary`/`sidebar-on-primary`, and not the
   `chrome-*`, `indicator` or `primary` ones, which belong to the light bar and the
   content. The menu icons carry the rail's amber fill: it is applied
   automatically by `iconos.Trazo` with a `sidebar-*` token and `DeToken` in an app's own
   glyphs (`Dibujo.EnCarril` by hand).

   **Shape is an axis on native too, and “Frame” (Marco) is the one you see without choosing**
   (owner, 2026-10-05: all at once and in every app; the web side is in
   [design.md](design.md#forma-marco)). The above describes **Rail** (Carril), which is the
   shape you choose. Four rules:
   - **The numbers come from the sheet, never from a constant.** `apps/nucleo/tokens`
     embeds a copy of `colors.css`, `appearance.css` and `theme.css` (copied by
     `scripts/sync-branding.sh`; `TestLaCopiaDeLaHojaEsLaDeBranding` fails if they
     diverge) and resolves the mode × color × app × shape cascade like the
     browser. A color is requested with `tema.Color(token, v)` and a shape measurement
     with `tema.Forma("radius-control")`, `"sheet-gap"`, `"nav-pad"`, `"row-inset"`…
     (the table is in `theme.css`, «Tokens de forma»). The headings that on the web
     are `sytue-h1`–`h4` **inside the sheet** are requested with `ui.EscalonTitulo(n)` and
     `ui.PesoTitulo(n)`; a dialog's heading is not, since it hangs outside the sheet.
   - **The sheet is painted ONCE, in `ui/marco.go`.** Fyne does not clip to rounded
     corners: the frame is the continuous color behind, the sheet's paper, the
     rectangular content on top, four corner masks in the frame's color
     over it and the `md` shadow in eight tiles. It is placed by `ui.Armazon` (the sheet
     starts where the menu ends, which lets the frame show) and, for what has no
     armature —the Hub, SyPhotos, Cast, the four editors (Docus, Calcus, Diapos,
     CertiPDF), “Sign in”—, by `BarraSuite.Montar`, all with the same `sheet-gap`
     (10 px from 768 px, 0 px below 768 px) so that the whole suite looks
     uniform between web and Fyne. The ERP carries a sheet like the others: its icon
     rail goes in `Armazon.Carril` —the sheet starts where it ends— and with it the module
     menu is a drawer at any width, which keeps its background. Below 768
     it is always full-bleed. No app paints sheet corners or shadows on its own.
   - **Whatever sits on the menu asks what is behind it** with `ui.TokenCarril()` or
     `ui.FondoCarril(v)` (`sidebar-bg` in Rail, `frame-from` in Frame), not with
     `sidebar-bg` by hand. In Frame the whole bar is chrome, so the bell's
     dot switches to `danger` and, like the account avatar, carries the ring in the rail's
     text color (`sidebar-fg`, 1.5 px: the web's `sytue-aro-carril`; 3:1 or more in
     all three colors and all three modes, `TestElPuntoDeAvisoLlevaAroSobreLaBarraDeColor`)
     and, with the “each app's own” color, the brand tile sits on a white plate.
   - **The driver's guards run in Rail** (`tema.FormaDeLosGuardianes()` in
     each package's `TestMain`): their hundreds of measurements were taken that way. What is specific
     to Frame has tests that select it (`enMarco(t)` in `ui/marco_test.go`), and
     `SYTUE_FORMA=marco` produces any test's screenshots in Frame. In a real window,
     `scripts/ventana-real.sh --forma carril|marco --color …`.
   - **The real window does NOT write to production.** The walkthrough clicks at fixed
     coordinates with the audit account, and a shifted coordinate ends up on any
     button: on 2026-10-05 it created a note, and the three editors left a template
     document behind on every run. The script launches the apps with `SYTUE_SOLO_LECTURA=1`, which
     cuts off before the network every request that is not a read (`sesion/conexion.go`; only
     the token exchange gets through). A step that needs to write is marked with `!` and only
     runs with `--escribe`, which leaves a note of what has to be deleted afterwards.
   - **The focus ring is measured in the window, not only in the tests.** Fyne's two painters
     do not stroke the same way: the software one paints a rectangle's `StrokeWidth`
     straddling the outline and the GL one inside the box. The open ring of `ui.Aro`
     used to be a stroke and came out right in the tests and one pixel further in in the real window,
     at ×1 and at ×2; now it is four strips painted pixel by pixel, which both treat
     the same. `scripts/ventana-real.sh --aros` opens the sample board (`pantalla.Aros`: one
     control of each family with focus on) at ×1 and ×2, in light and dark,
     and fails if a ring is not 2 px of the token flush against the box and at 3:1; the same
     measurement runs with the software painter in `TestLosArosDeFocoSonLosDeLaWeb`. What
     is drawn with a stroke is not protected by a test: either it is filled or it goes to the window.
     Two things measured that day stay as they are, so nobody
     rediscovers them. **FILLED rings** (solid buttons, fields, checkboxes) are in their
     place, but at ×1 they come out at 84 % in the window because of the GL painter's edge
     anti-aliasing: 3.70:1 the worst (the primary button, in light mode) and 100 % at ×2. They exceed 3:1 and
     changing them means redrawing more than a hundred controls: not worth it (decided that day).
     **The other strokes** —the translucent border of `Caja.ConTrazo`, the frame, the ring
     and the checkbox of the Bases grid, the CertiPDF marks, the Calcus table
     swatch— are painted by the GL painter inside the box, half a stroke width further in than
     the tests', at ×1 and at ×2, and that produces nothing visible: no double lines,
     no gaps, no eaten borders (measured with each place's primitives and, for the
     cards, in the real Hub). In the window a 1 px border falls where a CSS `border`
     puts it; the one that straddles it is the tests' painter. The Calcus
     cell frame and the separators are filled rectangles and come out identical.

   The preference goes in `ajustes.json` (`forma`, next to `tema` and `color`) and is chosen
   in the account menu; someone who has not chosen sees their organization's, which arrives
   in the id_token (`org_forma`, `org_color`) and is stored by the session
   (`auth.Session.Apariencia`): person → organization → house default. The first option of
   both selectors is “Organization”, which clears the choice and goes back to following it;
   a personal account does not see it.

   **And whatever is opened from there opens in the SYSTEM.** A search
   result or a notification from ANOTHER app is not painted inside the current one:
   it launches that app through its URL scheme, just like a Hub tile. It is the
   same rule as exception 2 and the same machinery.

   **And it applies equally to CONTENT, not only to navigation** (owner,
   2026-09-22): a document **opens in its own app**, not inside the one that
   listed it. A `.docx` from Drive opens **Docus**; a spreadsheet, **Calcus**; a
   presentation, **Diapos**; a PDF, **CertiPDF**; an image, **SyPhotos**. The
   same goes for an attachment in Sytue Mail or Sytue Rooms.

   Practical consequence, and a liberating one: **no app needs a viewer for
   the other apps' formats**. Drive's `PreviewDialog` is not ported; what
   is ported is the open action. And with the usual mechanism: the
   `sytue-<producto>` scheme is tried and, if the system has nothing to
   handle it, it falls back to the web — a file that does nothing when clicked is worse
   than one that opens the browser.

   What does **not** change is the hard rule: **the document is never corrupted**.
   Opening in another app means passing it a reference, not a copy: whoever saves is the
   app that owns the format, with its round trip proven.

   **FOUR THINGS ARE CROSS-CUTTING** across all of the system's apps (owner,
   2026-09-22). They are not settings of one app: they belong to the person, and that is why they
   are chosen from the account menu of any of the thirteen and apply to all of them.
   **But they do not live in the same place, and confusing that is the mistake to
   avoid:**

   |                | where it lives     | why                                                                                                                        |
   | -------------- | ------------------ | -------------------------------------------------------------------------------------------------------------------------- |
   | **Session**    | the system keyring | it is a secret. **You sign in ONCE**, not once per app                                                                     |
   | **Appearance** | shared preferences | it is not a secret, and the keyring would ask for permission on startup                                                    |
   | **Language**   | shared preferences | ditto                                                                                                                      |
   | **Presence**   | **the server**     | **others see it**; a local copy would be a second truth that goes stale as soon as it is changed from the web or the phone |

   And all four share the same problem: **thirteen processes and a change that
   all of them have to see**. For the session there is no leeway —signing out signs out of
   all of them, immediately—; for appearance and language, applying it on the
   others' next startup is acceptable **if it is said so**. Presence is already
   pushed by the server.

   **“My account” is the menu's exception**: it is not a native app, it opens the
   Sytue ID web page in the browser. It was ported to Fyne at one point and withdrawn.

2. **What is not copied is the SUBDOMAIN, not the separation.** Each product
   remains **its own application, also on desktop and mobile**: one
   installable binary per product, not one that contains them all. What is not
   replicated is the web deployment —subdomain, BFF, TLS—, because there
   parity is about **screens and pieces**.

   **And the apps call each other.** The native equivalent of going to another
   subdomain is invoking the other application through its URL scheme: `.desktop`
   and xdg on Linux, `CFBundleURLTypes` on macOS, protocol registration on
   Windows, `intent-filter` on Android, universal links on iOS. It is an action
   delegated to the system, **never one more screen of the same binary**.

   This exception was written the other way round in the first draft of this
   section (2026-09-21): it said that on native the products are _not_
   separate applications. The owner corrected it the same day, before
   anyone packaged anything.

**Where each thing lives:**

- **`apps/nucleo`** — what is NOT toolkit-specific and is used by all the clients:
  `ajustes`, `api`, `auth`, `ficheros`, `formula`, `htmltext`, `i18n`,
  `offline`, `sistema`, plus the **color tokens** (`tokens`, which interprets
  `branding/tokens/colors.css`) and the **fonts** (`fuentes`). The name does not
  include the toolkit **on purpose**: it changed twice in one month.
- **`apps/fyne/sesion`** — the cross-cutting part of a native app in one line:
  session, appearance, language and presence. No other one is set up ([ADR 0050](decisions.md#0050)).
- **`apps/fyne/tema`** — tokens, scale and the `fyne.Theme`, which exists to
  govern the TYPE and the canvas background, not to configure widgets. The
  variant is requested with `tema.De`, which resolves the person's choice: Fyne
  does not allow forcing it (`app/settings.go:75-86`).
- **`apps/fyne/ui`** — our own widgets. **Before writing the second one,
  read the whole package comment of `ui.go`**: there are six rules, each one
  with the measurement that justifies it, and skipping them costs sixteen apps.
- **`apps/gio`** — RETIRED on 2026-09-23, together with `services/gioweb` and the desktop in
  `apps/desktop`. The copy lives in `gio-archivado/`, outside git.

**There is ONE single native path: this one.** On 2026-09-21, setting up Wails v3 as well
—the web inside a WebView— was explored for a few hours as an alternative, and the
owner cancelled it the same day so as not to split the effort. No code reached the
repository. If it is proposed again, it should come with the question answered of what it
solves that Fyne does not.

**And no gate carries a toolkit's name inside it.** The parity registry
speaks of `escritorio` and `movil` on purpose: the toolkit changed twice
in one month and the gates survived because they did not name it.

**The four rules most often broken** (all six, in `ui.go`):

1. **Only primitives inside the renderer.** `canvas.Rectangle`, `canvas.Text`,
   `canvas.Image` and other `ui` widgets. Never `widget.Button` or
   `container.NewVBox`.
2. **A single computation of the size.** Each widget exposes `MedidaX(...)`; its
   `MinSize()` calls it and the guard compares against it AND against the
   painted pixels.
3. **The web's decimals are written as they are.** They are not rounded: Fyne
   quantizes to the DEVICE pixel, and rounding in our code freezes at ×1
   an error that a HiDPI screen fixes on its own.
4. **The border is never `StrokeWidth`** (Fyne paints it centered and it comes out at half
   strength): it is `ui.Caja`.

**What test a new widget leaves behind.** One that compares the DECLARED size, the one it
requests from Fyne and the PAINTED one, against the number measured in production —not against what
the widget itself computes—. It runs without CGO and without a display server: Fyne's
software painter is pure Go, so it goes in the pre-push. The example to copy is
`apps/fyne/ui/boton_test.go`.

**And that is why NOTHING is painted with `canvas.Shader`.** The type exists in Fyne v2.8.1 and
it is tempting —a GLSL fragment shader with its uniforms—, but **the software
painter does not paint it**: it does not fail, it does nothing. Its `switch` has no case for
`*canvas.Shader` (measured on 2026-09-21: a shader that returns opaque red leaves
**0 red pixels out of 10,000**). A widget that depends on a shader is invisible
wherever there is no GPU, starting with these guards. Whatever needs an effect
is precomputed into an `image.Image` and painted with `canvas.Image`, which both
painters treat the same; the worked example is `apps/fyne/ui/sombra.go`.

**Be careful also with `canvas.Rectangle.Shadow`**, which both do paint: its
`BlurRadius` means different things in each —a Gaussian with σ =
√(2·BlurRadius/scale) in software, `smoothstep` in GL—, so the shadow
**changes size with DPI** (σ of 7.00 px at ×1, 5.00 at ×2 and 4.00 at ×3 with the
same value). It is no good for pixel parity.

<a id="13-7-el-gate-de-paridad"></a>

<a id="137-el-gate-de-paridad-lo-derivado-manda-sobre-lo-declarado--normativo-2026-09-21"></a>

### 13.7 The parity gate: what is derived overrides what is declared — NORMATIVE (2026-09-21)

**What the web has is not declared: it is DERIVED from the files.** A registry
that is updated by hand goes stale silently, and this one already did: §13.5
governs 31 features while the web has **80 screens** (`.astro` under
`apps/frontend/apps/*/src/pages/`, excluding the public website) and **46 pieces**
(`apps/frontend/packages/ui/src/components/*.tsx`, excluding the TopBar, which is counted
separately because of what §13.6 says). No entry was false; the registry
simply stopped looking.

- **Gate**: `apps/frontend/packages/config/src/paridad-pantallas.test.ts`. It runs
  in `bun run test` (that is, in ci-frontend) and in the pre-push, also when the
  push only touches native code. Twelve milliseconds: it only reads files.
- **What it derives**: the web inventory. A new screen shows up on its own, without
  anyone recording it.
- **What is declared**, because it cannot be read from a file tree: the snapshot
  of the debt and the exceptions.

**How to say that something IS already ported.** The native file declares it
itself, with a marker in the code:

```go
// paridad-web: apps/frontend/apps/mail/src/pages/index.astro
```

It works from any native surface in `apps/` (except `apps/frontend`, which is
the origin) and **deliberately does not carry the name of any toolkit**: it has changed twice
in one month. The
registry speaks of `escritorio` and `movil`, and that is how it stays.

**How debt is paid off** — the two steps go together, in the same PR:

1. The screen is ported and the marker is put in the native file.
2. **Its line in `DEUDA_SIN_NATIVO` is deleted.** If it is not deleted, the gate turns
   red saying «YA NO ESTÁ PENDIENTE: …» (no longer pending). It is not a nuisance: paying off debt without
   lowering the snapshot leaves free room for the next one without anyone noticing, which
   is how we got to 35 features out of 35 in 'planificado'.

**How debt grows when it really has to grow.** A new web screen
that cannot be ported in this wave is added by hand to `DEUDA_SIN_NATIVO` and the PR
says when it will be closed. Adding that line is the act that is visible in
review; the gate exists so that it cannot be avoided.

**How a legitimate exception is added — and why it is almost never the right move.**
`SOLO_WEB` is for what will **NEVER** exist on desktop or mobile, not
for what does not exist yet: that is debt. **It is the last option, not the
first**, and it requires writing the reason in the entry itself. Today these are:

- `TopBar.tsx` — **is NO longer an exception as a whole** (§13.6, corrected on
  2026-09-22): its three cross-cutting pieces —search, notifications and account
  menu— **do go** into each Fyne app, at the top right. The only thing not
  ported is the **app switcher**, because on native you switch apps
  in the operating system. As long as the gate reasons per file and not per piece, the entry
  stays here with this reason written down.
- `apps/frontend/apps/www/` — the public website is SSG, without a session and without a backend;
  it is not a product of the suite, and a desktop app does not carry its own
  marketing website inside it.
- `apps/frontend/apps/account/src/pages/index.astro` (“My account”) — web only by
  the owner's decision (2026-09-22): the native account menu opens Sytue ID in the
  browser.
- `apps/frontend/apps/cast/src/pages/embed/[id].astro` — the player for the
  `<iframe>` of a third-party website; on native there is no third-party iframe.
- `apps/frontend/apps/docus/src/pages/wopi/editor.astro` — the page that hosts the
  external editor via WOPI; desktop opens it in the browser (owner, 2026-10-01).
- `apps/frontend/apps/console/` and `apps/frontend/apps/cloud-dashboard/` — Sytue
  Admin and Sytue DevX are web only **forever** (owner, 2026-10-01): they
  will not have a native app, so their screens are not debt.
- `apps/frontend/apps/pages/` — Sytue Web is web only **for now** (owner,
  2026-10-02): the catalog has it as `nativa: false` and its screens and its
  fourteen features do not count as debt. If it is ever ported, the exception
  is removed and they go back to being debt.

And they are not there for decoration: if what the exception covered disappears from the web, the
gate requires removing it.

**What the gate does NOT allow, however convenient it may seem.** Each product is its own
application on desktop and mobile too —what is not replicated from the web is the
subdomain and the deployment, not the separation—, so **a native file cannot
port screens from two products**. Products call each other via
deep link. It is a test and not a comment because the convenient way to port —a
list that serves Mail and Notepads, “since they are almost the same”— merges two
products without anyone deciding it and leaves no readable trace in any diff.

**The gate has been seen red in all four directions** (a new screen not
ported, a ported screen without lowering the snapshot, a marker pointing to a renamed
screen, and a native file serving two products), and in all four it gives
the file's name: a gate that says “something is missing” without saying what gets ignored by the
third day.

<a id="14-sytue-cloud--dx-y-servicios-gestionados-vercel--supabase--google-cloud"></a>

<a id="14-sytue-cloud--dx-y-servicios-gestionados-vercel--supabase--google-cloud"></a>

## 14. Sytue Cloud — DX and managed services (Vercel × Supabase × Google Cloud)

Project-centric DX, simpler than Google Cloud, with Vercel's immediacy. Deploy (connect repo →
Kaniko build → preview per PR → promote → rollback; automatic domains+TLS). FOSS/EU managed
services on PostgreSQL: **Sytue Postgres** (with automatic PostgREST REST API, pooling, backups,
branching), **DB Studio** (Supabase style, Sytue design), **Storage** (S3), **Functions** (serverless),
**Cron**, **Secrets/Env**, **Managed auth** (via Sytue ID, one realm per project). Details in `docs/architecture.md#3-sytue-cloud-paas`.

<a id="15-entornos-dev-local--base-scaleway-paridad-devprod"></a>

<a id="15-entornos-dev-local--base-scaleway-paridad-devprod"></a>

## 15. Environments: local dev + Scaleway base (dev/prod parity)

- **Dev = LOCAL:** Docker Compose in one command (`make dev-up`), `*.sytue.localhost` with mkcert TLS
  (Traefik), Postgres + MinIO + LiveKit + **all the Go services** + frontends. Seed included.
- **Prod = SCALEWAY:** Terraform (Kapsule, RDB Postgres, Object Storage, Registry, LB, DNS, Secret
  Manager, LiveKit) + Helm (umbrella `sytue-one`, only Go services + LiveKit + Traefik). EU only.
- Same logical configuration parameterized per environment; endpoints/scale change, not the architecture.
- **TESTING goes to an ephemeral dev environment on Scaleway or directly against production, not to the
  laptop** (requirement since 2026-08-09; scope corrected on 2026-08-10). Building images
  or bringing up services on the developer's machine has two problems: it eats the 8 GB they
  need to work, and **the result does not resemble production** —architecture, network,
  versions—, so a local green does not prove much.
  **Verifying a build** also counts as testing: the exception for `go build`/`go vet`/
  unit tests/linters (“they do not bring anything up”) was too broad and ended up as compiling in a loop on
  the laptop to keep checking the work. The default path is not to set up an ephemeral environment but
  **to push and let CI (GitHub Actions) verify it, since it already IS that remote environment**; the
  ephemeral environment is brought up only for what CI does not provide (live debugging, network or
  architecture failures) and **is deleted when done**. What remains local is what the pre-push hook
  runs by itself when pushing, which is part of the flow. And nothing is left running: if when starting
  there is a local dev environment running, it is brought down.

---

<a id="16-naming-dominios-y-dns-anexo-normativo"></a>

<a id="16-naming-dominios-y-dns-anexo-normativo"></a>

## 16. Naming, domains and DNS (normative annex)

> Reference document for the **brand system**, the **full table of
> domains/subdomains**, the **ports of the Go services**, the **DNS plan** for
> `sytue.eu` on **Scaleway DNS** (including the mail configuration for
> **Sytue Mail**, our own Go mail server) and the **naming conventions** for
> cloud resources and Kubernetes _labels_.
>
> Contract source of truth: [`docs/CONVENTIONS.md`](./CONVENTIONS.md) (v4). Every
> name, domain, subdomain or port described here **must** match the canonical
> contract. Sovereignty: **100 % EU on Scaleway** (EU regions only). No US providers
> for personal data.
>
> **Single and total stack (v4):** Backend = **Go** (one service per product, single
> module `git.sytue.eu/sytue/one`); Frontend = **Astro + Preact**; **single** datastore =
> **PostgreSQL** (state + queues + cache on Postgres — **no Redis/Valkey**); large
> blobs in **S3 Object Storage** (Scaleway prod / MinIO dev). **No third-party AGPL
> engine** except the Matrix transport authorized in §0.2 (Keycloak, Nextcloud, OnlyOffice,
> Mattermost and Stalwart are **out**). **LiveKit** transports _Sytue Rooms_ media.

---

<a id="161-sistema-de-marca"></a>

<a id="161-sistema-de-marca"></a>

### 16.1 Brand system

<a id="11-jerarquía-de-marca"></a>

#### 1.1 Brand hierarchy

| Level        | Name                   | What it is                                                                   |
| ------------ | ---------------------- | ---------------------------------------------------------------------------- |
| Parent brand | **Sytue**              | The parent company/brand. It is the "surname" of the whole ecosystem.        |
| Suite        | **Sytue One**          | The unified product bundle (Google Workspace style) and the portal/launcher. |
| Products     | **Sytue \<Producto\>** | Each surface of the suite (Sytue Mail, Sytue Drive, …).                      |

- **Naming scheme:** always `Sytue <Producto>` (a single space, `<Producto>`
  capitalized, singular and in **English** except for product names already in Spanish).
  Valid examples: `Sytue Mail`, `Sytue Drive`, `Sytue Calendar`, `Sytue Cloud`.
- **The portal** is called exactly **Sytue One** (not "Sytue Portal").
- **The IdP** is called **Sytue ID** (not "Sytue Identity" or "Sytue Auth"): it is the **core**
  piece of the suite and **our own Go service** (OIDC/OAuth2/SCIM/MFA), not a third-party
  engine.
- The brand is never written in lowercase in product text (lowercase `sytue` only
  appears as a technical identifier: domains, _slugs_, resource names, OIDC clients, etc.).

<a id="12-mapa-marca--identificador-técnico"></a>

#### 1.2 Brand → technical identifier map

Each product has a (human) brand name and a stable **slug** in _kebab-case_
(in English) that is reused in domains, OIDC clients, resource names and labels. In v4
each product is a **Go service** in `services/<slug>/` (single module `git.sytue.eu/sytue/one`).

| Brand (human)                | Technical slug   | Slug usage                                                                                                                   |
| ---------------------------- | ---------------- | ---------------------------------------------------------------------------------------------------------------------------- |
| Sytue One                    | `one` / `portal` | subdomain `one`; frontend app `portal`; client `sytue-one`                                                                   |
| Sytue ID                     | `id`             | subdomain `id`; Go service `services/id`; the suite's base client                                                            |
| Sytue Drive                  | `drive`          | subdomain `drive`; Go service `services/drive`; app `drive`; client `sytue-drive`                                            |
| Sytue Docs / Sheets / Slides | `docs`           | subdomain `docs`; Go service `services/docs` (+ collab WS); client `sytue-docs`                                              |
| Sytue Mail                   | `mail`           | subdomain `mail` + `mx`; Go service `services/mail`; client `sytue-mail`                                                     |
| Sytue Calendar               | `calendar`       | subdomain `calendar`; Go service `services/calendar`; client `sytue-calendar`                                                |
| Sytue Contacts               | `contacts`       | subdomain `contacts`; Go service `services/contacts`; client `sytue-contacts`                                                |
| Sytue Rooms                  | `rooms`          | subdomain `rooms`; Go service `services/rooms` (messaging + video calls, + LiveKit SFU at `rooms-sfu`); client `sytue-rooms` |
| Sytue Cloud                  | `cloud`          | subdomain `cloud`; Go service `services/cloud`; app `cloud-dashboard`; client `sytue-cloud` (+ `sytue-cloud-api`)            |
| Sytue Admin                  | `console`        | subdomain `console`; app `console`; client `sytue-console`                                                                   |
| Sytue Hub                    | `hub`            | subdomain `hub`; Go service `services/hub`; client `sytue-hub`                                                               |
| Sytue Mail — mail host       | `mx`             | subdomain `mx` (MX/SMTP/IMAP/JMAP), served by `services/mail`                                                                |
| Public website               | `www`            | apex `sytue.eu` + `www`; app `www`                                                                                           |

> **Consistency:** the slug is the stable piece; from it derive the domain, the OIDC client
> (`sytue-<slug>`), the Go service (`services/<slug>`), the resource name (`sytue-<env>-<slug>`)
> and the label `sytue.eu/product=<slug>`. See §4.
>
> **No engine slugs:** there is **no** longer `office` (OnlyOffice Doc Server), nor Keycloak _realms_
> or _themes_, nor Nextcloud/Mattermost/Stalwart federations. _Sytue Docs_ is
> a Go service with its own collaboration (CRDT/OT over WebSocket), with no separate Document Server.

<a id="13-dominio-raíz-y-entornos"></a>

#### 1.3 Root domain and environments

- **Production domain:** `sytue.eu` (registered and managed in **Scaleway DNS**, EU zone).
- **Development (local) domain:** `*.sytue.localhost` (Docker Compose + Traefik 3 + mkcert).
  Same slug scheme as production → **dev/prod parity** (e.g. `mail.sytue.localhost`
  in dev ⇄ `mail.sytue.eu` in prod).

---

<a id="162-tabla-completa-de-dominios-y-subdominios"></a>

<a id="162-tabla-completa-de-dominios-y-subdominios"></a>

### 16.2 Full table of domains and subdomains

All subdomains hang from `sytue.eu` and resolve (directly or indirectly) to the Scaleway
_ingress_ (Load Balancer → Traefik 3 → cert-manager 1.16 for TLS 1.3). The
backend of each surface is **our own Go service** (not a third-party engine).

<a id="21-superficies-de-producto-suite-sytue-one"></a>

#### 2.1 Product surfaces (Sytue One suite)

| Subdomain (prod)    | Product / function                          | Frontend (UX)                      | Backend (Go service)                                                                        | TLS                                      |
| ------------------- | ------------------------------------------- | ---------------------------------- | ------------------------------------------------------------------------------------------- | ---------------------------------------- |
| `sytue.eu` (apex)   | Public website / marketing                  | Astro + Preact                     | —                                                                                           | cert-manager                             |
| `www.sytue.eu`      | Public website (apex alias)                 | Astro + Preact                     | —                                                                                           | cert-manager                             |
| `one.sytue.eu`      | **Sytue One** — portal/launcher             | Astro + Preact                     | Sytue Hub (`services/hub`)                                                                  | cert-manager                             |
| `id.sytue.eu`       | **Sytue ID** — identity/SSO/MFA/SCIM        | Astro + Preact                     | `services/id` (OIDC/OAuth2/SCIM Go)                                                         | cert-manager                             |
| `drive.sytue.eu`    | **Sytue Drive** — files/sync                | Astro + Preact                     | `services/drive` (WebDAV + REST; PG + S3)                                                   | cert-manager                             |
| `docs.sytue.eu`     | **Sytue Docs/Sheets/Slides**                | Astro + Preact (our own editor)    | `services/docs` (doc model + CRDT/OT collab WS; PG + S3)                                    | cert-manager                             |
| `mail.sytue.eu`     | **Sytue Mail** — mail (HTTP/JMAP)           | Astro + Preact (JMAP)              | `services/mail` (SMTP/IMAP/JMAP Go; PG + S3)                                                | cert-manager                             |
| `calendar.sytue.eu` | **Sytue Calendar** — calendar               | Astro + Preact (CalDAV)            | `services/calendar` (CalDAV + REST; PG)                                                     | cert-manager                             |
| `contacts.sytue.eu` | **Sytue Contacts** — contacts               | Astro + Preact (CardDAV)           | `services/contacts` (CardDAV + REST; PG)                                                    | cert-manager                             |
| `notes.sytue.eu`    | **Sytue Notepads** — notes and notebooks    | Astro + Preact                     | `services/notes` (our own REST; client-side E2E encryption; PG)                             | cert-manager                             |
| `rooms.sytue.eu`    | **Sytue Rooms** — messaging and video calls | Astro + Preact (+ LiveKit web SDK) | `services/rooms` (WS + REST + rooms; PG). The **LiveKit** SFU lives at `rooms-sfu.sytue.eu` | cert-manager                             |
| `cloud.sytue.eu`    | **Sytue Cloud** — PaaS (dashboard)          | Astro + Preact                     | `services/cloud` (Go control plane + PostgREST; PG + S3)                                    | cert-manager                             |
| `admin.sytue.eu`    | **Sytue Admin** — admin/tenants             | Astro + Preact                     | Go admin API (on top of Sytue ID + Hub)                                                     | cert-manager                             |
| `hub.sytue.eu`      | **Sytue Hub** — integration (shell backend) | —                                  | `services/hub` (search/notif/presence/intents; PG)                                          | cert-manager                             |
| `mx.sytue.eu`       | Mail host (MX/SMTP/IMAP/JMAP)               | —                                  | `services/mail` (same binary as `mail.sytue.eu`)                                            | cert-manager (JMAP/web) + SMTP/IMAP cert |

> **Retired hosts (2026-09-11).** `chat.sytue.eu` and `meet.sytue.eu` (301 to
> Rooms) and `console.sytue.eu` (301 to Admin) ceased to exist: their
> redirect, their certificate and their DNS record were removed. Their `redirect_uris` left
> Sytue ID in migration 0041 and their logout destinations in 0059. A 301 that
> nobody follows is one more name to certify and one more surface to watch.

> **Canonical per product host (2026-09-26, ADR 0055).** The address that each surface
> announces (`PUBLIC_URL`, OIDC `redirect_uri`) is that of its product
> SUBDOMAIN with its prefix (`mail.sytue.eu/mail`, id mig. 0099). The
> organization host `<slug>.sytue.eu` —today `corporate.sytue.eu`— keeps
> serving the SAME prefix and its login redirects to the product host; it remains in
> transition until the organization becomes a PATH segment, and it survives
> as a mail domain. The `redirect_uri`s of both hosts are registered
> (0099 and 0054); the guard `platform/pkg/migcheck` requires both.

<a id="22-infraestructura-y-dominios-técnicos"></a>

#### 2.2 Infrastructure and technical domains

| Subdomain (prod)        | Function                                                                                                                                                              |
| ----------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `ingress.sytue.eu`      | Public entry point (Scaleway Load Balancer → Traefik). Target of the `CNAME`s of PaaS custom domains.                                                                 |
| `registry.sytue.eu`     | Alias of Scaleway Container Registry (images of the Go services + PaaS builds).                                                                                       |
| `status.sytue.eu`       | Status/incident page (our own statuspage, Astro).                                                                                                                     |
| `mta-sts.sytue.eu`      | HTTPS endpoint for the Sytue Mail MTA-STS _policy file_ (see §3.4).                                                                                                   |
| `autoconfig.sytue.eu`   | Mail client autoconfiguration (Thunderbird/generic). Served by `services/mail` (`internal/autoconfig`) through the `charts/mail` Ingress (`ingress.extraSubdomains`). |
| `autodiscover.sytue.eu` | Mail autodiscovery (Microsoft clients). Same backend and same Ingress as `autoconfig`.                                                                                |
| `imap.sytue.eu`         | Public IMAPS name announced by autoconfiguration (`IMAP_PUBLIC_HOST`). Alias of `mx` (see §3.4.7).                                                                    |
| `smtp.sytue.eu`         | Public _submission_ name announced by autoconfiguration (`SMTP_PUBLIC_HOST`). Alias of `mx` (see §3.4.7).                                                             |
| `livekit.sytue.eu`      | LiveKit WebSocket signaling (Sytue Rooms media SFU).                                                                                                                  |

<a id="23-dominios-de-ejecución-del-paas-sytue-cloud"></a>

#### 2.3 PaaS runtime domains (Sytue Cloud)

Apps deployed by **customers** on Sytue Cloud live in _wildcard_ zones separate from the
suite's domain, to isolate third-party content from Sytue's:

| Domain pattern                       | Function                                  | Resolution                                   |
| ------------------------------------ | ----------------------------------------- | -------------------------------------------- |
| `<app>.apps.sytue.eu`                | **Production** app of a customer project  | wildcard `*.apps.sytue.eu`                   |
| `<proyecto>-pr-<n>.preview.sytue.eu` | **Preview** per branch/PR (ephemeral)     | wildcard `*.preview.sytue.eu`                |
| `<dominio-propio-del-cliente>`       | Customer's own domain connected to an app | `CNAME` → `ingress.sytue.eu` + automatic TLS |

> Examples: `tienda.apps.sytue.eu` (prod), `tienda-pr-42.preview.sytue.eu` (preview of PR #42),
> `www.cliente.com` → `CNAME ingress.sytue.eu`.

<a id="24-dominio-de-desarrollo-paridad-devprod"></a>

#### 2.4 Development domain (dev/prod parity)

Same scheme under `*.sytue.localhost` via Traefik 3 + mkcert (see ports in §2.5 and in
`docs/CONVENTIONS.md` §6):

```
one.sytue.localhost          id.sytue.localhost           drive.sytue.localhost
docs.sytue.localhost         mail.sytue.localhost         calendar.sytue.localhost
contacts.sytue.localhost     rooms.sytue.localhost        rooms-sfu.sytue.localhost
cloud.sytue.localhost        admin.sytue.localhost      hub.sytue.localhost
mx.sytue.localhost           www.sytue.localhost / sytue.localhost
*.apps.sytue.localhost       *.preview.sytue.localhost
```

> **Removed in v4:** `office.sytue.localhost` (OnlyOffice) no longer exists. _Sytue Docs_ serves
> its collaboration (CRDT/OT WS) on `docs.sytue.localhost` itself.

<a id="25-puertos-de-los-servicios-go-dev--docker-compose"></a>

#### 2.5 Ports of the Go services (dev / docker-compose)

HTTP ports of each **Go service** and of the Astro frontends in dev (domain `*.sytue.localhost`
via Traefik). Source of truth: `docs/CONVENTIONS.md` §6.

| Go service (backend)             | Port                            | Astro frontend (dev) | Port |
| -------------------------------- | ------------------------------- | -------------------- | ---- |
| Sytue ID                         | 8081                            | portal               | 4321 |
| Sytue Drive (+WebDAV)            | 8082                            | console              | 4322 |
| Sytue Docs (+collab WS)          | 8083                            | cloud-dashboard      | 4323 |
| Sytue Rooms (+WS)                | 8085                            | www                  | 4324 |
| Sytue Mail (HTTP/JMAP)           | 8086 (SMTP/IMAP 25/465/587/993) | mail                 | 4325 |
| Sytue Calendar (+CalDAV)         | 8087                            | drive                | 4326 |
| Sytue Contacts (+CardDAV)        | 8088                            | calendar             | 4327 |
| Sytue Notepads                   | 8089                            | contacts             | 4328 |
| Sytue Cloud API                  | 8090                            | (free, was docs)     | 4329 |
| Sytue Cloud builder              | 8091                            | rooms                | 4330 |
| Sytue Hub                        | 8092                            | notes                | 4331 |
| Sytue Integration Gateway        | 8093                            |                      |      |
| Sytue Analytics collector        | 8094                            |                      |      |
| Sytue Analytics API              | 8095                            |                      |      |
| Sytue OCR (internal, no ingress) | 8096                            |                      |      |

| Infra (dev)                        | Port                                  |
| ---------------------------------- | ------------------------------------- |
| Traefik HTTP / HTTPS / dashboard   | 80 / 443 / 8080                       |
| PostgreSQL                         | 5432                                  |
| MinIO Object Storage API / console | 9000 / 9001                           |
| LiveKit (Meet, WebRTC)             | 7880 (WS) / 7881 (TCP) / 7882 UDP mux |
| Sytue Mail SMTP / IMAP             | 25 / 465 / 587 / 993                  |

> State, queues and cache live in **PostgreSQL** (5432). **LiveKit** uses the
> media ports; **Synapse** listens on 8008 inside the cluster and publishes only
> its client routes over HTTPS (§0.2). Its administration API stays internal.

---

<a id="163-plan-dns-para-sytueeu-en-scaleway-dns"></a>

<a id="163-plan-dns-para-sytueeu-en-scaleway-dns"></a>

### 16.3 DNS plan for `sytue.eu` on Scaleway DNS

Declarative management with **Terraform** (`provider scaleway/scaleway ~> 2.x`, module
`infra/terraform/modules/dns`). Zone type: **managed by Scaleway DNS** over the external
domain `sytue.eu`. All records are versioned in the Terraform state (Scaleway S3
backend, EU region).

> **Conventions:**
>
> - The `A`/`AAAA` records point to the Scaleway **Load Balancer** (IPv4 + IPv6).
> - Service subdomains point via `CNAME` to the ingress _host_ (`ingress.sytue.eu`)
>   except the apex and `ingress` (which carry direct `A`/`AAAA` records, because the apex does not allow a `CNAME`).
> - Placeholders: `<LB_IPV4>`, `<LB_IPV6>` = Load Balancer IPs; `<LB_IPV4_MAIL>`,
>   `<LB_IPV6_MAIL>` = public IP(s) of the mail service; `<DKIM_PUBLIC_KEY>` = DKIM public
>   key generated by _Sytue Mail_ (the private one **never** goes in git or in DNS).
> - Default TTL **300 s** (5 min) during deployment; raise to **3600 s** once stable.

<a id="31-registros-base-aaaaa-del-ingress-y-apex"></a>

#### 3.1 Base records (ingress and apex A/AAAA)

| Name              | Type   | Value            | TTL  | Notes                                                                   |
| ----------------- | ------ | ---------------- | ---- | ----------------------------------------------------------------------- |
| `sytue.eu` (apex) | `A`    | `<LB_IPV4>`      | 300  | Public website.                                                         |
| `sytue.eu` (apex) | `AAAA` | `<LB_IPV6>`      | 300  | Public website (IPv6).                                                  |
| `ingress`         | `A`    | `<LB_IPV4>`      | 300  | Cluster entry point.                                                    |
| `ingress`         | `AAAA` | `<LB_IPV6>`      | 300  | Entry point (IPv6).                                                     |
| `mx`              | `A`    | `<LB_IPV4_MAIL>` | 3600 | Public IP of the Sytue Mail node/LB (SMTP/IMAP need a direct A record). |
| `mx`              | `AAAA` | `<LB_IPV6_MAIL>` | 3600 | Mail IPv6.                                                              |

<a id="32-subdominios-de-servicio-cname--ingress"></a>

#### 3.2 Service subdomains (CNAME → ingress)

All web surfaces hang from the same ingress; host-based routing is done by Traefik 3
to the corresponding Go service.

| Name           | Type    | Value               | TTL  |
| -------------- | ------- | ------------------- | ---- |
| `www`          | `CNAME` | `sytue.eu.`         | 3600 |
| `one`          | `CNAME` | `ingress.sytue.eu.` | 300  |
| `id`           | `CNAME` | `ingress.sytue.eu.` | 300  |
| `drive`        | `CNAME` | `ingress.sytue.eu.` | 300  |
| `docs`         | `CNAME` | `ingress.sytue.eu.` | 300  |
| `mail`         | `CNAME` | `ingress.sytue.eu.` | 300  |
| `calendar`     | `CNAME` | `ingress.sytue.eu.` | 300  |
| `contacts`     | `CNAME` | `ingress.sytue.eu.` | 300  |
| `rooms`        | `CNAME` | `ingress.sytue.eu.` | 300  |
| `rooms-sfu`    | `CNAME` | `ingress.sytue.eu.` | 300  |
| `livekit`      | `CNAME` | `ingress.sytue.eu.` | 300  |
| `cloud`        | `CNAME` | `ingress.sytue.eu.` | 300  |
| `console`      | `CNAME` | `ingress.sytue.eu.` | 300  |
| `hub`          | `CNAME` | `ingress.sytue.eu.` | 300  |
| `registry`     | `CNAME` | `ingress.sytue.eu.` | 3600 |
| `status`       | `CNAME` | `ingress.sytue.eu.` | 3600 |
| `mta-sts`      | `CNAME` | `ingress.sytue.eu.` | 3600 |
| `autoconfig`   | `CNAME` | `ingress.sytue.eu.` | 3600 |
| `autodiscover` | `CNAME` | `ingress.sytue.eu.` | 3600 |

> **Apex without CNAME:** `www` points to the apex and the apex uses `A`/`AAAA` (RFC: the apex cannot be a
> `CNAME`). If Scaleway DNS offered apex-level _aliases_, they could be used instead of `A`/`AAAA`.
> **Removed:** there is no longer an `office` record (OnlyOffice is out of the v4 stack).

<a id="33-wildcards-del-paas-apps-y-preview"></a>

#### 3.3 PaaS wildcards (`*.apps` and `*.preview`)

| Name        | Type    | Value               | TTL | Notes                                                                                                                                                 |
| ----------- | ------- | ------------------- | --- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `*.apps`    | `CNAME` | `ingress.sytue.eu.` | 300 | Customers' production apps. Automatic per-host TLS via cert-manager (_wildcard_ certificate `*.apps.sytue.eu` with DNS-01, or per host with HTTP-01). |
| `*.preview` | `CNAME` | `ingress.sytue.eu.` | 300 | Ephemeral previews per PR. _Wildcard_ certificate `*.preview.sytue.eu` (DNS-01 against Scaleway DNS).                                                 |

> **Wildcard TLS:** the `*.apps.sytue.eu` and `*.preview.sytue.eu` certificates are issued with
> **cert-manager** using the **DNS-01** challenge against the Scaleway DNS API (an `Issuer`/`ClusterIssuer`
> with credentials in Scaleway Secret Manager, materialized as a Kubernetes `Secret`
> by the bootstrap — never in git). Customers' own
> domains are validated per host with HTTP-01 when connected.

<a id="34-correo--sytue-mail-mx--spf--dkim--dmarc--mta-sts--tls-rpt"></a>

#### 3.4 Mail — Sytue Mail (MX + SPF + DKIM + DMARC + MTA-STS + TLS-RPT)

The mail domain is `sytue.eu` and the mail host is `mx.sytue.eu`. _Sytue Mail_ is
**our own Go mail server** (clean-room on `emersion/go-smtp`, `go-imap`, `go-message`
— MIT libraries — and our own JMAP): SMTP 25/465/587, IMAP 993, JMAP over HTTP. This section
implements the complete chain of mail authentication and policies required by the contract
(`SPF+DKIM+DMARC+MTA-STS+TLS-RPT`).

##### 3.4.1 MX

| Name       | Type | Priority | Value          | TTL  |
| ---------- | ---- | -------- | -------------- | ---- |
| `sytue.eu` | `MX` | `10`     | `mx.sytue.eu.` | 3600 |

> A single MX at `mx.sytue.eu` (Sytue Mail). A second backup MX in the DR region (`nl-ams`)
> would be added as `MX 20 mx2.sytue.eu.` if mail high availability is enabled.

<a id="342-spf-autoriza-al-mx-a-enviar-rechaza-el-resto"></a>

##### 3.4.2 SPF (authorizes the MX to send; rejects everything else)

| Name       | Type  | Value              | TTL  |
| ---------- | ----- | ------------------ | ---- |
| `sytue.eu` | `TXT` | `"v=spf1 mx -all"` | 3600 |

> `mx` authorizes the hosts of the MX record; `-all` (hardfail) rejects any other origin.
> If mail is later sent from another EU IP/service, it is added with `ip4:`/`ip6:`/`include:`
> before `-all`.

<a id="343-dkim-firma-criptográfica-clave-pública-en-dns-privada-en-sytue-mail"></a>

##### 3.4.3 DKIM (cryptographic signature; public key in DNS, private key in Sytue Mail)

Recommended selector: `sytue` (rotatable to `sytue2`, `sytue3`… when rotating keys). _Sytue Mail_
generates the key pair; **only the public key** is published in DNS.

| Name               | Type  | Value                                   | TTL  |
| ------------------ | ----- | --------------------------------------- | ---- |
| `sytue._domainkey` | `TXT` | `"v=DKIM1; k=rsa; p=<DKIM_PUBLIC_KEY>"` | 3600 |

> Placeholder `<DKIM_PUBLIC_KEY>`: public RSA key (base64) issued by _Sytue Mail_. The private
> key remains in the service's secrets store (Secret Manager + Sealed Secret);
> **never** in git or in DNS. If the value exceeds 255 characters, it is split into concatenated
> _strings_ within the same `TXT`. Ed25519 (`k=ed25519`) is a valid alternative.

<a id="344-dmarc-política-de-alineación--reportes"></a>

##### 3.4.4 DMARC (alignment policy + reports)

| Name     | Type  | Value                                                                                                                          | TTL  |
| -------- | ----- | ------------------------------------------------------------------------------------------------------------------------------ | ---- |
| `_dmarc` | `TXT` | `"v=DMARC1; p=reject; rua=mailto:dmarc-reports@sytue.eu; ruf=mailto:dmarc-forensics@sytue.eu; fo=1; adkim=s; aspf=s; pct=100"` | 3600 |

> `p=reject` rejects non-aligned mail; `adkim=s`/`aspf=s` require strict alignment;
> `rua`/`ruf` receive the aggregate/forensic reports in _Sytue Mail_ mailboxes.
> **Progressive rollout:** start with `p=none` (monitoring only), move to `p=quarantine`
> and finally to `p=reject` once SPF/DKIM have been verified in the `rua` reports.

<a id="345-mta-sts-fuerza-tls-entre-servidores-de-correo"></a>

##### 3.4.5 MTA-STS (enforces TLS between mail servers)

Two parts: a DNS `TXT` record that announces the policy and a policy file served over
**HTTPS** at `https://mta-sts.sytue.eu/.well-known/mta-sts.txt`.

| Name       | Type  | Value                            | TTL  |
| ---------- | ----- | -------------------------------- | ---- |
| `_mta-sts` | `TXT` | `"v=STSv1; id=20260630T000000Z"` | 3600 |

> The `id` field changes every time the policy file is updated (format `<fechaZ>`).
> The subdomain `mta-sts.sytue.eu` is in §2.2/§3.2 (CNAME → ingress) and serves, via Traefik, the
> static file generated by _Sytue Mail_:
>
> ```text
> # https://mta-sts.sytue.eu/.well-known/mta-sts.txt
> version: STSv1
> mode: enforce
> mx: mx.sytue.eu
> max_age: 604800
> ```
>
> `mode: enforce` rejects delivery if there is no valid TLS. During rollout
> `mode: testing` can be used to receive reports without blocking.

<a id="346-tls-rpt-reportes-de-fallos-de-tls-de-transporte"></a>

##### 3.4.6 TLS-RPT (transport TLS failure reports)

| Name         | Type  | Value                                           | TTL  |
| ------------ | ----- | ----------------------------------------------- | ---- |
| `_smtp._tls` | `TXT` | `"v=TLSRPTv1; rua=mailto:tls-reports@sytue.eu"` | 3600 |

> Remote servers send TLS negotiation error reports to `tls-reports@sytue.eu`
> (a _Sytue Mail_ mailbox), complementing MTA-STS.

<a id="347-autodescubrimiento-de-clientes-nativos-correo-calendario-y-contactos"></a>

##### 3.4.7 Native client autodiscovery (mail, calendar and contacts)

**The goal:** that users type **only their email address and password** into
Apple Mail, Outlook, Thunderbird or DAVx5 and everything else resolves itself. This is achieved by
two pieces: the HTTP _endpoints_ the Go services already serve (next §) and **these
DNS records**. Without the records, the endpoints exist but nobody finds them.

Endpoints backing this section (already deployed; see `docs/architecture.md#5-capa-de-compatibilidad-entrante`):

| Endpoint                                                                | Service             | Queried by                         |
| ----------------------------------------------------------------------- | ------------------- | ---------------------------------- |
| `/.well-known/autoconfig/mail/config-v1.1.xml`                          | `services/mail`     | Thunderbird and compatible clients |
| `/mail/config-v1.1.xml?emailaddress=…`                                  | `services/mail`     | Thunderbird (host `autoconfig.`)   |
| `/autodiscover/autodiscover.xml` (and `/Autodiscover/Autodiscover.xml`) | `services/mail`     | Outlook (POX Autodiscover)         |
| `/.well-known/caldav` → `301` to `/dav/`                                | `services/calendar` | Apple Calendar, Thunderbird, DAVx5 |
| `/.well-known/carddav` → `301` to `/dav/`                               | `services/contacts` | Apple Contacts, DAVx5              |
| `/account/apple.mobileconfig` (with a session)                          | `services/id`       | iOS/macOS (configuration profile)  |

<a id="a-imprescindibles"></a>

###### A. Essential

Without these records autodiscovery **does not work** (the names the server itself
announces would not resolve, or the client would not find where to ask).

| Name           | Type    | Value               | TTL  | Purpose / who uses it                                                                                                                                          |
| -------------- | ------- | ------------------- | ---- | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `imap`         | `CNAME` | `mx.sytue.eu.`      | 3600 | Incoming server name that Sytue Mail **announces** (`IMAP_PUBLIC_HOST`, default `imap.<dominio>`) and that the Apple profile carries inside. All IMAP clients. |
| `smtp`         | `CNAME` | `mx.sytue.eu.`      | 3600 | Ditto for outgoing (`SMTP_PUBLIC_HOST`, default `smtp.<dominio>`). All SMTP clients.                                                                           |
| `autoconfig`   | `CNAME` | `ingress.sytue.eu.` | 3600 | **Thunderbird**: queries `https://autoconfig.<dominio>/mail/config-v1.1.xml?emailaddress=…` before falling back to the Mozilla database.                       |
| `autodiscover` | `CNAME` | `ingress.sytue.eu.` | 3600 | **Outlook**: `https://autodiscover.<dominio>/autodiscover/autodiscover.xml`.                                                                                   |
| `calendar`     | `CNAME` | `ingress.sytue.eu.` | 300  | CalDAV discovery target (already in §3.2; repeated for its role here).                                                                                         |
| `contacts`     | `CNAME` | `ingress.sytue.eu.` | 300  | CardDAV discovery target (already in §3.2).                                                                                                                    |

> `imap`/`smtp` point to `mx` (the mail IP), **not** to the ingress: IMAP 993 and submission 465
> do not go through Traefik. `autoconfig`/`autodiscover` do point to the ingress: they are HTTPS and are
> handled by `services/mail` (the `charts/mail` Ingress, `ingress.extraSubdomains`).

<a id="b-opcionales-recomendados-acortan-o-rescatan-el-descubrimiento"></a>

###### B. Optional (recommended: they shorten or rescue discovery)

SRV format: `<prioridad> <peso> <puerto> <destino>` (priority, weight, port, target). Lower priority wins; weight distributes
among equals.

| Name                 | Type  | Value (prio weight port target)  | TTL  | Purpose / who uses it                                                                                                                                 |
| -------------------- | ----- | -------------------------------- | ---- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `_imaps._tcp`        | `SRV` | `0 1 993 mx.sytue.eu.`           | 3600 | IMAP over implicit TLS (RFC 6186). Thunderbird and clients that do SRV.                                                                               |
| `_submissions._tcp`  | `SRV` | `0 1 465 mx.sytue.eu.`           | 3600 | Submission with implicit TLS (RFC 8314). **It is what Sytue Mail announces** in its autoconfiguration.                                                |
| `_submission._tcp`   | `SRV` | `0 1 587 mx.sytue.eu.`           | 3600 | Submission with STARTTLS. Publish **only** if the deployment serves 587 with a certificate; Sytue Mail does not announce it in the autoconfiguration. |
| `_autodiscover._tcp` | `SRV` | `0 0 443 autodiscover.sytue.eu.` | 3600 | **Outlook**: last resort if the HTTPS queries fail.                                                                                                   |
| `_caldavs._tcp`      | `SRV` | `0 1 443 calendar.sytue.eu.`     | 3600 | **CalDAV** (RFC 6764): allows discovering the calendar from the mail domain (Apple, DAVx5, Thunderbird).                                              |
| `_carddavs._tcp`     | `SRV` | `0 1 443 contacts.sytue.eu.`     | 3600 | **CardDAV** (RFC 6764): ditto for contacts.                                                                                                           |
| `_caldavs._tcp`      | `TXT` | `"path=/dav/"`                   | 3600 | DAV context path (RFC 6764 §6). Avoids depending on the well-known `301`.                                                                             |
| `_carddavs._tcp`     | `TXT` | `"path=/dav/"`                   | 3600 | Ditto for CardDAV.                                                                                                                                    |
| `_jmap._tcp`         | `SRV` | `0 1 443 mx.sytue.eu.`           | 3600 | JMAP discovery (JMAP clients; the webmail does not need it).                                                                                          |

> **`_caldavs`/`_carddavs` point to the product host** (`calendar.`/`contacts.`), not to `mx`:
> that is where the Ingress routes `/.well-known/caldav` and `/dav` to the Go service
> (`charts/calendar` and `charts/contacts`, `ingress.paths`).

<a id="c-lo-que-no-cubrimos-honestidad-de-alcance"></a>

###### C. What we do NOT cover (honesty about scope)

- **The apex** (`https://sytue.eu/.well-known/autoconfig/…` and `https://sytue.eu/autodiscover/…`)
  is **not** routed to `services/mail`. Both clients try it (Outlook, in fact, before
  `autodiscover.`) and move on when it does not answer, so the configuration ends up
  coming from `autoconfig.`/`autodiscover.`. Covering the apex would require its own rule in the
  `charts/mail` Ingress (`extraSubdomains` composes `<label>.<dominio>`, which does not work for the
  apex) and the mail certificate including the apex: it has been left out on purpose.
- **Exchange / ActiveSync**: not implemented. Outlook is configured as an **IMAP/SMTP** account
  via POX Autodiscover; there is no `Exchange Web Services` or EAS synchronization.
- **DAVx5 and Android clients** use the `_caldavs`/`_carddavs` records or directly the URL
  `https://calendar.<dominio>/dav/` — there is no proprietary Google discovery.

<a id="348-caa-restringe-qué-ca-pueden-emitir-certificados--buena-práctica"></a>

##### 3.4.8 CAA (restricts which CAs may issue certificates — good practice)

| Name       | Type  | Value                                | TTL  | Notes                                       |
| ---------- | ----- | ------------------------------------ | ---- | ------------------------------------------- |
| `sytue.eu` | `CAA` | `0 issue "letsencrypt.org"`          | 3600 | Let's Encrypt only (used by cert-manager).  |
| `sytue.eu` | `CAA` | `0 issuewild "letsencrypt.org"`      | 3600 | Allows _wildcards_ (PaaS) from the same CA. |
| `sytue.eu` | `CAA` | `0 iodef "mailto:security@sytue.eu"` | 3600 | Notification of unauthorized issuances.     |

<a id="35-resumen-del-fichero-de-zona-referencia"></a>

#### 3.5 Zone file summary (reference)

```dns
; ----- Base / ingress -----
sytue.eu.            300   IN A     <LB_IPV4>
sytue.eu.            300   IN AAAA  <LB_IPV6>
ingress.sytue.eu.    300   IN A     <LB_IPV4>
ingress.sytue.eu.    300   IN AAAA  <LB_IPV6>

; ----- Superficies de producto (CNAME → ingress → servicio Go) -----
www.sytue.eu.        3600  IN CNAME sytue.eu.
one.sytue.eu.        300   IN CNAME ingress.sytue.eu.
id.sytue.eu.         300   IN CNAME ingress.sytue.eu.
drive.sytue.eu.      300   IN CNAME ingress.sytue.eu.
docs.sytue.eu.       300   IN CNAME ingress.sytue.eu.
mail.sytue.eu.       300   IN CNAME ingress.sytue.eu.
calendar.sytue.eu.   300   IN CNAME ingress.sytue.eu.
contacts.sytue.eu.   300   IN CNAME ingress.sytue.eu.
rooms.sytue.eu.      300   IN CNAME ingress.sytue.eu.
rooms-sfu.sytue.eu.  300   IN CNAME ingress.sytue.eu.
livekit.sytue.eu.    300   IN CNAME ingress.sytue.eu.
cloud.sytue.eu.      300   IN CNAME ingress.sytue.eu.
admin.sytue.eu.    300   IN CNAME ingress.sytue.eu.
hub.sytue.eu.        300   IN CNAME ingress.sytue.eu.

; ----- Técnicos -----
registry.sytue.eu.   3600  IN CNAME ingress.sytue.eu.
status.sytue.eu.     3600  IN CNAME ingress.sytue.eu.
mta-sts.sytue.eu.    3600  IN CNAME ingress.sytue.eu.
autoconfig.sytue.eu. 3600  IN CNAME ingress.sytue.eu.
autodiscover.sytue.eu. 3600 IN CNAME ingress.sytue.eu.
; imap./smtp. van al CORREO (mx), no al ingress: 993/465 no pasan por Traefik.
imap.sytue.eu.       3600  IN CNAME mx.sytue.eu.
smtp.sytue.eu.       3600  IN CNAME mx.sytue.eu.

; ----- PaaS wildcards -----
*.apps.sytue.eu.     300   IN CNAME ingress.sytue.eu.
*.preview.sytue.eu.  300   IN CNAME ingress.sytue.eu.

; ----- Correo (Sytue Mail) -----
mx.sytue.eu.         3600  IN A     <LB_IPV4_MAIL>
mx.sytue.eu.         3600  IN AAAA  <LB_IPV6_MAIL>
sytue.eu.            3600  IN MX 10 mx.sytue.eu.
sytue.eu.            3600  IN TXT   "v=spf1 mx -all"
sytue._domainkey.sytue.eu. 3600 IN TXT "v=DKIM1; k=rsa; p=<DKIM_PUBLIC_KEY>"
_dmarc.sytue.eu.     3600  IN TXT   "v=DMARC1; p=reject; rua=mailto:dmarc-reports@sytue.eu; ruf=mailto:dmarc-forensics@sytue.eu; fo=1; adkim=s; aspf=s; pct=100"
_mta-sts.sytue.eu.   3600  IN TXT   "v=STSv1; id=20260630T000000Z"
_smtp._tls.sytue.eu. 3600  IN TXT   "v=TLSRPTv1; rua=mailto:tls-reports@sytue.eu"

; ----- Descubrimiento de clientes nativos (SRV, ver §3.4.7) -----
_imaps._tcp.sytue.eu.        3600 IN SRV 0 1 993 mx.sytue.eu.
_submissions._tcp.sytue.eu.  3600 IN SRV 0 1 465 mx.sytue.eu.
_submission._tcp.sytue.eu.   3600 IN SRV 0 1 587 mx.sytue.eu.   ; solo si se sirve el 587
_autodiscover._tcp.sytue.eu. 3600 IN SRV 0 0 443 autodiscover.sytue.eu.
_caldavs._tcp.sytue.eu.      3600 IN SRV 0 1 443 calendar.sytue.eu.
_carddavs._tcp.sytue.eu.     3600 IN SRV 0 1 443 contacts.sytue.eu.
_caldavs._tcp.sytue.eu.      3600 IN TXT "path=/dav/"
_carddavs._tcp.sytue.eu.     3600 IN TXT "path=/dav/"
_jmap._tcp.sytue.eu.         3600 IN SRV 0 1 443 mx.sytue.eu.

; ----- CAA -----
sytue.eu.            3600  IN CAA 0 issue "letsencrypt.org"
sytue.eu.            3600  IN CAA 0 issuewild "letsencrypt.org"
sytue.eu.            3600  IN CAA 0 iodef "mailto:security@sytue.eu"
```

> This file is a **reference**; the actual declarative source is Terraform
> (`scaleway_domain_record` in `infra/terraform/modules/dns`). The placeholders are resolved
> from the _outputs_ of the `loadbalancer`/`network` modules and from secrets (DKIM) in the store.

---

<a id="164-convencion-de-naming-de-recursos-cloud-y-labels-de-kubernetes"></a>

<a id="164-convención-de-naming-de-recursos-cloud-y-labels-de-kubernetes"></a>

### 16.4 Naming convention for cloud resources and Kubernetes labels

<a id="41-variables-comunes"></a>

#### 4.1 Common variables

| Variable    | Values                             | Description                                                 |
| ----------- | ---------------------------------- | ----------------------------------------------------------- |
| `<env>`     | `dev` · `staging` · `prod`         | Environment (dev = local Compose; staging/prod = Scaleway). |
| `<service>` | product/resource slug (§1.2)       | e.g. `id`, `drive`, `mail`, `hub`, `pg`.                    |
| `<tenant>`  | organization slug in _kebab-case_  | e.g. `acme`, `cliente-uno`.                                 |
| `<region>`  | `fr-par` (primary) · `nl-ams` (DR) | Scaleway EU regions only.                                   |

> In v4 the `<service>` slugs are **Go services** (`id`, `drive`, `docs`, `mail`, `calendar`,
> `contacts`, `rooms`, `cloud`, `hub`) plus infrastructure resources (`pg`, etc.).
> There are **no** longer engine slugs (`keycloak`, `nextcloud`, `mattermost`, `stalwart`,
> `onlyoffice`) nor `valkey`.

<a id="42-nombres-de-recursos-cloud-scaleway--terraform"></a>

#### 4.2 Cloud resource names (Scaleway / Terraform)

Canonical pattern: **`sytue-<env>-<service>`** (all lowercase, _kebab-case_). It applies to
Kapsule, RDB, Object Storage, Registry, Load Balancer, Secret Manager, DNS, networks, etc.

| Scaleway resource              | Name pattern                         | Examples                                                                                                                                        |
| ------------------------------ | ------------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| Kapsule cluster                | `sytue-<env>-k8s`                    | `sytue-prod-k8s`, `sytue-staging-k8s`                                                                                                           |
| Kapsule node pool              | `sytue-<env>-pool-<perfil>`          | `sytue-prod-pool-platform`, `sytue-prod-pool-workload`, `sytue-prod-pool-media`                                                                 |
| RDB database (PostgreSQL)      | `sytue-<env>-pg-<rol>`               | `sytue-prod-pg-suite`, `sytue-prod-pg-paas`                                                                                                     |
| Object Storage bucket (S3)     | `sytue-<env>-<purpose>`              | `sytue-prod-drive-blobs`, `sytue-prod-mail-blobs`, `sytue-prod-docs-blobs`, `sytue-prod-backups`, `sytue-prod-tfstate`, `sytue-prod-paas-cache` |
| Load Balancer                  | `sytue-<env>-lb`                     | `sytue-prod-lb`                                                                                                                                 |
| Private Network (VPC)          | `sytue-<env>-net`                    | `sytue-prod-net`                                                                                                                                |
| Container Registry (namespace) | `sytue-<env>-registry`               | `sytue-prod-registry`                                                                                                                           |
| Secret (Secret Manager)        | `sytue-<env>-<service>-<clave>`      | `sytue-prod-id-signing-key`, `sytue-prod-mail-dkim`, `sytue-prod-livekit-apikey`                                                                |
| DNS zone                       | `sytue.eu` (external, no env prefix) | —                                                                                                                                               |

> **No Valkey:** the `sytue-<env>-valkey-<service>` pattern does not exist (removed in v4; state,
> queues and cache live in **PostgreSQL**). **Per-product buckets:** each Go service that stores
> blobs uses its own bucket/prefix (`sytue-<env>-drive-blobs`, `-mail-blobs`, `-docs-blobs`).
> **PaaS buckets per project/tenant:** `sytue-<env>-paas-<tenant>-<proyecto>` (or per-project S3 prefixes
> inside the PaaS storage bucket). Multi-tenant PaaS resources
> also carry the tenant slug: `sytue-<env>-pg-<tenant>-<proyecto>`.

<a id="43-namespaces-de-kubernetes-kapsule"></a>

#### 4.3 Kubernetes namespaces (Kapsule)

| Type                       | Pattern           | Examples                                                  |
| -------------------------- | ----------------- | --------------------------------------------------------- |
| Platform (Sytue One suite) | `sytue-system`    | `sytue-system` (Go services, ingress, operators, LiveKit) |
| Per product (optional)     | `sytue-<service>` | `sytue-mail`, `sytue-rooms`, `sytue-cloud`                |
| Per tenant (multi-tenant)  | `tenant-<slug>`   | `tenant-acme`, `tenant-cliente-uno`                       |

> Each `tenant-<slug>` namespace carries **ResourceQuota + NetworkPolicy** (isolation), mapped
> 1:1 to the _Sytue ID_ group `org/<slug>` (see `docs/CONVENTIONS.md` §9). The platform
> namespace is `sytue-system`.

<a id="44-nombres-de-releases-helm-y-recursos-k8s"></a>

#### 4.4 Helm release names and k8s resources

- **Umbrella chart:** release `sytue-one` in `sytue-system` (only Go services + LiveKit + Traefik).
- **Subcharts/releases per Go service:** `sytue-<service>` (e.g. `sytue-id`, `sytue-drive`,
  `sytue-docs`, `sytue-mail`, `sytue-calendar`, `sytue-contacts`, `sytue-rooms`,
  `sytue-cloud`, `sytue-hub`). Third-party component: `sytue-livekit`.
- Derived resources (Deployment/Service/Ingress/PVC) inherit the release name +
  the standard Helm suffix (`sytue-<service>-<componente>`).

> **No engine releases:** `sytue-keycloak`, `sytue-nextcloud`,
> `sytue-mattermost`, `sytue-stalwart`, `sytue-onlyoffice` no longer exist.

<a id="45-labels-y-anotaciones-de-kubernetes"></a>

#### 4.5 Kubernetes labels and annotations

**Mandatory** labels on every object of the suite (standard `app.kubernetes.io/*` + our own
`sytue.eu/*` domain):

| Label                          | Value                                         | Required    | Example                                       |
| ------------------------------ | --------------------------------------------- | ----------- | --------------------------------------------- |
| `app.kubernetes.io/part-of`    | `sytue-one`                                   | Yes         | `sytue-one`                                   |
| `app.kubernetes.io/name`       | service slug                                  | Yes         | `mail`, `id`, `hub`                           |
| `app.kubernetes.io/instance`   | Helm release name                             | Yes         | `sytue-mail`                                  |
| `app.kubernetes.io/component`  | component role                                | Yes         | `frontend`, `api`, `worker`, `ingress`, `sfu` |
| `app.kubernetes.io/managed-by` | `Helm` (or `Terraform`)                       | Yes         | `Helm`                                        |
| `app.kubernetes.io/version`    | component version                             | Recommended | `1.0.0`                                       |
| `sytue.eu/env`                 | `<env>`                                       | Yes         | `prod`, `staging`, `dev`                      |
| `sytue.eu/stack`               | `go` (backend) / `astro` (frontend)           | Recommended | `go`                                          |
| `sytue.eu/product`             | product slug (§1.2)                           | Yes         | `mail`, `drive`, `cloud`                      |
| `sytue.eu/tenant`              | tenant `<slug>` (multi-tenant workloads only) | Conditional | `acme` (absent on the platform)               |

Example labels block (Deployment of the _Sytue Mail_ Go service in prod):

```yaml
metadata:
  labels:
    app.kubernetes.io/part-of: sytue-one
    app.kubernetes.io/name: mail
    app.kubernetes.io/instance: sytue-mail
    app.kubernetes.io/component: api
    app.kubernetes.io/managed-by: Helm
    app.kubernetes.io/version: "1.0.0"
    sytue.eu/env: prod
    sytue.eu/stack: go
    sytue.eu/product: mail
```

<a id="46-tags-de-recursos-scaleway"></a>

#### 4.6 Scaleway resource tags

Scaleway resources carry equivalent _tags_ (Scaleway uses simple `clave=valor` (key=value) tags
as strings), generated by Terraform:

```hcl
tags = [
  "part-of=sytue-one",
  "env=${var.env}",            # dev | staging | prod
  "product=${var.product}",    # mail | drive | hub | ...
  "stack=go",                  # backend Go (frontend usa stack=astro)
  "managed-by=terraform",
  "region=${var.region}",      # fr-par | nl-ams
]
```

<a id="47-resumen-de-coherencia-slug--recurso"></a>

#### 4.7 Slug → resource consistency summary

For a product with slug `<service>` in environment `<env>`:

| Plane                | Derived value                                                                |
| -------------------- | ---------------------------------------------------------------------------- |
| Subdomain            | `<service>.sytue.eu`                                                         |
| Go service           | `services/<service>/cmd/<service>/main.go` (module `git.sytue.eu/sytue/one`) |
| OIDC client          | `sytue-<service>` (Sytue Cloud adds `sytue-cloud-api`)                       |
| Scaleway resource    | `sytue-<env>-<service>` (RDB uses `-pg-<rol>`)                               |
| Helm release         | `sytue-<service>`                                                            |
| Namespace (optional) | `sytue-<service>` (platform) / `tenant-<slug>` (tenant)                      |
| Label `name`         | `<service>`                                                                  |
| Label `product`      | `<service>`                                                                  |
| S3 bucket            | `sytue-<env>-<purpose>` (e.g. `<service>-blobs`)                             |

<a id="pages-convenciones"></a>

<a id="convenciones-específicas-de-sytue-web"></a>

## Sytue Web-specific conventions

The Sytue Web IR is versioned wire format: keys are `snake_case`, incompatible changes require
another version and the Go and TypeScript mirrors are updated in the same delivery. The editor does not serialize
or normalize someone else's draft; it stores the validated IR and preserves optimistic concurrency. An
AI translation is a reviewable patch to the draft, never an automatic publication.

Publications and exports must use the public `/_pages/v1` contract; no per-target API is replicated or
invented. Any published resource is validated against the allowlist and
islands are generated in the common base. Domains are verified with TXT and are not accepted as an arbitrary
output URL; HTTPS checks use the fixed target authorized by configuration.
