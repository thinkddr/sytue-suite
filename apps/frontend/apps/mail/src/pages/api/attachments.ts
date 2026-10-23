/**
 * GET /api/attachments?emailId=            → lista los adjuntos de un correo leído
 *                                            (JSON: [{ name, type, size, inline, cid? }]).
 * GET /api/attachments?emailId=&index=N     → descarga el adjunto N con
 *                                            Content-Disposition: attachment.
 * GET /api/attachments?emailId=&index=N&inline=1
 *                                          → el mismo adjunto para pintarlo EN
 *                                            LÍNEA (`<img src>`), solo si es una
 *                                            imagen rasterizada.
 *
 * El servicio Go solo guarda el .eml crudo y un flag has_attachment (no expone los
 * adjuntos como blobs por parte —auditado): el SDK descarga el .eml y parsea el
 * MIME. Proxy BFF autorizado por cookie; el access token nunca llega al cliente.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { jmapClient } from '../../lib/jmap.js';

export const prerender = false;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const emailId = ctx.url.searchParams.get('emailId');
  if (!emailId) return json({ error: 'missing_emailId' }, 400);
  const indexRaw = ctx.url.searchParams.get('index');

  let attachments;
  try {
    attachments = await jmapClient(session.accessToken).getAttachments(emailId);
  } catch {
    return json({ error: 'jmap_error', message: 'Mail request failed' }, 502);
  }

  // Sin index: metadatos para la lista del lector.
  if (indexRaw == null) {
    return json({
      // El `cid` va SIEMPRE que la parte lo traiga: es lo que enlaza una imagen
      // en línea con el `<img src="cid:…">` del cuerpo. No es un dato sensible
      // —es un identificador que el propio mensaje lleva escrito— y sin él el
      // lector no puede pintar la imagen.
      attachments: attachments.map((a) => ({
        name: a.name,
        type: a.type,
        size: a.size,
        inline: a.inline,
        ...(a.cid ? { cid: a.cid } : {}),
      })),
    });
  }

  // Con index: descarga de ese adjunto.
  const index = Number.parseInt(indexRaw, 10);
  const att = Number.isInteger(index) ? attachments[index] : undefined;
  if (!att) return json({ error: 'not_found' }, 404);

  const filename = att.name.replace(/["\\\r\n]/g, '');
  // EN LÍNEA SOLO LAS IMÁGENES, Y NI SVG NI NADA QUE EJECUTE. Servir una parte
  // cualquiera con `inline` desde NUESTRO origen es ofrecer alojamiento a lo que
  // haya mandado quien sea: basta con abrir esta URL para que el navegador pinte
  // —y ejecute— un adjunto HTML o un SVG con script, con la sesión de la suite
  // delante. La lista blanca es de formatos que el navegador solo sabe
  // decodificar como píxeles.
  const tipo = (att.type || 'application/octet-stream').split(';')[0]!.trim().toLowerCase();
  const enLinea =
    ctx.url.searchParams.get('inline') === '1' &&
    /^image\/(jpeg|png|gif|webp|avif|bmp|x-icon|vnd\.microsoft\.icon)$/.test(tipo);
  // Copia a un ArrayBuffer propio: BodyInit lo acepta sin fricción de tipos
  // (Uint8Array<ArrayBufferLike> no encaja con el DOM lib estricto).
  const buf = new ArrayBuffer(att.content.byteLength);
  new Uint8Array(buf).set(att.content);
  return new Response(buf, {
    headers: {
      'content-type': att.type || 'application/octet-stream',
      'content-disposition': `${enLinea ? 'inline' : 'attachment'}; filename="${filename}"`,
      'content-length': String(att.content.byteLength),
      'cache-control': 'no-store',
      // Con `inline` el navegador decide qué hacer mirando el tipo: sin esto
      // podría olfatear el contenido y tratar como HTML lo que se anunció como
      // imagen, que es exactamente lo que la lista blanca de arriba evita.
      'x-content-type-options': 'nosniff',
      'content-security-policy': "default-src 'none'; sandbox",
    },
  });
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
