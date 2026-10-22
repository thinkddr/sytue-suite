/**
 * POST /api/intent — resuelve un intent cross-app via Sytue Hub (docs/CONVENTIONS.md §11).
 * Proxy BFF autorizado por la cookie de sesión httpOnly: reenvía la petición al
 * Hub (POST /api/v1/intents) con el access token de la sesión y devuelve el
 * IntentResult (deep-link primario + metadatos). El token JAMÁS llega al navegador.
 *
 * Contrato canónico del Hub: { type, params, ids } -> { type, url, target, label, links }.
 * Mail solo despacha sus intents (allowlist); el Hub revalida y NO ejecuta la
 * acción: compone el deep-link y el servicio destino aplica su autorización.
 *
 * CSRF: POST con content-type application/json (dispara preflight; una web ajena
 * no puede enviarlo sin CORS) + cookie de sesión same-origin, como el resto de BFF.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { serverEnv } from '@sytue/auth/server';

export const prerender = false;

const env = serverEnv(import.meta.env);

/**
 * El Hub, POR DENTRO. Esto corre en un pod, y desde ahí el nombre público no
 * resuelve: `getaddrinfo ETIMEOUT hub.sytue.eu`, medido el 2026-09-10. Salía
 * como 502 —«la llamada no llegó a ser HTTP»— y por el Service interno
 * contesta. Mismo patrón que `SYTUE_MAIL_INTERNAL_URL`, con respaldo a la
 * pública para no romper un despliegue sin la variable nueva.
 */
function hubInternalUrl(): string {
  return env.SYTUE_HUB_INTERNAL_URL ?? env.SYTUE_HUB_BASE_URL ?? 'https://hub.sytue.eu';
}

/** Intents que Mail INICIA (el Hub revalida). Adjuntar desde Drive no pasa por
 *  aquí: el compositor lista Drive y adjunta el enlace directamente (/api/drive-files). */
const ALLOWED = new Set(['create-event-from-mail']);

interface IntentRequest {
  type?: unknown;
  params?: unknown;
  ids?: unknown;
}

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: IntentRequest | null;
  try {
    body = (await ctx.request.json()) as IntentRequest;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }
  if (!body || typeof body.type !== 'string' || !ALLOWED.has(body.type)) {
    return json({ error: 'invalid_intent' }, 400);
  }

  // Sanea la forma antes de reenviar: params solo cadenas, ids solo cadenas.
  const params: Record<string, string> = {};
  if (body.params && typeof body.params === 'object') {
    for (const [k, v] of Object.entries(body.params as Record<string, unknown>)) {
      if (typeof v === 'string') params[k] = v;
    }
  }
  const req = {
    type: body.type,
    params,
    ...(Array.isArray(body.ids)
      ? { ids: body.ids.filter((s): s is string => typeof s === 'string') }
      : {}),
  };

  let res: Response;
  try {
    res = await fetch(`${hubInternalUrl()}/api/v1/intents`, {
      method: 'POST',
      headers: {
        authorization: `Bearer ${session.accessToken}`,
        'content-type': 'application/json',
        accept: 'application/json',
      },
      body: JSON.stringify(req),
    });
  } catch {
    return json({ error: 'hub_unreachable' }, 502);
  }
  // 400 del Hub (parámetros inválidos) se preserva; el resto se colapsa a 502.
  if (!res.ok) return json({ error: 'hub_error' }, res.status === 400 ? 400 : 502);
  return json(await res.json());
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
