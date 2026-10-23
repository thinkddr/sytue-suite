/**
 * GET /api/search?q=&limit=&position=&mailboxId= — busca correos por texto libre
 * (Email/query filter.text: FTS del servidor sobre asunto + remitente + cuerpo)
 * via el SDK JMAP. Proxy BFF autorizado por cookie. Query vacía ⇒ sin resultados.
 * `position` pagina por OFFSET (semántica JMAP `position`; la búsqueda ordena por
 * relevancia/fecha y no expone cursor keyset como el listado de buzón).
 */
import type { APIRoute } from 'astro';
import { consulta } from '@sytue/auth/server';
import { getApiSession } from '../../lib/session.js';
import { jmapClient } from '../../lib/jmap.js';

export const prerender = false;

const buscar: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const params = await consulta(ctx.request, ctx.url);
  const q = (params.get('q') ?? '').trim();
  if (!q) return json({ emails: [] });
  const limit = clampInt(params.get('limit'), 50, 1, 100);
  const position = clampInt(params.get('position'), 0, 0, 100_000);
  const mailboxId = params.get('mailboxId') ?? undefined;

  try {
    const emails = await jmapClient(session.accessToken).search(q, {
      limit,
      position,
      ...(mailboxId ? { mailboxId } : {}),
    });
    return json({ emails });
  } catch {
    return json({ error: 'jmap_error' }, 502);
  }
};

function clampInt(raw: string | null, fallback: number, min: number, max: number): number {
  const n = raw == null ? NaN : Number.parseInt(raw, 10);
  if (Number.isNaN(n)) return fallback;
  return Math.min(max, Math.max(min, n));
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

// GET y QUERY, el mismo handler. QUERY es el preferente del cliente y GET el
// respaldo para quien no lo hable; ver `consulta` en @sytue/auth/server.
export const GET: APIRoute = buscar;
export const QUERY: APIRoute = buscar;
