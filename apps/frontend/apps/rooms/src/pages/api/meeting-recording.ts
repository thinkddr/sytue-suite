/**
 * GET /api/meeting-recording?id=…&rec=… — abre una grabación terminada.
 *
 * Responde con un 302 a la URL firmada del bucket, no con el fichero ni con la URL
 * en un JSON. Así el enlace del panel es un `<a href>` de toda la vida: un clic
 * directo, sin bloqueador de ventanas emergentes de por medio (que es lo que pasa
 * al abrir una pestaña DESPUÉS de un `await`), y la URL firmada no se queda escrita
 * en el DOM esperando a que alguien la copie.
 *
 * Proxy BFF puro: quien decide si esta persona puede ver la grabación es el
 * servicio Go —anfitrión, admin o haber estado en la reunión—, no este endpoint.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { meetClient } from '../../lib/meet.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const id = ctx.url.searchParams.get('id')?.trim();
  const rec = ctx.url.searchParams.get('rec')?.trim();
  if (!id || !rec) return json({ error: 'missing_id' }, 400);

  const meet = meetClient({ accessToken: session.accessToken });
  try {
    const url = await meet.recordingURL(id, rec);
    return new Response(null, {
      status: 302,
      // `no-store` también aquí: la URL firmada caduca en minutos y una respuesta
      // cacheada mandaría a la gente a un enlace muerto.
      headers: { location: url, 'cache-control': 'no-store' },
    });
  } catch {
    return json({ error: 'meet_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
