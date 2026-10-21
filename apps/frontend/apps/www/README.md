# Sytue One — sitio público (`www`)

Landing de marketing de **Sytue One** (`sytue.eu` / `www.sytue.eu`): comunica la
suite de productividad **soberana europea**, nativa e integrada con Sytue ID.
**Astro 7 + Preact 10**, **estático (SSG)**, **Tailwind v4** y tokens de marca.
Usa **solo** `@sytue/ui`. Código **propietario** (`MIT`).

> Contrato: [`docs/CONVENTIONS.md`](../../../../docs/CONVENTIONS.md) §4, §5.
> Puerto dev: **4324**. Sin autenticación (sitio público).

## Idiomas y rutas

Los **26 idiomas de producto** (`docs/CONVENTIONS.md` §13.1) con el **inglés
de base y sin prefijo**: `/` es una página real, no una redirección, que es lo
que quieren el rastreador y el `x-default`.

| Idioma   | Portada | Resto                                                |
| -------- | ------- | ---------------------------------------------------- |
| inglés   | `/`     | `/downloads`, `/privacy`, `/terms`, `/product/<key>` |
| español  | `/es`   | `/es/downloads`, …                                   |
| francés  | `/fr`   | `/fr/downloads`, …                                   |
| alemán   | `/de`   | `/de/downloads`, …                                   |
| italiano | `/it`   | `/it/downloads`, …                                   |

Una sola plantilla por ruta, en `src/pages/[...locale]/`: el parámetro **rest**
casa también con cero segmentos, así que el mismo fichero genera `/` y `/es/…`.
Los **slugs van en inglés en todos los idiomas** (`/es/downloads`, no
`/es/descargas`): traducirlos exige una tabla de rutas por idioma en cada página
y en el selector, y con `hreflang` recíproco Google ya entiende que son la misma
página en otra lengua. El **404** queda fuera del esquema porque el servidor
estático sirve un único `404.html` para todo.

Las URLs españolas de la etapa anterior (`/descargas`, `/privacidad`,
`/terminos`, `/producto/*`, `/en`) **redirigen** a su equivalente
(`redirects` en `astro.config.mjs`).

Todo el texto sale del catálogo de `src/i18n/messages/` (inglés = fuente de
verdad; los otros 25 derivan su tipo con `AppCatalog<typeof en>`). Los
**nombres de producto** —Sytue, Mail, Drive, Docs, Rooms, Cloud— son marca y no
se traducen.

## Contenido

- **Hero** — propuesta de valor (soberanía + estándares abiertos + integración).
- **Productos** (`#products`) — rejilla generada desde el catálogo canónico (`suiteApps`), con enlace a la ficha de cada producto.
- **Integración** — login único, shell común y acciones cruzadas.
- **Soberanía** (`#sovereignty`) — 100 % UE, RGPD, sin lock-in.
- **Estándares abiertos** (`#standards`) — productos nativos sobre protocolos abiertos (OIDC, JMAP, CalDAV, CardDAV, WebDAV).
- **CTA** + **404** + `robots.txt`.

Los `id` de sección son los mismos en todos los idiomas: un ancla es un
identificador, no texto de escaparate.

El acceso (`Empezar`) enlaza al login de **Sytue One** (`one.sytue.eu/auth/login`).

## Build estático y runtime

`astro build` genera `dist/`. La imagen de producción lo sirve con `serve.mjs`
(servidor estático Bun sin dependencias, cabeceras de caché y fallback a
`404.html`). Se ejecuta en Scaleway/Kapsule (UE).

## SEO y rendimiento (contrato §0.8)

Superficie **pública indexable**: aquí se concentra el SEO (las apps
autenticadas van `noindex`). Estrategia y presupuestos en
[`docs/operations.md#4-rendimiento-y-seo`](../../../../docs/operations.md#4-rendimiento-y-seo).

- **SSG (`output: 'static'`)** — HTML pre-renderizado: TTFB mínimo y CWV verdes.
- **HTML semántico** — `header`/`main`/`nav`/`footer`, jerarquía de encabezados,
  `skip-link` al contenido y `aria-label` en la navegación, todo traducido.
- **Metadatos + OpenGraph + Twitter Cards** — en `MarketingLayout.astro`
  (título, descripción, `og:*`, `twitter:card=summary_large_image`).
- **Datos estructurados JSON-LD** (`src/lib/seo.ts`) — schema.org
  `Organization`, `WebSite` y `SoftwareApplication`, inyectados en la home.
- **`sitemap.xml`** — `@astrojs/sitemap` genera `sitemap-index.xml` +
  `sitemap-0.xml` (con `hreflang`); excluye `/404`.
- **`robots.txt`** (`public/robots.txt`) — permite lo público, bloquea rutas de
  utilidad (`/api/`, `/auth/`, previews) y apunta al sitemap.
- **Canonical + hreflang** — canónica del **propio idioma** de cada página y
  alternos recíprocos en los 26 idiomas (`en-GB`, `de-DE`, `pl-PL`…) +
  `x-default` → inglés, más `og:locale` y `og:locale:alternate`.
  `hreflangAlternates(path, locales)` **exige** la lista de idiomas en los que la
  página existe: declarar uno sin traducción manda al rastreador a un 404, así
  que la función no puede afirmarlo por su cuenta.
- **Selector de idioma** — cambia de idioma **sin perder la página**
  (`/es/privacy` → `/fr/privacy`) y nombra cada idioma en su propio idioma.
- **Imágenes AVIF/WebP** — `astro:assets` (Sharp) sirve formatos modernos con
  dimensiones explícitas (sin CLS) y `loading="lazy"`.
- **Presupuesto de bundle** — `vite.build.chunkSizeWarningLimit: 100` (kB);
  islands con JS mínimo (`client:visible`/`idle`).
- **Cabeceras de seguridad** — `serve.mjs` emite CSP estricta, HSTS, `nosniff`,
  `X-Frame-Options: DENY` y cache-control por tipo de recurso.

### Patrón `noindex` para superficies autenticadas (todas las apps)

El contrato exige que **las superficies autenticadas NO se indexen**. Este sitio
soporta `noindex` por página vía la prop del layout:

```astro
<MarketingLayout locale={locale} path="/…" title="…" description="…" noindex={true}>
  …
</MarketingLayout>
```

que emite `<meta name="robots" content="noindex, nofollow">`. Las **apps
autenticadas** (`portal`, `mail`, `drive`, …) deben aplicar el **mismo patrón en
su layout base, fijado siempre a `noindex`** (nunca condicional), ya que sirven
SSR con sesión. Además, su servidor/ingress debe emitir la cabecera
`X-Robots-Tag: noindex, nofollow` para blindar respuestas no-HTML. No requieren
`sitemap.xml` ni datos estructurados: el SEO se concentra en `www`.

## Scripts

```bash
bun run --filter @sytue/app-www dev      # http://localhost:4324
bun run --filter @sytue/app-www build
bun run --filter @sytue/app-www typecheck
```
