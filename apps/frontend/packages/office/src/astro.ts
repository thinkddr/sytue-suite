/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL BFF DE LA OFIMÁTICA, montado en cada app que lo usa.
 *
 * Los endpoints de `src/pages/` de este paquete (`/api/*`, `/open-from-drive`, el
 * canje de `/s/{token}` y el service worker) son los MISMOS para todas las apps de
 * ofimática: proxies con la sesión de quien llama hacia el único servicio Go de
 * documentos. Astro solo enruta los ficheros del `src/pages` de la app, así que se
 * inyectan desde aquí, con su ruta igual al fichero —como si estuvieran en la app—.
 *
 * Las PANTALLAS no van por aquí: cada app tiene las suyas en `src/pages/*.astro`
 * (son las que dicen qué producto es cada una, y las que cuenta la paridad con las
 * nativas) y montan las vistas de `src/vistas/`.
 */
import { readdirSync } from 'node:fs';
import type { AstroIntegration } from 'astro';

const PAGINAS = new URL('./pages/', import.meta.url);

/** `api/documents.ts` → `/api/documents`; `sw.js.ts` → `/sw.js`. */
export function patronDe(fichero: string): string {
  return `/${fichero.replace(/\.ts$/, '')}`;
}

/** Los endpoints que hay hoy, leídos del disco: uno nuevo se enruta sin tocar nada. */
export function endpoints(): string[] {
  return readdirSync(PAGINAS, { recursive: true, encoding: 'utf8' })
    .map((f) => f.split('\\').join('/'))
    .filter((f) => f.endsWith('.ts') && !f.includes('.test.'))
    .sort();
}

export function sytueOffice(): AstroIntegration {
  return {
    name: 'sytue-office',
    hooks: {
      'astro:config:setup': ({ injectRoute }) => {
        for (const fichero of endpoints()) {
          injectRoute({
            pattern: patronDe(fichero),
            entrypoint: new URL(fichero, PAGINAS),
            prerender: false,
          });
        }
      },
    },
  };
}
