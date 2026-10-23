/**
 * PUT /api/smime-identity — instala el certificado S/MIME PROPIO del buzón.
 *
 * PARA QUÉ SIRVE. El correo ya sale firmado por la autoridad de la propia
 * organización (ver /api/smime-anchor), pero ningún cliente fuera de Sytue la
 * conoce hasta que alguien instala su ancla a mano: el aviso «no es de
 * confianza» desaparece solo si el remitente instala en su lugar un
 * certificado de una CA PÚBLICA que ya viene de fábrica en los sistemas — el
 * mismo trato que ya recibe cualquier certificado TLS comprado a una CA
 * reconocida, aplicado a la firma del correo.
 *
 * Cuerpo: el PEM (certificado + cadena + clave) tal cual sale de
 * `openssl pkcs12 -in fichero.p12 -nodes`. Sin JSON: reenviarlo tal cual evita
 * escapar saltos de línea de un material que se maneja como fichero. Proxy BFF
 * autorizado por cookie: el access token nunca llega al navegador, y tampoco
 * el propio certificado se guarda aquí — solo se reenvía.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { mailPutText } from '../../lib/mailrest.js';

export const prerender = false;

/** Material S/MIME: unos pocos KB. El servicio ya acota a 128 KiB; se acota
 *  también aquí para no leer más de lo que jamás podría aceptar. */
const MAX_BYTES = 128 * 1024;

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}

export const PUT: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const len = Number(ctx.request.headers.get('content-length') ?? '0');
  if (Number.isFinite(len) && len > MAX_BYTES) return json({ error: 'too_large' }, 413);

  const pem = await ctx.request.text();
  if (!pem.trim()) return json({ error: 'missing_body' }, 400);

  let res: Response;
  try {
    res = await mailPutText(session.accessToken, '/api/v1/smime/identity', pem);
  } catch {
    return json({ error: 'mail_error' }, 502);
  }
  if (!res.ok) {
    const body = (await res.json().catch(() => ({}))) as { error?: { message?: unknown } };
    const message = typeof body.error?.message === 'string' ? body.error.message : '';
    return json(
      { error: 'mail_error', status: res.status, ...(message ? { message } : {}) },
      res.status >= 400 && res.status < 500 ? res.status : 502,
    );
  }
  return json({ ok: true });
};
