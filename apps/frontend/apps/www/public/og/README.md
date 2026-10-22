<!--
SPDX-License-Identifier: Apache-2.0
SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
-->

# Recursos sociales (OpenGraph / Twitter / JSON-LD)

Imágenes que **no** se procesan por `astro:assets` (deben ser URLs absolutas y
estables para los _crawlers_ sociales), servidas tal cual desde `/og/`. Las
referencia `src/lib/seo.ts` y `MarketingLayout.astro`.

| Fichero         | Uso                                            | Formato requerido                              |
| --------------- | ---------------------------------------------- | ---------------------------------------------- |
| `sytue-one.png` | `og:image` / `twitter:image` por defecto       | **PNG 1200×630**, < 150 kB                     |
| `logo.png`      | `logo` de `Organization` (JSON-LD, schema.org) | **PNG cuadrado** ≥ 112×112, fondo transparente |

Ambos se generan desde los SVG de `branding/` con `bun render-og.mjs` (en la raíz
de esta app). Van **versionados** —no se generan en el build— porque los
_crawlers_ sociales necesitan una URL absoluta y estable, y `astro:assets` no
toca `public/`. Al cambiar la identidad: editar el SVG en `branding/` y volver a
ejecutar el script.

> `og-image.svg` es **solo el símbolo**, sin el logotipo: el renderizador no tiene
> tipografías instaladas —un `<text>` sale en blanco— y el sitio habla cinco
> idiomas, así que una tarjeta sin palabras es la misma en todos.
