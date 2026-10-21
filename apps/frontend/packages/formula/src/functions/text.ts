/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Funciones de TEXTO (semántica OpenFormula / ECMA-376, código original
 * clean-room): CONCAT, CONCATENATE, LEFT, RIGHT, MID, LEN, LOWER, UPPER,
 * PROPER, TRIM, REPLACE, SUBSTITUTE, FIND, SEARCH, TEXT, VALUE, NUMBERVALUE,
 * REPT, TEXTJOIN, CHAR, CODE, UNICHAR, UNICODE, EXACT, CLEAN, T.
 *
 * Contrato en ../registry.ts. Las cadenas se miden en unidades UTF-16 (como
 * hacen LEN/LEFT/MID en una hoja real). NO se edita functions/index.ts.
 */
import type { Arg, FnTable } from '../registry.js';
import type { CellValue } from '../values.js';
import {
  ERR,
  FormulaError,
  isError,
  coerceNumber,
  coerceText,
  coerceBoolean,
  formatNumber,
} from '../values.js';
import { compilarRegex, Desmesurado, grupo, Maquina, type Coincidencia } from './regex.js';

/** Guard local sobre uniones que no son CellValue (p. ej. string[] | FormulaError). */
function isErr(v: unknown): v is FormulaError {
  return v instanceof FormulaError;
}

/* ------------------------------------------------------------------ */
/*  Ayudantes                                                          */
/* ------------------------------------------------------------------ */

/** Coacciona a texto el valor escalar del argumento (propaga errores). */
function textArg(a: Arg): string | FormulaError {
  return coerceText(a.value());
}

/** Coacciona a número entero (truncado) el argumento (propaga errores). */
function intArg(a: Arg): number | FormulaError {
  const n = coerceNumber(a.value());
  return isError(n) ? n : Math.trunc(n);
}

/** Escapa un carácter para insertarlo literal en una expresión regular. */
function escapeRe(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/* ------------------------------------------------------------------ */
/*  Expresiones regulares (REGEXTEST/EXTRACT/REPLACE)                  */
/* ------------------------------------------------------------------ */
// Sin el RegExp de JavaScript: `(a+)+$` congelaba con vuelta atrás la pestaña de quien
// abría la hoja (WEB-13). regex.ts es una máquina de Pike, lineal y con presupuesto, y
// con las coincidencias de RE2, las del motor nativo; referencias hacia atrás y
// lookaround dan #VALUE!, como allí.

const REGEX_MAX_PATTERN = 1000;

/** Compila un patrón ante un texto; inválido, no admitido o desmesurado → #VALUE!. */
function buildRegex(pattern: string, t: string, ci: boolean): Maquina | FormulaError {
  if (pattern.length > REGEX_MAX_PATTERN) return ERR.VALUE;
  const prog = compilarRegex(pattern, ci);
  return prog === null ? ERR.VALUE : new Maquina(prog, t);
}

/** Corre una búsqueda; la que se pasa de su presupuesto es #VALUE!. */
function conPresupuesto<T>(fn: () => T): T | FormulaError {
  try {
    return fn();
  } catch (e) {
    if (e instanceof Desmesurado) return ERR.VALUE;
    throw e;
  }
}

/** Lee el argumento `case_sensitivity` (1 = insensible a mayúsculas); FALSE si falta. */
function caseInsensitive(args: Arg[], i: number): boolean | FormulaError {
  if (i >= args.length) return false;
  const n = coerceNumber(args[i]!.value());
  return isError(n) ? n : n === 1;
}

/** Expande $&, $$ y $1…$99 de una plantilla de reemplazo con la coincidencia. */
function expandRepl(tpl: string, t: string, m: Coincidencia): string {
  return tpl.replace(/\$(\d{1,2}|[$&])/g, (_all, g: string) => {
    if (g === '$') return '$';
    if (g === '&') return grupo(t, m, 0);
    return grupo(t, m, Number(g));
  });
}

/**
 * GetSubstitution de ECMAScript, la de `String.prototype.replace`, que es la que se usa
 * al sustituir TODAS: `$$`, `$&`, `` $` ``, `$'` y `$1`…`$99`; un `$3` sin tercer grupo
 * se queda literal. La misma que `sustitucionJS` del motor nativo.
 */
function sustitucion(tpl: string, t: string, m: Coincidencia): string {
  const grupos = m.length / 2 - 1;
  let out = '';
  for (let i = 0; i < tpl.length; i++) {
    const c = tpl[i]!;
    if (c !== '$' || i + 1 >= tpl.length) {
      out += c;
      continue;
    }
    const sig = tpl[i + 1]!;
    if (sig === '$') {
      out += '$';
      i++;
    } else if (sig === '&') {
      out += grupo(t, m, 0);
      i++;
    } else if (sig === '`') {
      out += t.slice(0, m[0]);
      i++;
    } else if (sig === "'") {
      out += t.slice(m[1]);
      i++;
    } else if (sig >= '0' && sig <= '9') {
      let n = Number(sig);
      let largo = 1;
      const otro = tpl[i + 2];
      if (otro !== undefined && otro >= '0' && otro <= '9') {
        const nn = n * 10 + Number(otro);
        if (nn >= 1 && nn <= grupos) {
          n = nn;
          largo = 2;
        }
      }
      if (n < 1 || n > grupos) {
        out += '$';
        continue;
      }
      out += grupo(t, m, n);
      i += largo;
    } else {
      out += '$';
    }
  }
  return out;
}

/** Concatena todos los argumentos (rangos incluidos, en orden fila-columna). */
function concatImpl(args: Arg[]): CellValue {
  let out = '';
  for (const a of args) {
    for (const row of a.matrix()) {
      for (const v of row) {
        const t = coerceText(v);
        if (isError(t)) return t;
        out += t;
      }
    }
  }
  return out;
}

/* ------------------------------------------------------------------ */
/*  Formato numérico y de fecha para TEXT                              */
/* ------------------------------------------------------------------ */
// ponytail: soporta patrones «0 # ? , . %» y fecha/hora + secciones «;».
// NO soporta notación científica («0.00E+00»), fracciones («# ?/?») ni color
// ([Red]); ampliar aquí si una plantilla real lo pide. VALUE tampoco parsea
// fechas/horas de texto (eso es DATEVALUE/TIMEVALUE en datetime.ts).

const MONTHS_FULL = [
  'January',
  'February',
  'March',
  'April',
  'May',
  'June',
  'July',
  'August',
  'September',
  'October',
  'November',
  'December',
];
const MONTHS_ABBR = [
  'Jan',
  'Feb',
  'Mar',
  'Apr',
  'May',
  'Jun',
  'Jul',
  'Aug',
  'Sep',
  'Oct',
  'Nov',
  'Dec',
];
const DAYS_FULL = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'];
const DAYS_ABBR = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

function pad2(n: number): string {
  return String(n).padStart(2, '0');
}

/** Redondea `n` a `dec` decimales (mitad hacia el infinito, como Excel). `dec`
 *  puede ser negativo (redondea a decenas/centenas). Para DOLLAR/FIXED con
 *  decimales negativos; con dec ≥ 0 el propio patrón (toFixed) ya redondea. */
function roundDec(n: number, dec: number): number {
  const p = Math.pow(10, dec);
  return (Math.sign(n) * Math.round(Math.abs(n) * p)) / p;
}

/** Serial de hoja (1900, base 1899-12-30) → componentes de fecha en UTC. */
function serialToDate(serial: number): {
  Y: number;
  Mon: number;
  Day: number;
  H: number;
  Min: number;
  Sec: number;
  dow: number;
} {
  // 25569 = serial de 1970-01-01. Redondeo a segundos para limpiar el ruido f64.
  const ms = Math.round((serial - 25569) * 86400) * 1000;
  const d = new Date(ms);
  return {
    Y: d.getUTCFullYear(),
    Mon: d.getUTCMonth() + 1,
    Day: d.getUTCDate(),
    H: d.getUTCHours(),
    Min: d.getUTCMinutes(),
    Sec: d.getUTCSeconds(),
    dow: d.getUTCDay(),
  };
}

/** Divide un formato en secciones «;» respetando comillas y escapes. */
function splitSections(f: string): string[] {
  const out: string[] = [];
  let cur = '';
  for (let i = 0; i < f.length; i++) {
    const c = f[i]!;
    if (c === '"') {
      cur += c;
      i++;
      while (i < f.length && f[i] !== '"') cur += f[i++];
      if (i < f.length) cur += '"';
    } else if (c === '\\') {
      cur += c + (f[i + 1] ?? '');
      i++;
    } else if (c === ';') {
      out.push(cur);
      cur = '';
    } else {
      cur += c;
    }
  }
  out.push(cur);
  return out;
}

/** ¿La sección es un formato de fecha/hora? (letras de fecha fuera de comillas). */
function isDateFormat(section: string): boolean {
  let s = '';
  for (let i = 0; i < section.length; i++) {
    const c = section[i]!;
    if (c === '"') {
      i++;
      while (i < section.length && section[i] !== '"') i++;
    } else if (c === '\\') {
      i++;
    } else {
      s += c;
    }
  }
  return /[yYdDhHsSmM]/.test(s);
}

/** Aplica un patrón numérico (solo dígitos «0#?», «.», «,») a un número ≥ 0. */
function applyPattern(n: number, pattern: string): string {
  const dot = pattern.indexOf('.');
  const intPat = dot < 0 ? pattern : pattern.slice(0, dot);
  const fracPat = dot < 0 ? '' : pattern.slice(dot + 1);
  const useGroup = intPat.includes(',');
  const minInt = (intPat.match(/0/g) ?? []).length;
  const fracPlaces = fracPat.match(/[0#?]/g) ?? [];
  const fracLen = fracPlaces.length;

  const fixed = n.toFixed(fracLen);
  const parts = fixed.split('.');
  let ip = parts[0] ?? '';
  const fp = parts[1] ?? '';

  if (ip.length < minInt) ip = '0'.repeat(minInt - ip.length) + ip;
  if (minInt === 0 && ip === '0') ip = ''; // «#» sin parte entera → se omite el 0
  if (useGroup && ip.length > 0) ip = ip.replace(/\B(?=(\d{3})+(?!\d))/g, ',');

  // Recorta los ceros finales que caen sobre un «#» (opcional) de la fracción.
  let keep = fracLen;
  for (let k = fracLen - 1; k >= 0; k--) {
    if (fracPlaces[k] === '#' && fp[k] === '0') keep = k;
    else break;
  }
  const frac = fp.slice(0, keep);
  return keep > 0 ? ip + '.' + frac : ip;
}

/** Formatea una sección numérica: prefijo/sufijo literales + patrón + «%». */
function formatNumericSection(value: number, section: string): string {
  const toks: { t: 'num' | 'lit' | 'pct'; s: string }[] = [];
  for (let i = 0; i < section.length; i++) {
    const c = section[i]!;
    if (c === '"') {
      let lit = '';
      i++;
      while (i < section.length && section[i] !== '"') lit += section[i++];
      toks.push({ t: 'lit', s: lit });
    } else if (c === '\\') {
      toks.push({ t: 'lit', s: section[i + 1] ?? '' });
      i++;
    } else if (c === '%') {
      toks.push({ t: 'pct', s: '%' });
    } else if ('0#?.,'.includes(c)) {
      toks.push({ t: 'num', s: c });
    } else {
      toks.push({ t: 'lit', s: c });
    }
  }

  const percent = toks.filter((t) => t.t === 'pct').length;
  const scaled = value * Math.pow(100, percent);

  let firstNum = -1;
  toks.forEach((t, idx) => {
    if (t.t === 'num' && firstNum < 0) firstNum = idx;
  });

  let prefix = '';
  let suffix = '';
  let pattern = '';
  toks.forEach((t, idx) => {
    if (t.t === 'num') pattern += t.s;
    else if (firstNum < 0 || idx < firstNum) prefix += t.s;
    else suffix += t.s; // literales tras el patrón (incluye «%») → sufijo
  });

  if (firstNum < 0) return prefix + suffix;
  return prefix + applyPattern(scaled, pattern) + suffix;
}

/** Formatea una sección de fecha/hora a partir de un serial. */
function formatDateSection(serial: number, section: string): string {
  const t = serialToDate(serial);
  type Tk = { k: string; len: number; text?: string };
  const toks: Tk[] = [];
  let ampm = false;
  const f = section;
  const low = f.toLowerCase();
  let i = 0;
  while (i < f.length) {
    const c = f[i]!;
    if (c === '"') {
      let lit = '';
      i++;
      while (i < f.length && f[i] !== '"') lit += f[i++];
      i++;
      toks.push({ k: 'lit', len: 0, text: lit });
      continue;
    }
    if (c === '\\') {
      toks.push({ k: 'lit', len: 0, text: f[i + 1] ?? '' });
      i += 2;
      continue;
    }
    const rest = low.slice(i);
    if (rest.startsWith('am/pm')) {
      toks.push({ k: 'ap', len: 5, text: f.slice(i, i + 5) });
      ampm = true;
      i += 5;
      continue;
    }
    if (rest.startsWith('a/p')) {
      toks.push({ k: 'ap', len: 3, text: f.slice(i, i + 3) });
      ampm = true;
      i += 3;
      continue;
    }
    if (rest.startsWith('yyyy')) {
      toks.push({ k: 'y', len: 4 });
      i += 4;
      continue;
    }
    if (rest.startsWith('yy')) {
      toks.push({ k: 'y', len: 2 });
      i += 2;
      continue;
    }
    if (rest.startsWith('mmmmm')) {
      toks.push({ k: 'm', len: 5 });
      i += 5;
      continue;
    }
    if (rest.startsWith('mmmm')) {
      toks.push({ k: 'm', len: 4 });
      i += 4;
      continue;
    }
    if (rest.startsWith('mmm')) {
      toks.push({ k: 'm', len: 3 });
      i += 3;
      continue;
    }
    if (rest.startsWith('mm')) {
      toks.push({ k: 'm', len: 2 });
      i += 2;
      continue;
    }
    if (rest.startsWith('m')) {
      toks.push({ k: 'm', len: 1 });
      i += 1;
      continue;
    }
    if (rest.startsWith('dddd')) {
      toks.push({ k: 'wd', len: 4 });
      i += 4;
      continue;
    }
    if (rest.startsWith('ddd')) {
      toks.push({ k: 'wd', len: 3 });
      i += 3;
      continue;
    }
    if (rest.startsWith('dd')) {
      toks.push({ k: 'd', len: 2 });
      i += 2;
      continue;
    }
    if (rest.startsWith('d')) {
      toks.push({ k: 'd', len: 1 });
      i += 1;
      continue;
    }
    if (rest.startsWith('hh')) {
      toks.push({ k: 'h', len: 2 });
      i += 2;
      continue;
    }
    if (rest.startsWith('h')) {
      toks.push({ k: 'h', len: 1 });
      i += 1;
      continue;
    }
    if (rest.startsWith('ss')) {
      toks.push({ k: 's', len: 2 });
      i += 2;
      continue;
    }
    if (rest.startsWith('s')) {
      toks.push({ k: 's', len: 1 });
      i += 1;
      continue;
    }
    toks.push({ k: 'lit', len: 0, text: c });
    i += 1;
  }

  // Resuelve «m»/«mm»: minutos si va tras «h» o antes de «s»; si no, mes.
  for (let k = 0; k < toks.length; k++) {
    if (toks[k]!.k !== 'm') continue;
    let prev = '';
    for (let p = k - 1; p >= 0; p--)
      if (toks[p]!.k !== 'lit') {
        prev = toks[p]!.k;
        break;
      }
    let next = '';
    for (let p = k + 1; p < toks.length; p++)
      if (toks[p]!.k !== 'lit') {
        next = toks[p]!.k;
        break;
      }
    if (prev === 'h' || next === 's') toks[k]!.k = 'mi';
  }

  const h12 = t.H % 12 || 12;
  const pm = t.H >= 12;
  let out = '';
  for (const tk of toks) {
    switch (tk.k) {
      case 'lit':
        out += tk.text ?? '';
        break;
      case 'y':
        out += tk.len <= 2 ? pad2(t.Y % 100) : String(t.Y);
        break;
      case 'm':
        out +=
          tk.len >= 5
            ? MONTHS_FULL[t.Mon - 1]!.charAt(0)
            : tk.len === 4
              ? MONTHS_FULL[t.Mon - 1]!
              : tk.len === 3
                ? MONTHS_ABBR[t.Mon - 1]!
                : tk.len === 2
                  ? pad2(t.Mon)
                  : String(t.Mon);
        break;
      case 'mi':
        out += tk.len >= 2 ? pad2(t.Min) : String(t.Min);
        break;
      case 'd':
        out += tk.len >= 2 ? pad2(t.Day) : String(t.Day);
        break;
      case 'wd':
        out += tk.len >= 4 ? DAYS_FULL[t.dow]! : DAYS_ABBR[t.dow]!;
        break;
      case 'h': {
        const hv = ampm ? h12 : t.H;
        out += tk.len >= 2 ? pad2(hv) : String(hv);
        break;
      }
      case 's':
        out += tk.len >= 2 ? pad2(t.Sec) : String(t.Sec);
        break;
      case 'ap': {
        const txt = tk.text ?? '';
        const upper = /[A-Z]/.test(txt);
        if (txt.length === 3) out += pm ? (upper ? 'P' : 'p') : upper ? 'A' : 'a';
        else out += pm ? (upper ? 'PM' : 'pm') : upper ? 'AM' : 'am';
        break;
      }
    }
  }
  return out;
}

/** Aplica un formato «TEXT» completo (secciones, signo, número o fecha). */
function formatWithPattern(value: number, fmt: string): string {
  if (fmt === '' || /^\s*general\s*$/i.test(fmt)) return formatNumber(value);
  const sections = splitSections(fmt);
  let section: string;
  let sign = '';
  if (value < 0) {
    value = -value;
    const neg = sections[1];
    if (neg !== undefined && neg.trim() !== '') section = neg;
    else {
      section = sections[0]!;
      sign = '-';
    }
  } else if (value === 0 && sections.length >= 3) {
    section = sections[2]!;
  } else {
    section = sections[0]!;
  }
  if (isDateFormat(section)) return sign + formatDateSection(value, section);
  const body = formatNumericSection(value, section);
  // Excel no imprime «-0»: si la magnitud redondeada no tiene ningún dígito
  // significativo, se omite el signo (TEXT(-0.4,"0")="0", no "-0").
  return sign && /[1-9]/.test(body) ? sign + body : body;
}

/* ------------------------------------------------------------------ */
/*  Búsqueda con comodines para SEARCH                                 */
/* ------------------------------------------------------------------ */

/** Índice 0-based de la primera coincidencia de `pattern` (con «?»/«*»/«~»)
 *  en `text` desde `from`, insensible a mayúsculas; -1 si no aparece. */
function wildcardIndex(pattern: string, text: string, from: number): number {
  if (!/[*?~]/.test(pattern)) {
    return text.toLowerCase().indexOf(pattern.toLowerCase(), from);
  }
  let re = '';
  for (let i = 0; i < pattern.length; i++) {
    const c = pattern[i]!;
    if (c === '~') {
      const n = pattern[i + 1];
      if (n === '*' || n === '?' || n === '~') {
        re += escapeRe(n);
        i++;
      } else re += escapeRe('~');
    } else if (c === '*') re += '[\\s\\S]*';
    else if (c === '?') re += '[\\s\\S]';
    else re += escapeRe(c);
  }
  const rx = new RegExp('^(?:' + re + ')', 'i');
  for (let i = Math.max(0, from); i <= text.length; i++) {
    if (rx.test(text.slice(i))) return i;
  }
  return -1;
}

/* ------------------------------------------------------------------ */
/*  Ayudas para TEXTSPLIT / TEXTBEFORE / TEXTAFTER / ARRAYTOTEXT        */
/* ------------------------------------------------------------------ */

/** Recoge los delimitadores (texto) de un argumento, escalar o matriz, sin vacíos. */
function delimiters(a: Arg): string[] | FormulaError {
  const out: string[] = [];
  for (const row of a.matrix()) {
    for (const v of row) {
      const t = coerceText(v);
      if (isError(t)) return t;
      if (t !== '') out.push(t);
    }
  }
  return out;
}

/** Divide `text` por CUALQUIERA de los delimitadores. `ci` = insensible a caja. */
function splitByAny(text: string, delims: string[], ci: boolean): string[] {
  if (delims.length === 0) return [text];
  const re = new RegExp(delims.map(escapeRe).join('|'), ci ? 'i' : '');
  return text.split(re);
}

/** Índices 0-based de todas las apariciones de `delim` en `text` (según caja). */
function allOccurrences(text: string, delim: string, ci: boolean): number[] {
  const hay = ci ? text.toLowerCase() : text;
  const need = ci ? delim.toLowerCase() : delim;
  const out: number[] = [];
  let from = 0;
  for (;;) {
    const i = hay.indexOf(need, from);
    if (i < 0) break;
    out.push(i);
    from = i + need.length;
  }
  return out;
}

/* ------------------------------------------------------------------ */
/*  Tabla de funciones                                                 */
/* ------------------------------------------------------------------ */

/** Caracteres de ancho completo (U+FF01–FF5E, U+3000) → ancho medio. */
function anchoMedio(t: string): string {
  let out = '';
  for (const ch of t) {
    const c = ch.codePointAt(0)!;
    out += c >= 0xff01 && c <= 0xff5e ? String.fromCodePoint(c - 0xfee0) : c === 0x3000 ? ' ' : ch;
  }
  return out;
}

/** ASCII imprimible (0x20–0x7E) → ancho completo. */
function anchoCompleto(t: string): string {
  let out = '';
  for (const ch of t) {
    const c = ch.codePointAt(0)!;
    out += c >= 0x21 && c <= 0x7e ? String.fromCodePoint(c + 0xfee0) : c === 0x20 ? '\u3000' : ch;
  }
  return out;
}

const TAI_CIFRA = ['ศูนย์', 'หนึ่ง', 'สอง', 'สาม', 'สี่', 'ห้า', 'หก', 'เจ็ด', 'แปด', 'เก้า'];
const TAI_LUGAR = ['', 'สิบ', 'ร้อย', 'พัน', 'หมื่น', 'แสน'];

/** Un grupo de hasta seis cifras (< 1.000.000) en tailandés. `hayAntes`: hay millones delante. */
function grupoTailandes(g: number, hayAntes: boolean): string {
  let out = '';
  const ds = String(g).padStart(6, '0');
  for (let i = 0; i < 6; i++) {
    const d = Number(ds[i]);
    const lugar = 5 - i;
    if (d === 0) continue;
    if (lugar === 1) out += d === 1 ? 'สิบ' : d === 2 ? 'ยี่สิบ' : TAI_CIFRA[d]! + 'สิบ';
    else if (lugar === 0 && d === 1 && (g >= 10 || hayAntes)) out += 'เอ็ด';
    else out += TAI_CIFRA[d]! + TAI_LUGAR[lugar]!;
  }
  return out;
}

/** Un entero no negativo en tailandés: los millones se encadenan con «ล้าน». */
function enteroTailandes(n: number): string {
  if (n === 0) return TAI_CIFRA[0]!;
  const millones = Math.floor(n / 1e6);
  const resto = n % 1e6;
  return (
    (millones > 0 ? enteroTailandes(millones) + 'ล้าน' : '') +
    (resto > 0 ? grupoTailandes(resto, millones > 0) : '')
  );
}

/** BAHTTEXT: «ลบ» si es negativo, los baht + «บาท», y los satang + «สตางค์» o «ถ้วน» si no hay. */
function bahtText(n: number): string | FormulaError {
  const cent = Math.round(Math.abs(n) * 100);
  if (!Number.isFinite(cent) || cent >= 1e15) return ERR.VALUE;
  const baht = Math.floor(cent / 100);
  const satang = cent % 100;
  let out = n < 0 && cent > 0 ? 'ลบ' : '';
  if (baht > 0 || satang === 0) out += enteroTailandes(baht) + 'บาท';
  out += satang === 0 ? 'ถ้วน' : grupoTailandes(satang, false) + 'สตางค์';
  return out;
}

export const funcs: FnTable = {
  CONCAT(_ctx, args) {
    return concatImpl(args);
  },
  // CONCAT y CONCATENATE se comportan igual en las hojas modernas (unen rangos).
  CONCATENATE(_ctx, args) {
    return concatImpl(args);
  },

  LEFT(_ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const t = textArg(args[0]!);
    if (isError(t)) return t;
    const n = args.length > 1 ? intArg(args[1]!) : 1;
    if (isError(n)) return n;
    if (n < 0) return ERR.VALUE;
    return t.slice(0, n);
  },

  RIGHT(_ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const t = textArg(args[0]!);
    if (isError(t)) return t;
    const n = args.length > 1 ? intArg(args[1]!) : 1;
    if (isError(n)) return n;
    if (n < 0) return ERR.VALUE;
    return t.slice(Math.max(0, t.length - n));
  },

  MID(_ctx, args) {
    if (args.length !== 3) return ERR.VALUE;
    const t = textArg(args[0]!);
    if (isError(t)) return t;
    const start = intArg(args[1]!);
    if (isError(start)) return start;
    const num = intArg(args[2]!);
    if (isError(num)) return num;
    if (start < 1 || num < 0) return ERR.VALUE;
    return t.slice(start - 1, start - 1 + num);
  },

  LEN(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = textArg(args[0]!);
    return isError(t) ? t : t.length;
  },

  LOWER(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = textArg(args[0]!);
    return isError(t) ? t : t.toLowerCase();
  },

  UPPER(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = textArg(args[0]!);
    return isError(t) ? t : t.toUpperCase();
  },

  PROPER(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = textArg(args[0]!);
    if (isError(t)) return t;
    let out = '';
    let prevLetter = false;
    for (const ch of t) {
      if (/\p{L}/u.test(ch)) {
        out += prevLetter ? ch.toLowerCase() : ch.toUpperCase();
        prevLetter = true;
      } else {
        out += ch;
        prevLetter = false;
      }
    }
    return out;
  },

  TRIM(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = textArg(args[0]!);
    if (isError(t)) return t;
    // Solo el espacio ASCII: colapsa runs y recorta extremos (como Excel).
    return t
      .split(' ')
      .filter((w) => w !== '')
      .join(' ');
  },

  REPLACE(_ctx, args) {
    if (args.length !== 4) return ERR.VALUE;
    const old = textArg(args[0]!);
    if (isError(old)) return old;
    const start = intArg(args[1]!);
    if (isError(start)) return start;
    const num = intArg(args[2]!);
    if (isError(num)) return num;
    const rep = textArg(args[3]!);
    if (isError(rep)) return rep;
    if (start < 1 || num < 0) return ERR.VALUE;
    return old.slice(0, start - 1) + rep + old.slice(start - 1 + num);
  },

  SUBSTITUTE(_ctx, args) {
    if (args.length < 3 || args.length > 4) return ERR.VALUE;
    const src = textArg(args[0]!);
    if (isError(src)) return src;
    const oldS = textArg(args[1]!);
    if (isError(oldS)) return oldS;
    const newS = textArg(args[2]!);
    if (isError(newS)) return newS;
    if (oldS === '') return src; // buscar "" no cambia nada
    if (args.length === 4) {
      const inst = intArg(args[3]!);
      if (isError(inst)) return inst;
      if (inst < 1) return ERR.VALUE;
      let count = 0;
      let from = 0;
      for (;;) {
        const idx = src.indexOf(oldS, from);
        if (idx < 0) return src; // no hay tantas ocurrencias → sin cambios
        count++;
        if (count === inst) return src.slice(0, idx) + newS + src.slice(idx + oldS.length);
        from = idx + oldS.length;
      }
    }
    return src.split(oldS).join(newS);
  },

  FIND(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const find = textArg(args[0]!);
    if (isError(find)) return find;
    const within = textArg(args[1]!);
    if (isError(within)) return within;
    const start = args.length > 2 ? intArg(args[2]!) : 1;
    if (isError(start)) return start;
    if (start < 1 || start > within.length + 1) return ERR.VALUE;
    const idx = within.indexOf(find, start - 1);
    return idx < 0 ? ERR.VALUE : idx + 1;
  },

  SEARCH(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const find = textArg(args[0]!);
    if (isError(find)) return find;
    const within = textArg(args[1]!);
    if (isError(within)) return within;
    const start = args.length > 2 ? intArg(args[2]!) : 1;
    if (isError(start)) return start;
    if (start < 1 || start > within.length + 1) return ERR.VALUE;
    const idx = wildcardIndex(find, within, start - 1);
    return idx < 0 ? ERR.VALUE : idx + 1;
  },

  TEXT(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const raw = args[0]!.value();
    if (isError(raw)) return raw;
    const fmt = coerceText(args[1]!.value());
    if (isError(fmt)) return fmt;
    const n = coerceNumber(raw);
    if (isError(n)) return coerceText(raw); // valor de texto → se devuelve tal cual
    return formatWithPattern(n, fmt);
  },

  VALUE(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const v = args[0]!.value();
    if (isError(v)) return v;
    if (typeof v === 'number') return v;
    if (typeof v === 'boolean') return ERR.VALUE; // VALUE no acepta booleanos
    let s = v.trim();
    if (s === '') return ERR.VALUE; // ponytail: VALUE("")=#VALUE!; celda en blanco es otro caso
    let pct = 0;
    while (s.endsWith('%')) {
      pct++;
      s = s.slice(0, -1).trim();
    }
    s = s.replace(/^[\s$€£¥]+/, ''); // símbolo de moneda inicial
    s = s.replace(/,/g, ''); // separadores de miles
    if (!/^[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?$/.test(s)) return ERR.VALUE;
    return Number(s) / Math.pow(100, pct);
  },

  NUMBERVALUE(_ctx, args) {
    if (args.length < 1 || args.length > 3) return ERR.VALUE;
    const tv = textArg(args[0]!);
    if (isError(tv)) return tv;
    let decSep = '.';
    let grpSep = ',';
    if (args.length >= 2) {
      const d = textArg(args[1]!);
      if (isError(d)) return d;
      if (d.length > 0) decSep = d[0]!;
    }
    if (args.length >= 3) {
      const g = textArg(args[2]!);
      if (isError(g)) return g;
      if (g.length > 0) grpSep = g[0]!;
    }
    if (decSep === grpSep) return ERR.VALUE;
    let s = tv.replace(/\s+/g, ''); // NUMBERVALUE ignora los espacios
    if (s === '') return 0; // NUMBERVALUE("")=0
    let pct = 0;
    while (s.endsWith('%')) {
      pct++;
      s = s.slice(0, -1);
    }
    s = s.split(grpSep).join(''); // quita separadores de miles
    s = s.split(decSep).join('.'); // separador decimal → «.»
    if (!/^[-+]?(\d+(\.\d*)?|\.\d+)$/.test(s)) return ERR.VALUE;
    return Number(s) / Math.pow(100, pct);
  },

  REPT(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const t = textArg(args[0]!);
    if (isError(t)) return t;
    const times = intArg(args[1]!);
    if (isError(times)) return times;
    if (times < 0) return ERR.VALUE;
    if (t.length * times > 32767) return ERR.VALUE; // límite de celda
    return t.repeat(times);
  },

  TEXTJOIN(_ctx, args) {
    if (args.length < 3) return ERR.VALUE;
    const delim = textArg(args[0]!);
    if (isError(delim)) return delim;
    const ignoreEmpty = coerceBoolean(args[1]!.value());
    if (isError(ignoreEmpty)) return ignoreEmpty;
    const parts: string[] = [];
    for (let a = 2; a < args.length; a++) {
      for (const row of args[a]!.matrix()) {
        for (const v of row) {
          const t = coerceText(v);
          if (isError(t)) return t;
          if (ignoreEmpty && t === '') continue;
          parts.push(t);
        }
      }
    }
    return parts.join(delim);
  },

  CHAR(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const n = intArg(args[0]!);
    if (isError(n)) return n;
    if (n < 1 || n > 255) return ERR.VALUE;
    return String.fromCharCode(n);
  },

  CODE(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = textArg(args[0]!);
    if (isError(t)) return t;
    if (t === '') return ERR.VALUE;
    return t.charCodeAt(0);
  },

  UNICHAR(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const n = intArg(args[0]!);
    if (isError(n)) return n;
    if (n < 1 || n > 0x10ffff) return ERR.VALUE;
    try {
      return String.fromCodePoint(n);
    } catch {
      return ERR.VALUE;
    }
  },

  UNICODE(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = textArg(args[0]!);
    if (isError(t)) return t;
    if (t === '') return ERR.VALUE;
    return t.codePointAt(0)!;
  },

  EXACT(_ctx, args) {
    if (args.length !== 2) return ERR.VALUE;
    const a = textArg(args[0]!);
    if (isError(a)) return a;
    const b = textArg(args[1]!);
    if (isError(b)) return b;
    return a === b;
  },

  CLEAN(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = textArg(args[0]!);
    if (isError(t)) return t;
    // Quita los caracteres de control ASCII (0–31), como CLEAN clásico.
    // eslint-disable-next-line no-control-regex -- eliminación deliberada de caracteres de control
    return t.replace(/[\x00-\x1f]/g, '');
  },

  T(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const v = args[0]!.value();
    if (isError(v)) return v;
    return typeof v === 'string' ? v : '';
  },

  // Las funciones «B» cuentan BYTES en los idiomas de doble byte (japonés, chino, coreano);
  // en el resto de idiomas de Excel son LEFT, RIGHT, MID, LEN, FIND, SEARCH y REPLACE. Calcus
  // no tiene idioma de doble byte, así que son esas siete.
  LEFTB: (ctx, args) => funcs['LEFT']!(ctx, args),
  RIGHTB: (ctx, args) => funcs['RIGHT']!(ctx, args),
  MIDB: (ctx, args) => funcs['MID']!(ctx, args),
  LENB: (ctx, args) => funcs['LEN']!(ctx, args),
  FINDB: (ctx, args) => funcs['FIND']!(ctx, args),
  SEARCHB: (ctx, args) => funcs['SEARCH']!(ctx, args),
  REPLACEB: (ctx, args) => funcs['REPLACE']!(ctx, args),

  // ASC(texto) pasa a ANCHO MEDIO los caracteres de ancho completo (U+FF01–FF5E y el
  // espacio ideográfico). DBCS y JIS hacen el camino contrario. Solo el bloque ASCII: los
  // katakana de ancho medio no se convierten. (Excel en un idioma sin doble byte devuelve
  // el texto tal cual; LibreOffice convierte siempre, y aquí se hace como LibreOffice.)
  ASC(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = textArg(args[0]!);
    return isError(t) ? t : anchoMedio(t);
  },
  DBCS(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = textArg(args[0]!);
    return isError(t) ? t : anchoCompleto(t);
  },
  JIS(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = textArg(args[0]!);
    return isError(t) ? t : anchoCompleto(t);
  },

  // PHONETIC(ref): la lectura fonética de un texto japonés. Sin editor de métodos de
  // entrada no hay furigana guardado, y Excel devuelve entonces el propio texto.
  PHONETIC(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    return textArg(args[0]!);
  },

  // ENCODEURL(texto): codifica en porcentaje los bytes UTF-8 de todo salvo A-Z a-z 0-9 - . _ ~.
  ENCODEURL(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const t = textArg(args[0]!);
    if (isError(t)) return t;
    return encodeURIComponent(t).replace(
      /[!'()*]/g,
      (c) => '%' + c.charCodeAt(0).toString(16).toUpperCase(),
    );
  },

  // BAHTTEXT(número): el importe escrito en tailandés, en baht y satang.
  BAHTTEXT(_ctx, args) {
    if (args.length !== 1) return ERR.VALUE;
    const n = coerceNumber(args[0]!.value());
    if (isError(n)) return n;
    return bahtText(n);
  },

  // TEXTSPLIT(texto, delim_col, [delim_fila], [ignorar_vacios=FALSE],
  // [modo=0], [relleno=#N/A]): parte el texto en una MATRIZ. `delim_col` separa
  // columnas; `delim_fila`, filas. Rellena filas irregulares con `relleno`.
  TEXTSPLIT(_ctx, args) {
    if (args.length < 2 || args.length > 6) return ERR.VALUE;
    const text = textArg(args[0]!);
    if (isError(text)) return text;
    const colD = delimiters(args[1]!);
    if (isErr(colD)) return colD;
    let rowD: string[] = [];
    if (args.length >= 3) {
      const r = delimiters(args[2]!);
      if (isErr(r)) return r;
      rowD = r;
    }
    if (colD.length === 0 && rowD.length === 0) return ERR.VALUE; // sin delimitadores
    let ignoreEmpty = false;
    if (args.length >= 4) {
      const b = coerceBoolean(args[3]!.value());
      if (isError(b)) return b;
      ignoreEmpty = b;
    }
    let ci = false;
    if (args.length >= 5) {
      const mm = coerceNumber(args[4]!.value());
      if (isError(mm)) return mm;
      ci = mm === 1;
    }
    const pad: CellValue = args.length >= 6 ? args[5]!.value() : ERR.NA;
    if (isError(pad) && args.length >= 6) return pad;

    const rawRows = rowD.length ? splitByAny(text, rowD, ci) : [text];
    let grid = rawRows
      .map((r) => (colD.length ? splitByAny(r, colD, ci) : [r]))
      .map((cells) => (ignoreEmpty ? cells.filter((c) => c !== '') : cells))
      .filter((cells) => !(ignoreEmpty && cells.length === 0));
    if (grid.length === 0) grid = [['']];
    const width = Math.max(...grid.map((r) => r.length));
    return grid.map((r) => {
      const out: CellValue[] = r.slice();
      while (out.length < width) out.push(pad);
      return out;
    });
  },

  // TEXTBEFORE(texto, delim, [instancia=1], [modo=0], [_fin], [si_no_hay]):
  // el texto ANTES de la `instancia`-ésima aparición de `delim` (negativa =
  // desde el final).
  TEXTBEFORE(_ctx, args) {
    return textBeforeAfter(args, true);
  },
  // TEXTAFTER(texto, delim, [instancia=1], [modo=0], [_fin], [si_no_hay]): el
  // texto DESPUÉS de la aparición indicada.
  TEXTAFTER(_ctx, args) {
    return textBeforeAfter(args, false);
  },

  // ARRAYTOTEXT(array, [formato=0]): 0 conciso («a, b, c»); 1 estricto
  // («{"a";"b"}» con comillas en el texto, «,» entre columnas y «;» entre filas).
  ARRAYTOTEXT(_ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const m = args[0]!.matrix();
    let strict = false;
    if (args.length === 2) {
      const f = coerceNumber(args[1]!.value());
      if (isError(f)) return f;
      strict = f === 1;
    }
    if (!strict) {
      const parts: string[] = [];
      for (const row of m)
        for (const v of row) {
          const t = coerceText(v);
          if (isError(t)) return t;
          parts.push(t);
        }
      return parts.join(', ');
    }
    const rows = m.map((row) =>
      row
        .map((v) => (typeof v === 'string' ? `"${v}"` : coerceText(v)))
        .map((t) => (isError(t) ? '#ERR' : t))
        .join(','),
    );
    return '{' + rows.join(';') + '}';
  },

  // VALUETOTEXT(valor, [formato=0]): texto de un ÚNICO valor. 0 conciso (el texto
  // tal cual); 1 estricto (el texto entre comillas). Un error se convierte en su
  // código («#N/A»…), no se propaga.
  VALUETOTEXT(_ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const v = args[0]!.value();
    let strict = false;
    if (args.length === 2) {
      const f = coerceNumber(args[1]!.value());
      if (isError(f)) return f;
      strict = f === 1;
    }
    if (isError(v)) return v.code;
    if (typeof v === 'string') return strict ? `"${v}"` : v;
    return coerceText(v); // número → General; booleano → TRUE/FALSE
  },

  // DOLLAR(numero, [decimales=2]): moneda con separador de miles y símbolo «$»;
  // negativos entre paréntesis. `decimales` negativos redondean a la izquierda.
  DOLLAR(_ctx, args) {
    if (args.length < 1 || args.length > 2) return ERR.VALUE;
    const n = coerceNumber(args[0]!.value());
    if (isError(n)) return n;
    let dec = 2;
    if (args.length === 2) {
      const d = coerceNumber(args[1]!.value());
      if (isError(d)) return d;
      dec = Math.trunc(d);
    }
    const places = Math.max(dec, 0);
    const value = dec < 0 ? roundDec(n, dec) : n;
    const body = places > 0 ? '.' + '0'.repeat(places) : '';
    return formatWithPattern(value, `"$"#,##0${body};("$"#,##0${body})`);
  },

  // FIXED(numero, [decimales=2], [sin_comas=FALSE]): número con `decimales` fijos;
  // negativos con signo menos. `sin_comas` TRUE quita el separador de miles.
  FIXED(_ctx, args) {
    if (args.length < 1 || args.length > 3) return ERR.VALUE;
    const n = coerceNumber(args[0]!.value());
    if (isError(n)) return n;
    let dec = 2;
    if (args.length >= 2) {
      const d = coerceNumber(args[1]!.value());
      if (isError(d)) return d;
      dec = Math.trunc(d);
    }
    let noCommas = false;
    if (args.length >= 3) {
      const b = coerceBoolean(args[2]!.value());
      if (isError(b)) return b;
      noCommas = b;
    }
    const places = Math.max(dec, 0);
    const value = dec < 0 ? roundDec(n, dec) : n;
    const body = places > 0 ? '.' + '0'.repeat(places) : '';
    return formatWithPattern(value, (noCommas ? '0' : '#,##0') + body);
  },

  // REGEXTEST(texto, patrón, [case=0]): ¿el patrón encuentra alguna coincidencia?
  REGEXTEST(_ctx, args) {
    if (args.length < 2 || args.length > 3) return ERR.VALUE;
    const t = textArg(args[0]!);
    if (isError(t)) return t;
    const pat = textArg(args[1]!);
    if (isError(pat)) return pat;
    const ci = caseInsensitive(args, 2);
    if (isError(ci)) return ci;
    const re = buildRegex(pat, t, ci);
    if (isErr(re)) return re;
    return conPresupuesto(() => re.buscar(0) !== null);
  },

  // REGEXEXTRACT(texto, patrón, [modo=0], [case=0]): modo 0 → primera coincidencia
  // (escalar); 1 → todas (derrama en columna); 2 → grupos de captura de la 1ª
  // coincidencia (derrama en fila). Sin coincidencia → #N/A.
  REGEXEXTRACT(_ctx, args) {
    if (args.length < 2 || args.length > 4) return ERR.VALUE;
    const t = textArg(args[0]!);
    if (isError(t)) return t;
    const pat = textArg(args[1]!);
    if (isError(pat)) return pat;
    const mode = args.length >= 3 ? intArg(args[2]!) : 0;
    if (isError(mode)) return mode;
    const ci = caseInsensitive(args, 3);
    if (isError(ci)) return ci;
    const re = buildRegex(pat, t, ci);
    if (isErr(re)) return re;
    return conPresupuesto(() => {
      if (mode === 1) {
        const out: CellValue[][] = re.todas().map((m) => [grupo(t, m, 0)]);
        return out.length === 0 ? ERR.NA : out;
      }
      const m = re.buscar(0);
      if (!m) return ERR.NA;
      if (mode === 2) {
        const grupos = m.length / 2 - 1;
        if (grupos === 0) return grupo(t, m, 0);
        return [Array.from({ length: grupos }, (_, g) => grupo(t, m, g + 1))];
      }
      if (mode !== 0) return ERR.VALUE;
      return grupo(t, m, 0);
    });
  },

  // REGEXREPLACE(texto, patrón, reemplazo, [ocurrencia=0], [case=0]): ocurrencia
  // 0 sustituye todas; n>0 solo la n-ésima. Admite $&, $$ y $1…$99 en el reemplazo.
  REGEXREPLACE(_ctx, args) {
    if (args.length < 3 || args.length > 5) return ERR.VALUE;
    const t = textArg(args[0]!);
    if (isError(t)) return t;
    const pat = textArg(args[1]!);
    if (isError(pat)) return pat;
    const rep = textArg(args[2]!);
    if (isError(rep)) return rep;
    const occ = args.length >= 4 ? intArg(args[3]!) : 0;
    if (isError(occ)) return occ;
    if (occ < 0) return ERR.VALUE;
    const ci = caseInsensitive(args, 4);
    if (isError(ci)) return ci;
    const re = buildRegex(pat, t, ci);
    if (isErr(re)) return re;
    return conPresupuesto(() => {
      const todas = re.todas();
      if (occ > 0) {
        const m = todas[occ - 1];
        if (!m) return t; // menos de `occ` coincidencias: sin cambios
        return t.slice(0, m[0]) + expandRepl(rep, t, m) + t.slice(m[1]);
      }
      let out = '';
      let fin = 0;
      for (const m of todas) {
        out += t.slice(fin, m[0]) + sustitucion(rep, t, m);
        fin = m[1]!;
      }
      return out + t.slice(fin);
    });
  },
};

/** Núcleo de TEXTBEFORE/TEXTAFTER. `before` elige el lado del corte. */
function textBeforeAfter(args: Arg[], before: boolean): CellValue {
  if (args.length < 2 || args.length > 6) return ERR.VALUE;
  const text = textArg(args[0]!);
  if (isError(text)) return text;
  const delim = textArg(args[1]!);
  if (isError(delim)) return delim;
  if (delim === '') return ERR.VALUE;
  let inst = 1;
  if (args.length >= 3) {
    const n = intArg(args[2]!);
    if (isError(n)) return n;
    inst = n;
  }
  if (inst === 0) return ERR.VALUE;
  let ci = false;
  if (args.length >= 4) {
    const mm = coerceNumber(args[3]!.value());
    if (isError(mm)) return mm;
    ci = mm === 1;
  }
  const ifNotFound: CellValue | undefined = args.length >= 6 ? args[5]!.value() : undefined;

  const occ = allOccurrences(text, delim, ci);
  const pick = inst > 0 ? inst - 1 : occ.length + inst; // negativa: desde el final
  if (pick < 0 || pick >= occ.length) return ifNotFound ?? ERR.NA;
  const at = occ[pick]!;
  return before ? text.slice(0, at) : text.slice(at + delim.length);
}
