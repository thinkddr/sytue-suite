/**
 * POST /api/upload — sube un fichero a Sytue Drive (proxy BFF) y devuelve su
 * fileId + metadatos, para adjuntarlo luego a un mensaje (POST /api/send con
 * file_ids). El navegador envía los bytes como application/octet-stream con
 * ?name= y ?type= (evita la protección CSRF de Astro con content-types de
 * formulario); el BFF los reenvía a la API REST existente de Drive con el token
 * de la sesión httpOnly — que NUNCA llega al cliente.
 *
 * Con ?channel=<uuid> el fichero se sube al ÁMBITO DEL CANAL: solo lo leen sus
 * miembros. Sin canal va a la raíz del tenant (activo de la organización, p. ej. la
 * imagen de un emoji personalizado).
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { DriveClient, bearer, upstreamStatus } from '@sytue/api-clients';
import {
  driveBaseUrl,
  driveRootFolderId,
  driveUpload,
  driveChannelUpload,
  DriveError,
} from '../../lib/drive.js';

export const prerender = false;

/**
 * Tope declarado por el BFF (2 GiB por defecto; configurable vía política de Admin o env).
 * Solo se aplica al `content-length` anunciado, para cortar pronto y sin gastar red; el tope REAL
 * lo pone Drive (`MAX_UPLOAD_MB`, 5 GiB por defecto) y responde 413, que aquí se propaga tal cual.
 */
const DEFAULT_MAX_MB = 2048;
const envMaxMb = Number.parseInt(process.env.MAX_ATTACHMENT_MB ?? '', 10);
const MAX_BYTES =
  (Number.isFinite(envMaxMb) && envMaxMb > 0 ? envMaxMb : DEFAULT_MAX_MB) * 1024 * 1024;

/** Canal del adjunto (uuid): se valida antes de llamar a Drive. */
const UUID_RE = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  const url = new URL(ctx.request.url);
  const name = (url.searchParams.get('name') ?? '').trim() || 'archivo';
  const type =
    url.searchParams.get('type') ||
    ctx.request.headers.get('content-type') ||
    'application/octet-stream';

  // Rechazo temprano por content-length: cuando viene, evita gastar la subida entera
  // para acabar en un 413. Cuando no viene (chunked), el que corta es Drive.
  const declared = Number.parseInt(ctx.request.headers.get('content-length') ?? '', 10);
  if (Number.isFinite(declared) && declared > MAX_BYTES)
    return json({ error: 'too_large', maxBytes: MAX_BYTES }, 413);
  if (declared === 0) return json({ error: 'empty' }, 400);

  // El cuerpo se REENVÍA en streaming: sin `arrayBuffer()` no hay una copia del
  // fichero en el montón del proceso SSR, que era lo que obligaba a un tope pequeño
  // y lo que podía tumbarlo con varias subidas a la vez.
  const body = ctx.request.body;
  if (!body) return json({ error: 'empty' }, 400);

  try {
    // Con ?channel= el fichero se sube al ÁMBITO DEL CANAL: Drive lo saca de «Mi
    // unidad» (que es un espacio compartido de toda la organización) y solo lo sirve
    // a los miembros del canal. Sin canal se sube a la raíz: es el caso de la imagen
    // de un emoji personalizado, que SÍ es un activo del tenant.
    const channelId = (url.searchParams.get('channel') ?? '').trim();
    if (channelId && !UUID_RE.test(channelId)) return json({ error: 'bad_channel' }, 400);
    const file = channelId
      ? await driveChannelUpload(session.accessToken, channelId, name, type, body)
      : await driveUpload(
          session.accessToken,
          await driveRootFolderId(session.accessToken, ctx.request.signal),
          name,
          type,
          body,
          ctx.request.signal,
        );
    return json(
      {
        file: {
          id: file.id,
          fileId: file.id,
          name: file.name,
          contentType: file.content_type,
          sizeBytes: file.size_bytes,
        },
      },
      201,
    );
  } catch (e) {
    // 413 (demasiado grande) y 507 (cuota) del servicio se preservan; el resto es 502.
    const status =
      e instanceof DriveError && (e.status === 413 || e.status === 507) ? e.status : 502;
    return json({ error: 'upload_failed' }, status);
  }
};

/** Compensación de una subida nueva que perdió la carrera de importación. Drive
 * aplica los permisos del tenant y usuario; el cliente pasa solo su archivo nuevo. */
export const DELETE: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);
  const fileId = ctx.url.searchParams.get('fileId') ?? '';
  if (!UUID_RE.test(fileId)) return json({ error: 'bad_file' }, 400);
  try {
    const drive = new DriveClient({ baseUrl: driveBaseUrl(), auth: bearer(session.accessToken) });
    await drive.removeFileById(fileId, ctx.request.signal);
    return new Response(null, { status: 204 });
  } catch (err) {
    return json({ error: 'delete_failed' }, upstreamStatus(err));
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
