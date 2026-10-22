/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Helpers PUROS de las Tablas con formato (ListObject de Excel): catálogo de
 * estilos sobrios (sin degradados), saneado del SheetTable persistido, geometría
 * (rol cabecera/datos/totales de cada celda, banda alterna), auto-expansión al
 * escribir al lado, agregación de la fila de totales (SUM/AVERAGE/COUNT… con
 * semántica SUBTOTAL: el lienzo le pasa solo las filas visibles) y utilidades de
 * nombres únicos (tabla y columnas). Sin DOM ni estado de UI: los DATOS siguen
 * viviendo en `cells` (el texto crudo manda); la tabla solo aporta formato y
 * comportamiento. Self-test: packages/office/test/sheets-tables.test.ts (bun test).
 */
import { ERR, FormulaError } from '@sytue/formula';
import type { CellValue, RefRange, TableQuery } from '@sytue/formula';
import type { SheetTable, TableTotalFn } from '../types';
import { parseRangeRef } from './format';
import type { RangeRect } from './format';
import type { DocsKey } from '../../i18n';

/* ---------- Catálogo de estilos (claro/medio, SIN degradados) ---------- */

export interface TableStyle {
  id: string;
  /** Clave del catálogo con el nombre del estilo (antes era un rótulo fijo en castellano). */
  labelKey: DocsKey;
  /** Fondo de la fila de cabecera (#RRGGBB). */
  headerBg: string;
  /** Tinta de la cabecera (#RRGGBB). */
  headerFg: string;
  /** Relleno de las filas de datos con banda (las alternas). */
  band: string;
  /** Color del contorno y del separador de cabecera. */
  border: string;
}

/** 5 estilos sobrios (familias slate/azul/verde/ámbar + claro neutro), sin
 *  degradados: cabecera de color sólido, banda alterna muy tenue, contorno fino. */
export const TABLE_STYLES: readonly TableStyle[] = [
  {
    id: 'slate',
    labelKey: 'table.style.slate',
    headerBg: '#475569',
    headerFg: '#FFFFFF',
    band: '#F1F5F9',
    border: '#CBD5E1',
  },
  {
    id: 'blue',
    labelKey: 'table.style.blue',
    headerBg: '#2563EB',
    headerFg: '#FFFFFF',
    band: '#EFF6FF',
    border: '#BFDBFE',
  },
  {
    id: 'green',
    labelKey: 'table.style.green',
    headerBg: '#16A34A',
    headerFg: '#FFFFFF',
    band: '#F0FDF4',
    border: '#BBF7D0',
  },
  {
    id: 'amber',
    labelKey: 'table.style.amber',
    headerBg: '#B45309',
    headerFg: '#FFFFFF',
    band: '#FFFBEB',
    border: '#FDE68A',
  },
  {
    id: 'plain',
    labelKey: 'table.style.plain',
    headerBg: '#E2E8F0',
    headerFg: '#0A0A0A',
    band: '#F8FAFC',
    border: '#CBD5E1',
  },
];

const STYLE_IDS = new Set(TABLE_STYLES.map((s) => s.id));

/** Estilo por id; el primero (Pizarra) si el id falta o no existe. */
export function tableStyleOf(id: string | undefined): TableStyle {
  return TABLE_STYLES.find((s) => s.id === id) ?? TABLE_STYLES[0]!;
}

/* ---------- Funciones de la fila de totales ---------- */

export const TABLE_TOTAL_FNS: { value: TableTotalFn; labelKey: DocsKey }[] = [
  { value: 'none', labelKey: 'table.total.none' },
  { value: 'sum', labelKey: 'query.agg.sum' },
  { value: 'average', labelKey: 'query.agg.avg' },
  { value: 'count', labelKey: 'table.total.count' },
  { value: 'counta', labelKey: 'table.total.counta' },
  { value: 'min', labelKey: 'query.agg.min' },
  { value: 'max', labelKey: 'query.agg.max' },
];

const TOTAL_FNS: readonly TableTotalFn[] = TABLE_TOTAL_FNS.map((f) => f.value);

export function isTotalFn(v: unknown): v is TableTotalFn {
  return typeof v === 'string' && TOTAL_FNS.includes(v as TableTotalFn);
}

/**
 * Agrega los valores de una columna para la fila de totales (paridad Excel). El
 * lienzo pasa SOLO los valores de las filas visibles (semántica SUBTOTAL: ignora
 * las filtradas). `counta` cuenta celdas no vacías; el resto solo miran números.
 * Devuelve '' (celda en blanco) cuando no hay nada que agregar.
 */
export function tableTotalValue(fn: TableTotalFn, values: readonly CellValue[]): CellValue {
  if (fn === 'none') return '';
  if (fn === 'counta') return values.filter((v) => v !== '' && !(v instanceof FormulaError)).length;
  const nums = values.filter((v): v is number => typeof v === 'number' && Number.isFinite(v));
  if (fn === 'count') return nums.length;
  if (nums.length === 0) return fn === 'sum' ? 0 : '';
  switch (fn) {
    case 'sum':
      return nums.reduce((a, b) => a + b, 0);
    case 'average':
      return nums.reduce((a, b) => a + b, 0) / nums.length;
    case 'min':
      return Math.min(...nums);
    case 'max':
      return Math.max(...nums);
    default:
      return '';
  }
}

/* ---------- Nombres únicos ---------- */

/** Nombre de tabla libre en el libro: «Tabla1», «Tabla2»… (cotejo sin mayúsculas). */
export function uniqueTableName(existing: readonly string[], base = 'Tabla'): string {
  const taken = new Set(existing.map((n) => n.toLowerCase()));
  for (let i = 1; ; i++) {
    const name = `${base}${i}`;
    if (!taken.has(name.toLowerCase())) return name;
  }
}

/**
 * Nombres de columna únicos (criterio Excel): un encabezado vacío pasa a
 * «ColumnaN» (N = posición 1-based); un duplicado se sufija con «2», «3»…
 * Conserva el orden. Base de las referencias estructuradas (follow-up).
 */
export function uniqueHeaderNames(raw: readonly string[]): string[] {
  const out: string[] = [];
  const taken = new Set<string>();
  raw.forEach((r, i) => {
    let name = r.trim() === '' ? `Columna${i + 1}` : r.trim();
    if (taken.has(name.toLowerCase())) {
      let k = 2;
      while (taken.has(`${name}${k}`.toLowerCase())) k++;
      name = `${name}${k}`;
    }
    taken.add(name.toLowerCase());
    out.push(name);
  });
  return out;
}

/* ---------- Geometría ---------- */

/** Filas de DATOS (0-based, inclusivas) de una tabla: excluye cabecera y totales.
 *  Si no queda ninguna fila de datos, devuelve r1 > r2 (rango vacío). */
export function tableBodyRows(
  rect: RangeRect,
  headers: boolean,
  totals: boolean,
): { r1: number; r2: number } {
  return { r1: rect.r1 + (headers ? 1 : 0), r2: rect.r2 - (totals ? 1 : 0) };
}

/** Filas que ordena el «Ordenar» del desplegable del autofiltro. Si el autofiltro es
 *  el de una tabla (su cabecera), el cuerpo de la tabla, sin la fila de totales; si
 *  no, de la fila siguiente a la de los botones hasta la última con algo en sus
 *  columnas (`refs`, las claves de `cells`). r2 ≤ r1: nada que ordenar. */
export function autofilterBody(
  tables: CompiledTable[],
  af: { row: number; c1: number; c2: number },
  refs: Iterable<string>,
): { r1: number; r2: number } {
  const tb = tables.find(
    (ct) => ct.headers && ct.rect.r1 === af.row && ct.rect.c1 === af.c1 && ct.rect.c2 === af.c2,
  );
  if (tb) return { r1: tb.bodyR1, r2: tb.bodyR2 };
  let r2 = af.row;
  for (const ref of refs) {
    const rr = parseRangeRef(ref);
    if (rr && rr.c1 >= af.c1 && rr.c1 <= af.c2) r2 = Math.max(r2, rr.r1);
  }
  return { r1: af.row + 1, r2 };
}

export interface CompiledTable {
  rect: RangeRect;
  table: SheetTable;
  headers: boolean;
  totals: boolean;
  /** Fila superior de datos (0-based). */
  bodyR1: number;
  /** Fila inferior de datos (0-based); bodyR2 < bodyR1 si no hay datos. */
  bodyR2: number;
}

/** Compila una tabla: parsea su rango una vez y precalcula el cuerpo. null si el
 *  rango no parsea (docs a mano). El orden se conserva → índice = tabla original. */
export function compileTable(table: SheetTable): CompiledTable | null {
  const rect = parseRangeRef(table.range);
  if (!rect) return null;
  const headers = table.headers !== false;
  const totals = table.totalsRow === true;
  const body = tableBodyRows(rect, headers, totals);
  return { rect, table, headers, totals, bodyR1: body.r1, bodyR2: body.r2 };
}

/** Compila la lista de tablas (descarta las de rango inválido). */
export function compileTables(list: readonly SheetTable[]): CompiledTable[] {
  const out: CompiledTable[] = [];
  for (const t of list) {
    const ct = compileTable(t);
    if (ct) out.push(ct);
  }
  return out;
}

export type TableRole = 'header' | 'body' | 'total';

export interface TableCellInfo {
  ct: CompiledTable;
  role: TableRole;
  /** Solo role 'body': true = fila con banda (relleno alterno). */
  banded: boolean;
  /** Índice de columna 0-based dentro de la tabla. */
  colIndex: number;
}

/** Info de la celda (c,r) dentro de una tabla: la ÚLTIMA tabla que la cubre gana
 *  (mismo criterio que validationAt); null si ninguna la cubre. */
export function tableCellInfo(
  compiled: readonly CompiledTable[],
  c: number,
  r: number,
): TableCellInfo | null {
  let found: TableCellInfo | null = null;
  for (const ct of compiled) {
    const { rect } = ct;
    if (c < rect.c1 || c > rect.c2 || r < rect.r1 || r > rect.r2) continue;
    let role: TableRole;
    if (ct.headers && r === rect.r1) role = 'header';
    else if (ct.totals && r === rect.r2) role = 'total';
    else role = 'body';
    // Banda por paridad de la fila DENTRO del cuerpo (la primera fila sin relleno).
    const banded = role === 'body' && (r - ct.bodyR1) % 2 === 1;
    found = { ct, role, banded, colIndex: c - rect.c1 };
  }
  return found;
}

/**
 * Nuevo rectángulo de la tabla si escribir en (c,r) la extiende por su borde
 * derecho o inferior (auto-expansión de Excel); null si (c,r) no la amplía.
 * - Columna inmediatamente a la derecha, en la franja vertical de la tabla.
 * - Fila inmediatamente bajo los datos, en el ancho de la tabla, SIEMPRE que no
 *   haya fila de totales (con totales, la fila nueva debería insertarse sobre
 *   ellos: se deja como follow-up para no mover los totales por accidente).
 * `maxCol`/`maxRow` son los límites de la rejilla (0-based, inclusivos).
 */
export function expandRangeForEdit(
  ct: CompiledTable,
  c: number,
  r: number,
  maxCol: number,
  maxRow: number,
): RangeRect | null {
  const { rect } = ct;
  if (c === rect.c2 + 1 && c <= maxCol && r >= rect.r1 && r <= rect.r2) {
    return { c1: rect.c1, r1: rect.r1, c2: c, r2: rect.r2 };
  }
  if (!ct.totals && r === rect.r2 + 1 && r <= maxRow && c >= rect.c1 && c <= rect.c2) {
    return { c1: rect.c1, r1: rect.r1, c2: rect.c2, r2: r };
  }
  return null;
}

/* ---------- Saneado (persistencia o comando) ---------- */

/** SheetTable saneada desde datos no confiables; null si no vale. Campos nuevos
 *  opcionales y mínimos (Go trata doc:sheet como blob opaco). */
export function sanitizeTable(raw: unknown): SheetTable | null {
  if (!raw || typeof raw !== 'object') return null;
  const t = raw as Partial<SheetTable>;
  if (typeof t.name !== 'string' || t.name.trim() === '') return null;
  if (typeof t.range !== 'string' || parseRangeRef(t.range) === null) return null;
  const out: SheetTable = { name: t.name.trim(), range: t.range };
  if (typeof t.style === 'string' && STYLE_IDS.has(t.style)) out.style = t.style;
  if (t.headers === false) out.headers = false;
  if (t.totalsRow === true) out.totalsRow = true;
  if (t.totals && typeof t.totals === 'object') {
    const totals: Record<string, TableTotalFn> = {};
    for (const k in t.totals) {
      const col = Number(k);
      const v = (t.totals as Record<string, unknown>)[k];
      // 'none' no se persiste (es la ausencia de total): mantiene el JSON mínimo.
      if (Number.isInteger(col) && col >= 0 && isTotalFn(v) && v !== 'none') totals[k] = v;
    }
    if (Object.keys(totals).length > 0) out.totals = totals;
  }
  return out;
}

/* ---------- Referencias estructuradas Tabla[Columna] (paridad Excel/Sheets) ---------- */

/**
 * Tabla RESOLUBLE del libro: una tabla compilada + su hoja + los nombres únicos
 * de columna (derivados de la fila de cabecera). Es lo que consume el resolutor
 * de `Tabla[Columna]` que el editor inyecta en el motor.
 */
export interface ResolvableTable extends CompiledTable {
  /** Hoja donde vive la tabla (las referencias estructuradas son de libro). */
  sheet: string;
  /** Nombres únicos de columna; índice = desplazamiento desde rect.c1. */
  columns: string[];
}

/**
 * Lista de tablas resolubles del libro: compila cada tabla y deriva sus nombres
 * de columna de la fila de cabecera vía `header(sheet, c, r)` (texto crudo, que
 * el llamador lee del modelo). Sin cabecera → nombres implícitos «Columna1..N»
 * (criterio Excel, vía uniqueHeaderNames). O(tablas × columnas): son pocas.
 */
export const collectResolvableTables = (
  sheets: readonly { name: string; tables?: readonly SheetTable[] }[],
  header: (sheet: string, c: number, r: number) => string,
): ResolvableTable[] => {
  const out: ResolvableTable[] = [];
  for (const s of sheets) {
    for (const ct of compileTables(s.tables ?? [])) {
      const width = ct.rect.c2 - ct.rect.c1 + 1;
      const raw: string[] = [];
      for (let i = 0; i < width; i++)
        raw.push(ct.headers ? header(s.name, ct.rect.c1 + i, ct.rect.r1) : '');
      out.push({ ...ct, sheet: s.name, columns: uniqueHeaderNames(raw) });
    }
  }
  return out;
};

const eqCI = (a: string, b: string): boolean => a.toLowerCase() === b.toLowerCase();

/**
 * Resuelve una referencia estructurada `Tabla[Columna]` a un RefRange (o error),
 * anclada en la celda `here` que evalúa. Es el TableResolver que el editor inyecta
 * en el motor. Semántica de paridad Excel:
 *  - tabla implícita (`table === ''`, p. ej. `[@Col]`): la que contiene `here`; si
 *    `here` no cae en ninguna → undefined (el motor lo trata como #REF!).
 *  - tabla nombrada inexistente → #NAME?; columna inexistente → #REF!.
 *  - `@` (thisRow) fuera del cuerpo de datos → #VALUE!.
 *  - áreas #Cabeceras/#Totales sobre una tabla sin ellas → #REF!.
 * O(tablas + columnas): ambas son pocas.
 */
export const resolveStructuredRef = (
  tables: readonly ResolvableTable[],
  q: TableQuery,
  here: { sheet: string; col: number; row: number },
): RefRange | FormulaError | undefined => {
  let def: ResolvableTable | undefined;
  if (q.table === '') {
    def = tables.find(
      (t) =>
        t.sheet === here.sheet &&
        here.col >= t.rect.c1 &&
        here.col <= t.rect.c2 &&
        here.row >= t.rect.r1 &&
        here.row <= t.rect.r2,
    );
    if (!def) return undefined; // fuera de toda tabla → #REF!
  } else {
    def = tables.find((t) => eqCI(t.table.name, q.table));
    if (!def) return ERR.NAME; // tabla desconocida → #NAME?
  }

  // Columna concreta o todas (solo área, p. ej. Tabla[#Todo]).
  let c1 = def.rect.c1;
  let c2 = def.rect.c2;
  if (q.column !== undefined) {
    const idx = def.columns.findIndex((n) => eqCI(n, q.column!));
    if (idx < 0) return ERR.REF; // columna inexistente → #REF!
    c1 = c2 = def.rect.c1 + idx;
  }

  // Filas por área.
  let r1: number;
  let r2: number;
  switch (q.area) {
    case 'headers':
      if (!def.headers) return ERR.REF;
      r1 = r2 = def.rect.r1;
      break;
    case 'totals':
      if (!def.totals) return ERR.REF;
      r1 = r2 = def.rect.r2;
      break;
    case 'all':
      r1 = def.rect.r1;
      r2 = def.rect.r2;
      break;
    default: // 'data'
      r1 = def.bodyR1;
      r2 = def.bodyR2;
      if (r2 < r1) return ERR.REF; // tabla sin filas de datos
  }

  // `@` fila actual: intersecar con la fila de `here`; #VALUE! si cae fuera del cuerpo.
  if (q.thisRow) {
    if (here.sheet !== def.sheet || here.row < def.bodyR1 || here.row > def.bodyR2)
      return ERR.VALUE;
    r1 = r2 = here.row;
  }

  return { sheet: def.sheet, c1, r1, c2, r2 };
};
