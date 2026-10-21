/**
 * GET /api/changes?since= — delta incremental de la bandeja (modelo JMAP
 * Email/changes) contra el servicio Go (GET /api/v1/changes). Refresco en vivo de
 * la bandeja ABIERTA: el cliente lo sondea con backoff y aplica SOLO lo cambiado,
 * sin recargar toda la lista. Proxy BFF autorizado por cookie. Devuelve
 * { changed:[…], destroyed:[…ids], state }. Cada `changed` lleva `mailboxId` para
 * ubicarlo en la vista (carpeta/etiqueta). Owner-scoped: el servicio solo entrega
 * los cambios del propio usuario (auth + RLS).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, toEmailSummary } from '../../lib/mailrest.js';

export const prerender = false;

interface ChangesResponse {
  changed?: unknown[];
  destroyed?: string[];
  state?: string;
  has_more?: boolean;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const since = ctx.url.searchParams.get('since') ?? '';
  const qs = since ? `?since=${encodeURIComponent(since)}` : '';

  try {
    const res = await mailGet(session.accessToken, `/api/v1/changes${qs}`);
    if (!res.ok) return json({ error: 'mail_error', status: res.status }, 502);
    const body = (await res.json()) as ChangesResponse;
    const changed = (body.changed ?? []).map((m) => ({
      ...toEmailSummary(m as never),
      mailboxId: (m as { mailbox_id?: string }).mailbox_id ?? '',
    }));
    return json({
      changed,
      destroyed: body.destroyed ?? [],
      state: body.state ?? '',
      hasMore: body.has_more ?? false,
    });
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
