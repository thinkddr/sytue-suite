/**
 * Cliente de presencia de Sytue Rooms (navegador).
 *
 * La presencia vive en el HUB (heartbeat + consulta agregada), no en el servicio
 * Chat. El navegador habla con el proxy BFF same-origin (/api/presence), que
 * reenvia al Hub con el token de sesion (el access token no toca el navegador).
 *
 * - Heartbeat: PUT /api/presence {state,status} mantiene "en linea" (el Hub lo
 *   caduca por TTL efimero -> offline si no se renueva).
 * - Consulta en BATCH: GET /api/presence?subjects=a,b,c para pintar los puntos de
 *   otros (una sola peticion con todos los subs visibles; escala a cientos de miles).
 *
 * Reexporta el mapeo al PresenceStatus del Avatar de @sytue/ui.
 */
import type { PresenceStatus } from '@sytue/config';
import type { T, MessageKey } from '../i18n/index.js';

/** Estado de presencia del Hub (domain.PresenceState). */
export type PresenceState = 'online' | 'away' | 'busy' | 'offline';

/** Presencia agregada de un usuario tal y como la devuelve el Hub. */
export interface Presence {
  subject: string;
  state: PresenceState;
  /** Estado personalizado (emoji + texto), reutiliza el campo status del heartbeat. */
  status?: string;
  last_seen?: string;
}

/** Estados que el usuario puede FIJAR en el selector (offline no es elegible). */
export const SELECTABLE_STATES: PresenceState[] = ['online', 'away', 'busy'];

/**
 * Clave de catálogo de cada estado (misma nomenclatura que el Avatar de @sytue/ui).
 * El texto vive en `src/i18n`, no aquí: este módulo habla con el Hub, no con el usuario.
 */
export const STATE_KEY: Record<PresenceState, MessageKey> = {
  online: 'presence.online',
  away: 'presence.away',
  busy: 'presence.dnd',
  offline: 'presence.offline',
};

/** Mapea el estado del Hub al PresenceStatus del Avatar (@sytue/ui): busy -> dnd. */
export function toPresenceStatus(state: PresenceState): PresenceStatus {
  return state === 'busy' ? 'dnd' : state;
}

/**
 * Frase «X está escribiendo…» a partir de los nombres activos. Las tres variantes
 * son PLANTILLAS COMPLETAS del catálogo (una, dos, muchas): en varias lenguas el
 * verbo concuerda y el orden cambia, así que no se pegan trozos.
 */
export function typingLabel(t: T, names: string[]): string {
  if (names.length === 0) return '';
  if (names.length === 1) return t('typing.one', { name: names[0] ?? '' });
  if (names.length === 2) return t('typing.two', { first: names[0] ?? '', second: names[1] ?? '' });
  return t('typing.many');
}

/** Tope del Hub por consulta de presencia (`QueryPresence`). */
export const MAX_PRESENCE = 200;

/**
 * Consulta en batch la presencia de varios subjects. Devuelve un mapa sub ->
 * Presence (los subs desconocidos vuelven como offline desde el Hub). Lanza si el
 * proxy falla: el llamante degrada limpio (mantiene lo último conocido).
 */
export async function queryPresence(subjects: string[]): Promise<Record<string, Presence>> {
  const uniq = [...new Set(subjects.filter(Boolean))];
  const out: Record<string, Presence> = {};
  // De MAX_PRESENCE en MAX_PRESENCE: el Hub contesta 400 a partir de 200 sujetos, y los
  // miembros de un canal grande pasan del tope.
  const tandas: string[][] = [];
  for (let i = 0; i < uniq.length; i += MAX_PRESENCE) tandas.push(uniq.slice(i, i + MAX_PRESENCE));
  const respuestas = await Promise.all(
    tandas.map(async (tanda) => {
      const res = await fetch(`/api/presence?subjects=${tanda.map(encodeURIComponent).join(',')}`, {
        credentials: 'include',
        headers: { accept: 'application/json' },
      });
      if (!res.ok) throw new Error(`presence ${res.status}`);
      return (await res.json()) as { presence?: Presence[] };
    }),
  );
  for (const data of respuestas) for (const p of data.presence ?? []) out[p.subject] = p;
  return out;
}

/** Envía un heartbeat con el estado propio; devuelve el intervalo sugerido (~TTL/2). */
export async function sendHeartbeat(
  state: PresenceState,
  status: string,
): Promise<{ heartbeat_seconds?: number }> {
  const res = await fetch('/api/presence', {
    method: 'PUT',
    credentials: 'include',
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    body: JSON.stringify({ state, status }),
  });
  if (!res.ok) throw new Error(`heartbeat ${res.status}`);
  return (await res.json()) as { heartbeat_seconds?: number };
}
