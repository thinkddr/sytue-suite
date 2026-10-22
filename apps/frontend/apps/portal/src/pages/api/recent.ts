/**
 * GET /api/recent?limit= — actividad reciente cross-app del portal.
 *
 * Proxy BFF autorizado por la cookie de sesión httpOnly: resuelve el access
 * token en el servidor y consulta el endpoint de actividad reciente del Sytue
 * Hub (GET /api/v1/recent — recientes por origen sin término, agregados y
 * ordenados por recencia). El token JAMÁS llega al navegador. docs/CONVENTIONS.md §11.
 *
 * Degradación: el Hub ya degrada POR ORIGEN (un servicio caído aporta 0 ítems y
 * no rompe el resto). Lista vacía (200) es un estado vacío honesto. Si el Hub
 * entero no responde, devolvemos 502 para que la UI muestre su estado de error
 * con reintento — nunca un mock hardcodeado.
 */
import type { APIRoute } from 'astro';
import type { SearchResult } from '@sytue/api-clients';
import { getApiSession } from '../../lib/auth';
import { hubClient } from '../../lib/hub';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const limit = clampInt(ctx.url.searchParams.get('limit'), 6, 1, 20);

  let items: SearchResult[];
  try {
    // Por el SDK y no con `fetch` a mano: la forma del cable del Hub —y el paso
    // de timestamp RFC3339 a epoch ms— estaba escrita DOS veces, aquí y en el
    // cliente, y dos copias del mismo mapeo divergen en cuanto una cambia.
    // `limit` es POR ORIGEN en el Hub; el agregado se recorta abajo.
    items = await hubClient(session.accessToken).recent({ limit });
  } catch {
    return json({ error: 'hub_unreachable' }, 502);
  }

  return json({ items: items.slice(0, limit) } satisfies { items: SearchResult[] });
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
