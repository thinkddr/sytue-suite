/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Fecha y hora (TODAY, NOW, DATE, TIME, YEAR, MONTH, DAY, HOUR, MINUTE, SECOND,
 * WEEKDAY, WEEKNUM, EDATE, EOMONTH, DATEDIF, NETWORKDAYS, WORKDAY, DAYS,
 * DATEVALUE, TIMEVALUE). Semántica OpenFormula / ECMA-376, código original
 * (clean-room). NO edites functions/index.ts: se fusiona automáticamente.
 *
 * SISTEMA DE FECHA SERIE (ECMA-376 §18.17.4.1, «sistema 1900»):
 *   - El serial es un número: la parte entera cuenta días, la fraccional la hora.
 *   - Serial 1 = 1900-01-01 (el «cero», serial 0, es 1899-12-31, base del cálculo).
 *   - Se REPRODUCE el bug histórico de Lotus 1-2-3: 1900 se trata como bisiesto,
 *     así que existe el día fantasma 1900-02-29 = serial 60. Consecuencia: todo
 *     serial ≥ 61 (1900-03-01 en adelante) coincide EXACTAMENTE con Excel/Sheets.
 *   - Deviaciones deliberadas del bug (documentadas, sin valor práctico): DATE(1900,2,29)
 *     normaliza a 1900-03-01 (=61, no 60) y daysInMonth(1900,2)=28 (isLeap real).
 *   - TODAY/NOW son volátiles: leen ctx.now() (reloj inyectable, determinista en tests)
 *     y usan sus componentes LOCALES (fecha/hora local, como Excel).
 */
import type { Arg, FnTable } from '../registry.js';
import {
  coerceBoolean,
  coerceNumber,
  coerceText,
  ERR,
  FormulaError,
  isError,
  type CellValue,
} from '../values.js';

/* ---------- Conversión de fecha serie ↔ calendario ---------- */

const MS_PER_DAY = 86_400_000;
const SECS_PER_DAY = 86_400;
const MONTH_DAYS = [31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31];

/** Milisegundos UTC de la medianoche de (año, mes 1-based, día). Normaliza el
 *  desbordamiento de mes/día y evita la trampa de años de 2 cifras de Date.UTC. */
function ymdToMs(y: number, m: number, d: number): number {
  const dt = new Date(0);
  dt.setUTCFullYear(y, m - 1, d);
  return dt.getTime();
}

/** Serial 0 = 1899-12-31. */
const EPOCH_MS = ymdToMs(1899, 12, 31);

function isLeap(y: number): boolean {
  return (y % 4 === 0 && y % 100 !== 0) || y % 400 === 0;
}

function daysInMonth(y: number, m: number): number {
  if (m === 2) return isLeap(y) ? 29 : 28;
  return MONTH_DAYS[m - 1] ?? 30;
}

/** (año, mes, día) → serial entero, con el día fantasma 1900-02-29 = 60. */
function serialFromYMD(y: number, m: number, d: number): number {
  let s = Math.round((ymdToMs(y, m, d) - EPOCH_MS) / MS_PER_DAY);
  if (s > 59) s += 1; // añade el inexistente 29-feb-1900 (bug 1900 bisiesto)
  return s;
}

/** Rango válido del sistema de fechas: [0, 2958465] = 1900-01-00 … 9999-12-31.
 *  Fuera de él, las funciones que PRODUCEN una fecha devuelven #NUM! (como Excel). */
const MAX_SERIAL = serialFromYMD(9999, 12, 31);

/** Serial entero → {y, m, d} del calendario. Reproduce las quirks de Excel en
 *  los dos seriales especiales del sistema 1900. */
export function ymdFromSerial(day: number): { y: number; m: number; d: number } {
  if (day === 0) return { y: 1900, m: 1, d: 0 }; // «1900-01-00» de Excel (YEAR=1900)
  if (day === 60) return { y: 1900, m: 2, d: 29 }; // día fantasma
  const real = day > 60 ? day - 1 : day; // descuenta el 29-feb-1900 inexistente
  const dt = new Date(EPOCH_MS + real * MS_PER_DAY);
  return { y: dt.getUTCFullYear(), m: dt.getUTCMonth() + 1, d: dt.getUTCDate() };
}

/** Día de la semana 0=Domingo … 6=Sábado (serial 1 = 1900-01-01 = Domingo). */
function dowSun(day: number): number {
  return (((day - 1) % 7) + 7) % 7;
}

function isWeekend(day: number): boolean {
  const w = dowSun(day);
  return w === 0 || w === 6;
}

/* ---------- Parseo de texto de fecha/hora (DATEVALUE / TIMEVALUE) ---------- */

/** Valida (año, mes, día) reales y devuelve el serial; undefined si no existe. */
function checkedDate(y: number, m: number, d: number): number | undefined {
  if (m < 1 || m > 12) return undefined;
  if (d < 1 || d > daysInMonth(y, m)) return undefined;
  const s = serialFromYMD(y, m, d);
  return s < 0 ? undefined : s;
}

/** Expande un año de 2 cifras a la ventana de Excel (0-29 → 2000+, 30-99 → 1900+). */
function expandYear(y: number): number {
  return y < 100 ? y + (y < 30 ? 2000 : 1900) : y;
}

/** Formatos de fecha aceptados: ISO «AAAA-M-D», «AAAA/M/D» y US «M/D/AAAA». */
function parseDate(s: string): number | undefined {
  let m = /^(\d{4})-(\d{1,2})-(\d{1,2})$/.exec(s);
  if (m) return checkedDate(+m[1]!, +m[2]!, +m[3]!);
  m = /^(\d{1,4})\/(\d{1,2})\/(\d{1,4})$/.exec(s);
  if (m) {
    const a = +m[1]!;
    const b = +m[2]!;
    const c = +m[3]!;
    if (m[1]!.length === 4 || a > 31) return checkedDate(expandYear(a), b, c); // año primero
    let mo = a;
    let d = b;
    if (mo > 12 && d <= 12) [mo, d] = [d, mo]; // «D/M/AAAA» si el primero no es mes
    return checkedDate(expandYear(c), mo, d);
  }
  return undefined;
}

/** «H:M», «H:M:S», con AM/PM opcional → fracción de día [0, 1); undefined si no. */
function parseTime(s: string): number | undefined {
  const m = /^(\d{1,2}):(\d{1,2})(?::(\d{1,2}))?\s*([APap][Mm])?$/.exec(s.trim());
  if (!m) return undefined;
  let h = +m[1]!;
  const mi = +m[2]!;
  const se = m[3] ? +m[3] : 0;
  if (mi > 59 || se > 59) return undefined;
  const ap = m[4]?.toUpperCase();
  if (ap) {
    if (h < 1 || h > 12) return undefined;
    if (ap === 'PM' && h !== 12) h += 12;
    if (ap === 'AM' && h === 12) h = 0;
  } else if (h > 23) {
    return undefined;
  }
  return (h * 3600 + mi * 60 + se) / SECS_PER_DAY;
}

/** Fecha con hora opcional (o solo hora) → serial; undefined si no parsea. */
function parseDateTimeText(raw: string): number | undefined {
  const s = raw.trim();
  const tOnly = parseTime(s);
  if (tOnly !== undefined) return tOnly;
  const parts = s.split(/\s+/);
  const d = parseDate(parts[0]!);
  if (d === undefined) return undefined;
  if (parts.length === 1) return d;
  const t = parseTime(parts.slice(1).join(' '));
  return t === undefined ? undefined : d + t;
}

/* ---------- Coacción de argumentos ---------- */

/** Coacciona un valor a serial: número directo, o texto de fecha/hora; los
 *  errores se PROPAGAN (no se enmascaran como #VALUE!). */
function toSerial(v: CellValue): number | FormulaError {
  const n = coerceNumber(v);
  if (!isError(n)) return n;
  if (typeof v === 'string') {
    const s = parseDateTimeText(v);
    return s !== undefined ? s : ERR.VALUE;
  }
  return n; // v era un error → se propaga
}

function serialArg(args: Arg[], i: number): number | FormulaError {
  const a = args[i];
  return a ? toSerial(a.value()) : ERR.VALUE;
}

function numArg(args: Arg[], i: number): number | FormulaError {
  const a = args[i];
  return a ? coerceNumber(a.value()) : ERR.VALUE;
}

/** Reúne los seriales de un rango de festivos (blancos ignorados). */
function gatherHolidays(args: Arg[], i: number): Set<number> | FormulaError {
  const set = new Set<number>();
  const a = args[i];
  if (!a) return set;
  for (const row of a.matrix()) {
    for (const v of row) {
      if (v === '') continue; // celda en blanco ≈ "": se ignora
      const s = toSerial(v);
      if (isError(s)) return s;
      set.add(Math.floor(s));
    }
  }
  return set;
}

/** Cuenta días laborables (lun-vie) entre a y b inclusive, con a ≤ b. O(1)+resto. */
function countWeekdays(a: number, b: number): number {
  const days = b - a + 1;
  let count = Math.floor(days / 7) * 5;
  const extra = days % 7;
  const start = dowSun(a);
  for (let i = 0; i < extra; i++) {
    const w = (start + i) % 7;
    if (w >= 1 && w <= 5) count++; // lunes(1)…viernes(5)
  }
  return count;
}

/** Códigos numéricos de fin de semana de WORKDAY.INTL/NETWORKDAYS.INTL →
 *  días (0=dom … 6=sáb, base dowSun). 1-7 = dos días; 11-17 = un solo día. */
const WEEKEND_CODES: Record<number, number[]> = {
  1: [6, 0],
  2: [0, 1],
  3: [1, 2],
  4: [2, 3],
  5: [3, 4],
  6: [4, 5],
  7: [5, 6],
  11: [0],
  12: [1],
  13: [2],
  14: [3],
  15: [4],
  16: [5],
  17: [6],
};
/** Cadena «MTWTFSS» (lun→dom) a día dowSun (0=dom … 6=sáb). */
const STR_POS_TO_DOW = [1, 2, 3, 4, 5, 6, 0];

/** Resuelve el argumento `fin_de_semana` (INTL) a un conjunto de días dowSun.
 *  Ausente → sáb+dom. Código numérico inválido → #NUM!; cadena inválida (no
 *  «[01]{7}» o toda de unos) → #VALUE!, como Excel. */
function weekendSet(a: Arg | undefined): Set<number> | FormulaError {
  if (!a) return new Set([0, 6]);
  const v = a.value();
  if (typeof v === 'string') {
    if (!/^[01]{7}$/.test(v) || v === '1111111') return ERR.VALUE;
    const set = new Set<number>();
    for (let i = 0; i < 7; i++) if (v[i] === '1') set.add(STR_POS_TO_DOW[i]!);
    return set;
  }
  const n = coerceNumber(v);
  if (isError(n)) return n;
  const days = WEEKEND_CODES[Math.trunc(n)];
  return days ? new Set(days) : ERR.NUM;
}

/** Componentes {y,m,d} de un argumento serial; #NUM! si es negativo. */
function dateParts(args: Arg[], i: number): { y: number; m: number; d: number } | FormulaError {
  const s = serialArg(args, i);
  if (isError(s)) return s;
  if (s < 0) return ERR.NUM;
  return ymdFromSerial(Math.floor(s));
}

/** Segundos del día [0, 86400) de un argumento serial; #NUM! si es negativo. */
function daySeconds(args: Arg[], i: number): number | FormulaError {
  const s = serialArg(args, i);
  if (isError(s)) return s;
  if (s < 0) return ERR.NUM;
  return Math.round((s - Math.floor(s)) * SECS_PER_DAY) % SECS_PER_DAY;
}

/** Desplaza `months` meses; devuelve serial del mismo día (clamp) o fin de mes.
 *  Exportada para el calendario de cupones (COUPPCD/COUPNCD y familia), que
 *  genera sus fechas retrocediendo meses desde el vencimiento. */
export function shiftMonth(startSerial: number, months: number, endOfMonth: boolean): number {
  const { y, m, d } = ymdFromSerial(Math.floor(startSerial));
  const idx = y * 12 + (m - 1) + months;
  const ny = Math.floor(idx / 12);
  const nm = (((idx % 12) + 12) % 12) + 1;
  const last = daysInMonth(ny, nm);
  return serialFromYMD(ny, nm, endOfMonth ? last : Math.min(d, last));
}

/* ---------- Fracción de año (YEARFRAC) ---------- */

type YMD = { y: number; m: number; d: number };

function daysInYear(y: number): number {
  return isLeap(y) ? 366 : 365;
}

/** ¿Es el último día de febrero de su año? */
function isEndOfFeb(p: YMD): boolean {
  return p.m === 2 && p.d === daysInMonth(p.y, 2);
}

/** Días entre A y B por la base 30/360 US (NASD). */
function days360US(a: YMD, b: YMD): number {
  let d1 = a.d;
  let d2 = b.d;
  if (isEndOfFeb(a) && isEndOfFeb(b)) d2 = 30;
  if (isEndOfFeb(a)) d1 = 30;
  if (d1 === 31) d1 = 30;
  if (d2 === 31 && d1 === 30) d2 = 30;
  return (b.y - a.y) * 360 + (b.m - a.m) * 30 + (d2 - d1);
}

/** Días entre A y B por la base 30E/360 (europea). */
function days360EU(a: YMD, b: YMD): number {
  const d1 = a.d === 31 ? 30 : a.d;
  const d2 = b.d === 31 ? 30 : b.d;
  return (b.y - a.y) * 360 + (b.m - a.m) * 30 + (d2 - d1);
}

/** Fracción real/real: días reales sobre la longitud media de los años del tramo. */
function actualActual(aSer: number, bSer: number, a: YMD, b: YMD): number {
  const days = bSer - aSer;
  if (a.y === b.y) return days / daysInYear(a.y);
  let total = 0;
  for (let y = a.y; y <= b.y; y++) total += daysInYear(y);
  return days / (total / (b.y - a.y + 1));
}

function pad(n: number, len: number): string {
  return String(n).padStart(len, '0');
}

/**
 * ¿El serial cae en el último día de su mes? Lo necesita el calendario de
 * cupones: si el vencimiento es fin de mes, TODOS los cupones lo son (un bono
 * que vence el 31-ago paga el 28/29-feb, no el «31-feb» recortado al 28 y de
 * ahí en adelante), y esa diferencia mueve el devengado de un día entero.
 */
export function isMonthEnd(serial: number): boolean {
  const { y, m, d } = ymdFromSerial(Math.floor(serial));
  return d === daysInMonth(y, m);
}

/**
 * DÍAS (no fracción de año) entre dos seriales por la `base` 0-4. Es lo que
 * piden COUPDAYBS/COUPDAYSNC, que devuelven un recuento y no una proporción.
 * Bases 1/2/3 cuentan días reales; 0 y 4 son las dos convenciones 30/360.
 * Base fuera de 0-4 → #NUM!.
 */
export function dayCount(a: number, b: number, basis: number): number | FormulaError {
  const A = ymdFromSerial(Math.floor(a));
  const B = ymdFromSerial(Math.floor(b));
  switch (basis) {
    case 0:
      return days360US(A, B);
    case 1:
    case 2:
    case 3:
      return Math.floor(b) - Math.floor(a);
    case 4:
      return days360EU(A, B);
    default:
      return ERR.NUM;
  }
}

/**
 * Fracción de año entre dos seriales (ya truncados a día) por la `base` 0-4.
 * Simétrica (no depende del orden). Núcleo reutilizado por YEARFRAC y por las
 * financieras de descuento (DISC/INTRATE/RECEIVED). Serial negativo → #NUM!;
 * base fuera de 0-4 → #NUM!.
 */
export function yearFrac(a: number, b: number, basis: number): number | FormulaError {
  if (a < 0 || b < 0) return ERR.NUM;
  if (a === b) return 0;
  if (a > b) [a, b] = [b, a];
  const A = ymdFromSerial(a);
  const B = ymdFromSerial(b);
  switch (basis) {
    case 0:
      return days360US(A, B) / 360;
    case 1:
      return actualActual(a, b, A, B);
    case 2:
      return (b - a) / 360;
    case 3:
      return (b - a) / 365;
    case 4:
      return days360EU(A, B) / 360;
    default:
      return ERR.NUM;
  }
}

/* ---------- Registro ---------- */

export const funcs: FnTable = {
  // Fecha actual (sin hora), del reloj inyectable. Volátil.
  TODAY(ctx, args) {
    if (args.length !== 0) return ERR.VALUE;
    const d = ctx.now();
    return serialFromYMD(d.getFullYear(), d.getMonth() + 1, d.getDate());
  },

  // Fecha y hora actuales (serial con fracción). Volátil.
  NOW(ctx, args) {
    if (args.length !== 0) return ERR.VALUE;
    const d = ctx.now();
    const day = serialFromYMD(d.getFullYear(), d.getMonth() + 1, d.getDate());
    const secs =
      d.getHours() * 3600 + d.getMinutes() * 60 + d.getSeconds() + d.getMilliseconds() / 1000;
    return day + secs / SECS_PER_DAY;
  },

  // Serial de (año, mes, día). Mes/día desbordan (rollover); año 0-1899 → +1900.
  DATE(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const yv = numArg(args, 0);
    if (isError(yv)) return yv;
    const mv = numArg(args, 1);
    if (isError(mv)) return mv;
    const dv = numArg(args, 2);
    if (isError(dv)) return dv;
    let y = Math.trunc(yv);
    const m = Math.trunc(mv);
    const d = Math.trunc(dv);
    if (y < 0 || y > 9999) return ERR.NUM;
    if (y < 1900) y += 1900;
    const s = serialFromYMD(y, m, d);
    // Fuera de rango: antes de 1900-01-00 o, por desbordamiento de mes/día, tras 9999-12-31.
    return s < 0 || s > MAX_SERIAL ? ERR.NUM : s;
  },

  // Fracción de día de (hora, minuto, segundo). Envuelve mód 24 h; negativo → #NUM!.
  TIME(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const hv = numArg(args, 0);
    if (isError(hv)) return hv;
    const miv = numArg(args, 1);
    if (isError(miv)) return miv;
    const sv = numArg(args, 2);
    if (isError(sv)) return sv;
    const total = Math.trunc(hv) * 3600 + Math.trunc(miv) * 60 + Math.trunc(sv);
    if (total < 0) return ERR.NUM;
    return (total % SECS_PER_DAY) / SECS_PER_DAY;
  },

  YEAR(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const p = dateParts(args, 0);
    return p instanceof FormulaError ? p : p.y;
  },
  MONTH(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const p = dateParts(args, 0);
    return p instanceof FormulaError ? p : p.m;
  },
  DAY(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const p = dateParts(args, 0);
    return p instanceof FormulaError ? p : p.d;
  },

  HOUR(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = daySeconds(args, 0);
    return isError(t) ? t : Math.floor(t / 3600);
  },
  MINUTE(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = daySeconds(args, 0);
    return isError(t) ? t : Math.floor(t / 60) % 60;
  },
  SECOND(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = daySeconds(args, 0);
    return isError(t) ? t : t % 60;
  },

  // Día de la semana. Tipo 1 (dom=1…sáb=7) por defecto; 2, 3 y 11-17 soportados.
  WEEKDAY(_ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const s = serialArg(args, 0);
    if (isError(s)) return s;
    if (s < 0) return ERR.NUM;
    let type = 1;
    if (args.length === 2) {
      const t = numArg(args, 1);
      if (isError(t)) return t;
      type = Math.trunc(t);
    }
    const w = dowSun(Math.floor(s)); // 0=dom…6=sáb
    if (type === 1) return w + 1;
    if (type === 2) return ((w + 6) % 7) + 1;
    if (type === 3) return (w + 6) % 7;
    if (type >= 11 && type <= 17) {
      const startSun = (type - 10) % 7; // 11→lun(1)…17→dom(0)
      return ((w - startSun + 7) % 7) + 1;
    }
    return ERR.NUM;
  },

  // Número de semana. Tipo 1 (semanas dom, sem 1 = la del 1-ene) por defecto;
  // 2, 11-17 (día de inicio) y 21 (ISO 8601) soportados.
  WEEKNUM(_ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const s = serialArg(args, 0);
    if (isError(s)) return s;
    if (s < 0) return ERR.NUM;
    let type = 1;
    if (args.length === 2) {
      const t = numArg(args, 1);
      if (isError(t)) return t;
      type = Math.trunc(t);
    }
    const day = Math.floor(s);
    if (type === 21) {
      // ISO: la semana 1 es la del primer jueves; semanas empiezan en lunes.
      const isoDow = ((dowSun(day) + 6) % 7) + 1; // 1=lun…7=dom
      const thursday = day + (4 - isoDow);
      const jan1 = serialFromYMD(ymdFromSerial(thursday).y, 1, 1);
      return Math.floor((thursday - jan1) / 7) + 1;
    }
    let startSun: number;
    if (type === 1) startSun = 0;
    else if (type === 2) startSun = 1;
    else if (type >= 11 && type <= 17) startSun = (type - 10) % 7;
    else return ERR.NUM;
    const jan1 = serialFromYMD(ymdFromSerial(day).y, 1, 1);
    const offset = (dowSun(jan1) - startSun + 7) % 7;
    return Math.floor((day - (jan1 - offset)) / 7) + 1;
  },

  // Fecha `months` meses después (mismo día, con clamp a fin de mes).
  EDATE(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const s = serialArg(args, 0);
    if (isError(s)) return s;
    if (s < 0) return ERR.NUM; // fecha de inicio inválida (antes de 1900-01-00)
    const mo = numArg(args, 1);
    if (isError(mo)) return mo;
    const r = shiftMonth(s, Math.trunc(mo), false);
    return r < 0 || r > MAX_SERIAL ? ERR.NUM : r;
  },

  // Último día del mes `months` meses después.
  EOMONTH(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const s = serialArg(args, 0);
    if (isError(s)) return s;
    if (s < 0) return ERR.NUM; // fecha de inicio inválida (antes de 1900-01-00)
    const mo = numArg(args, 1);
    if (isError(mo)) return mo;
    const r = shiftMonth(s, Math.trunc(mo), true);
    return r < 0 || r > MAX_SERIAL ? ERR.NUM : r;
  },

  // Diferencia entre fechas por unidad. Función heredada (start ≤ end).
  DATEDIF(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const sS = serialArg(args, 0);
    if (isError(sS)) return sS;
    const eS = serialArg(args, 1);
    if (isError(eS)) return eS;
    const uv = args[2] ? coerceText(args[2].value()) : ERR.VALUE;
    if (isError(uv)) return uv;
    const start = Math.floor(sS);
    const end = Math.floor(eS);
    if (start < 0 || end < start) return ERR.NUM;
    const s = ymdFromSerial(start);
    const e = ymdFromSerial(end);
    switch (uv.trim().toUpperCase()) {
      case 'D':
        return end - start;
      case 'Y': {
        let y = e.y - s.y;
        if (e.m < s.m || (e.m === s.m && e.d < s.d)) y--;
        return y;
      }
      case 'M': {
        let m = (e.y - s.y) * 12 + (e.m - s.m);
        if (e.d < s.d) m--;
        return m;
      }
      case 'YM': {
        let m = (e.y - s.y) * 12 + (e.m - s.m);
        if (e.d < s.d) m--;
        return ((m % 12) + 12) % 12;
      }
      case 'MD': {
        // ponytail: "MD" es notoriamente inestable en Excel cerca de fin de mes;
        // implementamos la definición estándar (bug incluido) y no lo tocamos.
        let dd = e.d - s.d;
        if (dd < 0) {
          const pm = e.m === 1 ? 12 : e.m - 1;
          const py = e.m === 1 ? e.y - 1 : e.y;
          dd += daysInMonth(py, pm);
        }
        return dd;
      }
      case 'YD': {
        let a = serialFromYMD(e.y, s.m, s.d);
        if (a > end) a = serialFromYMD(e.y - 1, s.m, s.d);
        return end - a;
      }
      default:
        return ERR.NUM;
    }
  },

  // Días laborables (lun-vie) entre dos fechas, inclusive, menos festivos.
  // start > end → recuento negativo (como Excel).
  NETWORKDAYS(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const sS = serialArg(args, 0);
    if (isError(sS)) return sS;
    const eS = serialArg(args, 1);
    if (isError(eS)) return eS;
    const hol = gatherHolidays(args, 2);
    if (hol instanceof FormulaError) return hol;
    let a = Math.floor(sS);
    let b = Math.floor(eS);
    let sign = 1;
    if (a > b) {
      [a, b] = [b, a];
      sign = -1;
    }
    let count = countWeekdays(a, b);
    for (const h of hol) if (h >= a && h <= b && !isWeekend(h)) count--;
    return sign * count;
  },

  // Fecha a `days` días laborables de `start`, saltando fines de semana y festivos.
  WORKDAY(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const sS = serialArg(args, 0);
    if (isError(sS)) return sS;
    const dv = numArg(args, 1);
    if (isError(dv)) return dv;
    const hol = gatherHolidays(args, 2);
    if (hol instanceof FormulaError) return hol;
    let cur = Math.floor(sS);
    if (cur < 0) return ERR.NUM; // fecha de inicio inválida (antes de 1900-01-00)
    let rem = Math.abs(Math.trunc(dv));
    const dir = dv < 0 ? -1 : 1;
    // ponytail: avance día a día (O(días)); suficiente para el rango real de uso.
    while (rem > 0) {
      cur += dir;
      if (!isWeekend(cur) && !hol.has(cur)) rem--;
    }
    return cur < 0 || cur > MAX_SERIAL ? ERR.NUM : cur; // salió del rango de fechas
  },

  // Número de días entre dos fechas: end − start.
  DAYS(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const end = serialArg(args, 0);
    if (isError(end)) return end;
    const start = serialArg(args, 1);
    if (isError(start)) return start;
    return Math.floor(end) - Math.floor(start);
  },

  // Texto de fecha → serial (entero, ignora la hora). #VALUE! si no reconoce.
  DATEVALUE(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = args[0] ? coerceText(args[0].value()) : ERR.VALUE;
    if (isError(t)) return t;
    const d = parseDate(t.trim().split(/\s+/)[0]!);
    return d !== undefined ? d : ERR.VALUE;
  },

  // Texto de hora → fracción de día (ignora la fecha). #VALUE! si no reconoce.
  TIMEVALUE(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = args[0] ? coerceText(args[0].value()) : ERR.VALUE;
    if (isError(t)) return t;
    const s = t.trim();
    let frac = parseTime(s);
    if (frac === undefined) {
      const parts = s.split(/\s+/);
      if (parts.length >= 2) frac = parseTime(parts.slice(1).join(' '));
    }
    return frac !== undefined ? frac : ERR.VALUE;
  },

  // Número de semana ISO 8601 (semanas de lunes; la semana 1 contiene el 1er jueves).
  ISOWEEKNUM(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const s = serialArg(args, 0);
    if (isError(s)) return s;
    if (s < 0) return ERR.NUM;
    const day = Math.floor(s);
    const isoDow = ((dowSun(day) + 6) % 7) + 1; // 1=lun … 7=dom
    const thursday = day + (4 - isoDow);
    const jan1 = serialFromYMD(ymdFromSerial(thursday).y, 1, 1);
    return Math.floor((thursday - jan1) / 7) + 1;
  },

  // Fracción de año entre dos fechas. YEARFRAC(inicio, fin, [base=0]).
  // Bases: 0 US 30/360, 1 real/real, 2 real/360, 3 real/365, 4 europea 30E/360.
  // Siempre no negativa (simétrica en el orden de las fechas).
  YEARFRAC(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const s0 = serialArg(args, 0);
    if (isError(s0)) return s0;
    const e0 = serialArg(args, 1);
    if (isError(e0)) return e0;
    let basis = 0;
    if (args.length >= 3) {
      const b = numArg(args, 2);
      if (isError(b)) return b;
      basis = Math.trunc(b);
    }
    return yearFrac(Math.floor(s0), Math.floor(e0), basis);
  },

  // DATESTR(fecha): serial → texto ISO «AAAA-MM-DD» (inverso de DATEVALUE).
  DATESTR(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const s = serialArg(args, 0);
    if (isError(s)) return s;
    if (s < 0) return ERR.NUM;
    const { y, m, d } = ymdFromSerial(Math.floor(s));
    return `${pad(y, 4)}-${pad(m, 2)}-${pad(d, 2)}`;
  },

  // DAYS360(inicio, fin, [método=FALSE]): días entre fechas contando meses de 30
  // días (año de 360). método FALSE = US (NASD); TRUE = europeo (30E/360). No
  // simétrica: inicio > fin da resultado negativo.
  DAYS360(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const s0 = serialArg(args, 0);
    if (isError(s0)) return s0;
    const e0 = serialArg(args, 1);
    if (isError(e0)) return e0;
    let euro = false;
    if (args.length === 3) {
      const b = coerceBoolean(args[2]!.value());
      if (isError(b)) return b;
      euro = b;
    }
    const a = Math.floor(s0);
    const b = Math.floor(e0);
    if (a < 0 || b < 0) return ERR.NUM;
    const A = ymdFromSerial(a);
    const B = ymdFromSerial(b);
    return euro ? days360EU(A, B) : days360US(A, B);
  },

  // NETWORKDAYS.INTL(inicio, fin, [fin_de_semana=1], [festivos]): días laborables
  // inclusive con fin de semana configurable. inicio > fin → negativo.
  'NETWORKDAYS.INTL'(_ctx, args) {
    if (args.length < 2 || args.length > 4) return ERR.VALUE;
    const sS = serialArg(args, 0);
    if (isError(sS)) return sS;
    const eS = serialArg(args, 1);
    if (isError(eS)) return eS;
    const wknd = weekendSet(args[2]);
    if (wknd instanceof FormulaError) return wknd;
    const hol = gatherHolidays(args, 3);
    if (hol instanceof FormulaError) return hol;
    let a = Math.floor(sS);
    let b = Math.floor(eS);
    let sign = 1;
    if (a > b) {
      [a, b] = [b, a];
      sign = -1;
    }
    let count = 0;
    // ponytail: recorrido día a día (O(días)); basta para el rango real de uso.
    for (let d = a; d <= b; d++) if (!wknd.has(dowSun(d)) && !hol.has(d)) count++;
    return sign * count;
  },

  // WORKDAY.INTL(inicio, días, [fin_de_semana=1], [festivos]): fecha a `días`
  // laborables de `inicio` con fin de semana configurable.
  'WORKDAY.INTL'(_ctx, args) {
    if (args.length < 2 || args.length > 4) return ERR.VALUE;
    const sS = serialArg(args, 0);
    if (isError(sS)) return sS;
    const dv = numArg(args, 1);
    if (isError(dv)) return dv;
    const wknd = weekendSet(args[2]);
    if (wknd instanceof FormulaError) return wknd;
    const hol = gatherHolidays(args, 3);
    if (hol instanceof FormulaError) return hol;
    let cur = Math.floor(sS);
    if (cur < 0) return ERR.NUM;
    let rem = Math.abs(Math.trunc(dv));
    const dir = dv < 0 ? -1 : 1;
    while (rem > 0) {
      cur += dir;
      if (!wknd.has(dowSun(cur)) && !hol.has(cur)) rem--;
    }
    return cur < 0 || cur > MAX_SERIAL ? ERR.NUM : cur;
  },
};
