/*
 * POST /api/services/update { service, name?, config? } -> cambia un servicio
 * gestionado (nombre y/o configuración).
 *
 * BFF de edición. Proxy PURO al control-plane (PATCH /api/v1/services/{id}); sin
 * lógica (invariante). Va por POST porque el checkOrigin CSRF de Astro rechaza
 * PATCH con 403 (mismo patrón que /api/project-runtime).
 *
 * Hasta ahora un servicio era inmutable: un cron mal programado había que
 * borrarlo y crearlo otra vez. El `config` se MEZCLA en el servidor, así que aquí
 * solo viajan las claves que se editan. La validación real (cron válido, imagen
 * anclada al repo del proyecto, URL sin destinos internos) la hace el
 * control-plane. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../../lib/auth.js';
import { updateManagedService, httpErrorInfo } from '../../../lib/cloud.js';

export const prerender = false;

/** Claves editables, por si el navegador manda de más. El resto las pone el aprovisionamiento. */
const EDITABLES = ['backup_schedule', 'schedule', 'url', 'method', 'image', 'command'] as const;

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { service?: unknown; name?: unknown; config?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }
  const service = typeof body.service === 'string' ? body.service.trim() : '';
  if (!service) return json({ error: 'missing_service' }, 400);

  const patch: { name?: string; config?: Record<string, string> } = {};
  if (typeof body.name === 'string' && body.name.trim() !== '') patch.name = body.name.trim();
  if (typeof body.config === 'object' && body.config !== null) {
    const src = body.config as Record<string, unknown>;
    const cfg: Record<string, string> = {};
    for (const k of EDITABLES) {
      const v = src[k];
      if (typeof v === 'string') cfg[k] = v.trim().slice(0, 2048);
    }
    if (Object.keys(cfg).length > 0) patch.config = cfg;
  }
  if (!patch.name && !patch.config) return json({ error: 'no_changes' }, 400);

  try {
    const live = await updateManagedService(session, service, patch);
    return json({ ok: true, live });
  } catch (err) {
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
