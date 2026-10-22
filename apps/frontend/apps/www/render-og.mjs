/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Genera los binarios de `public/og/` desde los SVG de marca. Se ejecuta a mano
 * cuando cambia la identidad (`bun render-og.mjs`), no en cada build: los PNG van
 * versionados porque los crawlers sociales necesitan una URL estable y absoluta,
 * y porque `astro:assets` no toca `public/`.
 */
import sharp from 'sharp';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

const branding = fileURLToPath(new URL('../../../../branding/', import.meta.url));
const og = fileURLToPath(new URL('./public/og/', import.meta.url));

// og:image — 1200×630 exactos y < 150 kB: el contrato de public/og/README.md.
// `palette: true` deja la tarjeta en ~6 kB; son cuatro colores planos.
await sharp(readFileSync(`${branding}og/og-image.svg`), { density: 144 })
  .resize(1200, 630)
  .png({ compressionLevel: 9, palette: true })
  .toFile(`${og}sytue-one.png`);

// logo del JSON-LD Organization — cuadrado y con fondo TRANSPARENTE (schema.org).
await sharp(readFileSync(`${branding}logo/sytue-mark.svg`), { density: 600 })
  .resize(512, 512)
  .png({ compressionLevel: 9 })
  .toFile(`${og}logo.png`);

console.log('og/sytue-one.png y og/logo.png regenerados');
