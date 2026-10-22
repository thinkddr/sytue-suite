/**
 * Cliente de navegador de Sytue Rooms (la llamada embebida por canal de Chat):
 * habla con el endpoint BFF same-origin (cookie). El servidor reenvia al servicio
 * Go Sytue Meet con el token federado y devuelve la conexion de LiveKit (serverUrl
 * + token JWT minteado tras validar OIDC). Asi el access token de Sytue ID no toca
 * el navegador. Reexporta los tipos de dominio desde src/lib/meet.ts (VideoRoom
 * consume LiveKitConnection desde aqui).
 */
import { roomsMatrix } from './matrix/client.js';
import type { MatrixCallScope } from './matrix/types.js';
import type { LiveKitConnection, ChannelCall } from '../lib/meet.js';

export type { LiveKitConnection, ChannelCall };

async function postJson<T>(url: string, body: unknown): Promise<T> {
  const res = await fetch(url, {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify(body),
  });
  if (res.status === 401) {
    window.location.assign(
      withBase(
        import.meta.env.BASE_URL,
        `/auth/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`,
      ),
    );
    throw new Error('No autenticado');
  }
  if (!res.ok) {
    // El código viaja en el error para que quien llama pueda distinguir «la sala
    // está bloqueada» —que se responde llamando a la puerta— de un fallo real.
    let code = '';
    try {
      code = ((await res.json()) as { error?: string }).error ?? '';
    } catch {
      /* cuerpo vacío: nos queda el estado */
    }
    throw new LlamadaError(`Error ${res.status}`, res.status, code);
  }
  return (await res.json()) as T;
}

/** Error de una llamada al BFF con el código del servicio conservado. */
export class LlamadaError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
  ) {
    super(message);
    this.name = 'LlamadaError';
  }
}

async function getJson<T>(url: string): Promise<T> {
  const res = await fetch(url, { credentials: 'include', headers: { accept: 'application/json' } });
  if (res.status === 401) {
    window.location.assign(
      withBase(
        import.meta.env.BASE_URL,
        `/auth/login?returnTo=${encodeURIComponent(location.pathname + location.search)}`,
      ),
    );
    throw new Error('No autenticado');
  }
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as { error?: string } | null;
    throw new LlamadaError(`Error ${res.status}`, res.status, body?.error ?? '');
  }
  return (await res.json()) as T;
}

/** Inicia la llamada del canal: el BFF crea/reutiliza la sala y devuelve la conexion. */
export function startCall(channelId: string, title: string): Promise<ChannelCall> {
  return postJson('/api/call', { channelId, title, matrixDeviceId: roomsMatrix().deviceId });
}

/**
 * Llama a la puerta de la sala cerrada de un canal. No devuelve permiso: devuelve
 * que el anfitrión ya te ve en la cola.
 */
export function knockChannelCall(channelId: string): Promise<{ ok: boolean }> {
  return postJson('/api/call', { channelId, action: 'knock' });
}

/** ¿Cuántos participantes hay AHORA MISMO en la llamada del canal? 0 = sin llamada.
 *  Sondeo ligero para el indicador de «llamada en curso» de la cabecera. */
export function getActiveCall(channelId: string): Promise<{ participants: number }> {
  return getJson(`/api/call-active?channelId=${encodeURIComponent(channelId)}`);
}

// ---------------------------------------------------------------------------
// Reuniones sueltas (sin canal). Mismo BFF, otras puertas: `/api/meetings` para
// listar y convocar, `/api/meeting` para entrar, cerrar y moderar.
// ---------------------------------------------------------------------------
import type { Meeting, MeetingParticipant, MeetingRecording } from '../lib/meetingTypes.js';
/*
 * `withBase` en la vuelta al login: la app se sirve bajo su prefijo en el dominio
 * de organización (`<org>.sytue.eu/mail`, `/calendar`…), y ahí `/auth/login` a pelo
 * es la raíz del host, que devuelve el 404 de Traefik. O sea: al caducar la sesión
 * se acababa en una página de error en vez de en el formulario. El shim de `fetch`
 * (packages/ui/src/basePath.ts) no llega hasta aquí porque esto no es un fetch.
 */
import { withBase } from '@sytue/ui';

export type { Meeting, MeetingParticipant, MeetingRecording };

/** Convocar devuelve la conexión de LiveKit MÁS la reunión creada. */
export interface MeetingCall extends ChannelCall {
  meeting: Meeting;
}

/** Reuniones del tenant. Por defecto las activas: son a las que uno puede unirse. */
export function listMeetings(
  status: Meeting['status'] = 'active',
): Promise<{ meetings: Meeting[] }> {
  return getJson(`/api/meetings?status=${encodeURIComponent(status)}`);
}

/** Convoca una reunión suelta y entra. Título vacío ⇒ el BFF pone uno reconocible. */
export function createMeeting(title: string): Promise<MeetingCall> {
  return postJson('/api/meetings', { title, matrixDeviceId: roomsMatrix().deviceId });
}

/** Entra en una reunión por su id y la clave de su enlace (la de una reunión suelta). */
export function joinMeeting(id: string, clave = ''): Promise<ChannelCall> {
  return postJson('/api/meeting', {
    id,
    action: 'join',
    clave,
    matrixDeviceId: roomsMatrix().deviceId,
  });
}

/** Quién está dentro y qué se grabó. Una sola llamada: la vista pinta las dos cosas. */
export function meetingDetail(
  id: string,
): Promise<{ participants: MeetingParticipant[]; recordings: MeetingRecording[] }> {
  return getJson(`/api/meeting-detail?id=${encodeURIComponent(id)}`);
}

/** Cierra la reunión para todos. El servicio Go valida que seas host o admin. */
export function closeMeeting(id: string): Promise<{ ok: boolean }> {
  return postJson('/api/meeting', { id, action: 'close' });
}

/** Expulsa a un participante. `identity` es su sub de Sytue ID en la sala. */
export function removeMeetingParticipant(id: string, identity: string): Promise<{ ok: boolean }> {
  return postJson('/api/meeting', { id, action: 'remove', identity });
}

/**
 * Le corta el micrófono a un participante, sin echarlo de la reunión. Es lo que
 * hace falta para un micro abierto, donde expulsar es desproporcionado.
 */
export function muteMeetingParticipant(id: string, identity: string): Promise<{ ok: boolean }> {
  return postJson('/api/meeting', { id, action: 'mute', identity });
}

/**
 * Silencia a todos los que están dentro MENOS a quien lo pide. Es lo que hace
 * falta cuando alguien deja el micro abierto en una sala de treinta personas y no
 * se sabe quién es.
 */
export function muteAllMeetingParticipants(id: string): Promise<{ muted: number }> {
  return postJson('/api/meeting', { id, action: 'mute-all' });
}

/**
 * Retira o devuelve la palabra. Silenciar se deshace desde el otro lado pulsando
 * el botón del micrófono; esto no.
 */
export function setMeetingSpeech(
  id: string,
  identity: string,
  puede: boolean,
): Promise<{ ok: boolean }> {
  return postJson('/api/meeting', {
    id,
    action: puede ? 'grant-speech' : 'revoke-speech',
    identity,
  });
}

/**
 * Llama a la puerta de una sala bloqueada. Que responda no significa que puedas
 * entrar: significa que el anfitrión ya te ve en la cola.
 */
export function knockMeeting(id: string, clave = ''): Promise<{ ok: boolean }> {
  return postJson('/api/meeting', { id, action: 'knock', clave });
}

/** Quién espera en la puerta. Solo lo responde el servicio a quien puede abrir. */
export function listMeetingKnocks(
  id: string,
): Promise<{ knocks: { identity: string; name?: string }[] }> {
  return postJson('/api/meeting', { id, action: 'knocks' });
}

/** Abre la puerta a alguien, o lo retira de la cola. */
export function answerMeetingKnock(
  id: string,
  identity: string,
  admitir: boolean,
): Promise<{ ok: boolean }> {
  return postJson('/api/meeting', { id, action: admitir ? 'admit' : 'deny', identity });
}

/**
 * Cierra o reabre la sala. Con la puerta echada no entra nadie nuevo: es lo que
 * hace falta después de expulsar a alguien que se coló por el enlace, porque si
 * no vuelve a entrar antes de que te dé tiempo a seguir hablando.
 */
export function lockMeeting(id: string, locked: boolean): Promise<{ ok: boolean }> {
  return postJson('/api/meeting', { id, action: locked ? 'lock' : 'unlock' });
}

/**
 * Arranca la grabación de la reunión. El fichero NO existe al volver de aquí: el
 * grabador lo sube al terminar. Lo que se devuelve es el identificador con el
 * que se puede parar.
 */
export function startMeetingRecording(id: string): Promise<{ egressId?: string | undefined }> {
  return postJson('/api/meeting', { id, action: 'record' });
}

/** Detiene la grabación en curso. */
export function stopMeetingRecording(id: string): Promise<{ ok: boolean }> {
  return postJson('/api/meeting', { id, action: 'stop-record' });
}

export function callEncryption(roomId: string, joinId: string): Promise<MatrixCallScope> {
  const query = new URLSearchParams({ roomId, joinId, matrixDeviceId: roomsMatrix().deviceId });
  return getJson(`/api/call-encryption?${query}`);
}
export function readyCallEncryption(
  roomId: string,
  joinId: string,
  generation: number,
): Promise<void> {
  return postJson('/api/call-encryption', {
    roomId,
    joinId,
    generation,
    matrixDeviceId: roomsMatrix().deviceId,
    action: 'ready',
  });
}
export function reconnectCallEncryption(
  roomId: string,
  joinId: string,
  generation: number,
): Promise<LiveKitConnection> {
  return postJson('/api/call-encryption', {
    roomId,
    joinId,
    generation,
    matrixDeviceId: roomsMatrix().deviceId,
    action: 'reconnect',
  });
}
