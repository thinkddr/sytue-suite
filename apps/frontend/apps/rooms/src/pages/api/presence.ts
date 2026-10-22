/**
 * BFF de presencia — proxy hacia el HUB (no al servicio Chat). La presencia
 * (heartbeat + consulta agregada) vive en Sytue Hub; este proxy reenvia con el
 * access token de la sesion, que nunca llega al navegador.
 *   GET  /api/presence?subjects=a,b,c   -> Hub GET /api/v1/presence   -> {presence:[…]}
 *   PUT  /api/presence  {state,status}  -> Hub PUT /api/v1/presence   (heartbeat)
 */
import type { APIRoute } from 'astro';
import { serverEnv } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';

export const prerender = false;

function hubBase(): string {
  return (serverEnv(import.meta.env).SYTUE_HUB_BASE_URL ?? 'https://hub.sytue.eu').replace(
    /\/$/,
    '',
  );
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  // subjects viene ya decodificado por Astro; se re-codifica cada sub al reenviar
  // (sin tocar las comas, que el Hub usa como separador CSV).
  const subjects = (ctx.url.searchParams.get('subjects') ?? '')
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean)
    .map(encodeURIComponent)
    .join(',');
  if (!subjects) return json({ presence: [] });

  try {
    const res = await fetch(`${hubBase()}/api/v1/presence?subjects=${subjects}`, {
      headers: { authorization: `Bearer ${session.accessToken}`, accept: 'application/json' },
    });
    if (!res.ok) return json({ error: 'hub_error', status: res.status }, 502);
    return json(await res.json());
  } catch (err) {
    return json({ error: 'hub_error' }, 502);
  }
};

export const PUT: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: { state?: string; status?: string };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  try {
    const res = await fetch(`${hubBase()}/api/v1/presence`, {
      method: 'PUT',
      headers: {
        authorization: `Bearer ${session.accessToken}`,
        'content-type': 'application/json',
        accept: 'application/json',
      },
      body: JSON.stringify({ state: payload.state ?? 'online', status: payload.status ?? '' }),
    });
    if (!res.ok) return json({ error: 'hub_error', status: res.status }, 502);
    return json(await res.json());
  } catch (err) {
    return json({ error: 'hub_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
