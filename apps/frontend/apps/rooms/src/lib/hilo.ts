/**
 * Mensajes vivos del canal y del hilo abierto: cómo se aplica a un mensaje una
 * reacción (propia, optimista; o ajena, por WebSocket) o una edición, y a qué vista
 * le toca cada trama. Las respuestas de un hilo NO están en el histórico del canal
 * (viven solo en el panel de hilo), así que una trama sobre una respuesta tiene que
 * llegar al panel aunque el canal activo sea otro (el hilo se abre también desde la
 * vista global «Hilos»).
 */
import type { ChatMessage } from './chat.js';

/** Añade/quita un sub de la lista de reactores de un chip, SOLO si el chip ya la
 *  trae: si el servicio no envió `subs`, no se fabrica una lista parcial (el chip
 *  cae a la etiqueta de solo conteo en vez de mentir sobre quién reaccionó). */
function withSub(r: { subs?: string[] }, sub: string, added: boolean): { subs?: string[] } {
  if (!r.subs) return {};
  return { subs: added ? [...r.subs, sub] : r.subs.filter((s) => s !== sub) };
}

/** Alterna la reaccion del usuario de la sesion sobre un mensaje (optimista). */
export function toggleMyReaction(m: ChatMessage, emoji: string, mySub: string): ChatMessage {
  const reactions = [...(m.reactions ?? [])];
  const idx = reactions.findIndex((r) => r.emoji === emoji);
  if (idx === -1) {
    reactions.push({ emoji, count: 1, mine: true, subs: [mySub] });
  } else {
    const r = reactions[idx]!;
    if (r.mine) {
      const next = { ...r, count: r.count - 1, mine: false, ...withSub(r, mySub, false) };
      if (next.count <= 0) reactions.splice(idx, 1);
      else reactions[idx] = next;
    } else {
      reactions[idx] = { ...r, count: r.count + 1, mine: true, ...withSub(r, mySub, true) };
    }
  }
  return { ...m, reactions };
}

/** Aplica el delta de una reaccion AJENA (added/removed); no toca `mine`. */
export function applyReactionDelta(
  m: ChatMessage,
  emoji: string,
  added: boolean,
  subject: string,
): ChatMessage {
  const reactions = [...(m.reactions ?? [])];
  const idx = reactions.findIndex((r) => r.emoji === emoji);
  if (added) {
    if (idx === -1) reactions.push({ emoji, count: 1, mine: false, subs: [subject] });
    else {
      const r = reactions[idx]!;
      reactions[idx] = { ...r, count: r.count + 1, ...withSub(r, subject, true) };
    }
  } else if (idx !== -1) {
    const r = reactions[idx]!;
    const next = { ...r, count: r.count - 1, ...withSub(r, subject, false) };
    if (next.count <= 0) reactions.splice(idx, 1);
    else reactions[idx] = next;
  }
  return { ...m, reactions };
}

/** Aplica una edicion a un mensaje: solo cuerpo + marca editedAt (conserva reacciones,
 *  replyCount, etc., que el frame/respuesta de edicion no incluye). */
export function mergeEdit(m: ChatMessage, edited: ChatMessage): ChatMessage {
  return { ...m, body: edited.body, ...(edited.editedAt ? { editedAt: edited.editedAt } : {}) };
}

/** A qué vistas abiertas le toca una reacción AJENA que llega por WebSocket. La
 *  propia ya se pintó optimista al hacer clic: aplicarla otra vez la contaría doble. */
export function destinosDeReaccion(
  ev: { channelId: string; subject: string },
  ctx: { yo: string; canalActivo: string | null; canalDelHilo: string | null },
): { canal: boolean; hilo: boolean } {
  if (ev.subject === ctx.yo) return { canal: false, hilo: false };
  return {
    canal: ev.channelId === ctx.canalActivo,
    hilo: ev.channelId === ctx.canalDelHilo,
  };
}
