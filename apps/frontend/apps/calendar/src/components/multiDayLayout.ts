/**
 * Lógica PURA del layout de BARRAS multi-día: los eventos de todo el día y los
 * que abarcan varias jornadas se pintan como barras horizontales continuas que
 * cruzan las celdas. Sin DOM ni fechas mágicas de render: entran instantes ya
 * resueltos + un array de días consecutivos, sale geometría (índices, columna,
 * ancho, fila apilada, cortes de continuación y «+N más»). Sirve igual a la vista
 * MES (rejilla 6×7) que a la franja «todo el día» de SEMANA (1×7 o 1×1). La
 * conversión índice→píxel/CSS vive en el componente.
 */

const DAY_MS = 24 * 60 * 60 * 1000;

/** Medianoche local (copia; no muta la entrada). */
function startOfLocalDay(d: Date): Date {
  const x = new Date(d);
  x.setHours(0, 0, 0, 0);
  return x;
}

/** Nº de días naturales (locales) de `a` a `b`; DST-safe por redondeo a día. */
function dayDelta(a: Date, b: Date): number {
  return Math.round((startOfLocalDay(b).getTime() - startOfLocalDay(a).getTime()) / DAY_MS);
}

/** Barra: un evento acotado a índices [start, end] INCLUSIVOS de un array de días. */
export interface Bar<T> {
  item: T;
  start: number;
  end: number;
}

export interface PlacedBar<T> extends Bar<T> {
  /** Fila de apilado (0 = arriba); las barras solapadas nunca comparten fila. */
  row: number;
}

/** Segmento de una barra recortado a UNA semana, ya listo para render. */
export interface Segment<T> extends PlacedBar<T> {
  /** Columna 0..(perWeek-1) donde arranca el segmento dentro de la semana. */
  col: number;
  /** Nº de columnas que ocupa (>=1). */
  span: number;
  /** El evento empezó en una semana anterior (borde izq. recto, sin texto). */
  continuesLeft: boolean;
  /** El evento sigue en una semana posterior (borde der. recto). */
  continuesRight: boolean;
}

/**
 * Acota el intervalo [start, end) de un evento al array `days` (consecutivo,
 * ascendente, medianoche local) y lo expresa como índices INCLUSIVOS [start,end].
 * `end` es EXCLUSIVO (convención iCal: un todo-el-día de 3 días tiene DTEND el 4º);
 * el último día cubierto es el del instante justo anterior a `end`. Sin fin (o fin
 * <= inicio) cubre solo el día de su inicio. Devuelve null si no solapa `days`.
 */
export function barSpan(
  days: readonly Date[],
  start: Date,
  end: Date | null,
): { start: number; end: number } | null {
  if (days.length === 0) return null;
  const first = days[0]!;
  const lastIdx = days.length - 1;
  const startMs = start.getTime();
  const endMs = end ? end.getTime() : startMs;
  // Último día cubierto: end exclusivo ⇒ el instante 1 ms antes de end.
  const lastCovered = endMs > startMs ? new Date(endMs - 1) : start;
  let s = dayDelta(first, start);
  let e = dayDelta(first, lastCovered);
  if (e < 0 || s > lastIdx) return null; // fuera del rango visible
  if (s < 0) s = 0; // empezó antes de la rejilla: se recorta al borde
  if (e > lastIdx) e = lastIdx; // termina después: se recorta al borde
  if (e < s) return null;
  return { start: s, end: e };
}

/**
 * Asigna a cada barra una FILA de apilado (0 arriba) por «first-fit» voraz sobre
 * los intervalos [start,end] inclusivos: las barras que se solapan (comparten
 * algún día) nunca caen en la misma fila; las disjuntas reutilizan filas. Orden
 * ESTABLE y determinista: por inicio asc, luego por longitud desc (las barras
 * largas arriba, como Google Calendar) y desempate por el orden de entrada, de
 * modo que el mismo conjunto produce SIEMPRE el mismo apilado (memoizable).
 *
 * ponytail: O(n·R) tras el sort O(n log n); R = nº de filas, siempre pocas.
 */
export function assignRows<T>(bars: readonly Bar<T>[]): PlacedBar<T>[] {
  const order = bars.map((bar, i) => ({ bar, i }));
  order.sort(
    (a, b) =>
      a.bar.start - b.bar.start || b.bar.end - b.bar.start - (a.bar.end - a.bar.start) || a.i - b.i,
  );
  const rowEnds: number[] = []; // rowEnds[r] = último índice ocupado en la fila r
  const out: PlacedBar<T>[] = [];
  for (const { bar } of order) {
    let row = 0;
    // Primera fila cuyo último ocupante ya terminó antes de empezar esta barra.
    while (row < rowEnds.length && rowEnds[row]! >= bar.start) row++;
    rowEnds[row] = bar.end;
    out.push({ ...bar, row });
  }
  return out;
}

/**
 * Recorta cada barra colocada a las semanas de la rejilla (mes) o a la única
 * semana (franja de semana). Devuelve, POR semana, los segmentos ya en
 * coordenadas de columna (0..perWeek-1) con su ancho y los cortes de continuación
 * en los saltos de semana. Conserva la fila para el apilado vertical.
 */
export function sliceWeeks<T>(
  placed: readonly PlacedBar<T>[],
  weekCount: number,
  perWeek = 7,
): Segment<T>[][] {
  const weeks: Segment<T>[][] = Array.from({ length: weekCount }, () => []);
  for (const bar of placed) {
    const firstWeek = Math.floor(bar.start / perWeek);
    const lastWeek = Math.floor(bar.end / perWeek);
    for (let w = Math.max(0, firstWeek); w <= lastWeek && w < weekCount; w++) {
      const weekStart = w * perWeek;
      const weekEnd = weekStart + perWeek - 1;
      const segStart = Math.max(bar.start, weekStart);
      const segEnd = Math.min(bar.end, weekEnd);
      weeks[w]!.push({
        ...bar,
        col: segStart - weekStart,
        span: segEnd - segStart + 1,
        continuesLeft: bar.start < weekStart,
        continuesRight: bar.end > weekEnd,
      });
    }
  }
  return weeks;
}

/**
 * Limita las barras visibles de UNA semana a `maxRows` filas y cuenta, por
 * columna (día), cuántas quedan OCULTAS bajo ese tope (para el «+N más»). Las
 * barras de fila >= maxRows no se pintan; cada día suma las que lo cubren.
 */
export function limitWeekRows<T>(
  segments: readonly Segment<T>[],
  maxRows: number,
  perWeek = 7,
): { visible: Segment<T>[]; hiddenByCol: number[] } {
  const visible: Segment<T>[] = [];
  const hiddenByCol = new Array<number>(perWeek).fill(0);
  for (const seg of segments) {
    if (seg.row < maxRows) {
      visible.push(seg);
      continue;
    }
    for (let c = seg.col; c < seg.col + seg.span && c < perWeek; c++) hiddenByCol[c]! += 1;
  }
  return { visible, hiddenByCol };
}

/** Nº de filas de barras ocupadas en una semana (para reservar altura). */
export function rowCount<T>(segments: readonly Segment<T>[]): number {
  let max = -1;
  for (const s of segments) if (s.row > max) max = s.row;
  return max + 1;
}

// --- Color de texto AA sobre el relleno de la barra --------------------------

/** Canal sRGB→lineal para la luminancia relativa WCAG. */
function channel(c: number): number {
  const s = c / 255;
  return s <= 0.03928 ? s / 12.92 : ((s + 0.055) / 1.055) ** 2.4;
}

/** Luminancia relativa WCAG de un hex #rgb/#rrggbb (0 = negro, 1 = blanco). */
export function luminance(hex: string): number {
  const v = hex.replace('#', '').trim();
  const n =
    v.length === 3
      ? v
          .split('')
          .map((c) => c + c)
          .join('')
      : v;
  const r = parseInt(n.slice(0, 2), 16);
  const g = parseInt(n.slice(2, 4), 16);
  const b = parseInt(n.slice(4, 6), 16);
  if (Number.isNaN(r) || Number.isNaN(g) || Number.isNaN(b)) return 0;
  return 0.2126 * channel(r) + 0.7152 * channel(g) + 0.0722 * channel(b);
}

/**
 * Color de texto legible (contraste AA) sobre un relleno: blanco o tinta oscura,
 * el de MAYOR contraste WCAG. La paleta de calendarios es toda oscura ⇒ blanco;
 * si alguien elige un color claro, cae a tinta oscura. Así el texto de la barra
 * nunca depende solo del color y siempre supera 4.5:1 con una de las dos tintas.
 */
export function barTextColor(fill: string): string {
  const L = luminance(fill);
  const withWhite = 1.05 / (L + 0.05); // ratio vs blanco (L=1)
  const withBlack = (L + 0.05) / 0.05; // ratio vs negro (L=0)
  return withWhite >= withBlack ? '#ffffff' : '#111827';
}
