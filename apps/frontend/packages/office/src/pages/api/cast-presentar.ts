/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * BFF de «Presentar con Cast» (Sytue Diapos → Sytue Cast).
 *
 *   POST /api/cast-presentar            body {title, visibility?, ref?}  → { videoId }
 *   PUT  /api/cast-presentar?live=<id>  body {index, total, title?, ref?}
 *
 * El POST programa un directo en Cast para emitir desde el navegador (`POST /lives` con
 * source=webrtc); la emisión en sí —pantalla, cámara, micro— se hace en la página de Cast,
 * a la que Diapos manda con el id. El PUT reenvía la diapositiva en la que va quien
 * presenta (`PUT /lives/{id}/slide`), con el token de su sesión: el ACL lo pone Cast (solo el
 * dueño del directo).
 *
 * SOLO VIAJA LA POSICIÓN. Este endpoint no acepta —ni reenvía— el contenido de la
 * presentación ni las notas del ponente: cada campo se vuelve a construir aquí y lo
 * demás se descarta, de modo que un cliente que mandara `notes` no las sacaría de Diapos.
 *
 * No es un proxy abierto a Cast: dos operaciones, con sus campos validados.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext, castBaseUrl } from '../../lib/auth';
import { esUuid, json } from '../../lib/docsProxy';

export const prerender = false;

const VISIBILIDADES = new Set(['public', 'unlisted', 'org', 'people']);
const RE_REF = /^[a-z][a-z0-9-]{0,19}:[A-Za-z0-9_-]{1,100}$/;
const MAX_DIAPOSITIVAS = 2000;

/** Pasa a Cast lo que contesta: el estado y, si viene, el código del error del sobre. */
async function aCast(
  ruta: string,
  metodo: 'POST' | 'PUT',
  token: string,
  cuerpo: unknown,
): Promise<{ ok: boolean; status: number; datos: unknown }> {
  const res = await fetch(`${castBaseUrl()}/api/v1${ruta}`, {
    method: metodo,
    headers: { authorization: `Bearer ${token}`, 'content-type': 'application/json' },
    body: JSON.stringify(cuerpo),
    signal: AbortSignal.timeout(8000),
  });
  let datos: unknown = null;
  try {
    datos = await res.json();
  } catch {
    // Sin cuerpo JSON: basta con el estado.
  }
  return { ok: res.ok, status: res.status, datos };
}

function falloDe(r: { status: number; datos: unknown }): Response {
  const code = (r.datos as { error?: { code?: string } } | null)?.error?.code;
  return json(
    { error: code ?? 'cast_error' },
    r.status === 401 || r.status === 403 ? r.status : r.status >= 500 ? 502 : r.status,
  );
}

async function leerCuerpo(request: Request): Promise<Record<string, unknown> | null> {
  try {
    const o: unknown = await request.json();
    return o && typeof o === 'object' && !Array.isArray(o) ? (o as Record<string, unknown>) : null;
  } catch {
    return null;
  }
}

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const o = await leerCuerpo(ctx.request);
  if (!o) return json({ error: 'bad_request' }, 400);
  const title = typeof o['title'] === 'string' ? o['title'].replace(/\s+/g, ' ').trim() : '';
  if (title === '' || Array.from(title).length > 200) return json({ error: 'bad_title' }, 400);
  // Lo que se emite nace con visibilidad `org`, como lo que se sube (decisión del dueño, 28/09):
  // hacerlo público es un paso explícito de quien presenta.
  const visibility = typeof o['visibility'] === 'string' ? o['visibility'] : 'org';
  if (!VISIBILIDADES.has(visibility)) return json({ error: 'bad_visibility' }, 400);
  try {
    const r = await aCast('/lives', 'POST', session.tokens.accessToken, {
      title,
      visibility,
      source: 'webrtc',
    });
    if (!r.ok) return falloDe(r);
    const id = (r.datos as { video?: { id?: unknown } } | null)?.video?.id;
    if (typeof id !== 'string' || !esUuid(id)) return json({ error: 'cast_error' }, 502);
    return json({ videoId: id }, 201);
  } catch {
    return json({ error: 'cast_unavailable' }, 502);
  }
};

export const PUT: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const live = new URL(ctx.request.url).searchParams.get('live') ?? '';
  if (!esUuid(live)) return json({ error: 'bad_live' }, 400);
  const o = await leerCuerpo(ctx.request);
  if (!o) return json({ error: 'bad_request' }, 400);
  const { index, total } = o;
  if (
    !Number.isInteger(index) ||
    !Number.isInteger(total) ||
    (total as number) < 1 ||
    (total as number) > MAX_DIAPOSITIVAS ||
    (index as number) < 0 ||
    (index as number) >= (total as number)
  ) {
    return json({ error: 'bad_position' }, 400);
  }
  const title = typeof o['title'] === 'string' ? o['title'].slice(0, 400) : '';
  const ref = typeof o['ref'] === 'string' && RE_REF.test(o['ref']) ? o['ref'] : '';
  try {
    const r = await aCast(`/lives/${live}/slide`, 'PUT', session.tokens.accessToken, {
      index,
      total,
      title,
      ref,
    });
    return r.ok ? json({ ok: true }) : falloDe(r);
  } catch {
    return json({ error: 'cast_unavailable' }, 502);
  }
};
