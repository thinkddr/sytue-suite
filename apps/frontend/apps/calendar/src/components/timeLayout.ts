/**
 * Lógica pura de la rejilla horaria (vistas semana/día): traduce el intervalo
 * [inicio, fin) de un evento a minutos-desde-medianoche acotados al día, y
 * reparte en COLUMNAS los eventos que se solapan (como Google Calendar) para
 * pintarlos lado a lado. Sin DOM ni fechas mágicas: entra Date/minuto, sale
 * geometría. La conversión minuto→píxel vive en el componente (HOUR_PX).
 */

/** Minutos en un día. La rejilla siempre cubre [0, 1440). */
export const DAY_MINUTES = 24 * 60;

/** Paso de imantado del arrastre (crear/mover/redimensionar): 15 min. */
export const SNAP_MIN = 15;

/** Duración mínima que ocupa un bloque en la rejilla (legibilidad y área de clic). */
export const MIN_EVENT_MIN = 20;

/** Duración por defecto de un evento sin DTEND (o con fin <= inicio). */
const DEFAULT_EVENT_MIN = 30;

export interface Interval {
  /** Minutos desde 00:00 del día (0..1440). */
  startMin: number;
  endMin: number;
}

/**
 * Acota el evento [start, end) al día indicado y lo expresa en minutos desde
 * medianoche. Un evento que cruza a/desde días vecinos se recorta a [0, 1440].
 * Devuelve null si no solapa el día. `end=null` (o fin <= inicio) toma una
 * duración por defecto para que el punto sea visible/manipulable.
 */
export function dayMinutes(start: Date, end: Date | null, day: Date): Interval | null {
  const dayStart = new Date(day);
  dayStart.setHours(0, 0, 0, 0);
  const dayStartMs = dayStart.getTime();
  const dayEndMs = dayStartMs + DAY_MINUTES * 60_000;

  const startMs = start.getTime();
  let endMs = end ? end.getTime() : startMs + DEFAULT_EVENT_MIN * 60_000;
  if (endMs <= startMs) endMs = startMs + DEFAULT_EVENT_MIN * 60_000;

  if (startMs >= dayEndMs || endMs <= dayStartMs) return null; // no solapa el día

  const startMin = Math.max(0, Math.round((startMs - dayStartMs) / 60_000));
  const endMin = Math.min(DAY_MINUTES, Math.round((endMs - dayStartMs) / 60_000));
  if (endMin <= startMin) return null;
  return { startMin, endMin };
}

/** Imanta un minuto al paso `step` (por defecto SNAP_MIN), acotado a [0, 1440]. */
export function snapMin(min: number, step: number = SNAP_MIN): number {
  const s = Math.round(min / step) * step;
  return Math.max(0, Math.min(DAY_MINUTES, s));
}

/**
 * Reparte en columnas los intervalos que se solapan en el tiempo. Agrupa los
 * eventos transitivamente solapados en «clusters»; dentro de cada cluster asigna
 * a cada evento la primera columna libre (greedy) y devuelve el nº total de
 * columnas del cluster, con lo que el componente calcula ancho=1/cols y
 * left=col/cols. Los intervalos disjuntos ocupan el ancho completo (cols=1).
 *
 * Preserva las propiedades del objeto de entrada (genérico): el llamador adjunta
 * su evento y lo recupera en la salida. Orden de salida: por inicio (y fin desc).
 *
 * ponytail: O(n²) por día — n = eventos visibles de una jornada, siempre pocos.
 */
export function layoutTimed<T extends Interval>(
  items: readonly T[],
): Array<T & { col: number; cols: number }> {
  const sorted = [...items].sort((a, b) => a.startMin - b.startMin || b.endMin - a.endMin);
  const out: Array<T & { col: number; cols: number }> = [];
  let cluster: Array<T & { col: number; cols: number }> = [];
  let clusterEnd = -Infinity;

  const flush = (): void => {
    const cols = cluster.reduce((m, e) => Math.max(m, e.col + 1), 0);
    for (const e of cluster) e.cols = cols;
    out.push(...cluster);
    cluster = [];
    clusterEnd = -Infinity;
  };

  for (const item of sorted) {
    if (item.startMin >= clusterEnd) flush(); // no solapa el cluster actual: ciérralo
    // Primera columna cuyo último evento ya terminó al empezar éste.
    const colEnds: number[] = [];
    for (const e of cluster) colEnds[e.col] = Math.max(colEnds[e.col] ?? -Infinity, e.endMin);
    let col = 0;
    while (colEnds[col] !== undefined && colEnds[col]! > item.startMin) col++;
    cluster.push({ ...item, col, cols: 0 });
    clusterEnd = Math.max(clusterEnd, item.endMin);
  }
  flush();
  return out;
}
