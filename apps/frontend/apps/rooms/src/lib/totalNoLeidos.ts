import type { Channel, ChatMessage } from './chat.js';

/**
 * El total de no leídos de Rooms: lo que va delante del título de la pestaña —«(3)
 * Sytue Rooms», como Slack, Teams y Gmail— y, en Fyne, sobre el icono de la app.
 *
 * Cuentan los mensajes sin leer de cada canal, salvo los SILENCIADOS (`notifyLevel`
 * 'none'): silenciar es pedir que ese canal no llame la atención, y un número en la
 * pestaña es justo llamarla. Gemelo en Fyne: `TotalNoLeidos` (apps/fyne/apps/rooms/sala.go).
 */
export function totalNoLeidos(channels: readonly Channel[]): number {
  let total = 0;
  for (const c of channels) {
    if (c.muted || c.notifyLevel === 'none') continue;
    total += Math.max(0, c.unread ?? 0);
  }
  return total;
}

/**
 * ¿Un mensaje que llega al canal ABIERTO suma al total? Solo si la pestaña está oculta:
 * entonces nadie lo está leyendo, y quien cambió a Mail tiene que ver «(5)» igual que
 * por un canal de fondo. Cuentan los mensajes de otros, fuera de hilos, y no los de un
 * canal silenciado (misma regla que `totalNoLeidos`), ni si el canal aún no está en la
 * lista (no se sabe si está silenciado). Al volver a la pestaña se borran.
 * Gemelo en Fyne: `SumaNoVisto` (apps/fyne/apps/rooms/sala.go). Diferencia que queda:
 * «oculta» aquí es `visibilityState === 'hidden'`; en Fyne de escritorio, que la ventana
 * pierde el foco (`AlFondo`), porque el driver no dice si sigue a la vista. Con Rooms
 * visible en otro monitor y el foco en otra app, Fyne suma y la web no.
 */
export function sumaNoVisto(
  oculta: boolean,
  msg: Pick<ChatMessage, 'mine' | 'parentId'>,
  canal: Channel | undefined,
): boolean {
  if (!oculta || !canal || msg.mine || msg.parentId) return false;
  return !canal.muted && canal.notifyLevel !== 'none';
}

/** Más de 99 se escribe «99+»: el número exacto no cambia lo que hay que hacer. */
export const TOPE_NO_LEIDOS = 99;

/** El título de la pestaña con el total delante, o el de siempre si no hay nada. */
export function tituloConNoLeidos(base: string, total: number): string {
  if (total <= 0) return base;
  const n = total > TOPE_NO_LEIDOS ? `${TOPE_NO_LEIDOS}+` : String(total);
  return `(${n}) ${base}`;
}
