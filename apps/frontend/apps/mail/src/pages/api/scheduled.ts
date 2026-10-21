/**
 * Envíos DIFERIDOS de Sytue Mail (proxy BFF autorizado por cookie de sesión).
 *
 *   GET  /api/scheduled?cursor=…   → lista (keyset) los programados del usuario.
 *   POST /api/scheduled            → {action:'cancel', id}            cancela y
 *                                     reabre como borrador; o
 *                                     {action:'reschedule', id, sendAt} edita hora.
 *
 * Se usa POST (no DELETE/PUT) porque el checkOrigin CSRF de Astro rechaza esos
 * métodos desde el navegador; el BFF traduce a las rutas REST del servicio Go
 * (Bearer con el access token de sesión), que aísla por dueño (account del token).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, mailPost } from '../../lib/mailrest.js';

export const prerender = false;

interface RawItem {
  id: string;
  from?: string;
  recipients?: string[];
  subject?: string;
  release_at?: string;
  created_at?: string;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const cursor = ctx.url.searchParams.get('cursor') ?? '';
  const path = `/api/v1/scheduled${cursor ? `?cursor=${encodeURIComponent(cursor)}` : ''}`;
  try {
    const res = await mailGet(session.accessToken, path);
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const data = (await res.json()) as { scheduled?: RawItem[]; next_cursor?: string };
    return json({
      scheduled: (data.scheduled ?? []).map((i) => ({
        id: i.id,
        from: i.from ?? '',
        recipients: i.recipients ?? [],
        subject: i.subject ?? '(Sin asunto)',
        releaseAt: i.release_at ?? '',
        createdAt: i.created_at ?? '',
      })),
      nextCursor: data.next_cursor ?? '',
    });
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
};

interface Payload {
  action?: 'cancel' | 'reschedule';
  id?: string;
  sendAt?: string;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  if (!body.id) return json({ error: 'missing_id' }, 400);
  const id = encodeURIComponent(body.id); // el servicio Go valida que sea UUID

  try {
    if (body.action === 'cancel') {
      const res = await mailPost(session.accessToken, `/api/v1/outbound/${id}/cancel`, {});
      if (!res.ok) return json({ error: 'mail_error', status: res.status }, res.status);
      const data = (await res.json()) as { canceled?: boolean; kind?: string };
      return json({ canceled: Boolean(data.canceled), kind: data.kind });
    }
    if (body.action === 'reschedule') {
      if (!body.sendAt) return json({ error: 'missing_send_at' }, 400);
      const res = await mailPost(session.accessToken, `/api/v1/scheduled/${id}/reschedule`, {
        send_at: body.sendAt,
      });
      if (!res.ok) return json({ error: 'mail_error', status: res.status }, res.status);
      const data = (await res.json()) as { id?: string; release_at?: string };
      return json({ id: data.id, releaseAt: data.release_at });
    }
    return json({ error: 'bad_action' }, 400);
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
