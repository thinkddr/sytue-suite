/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * TABLA DINÁMICA: el cálculo entero, sin DOM ni motor de fórmulas. Recibe el origen
 * (la rejilla de VALORES ya calculados, con su fila de encabezados) y la definición
 * —campos en filas, columnas, valores y filtros— y devuelve la rejilla que se escribe
 * en la hoja, celda a celda, con qué va en negrita y qué formato de número lleva.
 *
 * Es el MISMO cálculo que `apps/nucleo/formula/analisis/dinamica.go` en la app nativa,
 * atado por los vectores de `apps/nucleo/formula/analisis/testdata/dinamica.json`: el
 * mismo origen y la misma definición dan la misma tabla en las dos.
 *
 * Diseño tabular, el de Excel y LibreOffice con los valores en columnas:
 *  - Filtros del informe arriba, uno por fila («Campo | (Todos)»), y una fila vacía.
 *  - Encabezado: con campos en columnas, una fila por campo de columna con sus
 *    elementos, y una última con los nombres de los campos de fila (y de los valores,
 *    si hay más de uno). Sin campos en columnas, una sola fila con todos los nombres.
 *  - Una fila por combinación de elementos de fila; con varios campos de fila, un
 *    subtotal tras cada grupo de los exteriores; y el total general al final. Igual en
 *    columnas.
 *
 * Los elementos se ordenan por su VALOR: primero los números (las fechas lo son),
 * luego el texto en orden español con números naturales, luego los lógicos y los
 * errores, y los vacíos al final. Cada casilla agrega los REGISTROS que le tocan —el
 * promedio de un subtotal es el de sus filas, no el promedio de promedios—.
 */
import { FormulaError, formatNumber, type CellValue } from '@sytue/formula';
import type { NumFmt } from '../types';
import { parseRangeRef, sanitizeNumFmt } from './format';

/** Funciones de resumen de un campo de valor (las de Excel y Calc). `count` cuenta lo
 *  no vacío, como el «Cuenta» de las dos. */
export type PivotFn =
  | 'sum'
  | 'count'
  | 'avg'
  | 'max'
  | 'min'
  | 'product'
  | 'countNums'
  | 'stdev'
  | 'stdevp'
  | 'var'
  | 'varp';

/** «Mostrar valores como». */
export type PivotShow = 'normal' | 'pctGrand' | 'pctCol' | 'pctRow' | 'pctParent';

/** Agrupar fechas (números de serie) por año, trimestre, mes o día. */
export type PivotDate = 'years' | 'quarters' | 'months' | 'days';

/** Un campo en filas, columnas o filtros: la columna del origen (0-based) y, si se
 *  quiere, cómo se agrupa, el orden y los elementos ocultos (por su clave). */
export interface PivotFieldRef {
  field: number;
  date?: PivotDate;
  bin?: { start: number; end: number; step: number };
  order?: 'desc';
  hidden?: string[];
}

/** Un campo de valores: la columna, cómo se resume y cómo se muestra. */
export interface PivotValueRef {
  field: number;
  fn: PivotFn;
  show?: PivotShow;
  /** Nombre propio; ausente = «Suma de Ventas». */
  name?: string;
  /** Formato de número de sus casillas (salvo los porcentajes, que llevan el suyo). */
  fmt?: NumFmt;
}

export interface PivotDef {
  rows: PivotFieldRef[];
  cols: PivotFieldRef[];
  values: PivotValueRef[];
  filters?: PivotFieldRef[];
  /** Campos que filtran SIN estar en la tabla: los de una segmentación de un campo que
   *  no va en filtros, filas ni columnas (en Excel, el pivotField sin eje con elementos
   *  ocultos). Solo cuentan sus `hidden`; no se pintan, y el de un campo que está en la
   *  tabla no cuenta. */
  hiddenFilters?: PivotFieldRef[];
  /** false = sin fila de total general. */
  grandRows?: boolean;
  /** false = sin columna de total general. */
  grandCols?: boolean;
  /** false = sin subtotales de los campos exteriores. */
  subtotals?: boolean;
}

/** Textos de la tabla, ya traducidos. */
export interface PivotLabels {
  blank: string;
  grand: string;
  totalOf: (item: string) => string;
  valueName: (fn: PivotFn, field: string) => string;
  all: string;
  multiple: string;
  quarter: (q: number) => string;
  month: (m: number) => string;
  day: (serial: number) => string;
  /** Texto de un valor de un campo del origen (con su formato, si lo tiene). */
  show: (v: CellValue, field: number) => string;
}

/** Una casilla de la tabla. `fmt.field`: lleva el formato de esa columna del origen;
 *  `fmt.value`: el del campo de valor de ese índice; `fmt.pct`: porcentaje. */
export interface PivotCell {
  v: string | number | boolean;
  bold?: true;
  fmt?: { field: number } | { value: number } | { pct: true };
}

/** La tabla: filas de casillas (null = vacía). */
export type PivotGrid = (PivotCell | null)[][];

/* ---------- Elementos ---------- */

/** Un elemento de un campo: su clave (identidad y filtros), cómo se ordena y su rótulo. */
interface Item {
  key: string;
  rank: number;
  num: number;
  text: string;
  label: PivotCell['v'];
  /** El rótulo es un número del origen tal cual: lleva el formato de su columna. */
  raw: boolean;
}

const COLLATOR = new Intl.Collator('es', { numeric: true });
const EPOCH = Date.UTC(1899, 11, 30);

/** Año, mes (1-12) y día de un número de serie de fecha (sistema de 1900). */
function ymd(serial: number): { y: number; m: number; d: number } {
  const t = new Date(EPOCH + Math.floor(serial) * 86400000);
  return { y: t.getUTCFullYear(), m: t.getUTCMonth() + 1, d: t.getUTCDate() };
}

function itemOf(v: CellValue, ref: PivotFieldRef, labels: PivotLabels): Item {
  if (typeof v === 'number' && Number.isFinite(v)) {
    if (ref.date) {
      const { y, m } = ymd(v);
      switch (ref.date) {
        case 'years':
          return { key: `y${y}`, rank: 0, num: y, text: '', label: String(y), raw: false };
        case 'quarters': {
          const q = Math.ceil(m / 3);
          return { key: `q${q}`, rank: 0, num: q, text: '', label: labels.quarter(q), raw: false };
        }
        case 'months':
          return { key: `m${m}`, rank: 0, num: m, text: '', label: labels.month(m), raw: false };
        default: {
          const d = Math.floor(v);
          return { key: `d${d}`, rank: 0, num: d, text: '', label: labels.day(d), raw: false };
        }
      }
    }
    if (ref.bin && ref.bin.step > 0) {
      const { start, end, step } = ref.bin;
      // Como en Excel y Calc, el final ENTRA: lo que pasa de él va a «>final».
      if (v < start)
        return {
          key: '<',
          rank: 0,
          num: -Infinity,
          text: '',
          label: `<${formatNumber(start)}`,
          raw: false,
        };
      if (v > end)
        return {
          key: '>',
          rank: 0,
          num: Infinity,
          text: '',
          label: `>${formatNumber(end)}`,
          raw: false,
        };
      const i = Math.floor((v - start) / step);
      const lo = start + i * step;
      const hi = Math.min(
        end,
        lo + step - (Number.isInteger(start) && Number.isInteger(step) ? 1 : 0),
      );
      return {
        key: `b${i}`,
        rank: 0,
        num: lo,
        text: '',
        label: `${formatNumber(lo)}-${formatNumber(hi)}`,
        raw: false,
      };
    }
    return { key: formatNumber(v), rank: 0, num: v, text: '', label: v, raw: true };
  }
  if (v instanceof FormulaError)
    return { key: v.code, rank: 3, num: 0, text: v.code, label: v.code, raw: false };
  if (typeof v === 'boolean') {
    const t = v ? 'TRUE' : 'FALSE';
    return { key: t, rank: 2, num: v ? 1 : 0, text: '', label: v, raw: false };
  }
  const s = typeof v === 'string' ? v : '';
  if (s === '') return { key: '', rank: 4, num: 0, text: '', label: labels.blank, raw: false };
  return { key: s, rank: 1, num: 0, text: s, label: s, raw: false };
}

/** Orden de dos elementos; `desc` invierte todo salvo que los vacíos van al final. */
function compareItems(a: Item, b: Item, desc: boolean): number {
  if (a.rank === 4 || b.rank === 4) return a.rank === b.rank ? 0 : a.rank === 4 ? 1 : -1;
  const c =
    a.rank - b.rank ||
    (a.num < b.num ? -1 : a.num > b.num ? 1 : 0) ||
    COLLATOR.compare(a.text, b.text);
  return desc ? -c : c;
}

/* ---------- Árbol de elementos ---------- */

interface Node {
  item: Item | null; // null en la raíz
  path: string; // claves unidas por \u0001 desde la raíz
  depth: number;
  children: Map<string, Node>;
  sorted?: Node[];
}

const SEP = '\u0001';

function newNode(item: Item | null, parent: Node | null): Node {
  const path =
    parent === null ? '' : parent.depth === 0 ? item!.key : `${parent.path}${SEP}${item!.key}`;
  return { item, path, depth: parent === null ? 0 : parent.depth + 1, children: new Map() };
}

/** ¿Es la primera fila (o columna) de su grupo en el nivel d? Solo entonces lleva rótulo. */
function firstAt(prev: Slot | undefined, cur: Slot, d: number): boolean {
  return !prev || prev.line.length <= d || prev.line[d] !== cur.line[d];
}

function insertPath(root: Node, items: Item[]): void {
  let n = root;
  for (const it of items) {
    let c = n.children.get(it.key);
    if (!c) {
      c = newNode(it, n);
      n.children.set(it.key, c);
    }
    n = c;
  }
}

function sortTree(n: Node, refs: readonly PivotFieldRef[]): void {
  const desc = refs[n.depth]?.order === 'desc';
  n.sorted = [...n.children.values()].sort((a, b) => compareItems(a.item!, b.item!, desc));
  for (const c of n.sorted) sortTree(c, refs);
}

/** Una fila o columna de la tabla: un elemento hoja, un subtotal o el total general. */
interface Slot {
  kind: 'leaf' | 'sub' | 'grand';
  node: Node;
  /** Los nodos de la raíz a este (sin la raíz). */
  line: Node[];
}

function slotsOf(root: Node, levels: number, subtotals: boolean, grand: boolean): Slot[] {
  const out: Slot[] = [];
  if (levels === 0) return [{ kind: 'grand', node: root, line: [] }];
  const walk = (n: Node, line: Node[]): void => {
    for (const c of n.sorted ?? []) {
      const l = [...line, c];
      if (c.depth === levels) out.push({ kind: 'leaf', node: c, line: l });
      else {
        walk(c, l);
        if (subtotals) out.push({ kind: 'sub', node: c, line: l });
      }
    }
  };
  walk(root, []);
  if (grand) out.push({ kind: 'grand', node: root, line: [] });
  return out;
}

/* ---------- Resumen ---------- */

interface Acc {
  records: number;
  filled: number;
  nums: number[];
  /** El primer error del campo en estos registros: se propaga, como en una fórmula. */
  err?: string;
}

function summarize(a: Acc | undefined, fn: PivotFn): number | string | null {
  if (!a || a.records === 0) return null;
  if (a.err !== undefined) return a.err;
  const n = a.nums;
  switch (fn) {
    case 'count':
      return a.filled;
    case 'countNums':
      return n.length;
    case 'avg': {
      if (n.length === 0) return '#DIV/0!';
      let s = 0;
      for (const x of n) s += x;
      return s / n.length;
    }
    case 'max':
      return n.length === 0 ? 0 : n.reduce((m, x) => (x > m ? x : m), n[0]!);
    case 'min':
      return n.length === 0 ? 0 : n.reduce((m, x) => (x < m ? x : m), n[0]!);
    case 'product': {
      if (n.length === 0) return 0;
      let p = 1;
      for (const x of n) p *= x;
      return p;
    }
    case 'stdev':
    case 'stdevp':
    case 'var':
    case 'varp': {
      const sample = fn === 'stdev' || fn === 'var';
      if (n.length < (sample ? 2 : 1)) return '#DIV/0!';
      let s = 0;
      for (const x of n) s += x;
      const mean = s / n.length;
      let q = 0;
      for (const x of n) q += (x - mean) * (x - mean);
      const v = q / (n.length - (sample ? 1 : 0));
      return fn === 'stdev' || fn === 'stdevp' ? Math.sqrt(v) : v;
    }
    default: {
      let s = 0;
      for (const x of n) s += x;
      return s;
    }
  }
}

/* ---------- La tabla ---------- */

/**
 * Calcula la tabla dinámica. `grid[0]` son los encabezados del origen; el resto, los
 * registros (las filas vacías del todo se ignoran).
 */
export function computePivotTable(
  grid: readonly (readonly CellValue[])[],
  def: PivotDef,
  labels: PivotLabels,
): PivotGrid {
  const header = grid[0] ?? [];
  const width = header.length;
  const valid = (r: PivotFieldRef | PivotValueRef): boolean =>
    Number.isInteger(r.field) && r.field >= 0 && r.field < width;
  const rows = def.rows.filter(valid);
  const cols = def.cols.filter(valid);
  const values = def.values.filter(valid);
  const filters = (def.filters ?? []).filter(valid);
  // El filtro oculto de un campo que ya está en la tabla duerme: manda el de la tabla.
  const placed = new Set([...filters, ...rows, ...cols].map((r) => r.field));
  const hiddenFilters = (def.hiddenFilters ?? []).filter((r) => valid(r) && !placed.has(r.field));
  const subtotals = def.subtotals !== false;
  const name = (f: number): string => labels.show(header[f] ?? '', -1);

  const body = grid.slice(1).filter((r) => r.some((v) => !(typeof v === 'string' && v === '')));
  const cell = (r: readonly CellValue[], f: number): CellValue => r[f] ?? '';

  // Registros que pasan los filtros y los elementos ocultos de filas y columnas.
  const hiding = [...filters, ...rows, ...cols, ...hiddenFilters].filter(
    (r) => (r.hidden?.length ?? 0) > 0,
  );
  const kept = body.filter((r) =>
    hiding.every((ref) => !ref.hidden!.includes(itemOf(cell(r, ref.field), ref, labels).key)),
  );

  const rowRoot = newNode(null, null);
  const colRoot = newNode(null, null);
  const accs = new Map<string, Acc[]>();
  const acc = (k: string): Acc[] => {
    let a = accs.get(k);
    if (!a) {
      a = values.map(() => ({ records: 0, filled: 0, nums: [] }));
      accs.set(k, a);
    }
    return a;
  };
  for (const r of kept) {
    const ri = rows.map((ref) => itemOf(cell(r, ref.field), ref, labels));
    const ci = cols.map((ref) => itemOf(cell(r, ref.field), ref, labels));
    insertPath(rowRoot, ri);
    insertPath(colRoot, ci);
    for (let a = 0; a <= ri.length; a++) {
      const rk = ri
        .slice(0, a)
        .map((i) => i.key)
        .join(SEP);
      for (let b = 0; b <= ci.length; b++) {
        const ck = ci
          .slice(0, b)
          .map((i) => i.key)
          .join(SEP);
        const list = acc(`${a}${SEP}${rk}${SEP}${SEP}${b}${SEP}${ck}`);
        values.forEach((vr, i) => {
          const x = cell(r, vr.field);
          const t = list[i]!;
          t.records++;
          if (!(typeof x === 'string' && x === '')) t.filled++;
          if (typeof x === 'number' && Number.isFinite(x)) t.nums.push(x);
          else if (x instanceof FormulaError) t.err ??= x.code;
        });
      }
    }
  }
  sortTree(rowRoot, rows);
  sortTree(colRoot, cols);

  const rowSlots = slotsOf(
    rowRoot,
    rows.length,
    subtotals && rows.length > 1,
    def.grandRows !== false,
  );
  const colSlots = slotsOf(
    colRoot,
    cols.length,
    subtotals && cols.length > 1,
    def.grandCols !== false,
  );
  const keyOf = (n: Node): string => `${n.depth}${SEP}${n.path}`;
  const raw = (rs: Node, cs: Node, i: number): number | string | null =>
    summarize(accs.get(`${keyOf(rs)}${SEP}${SEP}${keyOf(cs)}`)?.[i], values[i]!.fn);

  const parentOf = (n: Node, line: Node[]): Node => (n.depth <= 1 ? rowRoot : line[n.depth - 2]!);
  const shown = (rs: Slot, cs: Slot, i: number): PivotCell | null => {
    const v = raw(rs.node, cs.node, i);
    if (v === null) return null;
    const vr = values[i]!;
    const show = vr.show ?? 'normal';
    // Sin campos en columnas, la única columna de valores no es un total: no va en negrita.
    const bold = rs.kind !== 'leaf' || (cs.kind !== 'leaf' && cols.length > 0);
    const out = (x: string | number, fmt: NonNullable<PivotCell['fmt']>): PivotCell =>
      bold ? { v: x, bold: true, fmt } : { v: x, fmt };
    if (show === 'normal' || typeof v === 'string') return out(v, { value: i });
    const base =
      show === 'pctGrand'
        ? raw(rowRoot, colRoot, i)
        : show === 'pctCol'
          ? raw(rowRoot, cs.node, i)
          : show === 'pctRow'
            ? raw(rs.node, colRoot, i)
            : raw(parentOf(rs.node, rs.line), cs.node, i);
    if (typeof base !== 'number' || base === 0) return out('#DIV/0!', { pct: true });
    return out(v / base, { pct: true });
  };

  const out: PivotGrid = [];
  const L = Math.max(rows.length, 1);
  const V = values.length;
  const valueCols = V === 0 ? 0 : colSlots.length * V;
  const W = Math.max(L + valueCols, filters.length > 0 ? 2 : 1);
  const blankRow = (): (PivotCell | null)[] => new Array<PivotCell | null>(W).fill(null);
  const text = (it: Item, field: number): string =>
    typeof it.label === 'string' ? it.label : labels.show(it.label, it.raw ? field : -1);
  const labelCell = (it: Item, field: number): PivotCell =>
    it.raw ? { v: it.label, fmt: { field } } : { v: it.label };
  const valueName = (i: number): string =>
    values[i]!.name ?? labels.valueName(values[i]!.fn, name(values[i]!.field));

  // Filtros del informe.
  if (filters.length > 0) {
    for (const ref of filters) {
      const line = blankRow();
      line[0] = { v: name(ref.field) };
      const keys = new Map<string, Item>();
      for (const r of body) {
        const it = itemOf(cell(r, ref.field), ref, labels);
        keys.set(it.key, it);
      }
      const visible = [...keys.values()].filter((it) => !(ref.hidden ?? []).includes(it.key));
      line[1] =
        (ref.hidden?.length ?? 0) === 0
          ? { v: labels.all }
          : visible.length === 1
            ? labelCell(visible[0]!, ref.field)
            : { v: labels.multiple };
      out.push(line);
    }
    out.push(blankRow());
  }

  // Encabezado.
  if (cols.length === 0) {
    const line = blankRow();
    rows.forEach((ref, i) => (line[i] = { v: name(ref.field), bold: true }));
    for (let i = 0; i < V; i++) line[L + i] = { v: valueName(i), bold: true };
    out.push(line);
  } else {
    for (let d = 0; d < cols.length; d++) {
      const line = blankRow();
      if (d === 0 && V === 1) line[0] = { v: valueName(0), bold: true };
      colSlots.forEach((cs, s) => {
        const x = L + s * V;
        if (V === 0) return;
        if (cs.kind === 'grand') {
          if (d === 0) line[x] = { v: labels.grand, bold: true };
          return;
        }
        if (cs.kind === 'sub' && cs.node.depth === d + 1) {
          line[x] = { v: labels.totalOf(text(cs.node.item!, cols[d]!.field)), bold: true };
          return;
        }
        if (d < cs.line.length && firstAt(colSlots[s - 1], cs, d))
          line[x] = { ...labelCell(cs.line[d]!.item!, cols[d]!.field), bold: true };
      });
      out.push(line);
    }
    const line = blankRow();
    rows.forEach((ref, i) => (line[i] = { v: name(ref.field), bold: true }));
    if (V > 1) {
      colSlots.forEach((_, s) => {
        for (let i = 0; i < V; i++) line[L + s * V + i] = { v: valueName(i), bold: true };
      });
    }
    out.push(line);
  }

  // Cuerpo.
  rowSlots.forEach((rs, k) => {
    const line = blankRow();
    if (rs.kind === 'grand') line[0] = { v: labels.grand, bold: true };
    else if (rs.kind === 'sub') {
      const d = rs.node.depth - 1;
      line[d] = { v: labels.totalOf(text(rs.node.item!, rows[d]!.field)), bold: true };
    } else {
      rs.line.forEach((n, d) => {
        if (firstAt(rowSlots[k - 1], rs, d)) line[d] = labelCell(n.item!, rows[d]!.field);
      });
    }
    colSlots.forEach((cs, s) => {
      for (let i = 0; i < V; i++) line[L + s * V + i] = shown(rs, cs, i);
    });
    out.push(line);
  });
  return out;
}

/* ---------- Lo que se guarda en la hoja ---------- */

/** Una tabla dinámica de la hoja: su definición, de dónde lee y dónde escribe. */
export interface SheetPivot extends PivotDef {
  /** Nombre, único en la hoja («TablaDinámica1»). */
  name: string;
  /** Rango del origen con su fila de encabezados («A1:F40»). */
  source: string;
  /** Hoja del origen; ausente = la misma. */
  sourceSheet?: string;
  /** Esquina de arriba a la izquierda de la tabla («H1»). */
  anchor: string;
  /** Lo que ocupó la última vez que se escribió: se vacía al actualizar. */
  range?: string;
}

export const PIVOT_FNS: readonly PivotFn[] = [
  'sum',
  'count',
  'avg',
  'max',
  'min',
  'product',
  'countNums',
  'stdev',
  'stdevp',
  'var',
  'varp',
];
export const PIVOT_SHOWS: readonly PivotShow[] = [
  'normal',
  'pctGrand',
  'pctCol',
  'pctRow',
  'pctParent',
];
export const PIVOT_DATES: readonly PivotDate[] = ['years', 'quarters', 'months', 'days'];

function sanitizeFieldRef(raw: unknown): PivotFieldRef | null {
  if (!raw || typeof raw !== 'object') return null;
  const r = raw as Record<string, unknown>;
  const field = Math.trunc(Number(r['field']));
  if (!Number.isInteger(field) || field < 0) return null;
  const out: PivotFieldRef = { field };
  if (PIVOT_DATES.includes(r['date'] as PivotDate)) out.date = r['date'] as PivotDate;
  const bin = r['bin'] as Record<string, unknown> | undefined;
  if (bin && typeof bin === 'object') {
    const [start, end, step] = [Number(bin['start']), Number(bin['end']), Number(bin['step'])];
    if ([start, end, step].every(Number.isFinite) && step > 0 && end >= start)
      out.bin = { start, end, step };
  }
  if (r['order'] === 'desc') out.order = 'desc';
  if (Array.isArray(r['hidden'])) {
    const h = r['hidden'].filter((x): x is string => typeof x === 'string');
    if (h.length > 0) out.hidden = h;
  }
  return out;
}

function sanitizeValueRef(raw: unknown): PivotValueRef | null {
  if (!raw || typeof raw !== 'object') return null;
  const r = raw as Record<string, unknown>;
  const field = Math.trunc(Number(r['field']));
  if (!Number.isInteger(field) || field < 0) return null;
  const fn = PIVOT_FNS.includes(r['fn'] as PivotFn) ? (r['fn'] as PivotFn) : 'sum';
  const out: PivotValueRef = { field, fn };
  if (PIVOT_SHOWS.includes(r['show'] as PivotShow) && r['show'] !== 'normal')
    out.show = r['show'] as PivotShow;
  if (typeof r['name'] === 'string' && r['name'].trim() !== '')
    out.name = r['name'].trim().slice(0, 120);
  const fmt = sanitizeNumFmt(r['fmt']);
  if (fmt) out.fmt = fmt;
  return out;
}

const list = <T>(raw: unknown, f: (x: unknown) => T | null): T[] =>
  Array.isArray(raw) ? raw.map(f).filter((x): x is T => x !== null) : [];

/** La tabla dinámica leída del documento, o null si no vale. Lo que no se entiende de
 *  un campo se descarta; sin nombre, origen o esquina, la tabla entera. */
export function sanitizePivot(raw: unknown): SheetPivot | null {
  if (!raw || typeof raw !== 'object') return null;
  const r = raw as Record<string, unknown>;
  const name = typeof r['name'] === 'string' ? r['name'].trim() : '';
  const source = typeof r['source'] === 'string' ? r['source'].trim().toUpperCase() : '';
  const anchor = typeof r['anchor'] === 'string' ? r['anchor'].trim().toUpperCase() : '';
  if (name === '' || !parseRangeRef(source) || !parseRangeRef(anchor)) return null;
  const out: SheetPivot = {
    name: name.slice(0, 120),
    source,
    anchor,
    rows: list(r['rows'], sanitizeFieldRef),
    cols: list(r['cols'], sanitizeFieldRef),
    values: list(r['values'], sanitizeValueRef),
  };
  const filters = list(r['filters'], sanitizeFieldRef);
  if (filters.length > 0) out.filters = filters;
  // Un filtro oculto sin nada oculto no filtra: no se guarda.
  const hiddenFilters = list(r['hiddenFilters'], sanitizeFieldRef).filter((f) => f.hidden);
  if (hiddenFilters.length > 0) out.hiddenFilters = hiddenFilters;
  if (typeof r['sourceSheet'] === 'string' && r['sourceSheet'] !== '')
    out.sourceSheet = r['sourceSheet'];
  if (r['grandRows'] === false) out.grandRows = false;
  if (r['grandCols'] === false) out.grandCols = false;
  if (r['subtotals'] === false) out.subtotals = false;
  if (typeof r['range'] === 'string' && parseRangeRef(r['range']))
    out.range = r['range'].toUpperCase();
  return out;
}

/** El siguiente nombre libre: «TablaDinámica1», «TablaDinámica2»… (el prefijo, traducido). */
export function nextPivotName(prefix: string, taken: readonly string[]): string {
  for (let i = 1; ; i++) if (!taken.includes(`${prefix}${i}`)) return `${prefix}${i}`;
}

/** Los elementos distintos de un campo del origen, en el orden de la tabla: su clave
 *  (lo que se guarda en `hidden`) y su rótulo. Para la lista de «elementos que se ven». */
export function pivotItems(
  grid: readonly (readonly CellValue[])[],
  ref: PivotFieldRef,
  labels: PivotLabels,
): { key: string; label: string }[] {
  const seen = new Map<string, Item>();
  for (const r of grid.slice(1)) {
    if (!r.some((v) => !(typeof v === 'string' && v === ''))) continue;
    const it = itemOf(r[ref.field] ?? '', ref, labels);
    if (!seen.has(it.key)) seen.set(it.key, it);
  }
  return [...seen.values()]
    .sort((a, b) => compareItems(a, b, ref.order === 'desc'))
    .map((it) => ({
      key: it.key,
      label:
        typeof it.label === 'string' ? it.label : labels.show(it.label, it.raw ? ref.field : -1),
    }));
}
