/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Formateador PURO de MÁSCARAS de formato personalizado estilo Excel (sin DOM,
 * sin estado). `formatWithMask(mascara, n)` aplica una máscara a un número y
 * devuelve el texto (más un color opcional de [Rojo]/[Color] para el render).
 *
 * Soporta (MVP de los casos comunes):
 *   · Secciones «positivo;negativo;cero;texto» separadas por «;» (la de texto no
 *     se usa aquí: solo formateamos NÚMEROS, igual que el resto de presets).
 *   · Marcadores de dígito 0 (cero forzado) · # (opcional) · ? (espacio de
 *     alineación, aproximado como 0 en v1).
 *   · Separador de miles «,» y escalado por millares (comas finales), decimal «.»,
 *     porcentaje «%», literales entre comillas «"…"» y escape «\x», símbolo de
 *     moneda suelto (€, $, …) o en «[$€-xxx]».
 *   · Color al frente: [Rojo]/[Red], [Azul]/[Blue]… (subconjunto de Excel).
 *   · Máscaras de FECHA/HORA: yyyy/yy, mmmm/mmm/mm/m (mes o minuto según contexto),
 *     dddd/ddd/dd/d, hh/h, ss/s, AM/PM (12 h).
 *   · TIEMPO TRANSCURRIDO [h]/[hh], [m]/[mm], [s]/[ss]: la unidad entre corchetes
 *     acumula (26:30 no da la vuelta) y los tokens siguientes son su resto.
 *   · CONDICIONES de sección [>=100]/[<0]/[<>5]: manda la primera que casa; la
 *     sección sin condición hace de reserva.
 *   · Literales ENTRE placeholders de dígito entero: «00000-0000» → 12345-6789.
 *
 * CONVENCIÓN: la máscara se escribe con tokens de Excel («.» decimal, «,» miles);
 * la SALIDA se localiza a es-ES («.» miles, «,» decimal), como el resto de presets.
 *
 * ponytail: exótico documentado como follow-up (no MVP): notación científica E+
 * (magnitudes ≥1e21, donde toFixed emite exponencial y deforma el agrupado de
 * miles), relleno «*x», colores indexados [ColorN], sección de TEXTO (los strings
 * no pasan por numfmt) y el ancho real de «_»/«?» (se aproximan a un espacio).
 * Self-test: packages/office/test/sheets-numfmt-mask.test.ts (bun test).
 */
import { formatNumber } from '@sytue/formula';

/** Resultado de aplicar una máscara: texto + color opcional (de [Color]). */
export interface MaskResult {
  text: string;
  /** #RRGGBB del marcador [Color] de la sección activa (para el render). */
  color?: string;
}

/* ---------- Constantes de localización (es-ES, deterministas para los tests) ---------- */

const DECIMAL_SEP = ','; // salida es-ES
const GROUP_SEP = '.';

const MONTHS = [
  'enero',
  'febrero',
  'marzo',
  'abril',
  'mayo',
  'junio',
  'julio',
  'agosto',
  'septiembre',
  'octubre',
  'noviembre',
  'diciembre',
] as const;
const MONTHS_SHORT = [
  'ene',
  'feb',
  'mar',
  'abr',
  'may',
  'jun',
  'jul',
  'ago',
  'sep',
  'oct',
  'nov',
  'dic',
] as const;
const DAYS = ['domingo', 'lunes', 'martes', 'miércoles', 'jueves', 'viernes', 'sábado'] as const;
const DAYS_SHORT = ['dom', 'lun', 'mar', 'mié', 'jue', 'vie', 'sáb'] as const;

/** Colores nombrados que admite el marcador [Color] → #RRGGBB (subconjunto Excel). */
const MASK_COLORS: Record<string, string> = {
  black: '#000000',
  negro: '#000000',
  blue: '#2563EB',
  azul: '#2563EB',
  cyan: '#0891B2',
  green: '#16A34A',
  verde: '#16A34A',
  magenta: '#DB2777',
  red: '#DC2626',
  rojo: '#DC2626',
  white: '#FFFFFF',
  blanco: '#FFFFFF',
  yellow: '#CA8A04',
  amarillo: '#CA8A04',
};

/* ---------- Sistema de fecha 1900 del motor (espejo de sheets/format.ts) ---------- */

const DAY_MS = 86_400_000;
const DATE_EPOCH_MS = Date.UTC(1899, 11, 31);
const MAX_DATE_SERIAL = 2_958_465; // 9999-12-31

interface DateParts {
  y: number;
  mo: number; // 1..12
  d: number; // 1..31
  wd: number; // 0=domingo … 6=sábado
  hh: number; // 0..23
  mi: number; // 0..59
  ss: number; // 0..59
}

/** Serial (sistema 1900, con día fantasma 60) → partes UTC; null si fuera de rango. */
function serialToParts(n: number): DateParts | null {
  let dayInt = Math.floor(n);
  const frac = n - dayInt;
  let totalSec = Math.round(frac * 86_400);
  if (totalSec >= 86_400) {
    totalSec -= 86_400;
    dayInt += 1;
  }
  if (dayInt < 0 || dayInt > MAX_DATE_SERIAL) return null;
  const hh = Math.floor(totalSec / 3600);
  const mi = Math.floor((totalSec % 3600) / 60);
  const ss = totalSec % 60;
  if (dayInt === 60) return { y: 1900, mo: 2, d: 29, wd: 3, hh, mi, ss }; // fantasma 1900-02-29 (miércoles)
  const realDay = dayInt > 60 ? dayInt - 1 : dayInt;
  const date = new Date(DATE_EPOCH_MS + realDay * DAY_MS);
  return {
    y: date.getUTCFullYear(),
    mo: date.getUTCMonth() + 1,
    d: date.getUTCDate(),
    wd: date.getUTCDay(),
    hh,
    mi,
    ss,
  };
}

const pad2 = (n: number): string => String(n).padStart(2, '0');

/* ---------- Troceado en secciones (respeta comillas, escape y corchetes) ---------- */

/** Divide la máscara en secciones por «;» de primer nivel (ignora «;» dentro de
 *  "…", tras «\» o dentro de [ … ]). Máximo relevante: 4 (pos/neg/cero/texto). */
export function splitSections(mask: string): string[] {
  const out: string[] = [];
  let cur = '';
  for (let i = 0; i < mask.length; i++) {
    const ch = mask[i]!;
    if (ch === '"') {
      cur += ch;
      i++;
      while (i < mask.length && mask[i] !== '"') cur += mask[i++];
      if (i < mask.length) cur += mask[i]; // comilla de cierre
      continue;
    }
    if (ch === '\\') {
      cur += ch;
      if (i + 1 < mask.length) cur += mask[++i];
      continue;
    }
    if (ch === '[') {
      cur += ch;
      i++;
      while (i < mask.length && mask[i] !== ']') cur += mask[i++];
      if (i < mask.length) cur += mask[i];
      continue;
    }
    if (ch === ';') {
      out.push(cur);
      cur = '';
      continue;
    }
    cur += ch;
  }
  out.push(cur);
  return out;
}

/** Corchete de tiempo TRANSCURRIDO: [h]/[hh], [m]/[mm], [s]/[ss] (acumulan, no dan
 *  la vuelta). Se conserva en el cuerpo: lo consume formatElapsedSection. */
const ELAPSED_BRACKET = /^(h{1,2}|m{1,2}|s{1,2})$/i;

/** ¿El cuerpo (ya sin literales) lleva un corchete de tiempo transcurrido? */
const ELAPSED_IN_BODY = /\[(h{1,2}|m{1,2}|s{1,2})\]/i;

/** Condición de sección estilo Excel: «[>=100]», «[<0]», «[<>5]». */
const CONDITION_RE = /^\s*(<=|>=|<>|<|>|=)\s*(-?\d+(?:\.\d+)?)\s*$/;

/** Clasifica el contenido de un [ … ]: color conocido, símbolo de moneda
 *  «[$€-xxx]» (literal), tiempo transcurrido (se conserva) o ignorado. */
function classifyBracket(inner: string): { color?: string; literal?: string; keep?: string } {
  const key = inner.trim().toLowerCase();
  const col = MASK_COLORS[key];
  if (col) return { color: col };
  if (inner.startsWith('$')) return { literal: inner.slice(1).split('-')[0] ?? '' }; // [$€-C0A] → «€»
  if (ELAPSED_BRACKET.test(key)) return { keep: `[${key}]` };
  return {}; // [>=100] (ya consumida como condición), [ColorN]… → se descarta
}

/** Extrae el color de la sección y sustituye cada [ … ] por su literal (o nada). */
function stripBrackets(section: string): { color?: string; body: string } {
  let color: string | undefined;
  const body = section.replace(/\[([^\]]*)\]/g, (_m, inner: string) => {
    const c = classifyBracket(inner);
    if (c.color !== undefined && color === undefined) color = c.color;
    if (c.keep !== undefined) return c.keep;
    return c.literal !== undefined && c.literal !== '' ? `"${c.literal}"` : '';
  });
  return color !== undefined ? { color, body } : { body };
}

/** Condición de una sección, si la lleva: predicado sobre el número (o null). */
function readCondition(section: string): ((n: number) => boolean) | null {
  const m = /^\s*\[([^\]]*)\]/.exec(section);
  if (!m) return null;
  const c = CONDITION_RE.exec(m[1] ?? '');
  if (!c) return null;
  const op = c[1]!;
  const ref = Number(c[2]);
  if (!Number.isFinite(ref)) return null;
  return (n: number): boolean => {
    switch (op) {
      case '<':
        return n < ref;
      case '>':
        return n > ref;
      case '<=':
        return n <= ref;
      case '>=':
        return n >= ref;
      case '<>':
        return n !== ref;
      default:
        return n === ref;
    }
  };
}

/** Quita literales entre comillas y escapes para inspeccionar los tokens crudos. */
function stripLiterals(body: string): string {
  return body.replace(/"[^"]*"/g, '').replace(/\\./g, '');
}

/** ¿La sección es de FECHA/HORA? Presencia de y/d/h/s, o «m» sin marcadores de
 *  dígito (mes/minuto suelto). Sin ellos, es numérica. */
function isDateTime(body: string): boolean {
  const s = stripLiterals(body).toLowerCase();
  if (/[ydhs]/.test(s)) return true;
  return /m/.test(s) && !/[0#?]/.test(s);
}

/* ---------- Formateo de la sección NUMÉRICA ---------- */

type NumTok =
  | { t: 'lit'; s: string }
  | { t: 'd'; c: '0' | '#' | '?' }
  | { t: 'dot' }
  | { t: 'comma' }
  | { t: 'pct' };

/** Trocea una sección numérica en tokens (dígitos, punto, coma, %, literales). */
function scanNumberTokens(body: string): NumTok[] {
  const toks: NumTok[] = [];
  for (let i = 0; i < body.length; i++) {
    const ch = body[i]!;
    if (ch === '"') {
      let s = '';
      i++;
      while (i < body.length && body[i] !== '"') s += body[i++];
      toks.push({ t: 'lit', s });
      continue;
    }
    if (ch === '\\') {
      if (i + 1 < body.length) toks.push({ t: 'lit', s: body[++i]! });
      continue;
    }
    if (ch === '_') {
      if (i + 1 < body.length) i++; // «_x» = ancho de x → un espacio (aprox.)
      toks.push({ t: 'lit', s: ' ' });
      continue;
    }
    if (ch === '*') {
      if (i + 1 < body.length) i++; // «*x» = relleno → omitido (MVP)
      continue;
    }
    if (ch === '0' || ch === '#' || ch === '?') {
      toks.push({ t: 'd', c: ch });
      continue;
    }
    if (ch === '.') {
      toks.push({ t: 'dot' });
      continue;
    }
    if (ch === ',') {
      toks.push({ t: 'comma' });
      continue;
    }
    if (ch === '%') {
      toks.push({ t: 'pct' });
      continue;
    }
    toks.push({ t: 'lit', s: ch });
  }
  return toks;
}

/** Inserta el separador de miles cada 3 dígitos desde la derecha. */
function groupThousands(digits: string): string {
  let out = '';
  let cnt = 0;
  for (let i = digits.length - 1; i >= 0; i--) {
    out = digits[i]! + out;
    if (++cnt % 3 === 0 && i > 0) out = GROUP_SEP + out;
  }
  return out;
}

/** Formatea una MAGNITUD (>= 0) con una sección numérica de máscara. */
function formatNumberSection(body: string, magnitude: number): string {
  const toks = scanNumberTokens(body);

  // Placeholders de la parte entera y de la decimal (separadas por el 1.er punto).
  const intPh: ('0' | '#' | '?')[] = [];
  const fracPh: ('0' | '#' | '?')[] = [];
  let seenDot = false;
  for (const t of toks) {
    if (t.t === 'dot' && !seenDot) {
      seenDot = true;
      continue;
    }
    if (t.t === 'd') (seenDot ? fracPh : intPh).push(t.c);
  }
  const pctCount = toks.filter((t) => t.t === 'pct').length;

  // Comas de la parte entera: entre placeholders → agrupa; finales → escalan /1000.
  let grouping = false;
  let trailingCommas = 0;
  let intSoFar = 0;
  seenDot = false;
  for (const t of toks) {
    if (t.t === 'dot' && !seenDot) {
      seenDot = true;
      continue;
    }
    if (seenDot) continue;
    if (t.t === 'd') intSoFar++;
    else if (t.t === 'comma' && intSoFar > 0) {
      if (intSoFar < intPh.length) grouping = true;
      else trailingCommas++;
    }
  }

  const scaled = (magnitude * Math.pow(100, pctCount)) / Math.pow(1000, trailingCommas);
  // toFixed solo admite 0..100 dígitos; una máscara con >100 «#»/«0» decimales
  // lanzaría RangeError (alcanzable desde el input de máscara). Excel topa ~30.
  const fracCount = Math.min(fracPh.length, 100);
  const fixed = scaled.toFixed(fracCount);
  const dotAt = fixed.indexOf('.');
  const intDigits = dotAt === -1 ? fixed : fixed.slice(0, dotAt);
  const fracDigits = dotAt === -1 ? '' : fixed.slice(dotAt + 1);

  // Entero: rellena con ceros hasta los «0» exigidos; oculta el 0 si no hay «0».
  const minInt = intPh.filter((c) => c === '0').length;
  let intDisplay = intDigits;
  if (intDisplay === '0' && minInt === 0) intDisplay = '';
  else intDisplay = intDisplay.padStart(minInt, '0');
  if (grouping && intDisplay !== '') intDisplay = groupThousands(intDisplay);

  // Decimal: recorta los ceros finales que caen en «#» (0/? se conservan).
  let keep = fracCount;
  while (keep > 0 && fracPh[keep - 1] === '#' && fracDigits[keep - 1] === '0') keep--;
  const fracDisplay = fracDigits.slice(0, keep);
  const showFrac = keep > 0;

  // Emisión. SIN agrupado los dígitos enteros se REPARTEN entre sus placeholders de
  // derecha a izquierda (el primero absorbe el sobrante): así funcionan los literales
  // ENTRE dígitos enteros — «00000-0000» + 123456789 → «12345-6789», «00-00» + 1234 →
  // «12-34». Con agrupado el entero ya lleva separadores y sale entero en el primero.
  const intParts: string[] = [];
  if (!grouping && intPh.length > 0) {
    let rest = intDisplay;
    for (let k = intPh.length - 1; k >= 1; k--) {
      intParts[k] = rest.slice(-1);
      rest = rest.slice(0, -1);
    }
    intParts[0] = rest;
  }
  let out = '';
  let intIdx = 0;
  let intEmitted = false;
  let fracEmitted = false;
  let dotConsumed = false;
  for (const t of toks) {
    if (t.t === 'lit') out += t.s;
    else if (t.t === 'pct') out += '%';
    else if (t.t === 'comma')
      continue; // ya contabilizada
    else if (t.t === 'dot') {
      if (!dotConsumed) {
        dotConsumed = true;
        if (!intEmitted && intPh.length === 0) {
          out += intDisplay; // máscara «.00» sin placeholder entero
          intEmitted = true;
        }
        if (showFrac) out += DECIMAL_SEP;
      } else out += '.'; // punto extra → literal
    } else if (t.t === 'd') {
      if (!dotConsumed) {
        if (intParts.length > 0) {
          out += intParts[intIdx++] ?? '';
        } else if (!intEmitted) {
          out += intDisplay;
          intEmitted = true;
        }
      } else if (!fracEmitted) {
        out += fracDisplay;
        fracEmitted = true;
      }
    }
  }
  return out;
}

/* ---------- Formateo de la sección FECHA/HORA ---------- */

type DateTok = { lit: string } | { tok: string };

const DATE_TOKENS = [
  'yyyy',
  'yy',
  'mmmmm',
  'mmmm',
  'mmm',
  'mm',
  'm',
  'dddd',
  'ddd',
  'dd',
  'd',
  'hh',
  'h',
  'ss',
  's',
  'am/pm',
  'a/p',
] as const;

/** Trocea una sección de fecha/hora en tokens reconocidos + literales. */
function scanDateTokens(body: string): DateTok[] {
  const toks: DateTok[] = [];
  const lower = body.toLowerCase();
  for (let i = 0; i < body.length;) {
    const ch = body[i]!;
    if (ch === '"') {
      let s = '';
      i++;
      while (i < body.length && body[i] !== '"') s += body[i++];
      i++; // comilla de cierre
      toks.push({ lit: s });
      continue;
    }
    if (ch === '\\') {
      if (i + 1 < body.length) toks.push({ lit: body[i + 1]! });
      i += 2;
      continue;
    }
    const hit = DATE_TOKENS.find((tk) => lower.startsWith(tk, i));
    if (hit) {
      toks.push({ tok: body.slice(i, i + hit.length) });
      i += hit.length;
      continue;
    }
    toks.push({ lit: ch });
    i++;
  }
  return toks;
}

/** Marca los «m»/«mm» como MINUTO si hay h/s adyacente (token de fecha vecino);
 *  si no, quedan como MES. Devuelve el set de índices que son minuto. */
function minuteIndices(toks: DateTok[]): Set<number> {
  const kinds = toks.map((t) => ('tok' in t ? t.tok.toLowerCase() : null));
  const isH = (k: string | null): boolean => k === 'h' || k === 'hh';
  const isS = (k: string | null): boolean => k === 's' || k === 'ss';
  const prevTok = (i: number): string | null => {
    for (let j = i - 1; j >= 0; j--) {
      const k = kinds[j];
      if (k) return k;
    }
    return null;
  };
  const nextTok = (i: number): string | null => {
    for (let j = i + 1; j < kinds.length; j++) {
      const k = kinds[j];
      if (k) return k;
    }
    return null;
  };
  const mins = new Set<number>();
  kinds.forEach((k, i) => {
    if (k === 'm' || k === 'mm') {
      if (isH(prevTok(i)) || isS(nextTok(i))) mins.add(i);
    }
  });
  return mins;
}

/** Renderiza un token de fecha con las partes calculadas. */
function renderDateTok(tok: string, p: DateParts, minute: boolean, ampm: boolean): string {
  const low = tok.toLowerCase();
  switch (low) {
    case 'yyyy':
      return String(p.y).padStart(4, '0');
    case 'yy':
      return pad2(p.y % 100);
    case 'mmmmm':
      return (MONTHS[p.mo - 1] ?? '').charAt(0).toUpperCase();
    case 'mmmm':
      return MONTHS[p.mo - 1] ?? '';
    case 'mmm':
      return MONTHS_SHORT[p.mo - 1] ?? '';
    case 'mm':
      return minute ? pad2(p.mi) : pad2(p.mo);
    case 'm':
      return minute ? String(p.mi) : String(p.mo);
    case 'dddd':
      return DAYS[p.wd] ?? '';
    case 'ddd':
      return DAYS_SHORT[p.wd] ?? '';
    case 'dd':
      return pad2(p.d);
    case 'd':
      return String(p.d);
    case 'hh':
      return pad2(ampm ? (p.hh % 12 === 0 ? 12 : p.hh % 12) : p.hh);
    case 'h':
      return String(ampm ? (p.hh % 12 === 0 ? 12 : p.hh % 12) : p.hh);
    case 'ss':
      return pad2(p.ss);
    case 's':
      return String(p.ss);
    case 'am/pm':
      return p.hh < 12 ? (tok === 'AM/PM' ? 'AM' : 'am') : tok === 'AM/PM' ? 'PM' : 'pm';
    case 'a/p':
      return p.hh < 12 ? (tok === 'A/P' ? 'A' : 'a') : tok === 'A/P' ? 'P' : 'p';
    default:
      return tok;
  }
}

/** Formatea un serial con una sección de fecha/hora; General si está fuera de rango. */
function formatDateSection(body: string, serial: number): string {
  const p = serialToParts(serial);
  if (!p) return formatNumber(serial);
  const toks = scanDateTokens(body);
  const mins = minuteIndices(toks);
  const ampm = toks.some(
    (t) => 'tok' in t && (t.tok.toLowerCase() === 'am/pm' || t.tok.toLowerCase() === 'a/p'),
  );
  return toks
    .map((t, i) => ('tok' in t ? renderDateTok(t.tok, p, mins.has(i), ampm) : t.lit))
    .join('');
}

/* ---------- Formateo de TIEMPO TRANSCURRIDO ([h]:mm:ss) ---------- */

/** Resto de tokens de una sección de tiempo transcurrido (más largo primero). */
const ELAPSED_REST = ['hh', 'h', 'mm', 'm', 'ss', 's'] as const;

/**
 * Sección de tiempo TRANSCURRIDO: la unidad entre corchetes ACUMULA (26:30 h no da
 * la vuelta a 2:30, como sí harían «h»/«mm» normales) y los tokens siguientes son su
 * resto: en «[h]:mm:ss», mm∈0..59 y ss∈0..59. Los negativos salen con «-» delante
 * (Excel muestra «###»; un signo es más útil que un error de ancho).
 */
function formatElapsedSection(body: string, serial: number): string {
  const total = Math.round(Math.abs(serial) * 86_400); // duración en segundos
  const value = (unit: string, whole: boolean): number => {
    if (unit === 'h') return whole ? Math.floor(total / 3600) : Math.floor(total / 3600) % 24;
    if (unit === 'm') return whole ? Math.floor(total / 60) : Math.floor(total / 60) % 60;
    return whole ? total : total % 60;
  };
  let out = serial < 0 ? '-' : '';
  for (let i = 0; i < body.length;) {
    const ch = body[i]!;
    if (ch === '"') {
      i++;
      while (i < body.length && body[i] !== '"') out += body[i++];
      i++; // comilla de cierre
      continue;
    }
    if (ch === '\\') {
      if (i + 1 < body.length) out += body[i + 1];
      i += 2;
      continue;
    }
    if (ch === '[') {
      const close = body.indexOf(']', i);
      const inner = close === -1 ? '' : body.slice(i + 1, close).toLowerCase();
      if (close !== -1 && ELAPSED_BRACKET.test(inner)) {
        const v = value(inner[0]!, true);
        out += inner.length === 2 ? String(v).padStart(2, '0') : String(v);
        i = close + 1;
        continue;
      }
      out += ch;
      i++;
      continue;
    }
    const rest = ELAPSED_REST.find((tk) => body.toLowerCase().startsWith(tk, i));
    if (rest) {
      const v = value(rest[0]!, false);
      out += rest.length === 2 ? pad2(v) : String(v);
      i += rest.length;
      continue;
    }
    out += ch;
    i++;
  }
  return out;
}

/* ---------- Punto de entrada ---------- */

/**
 * Aplica una máscara personalizada a un número. Elige la sección por el signo
 * (positivo;negativo;cero); con una sola sección, antepone «-» a los negativos.
 * La sección puede ser numérica, de fecha/hora o el literal «General».
 */
export function formatWithMask(mask: string, n: number): MaskResult {
  if (!Number.isFinite(n)) return { text: formatNumber(n) };
  const sections = splitSections(mask);
  const conds = sections.map(readCondition);

  let section: string;
  let autoMinus = false;
  if (conds.some((c) => c !== null)) {
    // Con CONDICIONES manda la primera que casa; si ninguna, la primera sin condición
    // (la de reserva) y, a falta de ella, General. Aquí la posición ya no implica el
    // signo (no hay «sección de negativos»), así que se conserva salvo que la máscara
    // lo escriba: el guardia de abajo evita el doble «-».
    let idx = conds.findIndex((c) => c?.(n) === true);
    if (idx === -1) idx = conds.findIndex((c) => c === null);
    if (idx === -1) return { text: formatNumber(n) };
    section = sections[idx]!;
    autoMinus = n < 0;
  } else if (n < 0) {
    section = sections[1] ?? sections[0]!;
    autoMinus = sections.length < 2; // sin sección negativa dedicada → «-» automático
  } else if (n === 0) {
    section = sections[2] ?? sections[0]!;
  } else {
    section = sections[0]!;
  }

  const { color, body } = stripBrackets(section);
  const trimmed = body.trim();
  const magnitude = Math.abs(n);

  let text: string;
  if (trimmed === '') {
    text = ''; // sección vacía (p. ej. «0.00;;» oculta el cero)
    autoMinus = false;
  } else if (trimmed.toLowerCase() === 'general') {
    text = formatNumber(magnitude);
  } else if (ELAPSED_IN_BODY.test(stripLiterals(body))) {
    text = formatElapsedSection(body, n); // acumula: antes que la fecha (también casa h/s)
    autoMinus = false;
  } else if (isDateTime(body)) {
    text = formatDateSection(body, n); // la fecha usa el serial crudo (con signo)
    autoMinus = false;
  } else {
    text = formatNumberSection(body, magnitude);
  }
  // El guardia evita «--5» cuando la propia máscara ya escribe el signo («-0»).
  if (autoMinus && !text.startsWith('-')) text = `-${text}`;
  return color !== undefined ? { text, color } : { text };
}
