/**
 * GET /api/attachment?fileId=<uuid>[&channel=<uuid>&name=&download=1&thumb=1] —
 * sirve el contenido de un adjunto (fichero de Sytue Drive) al navegador para
 * previsualización o descarga. Proxy BFF sobre la API REST existente de Drive con el
 * token de la sesión httpOnly (que NUNCA llega al cliente). Drive aplica su propia
 * autorización/RLS.
 *
 * Con ?channel= va por la ruta de ÁMBITO DE CANAL de Drive, que exige ser MIEMBRO del
 * canal en el momento de la petición: es la verja de confidencialidad de los adjuntos
 * de un canal privado o un DM. Sin canal (imagen de un emoji personalizado) se sirve
 * la ruta personal de siempre. Con ?thumb=1 se pide la MINIATURA que genera Drive en
 * vez del original completo, propagando ETag/If-None-Match (304).
 *
 * Seguridad (mismo modelo que la vista previa de Drive/Mail): X-Content-Type-Options
 * nosniff + Content-Security-Policy: sandbox en los tipos que se sirven inline
 * (imágenes/PDF/texto), de modo que un SVG con <script> o un HTML no ejecuten nada
 * ni en <img> ni en navegación directa. Todo lo demás baja como adjunto.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { driveDownload, driveChannelDownload, DriveError } from '../../lib/drive.js';

export const prerender = false;

const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export const GET: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return new Response('unauthenticated', { status: 401 });

  const url = new URL(ctx.request.url);
  const fileId = url.searchParams.get('fileId') ?? '';
  if (!UUID_RE.test(fileId)) return new Response('bad_request', { status: 400 });
  const forceDownload = url.searchParams.get('download') === '1';
  const thumb = url.searchParams.get('thumb') === '1';
  const channelId = (url.searchParams.get('channel') ?? '').trim();
  if (channelId && !UUID_RE.test(channelId)) return new Response('bad_request', { status: 400 });
  // Nombre para Content-Disposition: lo aporta el cliente (metadatos del adjunto);
  // se saca cualquier comilla/barra para no romper la cabecera.
  const name =
    (url.searchParams.get('name') ?? 'descarga').replace(/["\\/\r\n]/g, '').slice(0, 255) ||
    'descarga';

  try {
    const upstream = channelId
      ? await channelUpstream(session.accessToken, channelId, fileId, thumb, ctx)
      : await driveDownload(session.accessToken, fileId);
    // Miniatura no modificada: se devuelve el 304 tal cual (con su ETag) para que el
    // navegador reuse la imagen cacheada sin volver a bajar bytes.
    if (upstream.status === 304) {
      const etag = upstream.headers.get('etag');
      return new Response(null, {
        status: 304,
        headers: etag ? { etag, 'cache-control': 'private, max-age=300' } : {},
      });
    }
    let type = upstream.headers.get('content-type') || 'application/octet-stream';
    const headers: Record<string, string> = {
      // Cacheable en privado un rato (el contenido de un fileId es inmutable), pero
      // nunca en caché compartida: lleva datos autorizados por sesión.
      'cache-control': 'private, max-age=300',
      'x-content-type-options': 'nosniff',
    };
    const etag = upstream.headers.get('etag');
    if (etag) headers['etag'] = etag;
    if (forceDownload) {
      headers['content-disposition'] = `attachment; filename="${name}"`;
    } else {
      // Texto a text/plain: servir text/html en el origen de la app con cookies
      // sería XSS almacenado. Solo imágenes/PDF/texto se muestran inline.
      if (type.startsWith('text/') || type === 'application/json')
        type = 'text/plain; charset=utf-8';
      const safe =
        type.startsWith('image/') || type === 'application/pdf' || type.startsWith('text/plain');
      headers['content-disposition'] = `${safe ? 'inline' : 'attachment'}; filename="${name}"`;
      // Cinturón para navegación directa (p. ej. SVG con <script>): documento
      // sandbox sin scripts ni origen; no afecta a <img> ni al visor de PDF.
      if (safe) headers['content-security-policy'] = 'sandbox';
    }
    headers['content-type'] = type;
    const len = upstream.headers.get('content-length');
    if (len) headers['content-length'] = len;
    // Cuerpo en streaming (sin bufferizar en el BFF).
    return new Response(upstream.body, { status: 200, headers });
  } catch (e) {
    const status = e instanceof DriveError && e.status >= 400 && e.status < 500 ? e.status : 502;
    return new Response('download_failed', { status });
  }
};

/**
 * Adjunto de un canal, con la miniatura como CAMINO PREFERENTE y el original como
 * respaldo: Drive solo miniaturiza JPEG/PNG/GIF (responde 415 en WebP, AVIF o SVG),
 * así que en ese caso se sirve el fichero completo en vez de dejar la imagen rota. Un
 * 404 (no eres miembro del canal) NO se reintenta: se propaga tal cual.
 */
async function channelUpstream(
  token: string,
  channelId: string,
  fileId: string,
  thumb: boolean,
  ctx: Parameters<APIRoute>[0],
): Promise<Response> {
  if (!thumb) return driveChannelDownload(token, channelId, fileId);
  try {
    return await driveChannelDownload(token, channelId, fileId, {
      thumb: true,
      ifNoneMatch: ctx.request.headers.get('if-none-match'),
    });
  } catch (e) {
    if (e instanceof DriveError && e.status === 415)
      return driveChannelDownload(token, channelId, fileId);
    throw e;
  }
}
