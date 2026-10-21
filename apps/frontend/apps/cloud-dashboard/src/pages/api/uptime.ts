/*
 * POST /api/uptime
 *   { project, name, target_url, interval_seconds }                 -> crea una sonda
 *   { check_id, name?, target_url?, interval_seconds?, enabled? }    -> actualiza (parcial)
 *   { check_id, delete: true }                                       -> elimina
 *
 * BFF de monitorización de uptime del proyecto. El borrado y la edición viajan
 * por POST porque el checkOrigin CSRF de Astro rechaza DELETE/PATCH con 403
 * (mismo patrón que /api/env). Devuelve { ok, live }: live=false cuando no hay
 * backend vivo y el cambio queda en demostración. El anti-SSRF real del objetivo
 * lo impone el control-plane (prober); aquí solo validamos la forma. NUNCA expone
 * tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import {
  createUptimeCheck,
  updateUptimeCheck,
  deleteUptimeCheck,
  httpErrorInfo,
} from '../../lib/cloud.js';

export const prerender = false;

const MIN_INTERVAL = 30;
const MAX_INTERVAL = 86400;
const MAX_NAME = 120;

/** URL pública https bien formada (el anti-SSRF por host lo hace el control-plane). */
function isHttpsUrl(raw: string): boolean {
  try {
    return new URL(raw).protocol === 'https:';
  } catch {
    return false;
  }
}

function validInterval(n: unknown): n is number {
  return typeof n === 'number' && Number.isInteger(n) && n >= MIN_INTERVAL && n <= MAX_INTERVAL;
}

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: {
    project?: unknown;
    check_id?: unknown;
    name?: unknown;
    target_url?: unknown;
    interval_seconds?: unknown;
    enabled?: unknown;
    delete?: unknown;
  };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  const checkId = typeof body.check_id === 'string' ? body.check_id.trim() : '';

  try {
    // Borrado (por check_id).
    if (body.delete === true) {
      if (!checkId) return json({ error: 'missing_check_id' }, 400);
      const live = await deleteUptimeCheck(session, checkId);
      return json({ ok: true, live });
    }

    // Actualización parcial (por check_id): valida solo los campos presentes.
    if (checkId) {
      const patch: {
        name?: string;
        targetUrl?: string;
        intervalSeconds?: number;
        enabled?: boolean;
      } = {};
      if (body.name !== undefined) {
        const name = typeof body.name === 'string' ? body.name.trim() : '';
        if (!name || name.length > MAX_NAME) return json({ error: 'invalid_name' }, 400);
        patch.name = name;
      }
      if (body.target_url !== undefined) {
        const target = typeof body.target_url === 'string' ? body.target_url.trim() : '';
        if (!isHttpsUrl(target)) return json({ error: 'invalid_target_url' }, 400);
        patch.targetUrl = target;
      }
      if (body.interval_seconds !== undefined) {
        if (!validInterval(body.interval_seconds)) return json({ error: 'invalid_interval' }, 400);
        patch.intervalSeconds = body.interval_seconds;
      }
      // Pausar/reanudar: el sondeador solo recoge las sondas activas, así que
      // esto es lo que calla una alerta durante un mantenimiento sin borrar la
      // sonda ni su historial.
      if (body.enabled !== undefined) {
        if (typeof body.enabled !== 'boolean') return json({ error: 'invalid_enabled' }, 400);
        patch.enabled = body.enabled;
      }
      const live = await updateUptimeCheck(session, checkId, patch);
      return json({ ok: true, live });
    }

    // Creación (por project slug).
    const project = typeof body.project === 'string' ? body.project.trim() : '';
    if (!project) return json({ error: 'missing_project' }, 400);
    const name = typeof body.name === 'string' ? body.name.trim() : '';
    if (!name || name.length > MAX_NAME) return json({ error: 'invalid_name' }, 400);
    const target = typeof body.target_url === 'string' ? body.target_url.trim() : '';
    if (!isHttpsUrl(target)) return json({ error: 'invalid_target_url' }, 400);
    if (!validInterval(body.interval_seconds)) return json({ error: 'invalid_interval' }, 400);

    const res = await createUptimeCheck(session, project, {
      name,
      targetUrl: target,
      intervalSeconds: body.interval_seconds,
    });
    return json({ ok: true, live: res.live, check: res.check });
  } catch (err) {
    // Error REAL del control-plane (conflict/forbidden/invalid_state/...): status real a la UI.
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
