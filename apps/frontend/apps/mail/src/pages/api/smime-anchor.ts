/**
 * GET /api/smime-anchor → el certificado de la autoridad S/MIME de la organización.
 *
 * PARA QUÉ SIRVE. El correo sale firmado con un certificado emitido por una
 * autoridad propia de la organización —no hay CA pública que emita S/MIME de
 * dominio gratis y automatizado—, así que cualquier cliente que no la conozca
 * enseña «el certificado no es fiable» en cada mensaje. Instalar este fichero
 * UNA VEZ en el dispositivo es lo que convierte esa advertencia en «firmado y de
 * confianza».
 *
 * Proxy BFF autorizado por cookie: el access token nunca llega al navegador. Se
 * reenvía tal cual, con el `Content-Disposition` que hace que el navegador lo
 * descargue en vez de pintarlo —el llavero del sistema es quien lo instala—.
 *
 * ES UN CERTIFICADO PÚBLICO, no una clave: es el ancla que se reparte. Va
 * autenticado igual que el resto para no revelar qué organizaciones existen en
 * la instalación.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailGet } from '../../lib/mailrest.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let res: Response;
  try {
    res = await mailGet(session.accessToken, '/api/v1/smime/anchor');
  } catch (err) {
    return json({ error: 'mail_error', message: String(err) }, 502);
  }
  // El 404 del servicio es «esta organización aún no tiene autoridad», que se
  // crea con el primer correo firmado. Se propaga tal cual: no es una avería.
  if (!res.ok)
    return json({ error: 'mail_error', status: res.status }, res.status === 404 ? 404 : 502);

  return new Response(await res.text(), {
    headers: {
      'content-type': 'application/x-pem-file',
      'content-disposition': 'attachment; filename="sytue-smime-ca.pem"',
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
