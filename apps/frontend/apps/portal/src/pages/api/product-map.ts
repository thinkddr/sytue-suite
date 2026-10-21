/**
 * GET /api/product-map — quién sirve cada producto de la suite en esta organización.
 *
 * La organización puede elegir que un producto lo sirva una aplicación conectada (Google
 * Sheets en vez de Sytue Sheets, Teams en vez de Rooms). El portal necesita ese mapa para
 * que sus teselas y sus accesos rápidos abran lo que se ha elegido y no siempre el
 * producto nativo.
 *
 * Proxy BFF autorizado por la cookie de sesión httpOnly, igual que /api/my-apps: el
 * access token se resuelve en el servidor y JAMÁS llega al navegador.
 *
 * Degradación: si la pasarela no responde, mapa vacío (200) y todo apunta al producto
 * nativo de Sytue. Como mucho el usuario aterriza en el producto de la suite en lugar de
 * en su sustituto; nunca en un enlace roto.
 */
import type { APIRoute } from 'astro';
import { getApiSession, gatewayBaseUrl } from '../../lib/auth';

export const prerender = false;

/** A dónde abrir un producto que sirve una aplicación conectada. */
export interface SubstituteTarget {
  /** Nombre de la aplicación (se enseña junto al del producto: «Sheets · Google»). */
  name: string;
  href: string;
}

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  try {
    const res = await fetch(
      `${gatewayBaseUrl().replace(/\/$/, '')}/api/v1/substitutions/resolved`,
      { headers: { authorization: `Bearer ${session.accessToken}` } },
    );
    if (!res.ok) return json({ products: {} });
    const wire = (await res.json()) as {
      products?: Record<string, { name?: unknown; href?: unknown }>;
    };
    const out: Record<string, SubstituteTarget> = {};
    for (const [product, entry] of Object.entries(wire.products ?? {})) {
      // Sin href no hay adónde ir: se omite en vez de servir una tesela que no lleva a
      // ninguna parte.
      if (typeof entry?.href !== 'string' || !entry.href) continue;
      out[product] = {
        name: typeof entry.name === 'string' ? entry.name : product,
        href: entry.href,
      };
    }
    return json({ products: out });
  } catch {
    return json({ products: {} });
  }
};
