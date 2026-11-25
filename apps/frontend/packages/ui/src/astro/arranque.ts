/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * `/arranque.js` de cada superficie SSR: los scripts de arranque del layout —el
 * prefijo de `/api` y `/auth`, el tema guardado y, si la app lo pide, el registro
 * del Service Worker— como FICHERO de este origen y no en línea.
 *
 * En línea, una CSP con `script-src` estricto los bloquea: Astro solo firma (hash)
 * los scripts que procesa, y estos los escribe el layout a mano con contenido que
 * depende del entorno (el prefijo REAL del contenedor, SYTUE_BASE_PATH, no el del
 * build). Servidos desde aquí los cubre `'self'` y la CSP puede quitar
 * `'unsafe-inline'`. «Mi cuenta» lo hacía así con su propio fichero; ahora es uno
 * para todas, que inyecta `sytueArranque()` (`@sytue/config/astro-arranque`).
 *
 * Se incluye con `<script is:inline src={withBase(prefijo, '/arranque.js')}>`,
 * SÍNCRONO a propósito: tiene que correr antes que las islas. La variante con el
 * registro del Service Worker es `arranque-sw.ts`.
 */
import type { APIRoute } from 'astro';
import { basePathScript, swRegisterScript } from '../basePath.js';
import { themeInitScript } from '../theme/ThemeProvider.js';

export const prerender = false;

/** La respuesta de `/arranque.js`; con `serviceWorker`, también registra el worker. */
export function respuestaDeArranque(serviceWorker: boolean): Response {
  // El `base` del build (Vite lo fija siempre; el `?? '/'` es para el tipado fuera de Astro).
  const build = import.meta.env.BASE_URL ?? '/';
  const prefijo = process.env['SYTUE_BASE_PATH'] ?? build;
  const partes = [basePathScript(prefijo), themeInitScript];
  if (serviceWorker) partes.push(swRegisterScript(build));
  return new Response(partes.join('\n'), {
    headers: {
      'content-type': 'text/javascript; charset=utf-8',
      // Cambia con cada despliegue, no con cada visita: cinco minutos bastan.
      'cache-control': 'public, max-age=300',
      'x-content-type-options': 'nosniff',
    },
  });
}

export const GET: APIRoute = () => respuestaDeArranque(false);
