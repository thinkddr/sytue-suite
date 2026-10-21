/**
 * POST /api/meeting — operaciones sobre UNA reunión, distinguidas por `action`:
 *   { id, action: 'join', clave? } → entra y devuelve la conexión de LiveKit (la clave
 *                                    es la del enlace de una reunión suelta)
 *   { id, action: 'close' }   → la cierra para todos (host/admin; lo valida Go)
 *   { id, action: 'remove', identity } → expulsa a un participante (host/admin)
 *   { id, action: 'mute', identity }   → le corta el micrófono, sin echarlo
 *   { id, action: 'record' | 'stop-record' } → graba la reunión (host/admin)
 *   { id, action: 'mute-all' }        → calla a todos menos a quien lo pide
 *   { id, action: 'lock' | 'unlock' }  → cierra o reabre la puerta de la sala
 *   { id, action: 'revoke-speech' | 'grant-speech', identity } → quita/devuelve la palabra
 *   { id, action: 'knock' }           → llama a la puerta de una sala bloqueada
 *   { id, action: 'knocks' }          → quién espera (host/admin)
 *   { id, action: 'admit' | 'deny', identity } → responde a la puerta
 *
 * Todo por POST a propósito: el `checkOrigin` (CSRF) de Astro rechaza los DELETE que
 * salen del navegador, así que un verbo semántico aquí costaría un 403 en producción
 * y una hora de depuración. El método REAL contra el servicio Go sí es DELETE.
 *
 * Proxy BFF puro: el access token de Sytue ID no llega al navegador y la autorización
 * (¿es host? ¿es admin?) la decide el servicio Go, no este endpoint.
 */
import type { APIRoute } from 'astro';
import { getApiSession } from '../../lib/session.js';
import { meetClient, MeetError } from '../../lib/meet.js';

import { isMatrixDeviceId } from '../../client/matrix/types.js';
export const prerender = false;

export const POST: APIRoute = async (ctx) => {
  const session = await getApiSession(ctx);
  if (!session) return json({ error: 'unauthenticated' }, 401);

  let payload: {
    id?: string;
    action?: string;
    identity?: string;
    clave?: string;
    matrixDeviceId?: string;
  };
  try {
    payload = (await ctx.request.json()) as typeof payload;
  } catch {
    return json({ error: 'invalid_json' }, 400);
  }

  const id = payload.id?.trim();
  if (!id) return json({ error: 'missing_id' }, 400);

  const meet = meetClient({ accessToken: session.accessToken });
  try {
    switch (payload.action) {
      case 'join': {
        const device = payload.matrixDeviceId;
        if (!isMatrixDeviceId(device)) return json({ error: 'missing_matrix_device_id' }, 400);
        return json(await meet.joinMeeting(id, session.user.name, payload.clave?.trim(), device));
      }
      case 'close':
        await meet.closeMeeting(id);
        return json({ ok: true });
      case 'knock':
        await meet.knock(id, payload.clave?.trim());
        return json({ ok: true });
      case 'knocks':
        return json({ knocks: await meet.knocks(id) });
      case 'admit':
      case 'deny': {
        const identity = payload.identity?.trim();
        if (!identity) return json({ error: 'missing_identity' }, 400);
        await meet.answerKnock(id, identity, payload.action === 'admit');
        return json({ ok: true });
      }
      case 'revoke-speech':
      case 'grant-speech': {
        const identity = payload.identity?.trim();
        if (!identity) return json({ error: 'missing_identity' }, 400);
        await meet.setCanPublish(id, identity, payload.action === 'grant-speech');
        return json({ ok: true });
      }
      case 'remove':
      case 'mute': {
        const identity = payload.identity?.trim();
        if (!identity) return json({ error: 'missing_identity' }, 400);
        if (payload.action === 'mute') await meet.muteParticipant(id, identity);
        else await meet.removeParticipant(id, identity);
        return json({ ok: true });
      }
      case 'lock':
      case 'unlock':
        await meet.lockRoom(id, payload.action === 'lock');
        return json({ ok: true });
      case 'mute-all':
        return json({ muted: await meet.muteAll(id) });
      case 'record':
        return json(await meet.startRecording(id));
      case 'stop-record':
        await meet.stopRecording(id);
        return json({ ok: true });
      default:
        return json({ error: 'unknown_action' }, 400);
    }
  } catch (err) {
    // El código y el estado del servicio se PROPAGAN. Envolverlo todo en un 502
    // genérico borraba la diferencia entre «la sala está bloqueada» —que tiene
    // respuesta: llamar a la puerta— y «el servicio no responde», que no.
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
