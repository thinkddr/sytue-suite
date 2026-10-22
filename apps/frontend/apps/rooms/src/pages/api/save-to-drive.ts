/**
 * POST /api/save-to-drive — guarda en «Mi Drive» una copia de un adjunto del canal
 * (proxy BFF hacia Sytue Drive). Equivale al «Save to Drive» de Slack y al «Copiar a»
 * de Teams.
 *
 * POST y no GET porque crea un recurso, y con `channelId` obligatorio: la autorización
 * es la MEMBRESÍA del canal, que es la única verja que ve un adjunto: vive en la carpeta
 * del canal y no es visible desde «Mi unidad». Quien no está en el canal recibe 404 del
 * servicio, sin distinguir si el fichero existe.
 *
 * No duplica bytes: el servidor referencia el mismo blob y crea solo el metadato nuevo,
 * que es lo que hace que el fichero sobreviva a que borren el mensaje.
 */
import type { APIRoute } from 'astro';
import { DriveClient, bearer, HttpError } from '@sytue/api-clients';
import { getApiSession } from '../../lib/session.js';
import { driveBaseUrl } from '../../lib/drive.js';

export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let body: { fileId?: unknown; channelId?: unknown };
  try {
    body = (await ctx.request.json()) as typeof body;
  } catch {
    return json({ error: 'bad_request' }, 400);
  }
  const fileId = typeof body.fileId === 'string' ? body.fileId : '';
  const channelId = typeof body.channelId === 'string' ? body.channelId : '';
  if (!fileId || !channelId) return json({ error: 'bad_request' }, 400);

  const client = new DriveClient({ baseUrl: driveBaseUrl(), auth: bearer(session.accessToken) });
  try {
    const file = await client.saveChannelFileToMyDrive(channelId, fileId);
    return json({ id: file.id, name: file.name }, 201);
  } catch (err) {
    // El 404 del servicio se propaga TAL CUAL: es la respuesta de «no eres miembro» y
    // convertirlo en 500 haría parecer una avería lo que es una decisión de permisos.
    if (err instanceof HttpError) return json({ error: 'drive_error' }, err.status);
    return json({ error: 'drive_error' }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
