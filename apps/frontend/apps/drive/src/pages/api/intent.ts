/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * POST /api/intent — resuelve un intent cross-app via Sytue Hub (docs/CONVENTIONS.md §11).
 * Uso de Drive: «Compartir a Rooms» (share-to-chat) — el Hub compone el deep-link a
 * Sytue Rooms (Chat) para compartir el recurso en un canal. Proxy BFF autorizado por
 * la cookie de sesión httpOnly: reenvía al Hub con el access token de la sesión y
 * devuelve el IntentResult (deep-link + metadatos). El token NUNCA llega al cliente.
 *
 * El Hub NO ejecuta la acción ni puentea permisos: compone el enlace y Chat aplica
 * su autorización al publicar (no se puede compartir en un canal donde no estás).
 *
 * CSRF: POST content-type application/json (preflight) + cookie same-origin, como el
 * resto de BFF de la app.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext, hubInternalUrl } from '../../lib/auth';

export const prerender = false;

/**
 * Intents que Drive puede iniciar (el Hub revalida el tipo y los parámetros).
 *
 * Los dos son «desde un fichero, hacia otro producto»: compartirlo en un canal de
 * Rooms o adjuntarlo a un correo nuevo. Es una allowlist y no un pasamanos: este
 * BFF lleva el token de la sesión, así que dejar pasar cualquier tipo convertiría
 * a Drive en un trampolín para disparar intents que no son suyos.
 */
const ALLOWED = new Set(['share-to-chat', 'attach-drive-to-mail']);

interface IntentBody {
  type?: unknown;
  params?: unknown;
  ids?: unknown;
}

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: IntentBody | null;
  try {
    body = (await ctx.request.json()) as IntentBody;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  if (!body || typeof body.type !== 'string' || !ALLOWED.has(body.type)) {
    return json({ error: 'invalid_intent' }, 400);
  }

  const params: Record<string, string> = {};
  if (body.params && typeof body.params === 'object') {
    for (const [k, v] of Object.entries(body.params as Record<string, unknown>)) {
      if (typeof v === 'string') params[k] = v;
    }
  }
  const req = {
    type: body.type,
    params,
    ...(Array.isArray(body.ids)
      ? { ids: body.ids.filter((s): s is string => typeof s === 'string') }
      : {}),
  };

  let res: Response;
  try {
    res = await fetch(`${hubInternalUrl()}/api/v1/intents`, {
      method: 'POST',
      headers: {
        authorization: `Bearer ${session.tokens.accessToken}`,
        'content-type': 'application/json',
        accept: 'application/json',
      },
      body: JSON.stringify(req),
    });
  } catch {
    return json({ error: 'hub_unreachable' }, 502);
  }
  if (!res.ok) return json({ error: 'hub_error' }, res.status === 400 ? 400 : 502);
  return json(await res.json());
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
