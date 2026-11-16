/**
 * Alt+↑/↓ (canal anterior/siguiente) y Alt+Mayús+↑/↓ (el anterior/siguiente con algo
 * pendiente), los atajos de Slack, Element y Teams. Lógica pura: el componente solo
 * escucha la tecla y abre lo que devuelve. Gemelo en Fyne: `OrdenVisible` y
 * `CanalVecino` (apps/fyne/apps/rooms/lateral.go).
 */
import { enCampoDeTexto, haySuperposicionAbierta } from '@sytue/ui';
import { pendientesDe, type SidebarGroup } from './categories.js';
import type { Channel } from './chat.js';

/**
 * Los canales en el orden en que se ven en el carril: sección a sección (Favoritos
 * primero, luego Canales, las categorías propias y los directos, que es como las
 * devuelve `groupChannels`) y sin los de las secciones plegadas, que no se ven. La
 * flecha recorre lo que hay delante, no una lista que nadie ve.
 */
export function ordenVisible(
  groups: readonly SidebarGroup[],
  plegada: (g: SidebarGroup) => boolean,
): Channel[] {
  return groups.flatMap((g) => (plegada(g) ? [] : g.channels));
}

/**
 * El canal al que lleva la flecha desde el abierto (`paso` 1 baja, -1 sube), o null si
 * no hay adónde ir.
 *
 * - A secas no da la vuelta: al final del carril la flecha se queda, como en Slack.
 * - Con `soloPendientes` salta al siguiente con algo pendiente (la regla del badge de
 *   la sección: los silenciados no cuentan) y SÍ da la vuelta: lo que se busca es «lo
 *   siguiente sin leer», esté donde esté. El abierto nunca cuenta: se está leyendo.
 * - Si el abierto no está a la vista (en una sección plegada, o ninguno abierto), se
 *   empieza por el primero al bajar y por el último al subir.
 */
export function canalVecino(
  orden: readonly Channel[],
  activoId: string | null,
  paso: 1 | -1,
  soloPendientes = false,
): Channel | null {
  const n = orden.length;
  if (n === 0) return null;
  const i = activoId === null ? -1 : orden.findIndex((c) => c.id === activoId);
  if (!soloPendientes) {
    if (i < 0) return paso === 1 ? orden[0]! : orden[n - 1]!;
    return orden[i + paso] ?? null;
  }
  // Desde el abierto (o desde fuera del carril si no está a la vista) y dando la
  // vuelta una sola vez.
  const inicio = i < 0 ? (paso === 1 ? -1 : n) : i;
  for (let k = 1; k <= n; k++) {
    const c = orden[(((inicio + paso * k) % n) + n) % n]!;
    if (c.id !== activoId && pendientesDe(c) > 0) return c;
  }
  return null;
}

/** Lo que de un `keydown` mira `pasoDeNavegacion`. */
export interface TeclaNavegar {
  key: string;
  altKey: boolean;
  shiftKey: boolean;
  ctrlKey: boolean;
  metaKey: boolean;
  target: EventTarget | null;
}

/**
 * ¿Esta tecla mueve de canal? Devuelve el paso y si es el salto a pendientes, o null.
 *
 * En un campo con texto no: en macOS ⌥+flecha mueve el cursor por párrafos y con
 * Mayús selecciona, y quitárselo a quien escribe es peor que no tener el atajo. Con la
 * caja vacía sí, que es donde está el foco casi siempre. Con un diálogo o un menú
 * delante, el teclado es suyo.
 */
export function pasoDeNavegacion(
  e: TeclaNavegar,
): { paso: 1 | -1; soloPendientes: boolean } | null {
  if (!e.altKey || e.ctrlKey || e.metaKey) return null;
  if (e.key !== 'ArrowUp' && e.key !== 'ArrowDown') return null;
  if (enCampoDeTexto(e.target) && textoDe(e.target) !== '') return null;
  if (haySuperposicionAbierta()) return null;
  return { paso: e.key === 'ArrowDown' ? 1 : -1, soloPendientes: e.shiftKey };
}

/**
 * El texto de un campo o de un `contenteditable`: un espacio ya es texto. Un `<select>`
 * nunca cuenta como vacío: Alt+↓ es la tecla que despliega sus opciones en Windows y
 * Linux, y su valor puede ser '' con una opción «ninguna» elegida.
 */
function textoDe(destino: EventTarget | null): string {
  const el = destino as HTMLElement;
  if (el.isContentEditable) return el.textContent ?? '';
  if (/^SELECT$/i.test(el.tagName ?? '')) return 'select';
  return (el as HTMLInputElement).value ?? '';
}
