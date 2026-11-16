/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La rejilla de Calcus: hasta dónde llega una hoja, cuánto de ella se pinta y qué
 * trozo cabe en la ventana. Puro, sin DOM: lo usan SheetCanvas, la tabla dinámica y
 * las segmentaciones, y lo prueban los tests.
 *
 * Tres tamaños distintos, y confundirlos es el error que había:
 * - El LÍMITE lógico (MAX_*) es el de Excel. Una celda importada en la fila 5.000 o
 *   en la columna AB existe, se puede navegar hasta ella y ninguna operación la tira
 *   por «salirse de la rejilla».
 * - La EXTENSIÓN es lo que se puede recorrer con la barra de desplazamiento: lo usado,
 *   lo que pide la selección y lo que se ve, siempre con una rejilla mínima de holgura
 *   por delante. Crece al desplazarse, como en cualquier hoja de cálculo.
 * - La VENTANA es lo que se pinta de verdad: las filas y columnas visibles más un
 *   margen. Una hoja de 100.000 filas pinta las mismas celdas que una de 100.
 */

/** Columnas de una hoja de Excel (A..XFD). */
export const MAX_COLS = 16_384;
/** Filas de una hoja de Excel. */
export const MAX_ROWS = 1_048_576;

/** Rejilla mínima (A..Z × 1..100): lo que se ve en una hoja vacía y la holgura que
 *  queda siempre a la derecha y debajo de lo usado. */
export const MIN_COLS = 26;
export const MIN_ROWS = 100;

/** Lo que se adelanta la extensión respecto a lo que se ve, en filas y columnas. Tiene
 *  que ser MAYOR que el paso con el que se cuantiza el desplazamiento (VISTA_PASO_*):
 *  si no, al llegar al borde no quedaría por dónde seguir bajando. */
const ADELANTO_FILAS = 24;
const ADELANTO_COLS = 4;

/** El desplazamiento se cuantiza a bloques de este tamaño (en filas y columnas) para no
 *  repintar el lienzo entero en cada píxel de rueda: solo al cruzar de bloque. */
export const VISTA_PASO_FILAS = 8;
export const VISTA_PASO_COLS = 2;
/** Margen de filas y columnas que se pintan de más a cada lado de la ventana. */
const MARGEN_FILAS = 6;
const MARGEN_COLS = 2;

/** Lo que se ve del contenedor de desplazamiento, en px de maqueta (ya sin zoom). */
export interface Vista {
  top: number;
  left: number;
  w: number;
  h: number;
}

/**
 * Columnas y filas que se pueden recorrer. `ultCol`/`ultFila` son la última columna y
 * fila usadas (-1 si la hoja está vacía); `pideCols`/`pideFilas`, cuántas hacen falta
 * para que quepa la selección; `vista`, lo que se está viendo, con el tamaño de celda.
 */
export function extension(
  ultCol: number,
  ultFila: number,
  pideCols: number,
  pideFilas: number,
  vista: Vista,
  cellW: number,
  cellH: number,
): { cols: number; rows: number } {
  const vistaCols = Math.ceil((vista.left + vista.w) / cellW) + ADELANTO_COLS;
  const vistaFilas = Math.ceil((vista.top + vista.h) / cellH) + ADELANTO_FILAS;
  return {
    cols: Math.min(MAX_COLS, Math.max(MIN_COLS, ultCol + 1 + MIN_COLS, pideCols, vistaCols)),
    rows: Math.min(MAX_ROWS, Math.max(MIN_ROWS, ultFila + 1 + MIN_ROWS, pideFilas, vistaFilas)),
  };
}

/** Redondea hacia abajo a un múltiplo de `paso` (px). */
export function cuantiza(px: number, paso: number): number {
  return Math.max(0, Math.floor(px / paso) * paso);
}

/**
 * Tramo [desde, hasta) de índices VISIBLES que pinta la ventana en un eje. `inicio` y
 * `largo` son lo que se ve, en px de maqueta; `origen`, dónde empieza el índice 0 (el
 * ancho de las cabeceras); `paso`, lo que mide cada unidad. Nunca baja de `fijas` (las
 * inmovilizadas se pintan siempre aparte) ni pasa de `total`.
 */
export function tramo(
  inicio: number,
  largo: number,
  origen: number,
  paso: number,
  fijas: number,
  total: number,
  eje: 'filas' | 'cols',
): { desde: number; hasta: number } {
  const margen = eje === 'filas' ? MARGEN_FILAS : MARGEN_COLS;
  // El inicio llega cuantizado hacia abajo: se alarga un bloque para cubrir lo que falte.
  const bloque = (eje === 'filas' ? VISTA_PASO_FILAS : VISTA_PASO_COLS) * paso;
  const a = Math.floor((inicio - origen) / paso) - margen;
  const b = Math.ceil((inicio + largo + bloque - origen) / paso) + margen;
  const clamp = (n: number): number => Math.min(total, Math.max(fijas, n));
  return { desde: clamp(a), hasta: clamp(b) };
}

/** Primera posición de `vis` (ordenado ascendente) con valor ≥ `x`; `vis.length` si
 *  no hay ninguna. Búsqueda binaria. */
export function primeroDesde(vis: readonly number[], x: number): number {
  let lo = 0;
  let hi = vis.length;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if (vis[mid]! < x) lo = mid + 1;
    else hi = mid;
  }
  return lo;
}

/** Índice visible de `x` en `vis` (ordenado ascendente), o -1 si no está: oculta por el
 *  filtro o el esquema, o fuera de la extensión. `indexOf` sobre las filas de una tabla
 *  de 50.000 eran 2.500 millones de comparaciones por render. */
export function indiceVisible(vis: readonly number[], x: number): number {
  const i = primeroDesde(vis, x);
  return vis[i] === x ? i : -1;
}

/**
 * Destino de Ctrl+flecha en un eje, con el criterio de Excel. `llenas` son las
 * posiciones con datos de esa fila o columna, ordenadas; `max`, el límite del eje.
 * - Si la celda y la siguiente tienen datos, al final del bloque.
 * - Si no, a la próxima celda con datos en esa dirección.
 * - Y si no queda ninguna, al borde de la hoja.
 */
export function saltoADatos(
  llenas: readonly number[],
  desde: number,
  paso: 1 | -1,
  max: number,
): number {
  const siguiente = desde + paso;
  if (siguiente < 0 || siguiente >= max) return desde;
  const k = indiceVisible(llenas, desde);
  if (k >= 0 && llenas[k + paso] === siguiente) {
    let i = k + paso;
    while (llenas[i + paso] === llenas[i]! + paso) i += paso;
    return llenas[i]!;
  }
  const destino =
    paso === 1 ? llenas[primeroDesde(llenas, desde + 1)] : llenas[primeroDesde(llenas, desde) - 1];
  return destino ?? (paso === 1 ? max - 1 : 0);
}

/* ---------- Medidas propias de filas y columnas (anchos y altos de un .xlsx o un .ods) ----------
 * `SheetData.colWidths` y `rowHeights` guardan en px de maqueta solo lo que se aparta de la
 * medida de serie. La geometría deja de ser «índice × paso» y pasa a ser una lista de
 * posiciones acumuladas, que se busca por bisección: es lo único que cambia respecto a la
 * rejilla de paso fijo, y sin medidas propias nada de esto se ejecuta. */

/** Límites de una medida propia (px de maqueta): lo que se ve y lo que cabe en Excel. */
export const MEDIDA_MIN = 4;
export const ANCHO_MAX = 1790;
export const ALTO_MAX = 545;

/** Medida de la unidad `i` (columna o fila 0-based): la propia si es válida, y si no, la de serie. */
export function medidaDe(
  medidas: Readonly<Record<string, number>> | undefined,
  i: number,
  serie: number,
  tope: number = ANCHO_MAX,
): number {
  const m = medidas?.[String(i)];
  if (typeof m !== 'number' || !Number.isFinite(m) || m < MEDIDA_MIN) return serie;
  return Math.min(m, tope);
}

/**
 * Posiciones acumuladas de las unidades visibles: `pos[k]` es el borde inicial de la k-ésima
 * (el origen más lo que miden las anteriores) y `pos[n]`, el final de la última.
 */
export function acumulado(
  origen: number,
  visibles: readonly number[],
  medida: (i: number) => number,
): number[] {
  const pos = new Array<number>(visibles.length + 1);
  let x = origen;
  for (let k = 0; k < visibles.length; k++) {
    pos[k] = x;
    x += medida(visibles[k]!);
  }
  pos[visibles.length] = x;
  return pos;
}

/** Índice visible (0..n-1) de la unidad que cubre la coordenada `x`; se acota a los extremos. */
export function indiceEn(pos: readonly number[], x: number): number {
  const n = pos.length - 1;
  if (n <= 0) return 0;
  let lo = 0;
  let hi = n - 1;
  while (lo < hi) {
    const mid = (lo + hi + 1) >> 1;
    if (pos[mid]! <= x) lo = mid;
    else hi = mid - 1;
  }
  return lo;
}

/* ---------- Redimensionar a mano: arrastrar el borde, doble clic o valor exacto ---------- */

/**
 * Lo que pide el comando `colwidth`/`rowheight`: un número de px de maqueta, `'auto'`
 * (autoajuste) o nada válido (null). Se acepta «120», «120px» o «120,5»: es lo que la
 * gente teclea en el cuadro de «Ancho de columna…».
 */
export function leerMedida(valor: string | undefined): number | 'auto' | null {
  const v = (valor ?? '').trim().toLowerCase();
  if (v === 'auto') return 'auto';
  const m = /^(\d+(?:[.,]\d+)?)\s*(?:px)?$/.exec(v);
  if (!m) return null;
  const n = Number(m[1]!.replace(',', '.'));
  return Number.isFinite(n) ? n : null;
}

/**
 * Mapa de medidas con `indices` puestos a `px` (null = volver a la de serie). La medida se
 * redondea y se acota a MEDIDA_MIN..tope; la que coincide con la de serie BORRA su clave,
 * para que el modelo siga guardando solo lo que se aparta (y la coedición, que fusiona por
 * clave, no arrastre ruido). Devuelve undefined si no queda ninguna.
 */
export function conMedidas(
  medidas: Readonly<Record<string, number>> | undefined,
  indices: readonly number[],
  px: number | null,
  serie: number,
  tope: number = ANCHO_MAX,
): Record<string, number> | undefined {
  const out: Record<string, number> = { ...medidas };
  const m = px === null ? serie : Math.min(tope, Math.max(MEDIDA_MIN, Math.round(px)));
  for (const i of indices) {
    if (m === serie) delete out[String(i)];
    else out[String(i)] = m;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

/** Aire a cada lado del texto en una celda (el `px-1` de la rejilla más el borde). */
export const AIRE_AJUSTE = 10;

/**
 * Ancho de autoajuste de una columna a partir de lo que miden sus textos (px, ya medidos
 * con `measureText`): el más ancho más el aire, acotado. Una columna vacía vuelve a la de
 * serie, como el doble clic de Excel sobre una columna sin datos.
 */
export function anchoAjustado(anchos: readonly number[], serie: number): number {
  let max = 0;
  for (const a of anchos) if (Number.isFinite(a) && a > max) max = a;
  if (max <= 0) return serie;
  return Math.min(ANCHO_MAX, Math.max(MEDIDA_MIN, Math.ceil(max + AIRE_AJUSTE)));
}

/**
 * Alto de autoajuste de una fila a partir de los tamaños de letra (pt) de sus celdas: la
 * línea de la letra mayor (×4/3 a px, interlineado 1,3) más el aire; nunca por debajo de la
 * de serie. Sin tamaños propios vuelve a la de serie (no se mide el texto ajustado en
 * varias líneas: la rejilla no lo pinta así todavía).
 */
export function altoAjustado(tamanosPt: readonly number[], serie: number): number {
  let max = 0;
  for (const t of tamanosPt) if (Number.isFinite(t) && t > max) max = t;
  if (max <= 0) return serie;
  return Math.min(ALTO_MAX, Math.max(serie, Math.ceil(((max * 4) / 3) * 1.3 + 6)));
}

/**
 * `tramo` para posiciones acumuladas (medidas propias): [desde, hasta) de índices visibles
 * que cubre la ventana [inicio, inicio + largo), con el mismo margen de unidades y
 * respetando las inmovilizadas y el total.
 */
export function tramoPorPosiciones(
  pos: readonly number[],
  inicio: number,
  largo: number,
  fijas: number,
  total: number,
  eje: 'filas' | 'cols',
): { desde: number; hasta: number } {
  const margen = eje === 'filas' ? MARGEN_FILAS : MARGEN_COLS;
  // El inicio llega cuantizado hacia abajo: se alarga un bloque (de la unidad de serie) para
  // cubrir lo que falte, como `tramo`.
  const bloque =
    (eje === 'filas' ? VISTA_PASO_FILAS : VISTA_PASO_COLS) * (eje === 'filas' ? 24 : 96);
  const a = indiceEn(pos, inicio) - margen;
  const b = indiceEn(pos, inicio + largo + bloque) + 1 + margen;
  const clamp = (n: number): number => Math.min(total, Math.max(fijas, n));
  return { desde: clamp(a), hasta: clamp(b) };
}
