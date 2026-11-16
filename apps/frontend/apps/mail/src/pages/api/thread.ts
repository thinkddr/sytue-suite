/**
 * GET /api/thread?threadId= — el hilo COMPLETO atravesando carpetas, contra el
 * servicio Go (GET /api/v1/threads/{id}/messages). La lista de la carpeta solo
 * trae lo que hay EN esa carpeta, así que la vista de conversación no podía
 * enseñar la copia en Enviados de tu propia respuesta ni lo ya archivado: esto
 * es lo que la completa. Proxy BFF autorizado por cookie.
 *
 * Cada mensaje lleva `mailboxId`: el cliente decide qué carpetas mezclar en la
 * conversación (Papelera y Spam quedan fuera salvo que estés en ellas). Sin
 * paginación —un hilo son decenas de mensajes y el servicio impone un tope—.
 * Owner-scoped: un hilo ajeno sale vacío (el servicio filtra por cuenta + RLS).
 *
 * Con los mensajes va el estado del hilo: `muted` (silenciado) y `snoozedUntil`
 * (hasta cuándo está pospuesto, o null), que el lector enseña y usa para decidir
 * qué ofrecer.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, toEmailSummary } from '../../lib/mailrest.js';

export const prerender = false;

interface ThreadResponse {
  messages?: unknown[];
  muted?: boolean;
  snoozed_until?: string | null;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const threadId = ctx.url.searchParams.get('threadId');
  if (!threadId || !/^[0-9a-f-]{36}$/i.test(threadId))
    return json({ error: 'missing_threadId' }, 400);

  try {
    const res = await mailGet(
      session.accessToken,
      `/api/v1/threads/${encodeURIComponent(threadId)}/messages`,
    );
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as ThreadResponse;
    const emails = (body.messages ?? []).map((m) => ({
      ...toEmailSummary(m as never),
      mailboxId: (m as { mailbox_id?: string }).mailbox_id ?? '',
    }));
    return json({
      emails,
      muted: body.muted === true,
      snoozedUntil: body.snoozed_until ?? null,
    });
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
