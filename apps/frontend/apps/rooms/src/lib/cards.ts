/**
 * Estado de los objetos ricos (encuesta / tarea) en el cliente: el cálculo optimista
 * de un voto y la aplicación del recuento que llega del servidor.
 *
 * Vive aparte de los componentes porque es la única lógica de la función que puede
 * salir mal en silencio —contar de más al votar dos veces, o dejar dos opciones
 * marcadas en una encuesta de respuesta única— y así se puede probar sin montar el
 * chat entero (ver cards.test.ts).
 */
import type { CardVote, ChatMessage, MessageCard } from '../client/chatApi.js';

/**
 * Recuento resultante de que YO marque (add) o desmarque un valor de la tarjeta.
 * Espeja lo que hace el servidor: en una encuesta de respuesta ÚNICA, marcar una
 * opción retira mi voto de las demás (no acumula). Idempotente: marcar lo ya marcado
 * no suma, desmarcar lo no marcado no resta.
 */
export function toggleMyCardVote(
  card: MessageCard,
  value: string,
  add: boolean,
  mySub: string,
): CardVote[] {
  const single = card.kind === 'poll' && !card.multi;
  const votes: CardVote[] = (card.votes ?? []).map((v) => ({
    ...v,
    ...(v.subs ? { subs: [...v.subs] } : {}),
  }));

  const withoutMe = (v: CardVote): CardVote => ({
    value: v.value,
    count: Math.max(0, v.count - 1),
    mine: false,
    ...(v.subs ? { subs: v.subs.filter((s) => s !== mySub) } : {}),
  });

  let out: CardVote[] = votes.map((v) => {
    // Al marcar en respuesta única, mis votos a OTRAS opciones se retiran.
    if (v.mine && (v.value === value ? !add : add && single)) return withoutMe(v);
    return v;
  });

  if (add) {
    const existing = out.find((v) => v.value === value);
    if (existing && !existing.mine) {
      existing.count += 1;
      existing.mine = true;
      if (existing.subs) existing.subs = [...existing.subs, mySub];
    } else if (!existing) {
      out.push({ value, count: 1, mine: true, subs: [mySub] });
    }
  }

  // Un valor sin votos deja de existir (así el 0 % no ocupa sitio en la tarjeta).
  out = out.filter((v) => v.count > 0);
  return out.sort((a, b) => (a.value < b.value ? -1 : a.value > b.value ? 1 : 0));
}

/** Devuelve el mensaje con el recuento de su tarjeta sustituido (no-op si no es él
 *  o si no lleva tarjeta). El recuento del servidor SUSTITUYE, nunca se fusiona:
 *  llega completo y aplicarlo así es idempotente. */
export function applyCardVotes(m: ChatMessage, messageId: string, votes: CardVote[]): ChatMessage {
  if (m.id !== messageId || !m.card) return m;
  return { ...m, card: { ...m.card, votes } };
}
