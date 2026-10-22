/*
 * POST /api/alerts
 *   { event, channel_url }        -> crea una política (alcance organización)
 *   { policy_id, delete: true }   -> elimina una política
 *
 * BFF de alertas por evento del tenant (requiere rol admin/owner en el
 * control-plane). El borrado viaja por POST porque el checkOrigin CSRF de Astro
 * rechaza DELETE con 403 (mismo patrón que /api/env). Devuelve { ok, live }:
 * live=false cuando no hay backend vivo y el cambio queda en demostración. El
 * anti-SSRF real del webhook lo impone el control-plane (ValidateWebhookURL);
 * aquí solo validamos evento y forma de URL. NUNCA expone tokens.
 */
import type { APIRoute } from 'astro';
import { bff, toBffContext } from '../../lib/auth.js';
import { createAlertPolicy, deleteAlertPolicy, httpErrorInfo } from '../../lib/cloud.js';
import type { AlertEvent } from '../../lib/types.js';

export const prerender = false;

const EVENTS: readonly AlertEvent[] = [
  'deploy.failed',
  'deploy.ready',
  'uptime.down',
  'uptime.up',
  'billing.spend_threshold',
  'sizing.advice',
];

/**
 * Canal admitido: una URL http(s) —el anti-SSRF por host lo hace el
 * control-plane— o un `mailto:` con una dirección.
 *
 * El correo entra como canal y no como campo aparte porque el modelo ya es una
 * URL: sin columna nueva y sin un segundo camino que mantener.
 */
function isAlertChannel(raw: string): boolean {
  try {
    const u = new URL(raw);
    if (u.protocol === 'mailto:') {
      // `new URL('mailto:x')` acepta casi cualquier cosa: la forma de la
      // dirección se comprueba aparte (y el control-plane la vuelve a mirar).
      const addr = u.pathname.split('?')[0]?.trim() ?? '';
      return /^[^@\s]+@[^@\s.]+(\.[^@\s.]+)+$/.test(addr);
    }
    return u.protocol === 'https:' || u.protocol === 'http:';
  } catch {
    return false;
  }
}

export const GET: APIRoute = () => json({ error: 'method_not_allowed' }, 405);

export const POST: APIRoute = async (ctx) => {
  const session = await bff.getSession(toBffContext(ctx));
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: {
    event?: unknown;
    channel_url?: unknown;
    policy_id?: unknown;
    delete?: unknown;
  };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'invalid_body' }, 400);
  }

  try {
    if (body.delete === true) {
      const id = typeof body.policy_id === 'string' ? body.policy_id.trim() : '';
      if (!id) return json({ error: 'missing_policy_id' }, 400);
      const live = await deleteAlertPolicy(session, id);
      return json({ ok: true, live });
    }

    const event = typeof body.event === 'string' ? (body.event as AlertEvent) : undefined;
    if (!event || !EVENTS.includes(event)) return json({ error: 'invalid_event' }, 400);
    const channel = typeof body.channel_url === 'string' ? body.channel_url.trim() : '';
    if (!isAlertChannel(channel)) return json({ error: 'invalid_channel_url' }, 400);

    const res = await createAlertPolicy(session, { event, channelUrl: channel });
    return json({ ok: true, live: res.live, policy: res.policy });
  } catch (err) {
    // Error REAL del control-plane (forbidden 403 si no es admin, conflict...): status real a la UI.
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
