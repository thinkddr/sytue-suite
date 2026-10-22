/**
 * GET /api/my-apps — apps conectadas (integraciones) de la org, como tiles de producto.
 *
 * Proxy BFF autorizado por la cookie de sesión httpOnly: resuelve el access token en el
 * servidor y consulta el Sytue Integration Gateway (GET /api/v1/my-apps — instancias
 * ACTIVAS de la org con su URL de lanzamiento resuelta). El token JAMÁS llega al
 * navegador. docs/CONVENTIONS.md §11.
 *
 * Degradación: si el gateway no responde, devolvemos lista vacía (200): el launcher
 * muestra solo las apps propias, sin romper. Es info de conveniencia, no crítica.
 */
import type { APIRoute } from 'astro';
import { getApiSession, gatewayBaseUrl } from '../../lib/auth';

export const prerender = false;

export interface MyApp {
  key: string;
  name: string;
  description: string;
  href: string;
  iconUrl: string;
  color: string;
  status: string;
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  try {
    const res = await fetch(`${gatewayBaseUrl().replace(/\/$/, '')}/api/v1/my-apps`, {
      headers: { authorization: `Bearer ${session.accessToken}` },
    });
    if (!res.ok) return json({ apps: [] });
    const wire = (await res.json()) as { apps?: MyApp[] };
    return json({ apps: Array.isArray(wire.apps) ? wire.apps : [] });
  } catch {
    return json({ apps: [] });
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
