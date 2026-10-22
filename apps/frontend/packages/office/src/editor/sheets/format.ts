/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Helpers PUROS de formato de la hoja de cálculo (sin DOM, sin estado de UI):
 * catálogo y aplicación de formatos de número (Intl es-ES) sobre el VALOR
 * crudo de la celda, evaluación de reglas de formato condicional, comparador
 * de ordenación y desplazamiento de referencias del autorelleno (shiftRefs).
 * El texto crudo de la celda nunca se toca: el formato es solo de render.
 * Self-test: packages/office/test/sheets-format.test.ts (bun test).
 */
import {
  FormulaError,
  colToNum,
  compareValues,
  formatNumber,
  numToCol,
  parseFormula,
} from '@sytue/formula';
import type { AstNode, CellRef, CellValue, TableArea } from '@sytue/formula';
import type {
  BorderStroke,
  CellBorder,
  CellStyle,
  ChartLegend,
  ChartSpec,
  CondIconSet,
  CondKind,
  CondOp,
  CondRule,
  DataValidation,
  DataValidationOp,
  DataValidationType,
  NumFmt,
  NumFmtId,
  RangoEditable,
  SheetProtection,
  Sparkline,
  WorkbookProtection,
} from '../types';
import { formatWithMask } from './numfmt-mask';
import { MAX_COLS } from './rejilla';
import type { DocsKey, DocsT } from '../../i18n';

/* ---------- HTML de celda (formato en línea: alineación → negrita → cursiva) ----------
 * Envoltorios canónicos que escribe/lee el lienzo; parseCell/renderCellHtml son
 * inversas, con lo que los conmutadores resultan deterministas. Puro (sin DOM):
 * el texto tecleado a mano queda como contenido opaco.
 */

export function escapeHtml(text: string): string {
  return text
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

/** Texto plano de una celda (para la barra de fórmulas y el formato numérico). */
export function stripHtml(html: string): string {
  return html
    .replace(/<[^>]*>/g, '')
    .replace(/&nbsp;/g, ' ')
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&quot;/g, '"')
    .replace(/&amp;/g, '&');
}

export interface CellParts {
  align: '' | 'center' | 'right';
  b: boolean;
  i: boolean;
  inner: string;
}

const ALIGN_RE = /^<div style="text-align:\s*(center|right);?">([\s\S]*)<\/div>$/i;
const B_RE = /^<b>([\s\S]*)<\/b>$/i;
const I_RE = /^<i>([\s\S]*)<\/i>$/i;

export function parseCell(html: string): CellParts {
  let inner = html.trim();
  let align: CellParts['align'] = '';
  let m = ALIGN_RE.exec(inner);
  if (m) {
    align = (m[1] ?? '').toLowerCase() === 'right' ? 'right' : 'center';
    inner = (m[2] ?? '').trim();
  }
  let b = false;
  m = B_RE.exec(inner);
  if (m) {
    b = true;
    inner = (m[1] ?? '').trim();
  }
  let i = false;
  m = I_RE.exec(inner);
  if (m) {
    i = true;
    inner = (m[1] ?? '').trim();
  }
  return { align, b, i, inner };
}

export function renderCellHtml(p: CellParts): string {
  let out = p.inner.trim();
  if (out === '' || out === '<br>') return '';
  if (p.i) out = `<i>${out}</i>`;
  if (p.b) out = `<b>${out}</b>`;
  if (p.align !== '') out = `<div style="text-align: ${p.align};">${out}</div>`;
  return out;
}

/** Texto crudo de una celda: HTML → texto plano (una fórmula «=…» o el literal). */
export function rawText(html: string): string {
  return stripHtml(html);
}

/** ¿El texto crudo de esta celda es una fórmula? (empieza por «=»). */
export function isFormula(html: string): boolean {
  return rawText(html).startsWith('=');
}

/**
 * Desplaza dc columnas y dr filas el HTML de una celda: si es fórmula, ajusta
 * sus referencias relativas (shiftRefs) conservando negrita/cursiva/alineación;
 * si es literal, lo devuelve tal cual. Lo comparten el autorelleno y el pegado.
 */
export function shiftCellHtml(html: string, dc: number, dr: number): string {
  if (html === '' || !isFormula(html)) return html;
  const p = parseCell(html);
  p.inner = escapeHtml(shiftRefs(rawText(html), dc, dr));
  return renderCellHtml(p);
}

/* ---------- Catálogos (los consume InicioSheets) ---------- */

export const NUMFMT_IDS: readonly NumFmtId[] = [
  'general',
  'number',
  'thousands',
  'currency',
  'percent',
  'date',
  'custom',
];

/** Presets del grupo Número (mutable a propósito: RibbonSelect pide options[]). */
export const NUMFMT_PRESETS: { value: NumFmtId; labelKey: DocsKey }[] = [
  { value: 'general', labelKey: 'numfmt.general' },
  { value: 'number', labelKey: 'numfmt.number' },
  { value: 'thousands', labelKey: 'sheet.mask.thousands' },
  { value: 'currency', labelKey: 'numfmt.currency' },
  { value: 'percent', labelKey: 'sheet.mask.percent' },
  { value: 'date', labelKey: 'numfmt.date' },
];

export const COND_OPS: { value: CondOp; labelKey: DocsKey }[] = [
  { value: 'gt', labelKey: 'cond.gt' },
  { value: 'ge', labelKey: 'cond.gte' },
  { value: 'lt', labelKey: 'cond.lt' },
  { value: 'le', labelKey: 'cond.lte' },
  { value: 'eq', labelKey: 'cond.eq' },
  { value: 'ne', labelKey: 'cond.ne' },
  { value: 'contains', labelKey: 'cond.contains' },
];

/** Decimales por defecto de cada preset (criterio Excel; general/date no aplican). */
const DEFAULT_DECIMALS: Record<NumFmtId, number> = {
  general: 0,
  number: 2,
  thousands: 2,
  currency: 2,
  percent: 0,
  date: 0,
  custom: 0, // los decimales los define la máscara, no este campo
};

export function defaultDecimals(id: NumFmtId): number {
  return DEFAULT_DECIMALS[id];
}

/* ---------- Formato de número ---------- */

/* Un Intl.NumberFormat por (id, decimales): construirlos es caro y el render
 * de la rejilla los pide por celda. */
const NF_CACHE = new Map<string, Intl.NumberFormat>();

function nf(key: string, opts: Intl.NumberFormatOptions): Intl.NumberFormat {
  let f = NF_CACHE.get(key);
  if (!f) {
    f = new Intl.NumberFormat('es-ES', opts);
    NF_CACHE.set(key, f);
  }
  return f;
}

/* Sistema de fechas 1900 del motor (packages/formula functions/datetime.ts):
 * serial 0 = 1899-12-31 UTC y día fantasma 60 = «1900-02-29» (bug histórico).
 * Ese helper es privado del paquete; se reproduce aquí la conversión (4 líneas). */
const DAY_MS = 86_400_000;
const DATE_EPOCH_MS = Date.UTC(1899, 11, 31);
const MAX_DATE_SERIAL = 2_958_465; // 9999-12-31
const DATE_FMT = new Intl.DateTimeFormat('es-ES', { timeZone: 'UTC' });

function formatDateSerial(n: number): string {
  const day = Math.trunc(n); // ponytail: solo la parte de día (sin hora en v1)
  if (day < 1 || day > MAX_DATE_SERIAL) return formatNumber(n); // fuera del sistema → General
  if (day === 60) return '29/2/1900'; // día fantasma: no existe en el calendario real
  const real = day > 60 ? day - 1 : day;
  return DATE_FMT.format(new Date(DATE_EPOCH_MS + real * DAY_MS));
}

/** Aplica un formato de número a un número (los demás tipos no pasan por aquí). */
export function applyNumFmt(fmt: NumFmt, n: number): string {
  if (!Number.isFinite(n)) return formatNumber(n);
  const d = fmt.decimals ?? DEFAULT_DECIMALS[fmt.id];
  switch (fmt.id) {
    case 'number':
      return nf(`n${d}`, {
        useGrouping: false,
        minimumFractionDigits: d,
        maximumFractionDigits: d,
      }).format(n);
    case 'thousands':
      // 'always': el min2 del CLDR es-ES dejaría 1234 sin separador de miles.
      return nf(`t${d}`, {
        useGrouping: 'always',
        minimumFractionDigits: d,
        maximumFractionDigits: d,
      }).format(n);
    case 'currency':
      return nf(`c${d}`, {
        style: 'currency',
        currency: 'EUR',
        useGrouping: 'always',
        minimumFractionDigits: d,
        maximumFractionDigits: d,
      }).format(n);
    case 'percent':
      return nf(`p${d}`, {
        style: 'percent',
        minimumFractionDigits: d,
        maximumFractionDigits: d,
      }).format(n);
    case 'date':
      return formatDateSerial(n);
    case 'custom':
      return fmt.mask ? formatWithMask(fmt.mask, n).text : formatNumber(n);
    default:
      return formatNumber(n); // general
  }
}

/** Texto a mostrar para un CellValue: números → numfmt (o General del motor);
 *  texto/booleano/error pasan de largo (mismo criterio que Engine.getDisplay). */
export function formatCellValue(v: CellValue, fmt?: NumFmt): string {
  if (v instanceof FormulaError) return v.code;
  if (typeof v === 'boolean') return v ? 'TRUE' : 'FALSE';
  if (typeof v === 'number') return fmt ? applyNumFmt(fmt, v) : formatNumber(v);
  return v;
}

/** Formato saneado desde datos no confiables (persistencia); null si no vale.
 *  `custom` exige una máscara no vacía (≤ 200 chars); sin ella no es un formato. */
export function sanitizeNumFmt(raw: unknown): NumFmt | null {
  if (!raw || typeof raw !== 'object') return null;
  const f = raw as Partial<NumFmt>;
  if (!f.id || !NUMFMT_IDS.includes(f.id)) return null;
  if (f.id === 'custom') {
    if (typeof f.mask !== 'string' || f.mask.trim() === '') return null;
    return { id: 'custom', mask: f.mask.slice(0, 200) };
  }
  const out: NumFmt = { id: f.id };
  if (typeof f.decimals === 'number' && Number.isFinite(f.decimals)) {
    out.decimals = Math.min(8, Math.max(0, Math.trunc(f.decimals)));
  }
  return out;
}

/* ---------- Formato condicional ---------- */

/** Número desde texto es-ES del usuario; null si no es numérico.
 *  Coma decimal si aparece; si no, punto decimal («1.234» se lee 1,234). */
export function parseEsNumber(text: string): number | null {
  const cleaned = text.replace(/[^0-9.,-]/g, '');
  if (cleaned === '' || cleaned === '-') return null;
  const normalized =
    cleaned.lastIndexOf(',') > cleaned.lastIndexOf('.')
      ? cleaned.replace(/\./g, '').replace(/,/g, '.')
      : cleaned.replace(/,/g, '');
  const n = Number(normalized);
  return Number.isFinite(n) ? n : null;
}

/** Igual que parseEsNumber pero ESTRICTO en la FORMA: rechaza texto con letras o
 *  símbolos (moneda, unidades…) en lugar de quedarse con los dígitos. Puerta de
 *  las validaciones/operandos numéricos, donde «A1», «$50» o «12 kg» NO deben
 *  colar como número (el extractor tolerante sí los aceptaría). null si no vale. */
export const parseEsNumberStrict = (text: string): number | null =>
  /[^\d\s.,+-]/.test(text) ? null : parseEsNumber(text);

/**
 * Fecha del usuario → nº de serie (sistema 1900 del motor, igual que
 * formatDateSerial): «dd/mm/aaaa» (con «/», «-» o «.»), «aaaa-mm-dd», o un
 * número suelto que ya ES un serial. null si no es una fecha válida. Años de 4
 * cifras (1900-9999); ponytail: sin heurística de 2 cifras (añadir si se pide).
 */
export function parseEsDate(text: string): number | null {
  const t = text.trim();
  if (t === '') return null;
  // Un número suelto ya es su nº de serie (permite operar con seriales crudos).
  if (/^[-+]?\d+(?:[.,]\d+)?$/.test(t)) return parseEsNumber(t);
  const m = /^(\d{1,4})[/.-](\d{1,2})[/.-](\d{1,4})$/.exec(t);
  if (!m) return null;
  const y = m[1]!.length === 4 ? +m[1]! : +m[3]!;
  const d = m[1]!.length === 4 ? +m[3]! : +m[1]!;
  const mo = +m[2]!;
  if (y < 1900 || y > 9999 || mo < 1 || mo > 12 || d < 1 || d > 31) return null;
  const utc = Date.UTC(y, mo - 1, d);
  const dt = new Date(utc);
  // Rechaza desbordes de calendario (p. ej. 31/02 → marzo).
  if (dt.getUTCFullYear() !== y || dt.getUTCMonth() !== mo - 1 || dt.getUTCDate() !== d)
    return null;
  const realDays = Math.round((utc - DATE_EPOCH_MS) / DAY_MS);
  if (realDays < 1) return null;
  return realDays >= 60 ? realDays + 1 : realDays; // salta el día fantasma 1900-02-29 (serial 60)
}

export interface RangeRect {
  c1: number;
  r1: number;
  c2: number;
  r2: number;
}

const RANGE_RE = /^\$?([A-Za-z]{1,3})\$?(\d{1,7})(?::\$?([A-Za-z]{1,3})\$?(\d{1,7}))?$/;

/** «B2» o «A1:C10» (admite $ y minúsculas) → rect 0-based normalizado; null si no parsea. */
export function parseRangeRef(text: string): RangeRect | null {
  const m = RANGE_RE.exec(text.trim());
  if (!m) return null;
  const c1 = colToNum(m[1]!);
  const r1 = Number(m[2]) - 1;
  const c2 = m[3] ? colToNum(m[3]) : c1;
  const r2 = m[4] ? Number(m[4]) - 1 : r1;
  return {
    c1: Math.min(c1, c2),
    r1: Math.min(r1, r2),
    c2: Math.max(c1, c2),
    r2: Math.max(r1, r2),
  };
}

/** ¿Cumple el VALOR de la celda la regla `cell`? Errores y celdas vacías nunca
 *  disparan; sin op/value (otros `kind`) tampoco. */
export function condMatches(rule: CondRule, v: CellValue): boolean {
  if (rule.op === undefined || rule.value === undefined) return false;
  if (v instanceof FormulaError || v === '') return false;
  const text =
    typeof v === 'boolean' ? (v ? 'TRUE' : 'FALSE') : typeof v === 'number' ? formatNumber(v) : v;
  if (rule.op === 'contains') {
    return text.toLowerCase().includes(rule.value.trim().toLowerCase());
  }
  const rv = parseEsNumber(rule.value);
  if (typeof v === 'number' && rv !== null) {
    switch (rule.op) {
      case 'gt':
        return v > rv;
      case 'ge':
        return v >= rv;
      case 'lt':
        return v < rv;
      case 'le':
        return v <= rv;
      case 'eq':
        return v === rv;
      case 'ne':
        return v !== rv;
      default:
        return false;
    }
  }
  // ponytail: gt/lt no ordenan texto (solo igualdad); ampliar si alguien lo pide.
  if (rule.op === 'eq' || rule.op === 'ne') {
    const equal = text.trim().toLowerCase() === rule.value.trim().toLowerCase();
    return rule.op === 'eq' ? equal : !equal;
  }
  return false;
}

export interface CompiledCond {
  rect: RangeRect;
  rule: CondRule;
}

/** Compila las reglas (parsea cada rango una sola vez); descarta las inválidas. */
export function compileCondRules(rules: readonly CondRule[]): CompiledCond[] {
  const out: CompiledCond[] = [];
  for (const rule of rules) {
    const rect = parseRangeRef(rule.range);
    if (rect) out.push({ rect, rule });
  }
  return out;
}

/** Fondo de la celda (c,r) según las reglas: gana la PRIMERA que cumple; null si ninguna. */
export function condFill(
  compiled: readonly CompiledCond[],
  c: number,
  r: number,
  v: CellValue,
): string | null {
  for (const k of compiled) {
    const { rect } = k;
    if (c >= rect.c1 && c <= rect.c2 && r >= rect.r1 && r <= rect.r2 && condMatches(k.rule, v)) {
      return k.rule.fill ?? null;
    }
  }
  return null;
}

/** Solo #RRGGBB (lo que ofrece la paleta): nada raro acaba en el style inline. */
function isHexColor(text: string): boolean {
  return /^#[0-9a-fA-F]{6}$/.test(text);
}

const COND_KINDS: readonly CondKind[] = [
  'cell',
  'formula',
  'top',
  'bottom',
  'dup',
  'unique',
  'dataBar',
  'colorScale',
  'iconSet',
];
const ICON_SETS: readonly CondIconSet[] = ['arrows', 'traffic', 'symbols'];

/**
 * Regla de formato condicional saneada desde datos no confiables (persistencia
 * o comando); null si no vale. `kind` ausente ⇒ 'cell' (retrocompatible con
 * {range, op, value, fill}). Cada tipo valida solo SUS campos, así el JSON
 * persistido queda mínimo.
 */
export function sanitizeCondRule(raw: unknown): CondRule | null {
  if (!raw || typeof raw !== 'object') return null;
  const r = raw as Partial<CondRule>;
  if (typeof r.range !== 'string' || parseRangeRef(r.range) === null) return null;
  const kind: CondKind = r.kind && COND_KINDS.includes(r.kind) ? r.kind : 'cell';
  const out: CondRule = { range: r.range };
  // Lo que pinta la regla: el relleno, el color de letra o los dos.
  const withFill = (): CondRule | null => {
    const fill = typeof r.fill === 'string' && isHexColor(r.fill) ? r.fill : undefined;
    const ink =
      typeof r.textColor === 'string' && isHexColor(r.textColor) ? r.textColor : undefined;
    if (!fill && !ink) return null;
    if (fill) out.fill = fill;
    if (ink) out.textColor = ink;
    return out;
  };
  switch (kind) {
    case 'cell': {
      if (typeof r.value !== 'string') return null;
      if (!r.op || !COND_OPS.some((o) => o.value === r.op)) return null;
      out.op = r.op;
      out.value = r.value;
      return withFill(); // kind omitido a propósito: 'cell' es el defecto
    }
    case 'formula': {
      if (typeof r.formula !== 'string' || r.formula.trim() === '') return null;
      out.kind = 'formula';
      out.formula = r.formula.trim();
      return withFill();
    }
    case 'top':
    case 'bottom': {
      const n = Math.trunc(Number(r.rank));
      if (!Number.isFinite(n) || n <= 0) return null;
      out.kind = kind;
      out.rank = r.percent ? Math.min(100, n) : Math.min(1000, n);
      if (r.percent === true) out.percent = true;
      return withFill();
    }
    case 'dup':
    case 'unique': {
      out.kind = kind;
      return withFill();
    }
    case 'dataBar': {
      if (typeof r.color !== 'string' || !isHexColor(r.color)) return null;
      out.kind = 'dataBar';
      out.color = r.color;
      return out;
    }
    case 'colorScale': {
      if (!Array.isArray(r.scale)) return null;
      const scale = r.scale.filter((c): c is string => typeof c === 'string' && isHexColor(c));
      if (scale.length !== 2 && scale.length !== 3) return null;
      out.kind = 'colorScale';
      out.scale = scale;
      return out;
    }
    case 'iconSet': {
      const icons = r.icons && ICON_SETS.includes(r.icons) ? r.icons : 'arrows';
      out.kind = 'iconSet';
      out.icons = icons;
      return out;
    }
    default:
      return null;
  }
}

/** Etiqueta legible de una regla para el gestor de reglas, en el idioma de `t`
 *  (la misma que compone Calcus en nativo, `etiquetaRegla`). */
export function condRuleLabel(r: CondRule, t: DocsT): string {
  const kind = r.kind ?? 'cell';
  switch (kind) {
    case 'formula':
      return t('sheet.condLabel.formula', { formula: r.formula ?? '' });
    case 'top':
      return t(r.percent ? 'sheet.condLabel.topPercent' : 'sheet.condLabel.top', {
        n: r.rank ?? 0,
      });
    case 'bottom':
      return t(r.percent ? 'sheet.condLabel.bottomPercent' : 'sheet.condLabel.bottom', {
        n: r.rank ?? 0,
      });
    case 'dup':
      return t('sheet.cond.dup');
    case 'unique':
      return t('sheet.cond.unique');
    case 'dataBar':
      return t('sheet.cond.dataBar');
    case 'colorScale':
      return t('sheet.condLabel.colorScale', { n: r.scale?.length ?? 2 });
    case 'iconSet':
      return t('sheet.condLabel.iconSet', { icons: r.icons ?? 'arrows' });
    default: {
      const op = opLabelOf(r.op);
      return `${op === '' ? '' : t(op)} ${r.value ?? ''}`.trim();
    }
  }
}

/** Clave del operador, no su texto: quien lo pinta tiene el traductor. */
function opLabelOf(op: CondOp | undefined): DocsKey | '' {
  return COND_OPS.find((o) => o.value === op)?.labelKey ?? '';
}

/* ---------- Formato condicional avanzado: decoración por celda ---------- */

/** Barra de datos de una celda: fracción 0..1 y su posición respecto al cero. */
export interface CellDataBar {
  /** Longitud de la barra 0..1 (relativa al mayor |valor| del rango). */
  frac: number;
  /** Posición del eje cero 0..1 (0 = todo positivo, 1 = todo negativo). */
  axis: number;
  /** El valor es negativo (barra a la izquierda del eje). */
  neg: boolean;
  color: string;
}

/** Decoración calculada para una celda por el formato condicional avanzado. */
export interface CellDecor {
  /** Relleno de fondo #RRGGBB (cell/formula/top/bottom/dup/unique/colorScale). */
  fill?: string;
  /** Color de letra #RRGGBB (cell/formula/top/bottom/dup/unique). */
  textColor?: string;
  /** Barra de datos proporcional (dataBar). */
  bar?: CellDataBar;
  /** Icono principal: `${set}:${level}` (iconSet), level 0=bajo 1=medio 2=alto. */
  icon?: string;
}

const A1 = (c: number, r: number): string => `${numToCol(c)}${r + 1}`;

/** «B2» (o «$b$2») → ref A1 normalizada de UNA celda; null si es rango o no parsea. */
export const singleCellRef = (ref: string): string | null => {
  const r = parseRangeRef(ref);
  return r && r.c1 === r.c2 && r.r1 === r.r2 ? A1(r.c1, r.r1) : null;
};

/** Clasifica el RHS de una restricción del Solver: CELDA (ref A1 normalizada) o
 *  NÚMERO (es-ES). La celda se prueba PRIMERO y sin ambigüedad: toda ref válida
 *  empieza por letra y ningún literal numérico casa `parseRangeRef`; probar el
 *  número antes lo rompería, porque `parseEsNumber('D3')` da 3 (la fila, no la
 *  celda). Un rango («A1:B2») no es RHS válido → null (no se coacciona a número).
 *  null = ni celda ni número → restricción inválida. */
export const classifyConstraintRhs = (
  rhs: string,
): { cell: string; num: null } | { cell: null; num: number } | null => {
  const cell = singleCellRef(rhs);
  if (cell) return { cell, num: null };
  if (parseRangeRef(rhs)) return null; // parece ref (rango) pero no una sola celda
  const num = parseEsNumber(rhs);
  return num === null ? null : { cell: null, num };
};

/** Número calculado de una celda para agregados (min/max/rank); null si no es número. */
function numAt(get: (c: number, r: number) => CellValue, c: number, r: number): number | null {
  const v = get(c, r);
  return typeof v === 'number' && Number.isFinite(v) ? v : null;
}

/** Todos los números del rectángulo (para min/max, rank, escalas). */
function rectNumbers(rect: RangeRect, get: (c: number, r: number) => CellValue): number[] {
  const out: number[] = [];
  for (let r = rect.r1; r <= rect.r2; r++) {
    for (let c = rect.c1; c <= rect.c2; c++) {
      const n = numAt(get, c, r);
      if (n !== null) out.push(n);
    }
  }
  return out;
}

/** #RRGGBB → [r,g,b]; asume ya validado (isHexColor). */
function hexToRgb(hex: string): [number, number, number] {
  return [
    parseInt(hex.slice(1, 3), 16),
    parseInt(hex.slice(3, 5), 16),
    parseInt(hex.slice(5, 7), 16),
  ];
}

function toHex2(n: number): string {
  return Math.round(clampNum(n, 0, 255))
    .toString(16)
    .padStart(2, '0');
}

function clampNum(n: number, lo: number, hi: number): number {
  return Math.min(hi, Math.max(lo, n));
}

/** Interpola dos colores #RRGGBB por t∈[0,1]. */
export function lerpHex(a: string, b: string, t: number): string {
  const ca = hexToRgb(a);
  const cb = hexToRgb(b);
  const u = clampNum(t, 0, 1);
  return `#${toHex2(ca[0] + (cb[0] - ca[0]) * u)}${toHex2(ca[1] + (cb[1] - ca[1]) * u)}${toHex2(
    ca[2] + (cb[2] - ca[2]) * u,
  )}`;
}

/** Color de una escala 2/3 colores para un valor normalizado t∈[0,1]. */
export function colorScaleAt(scale: readonly string[], t: number): string {
  const u = clampNum(t, 0, 1);
  if (scale.length >= 3) {
    return u <= 0.5
      ? lerpHex(scale[0]!, scale[1]!, u / 0.5)
      : lerpHex(scale[1]!, scale[2]!, (u - 0.5) / 0.5);
  }
  return lerpHex(scale[0]!, scale[1] ?? scale[0]!, u);
}

/**
 * Construye el mapa de decoración (ref A1 → CellDecor) de una hoja a partir de
 * sus reglas, en ORDEN DE PRIORIDAD (la primera que produce cada canal gana:
 * relleno, barra e icono son independientes). Puro: recibe `get` (valor
 * calculado de una celda) y `evalFormula` (evalúa una regla de fórmula en una
 * celda concreta; lo inyecta el lienzo con una celda de apoyo del motor).
 */
export function buildCondDecor(
  rules: readonly CondRule[],
  get: (c: number, r: number) => CellValue,
  evalFormula: (formula: string, dc: number, dr: number) => boolean,
): Map<string, CellDecor> {
  const map = new Map<string, CellDecor>();
  const decorAt = (ref: string): CellDecor => {
    let d = map.get(ref);
    if (!d) {
      d = {};
      map.set(ref, d);
    }
    return d;
  };
  const setFill = (c: number, r: number, fill: string): void => {
    const d = decorAt(A1(c, r));
    if (d.fill === undefined) d.fill = fill; // primera regla (mayor prioridad) gana
  };
  // Lo que pinta una regla que cumple: su relleno y su color de letra, cada canal el de la
  // primera regla que lo da.
  const paint = (c: number, r: number, rule: CondRule): void => {
    if (rule.fill) setFill(c, r, rule.fill);
    if (rule.textColor) {
      const d = decorAt(A1(c, r));
      if (d.textColor === undefined) d.textColor = rule.textColor;
    }
  };

  for (const rule of rules) {
    const rect = parseRangeRef(rule.range);
    if (!rect) continue;
    const kind = rule.kind ?? 'cell';

    const paints = Boolean(rule.fill || rule.textColor);
    if (kind === 'cell') {
      for (let r = rect.r1; r <= rect.r2; r++) {
        for (let c = rect.c1; c <= rect.c2; c++) {
          if (paints && condMatches(rule, get(c, r))) paint(c, r, rule);
        }
      }
    } else if (kind === 'formula' && rule.formula && paints) {
      for (let r = rect.r1; r <= rect.r2; r++) {
        for (let c = rect.c1; c <= rect.c2; c++) {
          if (evalFormula(rule.formula, c - rect.c1, r - rect.r1)) paint(c, r, rule);
        }
      }
    } else if ((kind === 'top' || kind === 'bottom') && paints && rule.rank) {
      const nums = rectNumbers(rect, get);
      if (nums.length === 0) continue;
      const sorted = [...nums].sort((a, b) => (kind === 'top' ? b - a : a - b));
      const count = rule.percent
        ? Math.max(1, Math.ceil((rule.rank / 100) * sorted.length))
        : Math.min(rule.rank, sorted.length);
      const threshold = sorted[count - 1]!; // incluye empates (criterio Excel)
      for (let r = rect.r1; r <= rect.r2; r++) {
        for (let c = rect.c1; c <= rect.c2; c++) {
          const n = numAt(get, c, r);
          if (n !== null && (kind === 'top' ? n >= threshold : n <= threshold)) {
            paint(c, r, rule);
          }
        }
      }
    } else if ((kind === 'dup' || kind === 'unique') && paints) {
      const counts = new Map<string, number>();
      for (let r = rect.r1; r <= rect.r2; r++) {
        for (let c = rect.c1; c <= rect.c2; c++) {
          const t = formatCellValue(get(c, r));
          if (t !== '') counts.set(t, (counts.get(t) ?? 0) + 1);
        }
      }
      for (let r = rect.r1; r <= rect.r2; r++) {
        for (let c = rect.c1; c <= rect.c2; c++) {
          const t = formatCellValue(get(c, r));
          if (t === '') continue;
          const n = counts.get(t) ?? 0;
          if (kind === 'dup' ? n > 1 : n === 1) paint(c, r, rule);
        }
      }
    } else if (kind === 'colorScale' && rule.scale) {
      const nums = rectNumbers(rect, get);
      if (nums.length === 0) continue;
      const min = Math.min(...nums);
      const max = Math.max(...nums);
      const span = max - min;
      for (let r = rect.r1; r <= rect.r2; r++) {
        for (let c = rect.c1; c <= rect.c2; c++) {
          const n = numAt(get, c, r);
          if (n === null) continue;
          setFill(c, r, colorScaleAt(rule.scale, span === 0 ? 0.5 : (n - min) / span));
        }
      }
    } else if (kind === 'dataBar' && rule.color) {
      const nums = rectNumbers(rect, get);
      if (nums.length === 0) continue;
      const lo = Math.min(0, ...nums);
      const hi = Math.max(0, ...nums);
      const denom = Math.max(Math.abs(lo), Math.abs(hi)) || 1;
      const axis = hi === lo ? 0 : (0 - lo) / (hi - lo); // posición del cero 0..1
      for (let r = rect.r1; r <= rect.r2; r++) {
        for (let c = rect.c1; c <= rect.c2; c++) {
          const n = numAt(get, c, r);
          if (n === null || n === 0) continue;
          const d = decorAt(A1(c, r));
          if (d.bar === undefined) {
            d.bar = { frac: Math.abs(n) / denom, axis, neg: n < 0, color: rule.color };
          }
        }
      }
    } else if (kind === 'iconSet') {
      const set = rule.icons ?? 'arrows';
      const nums = rectNumbers(rect, get);
      if (nums.length === 0) continue;
      const min = Math.min(...nums);
      const max = Math.max(...nums);
      const span = max - min;
      for (let r = rect.r1; r <= rect.r2; r++) {
        for (let c = rect.c1; c <= rect.c2; c++) {
          const n = numAt(get, c, r);
          if (n === null) continue;
          const t = span === 0 ? 1 : (n - min) / span;
          const level = t >= 2 / 3 ? 2 : t >= 1 / 3 ? 1 : 0;
          const d = decorAt(A1(c, r));
          if (d.icon === undefined) d.icon = `${set}:${level}`;
        }
      }
    }
  }
  return map;
}

/**
 * CSS `background-image` de una barra de datos (bordes DUROS, sin degradado
 * visible: el diseño prohíbe degradados; una barra sólida es un rectángulo con
 * cortes nítidos). El eje del cero va en `axis`; los negativos crecen a su
 * izquierda. Devuelve '' si la barra es nula.
 */
export function dataBarBackground(bar: CellDataBar): string {
  // La barra se pinta al 42 % del color de la regla: con el color lleno el dato —que va
  // encima— quedaba ilegible sobre el azul, y la barra pedía más atención que la cifra.
  const color = `color-mix(in srgb, ${bar.color} 42%, transparent)`;
  const axisPct = bar.axis * 100;
  if (bar.neg) {
    const start = Math.max(0, axisPct - bar.frac * bar.axis * 100);
    return `linear-gradient(90deg, transparent 0 ${pct(start)}%, ${color} ${pct(start)}% ${pct(
      axisPct,
    )}%, transparent ${pct(axisPct)}%)`;
  }
  const end = axisPct + bar.frac * (100 - axisPct);
  return `linear-gradient(90deg, transparent 0 ${pct(axisPct)}%, ${color} ${pct(axisPct)}% ${pct(
    end,
  )}%, transparent ${pct(end)}%)`;
}

const pct = (n: number): number => Math.round(clampNum(n, 0, 100) * 10) / 10;

/* ---------- Juegos de iconos (SVG clean-room, 3 niveles cada uno) ---------- */

/** Verde/ámbar/rojo de CONTENT_PALETTE por nivel (alto→bajo). */
const ICON_COLORS = ['#DC2626', '#D97706', '#16A34A'] as const; // 0 bajo, 1 medio, 2 alto

/** Trazos por juego y nivel: 3 flechas (arriba/lado/abajo), semáforo (círculo),
 *  símbolos (aspa/guion/check). Grid 12, currentColor via `fill`/`stroke`. */
function iconInner(set: CondIconSet, level: number): string {
  const col = ICON_COLORS[level] ?? ICON_COLORS[2];
  if (set === 'traffic') {
    return `<circle cx="6" cy="6" r="4.5" fill="${col}"/>`;
  }
  if (set === 'symbols') {
    if (level === 2)
      return `<path d="M2.5 6.3 5 8.8 9.5 3.5" fill="none" stroke="${col}" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"/>`;
    if (level === 1)
      return `<path d="M2.5 6h7" fill="none" stroke="${col}" stroke-width="1.8" stroke-linecap="round"/>`;
    return `<path d="M3 3l6 6M9 3l-6 6" fill="none" stroke="${col}" stroke-width="1.8" stroke-linecap="round"/>`;
  }
  // arrows: triángulo direccional
  if (level === 2) return `<path d="M6 2.5 10 8.5H2Z" fill="${col}"/>`;
  if (level === 1) return `<path d="M2.5 3.5 9.5 6 2.5 8.5Z" fill="${col}"/>`;
  return `<path d="M2 3.5h8L6 9.5Z" fill="${col}"/>`;
}

/** SVG string (12×12) de un icono de conjunto para incrustar en la celda. */
export function iconSetSvg(icon: string): string {
  const [set, lvlRaw] = icon.split(':');
  const level = clampNum(Math.trunc(Number(lvlRaw)), 0, 2);
  const s = set === 'traffic' || set === 'symbols' ? set : 'arrows';
  return `<svg viewBox="0 0 12 12" width="12" height="12" style="display:inline-block;vertical-align:-1px;margin-right:3px" aria-hidden="true">${iconInner(
    s,
    level,
  )}</svg>`;
}

/* ---------- Validación de datos (pestaña Datos): lista/número/fecha/longitud/fórmula ---------- */

/** Criterios ofrecidos por el diálogo (value = tipo, label = es-ES). */
export const DATA_VALIDATION_TYPES: { value: DataValidationType; labelKey: DocsKey }[] = [
  { value: 'list', labelKey: 'valid.kind.list' },
  { value: 'number', labelKey: 'valid.kind.number' },
  { value: 'date', labelKey: 'valid.kind.date' },
  { value: 'textLength', labelKey: 'valid.kind.textLength' },
  { value: 'formula', labelKey: 'cond.formula' },
];

/** Operadores de comparación (number/date/textLength) para el diálogo. */
export const DATA_VALIDATION_OPS: { value: DataValidationOp; labelKey: DocsKey }[] = [
  { value: 'between', labelKey: 'query.op.between' },
  { value: 'notBetween', labelKey: 'cond.notBetween' },
  { value: 'eq', labelKey: 'query.op.eq' },
  { value: 'ne', labelKey: 'query.op.ne' },
  { value: 'gt', labelKey: 'query.op.gt' },
  { value: 'lt', labelKey: 'query.op.lt' },
  { value: 'ge', labelKey: 'cond.gte' },
  { value: 'le', labelKey: 'cond.lte' },
];

const DV_TYPE_SET: readonly DataValidationType[] = [
  'list',
  'number',
  'date',
  'textLength',
  'formula',
];
const DV_OP_SET: readonly DataValidationOp[] = [
  'between',
  'notBetween',
  'eq',
  'ne',
  'gt',
  'lt',
  'ge',
  'le',
];

/** Interpreta un operando según el criterio: fecha → serial; el resto (number,
 *  textLength) → número es-ES ESTRICTO (rechaza «5abc», no lo lee como 5). null
 *  si no es legible. */
const parseOperand = (type: DataValidationType, raw: string): number | null =>
  type === 'date' ? parseEsDate(raw) : parseEsNumberStrict(raw);

/** Trocea el texto del usuario (una entrada por línea o por coma) en valores
 *  únicos, sin espacios sobrantes ni vacíos, conservando el orden de aparición. */
export function parseValidationValues(text: string): string[] {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const part of text.split(/[\n,]/)) {
    const v = part.trim();
    if (v !== '' && !seen.has(v)) {
      seen.add(v);
      out.push(v);
    }
  }
  return out;
}

/**
 * Validación saneada desde datos no confiables (persistencia o comando); null
 * si no vale. `type` ausente ⇒ 'list' (retrocompatible con {range, values,
 * strict}). Cada criterio valida solo SUS campos y el JSON persistido queda
 * mínimo (el tipo 'list' no se escribe). El mensaje de error y `strict` son
 * comunes a todos.
 */
export function sanitizeDataValidation(raw: unknown): DataValidation | null {
  if (!raw || typeof raw !== 'object') return null;
  const d = raw as Partial<DataValidation>;
  if (typeof d.range !== 'string' || parseRangeRef(d.range) === null) return null;
  const type: DataValidationType =
    typeof d.type === 'string' && DV_TYPE_SET.includes(d.type) ? d.type : 'list';
  const out: DataValidation = { range: d.range };
  if (type !== 'list') out.type = type; // 'list' queda implícito (retrocompat, JSON mínimo)
  if (typeof d.error === 'string' && d.error.trim() !== '')
    out.error = d.error.trim().slice(0, 200);
  if (d.strict === true) out.strict = true;

  if (type === 'list') {
    if (!Array.isArray(d.values)) return null;
    const values = parseValidationValues(
      d.values.filter((v): v is string => typeof v === 'string').join('\n'),
    );
    if (values.length === 0) return null;
    out.values = values;
    return out;
  }
  if (type === 'formula') {
    if (typeof d.formula !== 'string' || d.formula.trim() === '') return null;
    out.formula = d.formula.trim();
    return out;
  }
  // number | date | textLength: operador + operandos (a, y b si es entre/no-entre).
  if (typeof d.op !== 'string' || !DV_OP_SET.includes(d.op)) return null;
  if (typeof d.a !== 'string' || parseOperand(type, d.a) === null) return null;
  out.op = d.op;
  out.a = d.a.trim();
  if (d.op === 'between' || d.op === 'notBetween') {
    if (typeof d.b !== 'string' || parseOperand(type, d.b) === null) return null;
    out.b = d.b.trim();
  }
  if (type === 'number' && d.integer === true) out.integer = true;
  return out;
}

export interface CompiledValidation {
  rect: RangeRect;
  v: DataValidation;
}

/** Compila las validaciones (parsea cada rango una vez); descarta las inválidas. */
export function compileValidations(list: readonly DataValidation[]): CompiledValidation[] {
  const out: CompiledValidation[] = [];
  for (const v of list) {
    const rect = parseRangeRef(v.range);
    if (rect) out.push({ rect, v });
  }
  return out;
}

/** Validación que cubre (c,r): gana la ÚLTIMA definida (la más reciente); null si ninguna. */
export function validationAt(
  compiled: readonly CompiledValidation[],
  c: number,
  r: number,
): DataValidation | null {
  let found: DataValidation | null = null;
  for (const k of compiled) {
    const { rect } = k;
    if (c >= rect.c1 && c <= rect.c2 && r >= rect.r1 && r <= rect.r2) found = k.v;
  }
  return found;
}

/** ¿El texto es un valor de la LISTA permitida? La celda vacía SIEMPRE se admite
 *  (borrar). ponytail: cotejo exacto y sensible a mayúsculas; normalizar si se pide. */
export function isAllowedValue(v: DataValidation, text: string): boolean {
  const t = text.trim();
  return t === '' || (v.values ?? []).includes(t);
}

/** Compara x con los operandos de la validación según su `op`. Un operando
 *  ausente o ilegible NO restringe (no rechaza por configuración incompleta). */
function compareValidationOp(v: DataValidation, x: number): boolean {
  const type = v.type ?? 'list';
  const a = v.a === undefined ? null : parseOperand(type, v.a);
  if (a === null) return true;
  switch (v.op) {
    case 'gt':
      return x > a;
    case 'lt':
      return x < a;
    case 'ge':
      return x >= a;
    case 'le':
      return x <= a;
    case 'eq':
      return x === a;
    case 'ne':
      return x !== a;
    case 'between':
    case 'notBetween': {
      const b = v.b === undefined ? null : parseOperand(type, v.b);
      if (b === null) return true;
      const inside = x >= Math.min(a, b) && x <= Math.max(a, b);
      return v.op === 'between' ? inside : !inside;
    }
    default:
      return true; // op ausente → no restringe
  }
}

/**
 * ¿El texto candidato satisface la validación? La celda VACÍA siempre se admite
 * (permite borrar). `list` reutiliza isAllowedValue; `number`/`date`/`textLength`
 * comparan el número/serial/longitud del texto; `formula` DELEGA en `evalFormula`
 * (lo inyecta el lienzo evaluando la expresión con la celda candidata puesta en
 * un motor). Sin evaluador, una validación `formula` se da por válida (no se
 * puede comprobar). Puro salvo por el evaluador inyectado. El candidato se coteja
 * como TEXTO literal; ponytail: una fórmula tecleada no se evalúa aquí (coherente
 * con la validación de lista), evaluar su resultado numérico es un follow-up.
 */
export function validateValue(
  v: DataValidation,
  text: string,
  evalFormula?: (formula: string) => boolean,
): boolean {
  const t = text.trim();
  if (t === '') return true;
  switch (v.type ?? 'list') {
    case 'formula':
      return v.formula === undefined || evalFormula === undefined ? true : evalFormula(v.formula);
    case 'number': {
      // Estricto en la forma: «A1»/«abc123»/«12 kg» NO son números (Excel los
      // rechaza), a diferencia del extractor tolerante que se quedaría los dígitos.
      const n = parseEsNumberStrict(t);
      if (n === null || (v.integer && !Number.isInteger(n))) return false;
      return compareValidationOp(v, n);
    }
    case 'date': {
      const n = parseEsDate(t);
      return n === null ? false : compareValidationOp(v, n);
    }
    case 'textLength':
      return compareValidationOp(v, t.length);
    default:
      return isAllowedValue(v, t);
  }
}

/** Mensaje de rechazo de una validación (usa `error` si el usuario lo puso). */
export function validationMessage(v: DataValidation, ref: string): string {
  if (v.error && v.error.trim() !== '') return v.error;
  if ((v.type ?? 'list') === 'list') {
    return `La celda ${ref} solo admite: ${(v.values ?? []).join(', ')}.`;
  }
  return `El valor de ${ref} no cumple la validación de datos.`;
}

const OP_SYM: Record<Exclude<DataValidationOp, 'between' | 'notBetween'>, string> = {
  eq: '=',
  ne: '≠',
  gt: '>',
  lt: '<',
  ge: '≥',
  le: '≤',
};

/** Etiqueta breve de una validación para el gestor del diálogo, en el idioma de `t`
 *  (la misma que compone Calcus en nativo, `etiquetaValidacion`). */
export function validationLabel(v: DataValidation, t: DocsT): string {
  const type = v.type ?? 'list';
  if (type === 'list') return (v.values ?? []).join(', ');
  if (type === 'formula') return t('sheet.condLabel.formula', { formula: v.formula ?? '' });
  const noun = t(
    type === 'number'
      ? 'valid.kind.number'
      : type === 'date'
        ? 'valid.kind.date'
        : 'valid.kind.textLength',
  );
  const range = v.op === 'between' || v.op === 'notBetween';
  const op =
    v.op === 'between'
      ? t('query.op.between')
      : v.op === 'notBetween'
        ? t('cond.notBetween')
        : v.op
          ? OP_SYM[v.op]
          : '';
  const operands = range ? `${v.a ?? ''} ${t('common.and')} ${v.b ?? ''}` : (v.a ?? '');
  return `${noun} ${op} ${operands}`.trim();
}

/* ---------- Gráficos (fase Visual): saneado del ChartSpec persistido ---------- */

const CHART_TYPES: readonly ChartSpec['type'][] = [
  'bar',
  'hbar',
  'line',
  'pie',
  'scatter',
  'area',
  'donut',
  'radar',
  'combo',
];

/** Índices de serie saneados: enteros ≥0 y por debajo del límite de columnas de la
 *  hoja, únicos y en orden; `undefined` si no queda ninguno (para no ensuciar el JSON). */
function cleanSeriesIdx(raw: unknown): number[] | undefined {
  if (!Array.isArray(raw)) return undefined;
  const out = [
    ...new Set(raw.filter((n): n is number => Number.isInteger(n) && n >= 0 && n < MAX_COLS)),
  ].sort((a, b) => a - b);
  return out.length > 0 ? out : undefined;
}

/** Tope de x/y de la tarjeta de gráfico sobre la rejilla (px de maqueta @100%;
 *  la rejilla 26×100 mide 2536×2424, así la tarjeta nunca queda fuera). */
export const CHART_POS_MAX = 2200;

/** Topes del tamaño del área de un gráfico (px de maqueta). */
export const CHART_SIZE_MIN = 120;
export const CHART_SIZE_MAX = 1400;

const CHART_LEGENDS: readonly ChartLegend[] = ['bottom', 'top', 'right', 'none'];

/** Título de eje saneado: texto recortado a 80 caracteres; undefined si queda vacío. */
function cleanAxisTitle(v: unknown): string | undefined {
  if (typeof v !== 'string') return undefined;
  const t = v.trim().slice(0, 80);
  return t === '' ? undefined : t;
}

/** Colores por serie: #RRGGBB o '' (el de su puesto), como mucho 26, sin los '' del
 *  final; undefined si no queda ninguno. */
function cleanSeriesColors(v: unknown): string[] | undefined {
  if (!Array.isArray(v)) return undefined;
  const out = v.slice(0, 26).map((c) => (typeof c === 'string' && isHexColor(c) ? c : ''));
  while (out.length > 0 && out[out.length - 1] === '') out.pop();
  return out.length > 0 ? out : undefined;
}

function clampChartSize(n: number): number {
  return Math.min(CHART_SIZE_MAX, Math.max(CHART_SIZE_MIN, Math.round(n)));
}

function clampChartPos(n: number): number {
  return Math.min(CHART_POS_MAX, Math.max(0, Math.round(n)));
}

/** ChartSpec saneado en profundidad desde datos no confiables (persistencia o
 *  comando); null si no vale. Un id ausente se regenera (docs editados a mano). */
export function sanitizeChartSpec(raw: unknown): ChartSpec | null {
  if (!raw || typeof raw !== 'object') return null;
  const c = raw as Partial<ChartSpec>;
  if (!c.type || !CHART_TYPES.includes(c.type)) return null;
  if (typeof c.range !== 'string' || parseRangeRef(c.range) === null) return null;
  const out: ChartSpec = {
    id: typeof c.id === 'string' && c.id !== '' ? c.id : crypto.randomUUID(),
    type: c.type,
    range: c.range,
  };
  if (typeof c.title === 'string' && c.title !== '') out.title = c.title;
  if (typeof c.x === 'number' && Number.isFinite(c.x)) out.x = clampChartPos(c.x);
  if (typeof c.y === 'number' && Number.isFinite(c.y)) out.y = clampChartPos(c.y);
  const lineSeries = cleanSeriesIdx(c.lineSeries);
  if (lineSeries) out.lineSeries = lineSeries;
  const secondaryAxis = cleanSeriesIdx(c.secondaryAxis);
  if (secondaryAxis) out.secondaryAxis = secondaryAxis;
  if (typeof c.w === 'number' && Number.isFinite(c.w)) out.w = clampChartSize(c.w);
  if (typeof c.h === 'number' && Number.isFinite(c.h)) out.h = clampChartSize(c.h);
  if (c.byRow === true) out.byRow = true;
  if (c.stacked === true) out.stacked = true;
  if (typeof c.legend === 'string' && CHART_LEGENDS.includes(c.legend) && c.legend !== 'bottom')
    out.legend = c.legend;
  if (c.labels === true) out.labels = true;
  if (c.grid === false) out.grid = false;
  const xTitle = cleanAxisTitle(c.xTitle);
  if (xTitle) out.xTitle = xTitle;
  const yTitle = cleanAxisTitle(c.yTitle);
  if (yTitle) out.yTitle = yTitle;
  const colors = cleanSeriesColors(c.colors);
  if (colors) out.colors = colors;
  return out;
}

/* ---------- Ordenación (comando Ordenar de la pestaña Datos) ---------- */

/**
 * Comparador de filas para Ordenar: ascendente con `dir = 1`, descendente con
 * `-1`. Rango de tipos del criterio Excel: número < texto < booleano < error,
 * y las celdas VACÍAS siempre al final, en ambas direcciones.
 */
export function sortCellCompare(a: CellValue, b: CellValue, dir: 1 | -1): number {
  const ea = a === '';
  const eb = b === '';
  if (ea || eb) return ea === eb ? 0 : ea ? 1 : -1;
  const ra =
    a instanceof FormulaError ? 3 : typeof a === 'number' ? 0 : typeof a === 'string' ? 1 : 2;
  const rb =
    b instanceof FormulaError ? 3 : typeof b === 'number' ? 0 : typeof b === 'string' ? 1 : 2;
  if (ra !== rb) return dir * (ra - rb);
  if (a instanceof FormulaError) return 0; // errores entre sí: orden estable
  const c = compareValues(a, b); // aquí ninguno es error → devuelve número
  return typeof c === 'number' ? dir * c : 0;
}

/* ---------- Autorelleno: desplazar referencias (AST, nunca regex) ---------- */

/* Precedencias del serializador, espejo del parser de @sytue/formula:
 * comparación < concatenación < suma < producto < potencia < unario < «%».
 * (El «-» unario liga MÁS que «^», semántica ECMA-376: -2^2 = 4.) */
const BIN_PREC: Record<string, number> = {
  '=': 1,
  '<>': 1,
  '<': 1,
  '>': 1,
  '<=': 1,
  '>=': 1,
  '&': 2,
  '+': 3,
  '-': 3,
  '*': 4,
  '/': 4,
  '^': 5,
};
const PREC_UNARY = 6;
const PREC_PERCENT = 7;
const PREC_ATOM = 8;

function nodePrec(n: AstNode): number {
  if (n.kind === 'binary') return BIN_PREC[n.op] ?? PREC_ATOM;
  if (n.kind === 'unary') return PREC_UNARY;
  if (n.kind === 'percent') return PREC_PERCENT;
  return PREC_ATOM;
}

/** «Hoja1!» o «'Mi hoja'!» (comillas si el nombre no es una palabra simple). */
export function sheetPrefix(sheet: string | undefined): string {
  if (sheet === undefined) return '';
  return /^[A-Za-z_][A-Za-z0-9_.$]*$/.test(sheet) ? `${sheet}!` : `'${sheet.replace(/'/g, "''")}'!`;
}

function refText(ref: CellRef, withSheet: boolean): string {
  const s = withSheet ? sheetPrefix(ref.sheet) : '';
  return `${s}${ref.colAbs ? '$' : ''}${numToCol(ref.col)}${ref.rowAbs ? '$' : ''}${ref.row + 1}`;
}

/** Extremo de un rango de EJE ENTERO: solo la columna ($A) o la fila ($1). */
function axisPart(ref: CellRef, axis: 'col' | 'row', withSheet: boolean): string {
  const s = withSheet ? sheetPrefix(ref.sheet) : '';
  return axis === 'col'
    ? `${s}${ref.colAbs ? '$' : ''}${numToCol(ref.col)}`
    : `${s}${ref.rowAbs ? '$' : ''}${ref.row + 1}`;
}

function wrap(n: AstNode, min: number): string {
  const s = serialize(n);
  return nodePrec(n) < min ? `(${s})` : s;
}

/** AST → texto que el parser re-lee al MISMO AST: paréntesis solo donde la
 *  precedencia los exige; separador de argumentos «;» (convención es-ES,
 *  el parser admite ambos). */
function serialize(n: AstNode): string {
  switch (n.kind) {
    case 'num':
      return formatNumber(n.value);
    case 'str':
      return `"${n.value.replace(/"/g, '""')}"`;
    case 'bool':
      return n.value ? 'TRUE' : 'FALSE';
    case 'array':
      // Constante matricial: «,» entre columnas y «;» entre filas, siempre (no hay variante regional).
      return `{${n.rows
        .map((r) =>
          r
            .map((v) =>
              v instanceof FormulaError
                ? v.code
                : typeof v === 'string'
                  ? v === ''
                    ? ''
                    : `"${v.replace(/"/g, '""')}"`
                  : typeof v === 'boolean'
                    ? v
                      ? 'TRUE'
                      : 'FALSE'
                    : formatNumber(v),
            )
            .join(','),
        )
        .join(';')}}`;
    case 'ref':
      return refText(n.ref, true);
    case 'range':
      // El extremo derecho nunca lleva hoja («Hoja1!A1:B2»): así lo exige el parser.
      if (n.whole === 'col') return `${axisPart(n.a, 'col', true)}:${axisPart(n.b, 'col', false)}`;
      if (n.whole === 'row') return `${axisPart(n.a, 'row', true)}:${axisPart(n.b, 'row', false)}`;
      return `${refText(n.a, true)}:${refText(n.b, false)}`;
    case 'name':
      return `${sheetPrefix(n.sheet)}${n.name}`;
    case 'unary':
      return n.op + wrap(n.operand, PREC_UNARY);
    case 'percent':
      return `${wrap(n.operand, PREC_PERCENT)}%`;
    case 'binary': {
      const p = BIN_PREC[n.op] ?? PREC_ATOM;
      // «^» asocia por la derecha y el resto por la izquierda: el hijo del lado
      // no asociativo con IGUAL precedencia necesita paréntesis para conservar la forma.
      return `${wrap(n.left, n.op === '^' ? p + 1 : p)}${n.op}${wrap(n.right, n.op === '^' ? p : p + 1)}`;
    }
    case 'call':
      return `${n.name}(${n.args.map(serialize).join(';')})`;
    case 'structured':
      return serializeStructured(n);
    case 'empty':
      return ''; // un argumento omitido se escribe como nada: «F(a;;c)»
  }
}

/** Especificador de área ES para re-serializar una referencia estructurada. */
const AREA_SPEC: Record<Exclude<TableArea, 'data'>, string> = {
  all: '#Todo',
  headers: '#Cabeceras',
  totals: '#Totales',
};

/**
 * Re-serializa una referencia estructurada `Tabla[…]` a una forma que el parser
 * re-lee al MISMO AST (para el desplazamiento de copiar/pegar; las estructuradas
 * son por-NOMBRE, así que no se desplazan, solo se reescriben). Empareja las
 * formas que produce parseBracketSpec: `Tabla[Col]`, `Tabla[@Col]`, `Tabla[#Área]`
 * y la anidada `Tabla[[#Área],[Col]]` cuando hay área y columna.
 * ponytail: escapa solo un «@»/«#» inicial del nombre de columna (caso real); un
 * «[»/«]»/«,» dentro del nombre no se escapa (nombres así no ocurren en la práctica).
 */
function serializeStructured(n: Extract<AstNode, { kind: 'structured' }>): string {
  const col = n.column !== undefined && /^[@#]/.test(n.column) ? `'${n.column}` : n.column;
  let inner: string;
  if (n.thisRow) inner = col === undefined ? '#Esta fila' : `@${col}`;
  else if (n.area === 'data') inner = col === undefined ? '#Datos' : col;
  else if (col === undefined) inner = AREA_SPEC[n.area];
  else inner = `[${AREA_SPEC[n.area]}],[${col}]`;
  return `${n.table}[${inner}]`;
}

/**
 * Desplaza las referencias RELATIVAS de una fórmula dc columnas y dr filas
 * (las ancladas con «$» no se mueven) vía parseFormula + serializador — nunca
 * regex: los «A1» dentro de cadenas y los nombres de hoja quedan intactos.
 * Si la fórmula no parsea se devuelve tal cual (se copia sin ajustar). Si
 * alguna referencia se sale de la rejilla por arriba o por la izquierda,
 * devuelve el literal «#REF!» (sin «=»): la celda lo muestra como texto,
 * aproximación del =#REF! de Excel (el parser no admite «#»).
 */
export function shiftRefs(formula: string, dc: number, dr: number): string {
  let ast: AstNode;
  try {
    ast = parseFormula(formula);
  } catch {
    return formula;
  }
  let ok = true;
  const shift = (ref: CellRef): void => {
    if (!ref.colAbs) ref.col += dc;
    if (!ref.rowAbs) ref.row += dr;
    if (ref.col < 0 || ref.row < 0) ok = false;
  };
  // Rango de eje entero: desplaza SOLO el eje que manda (una columna entera no se
  // mueve al arrastrar en vertical; y su fila fantasma no debe dar #REF!).
  const shiftAxis = (ref: CellRef, axis: 'col' | 'row'): void => {
    if (axis === 'col') {
      if (!ref.colAbs) ref.col += dc;
      if (ref.col < 0) ok = false;
    } else {
      if (!ref.rowAbs) ref.row += dr;
      if (ref.row < 0) ok = false;
    }
  };
  const walk = (n: AstNode): void => {
    switch (n.kind) {
      case 'ref':
        shift(n.ref);
        break;
      case 'range':
        if (n.whole) {
          shiftAxis(n.a, n.whole);
          shiftAxis(n.b, n.whole);
        } else {
          shift(n.a);
          shift(n.b);
        }
        break;
      case 'unary':
      case 'percent':
        walk(n.operand);
        break;
      case 'binary':
        walk(n.left);
        walk(n.right);
        break;
      case 'call':
        for (const a of n.args) walk(a);
        break;
      default:
        break;
    }
  };
  walk(ast);
  return ok ? `=${serialize(ast)}` : '#REF!';
}

/* ---------- Formato visual de celda (InicioSheets: fuente, color, bordes…) ---------- */

/** Familias ofrecidas (mismas 6 que la pestaña Inicio del folio, §6.3) → stack CSS. */
const FONT_STACKS: Record<string, string> = {
  Inter: 'Inter, system-ui, sans-serif',
  Arial: 'Arial, Helvetica, sans-serif',
  Georgia: 'Georgia, serif',
  'Times New Roman': '"Times New Roman", Times, serif',
  'Courier New': '"Courier New", monospace',
  'JetBrains Mono': '"JetBrains Mono", monospace',
};

/** Catálogo de familias para el RibbonSelect (value = label = nombre). */
export const FONT_FAMILIES: { value: string; label: string }[] = Object.keys(FONT_STACKS).map(
  (f) => ({ value: f, label: f }),
);

/** Tallas en pt para el RibbonSelect (estilo de celda, sin el tope de execCommand). */
export const FONT_SIZES: { value: string; label: string }[] = [
  8, 9, 10, 11, 12, 14, 16, 18, 20, 24, 28, 36, 48, 72,
].map((n) => ({ value: String(n), label: String(n) }));

/** Color de los bordes manuales (token de marca, sin hex de UI nuevo). */
const BORDER_CSS = '1px solid var(--sytue-border-strong)';

/** El CSS de cada trazo de borde: grosor y estilo. */
const STROKE_CSS: Record<BorderStroke, string> = {
  medium: '2px solid',
  thick: '3px solid',
  dashed: '1px dashed',
  dotted: '1px dotted',
  double: '3px double',
};
const STROKES = Object.keys(STROKE_CSS) as BorderStroke[];

/** El CSS de un lado del borde: su trazo y su color, o la línea fina del color de serie. */
function borderSideCss(stroke: BorderStroke | undefined, color: string | undefined): string {
  if (!stroke && !color) return BORDER_CSS;
  return `${stroke ? STROKE_CSS[stroke] : '1px solid'} ${color ?? 'var(--sytue-border-strong)'}`;
}

const VALIGN_JUSTIFY: Record<NonNullable<CellStyle['valign']>, string> = {
  top: 'flex-start',
  middle: 'center',
  bottom: 'flex-end',
};

function isHex(text: unknown): text is string {
  return typeof text === 'string' && /^#[0-9a-fA-F]{6}$/.test(text);
}

/** Borde saneado; null si no queda ningún lado. El color y el trazo, solo de los lados que se
 *  ven, y solo #RRGGBB y los trazos conocidos. */
function sanitizeBorder(raw: unknown): CellBorder | null {
  if (!raw || typeof raw !== 'object') return null;
  const b = raw as Record<string, unknown>;
  const out: CellBorder = {};
  for (const side of ['t', 'r', 'b', 'l'] as const) {
    if (b[side] !== true) continue;
    out[side] = true;
    const color = b[`${side}c`];
    if (isHex(color)) out[`${side}c`] = color;
    const stroke = b[`${side}s`];
    if (typeof stroke === 'string' && (STROKES as string[]).includes(stroke)) {
      out[`${side}s`] = stroke as BorderStroke;
    }
  }
  return out.t || out.r || out.b || out.l ? out : null;
}

/**
 * Estilo de celda saneado desde datos no confiables (persistencia o comando);
 * null si no queda ninguna propiedad válida (no se guardan objetos vacíos).
 * Solo se aceptan familias del catálogo y colores #RRGGBB: nada arbitrario
 * llega al style inline.
 */
export function sanitizeCellStyle(raw: unknown): CellStyle | null {
  if (!raw || typeof raw !== 'object') return null;
  const s = raw as Record<string, unknown>;
  const out: CellStyle = {};
  if (typeof s['font'] === 'string' && s['font'] in FONT_STACKS) out.font = s['font'];
  if (typeof s['size'] === 'number' && Number.isFinite(s['size'])) {
    out.size = Math.min(96, Math.max(6, Math.trunc(s['size'])));
  }
  if (isHex(s['color'])) out.color = s['color'];
  if (isHex(s['fill'])) out.fill = s['fill'];
  if (s['u'] === true) out.u = true;
  if (s['s'] === true) out.s = true;
  if (s['valign'] === 'top' || s['valign'] === 'middle' || s['valign'] === 'bottom') {
    out.valign = s['valign'];
  }
  if (s['wrap'] === true) out.wrap = true;
  const border = sanitizeBorder(s['border']);
  if (border) out.border = border;
  // Protección de celda: solo el desbloqueo explícito y el ocultado se guardan
  // (los valores por defecto —bloqueada, fórmula visible— quedan implícitos).
  if (s['locked'] === false) out.locked = false;
  if (s['hidden'] === true) out.hidden = true;
  return Object.keys(out).length > 0 ? out : null;
}

/** Aplica un parche de estilo sobre uno previo; devuelve null si queda vacío
 *  (para borrarlo del mapa). `undefined` en el parche = «no tocar»; `null` = quitar. */
export function mergeCellStyle(
  base: CellStyle | undefined,
  patch: Partial<Record<keyof CellStyle, unknown>>,
): CellStyle | null {
  return sanitizeCellStyle({ ...base, ...patch });
}

/**
 * CellStyle → propiedades CSS para el div de la celda (camelCase, Preact). NO
 * incluye el relleno (`background`): lo gestiona el lienzo junto a la selección
 * y el formato condicional. Bordes con box-sizing:border-box → sin desplazar la
 * rejilla. `wrap` mantiene la altura fija (overflow oculto): el auto-alto de
 * fila queda pendiente (la geometría de congelar/autorelleno asume 24px).
 */
export function cellStyleCss(style: CellStyle): Record<string, string> {
  const css: Record<string, string> = {};
  if (style.font && style.font in FONT_STACKS) css['fontFamily'] = FONT_STACKS[style.font]!;
  if (typeof style.size === 'number') css['fontSize'] = `${style.size}pt`;
  if (style.color) css['color'] = style.color;
  const deco = [style.u ? 'underline' : '', style.s ? 'line-through' : ''].filter(Boolean);
  if (deco.length) css['textDecorationLine'] = deco.join(' ');
  if (style.wrap) {
    css['whiteSpace'] = 'normal';
    css['wordBreak'] = 'break-word';
  }
  if (style.valign) {
    css['display'] = 'flex';
    css['flexDirection'] = 'column';
    css['justifyContent'] = VALIGN_JUSTIFY[style.valign];
  }
  const b = style.border;
  if (b?.t) css['borderTop'] = borderSideCss(b.ts, b.tc);
  if (b?.r) css['borderRight'] = borderSideCss(b.rs, b.rc);
  if (b?.b) css['borderBottom'] = borderSideCss(b.bs, b.bc);
  if (b?.l) css['borderLeft'] = borderSideCss(b.ls, b.lc);
  return css;
}

/* ---------- Protección de hoja y celda (paridad Excel) ---------- */

/**
 * Estilo EFECTIVO de una celda para quien edita: igual que el guardado, pero
 * DESBLOQUEADO si la celda cae en un rango editable concedido a esta persona (`sub`) o a
 * uno de sus grupos. Es lo que ve el editor; quien decide es el servidor, que aplica la
 * misma regla (services/docs/internal/domain/proteccion_rangos.go).
 * ponytail: solo rangos «A1:C5»; las columnas o filas enteras que el servidor acepta
 * aquí no desbloquean (la celda se ve bloqueada aunque el servidor la deje escribir).
 */
export function estiloParaMi(
  protect: SheetProtection | undefined,
  ref: string,
  style: CellStyle | undefined,
  yo: { sub: string; groups?: readonly string[] },
): CellStyle | undefined {
  if (!protect?.ranges?.length || yo.sub === '') return style;
  const m = /^([A-Za-z]{1,3})(\d{1,7})$/.exec(ref);
  if (!m) return style;
  const c = colToNum(m[1]!);
  const r = Number(m[2]) - 1;
  const mio = protect.ranges.some((g) => {
    if (!(g.users?.includes(yo.sub) || g.groups?.some((x) => yo.groups?.includes(x)))) return false;
    const rc = parseRangeRef(g.ref);
    return rc !== null && c >= rc.c1 && c <= rc.c2 && r >= rc.r1 && r <= rc.r2;
  });
  return mio ? { ...style, locked: false } : style;
}

/**
 * ¿Se puede EDITAR el contenido de una celda? Sin la hoja protegida, siempre;
 * con la hoja protegida, solo las celdas explícitamente DESBLOQUEADAS
 * (locked === false). El bloqueo por defecto es TRUE (Excel): una celda sin
 * estilo, o con estilo pero sin `locked`, está bloqueada. Predicado PURO.
 */
export function canEditCell(protectedSheet: boolean, style: CellStyle | undefined): boolean {
  if (!protectedSheet) return true;
  return style?.locked === false;
}

/** ¿Debe OCULTARSE la fórmula de la celda? (hoja protegida + celda `hidden`). */
export function isFormulaHidden(protectedSheet: boolean, style: CellStyle | undefined): boolean {
  return protectedSheet && style?.hidden === true;
}

/**
 * ¿Se puede EDITAR el contenido de la celda por barra de fórmulas o en-celda?
 * Como `canEditCell`, pero además veta las celdas DESBLOQUEADAS cuya FÓRMULA esté
 * OCULTA (hoja protegida + `hidden` + el crudo es una fórmula): editarlas la
 * revelaría (el editor muestra el «=…» crudo) o la borraría al confirmar la barra
 * —que se vacía para ocultar la fórmula—. Excel oculta la fórmula; aquí, además,
 * protegemos su edición. `raw` = texto crudo de la celda. Predicado PURO.
 */
export function canEditCellContent(
  protectedSheet: boolean,
  style: CellStyle | undefined,
  raw: string,
): boolean {
  if (!canEditCell(protectedSheet, style)) return false;
  return !(isFormulaHidden(protectedSheet, style) && isFormula(raw));
}

/**
 * Protección de hoja saneada desde datos no confiables (persistencia o comando);
 * null solo si `raw` no es un objeto. La PRESENCIA del objeto (aunque quede {})
 * significa hoja protegida; se persisten solo las opciones que difieren de su
 * valor por defecto (seleccionar = true, formatear = false) para un JSON mínimo.
 */
export function sanitizeProtection(raw: unknown): SheetProtection | null {
  if (!raw || typeof raw !== 'object') return null;
  const p = raw as Partial<SheetProtection>;
  const out: SheetProtection = {};
  if (p.hasPassword === true) out.hasPassword = true; // lo fija el servidor; la contraseña nunca entra
  const rangos = rangosSaneados(p.ranges);
  if (rangos.length > 0) out.ranges = rangos;
  if (p.selectCells === false) out.selectCells = false;
  if (p.formatCells === true) out.formatCells = true;
  // Permisos granulares: solo se persisten los CONCEDIDOS (ausente = false).
  for (const k of SHEET_PERMISSIONS) {
    if (p[k] === true) out[k] = true;
  }
  return out;
}

/** Rangos editables saneados: solo los que tienen una referencia que se lee y alguien concedido. */
function rangosSaneados(raw: unknown): RangoEditable[] {
  if (!Array.isArray(raw)) return [];
  const lista = (v: unknown): string[] =>
    Array.isArray(v) ? v.filter((x): x is string => typeof x === 'string' && x !== '') : [];
  const out: RangoEditable[] = [];
  for (const g of raw as Partial<RangoEditable>[]) {
    if (typeof g?.ref !== 'string' || parseRangeRef(g.ref) === null) continue;
    const users = lista(g.users);
    const groups = lista(g.groups);
    if (users.length + groups.length === 0) continue;
    out.push({
      ref: g.ref.trim().toUpperCase().replace(/\$/g, ''),
      ...(users.length ? { users } : {}),
      ...(groups.length ? { groups } : {}),
    });
  }
  return out;
}

/**
 * Permisos granulares del cuadro «Proteger hoja» de Excel: con la hoja protegida
 * la acción está vetada salvo que el permiso se haya concedido explícitamente.
 * `formatCells` va aparte (es el permiso histórico y ya tenía su propio camino).
 */
export const SHEET_PERMISSIONS = [
  'insertRows',
  'deleteRows',
  'insertCols',
  'deleteCols',
  'sort',
  'autoFilter',
  'objects',
  'scenarios',
] as const;

export type SheetPermission = (typeof SHEET_PERMISSIONS)[number];

/**
 * ¿La hoja permite `perm`? Sin protección, todo vale. Con la hoja protegida solo
 * si el permiso está CONCEDIDO (ausente = false, criterio Excel). Predicado PURO.
 */
export function isPermissionGranted(
  protect: SheetProtection | undefined,
  perm: SheetPermission,
): boolean {
  if (!protect) return true;
  return protect[perm] === true;
}

/**
 * ¿Se puede MODIFICAR LA ESTRUCTURA del libro (añadir/borrar/renombrar/duplicar/
 * reordenar hojas)? Solo con el libro SIN proteger: la PRESENCIA de la protección
 * la bloquea. Cambiar de hoja o editar celdas no son cambios de estructura y no
 * pasan por aquí. `protect` = SheetModel.protect. Predicado PURO.
 */
export function canEditStructure(protect: WorkbookProtection | undefined): boolean {
  return protect == null;
}

/**
 * Protección de LIBRO saneada desde datos no confiables (persistencia o comando);
 * null solo si `raw` no es un objeto. La PRESENCIA (aunque quede {}) = libro
 * protegido; solo se persiste `hasPassword` (lo fija el servidor), igual criterio
 * que `sanitizeProtection` de la hoja.
 */
export function sanitizeWorkbookProtection(raw: unknown): WorkbookProtection | null {
  if (!raw || typeof raw !== 'object') return null;
  const p = raw as Partial<WorkbookProtection>;
  const out: WorkbookProtection = {};
  if (p.hasPassword === true) out.hasPassword = true;
  return out;
}

/* ---------- Portapapeles: TSV (interoperable con Excel/Sheets) ---------- */

/** Celda → texto TSV de una línea (tabs/saltos → espacio; las celdas son de una línea). */
function tsvCell(v: string): string {
  return v.replace(/[\t\r\n]+/g, ' ');
}

/** Rejilla de valores → TSV (filas por «\n», columnas por «\t»). */
export function serializeTsv(rows: readonly (readonly string[])[]): string {
  return rows.map((row) => row.map(tsvCell).join('\t')).join('\n');
}

/** TSV → rejilla de valores. Ignora un único salto final (los portapapeles lo añaden). */
export function parseTsv(text: string): string[][] {
  let t = text.replace(/\r\n?/g, '\n');
  if (t.endsWith('\n')) t = t.slice(0, -1);
  if (t === '') return [['']];
  return t.split('\n').map((line) => line.split('\t'));
}

/* ---------- Herramientas de datos (pestaña Datos) ---------- */

/**
 * Índices (0-based, dentro del bloque) de las filas a CONSERVAR al quitar
 * duplicados: se queda la PRIMERA aparición de cada clave. `keys[i]` es la
 * huella de la fila i (p. ej. sus celdas unidas por «U+0000»).
 */
export function dedupeRowIndices(keys: readonly string[]): number[] {
  const seen = new Set<string>();
  const keep: number[] = [];
  keys.forEach((k, i) => {
    if (!seen.has(k)) {
      seen.add(k);
      keep.push(i);
    }
  });
  return keep;
}

/** Resumen de una selección para la barra de hojas (Excel/Numbers). */
export interface SelectionStats {
  /** Celdas NO vacías del rango (criterio «Recuento» de Excel: también el texto). */
  count: number;
  /** Suma de las numéricas. */
  sum: number;
  /** Promedio de las numéricas, a seis decimales. */
  avg: number;
}

/**
 * Recuento, suma y promedio de los valores de una selección; null si no hay ni
 * un número (sin nada que resumir, la barra no pinta el bloque). Los errores y
 * los booleanos cuentan como celda ocupada pero no entran en la aritmética,
 * como en Excel. Quien llama pasa SOLO los valores visibles: lo que el filtro o
 * el esquema esconden no se resume.
 */
export function selectionStats(values: Iterable<CellValue>): SelectionStats | null {
  let count = 0;
  let nums = 0;
  let sum = 0;
  for (const v of values) {
    if (v === '') continue;
    count++;
    if (typeof v === 'number' && Number.isFinite(v)) {
      nums++;
      sum += v;
    }
  }
  if (nums === 0) return null;
  // Promedio a seis decimales: los quince dígitos del cálculo no caben en la barra
  // y aquí es un vistazo, no un dato. Los números enormes se dejan tal cual, que
  // multiplicarlos por 1e6 los sacaría del entero seguro y falsearía el redondeo.
  const avg = sum / nums;
  return { count, sum, avg: Math.abs(avg) < 1e9 ? Math.round(avg * 1e6) / 1e6 : avg };
}

/* ---------- Sparklines (mini-gráficos de celda) ---------- */

export const SPARK_TYPES: readonly Sparkline['type'][] = ['line', 'column', 'winloss'];
/** Color por defecto de una sparkline: azul de serie de CONTENT_PALETTE. */
const SPARK_BLUE = '#2563EB';
const SPARK_RED = '#DC2626'; // pérdidas (win/loss y columnas negativas)
/** Lienzo de la sparkline en px de maqueta (celda 96×24 menos aire). */
const SPARK_W = 84;
const SPARK_H = 18;

export interface SparkGeom {
  /** line: puntos [x,y] en el lienzo SPARK_W×SPARK_H. */
  points: [number, number][];
  /** column/winloss: barras. */
  bars: { x: number; y: number; w: number; h: number; neg: boolean }[];
}

/** Geometría pura de una sparkline (sin DOM). Números no finitos cuentan 0. */
export function sparklineGeom(values: readonly number[], type: Sparkline['type']): SparkGeom {
  const vals = values.map((v) => (Number.isFinite(v) ? v : 0));
  const n = vals.length;
  const pad = 2;
  const w = SPARK_W;
  const h = SPARK_H;
  const innerH = h - pad * 2;
  const out: SparkGeom = { points: [], bars: [] };
  if (n === 0) return out;
  if (type === 'line') {
    const min = Math.min(...vals);
    const max = Math.max(...vals);
    const span = max - min || 1;
    const step = n > 1 ? w / (n - 1) : 0;
    out.points = vals.map((v, i) => [
      round2(n > 1 ? i * step : w / 2),
      round2(pad + (1 - (v - min) / span) * innerH),
    ]);
    return out;
  }
  const slot = w / n;
  const bw = Math.max(1, slot * 0.7);
  if (type === 'winloss') {
    const mid = h / 2;
    const bh = innerH / 2;
    out.bars = vals.map((v, i) => ({
      x: round2(i * slot + (slot - bw) / 2),
      y: round2(v >= 0 ? mid - bh : mid),
      w: round2(bw),
      h: round2(v === 0 ? 0 : bh),
      neg: v < 0,
    }));
    return out;
  }
  // column: alturas proporcionales sobre la línea del cero.
  const lo = Math.min(0, ...vals);
  const hi = Math.max(0, ...vals);
  const span = hi - lo || 1;
  const zeroY = pad + (hi / span) * innerH;
  out.bars = vals.map((v, i) => {
    const vy = pad + ((hi - v) / span) * innerH;
    return {
      x: round2(i * slot + (slot - bw) / 2),
      y: round2(Math.min(vy, zeroY)),
      w: round2(bw),
      h: round2(Math.abs(vy - zeroY)),
      neg: v < 0,
    };
  });
  return out;
}

const round2 = (n: number): number => Math.round(n * 100) / 100;

/** SVG string de una sparkline (para incrustar como innerHTML de la celda). */
export function sparklineSvg(
  values: readonly number[],
  type: Sparkline['type'],
  color?: string,
): string {
  const g = sparklineGeom(values, type);
  const main = color && isHexColor(color) ? color : SPARK_BLUE;
  const head = `<svg viewBox="0 0 ${SPARK_W} ${SPARK_H}" preserveAspectRatio="none" width="100%" height="${SPARK_H}" style="display:block" aria-hidden="true">`;
  let body = '';
  if (type === 'line') {
    if (g.points.length === 0) return `${head}</svg>`;
    const d = g.points.map((p, i) => `${i === 0 ? 'M' : 'L'}${p[0]} ${p[1]}`).join('');
    body = `<path d="${d}" fill="none" stroke="${main}" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round"/>`;
  } else {
    body = g.bars
      .filter((b) => b.h > 0)
      .map(
        (b) =>
          `<rect x="${b.x}" y="${b.y}" width="${b.w}" height="${b.h}" fill="${
            type === 'winloss' ? (b.neg ? SPARK_RED : main) : main
          }"/>`,
      )
      .join('');
  }
  return `${head}${body}</svg>`;
}

/** Sparkline saneada desde datos no confiables (persistencia o comando); null si no vale. */
export function sanitizeSparkline(raw: unknown): Sparkline | null {
  if (!raw || typeof raw !== 'object') return null;
  const s = raw as Partial<Sparkline>;
  if (typeof s.cell !== 'string' || parseRangeRef(s.cell) === null) return null;
  if (typeof s.range !== 'string' || parseRangeRef(s.range) === null) return null;
  if (!s.type || !SPARK_TYPES.includes(s.type)) return null;
  const out: Sparkline = { cell: s.cell, range: s.range, type: s.type };
  if (typeof s.color === 'string' && isHexColor(s.color)) out.color = s.color;
  return out;
}
