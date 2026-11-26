<!--
SPDX-License-Identifier: Apache-2.0
SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
-->

<a id="recursos-sociales-opengraph--twitter--json-ld"></a>

# Social assets (OpenGraph / Twitter / JSON-LD)

Images that are **not** processed by `astro:assets` (they must be absolute, stable
URLs for social _crawlers_), served as-is from `/og/`. They are
referenced by `src/lib/seo.ts` and `MarketingLayout.astro`.

| File            | Use                                         | Required format                                  |
| --------------- | ------------------------------------------- | ------------------------------------------------ |
| `sytue-one.png` | Default `og:image` / `twitter:image`        | **PNG 1200×630**, < 150 kB                       |
| `logo.png`      | `Organization` `logo` (JSON-LD, schema.org) | **Square PNG** ≥ 112×112, transparent background |

Both are generated from the SVGs in `branding/` with `bun render-og.mjs` (at the root
of this app). They are **checked in** (not generated at build time) because
social _crawlers_ need an absolute, stable URL, and `astro:assets` does not
touch `public/`. When the brand identity changes: edit the SVG in `branding/` and run
the script again.

> `og-image.svg` is **only the symbol**, without the wordmark: the renderer has no
> fonts installed (a `<text>` comes out blank) and the site speaks five
> languages, so a card without words is the same in all of them.
