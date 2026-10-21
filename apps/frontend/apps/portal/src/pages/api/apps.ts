/**
 * GET /api/apps — qué productos de la suite levanta ESTA instalación.
 *
 * Proxy BFF autorizado por la cookie de sesión httpOnly: resuelve el access token
 * en el servidor y consulta al Hub (GET /api/v1/apps), que lo lee de su propia
 * configuración de despliegue. El token JAMÁS llega al navegador
 * (docs/CONVENTIONS.md §11).
 *
 * PARA QUÉ. No toda instalación despliega la suite entera: el perfil piloto se
 * queda sin Sytue DevX, y su tesela seguía en el lanzador llevando a un
 * `cloud.<dominio>` que ahí no existe. Un acceso que solo puede acabar en error
 * parece una avería del producto.
 *
 * Degradación: lista vacía (200) si el Hub no responde o no lo declara. Vacío
 * significa «no se sabe», y el lanzador presenta la suite entera, que es como se
 * ha comportado siempre. Es preferible ofrecer algo que no está a esconder media
 * suite porque una petición falló.
 */
import type { APIRoute } from 'astro';
import { getApiSession, hubInternalUrl } from '../../lib/auth';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  try {
    const res = await fetch(`${hubInternalUrl().replace(/\/$/, '')}/api/v1/apps`, {
      headers: { authorization: `Bearer ${session.accessToken}` },
    });
    if (!res.ok) return json({ apps: [] });
    const wire = (await res.json()) as { apps?: unknown };
    const apps = Array.isArray(wire.apps) ? wire.apps.filter((a) => typeof a === 'string') : [];
    return json({ apps });
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
