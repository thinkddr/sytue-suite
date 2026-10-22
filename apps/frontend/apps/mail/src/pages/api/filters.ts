/**
 * /api/filters — reglas/filtros de servidor (auto-mover/marcar el correo entrante
 * por remitente/asunto). Proxy BFF autorizado por cookie al servicio Go de correo
 * (GET/POST/PUT/DELETE /api/v1/filters) vía el SDK JMAP.
 *
 * El navegador solo usa GET y POST (el checkOrigin CSRF de Astro rechaza
 * PUT/DELETE con cuerpo desde el navegador): POST crea (sin id), actualiza (con
 * id) o borra ({ id, delete:true }); el SDK traduce a los verbos REST reales.
 */
import type { APIRoute } from 'astro';
import type { MailFilterInput } from '@sytue/api-clients/mail';
import { getApiSession } from '../../lib/session.js';
import { jmapClient } from '../../lib/jmap.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const filters = await jmapClient(session.accessToken).listFilters();
    return json({ filters });
  } catch {
    return json({ error: 'jmap_error' }, 502);
  }
};

interface Payload extends MailFilterInput {
  id?: string;
  delete?: boolean;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  const client = jmapClient(session.accessToken);
  try {
    if (body.delete) {
      if (!body.id) return json({ error: 'invalid_payload' }, 400);
      await client.deleteFilter(body.id);
      return json({ ok: true });
    }
    const input: MailFilterInput = {
      name: body.name ?? '',
      enabled: body.enabled ?? true,
      priority: body.priority ?? 0,
      matchFrom: body.matchFrom ?? '',
      matchSubject: body.matchSubject ?? '',
      moveTo: body.moveTo ?? null,
      markRead: body.markRead ?? false,
      markFlagged: body.markFlagged ?? false,
    };
    if (body.id) {
      await client.updateFilter(body.id, input);
      return json({ ok: true });
    }
    const filter = await client.createFilter(input);
    return json({ filter }, 201);
  } catch {
    return json({ error: 'jmap_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
