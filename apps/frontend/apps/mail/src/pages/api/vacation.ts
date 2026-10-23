/**
 * /api/vacation — autorespuesta / vacaciones (config de fuera-de-oficina), proxy
 * BFF al servicio Go (REST /api/v1/vacation). GET lee; POST guarda (el checkOrigin
 * CSRF de Astro rechaza el PUT del navegador, así que el BFF traduce POST → PUT).
 * La aplica la ENTREGA ENTRANTE del servicio (RFC 3834 + dedupe); aquí solo get/set.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, mailOrigin } from '../../lib/mailrest.js';

export const prerender = false;

interface RawVacation {
  enabled?: boolean;
  subject?: string;
  body?: string;
  starts_at?: string;
  ends_at?: string;
}

/** Normaliza el snake_case del Go a la forma camelCase del cliente. */
function mapVacation(v: RawVacation) {
  return {
    enabled: v.enabled ?? false,
    subject: v.subject ?? '',
    body: v.body ?? '',
    startsAt: v.starts_at ?? '',
    endsAt: v.ends_at ?? '',
  };
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await mailGet(session.accessToken, '/api/v1/vacation');
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    return json(mapVacation((await res.json()) as RawVacation));
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

interface Payload {
  enabled?: boolean;
  subject?: string;
  body?: string;
  startsAt?: string;
  endsAt?: string;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  try {
    // POST del navegador → PUT autorizado (Bearer) al servicio Go.
    const res = await fetch(`${mailOrigin()}/api/v1/vacation`, {
      method: 'PUT',
      headers: {
        authorization: `Bearer ${session.accessToken}`,
        'content-type': 'application/json',
        accept: 'application/json',
      },
      body: JSON.stringify({
        enabled: body.enabled === true,
        subject: body.subject ?? '',
        body: body.body ?? '',
        starts_at: body.startsAt ?? '',
        ends_at: body.endsAt ?? '',
      }),
    });
    if (!res.ok)
      return json({ error: 'mail_error', status: res.status }, res.status === 400 ? 400 : 502);
    return json({ ok: true });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
