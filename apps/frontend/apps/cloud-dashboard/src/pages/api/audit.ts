/*
 * BFF del visor de auditoría.
 *   GET /api/audit?actor=&action=&since=&until=&cursor=&limit=  -> { events, nextCursor }
 *   GET /api/audit?format=csv&...                               -> descarga CSV
 *
 * Proxy puro: los filtros y la paginación keyset los resuelve el control-plane (es
 * quien tiene el índice y el aislamiento por organización); aquí solo se adjunta el
 * access token de la sesión, que NUNCA llega al navegador. El CSV se reenvía en
 * STREAMING —el cuerpo pasa de un fetch al otro— para no materializar en Node un
 * export que puede ser de decenas de miles de filas.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { exportAuditCsv, httpErrorInfo, loadAudit, type AuditQuery } from '../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const p = new URL(ctx.request.url).searchParams;
  const limit = Number(p.get('limit'));
  const query: AuditQuery = {
    actor: p.get('actor') ?? undefined,
    action: p.get('action') ?? undefined,
    since: p.get('since') ?? undefined,
    until: p.get('until') ?? undefined,
    cursor: p.get('cursor') ?? undefined,
    limit: Number.isFinite(limit) && limit > 0 ? limit : undefined,
  };

  if (p.get('format') === 'csv') {
    try {
      const upstream = await exportAuditCsv(session, query);
      // Se conserva el filename que compone el control-plane (lleva la org y la fecha).
      return new Response(upstream.body, {
        status: 200,
        headers: {
          'content-type': 'text/csv; charset=utf-8',
          'content-disposition':
            upstream.headers.get('content-disposition') ?? 'attachment; filename="audit.csv"',
          'cache-control': 'no-store',
        },
      });
    } catch (err) {
      const info = httpErrorInfo(err);
      if (info) return json({ error: info.code }, info.status);
      return json({ error: 'control_plane_unavailable' }, 503);
    }
  }

  const { live, data, error } = await loadAudit(session, query);
  // Un 403 del control-plane no puede volver como 200 con lista vacía: el admin
  // creería que su organización no tiene traza.
  if (error) return json({ error: error.code }, error.status);
  return json({ live, events: data.events, nextCursor: data.nextCursor });
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
