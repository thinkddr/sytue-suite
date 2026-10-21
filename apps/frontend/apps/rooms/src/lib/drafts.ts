/** Composer drafts stay in RAM while Matrix stores the durable encrypted copy in the actor's private room. */
import { putDraft } from '../client/chatApi.js';

const PREFIX = 'sytue-rooms-draft:';
const local = new Map<string, string>();

/** Rebote del empuje al servidor. El composer ya rebota 400 ms su escritura local;
 *  esto agrupa además las ráfagas de tecleo en una sola petición. */
const SYNC_DELAY_MS = 1500;

/** Clave de localStorage del borrador de un ámbito de redacción (canal o hilo). */
export function draftKey(id: string): string {
  return `${PREFIX}${id}`;
}

/** Ámbito de redacción de un borrador: un canal, o un hilo dentro de él. */
export interface DraftScope {
  channelId: string;
  /** Id del mensaje raíz si el borrador es del composer de un hilo. */
  parentId?: string;
}

/** Un borrador local con su ámbito ya resuelto (lo que pinta la vista «Borradores»). */
export interface LocalDraft extends DraftScope {
  body: string;
}

// Los ids de canal y de mensaje son UUID: anclar el parseo en su forma es lo que
// permite que el ownerId (un `sub` de Sytue ID, que puede llevar «:») no rompa el
// troceado por dos puntos.
const UUID = '[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}';
// El cuantificador del ownerId es PEREZOSO: con uno ávido, `usr:<canal>:thread:<raíz>`
// casaría tomando «usr:<canal>:thread» como dueño y la RAÍZ como canal — el borrador
// del hilo se guardaría contra un canal inexistente.
const SCOPE_RE = new RegExp(`^(.+?):(${UUID})(?::thread:(${UUID}))?$`);

/**
 * Trocea un id de ámbito (`<ownerId>:<channelId>[:thread:<rootId>]`) en sus piezas.
 * Devuelve null si no tiene esa forma (nada que sincronizar). Puro y testeable.
 */
export function parseDraftId(id: string): (DraftScope & { ownerId: string }) | null {
  const m = SCOPE_RE.exec(id);
  if (!m) return null;
  return {
    ownerId: m[1]!,
    channelId: m[2]!,
    ...(m[3] ? { parentId: m[3] } : {}),
  };
}

/** Id de ámbito a partir de sus piezas (inversa de parseDraftId). */
export function draftId(ownerId: string, scope: DraftScope): string {
  return scope.parentId
    ? `${ownerId}:${scope.channelId}:thread:${scope.parentId}`
    : `${ownerId}:${scope.channelId}`;
}

/** Restaura el borrador guardado (cadena vacía si no hay o no se puede leer). */
export function loadDraft(id: string): string {
  if (local.has(id)) return local.get(id) ?? '';
  // Migrate the previous unencrypted browser copy once, then remove it.
  try {
    const old = localStorage.getItem(draftKey(id));
    if (old !== null) {
      local.set(id, old);
      localStorage.removeItem(draftKey(id));
      queueSync(id, old);
    }
  } catch {
    /* Storage can be unavailable; the current draft remains in RAM. */
  }
  return local.get(id) ?? '';
}
export function saveDraft(id: string, text: string): void {
  if (text.trim()) local.set(id, text);
  else local.delete(id);
  try {
    localStorage.removeItem(draftKey(id));
  } catch {
    /* legacy migration only */
  }
  queueSync(id, text);
  notify();
}
export function clearDraft(id: string): void {
  local.delete(id);
  try {
    localStorage.removeItem(draftKey(id));
  } catch {
    /* legacy migration only */
  }
  queueSync(id, '');
  notify();
}

/**
 * Borradores locales de una cuenta, de la memoria del cliente. Es lo que
 * cuenta la insignia del sidebar y lo que lista la vista «Borradores»: se lee de
 * local y no del servidor para que el contador reaccione a la misma pulsación que
 * escribe el texto, sin una petición por letra.
 */
export function listLocalDrafts(ownerId: string): LocalDraft[] {
  try {
    const legacy: string[] = [];
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (key?.startsWith(PREFIX) && parseDraftId(key.slice(PREFIX.length))?.ownerId === ownerId)
        legacy.push(key.slice(PREFIX.length));
    }
    for (const id of legacy) loadDraft(id);
  } catch {
    /* RAM remains available when browser storage is disabled. */
  }
  const out: LocalDraft[] = [];
  for (const [id, body] of local) {
    const scope = parseDraftId(id);
    if (scope?.ownerId === ownerId && body.trim())
      out.push({
        channelId: scope.channelId,
        body,
        ...(scope.parentId ? { parentId: scope.parentId } : {}),
      });
  }
  return out;
}

/**
 * Vuelca en local los borradores que vienen del servidor (al arrancar la app), SIN
 * pisar los que ya existen en este dispositivo. Devuelve cuántos escribió, para que
 * la vista sepa si tiene que recomponerse.
 *
 * ponytail: gana el LOCAL cuando hay borrador en ambos lados. Sin marca de tiempo
 * local no se puede saber cuál es más reciente, y quedarse con lo que el usuario
 * acaba de escribir AQUÍ es lo que no pierde trabajo. Si hiciera falta la resolución
 * fina, el camino es guardar el instante junto al texto.
 */
export function hydrateDrafts(ownerId: string, remote: LocalDraft[]): number {
  let written = 0;
  for (const d of remote) {
    if (!d.body.trim()) continue;
    const id = draftId(ownerId, d);
    if (loadDraft(id)) continue;
    local.set(id, d.body);
    synced.set(id, d.body);
    written++;
  }
  if (written > 0) notify();
  return written;
}

// --- Empuje al servidor (rebotado) y aviso a la interfaz -------------------

/** Último cuerpo que YA está en el servidor por ámbito: evita repetir la misma
 *  escritura en cada cambio de canal (el composer guarda al desmontarse). */
const synced = new Map<string, string>();
const timers = new Map<string, ReturnType<typeof setTimeout>>();
const listeners = new Set<() => void>();

function queueSync(id: string, text: string): void {
  const scope = parseDraftId(id);
  if (!scope) return;
  const body = text.trim() ? text : '';
  if (synced.get(id) === body) return;
  const pending = timers.get(id);
  if (pending) clearTimeout(pending);
  timers.set(
    id,
    setTimeout(() => {
      timers.delete(id);
      synced.set(id, body);
      putDraft(scope.channelId, scope.parentId, body).catch(() => {
        // Sin red o sin permiso: la copia local sigue ahí y el próximo guardado
        // reintenta. Se olvida lo «sincronizado» para no dar por bueno lo que falló.
        synced.delete(id);
      });
    }, SYNC_DELAY_MS),
  );
}

/**
 * Se suscribe a los cambios de borradores de esta pestaña (para el contador del
 * sidebar y la vista «Borradores»). Devuelve la baja.
 */
export function subscribeDrafts(fn: () => void): () => void {
  listeners.add(fn);
  return () => {
    listeners.delete(fn);
  };
}

function notify(): void {
  for (const fn of listeners) fn();
}

/** Account changes and logout must discard plaintext and cancel delayed writes. */
export function clearDraftMemory(): void {
  for (const timer of timers.values()) clearTimeout(timer);
  timers.clear();
  synced.clear();
  local.clear();
  notify();
}
