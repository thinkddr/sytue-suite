<a id="sytue-one--sitio-público-www"></a>
# Sytue One — public website (`www`)

Marketing landing page for **Sytue One** (`sytue.eu` / `www.sytue.eu`): it presents the
**European sovereign** productivity suite, native and integrated with Sytue ID.
**Astro 7 + Preact 10**, **static (SSG)**, **Tailwind v4** and brand tokens.
Uses **only** `@sytue/ui`. **Proprietary** code (`MIT`).

> Contract: [`docs/CONVENTIONS.md`](../../../../docs/CONVENTIONS.md) §4, §5.
> Dev port: **4324**. No authentication (public site).

<a id="idiomas-y-rutas"></a>
## Languages and routes

The **26 product languages** (`docs/CONVENTIONS.md` §13.1), with **English
as the base and without a prefix**: `/` is a real page, not a redirect, which is
what the crawler and `x-default` want.

| Language | Home    | Rest                                                 |
| -------- | ------- | ---------------------------------------------------- |
| English  | `/`     | `/downloads`, `/privacy`, `/terms`, `/product/<key>` |
| Spanish  | `/es`   | `/es/downloads`, …                                   |
| French   | `/fr`   | `/fr/downloads`, …                                   |
| German   | `/de`   | `/de/downloads`, …                                   |
| Italian  | `/it`   | `/it/downloads`, …                                   |

A single template per route, in `src/pages/[...locale]/`: the **rest** parameter
also matches zero segments, so the same file generates `/` and `/es/…`.
**Slugs are in English in every language** (`/es/downloads`, not
`/es/descargas`): translating them requires a per-language route table on every page
and in the switcher, and with reciprocal `hreflang` Google already understands that they are the same
page in another language. The **404** stays outside the scheme because the static
server serves a single `404.html` for everything.

The Spanish URLs from the previous stage (`/descargas`, `/privacidad`,
`/terminos`, `/producto/*`, `/en`) **redirect** to their equivalent
(`redirects` in `astro.config.mjs`).

All text comes from the catalog in `src/i18n/messages/` (English = source of
truth; the other 25 derive their type with `AppCatalog<typeof en>`). The
**product names** —Sytue, Mail, Drive, Docs, Rooms, Cloud— are brand names and are not
translated.

<a id="contenido"></a>
## Content

- **Hero** — value proposition (sovereignty + open standards + integration).
- **Products** (`#products`) — grid generated from the canonical catalog (`suiteApps`), with a link to each product's page.
- **Integration** — single sign-on, shared shell and cross-app actions.
- **Sovereignty** (`#sovereignty`) — 100 % EU, GDPR, no lock-in.
- **Open standards** (`#standards`) — native products on open protocols (OIDC, JMAP, CalDAV, CardDAV, WebDAV).
- **CTA** + **404** + `robots.txt`.

Section `id`s are the same in every language: an anchor is an
identifier, not shop-window text.

The sign-in entry point (`Empezar`) links to the **Sytue One** login (`one.sytue.eu/auth/login`).

<a id="build-estático-y-runtime"></a>
## Static build and runtime

`astro build` generates `dist/`. The production image serves it with `serve.mjs`
(a dependency-free Bun static server, with cache headers and a fallback to
`404.html`). It runs on Scaleway/Kapsule (EU).

<a id="seo-y-rendimiento-contrato-08"></a>
## SEO and performance (contract §0.8)

**Public indexable** surface: this is where SEO is concentrated (the authenticated
apps are `noindex`). Strategy and budgets in
[`docs/operations.md#4-rendimiento-y-seo`](../../../../docs/operations.md#4-rendimiento-y-seo).

- **SSG (`output: 'static'`)** — pre-rendered HTML: minimal TTFB and green CWV.
- **Semantic HTML** — `header`/`main`/`nav`/`footer`, heading hierarchy,
  `skip-link` to the content and `aria-label` on the navigation, all translated.
- **Metadata + OpenGraph + Twitter Cards** — in `MarketingLayout.astro`
  (title, description, `og:*`, `twitter:card=summary_large_image`).
- **JSON-LD structured data** (`src/lib/seo.ts`) — schema.org
  `Organization`, `WebSite` and `SoftwareApplication`, injected into the home page.
- **`sitemap.xml`** — `@astrojs/sitemap` generates `sitemap-index.xml` +
  `sitemap-0.xml` (with `hreflang`); excludes `/404`.
- **`robots.txt`** (`public/robots.txt`) — allows the public pages, blocks utility
  routes (`/api/`, `/auth/`, previews) and points to the sitemap.
- **Canonical + hreflang** — each page's canonical is in **its own language**, with
  reciprocal alternates in the 26 languages (`en-GB`, `de-DE`, `pl-PL`…) +
  `x-default` → English, plus `og:locale` and `og:locale:alternate`.
  `hreflangAlternates(path, locales)` **requires** the list of languages in which the
  page exists: declaring one without a translation sends the crawler to a 404, so
  the function cannot assert it on its own.
- **Language switcher** — changes language **without losing the page**
  (`/es/privacy` → `/fr/privacy`) and names each language in its own language.
- **AVIF/WebP images** — `astro:assets` (Sharp) serves modern formats with
  explicit dimensions (no CLS) and `loading="lazy"`.
- **Bundle budget** — `vite.build.chunkSizeWarningLimit: 100` (kB);
  islands with minimal JS (`client:visible`/`idle`).
- **Security headers** — `serve.mjs` emits a strict CSP, HSTS, `nosniff`,
  `X-Frame-Options: DENY` and cache-control per resource type.

<a id="patrón-noindex-para-superficies-autenticadas-todas-las-apps"></a>
### `noindex` pattern for authenticated surfaces (all apps)

The contract requires that **authenticated surfaces are NOT indexed**. This site
supports per-page `noindex` via the layout prop:

```astro
<MarketingLayout locale={locale} path="/…" title="…" description="…" noindex={true}>
  …
</MarketingLayout>
```

which emits `<meta name="robots" content="noindex, nofollow">`. The **authenticated
apps** (`portal`, `mail`, `drive`, …) must apply the **same pattern in
their base layout, always set to `noindex`** (never conditional), since they serve
SSR with a session. In addition, their server/ingress must emit the
`X-Robots-Tag: noindex, nofollow` header to protect non-HTML responses. They do not need
`sitemap.xml` or structured data: SEO is concentrated in `www`.

## Scripts

```bash
bun run --filter @sytue/app-www dev      # http://localhost:4324
bun run --filter @sytue/app-www build
bun run --filter @sytue/app-www typecheck
```
