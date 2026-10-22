/**
 * Registro (a nivel de módulo) de los emoji personalizados del tenant (Ola 14).
 *
 * Se rellena UNA vez al cargar Rooms (ChatApp) y lo consultan, SIN enhebrar props,
 * MessageBody (render de `:nombre:` en el cuerpo), las reacciones y el EmojiPicker
 * —igual que el mapa EMOJI unicode de la Ola 1 es de módulo—. El catálogo es
 * acotado por tenant (cientos como mucho), así que un mapa en memoria basta y evita
 * pasar la tabla por 6 componentes de mensaje.
 *
 * `name` → fileId de Drive; la imagen se sirve por el proxy de adjuntos same-origin
 * (/api/attachment), que responde con X-Content-Type-Options: nosniff + CSP sandbox.
 */
import { withBase } from '@sytue/ui';
import type { CustomEmoji } from './chat.js';

export type { CustomEmoji };

let list: CustomEmoji[] = [];
let map: Record<string, string> = {};
const subscribers = new Set<() => void>();

/** Reemplaza el catálogo del tenant y avisa a los suscriptores (p. ej. el picker
 *  abierto, que se re-renderiza al terminar la carga o tras subir un emoji). */
export function setCustomEmoji(next: CustomEmoji[]): void {
  list = next;
  map = Object.fromEntries(next.map((e) => [e.name, e.fileId]));
  for (const fn of subscribers) fn();
}

/** Catálogo actual (orden del backend, por nombre). */
export function customEmojiList(): CustomEmoji[] {
  return list;
}

/** Mapa nombre → fileId, para resolver un `:nombre:` en O(1). */
export function customEmojiMap(): Record<string, string> {
  return map;
}

/** Suscribe un callback a los cambios del catálogo; devuelve la función de baja. */
export function subscribeCustomEmoji(fn: () => void): () => void {
  subscribers.add(fn);
  return () => {
    subscribers.delete(fn);
  };
}

/** URL same-origin (proxy de adjuntos, nosniff) de la imagen de un emoji custom. */
export function customEmojiSrc(fileId: string): string {
  // CON withBase: esto acaba en un `src`, no en un `fetch`, así que el shim de
  // `basePathScript` —que prefija las rutas `/api/` de fetch— no lo alcanza. Sin
  // el prefijo, en `<organizacion>.sytue.eu` la petición sale a la RAÍZ del host,
  // donde el ingress del tenant no enruta `/api`: 404 de Traefik. Medido.
  return withBase(import.meta.env.BASE_URL, `/api/attachment?fileId=${encodeURIComponent(fileId)}`);
}
