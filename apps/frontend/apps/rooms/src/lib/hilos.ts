// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// El indicador de hilo del canal y «También enviar al canal» (D-ROOM-05).
//
// Funciones puras que comparten el cliente REST del servidor (lib/chat.ts) y el del
// navegador (client/chatApi.ts): el mapeo del shape del servicio y cómo cambia la raíz
// cuando llega una respuesta. Puras para que el feed, el panel del hilo y los tests
// las usen igual, sin depender de que ChatApp esté montado.
import type { ChatMessage, ThreadParticipant } from './chat.js';

/** Cuántos avatares lleva el indicador; el servicio manda los mismos como mucho. */
export const MAX_PARTICIPANTES = 3;

/** Los campos del indicador tal y como los emite el servicio Go (snake_case). */
export interface RawThreadInfo {
  last_reply_at?: string;
  thread_participants?: { sub: string; name?: string }[];
  also_in_channel?: boolean;
}

/** Mapea los campos del indicador del servicio a los del UI. Lo que no viene, no se
 *  pone: los mensajes sin hilo siguen siendo objetos pequeños. */
export function threadInfo(
  m: RawThreadInfo,
): Pick<ChatMessage, 'lastReplyAt' | 'threadParticipants' | 'alsoInChannel'> {
  return {
    ...(m.last_reply_at ? { lastReplyAt: m.last_reply_at } : {}),
    ...(m.thread_participants?.length
      ? {
          threadParticipants: m.thread_participants
            .slice(0, MAX_PARTICIPANTES)
            .map((p) => ({ id: p.sub, name: p.name || p.sub })),
        }
      : {}),
    ...(m.also_in_channel ? { alsoInChannel: true } : {}),
  };
}

/** La respuesta se ve también en el feed del canal («También enviar al canal»). */
export function esDifundida(m: Pick<ChatMessage, 'parentId' | 'alsoInChannel'>): boolean {
  return Boolean(m.parentId && m.alsoInChannel);
}

/**
 * La raíz tras sumar una respuesta: una más en la cuenta, la última respuesta es esta
 * (si es más reciente que la que había) y su autor pasa al primer avatar sin repetirse.
 * Es lo mismo que agregaría el servicio, así el indicador no espera a recargar.
 */
export function anotarRespuesta(root: ChatMessage, reply: ChatMessage): ChatMessage {
  const yo: ThreadParticipant = { id: reply.author.id, name: reply.author.name };
  const otros = (root.threadParticipants ?? []).filter((p) => p.id !== yo.id);
  const masReciente =
    !root.lastReplyAt || Date.parse(reply.createdAt) >= Date.parse(root.lastReplyAt);
  return {
    ...root,
    replyCount: (root.replyCount ?? 0) + 1,
    ...(masReciente
      ? {
          lastReplyAt: reply.createdAt,
          threadParticipants: [yo, ...otros].slice(0, MAX_PARTICIPANTES),
        }
      : {}),
  };
}

/** Deshace `anotarRespuesta` cuando el envío falla: la cuenta baja y el indicador
 *  vuelve a ser el de `antes`. */
export function deshacerRespuesta(root: ChatMessage, antes: ChatMessage): ChatMessage {
  const out: ChatMessage = { ...root, replyCount: Math.max(0, (root.replyCount ?? 1) - 1) };
  delete out.lastReplyAt;
  delete out.threadParticipants;
  if (antes.lastReplyAt) out.lastReplyAt = antes.lastReplyAt;
  if (antes.threadParticipants) out.threadParticipants = antes.threadParticipants;
  return out;
}
