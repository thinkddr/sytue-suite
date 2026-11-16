/**
 * /api/snooze — posponer conversaciones, proxy BFF al servicio Go (REST
 * /api/v1/snoozes y /api/v1/threads/{id}/snooze). GET lista las pospuestas de la
 * cuenta; POST multiplexa por la forma del cuerpo, porque el checkOrigin CSRF de
 * Astro rechaza PUT y DELETE desde el navegador:
 *   { threadId, until }         → posponer hasta `until` (o cambiar la fecha)
 *   { threadId, cancel: true }  → quitar la posposición (vuelve ya a Recibidos)
 *
 * Los rechazos que la pantalla sabe explicar (fecha fuera de rango, nada en
 * Recibidos, ya no estaba pospuesta) conservan su estado y el `code` del servicio;
 * el resto es un 502.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailDelete, mailGet, mailPut } from '../../lib/mailrest.js';
import { json, rechazoDelServicio } from '../../lib/bffRespuestas.js';

export const prerender = false;

const UUID = /^[0-9a-f-]{36}$/i;

interface RawSnooze {
  thread_id: string;
  until: string;
}

/** La posposición del Go (snake_case) en la forma camelCase del cliente. */
function mapSnooze(s: RawSnooze) {
  return { threadId: s.thread_id, until: s.until };
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  try {
    const res = await mailGet(session.accessToken, '/api/v1/snoozes');
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as { snoozes?: RawSnooze[] };
    return json({ snoozes: (body.snoozes ?? []).map(mapSnooze) });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};

interface Payload {
  threadId?: string;
  until?: string;
  cancel?: boolean;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const body = (await ctx.request.json().catch(() => ({}))) as Payload;
  if (typeof body.threadId !== 'string' || !UUID.test(body.threadId)) {
    return json({ error: 'invalid_thread' }, 400);
  }
  const ruta = `/api/v1/threads/${body.threadId}/snooze`;
  try {
    if (body.cancel === true) {
      const res = await mailDelete(session.accessToken, ruta);
      if (!res.ok) return rechazoDelServicio(res);
      return json({ ok: true });
    }
    if (typeof body.until !== 'string' || Number.isNaN(Date.parse(body.until))) {
      return json({ error: 'invalid_payload' }, 400);
    }
    const res = await mailPut(session.accessToken, ruta, {
      until: new Date(body.until).toISOString(),
    });
    if (!res.ok) return rechazoDelServicio(res);
    return json({ snooze: mapSnooze((await res.json()) as RawSnooze) });
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
};
