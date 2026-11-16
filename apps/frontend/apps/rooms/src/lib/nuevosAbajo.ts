import type { ChatMessage } from './chat.js';

/**
 * «↓ N mensajes nuevos» (Slack, Teams, Element): quien se ha subido a leer el historial
 * no se entera de lo que llega abajo, porque la lista solo sigue al fondo si ya estaba
 * en él. Esto dice QUÉ ha llegado al final desde la última vez que se miró.
 *
 * `anteriores` son los ids de la lista anterior. El ancla es el último mensaje de la
 * lista nueva que ya estaba en ella; lo que va detrás es lo llegado, salvo lo propio (lo
 * que una escribe ya lo ha visto) y lo que aún no existe en el servidor (el optimista en
 * vuelo cambiará de id al confirmarse). Anclar en el último que SIGUE y no en el último
 * que había importa: si se borra el del final, o el optimista propio del final se
 * confirma con otro id, el último anterior desaparece y la cuenta no debe perderse.
 * Devuelve null si no queda ninguno de los anteriores: la lista es OTRA (otro canal, la
 * ventana centrada de un permalink) y la cuenta se reinicia, en vez de sumar como nuevo
 * todo un canal.
 *
 * Un prepend de historial no mueve el ancla y no da nada; la lista capada (pierde por
 * arriba al crecer por abajo) conserva el ancla y cuenta bien.
 *
 * Gemelo en Fyne: `LlegadosAbajo` (apps/fyne/apps/rooms/historial.go).
 */
export function llegadosAbajo(
  messages: readonly ChatMessage[],
  anteriores: readonly string[],
): string[] | null {
  if (anteriores.length === 0) return [];
  const previos = new Set(anteriores);
  let i = messages.length - 1;
  while (i >= 0 && !previos.has(messages[i]!.id)) i--;
  if (i < 0) return null;
  const out: string[] = [];
  for (let j = i + 1; j < messages.length; j++) {
    const m = messages[j]!;
    if (!m.mine && !m.pending) out.push(m.id);
  }
  return out;
}
