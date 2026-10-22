/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Filtros por CONDICIÓN del autofiltro de Calcus (mayor/menor/entre, contiene/
 * empieza/termina, vacías, N superiores/inferiores, por encima/debajo de la media,
 * por color de relleno o de texto) y VISTAS de filtro con nombre.
 *
 * Todo PURO (sin DOM ni estado): el lienzo le da las celdas ya calculadas y recibe
 * las filas ocultas. Una condición vive en `SheetAutoFilter.cond[col]` (modelo
 * compartido); una VISTA es una foto `{excl, cond}` con nombre que es de UNA persona:
 * se guarda en su navegador (clave por usuario y documento, como las macros) y nunca
 * entra en el modelo, así que activarla no cambia lo que ven los demás.
 * Self-test: packages/office/test/sheets-filtro.test.ts (bun test).
 */
import type { CellValue } from '@sytue/formula';
import type { CellStyle, FilterCondition, FilterOp, SheetAutoFilter } from '../types';
import { formatCellValue, parseEsNumberStrict } from './format';

/** Los operadores, en el orden en que los enseña el menú. */
export const FILTER_OPS: readonly FilterOp[] = [
  'gt',
  'ge',
  'lt',
  'le',
  'eq',
  'ne',
  'between',
  'notBetween',
  'contains',
  'notContains',
  'begins',
  'ends',
  'blank',
  'notBlank',
  'top',
  'bottom',
  'aboveAvg',
  'belowAvg',
  'fill',
  'fontColor',
];

/** Cuántos operandos de texto pide cada operador (0, 1 o 2). */
export function operandCount(op: FilterOp): 0 | 1 | 2 {
  if (op === 'between' || op === 'notBetween') return 2;
  if (
    op === 'blank' ||
    op === 'notBlank' ||
    op === 'top' ||
    op === 'bottom' ||
    op === 'aboveAvg' ||
    op === 'belowAvg' ||
    op === 'fill' ||
    op === 'fontColor'
  )
    return 0;
  return 1;
}

/** Una celda del cuerpo del filtro: su valor calculado y su estilo manual. */
export interface FilterCell {
  v: CellValue;
  style?: CellStyle | undefined;
}

const isHex = (s: unknown): s is string => typeof s === 'string' && /^#[0-9a-fA-F]{6}$/.test(s);

/** Condición saneada desde datos no confiables (documento, comando o vista); null si no vale. */
export function sanitizeFilterCond(raw: unknown): FilterCondition | null {
  if (!raw || typeof raw !== 'object') return null;
  const r = raw as Record<string, unknown>;
  const op = FILTER_OPS.find((o) => o === r['op']);
  if (!op) return null;
  const out: FilterCondition = { op };
  const k = operandCount(op);
  const text = (x: unknown): string | null =>
    typeof x === 'string' && x.length <= 255 && x.trim() !== '' ? x : null;
  if (k >= 1) {
    const a = text(r['a']);
    if (a === null) return null;
    out.a = a;
  }
  if (k === 2) {
    const b = text(r['b']);
    if (b === null) return null;
    out.b = b;
  }
  if (op === 'top' || op === 'bottom') {
    const n = Math.trunc(Number(r['n']));
    if (!Number.isFinite(n) || n < 1 || n > 100000) return null;
    out.n = n;
    if (r['percent'] === true && n <= 100) out.percent = true;
  }
  if (op === 'fill' || op === 'fontColor') {
    if (!isHex(r['color'])) return null;
    out.color = r['color'].toLowerCase();
  }
  return out;
}

/** `cond` de un autofiltro saneado columna a columna (dentro de [c1, c2]); undefined si queda vacío. */
export function sanitizeFilterConds(
  raw: unknown,
  c1: number,
  c2: number,
): Record<string, FilterCondition> | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const out: Record<string, FilterCondition> = {};
  for (const key in raw as Record<string, unknown>) {
    const c = Number(key);
    if (!Number.isInteger(c) || c < c1 || c > c2) continue;
    const cond = sanitizeFilterCond((raw as Record<string, unknown>)[key]);
    if (cond) out[String(c)] = cond;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

const num = (v: CellValue): number | null => (typeof v === 'number' ? v : null);

/**
 * ¿Pasa cada celda la condición? Devuelve un booleano por celda (true = se ve). Los
 * N superiores y la media se calculan sobre TODA la columna del cuerpo, no sobre lo que
 * ya dejan pasar otras columnas (como el formato condicional).
 */
export function evalFilterColumn(cond: FilterCondition, cells: readonly FilterCell[]): boolean[] {
  const nums = cells.map((c) => num(c.v)).filter((n): n is number => n !== null);
  const a = cond.a ?? '';
  const aNum = parseEsNumberStrict(a);
  const bNum = parseEsNumberStrict(cond.b ?? '');
  const aLow = a.trim().toLowerCase();
  let umbral: number | null = null;
  if ((cond.op === 'top' || cond.op === 'bottom') && nums.length > 0) {
    const sorted = [...nums].sort((x, y) => (cond.op === 'top' ? y - x : x - y));
    const count = cond.percent
      ? Math.max(1, Math.ceil(((cond.n ?? 1) / 100) * sorted.length))
      : Math.min(cond.n ?? 1, sorted.length);
    umbral = sorted[count - 1]!; // incluye empates (criterio Excel)
  }
  const media = nums.length > 0 ? nums.reduce((s, n) => s + n, 0) / nums.length : null;
  const color = cond.color?.toLowerCase();

  return cells.map(({ v, style }) => {
    const n = num(v);
    const text = formatCellValue(v);
    const low = text.trim().toLowerCase();
    const enRango = (lo: number, hi: number): boolean =>
      n !== null && n >= Math.min(lo, hi) && n <= Math.max(lo, hi);
    switch (cond.op) {
      case 'gt':
      case 'ge':
      case 'lt':
      case 'le': {
        if (n !== null && aNum !== null)
          return cond.op === 'gt'
            ? n > aNum
            : cond.op === 'ge'
              ? n >= aNum
              : cond.op === 'lt'
                ? n < aNum
                : n <= aNum;
        if (low === '' || (n !== null) !== (aNum !== null)) return false; // texto contra número: no se ordenan
        const o = low.localeCompare(aLow, 'es', { numeric: true });
        return cond.op === 'gt'
          ? o > 0
          : cond.op === 'ge'
            ? o >= 0
            : cond.op === 'lt'
              ? o < 0
              : o <= 0;
      }
      case 'eq':
        return n !== null && aNum !== null ? n === aNum : low === aLow;
      case 'ne':
        return n !== null && aNum !== null ? n !== aNum : low !== aLow;
      case 'between':
        return aNum !== null && bNum !== null && enRango(aNum, bNum);
      case 'notBetween':
        // Una celda sin número no está «fuera» de nada: se oculta, no se cuela.
        return aNum !== null && bNum !== null && n !== null && !enRango(aNum, bNum);
      case 'contains':
        return low.includes(aLow);
      case 'notContains':
        return !low.includes(aLow);
      case 'begins':
        return low.startsWith(aLow);
      case 'ends':
        return low.endsWith(aLow);
      case 'blank':
        return text === '';
      case 'notBlank':
        return text !== '';
      case 'top':
        return n !== null && umbral !== null && n >= umbral;
      case 'bottom':
        return n !== null && umbral !== null && n <= umbral;
      case 'aboveAvg':
        return n !== null && media !== null && n > media;
      case 'belowAvg':
        return n !== null && media !== null && n < media;
      case 'fill':
        return color !== undefined && style?.fill?.toLowerCase() === color;
      case 'fontColor':
        return color !== undefined && style?.color?.toLowerCase() === color;
    }
  });
}

/** Lo que el filtro necesita leer del lienzo, por posición (col, fila) 0-based. */
export interface FilterSource {
  value: (c: number, r: number) => CellValue;
  style: (c: number, r: number) => CellStyle | undefined;
}

/**
 * Filas ocultas por un autofiltro (excl + cond): una fila del cuerpo se oculta si en
 * ALGUNA columna filtrada no pasa (intersección, criterio Excel). `protect` son filas
 * que nunca se ocultan (p. ej. el total de una tabla). `rows` es el nº de filas de la hoja.
 */
export function hiddenByFilter(
  af: Pick<SheetAutoFilter, 'row' | 'c1' | 'c2' | 'excl' | 'cond'> | undefined,
  src: FilterSource,
  rows: number,
  protect?: ReadonlySet<number>,
): Set<number> {
  const hidden = new Set<number>();
  if (!af) return hidden;
  for (const key in af.excl) {
    const c = Number(key);
    const excl = new Set(af.excl[key]);
    if (excl.size === 0 || c < af.c1 || c > af.c2) continue;
    for (let r = af.row + 1; r < rows; r++) {
      if (!protect?.has(r) && excl.has(formatCellValue(src.value(c, r)))) hidden.add(r);
    }
  }
  for (const key in af.cond) {
    const c = Number(key);
    const cond = af.cond[key];
    if (!cond || c < af.c1 || c > af.c2) continue;
    const body: FilterCell[] = [];
    for (let r = af.row + 1; r < rows; r++)
      body.push({ v: src.value(c, r), style: src.style(c, r) });
    const pasa = evalFilterColumn(cond, body);
    pasa.forEach((ok, i) => {
      const r = af.row + 1 + i;
      if (!ok && !protect?.has(r)) hidden.add(r);
    });
  }
  return hidden;
}

/* ---------- Edición de criterios (compartida con las vistas) ---------- */

/** Los criterios de un filtro: lo que comparten el autofiltro y una vista. */
export type Criterios = Pick<SheetAutoFilter, 'excl' | 'cond'>;

/** Criterios sin claves vacías ni `undefined` (el modelo guarda JSON mínimo). */
const crit = (excl: Record<string, string[]>, cond: Record<string, FilterCondition>): Criterios => {
  const out: Criterios = {};
  if (Object.keys(excl).length > 0) out.excl = excl;
  if (Object.keys(cond).length > 0) out.cond = cond;
  return out;
};

/** Criterios con los valores EXCLUIDOS de `col` sustituidos; excluir algo quita la condición de
 *  esa columna (los valores y la condición son dos maneras de filtrar la misma columna). */
export function setExcluded(t: Criterios, col: number, excluded: readonly string[]): Criterios {
  const excl = { ...t.excl };
  const cond = { ...t.cond };
  if (excluded.length > 0) {
    excl[String(col)] = [...excluded];
    delete cond[String(col)];
  } else delete excl[String(col)];
  return crit(excl, cond);
}

/** Oculta o muestra UN valor de la columna. */
export const toggleValue = (
  t: Criterios,
  col: number,
  value: string,
  hidden: boolean,
): Criterios => {
  const set = new Set(t.excl?.[String(col)] ?? []);
  if (hidden) set.add(value);
  else set.delete(value);
  return setExcluded(t, col, [...set]);
};

/** Pone (o con `null` quita) la condición de `col`; ponerla quita los valores excluidos de esa columna. */
export function setCond(t: Criterios, col: number, cond: FilterCondition | null): Criterios {
  const excl = { ...t.excl };
  const conds = { ...t.cond };
  if (cond) {
    conds[String(col)] = cond;
    delete excl[String(col)];
  } else delete conds[String(col)];
  return crit(excl, conds);
}

/* ---------- Vistas de filtro (por persona, fuera del modelo compartido) ---------- */

/** Una vista: la foto de los criterios del filtro, con nombre. */
export interface FilterView {
  id: string;
  name: string;
  excl?: Record<string, string[]>;
  cond?: Record<string, FilterCondition>;
}

const MAX_VIEWS = 50;

/** Clave de almacenamiento: por usuario Y documento, para no mezclar cuentas ni libros. */
export const filterViewsKey = (userSub: string | undefined, docId: string): string =>
  `sytue:docs:filterviews:${userSub ?? 'anon'}:${docId}`;

/** Lo que se guarda de una persona en un documento: sus vistas por hoja y la que tiene activa en cada una. */
export interface FilterViewsState {
  views: Record<string, FilterView[]>;
  active: Record<string, string>;
}

/** La vista activa de una hoja, o null (= el filtro compartido). */
export const activeView = (st: FilterViewsState, sheet: string): FilterView | null =>
  st.views[sheet]?.find((v) => v.id === st.active[sheet]) ?? null;

/** El estado persistido; tolerante (frontera de confianza), nunca lanza. */
export function parseFilterViews(json: string | null): FilterViewsState {
  const out: FilterViewsState = { views: {}, active: {} };
  let data: unknown;
  try {
    data = json ? JSON.parse(json) : null;
  } catch {
    return out;
  }
  if (!data || typeof data !== 'object') return out;
  const { views, active } = data as { views?: unknown; active?: unknown };
  if (views && typeof views === 'object') {
    for (const sheet in views as Record<string, unknown>) {
      const list = (views as Record<string, unknown>)[sheet];
      if (!Array.isArray(list)) continue;
      const ok: FilterView[] = [];
      for (const raw of list.slice(0, MAX_VIEWS)) {
        if (!raw || typeof raw !== 'object') continue;
        const r = raw as Record<string, unknown>;
        if (typeof r['id'] !== 'string' || typeof r['name'] !== 'string' || r['name'].trim() === '')
          continue;
        const view: FilterView = { id: r['id'], name: r['name'].slice(0, 60) };
        const excl = sanitizeExcl(r['excl']);
        if (excl) view.excl = excl;
        const cond = sanitizeFilterConds(r['cond'], 0, Number.MAX_SAFE_INTEGER);
        if (cond) view.cond = cond;
        ok.push(view);
      }
      if (ok.length > 0) out.views[sheet] = ok;
    }
  }
  if (active && typeof active === 'object') {
    for (const sheet in active as Record<string, unknown>) {
      const id = (active as Record<string, unknown>)[sheet];
      if (typeof id === 'string' && out.views[sheet]?.some((v) => v.id === id))
        out.active[sheet] = id;
    }
  }
  return out;
}

/**
 * Une dos estados de vistas sin perder ninguna: las de `base` (el servidor) mandan en un
 * conflicto de id y las que solo trae `extra` (las del navegador, aún sin subir) se añaden,
 * con el tope de vistas por hoja. La vista activa es la de `base` y, si no hay, la de `extra`.
 */
export function mergeFilterViews(
  base: FilterViewsState,
  extra: FilterViewsState,
): FilterViewsState {
  const views: Record<string, FilterView[]> = {};
  for (const sheet of new Set([...Object.keys(base.views), ...Object.keys(extra.views)])) {
    const lista = [...(base.views[sheet] ?? [])];
    for (const v of extra.views[sheet] ?? []) {
      if (lista.length < MAX_VIEWS && !lista.some((x) => x.id === v.id)) lista.push(v);
    }
    if (lista.length > 0) views[sheet] = lista;
  }
  const active: Record<string, string> = {};
  for (const sheet in views) {
    const id = base.active[sheet] ?? extra.active[sheet];
    if (id && views[sheet]?.some((v) => v.id === id)) active[sheet] = id;
  }
  return { views, active };
}

/** ¿No hay ninguna vista? */
export const emptyFilterViews = (st: FilterViewsState): boolean =>
  Object.keys(st.views).length === 0;

function sanitizeExcl(raw: unknown): Record<string, string[]> | undefined {
  if (!raw || typeof raw !== 'object') return undefined;
  const out: Record<string, string[]> = {};
  for (const key in raw as Record<string, unknown>) {
    const vals = (raw as Record<string, unknown>)[key];
    if (!Number.isInteger(Number(key)) || !Array.isArray(vals)) continue;
    const list = vals.filter((v): v is string => typeof v === 'string');
    if (list.length > 0) out[key] = list;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

/** La misma vista con otros criterios. */
export function viewWithCriterios(v: FilterView, t: Criterios): FilterView {
  const out: FilterView = { id: v.id, name: v.name };
  if (t.excl) out.excl = t.excl;
  if (t.cond) out.cond = t.cond;
  return out;
}

/** Foto de los criterios actuales de un autofiltro como vista nueva. */
export function viewFromFilter(
  id: string,
  name: string,
  af: Pick<SheetAutoFilter, 'excl' | 'cond'>,
): FilterView {
  const v: FilterView = { id, name: name.trim().slice(0, 60) };
  if (af.excl) v.excl = structuredClone(af.excl);
  if (af.cond) v.cond = structuredClone(af.cond);
  return v;
}

/** El autofiltro con otros criterios (la fila y las columnas son las suyas). */
export function withCriterios(af: SheetAutoFilter, t: Criterios): SheetAutoFilter {
  const out: SheetAutoFilter = { row: af.row, c1: af.c1, c2: af.c2 };
  if (t.excl) out.excl = t.excl;
  if (t.cond) out.cond = t.cond;
  return out;
}

/**
 * El autofiltro que SE VE: el compartido con los criterios de la vista activa encima.
 * Sin vista, el propio autofiltro. Solo cambia lo que ve esta persona; el modelo no se toca.
 */
export function withView(
  af: SheetAutoFilter,
  view: FilterView | null | undefined,
): SheetAutoFilter {
  return view ? withCriterios(af, view) : af;
}
