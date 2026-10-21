/**
 * POST /api/call — inicia la llamada de un canal de Sytue Rooms. Proxy BFF PURO
 * (autorizado por cookie) hacia el servicio Go Sytue Meet: crea o reutiliza la sala
 * del canal y mintea el JWT de LiveKit. Devuelve { connection, roomTitle } para que
 * el navegador conecte el LiveKit web SDK. El access token de Sytue ID nunca llega
 * al navegador. CERO logica de negocio: el reuse/create de la sala lo decide Go, no
 * este endpoint. docs/CONVENTIONS.md §8.
 *
 * Cuerpo: { channelId: string, title: string }
 *         { channelId: string, action: 'knock' } → llama a la puerta si está cerrada
 *         { documentId: string, title: string } → la llamada de un documento de Sytue Docs
 *             (solo su id y su título; la ACL del documento la aplica Docs, no este endpoint)
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { meetClient, MeetError } from '../../lib/meet.js';

import { isMatrixDeviceId } from '../../client/matrix/types.js';
export const prerender = false;

const RE_UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: {
    channelId?: string;
    documentId?: string;
    title?: string;
    action?: string;
    matrixDeviceId?: string;
  };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const documentId = payload.documentId?.trim();
  if (documentId && !RE_UUID.test(documentId)) return json({ error: 'invalid_documentId' }, 400);
  const channelId = payload.channelId?.trim();
  if (!channelId && !documentId) return json({ error: 'missing_channelId' }, 400);
  // El servicio Go exige un titulo no vacio para la sala; el nombre del canal lo
  // aporta el cliente, pero caemos a uno legible si llegara vacio (no es logica de
  // negocio: solo saneo del input hacia el contrato del servicio).
  const title = payload.title?.trim() || (documentId ? 'Documento' : 'Sala del canal');

  const meet = meetClient({ accessToken: session.accessToken });
  if (payload.action === 'knock' && channelId) {
    try {
      await meet.knockChannel(channelId);
      return json({ ok: true });
    } catch (err) {
      if (err instanceof MeetError && err.code) return json({ error: err.code }, err.status);
      return json({ error: 'meet_error', message: String(err) }, 502);
    }
  }

  const device = payload.matrixDeviceId;
  if (!isMatrixDeviceId(device)) return json({ error: 'missing_matrix_device_id' }, 400);
  try {
    const call = documentId
      ? await meet.callForSource('document', documentId, title, session.user.name, device)
      : await meet.callForChannel(channelId as string, title, session.user.name, device);
    return json(call);
  } catch (err) {
    // Igual que en /api/meeting: el código del servicio se propaga. Aquí importa
    // especialmente, porque la llamada de canal es la vía por la que uno se
    // encuentra la puerta cerrada.
    if (err instanceof MeetError && err.code) {
      return json({ error: err.code }, err.status);
    }
    return json({ error: 'meet_error', message: String(err) }, 502);
  }
};

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { 'content-type': 'application/json; charset=utf-8', 'cache-control': 'no-store' },
  });
}
