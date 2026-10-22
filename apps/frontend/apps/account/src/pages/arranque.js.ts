/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los dos scripts de arranque del layout —el prefijo de `/api` y `/auth`, y el tema
 * guardado— como FICHERO y no en línea.
 *
 * «Mi cuenta» comparte origen con Sytue ID y lleva `script-src` estricto
 * (docs/security.md#riesgo-aceptado-mi-cuenta-en-el-origen-del-idp): solo corren
 * los scripts de este origen y los que Astro procesa con su hash. Estos dos los
 * escribe el layout a mano, así que Astro no los ve; en línea, el navegador los
 * bloqueaba —medido: sin el prefijo, TODAS las llamadas de la app daban 404—.
 * Servidos desde aquí los cubre `'self'`, y el prefijo sigue siendo el REAL del
 * contenedor (SYTUE_BASE_PATH) y no el del build.
 */
import type { APIRoute } from 'astro';
import { serverEnv } from '@sytue/auth/server';
import { basePathScript, themeInitScript } from '@sytue/ui';

export const prerender = false;

export const GET: APIRoute = () =>
  new Response(
    basePathScript(serverEnv(import.meta.env).SYTUE_BASE_PATH ?? import.meta.env.BASE_URL) +
      '\n' +
      themeInitScript,
    {
      headers: {
        'content-type': 'text/javascript; charset=utf-8',
        // Cambia con cada despliegue, no con cada visita: cinco minutos bastan.
        'cache-control': 'public, max-age=300',
      },
    },
  );
