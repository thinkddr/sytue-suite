/**
 * Cliente REST del lado servidor para la VIDEOLLAMADA de Sytue Rooms. Habla con la API
 * de salas del servicio Go (el mismo que sirve la mensajeria desde la fusion) con el
 * access token de Sytue ID (Authorization: Bearer) que solo vive en el servidor. Al
 * unirse, el servicio Go MINTEA el JWT de LiveKit (con su API key/secret) tras validar
 * OIDC y devuelve la URL WSS del SFU + el token para el LiveKit web SDK. El access
 * token de Sytue ID nunca llega al navegador.
 *
 * Dos caminos de entrada a una sala, y ninguno duplica logica en JS:
 *   - la llamada DE UN CANAL (`callForChannel`), que reutiliza la sala del canal;
 *   - la REUNION suelta (`createMeeting`/`joinMeeting`), sin canal detras, para
 *     convocar a gente por enlace igual que en Meet/Zoom.
 * La orquestacion de reuse/create vive en Go: aqui solo se encadenan llamadas.
 *
 * LiveKit (Apache-2.0) es la unica aplicacion de terceros (SFU de medios WebRTC).
 * docs/CONVENTIONS.md §1, §8.
 */
import type { MatrixCallScope } from '../client/matrix/types.js';
import { serverEnv } from '@sytue/auth/server';
import type { Meeting, MeetingParticipant, MeetingRecording } from './meetingTypes.js';

export type { Meeting, MeetingParticipant, MeetingRecording };

/** Datos de conexion a LiveKit que entrega el BFF tras validar OIDC. */
export interface LiveKitConnection {
  encryption?: MatrixCallScope;
  joinId?: string;
  /** URL WSS del SFU LiveKit. */
  serverUrl: string;
  /** JWT de acceso a LiveKit minteado por el servicio Go `sytue-meet`. */
  token: string;
  /** Nombre de la sala. */
  room: string;
  /** Identidad del participante. */
  identity: string;
}

/** Resultado de iniciar la llamada de un canal: conexion LiveKit + titulo de sala. */
export interface ChannelCall {
  connection: LiveKitConnection;
  roomTitle: string;
  /**
   * UUID de la sala. Hace falta para moderar: `connection.room` es el NOMBRE de
   * la sala en LiveKit, y la API de Sytue Rooms se dirige por id.
   */
  roomId: string;
  /**
   * Sub de quien creó la sala. La interfaz lo usa para pintar las acciones de
   * moderar solo a quien puede usarlas; quien AUTORIZA es el servicio Go, que
   * reautoriza cada petición. Ocultar un botón no es una puerta.
   */
  hostIdentity: string;
  /**
   * ¿Está la sala bloqueada AHORA? Se lee al entrar y no se supone `false`: quien
   * la cerró y recarga la página tiene que seguir viendo el candado echado, o
   * pulsará «bloquear» sobre una sala ya bloqueada y la abrirá sin querer.
   */
  locked: boolean;
}

/**
 * Error del servicio con su CÓDIGO conservado.
 *
 * Sin esto, «la sala está bloqueada» llegaba al navegador como un 502 genérico
 * del proxy y no había forma de ofrecer llamar a la puerta: el código se perdía
 * en el `String(err)` del BFF. El código es lo que el cliente traduce; el mensaje
 * del servicio es solo el respaldo.
 */
export class MeetError extends Error {
  constructor(
    message: string,
    readonly status: number,
    readonly code: string,
  ) {
    super(message);
    this.name = 'MeetError';
  }
}

interface MeetClientDeps {
  accessToken: string;
}

/** Sala tal como la devuelve el servicio Go (subconjunto que usamos aqui). */
interface Room {
  id: string;
  name: string;
  title?: string;
  created_by?: string;
  /** Puerta echada: no entra nadie nuevo (el servicio lo omite cuando es false). */
  locked?: boolean;
}

/** Sala en crudo del servicio Go (snake_case) antes de normalizar a `Meeting`. */
interface RawRoom {
  id: string;
  name: string;
  title?: string;
  status?: string;
  created_by?: string;
  started_at?: string;
  ended_at?: string;
  metadata?: { source?: { kind?: string; ref_id?: string } };
  link_key?: string;
}

function toMeeting(r: RawRoom): Meeting {
  const src = r.metadata?.source;
  return {
    id: r.id,
    name: r.name,
    title: r.title || r.name,
    status: (r.status as Meeting['status']) ?? 'scheduled',
    createdBy: r.created_by ?? '',
    startedAt: r.started_at,
    endedAt: r.ended_at,
    channelId: src?.kind === 'chat' ? src.ref_id : undefined,
    linkKey: r.link_key || undefined,
  };
}

/** Participante en crudo del servicio Go, antes de normalizar a `MeetingParticipant`. */
interface RawParticipant {
  id?: string;
  identity?: string;
  name?: string;
  role?: string;
  joined_at?: string;
  left_at?: string;
}

/** Grabación en crudo del servicio Go, antes de normalizar a `MeetingRecording`. */
interface RawRecording {
  id?: string;
  status?: string;
  size_bytes?: number;
  duration_sec?: number;
  started_at?: string;
  ended_at?: string;
}

/** Respuesta de POST /rooms/{id}/join (campos en snake_case del servicio Go). */
interface JoinResult {
  encryption?: MatrixCallScope;
  join_id?: string;
  token: string;
  identity: string;
  livekit_url?: string;
  room?: Room;
}

// La videollamada la sirve el MISMO servicio que la mensajería desde la fusión, así que
// esta base es la de Rooms. SYTUE_MEET_API_URL se sigue leyendo como respaldo.
function apiBase(): string {
  const env = serverEnv(import.meta.env);
  // Por el Service INTERNO: esto corre en un pod y el nombre público no
  // resuelve desde dentro (`getaddrinfo ETIMEOUT`, medido el 2026-09-10). Sale
  // como 502 —«la llamada no llegó a ser HTTP»—. Mismo patrón que
  // `SYTUE_MAIL_INTERNAL_URL`; la pública queda de respaldo para un despliegue
  // que aún no tenga la variable nueva.
  return (
    env.SYTUE_ROOMS_INTERNAL_URL ??
    env.SYTUE_ROOMS_API_URL ??
    env.SYTUE_MEET_API_URL ??
    'https://rooms.sytue.eu'
  );
}

export function meetClient({ accessToken }: MeetClientDeps) {
  const base = apiBase().replace(/\/$/, '');

  // `clave`: la del enlace de una reunión suelta. Va en una cabecera y no en la ruta,
  // que acaba en los registros de acceso de cada salto.
  async function call<T>(method: string, path: string, body?: unknown, clave?: string): Promise<T> {
    const res = await fetch(`${base}/api/v1${path}`, {
      method,
      headers: {
        authorization: `Bearer ${accessToken}`,
        accept: 'application/json',
        ...(body !== undefined ? { 'content-type': 'application/json' } : {}),
        ...(clave ? { 'x-sytue-room-key': clave } : {}),
      },
      body: body !== undefined ? JSON.stringify(body) : null,
    });
    if (!res.ok) {
      let code = '';
      try {
        const cuerpo = (await res.json()) as { error?: { code?: string } | string };
        code = typeof cuerpo.error === 'string' ? cuerpo.error : (cuerpo.error?.code ?? '');
      } catch {
        // Cuerpo vacío o no-JSON: nos quedamos con el estado, que ya dice algo.
      }
      throw new MeetError(`Sytue Meet ${method} ${path} -> ${res.status}`, res.status, code);
    }
    if (res.status === 204) return undefined as T;
    return (await res.json()) as T;
  }

  return {
    encryption(id: string, deviceId: string, joinId: string): Promise<MatrixCallScope> {
      const query = new URLSearchParams({ matrix_device_id: deviceId, join_id: joinId });
      return call('GET', `/rooms/${encodeURIComponent(id)}/encryption?${query}`);
    },
    readyEncryption(
      id: string,
      deviceId: string,
      joinId: string,
      generation: number,
    ): Promise<void> {
      return call('POST', `/rooms/${encodeURIComponent(id)}/encryption/ready`, {
        matrix_device_id: deviceId,
        join_id: joinId,
        generation,
      });
    },
    async reconnectEncryption(
      id: string,
      deviceId: string,
      joinId: string,
      generation: number,
    ): Promise<LiveKitConnection> {
      const conn = await call<JoinResult>(
        'POST',
        `/rooms/${encodeURIComponent(id)}/encryption/reconnect`,
        { matrix_device_id: deviceId, join_id: joinId, generation },
      );
      return {
        token: conn.token,
        identity: conn.identity,
        room: conn.encryption?.livekit_room ?? conn.room?.name ?? '',
        serverUrl: conn.livekit_url ?? '',
        ...(conn.encryption ? { encryption: conn.encryption } : {}),
        ...(conn.join_id ? { joinId: conn.join_id } : {}),
      };
    },
    /**
     * Inicia (o reutiliza) la sala de un canal de Chat y se une. Dos hops, sin nada
     * de logica de dedup en JS — la orquestacion de reuse/create vive en Go:
     *   1) POST /rooms/from-source {kind:'chat', ref_id, title} -> el servicio Go
     *      devuelve o crea la sala del canal.
     *   2) POST /rooms/{id}/join -> mintea el JWT de LiveKit del participante y
     *      devuelve la conexion (serverUrl + token) para el web SDK.
     */
    async callForChannel(
      channelId: string,
      title: string,
      displayName?: string,
      matrixDeviceId?: string,
    ): Promise<ChannelCall> {
      return this.callForSource('chat', channelId, title, displayName, matrixDeviceId);
    },

    /**
     * Lo mismo para cualquier origen del servicio: un canal (`chat`) o un documento de
     * Sytue Docs (`document`, «Llamar desde el documento»). De un documento solo viajan su
     * id y su título; quién puede entrar lo decide su ACL en Docs, no este cliente.
     */
    async callForSource(
      kind: 'chat' | 'document',
      refId: string,
      title: string,
      displayName?: string,
      matrixDeviceId?: string,
    ): Promise<ChannelCall> {
      const room = await call<Room>('POST', '/rooms/from-source', {
        kind,
        ref_id: refId,
        title,
      });
      const conn = await call<JoinResult>('POST', `/rooms/${encodeURIComponent(room.id)}/join`, {
        display_name: displayName,
        matrix_device_id: matrixDeviceId,
      });
      return {
        connection: {
          ...(conn.encryption ? { encryption: conn.encryption } : {}),
          ...(conn.join_id ? { joinId: conn.join_id } : {}),
          token: conn.token,
          identity: conn.identity,
          room: conn.encryption?.livekit_room ?? conn.room?.name ?? room.name,
          serverUrl:
            conn.livekit_url ||
            (serverEnv(import.meta.env).SYTUE_LIVEKIT_URL ?? 'wss://livekit.sytue.eu'),
        },
        roomTitle: conn.room?.title || room.title || title,
        roomId: room.id,
        hostIdentity: room.created_by ?? '',
        locked: conn.room?.locked ?? room.locked ?? false,
      };
    },

    /**
     * ¿Hay una llamada EN CURSO en el canal? Consulta la sala del canal por su origen
     * SIN crearla (GET /rooms/by-source; 404 = no hay sala) y, si existe, cuenta los
     * participantes presentes. Sondeo ligero para el indicador de «llamada en curso»:
     * dos lecturas, cero escrituras. participants=0 => no mostrar banner.
     */
    async activeCall(channelId: string): Promise<{ participants: number }> {
      const res = await fetch(
        `${base}/api/v1/rooms/by-source?kind=chat&ref_id=${encodeURIComponent(channelId)}`,
        { headers: { authorization: `Bearer ${accessToken}`, accept: 'application/json' } },
      );
      if (res.status === 404) return { participants: 0 };
      if (!res.ok) throw new Error(`Sytue Meet GET /rooms/by-source -> ${res.status}`);
      const room = (await res.json()) as Room;
      const parts = await call<{ participants?: unknown[] }>(
        'GET',
        `/rooms/${encodeURIComponent(room.id)}/participants?present=1`,
      );
      return { participants: parts.participants?.length ?? 0 };
    },

    // ---------------------------------------------------------------------------
    // Reuniones sueltas (sin canal). El servicio Go ya las servia; hasta ahora no
    // habia forma de convocar una desde el producto: solo existia la llamada de canal.
    // ---------------------------------------------------------------------------

    /** Convoca una reunion y entra: crea la sala y mintea la conexion en un paso. */
    async createMeeting(
      title: string,
      displayName?: string,
      matrixDeviceId?: string,
    ): Promise<ChannelCall & { meeting: Meeting }> {
      const room = await call<RawRoom>('POST', '/rooms', { title });
      return {
        meeting: toMeeting(room),
        ...(await this.joinMeeting(room.id, displayName, room.link_key, matrixDeviceId)),
      };
    },

    /**
     * Entra en una reunion existente por su id (el que viaja en el enlace). Un 404 o un
     * 403 del servicio Go se propagan tal cual: quien no puede entrar tiene que ver por
     * que, no una sala vacia.
     */
    async joinMeeting(
      id: string,
      displayName?: string,
      clave?: string,
      matrixDeviceId?: string,
    ): Promise<ChannelCall> {
      const conn = await call<JoinResult>(
        'POST',
        `/rooms/${encodeURIComponent(id)}/join`,
        { display_name: displayName, matrix_device_id: matrixDeviceId },
        clave,
      );
      return {
        connection: {
          ...(conn.encryption ? { encryption: conn.encryption } : {}),
          ...(conn.join_id ? { joinId: conn.join_id } : {}),
          token: conn.token,
          identity: conn.identity,
          room: conn.encryption?.livekit_room ?? conn.room?.name ?? '',
          serverUrl:
            conn.livekit_url ||
            (serverEnv(import.meta.env).SYTUE_LIVEKIT_URL ?? 'wss://livekit.sytue.eu'),
        },
        roomTitle: conn.room?.title ?? '',
        roomId: id,
        hostIdentity: conn.room?.created_by ?? '',
        locked: conn.room?.locked ?? false,
      };
    },

    /**
     * Reuniones del tenant. `status` acota el ciclo de vida (activas por defecto: es
     * lo unico a lo que uno puede unirse) y el servicio aplica la RLS del tenant.
     */
    async listMeetings(status: Meeting['status'] = 'active', limit = 50): Promise<Meeting[]> {
      const res = await call<{ rooms?: RawRoom[] }>(
        'GET',
        `/rooms?status=${encodeURIComponent(status)}&limit=${limit}`,
      );
      return (res.rooms ?? []).map(toMeeting);
    },

    /** Quien esta (o estuvo) en la reunion. `present` deja solo a los que siguen dentro. */
    async meetingParticipants(id: string, present = true): Promise<MeetingParticipant[]> {
      const res = await call<{ participants?: RawParticipant[] }>(
        'GET',
        `/rooms/${encodeURIComponent(id)}/participants${present ? '?present=1' : ''}`,
      );
      return (res.participants ?? []).map((p) => ({
        id: p.id ?? '',
        identity: p.identity ?? '',
        name: p.name ?? p.identity ?? '',
        role: (p.role as MeetingParticipant['role']) ?? 'speaker',
        joinedAt: p.joined_at ?? '',
        leftAt: p.left_at,
      }));
    },

    /** Grabaciones de una reunion (metadatos; el fichero vive en Object Storage). */
    async meetingRecordings(id: string): Promise<MeetingRecording[]> {
      const res = await call<{ recordings?: RawRecording[] }>(
        'GET',
        `/rooms/${encodeURIComponent(id)}/recordings`,
      );
      return (res.recordings ?? []).map((r) => ({
        id: r.id ?? '',
        status: (r.status as MeetingRecording['status']) ?? 'started',
        sizeBytes: r.size_bytes ?? 0,
        durationSec: r.duration_sec ?? 0,
        startedAt: r.started_at ?? '',
        endedAt: r.ended_at,
      }));
    },

    /** Expulsa a un participante. Solo host/admin: el servicio Go valida el rol. */
    async removeParticipant(id: string, identity: string): Promise<void> {
      await call<void>(
        'DELETE',
        `/rooms/${encodeURIComponent(id)}/participants/${encodeURIComponent(identity)}`,
      );
    },

    /**
     * Corta el micrófono de un participante sin echarlo (anfitrión o admin: lo
     * valida el servicio Go). Solo silencia sus pistas de audio.
     */
    async muteParticipant(id: string, identity: string): Promise<void> {
      await call<void>(
        'POST',
        `/rooms/${encodeURIComponent(id)}/participants/${encodeURIComponent(identity)}/mute`,
      );
    },

    /**
     * Arranca la grabación (anfitrión o admin: lo valida el servicio Go). El 202
     * es deliberado: cuando esto vuelve el grabador está entrando en la sala, no
     * hay fichero todavía.
     */
    async startRecording(id: string): Promise<{ egressId?: string | undefined }> {
      const r = await call<{ egress_id?: string }>(
        'POST',
        `/rooms/${encodeURIComponent(id)}/recording`,
      );
      return { egressId: r?.egress_id };
    },

    /** Detiene la grabación en curso. */
    async stopRecording(id: string): Promise<void> {
      await call<void>('DELETE', `/rooms/${encodeURIComponent(id)}/recording`);
    },

    /**
     * Cierra o reabre la puerta de la sala. Bloqueada, nadie NUEVO entra; quien
     * ya está dentro se queda (para eso están expulsar y cerrar).
     */
    async lockRoom(id: string, locked: boolean): Promise<void> {
      await call<unknown>(locked ? 'POST' : 'DELETE', `/rooms/${encodeURIComponent(id)}/lock`);
    },

    /**
     * Retira (o devuelve) la palabra a un participante: el escalón entre
     * silenciar —que el otro deshace pulsando su botón— y expulsar.
     */
    async setCanPublish(id: string, identity: string, puede: boolean): Promise<void> {
      await call<void>(
        puede ? 'POST' : 'DELETE',
        `/rooms/${encodeURIComponent(id)}/participants/${encodeURIComponent(identity)}/speech`,
      );
    },

    /**
     * Llama a la puerta de la sala DE UN CANAL. Resuelve la sala por su origen
     * —sin crearla— y llama. Hace falta porque quien se encuentra la puerta
     * cerrada no llegó a recibir el id de la sala: el join falló antes.
     */
    async knockChannel(channelId: string): Promise<void> {
      const res = await fetch(
        `${base}/api/v1/rooms/by-source?kind=chat&ref_id=${encodeURIComponent(channelId)}`,
        { headers: { authorization: `Bearer ${accessToken}`, accept: 'application/json' } },
      );
      if (!res.ok) throw new MeetError('sala del canal no encontrada', res.status, 'not_found');
      const room = (await res.json()) as Room;
      await call<unknown>('POST', `/rooms/${encodeURIComponent(room.id)}/knock`, {});
    },

    /** Llama a la puerta de una sala bloqueada. 202: apuntado, esperando. */
    async knock(id: string, clave?: string): Promise<void> {
      await call<unknown>('POST', `/rooms/${encodeURIComponent(id)}/knock`, {}, clave);
    },

    /** Quién espera en la puerta (anfitrión o admin). */
    async knocks(id: string): Promise<{ identity: string; name?: string }[]> {
      const r = await call<{ knocks?: { identity: string; name?: string }[] }>(
        'GET',
        `/rooms/${encodeURIComponent(id)}/knocks`,
      );
      return r?.knocks ?? [];
    },

    /** Abre la puerta a alguien, o lo retira de la cola. */
    async answerKnock(id: string, identity: string, admitir: boolean): Promise<void> {
      await call<void>(
        admitir ? 'POST' : 'DELETE',
        `/rooms/${encodeURIComponent(id)}/knocks/${encodeURIComponent(identity)}`,
      );
    },

    /** Silencia a todos menos a quien lo pide. Devuelve a cuántos ha callado. */
    async muteAll(id: string): Promise<number> {
      const r = await call<{ muted?: number }>(
        'POST',
        `/rooms/${encodeURIComponent(id)}/participants/mute-all`,
        {},
      );
      return r?.muted ?? 0;
    },

    /** URL firmada (temporal) para ver una grabación terminada. */
    async recordingURL(id: string, recordingID: string): Promise<string> {
      const r = await call<{ url?: string }>(
        'GET',
        `/rooms/${encodeURIComponent(id)}/recordings/${encodeURIComponent(recordingID)}`,
      );
      if (!r?.url) throw new Error('Sytue Meet: grabación sin URL');
      return r.url;
    },

    /** Cierra la reunion para todos (host/admin). LiveKit desconecta a quien quede. */
    async closeMeeting(id: string): Promise<void> {
      await call<void>('DELETE', `/rooms/${encodeURIComponent(id)}`);
    },
  };
}

export type MeetServerClient = ReturnType<typeof meetClient>;
