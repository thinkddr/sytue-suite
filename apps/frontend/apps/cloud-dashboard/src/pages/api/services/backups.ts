/*
 * GET  /api/services/backups?service=<id>  -> lista de backups de un Postgres
 * GET  /api/services/backups?service=<id>&download=<fichero> -> URL de descarga
 * POST /api/services/backups { service, backup } -> restaura ese backup (DESTRUCTIVO)
 * POST /api/services/backups { service, backup_now: true } -> pide una copia ahora
 *
 * BFF de backups/restore de Sytue Postgres. Proxy PURO al control-plane
 * (GET/POST /api/v1/services/{id}/{backups,restore}); sin lógica (invariante). El
 * restore viaja por POST porque el checkOrigin CSRF de Astro rechaza métodos no
 * simples con 403 (mismo patrón que /api/alerts y /api/billing/spend-limit).
 * Devuelve { ok, live }: live=false cuando no hay backend vivo y el cambio queda
 * en demostración. La autorización real (admin/owner + scope deploy) la impone el
 * control-plane. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import {
  loadServiceBackups,
  serviceBackupDownloadURL,
  restoreService,
  backupServiceNow,
  httpErrorInfo,
} from '../../../lib/cloud.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const serviceId = new URL(ctx.request.url).searchParams.get('service')?.trim();
  if (!serviceId) return json({ error: 'missing_service' }, 400);

  // Descargar va en el MISMO GET que listar: es el mismo recurso visto de dos
  // maneras, y una ruta aparte solo repetiría la sesión y la validación.
  const download = new URL(ctx.request.url).searchParams.get('download')?.trim();
  if (download) {
    try {
      const url = await serviceBackupDownloadURL(session, serviceId, download);
      if (!url) return json({ error: 'unavailable' }, 503);
      // La URL firmada NO se cachea en ningún sitio: es una credencial temporal.
      return json({ url });
    } catch (err) {
      const info = httpErrorInfo(err);
      if (info) return json({ error: info.code }, info.status);
      throw err;
    }
  }

  const { data, live } = await loadServiceBackups(session, serviceId);
  return json({ backups: data, live });
};

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { service?: unknown; backup?: unknown; backup_now?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const service = typeof body.service === 'string' ? body.service.trim() : '';
  if (!service) return json({ error: 'missing_service' }, 400);

  // Copia a demanda: no lleva nombre de backup porque lo elige el servidor (la
  // fecha), y no destruye nada, así que va antes de exigir `backup`.
  if (body.backup_now === true) {
    try {
      const live = await backupServiceNow(session, service);
      return json({ ok: true, live });
    } catch (err) {
      const info = httpErrorInfo(err);
      if (info) return json({ error: info.code }, info.status);
      throw err;
    }
  }

  const backup = typeof body.backup === 'string' ? body.backup.trim() : '';
  if (!backup) return json({ error: 'missing_backup' }, 400);

  try {
    const live = await restoreService(session, service, backup);
    return json({ ok: true, live });
  } catch (err) {
    // Error REAL del control-plane (403 sin rol, 409 estado inválido…): status real a la UI.
    const info = httpErrorInfo(err);
    if (info) return json({ error: info.code }, info.status);
    throw err;
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
