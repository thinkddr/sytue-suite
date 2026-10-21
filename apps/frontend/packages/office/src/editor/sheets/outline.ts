/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Helpers PUROS del ESQUEMA (agrupar/outline) y los SUBTOTALES de la hoja. Sin
 * DOM ni estado de UI: geometría de grupos (qué filas/columnas se ocultan por
 * nivel/colapso, nivel máximo, mostrar hasta un nivel, agrupar/desagrupar) y el
 * plan de subtotales (dónde van los cortes de grupo, qué rangos agregar y los
 * grupos del esquema resultante). Un «grupo» opera sobre UNIDADES abstractas
 * (filas o columnas), así el mismo código sirve para ambos ejes.
 * Self-test: packages/office/test/sheets-outline.test.ts (bun test).
 */
import type { SheetGroup, TableTotalFn } from '../types';

/** Máximo nivel de anidamiento (como Excel: 8 niveles; aquí 1..7 + el detalle). */
export const MAX_OUTLINE_LEVEL = 7;

/* ---------- Saneado (persistencia o comando) ---------- */

/** Grupo saneado contra un número de unidades `max` (ROWS o COLS); null si no vale. */
export function sanitizeGroup(raw: unknown, max: number): SheetGroup | null {
  if (!raw || typeof raw !== 'object') return null;
  const g = raw as Partial<SheetGroup>;
  const start = Math.trunc(Number(g.start));
  const end = Math.trunc(Number(g.end));
  const level = Math.trunc(Number(g.level));
  if (!Number.isInteger(start) || !Number.isInteger(end) || !Number.isInteger(level)) return null;
  if (start < 0 || end >= max || start > end) return null;
  if (level < 1 || level > MAX_OUTLINE_LEVEL) return null;
  const out: SheetGroup = { start, end, level };
  if (g.collapsed === true) out.collapsed = true;
  return out;
}

/** Lista de grupos saneada; descarta los inválidos. Vacío ⇒ el campo no se persiste. */
export function sanitizeGroups(raw: unknown, max: number): SheetGroup[] {
  if (!Array.isArray(raw)) return [];
  const out: SheetGroup[] = [];
  for (const g of raw) {
    const c = sanitizeGroup(g, max);
    if (c) out.push(c);
  }
  return sortGroups(out);
}

/** Orden estable: por inicio, luego por nivel (externo primero), luego por fin. */
function sortGroups(groups: SheetGroup[]): SheetGroup[] {
  return groups.sort((a, b) => a.start - b.start || a.level - b.level || a.end - b.end);
}

/* ---------- Geometría del esquema ---------- */

/** Unidades (filas/columnas) OCULTAS: las del detalle de todo grupo colapsado. */
export function hiddenUnits(groups: readonly SheetGroup[]): Set<number> {
  const hidden = new Set<number>();
  for (const g of groups) {
    if (!g.collapsed) continue;
    for (let u = g.start; u <= g.end; u++) hidden.add(u);
  }
  return hidden;
}

/** Nivel de esquema máximo presente (0 si no hay grupos). */
export function maxOutlineLevel(groups: readonly SheetGroup[]): number {
  let m = 0;
  for (const g of groups) if (g.level > m) m = g.level;
  return m;
}

/**
 * Aplica «mostrar hasta el nivel L» (botones 1..N de la barra de niveles): colapsa
 * los grupos de nivel ≥ L y expande los de nivel < L. L=1 deja solo el resumen más
 * externo; L=maxLevel+1 lo expande todo. Devuelve una lista NUEVA (no muta).
 */
export function groupsAtLevel(groups: readonly SheetGroup[], level: number): SheetGroup[] {
  return groups.map((g) => {
    const out: SheetGroup = { start: g.start, end: g.end, level: g.level };
    if (g.level >= level) out.collapsed = true;
    return out;
  });
}

/** Conmuta el colapso del grupo que coincide en start/end/level; el resto intacto. */
export function toggleGroup(
  groups: readonly SheetGroup[],
  start: number,
  end: number,
  level: number,
): SheetGroup[] {
  return groups.map((g) => {
    if (g.start !== start || g.end !== end || g.level !== level) return g;
    const out: SheetGroup = { start: g.start, end: g.end, level: g.level };
    if (!g.collapsed) out.collapsed = true;
    return out;
  });
}

/**
 * Agrupa [start,end]: el nivel = 1 + la profundidad de los grupos que CONTIENEN
 * el rango (anidamiento). No duplica un grupo idéntico ni sobrepasa el tope de
 * anidamiento. Devuelve la lista nueva ordenada.
 */
export function addGroup(
  groups: readonly SheetGroup[],
  start: number,
  end: number,
  max: number,
): SheetGroup[] {
  const s = Math.max(0, Math.min(start, end));
  const e = Math.min(max - 1, Math.max(start, end));
  if (s > e) return [...groups];
  let level = 1;
  for (const g of groups) {
    if (g.start <= s && g.end >= e) level = Math.max(level, g.level + 1);
  }
  if (level > MAX_OUTLINE_LEVEL) return [...groups];
  if (groups.some((g) => g.start === s && g.end === e && g.level === level)) return [...groups];
  return sortGroups([...groups, { start: s, end: e, level }]);
}

/**
 * Desagrupa [start,end]: quita los grupos del nivel MÁS PROFUNDO que solapan la
 * selección (pela una capa de anidamiento, criterio Excel). Sin solape, no cambia.
 */
export function removeGroup(
  groups: readonly SheetGroup[],
  start: number,
  end: number,
): SheetGroup[] {
  const s = Math.min(start, end);
  const e = Math.max(start, end);
  const overlap = groups.filter((g) => g.start <= e && g.end >= s);
  if (overlap.length === 0) return [...groups];
  const deepest = Math.max(...overlap.map((g) => g.level));
  return groups.filter((g) => !(g.level === deepest && g.start <= e && g.end >= s));
}

/* ---------- Subtotales automáticos ---------- */

/** Número de SUBTOTAL de cada agregación (el de 1-11: un grupo contraído no cambia
 *  los totales, que es lo que hace el asistente de cualquier hoja). */
export const SUBTOTAL_FN_NUM: Record<Exclude<TableTotalFn, 'none'>, number> = {
  average: 1,
  count: 2,
  counta: 3,
  max: 4,
  min: 5,
  sum: 9,
};

/** Fila del plan de salida: dato original, subtotal de bloque o total general. */
export type SubtotalOut =
  | { type: 'data'; src: number }
  | { type: 'subtotal'; key: string; from: number; to: number }
  | { type: 'grand'; blocks: [number, number][] };

export interface SubtotalPlan {
  /** Secuencia de filas de salida del cuerpo (sin la cabecera). */
  out: SubtotalOut[];
  /** Grupos del esquema en índices RELATIVOS al cuerpo (0 = 1.ª fila de datos de salida). */
  groups: SheetGroup[];
}

/**
 * Plan de subtotales a partir de las CLAVES de agrupación (el texto de la columna
 * de grupo por cada fila de datos, en orden de lectura). Inserta un subtotal cada
 * vez que la clave CAMBIA respecto a la fila anterior (criterio Excel: los datos
 * se asumen ya ordenados por esa columna) más un total general al final.
 *
 * Genera dos niveles de esquema: nivel 2 = las filas de datos de cada bloque
 * (colapsar deja su subtotal), nivel 1 = todo menos el total general (colapsar
 * deja solo el total general). `from`/`to` y los bloques son índices de SALIDA.
 */
export function planSubtotals(keys: readonly string[]): SubtotalPlan {
  const out: SubtotalOut[] = [];
  const groups: SheetGroup[] = [];
  if (keys.length === 0) return { out, groups };

  const dataBlocks: [number, number][] = [];
  let blockKey = keys[0]!;
  let blockStart = 0; // índice de salida de la 1.ª fila de datos del bloque en curso

  const flush = (key: string): void => {
    const from = blockStart;
    const to = out.length - 1; // última fila de datos añadida
    out.push({ type: 'subtotal', key, from, to });
    groups.push({ start: from, end: to, level: 2 }); // detalle = datos; resumen = el subtotal (fuera)
    dataBlocks.push([from, to]);
    blockStart = out.length; // el próximo bloque arranca tras el subtotal
  };

  keys.forEach((k, i) => {
    if (i > 0 && k !== blockKey) {
      flush(blockKey);
      blockKey = k;
    }
    out.push({ type: 'data', src: i });
  });
  flush(blockKey); // cierra el último bloque

  out.push({ type: 'grand', blocks: dataBlocks });
  groups.push({ start: 0, end: out.length - 2, level: 1 }); // detalle = todo menos el total general
  return { out, groups };
}

/* ---------- Asistente de Subtotales: aplicar y quitar ---------- */

/** Lo que pide el asistente. Coordenadas ABSOLUTAS 0-based; `r1` es la cabecera. */
export interface SubtotalRequest {
  c1: number;
  r1: number;
  c2: number;
  r2: number;
  /** Columna por la que se agrupa (absoluta). */
  groupCol: number;
  fn: Exclude<TableTotalFn, 'none'>;
  /** Columnas a las que se añade el subtotal (absolutas); ausente = las que tengan
   *  números (o algo, con «counta»). */
  cols?: readonly number[];
  /** true = «Quitar todos»: fuera las filas de subtotal y el esquema de filas. */
  remove?: boolean;
  /** Filas de la rejilla. */
  rows: number;
  /** Última fila con contenido de la hoja (−1 si está vacía). */
  maxUsedRow: number;
}

/** Cómo se lee la hoja. Se inyecta: la lógica se prueba sin motor ni DOM. */
export interface SubtotalReader {
  /** Texto crudo (el literal o la fórmula con su «=»). */
  raw: (c: number, r: number) => string;
  /** Texto con el que se agrupa (el valor mostrado). */
  key: (c: number, r: number) => string;
  /** ¿El valor calculado es un número? */
  isNumber: (c: number, r: number) => boolean;
  /** ¿El valor calculado es algo (no vacío)? */
  hasValue: (c: number, r: number) => boolean;
}

/** Los rótulos, ya traducidos. */
export interface SubtotalLabels {
  grand: string;
  total: string;
  totalOf: (key: string) => string;
}

/** Una celda que escribe el asistente (en negrita). `text` es el crudo. */
export interface SubtotalWrite {
  c: number;
  r: number;
  text: string;
}

export type SubtotalResult =
  | { error: 'no-data' | 'no-room' }
  | {
      /** from[r] = de qué fila viene la fila r de la hoja nueva (null: vacía o nueva). */
      from: (number | null)[];
      writes: SubtotalWrite[];
      /** El esquema de filas resultante (sustituye al que hubiera). */
      groups: SheetGroup[];
      /** Última fila del rango tras aplicar (para la selección). */
      lastRow: number;
      /** Bloques de subtotal creados (0 al quitar). */
      blocks: number;
    };

/** ¿Es una fila de subtotal? Alguna celda del rango lleva un SUBTOTAL. Es lo que Excel
 *  y LibreOffice quitan con «Quitar todos» y sustituyen al volver a aplicar. */
const SUBTOTAL_RE = /^=.*\bSUBTOTAL\s*\(/i;

/** Nombre de columna A..Z, AA… (0-based). */
function colLetters(c: number): string {
  let s = '';
  for (let n = c + 1; n > 0; n = Math.floor((n - 1) / 26)) {
    s = String.fromCharCode(65 + ((n - 1) % 26)) + s;
  }
  return s;
}

/**
 * El asistente de Subtotales entero, sin tocar la hoja: dice cómo queda. Agrupa por
 * `groupCol` en cada cambio de valor (los datos se suponen ordenados), pone una fila
 * `SUBTOTAL(n; bloque)` tras cada grupo y un total general `SUBTOTAL(n; todo)` —que no
 * cuenta dos veces porque SUBTOTAL se salta los subtotales—, y crea el esquema de dos
 * niveles. Las filas de subtotal que ya hubiera en el rango se QUITAN antes (la opción
 * «Reemplazar subtotales actuales», marcada por defecto en cualquier hoja).
 *
 * Todo lo que hay debajo del rango se desplaza con él. Si no cabe en la rejilla, no
 * se toca nada ('no-room'). Vectores compartidos con Go:
 * apps/nucleo/formula/analisis/testdata/esquema.json.
 *
 * ponytail: sin subtotales anidados (desmarcar «Reemplazar»), sin salto de página por
 * grupo y con el resumen siempre DEBAJO de los datos; se añaden si alguien los pide.
 */
export function applySubtotals(
  req: SubtotalRequest,
  read: SubtotalReader,
  labels: SubtotalLabels,
): SubtotalResult {
  const { c1, r1, c2, rows } = req;
  const r2 = Math.min(req.r2, rows - 1);
  const isSubtotalRow = (r: number): boolean => {
    for (let c = c1; c <= c2; c++) if (SUBTOTAL_RE.test(read.raw(c, r))) return true;
    return false;
  };
  const isEmptyRow = (r: number): boolean => {
    for (let c = c1; c <= c2; c++) if (read.raw(c, r) !== '') return false;
    return true;
  };
  // Cuerpo = las filas bajo la cabecera, sin las vacías del final.
  const body: number[] = [];
  for (let r = r1 + 1; r <= r2; r++) body.push(r);
  while (body.length > 0 && isEmptyRow(body[body.length - 1]!)) body.pop();
  const data = body.filter((r) => !isSubtotalRow(r));

  const out: (number | null)[] = []; // filas del cuerpo nuevo: la de origen o null
  const writes: SubtotalWrite[] = [];
  let groups: SheetGroup[] = [];
  let blocks = 0;
  if (req.remove) {
    out.push(...data);
  } else {
    if (data.length === 0) return { error: 'no-data' };
    const plan = planSubtotals(data.map((r) => read.key(req.groupCol, r)));
    const wanted = req.cols ?? [];
    const aggCols: number[] = [];
    for (let c = c1; c <= c2; c++) {
      if (c === req.groupCol) continue;
      const pick = req.cols
        ? wanted.includes(c)
        : data.some((r) => (req.fn === 'counta' ? read.hasValue(c, r) : read.isNumber(c, r)));
      if (pick) aggCols.push(c);
    }
    const n = SUBTOTAL_FN_NUM[req.fn];
    const range = (c: number, from: number, to: number): string =>
      `${colLetters(c)}${r1 + from + 2}:${colLetters(c)}${r1 + to + 2}`;
    plan.out.forEach((o, i) => {
      const r = r1 + 1 + i;
      if (o.type === 'data') {
        out.push(data[o.src]!);
        return;
      }
      out.push(null);
      const grand = o.type === 'grand';
      const label = grand ? labels.grand : o.key === '' ? labels.total : labels.totalOf(o.key);
      writes.push({ c: req.groupCol, r, text: label });
      for (const c of aggCols) {
        const [from, to] = grand ? [0, plan.out.length - 2] : [o.from, o.to];
        writes.push({ c, r, text: `=SUBTOTAL(${n};${range(c, from, to)})` });
      }
    });
    groups = plan.groups.map((g) => ({
      start: r1 + 1 + g.start,
      end: Math.min(rows - 1, r1 + 1 + g.end),
      level: g.level,
    }));
    blocks = plan.groups.filter((g) => g.level === 2).length;
  }

  const lastBody = body.length > 0 ? body[body.length - 1]! : r1;
  const shift = out.length - body.length;
  if (r1 + out.length > rows - 1 || req.maxUsedRow + shift > rows - 1) return { error: 'no-room' };
  const from: (number | null)[] = new Array<number | null>(rows).fill(null);
  for (let r = 0; r <= r1; r++) from[r] = r;
  out.forEach((src, i) => (from[r1 + 1 + i] = src));
  for (let r = lastBody + 1; r < rows; r++)
    if (r + shift >= 0 && r + shift < rows) from[r + shift] = r;
  return { from, writes, groups, lastRow: Math.min(rows - 1, r1 + out.length), blocks };
}
