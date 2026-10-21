/**
 * GET /api/export?format=json|csv|xml — «Tus datos»: la copia completa en un ZIP.
 *
 * La arma Sytue Hub, que junta la entrega de cada producto; este BFF solo pone el
 * token de la sesión (que nunca llega al navegador) y pasa el cuerpo TAL CUAL, en
 * streaming. Leerlo entero aquí sería meter en la memoria del pod una copia que
 * puede pesar gigas.
 */
import type { APIRoute } from 'astro';
import { getApiSession, hubInternalUrl } from '../../lib/auth';

export const prerender = false;

const FORMATOS = new Set(['json', 'csv', 'xml']);

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return new Response(null, { status: 401 });

  const pedido = ctx.url.searchParams.get('format') ?? 'json';
  const formato = FORMATOS.has(pedido) ? pedido : 'json';

  let upstream: Response;
  try {
    upstream = await fetch(`${hubInternalUrl()}/api/v1/export?format=${formato}`, {
      headers: { authorization: `Bearer ${session.accessToken}` },
      // Si quien descarga cancela, que el Hub (y cada producto) dejen de trabajar.
      signal: ctx.request.signal,
    });
  } catch {
    return new Response(null, { status: 502 });
  }
  if (!upstream.ok || !upstream.body) {
    // 429 = ya hay una copia preparándose para esta persona: se dice como tal.
    return new Response(null, { status: upstream.status === 429 ? 429 : 502 });
  }
  return new Response(upstream.body, {
    status: 200,
    headers: {
      'content-type': 'application/zip',
      'content-disposition':
        upstream.headers.get('content-disposition') ?? 'attachment; filename="sytue-one.zip"',
      'cache-control': 'no-store',
    },
  });
};
