/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/ai  {"path": "/ruta/fichero.md", "accion": "resumir"|"puntos"}  →  { text }
 *
 * Resume un fichero de Drive sin descargarlo al navegador.
 *
 * EL FICHERO NO BAJA AL CLIENTE. Se lee aquí, con el token de la sesión, y solo
 * sale el resumen. Pedirle al navegador que descargue el fichero para volver a
 * subirlo al asistente haría viajar el mismo contenido dos veces por la red del
 * usuario para devolver un párrafo.
 *
 * ponytail: SOLO FICHEROS DE TEXTO. Un PDF o un .docx necesitan un extractor
 * —el OCR de la casa es de imágenes y no lee PDF— y montarlo aquí sería un
 * servicio nuevo, no un proxy. Cuando exista un extractor, este handler solo
 * tiene que cambiar de dónde saca `texto`; el resto vale igual.
 */
import type { APIRoute } from 'astro';
import { serverEnv } from '@sytue/auth/server';
import { bff, driveClientFor, toBffContext } from '../../lib/auth';
import { esTexto } from '../../lib/nodes.js';

export const prerender = false;

const env = serverEnv(import.meta.env);

/** Lo que se lee del fichero, en bytes. Un resumen no mejora con más. */
const MAX_BYTES = 200 * 1024;

const TIMEOUT_MS = 130_000;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

function endpointAsistencia(base: string): string {
  const limpia = base.replace(/\/+$/, '');
  return limpia.endsWith('/api/v1/assist') ? limpia : `${limpia}/api/v1/assist`;
}

const INSTRUCCIONES = {
  resumir:
    'Resume este documento en un párrafo, en su mismo idioma. Di lo que dice, no lo que trata de. Conserva los datos concretos (fechas, cifras, nombres). Devuelve solo el resumen.',
  puntos:
    'Resume este documento en como mucho seis puntos, en su mismo idioma. Un punto por idea, empezando cada uno con «- ». Conserva los datos concretos. Devuelve solo la lista.',
} as const;

export const POST: APIRoute = async (ctx) => {
  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  const url = env.SYTUE_AI_URL;
  if (!url) return json({ error: 'ai_not_configured' }, 503);

  let payload: unknown;
  try {
    payload = await ctx.request.json();
  } catch {
    return json({ error: 'bad_request' }, 400);
  }
  const p = (payload ?? {}) as Record<string, unknown>;
  const path = typeof p.path === 'string' ? p.path : '';
  const accion = p.accion === 'puntos' ? 'puntos' : 'resumir';
  if (path === '' || !path.startsWith('/')) return json({ error: 'bad_request' }, 400);

  let res: Response;
  try {
    res = await client.openDownload(path);
  } catch {
    return json({ error: 'upstream_unreachable' }, 502);
  }
  if (!res.ok) return json({ error: res.status === 404 ? 'not_found' : 'ai_failed' }, 502);

  const tipo = res.headers.get('content-type') ?? '';
  if (!esTexto(tipo)) {
    // Se CIERRA el cuerpo: dejarlo colgando mantiene abierta la conexión con el
    // servicio de Drive hasta que el recolector se acuerde.
    await res.body?.cancel();
    return json({ error: 'unsupported_type' }, 415);
  }

  const bytes = new Uint8Array(await res.arrayBuffer()).slice(0, MAX_BYTES);
  // `fatal: false`: un fichero con bytes sueltos se lee con los rombos de
  // sustitución en vez de tumbar la petición entera por un byte.
  const texto = new TextDecoder('utf-8', { fatal: false }).decode(bytes).trim();
  if (texto === '') return json({ error: 'empty_file' }, 422);

  const sesion = await bff.getSession(toBffContext(ctx));
  let upstream: Response;
  try {
    upstream = await fetch(endpointAsistencia(url), {
      method: 'POST',
      headers: {
        'content-type': 'application/json',
        authorization: `Bearer ${sesion?.tokens.accessToken ?? ''}`,
      },
      body: JSON.stringify({
        action: accion,
        instruction: INSTRUCCIONES[accion],
        context: texto,
        kind: 'file',
        modo: 'calidad',
      }),
      signal: AbortSignal.timeout(TIMEOUT_MS),
    });
  } catch (e) {
    const timedOut = e instanceof Error && e.name === 'TimeoutError';
    return json({ error: timedOut ? 'ai_timeout' : 'upstream_unreachable' }, timedOut ? 504 : 502);
  }
  if (!upstream.ok) return json({ error: 'ai_failed' }, 502);
  return new Response(await upstream.text(), {
    status: 200,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
};
