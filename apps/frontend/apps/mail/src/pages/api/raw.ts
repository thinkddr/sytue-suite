/**
 * GET /api/raw?emailId=…&name=… → descarga el mensaje ORIGINAL (.eml, RFC 5322).
 *
 * El servicio Go guarda el mensaje entero tal como llegó y lo sirve desde el
 * primer día (GET /api/v1/messages/{id}/raw), pero no había por dónde pedirlo:
 * lo que la app sabía dar era el cuerpo ya interpretado y los adjuntos sueltos.
 * Con el original en la mano se conserva el correo como prueba —cabeceras,
 * firmas DKIM y todo—, se abre en Thunderbird o Apple Mail, y se puede reenviar
 * a quien lo pida sin que la maqueta pase por aquí.
 *
 * Proxy BFF autorizado por cookie; el access token nunca llega al navegador. El
 * cuerpo se REENVÍA en streaming: un mensaje con adjuntos pesa megas y no tiene
 * por qué caber entero en la memoria del BFF.
 *
 * `name` es solo cosmético (el asunto, para que el fichero no se llame como un
 * UUID) y se desinfecta AQUÍ: es texto que viene del navegador y acaba dentro de
 * una cabecera de respuesta.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet, nombreArchivo } from '../../lib/mailrest.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const emailId = ctx.url.searchParams.get('emailId');
  if (!emailId) return json({ error: 'missing_emailId' }, 400);

  let res: Response;
  try {
    res = await mailGet(session.accessToken, `/api/v1/messages/${encodeURIComponent(emailId)}/raw`);
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
  // El 404 del servicio también cubre «no es tuyo» (owner-scoped): se propaga tal
  // cual para no revelar si el mensaje existe en la cuenta de otra persona.
  if (!res.ok || !res.body) {
    return json({ error: 'mail_error', status: res.status }, res.status === 404 ? 404 : 502);
  }

  const nombre = nombreArchivo(ctx.url.searchParams.get('name'), emailId);
  // Doble forma (RFC 6266): `filename` ASCII para los clientes viejos y
  // `filename*` en UTF-8 para que los acentos del asunto lleguen enteros.
  const ascii = nombre.replace(/[^\x20-\x7e]/g, '_');
  return new Response(res.body, {
    headers: {
      'content-type': 'message/rfc822',
      'content-disposition': `attachment; filename="${ascii}"; filename*=UTF-8''${encodeURIComponent(nombre)}`,
      'cache-control': 'no-store',
    },
  });
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
