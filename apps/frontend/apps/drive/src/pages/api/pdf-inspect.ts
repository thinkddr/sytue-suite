/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/pdf-inspect — índice de un PDF de Drive: qué páginas tiene, cómo son y
 * qué dice cada una. Lo consume el diálogo de organizar páginas para que se puedan
 * ELEGIR reconociéndolas, en vez de escribir «1,3,5-8» de memoria.
 *
 * Puente igual que /api/pdf-pages: descarga el fichero con el token de la sesión y
 * lo pasa por `POST /api/v1/pdf/inspect` de Docs. El token no llega al cliente y
 * nada se guarda — el índice se calcula y se devuelve.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext, driveClientFor, docsBaseUrl } from '../../lib/auth';

export const prerender = false;

const MAX_BYTES = 64 * 1024 * 1024;

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let path = '';
  try {
    const cuerpo = (await ctx.request.json()) as { path?: unknown };
    path = typeof cuerpo.path === 'string' ? cuerpo.path.trim() : '';
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  if (!path) return json({ error: 'missing_path' }, 400);

  const client = await driveClientFor(ctx);
  if (!client) return json({ error: 'unauthenticated' }, 401);

  let original: Blob;
  try {
    original = await client.download(path);
  } catch {
    return json({ error: 'download_failed' }, 404);
  }
  if (original.size > MAX_BYTES) return json({ error: 'too_large' }, 413);

  const form = new FormData();
  form.append('file', original, path.split('/').pop() ?? 'documento.pdf');

  let res: Response;
  try {
    res = await fetch(`${docsBaseUrl()}/api/v1/pdf/inspect`, {
      method: 'POST',
      headers: { authorization: `Bearer ${session.tokens.accessToken}` },
      body: form,
    });
  } catch {
    return json({ error: 'docs_unreachable' }, 502);
  }
  if (!res.ok) {
    const detalle = await res.text().catch(() => '');
    return json(
      { error: 'inspect_failed', message: mensajeDe(detalle) },
      res.status >= 500 ? 502 : res.status,
    );
  }
  return json(await res.json(), 200);
};

/** Extrae el mensaje legible del error de Docs, si lo trae. */
function mensajeDe(cuerpo: string): string | undefined {
  try {
    const j = JSON.parse(cuerpo) as { message?: unknown; error?: unknown };
    if (typeof j.message === 'string' && j.message !== '') return j.message;
    if (typeof j.error === 'string' && j.error !== '') return j.error;
  } catch {
    // cuerpo no-JSON: no hay nada que propagar
  }
  return undefined;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
