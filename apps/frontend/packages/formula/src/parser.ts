/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Tokenizador + parser de descenso recursivo de expresiones de fórmula (lo que
 * va tras el «=»). Semántica de precedencia ECMA-376 (la negación unaria liga
 * más fuerte que «^», por eso -2^2 = 4). Código original (clean-room).
 */
import { ERR, type CellValue, type FormulaError } from './values.js';

/** Referencia a una celda. `col`/`row` son 0-based. `sheet` = undefined si no cualifica hoja. */
export interface CellRef {
  sheet: string | undefined;
  col: number;
  row: number;
  colAbs: boolean;
  rowAbs: boolean;
}

export type BinOp = '+' | '-' | '*' | '/' | '^' | '&' | '=' | '<>' | '<' | '>' | '<=' | '>=';

/**
 * Área de una referencia estructurada de tabla (`Tabla[#…]`): el cuerpo de datos
 * (por defecto), todo (cabeceras+datos+totales), solo cabeceras o la fila de
 * totales. Es sintáctica: el resolutor de tablas del editor la traduce a filas.
 */
export type TableArea = 'data' | 'all' | 'headers' | 'totals';

/** Nodo del AST tipado. */
export type AstNode =
  | { kind: 'num'; value: number }
  | { kind: 'str'; value: string }
  | { kind: 'bool'; value: boolean }
  // Constante matricial {1,2;3,4}: filas separadas por «;» y columnas por «,». Solo literales.
  | { kind: 'array'; rows: CellValue[][] }
  | { kind: 'ref'; ref: CellRef }
  // Rango de celdas (A1:B2) o de EJE ENTERO (A:A, A:C, 1:1, 3:5). `whole`:
  //  - undefined → rango de celdas normal; `a`/`b` son celdas completas.
  //  - 'col'      → columna(s) entera(s); solo `a.col..b.col` cuentan (las filas
  //                 se acotan al rango usado de la hoja al evaluar; `row` = 0).
  //  - 'row'      → fila(s) entera(s); solo `a.row..b.row` cuentan (`col` = 0).
  // Los extremos de eje entero se normalizan al parsear (C:A → A:C, 5:3 → 3:5).
  | { kind: 'range'; a: CellRef; b: CellRef; whole?: 'col' | 'row' }
  | { kind: 'name'; name: string; sheet: string | undefined } // nombre no soportado → #NAME? al evaluar
  // Referencia estructurada `Tabla[Columna]` (paridad Excel/Sheets). `table` = ''
  // si la tabla es implícita (la que contiene la celda). `column` = undefined si
  // solo se pide un área (p. ej. `Tabla[#Todo]`). El motor la resuelve a un
  // RefRange vía el resolutor inyectado; #REF!/#NAME? si no existe.
  | {
      kind: 'structured';
      table: string;
      column: string | undefined;
      area: TableArea;
      thisRow: boolean;
    }
  | { kind: 'unary'; op: '-' | '+'; operand: AstNode }
  | { kind: 'percent'; operand: AstNode }
  | { kind: 'binary'; op: BinOp; left: AstNode; right: AstNode }
  | { kind: 'call'; name: string; args: AstNode[] }
  // Argumento OMITIDO entre separadores: IF(A1,,2), GROUPBY(A:A,B:B,SUM,,0). Para quien lo
  // recibe es un valor en blanco; GROUPBY/PIVOTBY y los LAMBDA con parámetros opcionales
  // lo distinguen de un argumento vacío de verdad (ISOMITTED).
  | { kind: 'empty' };

export class ParseError extends Error {}

/* ---------- Utilidades de coordenadas A1 ---------- */

/** «A»→0, «Z»→25, «AA»→26… (columna a índice 0-based). */
export function colToNum(letters: string): number {
  let n = 0;
  const up = letters.toUpperCase();
  for (let i = 0; i < up.length; i++) n = n * 26 + (up.charCodeAt(i) - 64);
  return n - 1;
}

/** 0→«A», 25→«Z», 26→«AA»… (índice 0-based a columna). */
export function numToCol(col: number): string {
  let n = col + 1;
  let out = '';
  while (n > 0) {
    const r = (n - 1) % 26;
    out = String.fromCharCode(65 + r) + out;
    n = Math.floor((n - 1) / 26);
  }
  return out;
}

const CELL_RE = /^(\$?)([A-Za-z]{1,3})(\$?)([0-9]+)$/;
const COL_RE = /^(\$?)([A-Za-z]{1,3})$/; // columna sola: A, $A, AB…
const ROW_RE = /^(\$?)([0-9]{1,7})$/; // fila sola: 1, $1, 1048576…

/** Parsea «A1», «$A$1», «B$3»… a CellRef (sin hoja). Lanza si no encaja. */
export function parseCellWord(w: string, sheet: string | undefined): CellRef {
  const m = CELL_RE.exec(w);
  if (!m) throw new ParseError(`referencia inválida: ${w}`);
  return {
    sheet,
    col: colToNum(m[2]!),
    row: Number(m[4]) - 1,
    colAbs: m[1] === '$',
    rowAbs: m[3] === '$',
  };
}

/** Tipo de extremo de referencia: celda completa (A1), columna sola (A/$A) o
 *  fila sola (1/$1). */
export type RefEndKind = 'cell' | 'col' | 'row';

/** Un extremo de referencia ya clasificado. Para 'col' `row`=0 (irrelevante);
 *  para 'row' `col`=0. El `whole` del nodo de rango dice qué eje manda. */
export interface RefEnd {
  kind: RefEndKind;
  col: number;
  row: number;
  colAbs: boolean;
  rowAbs: boolean;
}

/**
 * Clasifica una palabra/token como extremo de referencia: celda (A1/$A$1),
 * columna entera (A/$A) o fila entera (1/$1); null si no es ninguna. Es la ÚNICA
 * fuente de verdad del reconocimiento de referencias (la reutilizan el parser y
 * el editor, para no repetir regex de refs a mano). Una fila desnuda llega como
 * el texto del token numérico (p. ej. «3»).
 */
export function parseRefEnd(w: string): RefEnd | null {
  const cell = CELL_RE.exec(w);
  if (cell) {
    return {
      kind: 'cell',
      col: colToNum(cell[2]!),
      row: Number(cell[4]) - 1,
      colAbs: cell[1] === '$',
      rowAbs: cell[3] === '$',
    };
  }
  const col = COL_RE.exec(w);
  if (col)
    return { kind: 'col', col: colToNum(col[2]!), row: 0, colAbs: col[1] === '$', rowAbs: false };
  const row = ROW_RE.exec(w);
  if (row)
    return { kind: 'row', col: 0, row: Number(row[2]) - 1, colAbs: false, rowAbs: row[1] === '$' };
  return null;
}

/** Construye un CellRef a partir de un extremo clasificado y una hoja. */
function endRef(e: RefEnd, sheet: string | undefined): CellRef {
  return { sheet, col: e.col, row: e.row, colAbs: e.colAbs, rowAbs: e.rowAbs };
}

/* ---------- Tokenizador ---------- */

interface Tok {
  t:
    | 'num'
    | 'str'
    | 'word'
    | 'qsheet'
    | 'op'
    | 'lparen'
    | 'rparen'
    | 'sep'
    | 'colon'
    | 'bang'
    | 'percent'
    | 'bracket'
    | 'array';
  v: string; // en 'bracket': el texto interior crudo (sin los corchetes exteriores)
}

const WORD_START = /[A-Za-z_$]/;
const WORD_REST = /[A-Za-z0-9_$.]/;
const DIGIT = /[0-9]/;

function lex(s: string): Tok[] {
  const toks: Tok[] = [];
  const n = s.length;
  let i = 0;
  while (i < n) {
    const c = s[i]!;
    if (c === ' ' || c === '\t' || c === '\n' || c === '\r') {
      i++;
      continue;
    }
    if (DIGIT.test(c) || (c === '.' && DIGIT.test(s[i + 1] ?? ''))) {
      let j = i + 1;
      while (j < n && /[0-9.]/.test(s[j]!)) j++;
      if (s[j] === 'e' || s[j] === 'E') {
        j++;
        if (s[j] === '+' || s[j] === '-') j++;
        while (j < n && DIGIT.test(s[j]!)) j++;
      }
      toks.push({ t: 'num', v: s.slice(i, j) });
      i = j;
      continue;
    }
    if (c === '"') {
      let j = i + 1;
      let out = '';
      while (j < n) {
        if (s[j] === '"') {
          if (s[j + 1] === '"') {
            out += '"';
            j += 2;
            continue;
          }
          j++;
          break;
        }
        out += s[j];
        j++;
      }
      toks.push({ t: 'str', v: out });
      i = j;
      continue;
    }
    if (c === "'") {
      let j = i + 1;
      let out = '';
      while (j < n) {
        if (s[j] === "'") {
          if (s[j + 1] === "'") {
            out += "'";
            j += 2;
            continue;
          }
          j++;
          break;
        }
        out += s[j];
        j++;
      }
      toks.push({ t: 'qsheet', v: out });
      i = j;
      continue;
    }
    const two = s.slice(i, i + 2);
    if (two === '<=' || two === '>=' || two === '<>') {
      toks.push({ t: 'op', v: two });
      i += 2;
      continue;
    }
    if ('+-*/^&=<>'.includes(c)) {
      toks.push({ t: 'op', v: c });
      i++;
      continue;
    }
    if (c === '[') {
      // Región de corchetes de una referencia estructurada `Tabla[…]`. Captura
      // la zona balanceada (admite un nivel anidado `[[#Datos],[Col]]`) como un
      // solo token con el texto interior crudo; el parser lo interpreta luego.
      // El apóstrofo escapa el siguiente carácter (así `'[`/`']` no descuadran).
      let depth = 1;
      let j = i + 1;
      let out = '';
      while (j < n && depth > 0) {
        const ch = s[j]!;
        if (ch === "'") {
          out += ch + (s[j + 1] ?? '');
          j += 2;
          continue;
        }
        if (ch === '[') depth++;
        else if (ch === ']') {
          depth--;
          if (depth === 0) {
            j++;
            break;
          }
        }
        out += ch;
        j++;
      }
      if (depth !== 0) throw new ParseError('falta «]» en la referencia de tabla');
      toks.push({ t: 'bracket', v: out });
      i = j;
      continue;
    }
    if (c === '{') {
      // Constante matricial: se captura entera (respetando los textos entre comillas) como un
      // solo token con su interior crudo; el parser lo lee con `parseArrayLiteral`.
      let j = i + 1;
      let enTexto = false;
      while (j < n && (enTexto || s[j] !== '}')) {
        if (s[j] === '"') enTexto = !enTexto;
        j++;
      }
      if (j >= n) throw new ParseError('falta «}» en la constante matricial');
      toks.push({ t: 'array', v: s.slice(i + 1, j) });
      i = j + 1;
      continue;
    }
    if (c === '(') {
      toks.push({ t: 'lparen', v: c });
      i++;
      continue;
    }
    if (c === ')') {
      toks.push({ t: 'rparen', v: c });
      i++;
      continue;
    }
    if (c === ',' || c === ';') {
      toks.push({ t: 'sep', v: c });
      i++;
      continue;
    }
    if (c === ':') {
      toks.push({ t: 'colon', v: c });
      i++;
      continue;
    }
    if (c === '!') {
      toks.push({ t: 'bang', v: c });
      i++;
      continue;
    }
    if (c === '%') {
      toks.push({ t: 'percent', v: c });
      i++;
      continue;
    }
    if (WORD_START.test(c)) {
      let j = i + 1;
      while (j < n && WORD_REST.test(s[j]!)) j++;
      toks.push({ t: 'word', v: s.slice(i, j) });
      i = j;
      continue;
    }
    throw new ParseError(`carácter inesperado: «${c}»`);
  }
  return toks;
}

/** Los errores que se pueden escribir como elemento de una constante matricial. */
const ERRORES_LITERALES: Record<string, FormulaError> = {
  '#DIV/0!': ERR.DIV0,
  '#VALUE!': ERR.VALUE,
  '#NAME?': ERR.NAME,
  '#REF!': ERR.REF,
  '#N/A': ERR.NA,
  '#NUM!': ERR.NUM,
  '#NULL!': ERR.NULL,
};

/**
 * El interior de una constante matricial («1,2;3,4» o «"a","b"») como filas de valores.
 * Elementos: números (con signo), textos entre comillas, VERDADERO/FALSO, errores y huecos
 * (un hueco es un blanco). Todas las filas con las mismas columnas, como en Excel.
 */
export function parseArrayLiteral(inner: string): CellValue[][] {
  const rows: CellValue[][] = [[]];
  let cur = '';
  let enTexto = false;
  let eraTexto = false; // el elemento llevaba comillas: «""» es un texto vacío, no un hueco
  const cierra = (): void => {
    const t = cur.trim();
    let v: CellValue;
    if (eraTexto) {
      if (!/^"(?:[^"]|"")*"$/.test(t))
        throw new ParseError('elemento de constante matricial no válido');
      v = t.slice(1, -1).replace(/""/g, '"');
    } else if (t === '') v = '';
    else if (/^(TRUE|VERDADERO)$/i.test(t)) v = true;
    else if (/^(FALSE|FALSO)$/i.test(t)) v = false;
    else if (ERRORES_LITERALES[t.toUpperCase()]) v = ERRORES_LITERALES[t.toUpperCase()]!;
    else if (/^[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?$/.test(t)) v = Number(t);
    else throw new ParseError(`elemento de constante matricial no válido: «${t}»`);
    rows[rows.length - 1]!.push(v);
    cur = '';
    eraTexto = false;
  };
  for (const c of inner) {
    if (c === '"') {
      enTexto = !enTexto;
      eraTexto = true;
      cur += c;
    } else if (!enTexto && c === ',') cierra();
    else if (!enTexto && c === ';') {
      cierra();
      rows.push([]);
    } else cur += c;
  }
  cierra();
  const ancho = rows[0]!.length;
  if (rows.some((r) => r.length !== ancho))
    throw new ParseError('las filas de una constante matricial deben tener las mismas columnas');
  return rows;
}

/* ---------- Parser (descenso recursivo por precedencia) ---------- */

class Parser {
  private p = 0;
  constructor(private readonly toks: Tok[]) {}

  private peek(): Tok | undefined {
    return this.toks[this.p];
  }
  private next(): Tok {
    const t = this.toks[this.p];
    if (!t) throw new ParseError('fin de expresión inesperado');
    this.p++;
    return t;
  }
  private isOp(...ops: string[]): boolean {
    const t = this.peek();
    return !!t && t.t === 'op' && ops.includes(t.v);
  }

  parse(): AstNode {
    const node = this.compare();
    if (this.peek()) throw new ParseError(`sobra «${this.peek()!.v}» tras la expresión`);
    return node;
  }

  private compare(): AstNode {
    let left = this.concat();
    while (this.isOp('=', '<>', '<', '>', '<=', '>=')) {
      const op = this.next().v as BinOp;
      const right = this.concat();
      left = { kind: 'binary', op, left, right };
    }
    return left;
  }
  private concat(): AstNode {
    let left = this.addSub();
    while (this.isOp('&')) {
      this.next();
      const right = this.addSub();
      left = { kind: 'binary', op: '&', left, right };
    }
    return left;
  }
  private addSub(): AstNode {
    let left = this.mulDiv();
    while (this.isOp('+', '-')) {
      const op = this.next().v as BinOp;
      const right = this.mulDiv();
      left = { kind: 'binary', op, left, right };
    }
    return left;
  }
  private mulDiv(): AstNode {
    let left = this.power();
    while (this.isOp('*', '/')) {
      const op = this.next().v as BinOp;
      const right = this.power();
      left = { kind: 'binary', op, left, right };
    }
    return left;
  }
  private power(): AstNode {
    const base = this.unary();
    if (this.isOp('^')) {
      this.next();
      const exp = this.power(); // asociativo por la derecha
      return { kind: 'binary', op: '^', left: base, right: exp };
    }
    return base;
  }
  private unary(): AstNode {
    if (this.isOp('-', '+')) {
      const op = this.next().v as '-' | '+';
      return { kind: 'unary', op, operand: this.unary() };
    }
    return this.postfix();
  }
  private postfix(): AstNode {
    let node = this.primary();
    while (this.peek()?.t === 'percent') {
      this.next();
      node = { kind: 'percent', operand: node };
    }
    return node;
  }

  private primary(): AstNode {
    const t = this.next();
    if (t.t === 'num') {
      // Fila desnuda como extremo izquierdo de un rango de filas enteras: «3:5»
      // (el lexer tokeniza «3» como número, no como palabra).
      if (this.peek()?.t === 'colon') {
        const a = parseRefEnd(t.v);
        if (a && a.kind === 'row') {
          this.next(); // «:»
          return this.finishRange(a, undefined);
        }
      }
      return { kind: 'num', value: Number(t.v) };
    }
    if (t.t === 'str') return { kind: 'str', value: t.v };
    if (t.t === 'array') return { kind: 'array', rows: parseArrayLiteral(t.v) };
    if (t.t === 'lparen') {
      const e = this.compare();
      const close = this.next();
      if (close.t !== 'rparen') throw new ParseError('falta «)»');
      return e;
    }
    if (t.t === 'word' || t.t === 'qsheet') return this.reference(t);
    // Corchete suelto `[@Columna]` / `[Columna]`: referencia estructurada de la
    // tabla implícita (la que contiene la celda que se evalúa).
    if (t.t === 'bracket') return structuredNode('', t.v);
    throw new ParseError(`token inesperado: «${t.v}»`);
  }

  /** Resuelve una palabra/hoja en ref, rango, llamada, booleano o nombre. */
  private reference(head: Tok): AstNode {
    let sheet: string | undefined;
    let word = head;
    if (this.peek()?.t === 'bang') {
      this.next(); // '!'
      sheet = head.v;
      const ref = this.next();
      // Tras «!» va una referencia: word (A1, A, $1…) o un num (fila desnuda: Hoja1!3:5).
      if (ref.t !== 'word' && ref.t !== 'num')
        throw new ParseError('se esperaba una referencia tras «!»');
      word = ref;
    }

    // Referencia estructurada `Tabla[Columna]`: palabra (no cualificada, las
    // tablas son globales) seguida de un corchete.
    if (!sheet && this.peek()?.t === 'bracket') {
      return structuredNode(word.v, this.next().v);
    }

    // Llamada a función: palabra (no cualificada) seguida de «(».
    if (!sheet && this.peek()?.t === 'lparen') {
      this.next();
      const args: AstNode[] = [];
      // Un argumento puede faltar entre dos separadores o antes del «)» tras un «,»;
      // «F()» sin nada sigue siendo cero argumentos.
      const argumento = (): AstNode => {
        const t = this.peek()?.t;
        return t === 'sep' || t === 'rparen' ? { kind: 'empty' } : this.compare();
      };
      if (this.peek()?.t !== 'rparen') {
        args.push(argumento());
        while (this.peek()?.t === 'sep') {
          this.next();
          args.push(argumento());
        }
      }
      const close = this.next();
      if (close.t !== 'rparen') throw new ParseError('falta «)» en la llamada');
      return { kind: 'call', name: word.v, args };
    }

    const up = word.v.toUpperCase();
    if (!sheet && up === 'TRUE') return { kind: 'bool', value: true };
    if (!sheet && up === 'FALSE') return { kind: 'bool', value: false };

    const end = parseRefEnd(word.v);
    if (end) {
      if (this.peek()?.t === 'colon') {
        this.next();
        return this.finishRange(end, sheet); // A1:B2, A:C, $1:$3, Hoja1!A:A…
      }
      // Solo una CELDA suelta es referencia; una columna/fila sola («A», «$3») no
      // lo es → cae a nombre (dará #NAME? al evaluar, como Excel).
      if (end.kind === 'cell') return { kind: 'ref', ref: endRef(end, sheet) };
    }

    return { kind: 'name', name: word.v, sheet };
  }

  /**
   * Cierra un rango tras el «:»: parsea el extremo derecho (word o num), exige
   * que ambos extremos sean del MISMO tipo (celda/columna/fila) y normaliza los
   * de eje entero invertidos (C:A → A:C, 5:3 → 3:5). El extremo derecho hereda la
   * hoja del izquierdo.
   */
  private finishRange(a: RefEnd, sheet: string | undefined): AstNode {
    const bTok = this.next();
    const b = bTok.t === 'word' || bTok.t === 'num' ? parseRefEnd(bTok.v) : null;
    if (!b) throw new ParseError('se esperaba una referencia tras «:»');
    if (a.kind !== b.kind) throw new ParseError('extremos de rango incompatibles');
    if (a.kind === 'cell') return { kind: 'range', a: endRef(a, sheet), b: endRef(b, sheet) };
    const whole: 'col' | 'row' = a.kind === 'col' ? 'col' : 'row';
    const swap = whole === 'col' ? a.col > b.col : a.row > b.row;
    const [lo, hi] = swap ? [b, a] : [a, b];
    return { kind: 'range', a: endRef(lo, sheet), b: endRef(hi, sheet), whole };
  }
}

/* ---------- Referencias estructuradas de tabla ---------- */

/** Construye el nodo `structured` a partir del nombre de tabla y el texto crudo
 *  interior del corchete (ya sin los corchetes exteriores). */
const structuredNode = (table: string, inner: string): AstNode => {
  const spec = parseBracketSpec(inner);
  return { kind: 'structured', table, column: spec.column, area: spec.area, thisRow: spec.thisRow };
};

/** Quita el apóstrofo que escapa a un carácter especial de tabla (`[ ] # ' @`). */
const unescapeSpec = (s: string): string => s.replace(/'([[\]#'@])/g, '$1');

/** Mapea una palabra clave de área `#…` (ES/EN) a su área, o a la marca de fila
 *  actual (`#Esta fila`/`#This Row`), o undefined si no la reconoce. */
const specifierArea = (s: string): TableArea | 'thisRow' | undefined => {
  switch (s.slice(1).replace(/\s+/g, '').toLowerCase()) {
    case 'todo':
    case 'all':
      return 'all';
    case 'datos':
    case 'data':
      return 'data';
    case 'cabeceras':
    case 'headers':
    case 'header':
      return 'headers';
    case 'totales':
    case 'totals':
    case 'total':
      return 'totals';
    case 'estafila':
    case 'thisrow':
      return 'thisRow';
    default:
      return undefined; // `#desconocido` → no aporta (la ref quedará sin resolver)
  }
};

/** Clasifica un especificador simple: `#Área`, `@Columna`/`@`, o un nombre de
 *  columna. El prefijo `#`/`@` se mira SIN desescapar (así `['@Total]` es una
 *  columna literal, no la fila actual); el nombre sí se desescapa. */
const classifyPart = (raw: string): { column?: string; area?: TableArea; thisRow?: boolean } => {
  const t = raw.trim();
  if (t.startsWith('#')) {
    const a = specifierArea(t);
    if (a === 'thisRow') return { thisRow: true };
    return a ? { area: a } : {};
  }
  if (t.startsWith('@')) {
    const rest = t.slice(1).trim();
    return rest ? { thisRow: true, column: unescapeSpec(rest) } : { thisRow: true };
  }
  return { column: unescapeSpec(t) };
};

/** Extrae los grupos `[…]` de un especificador anidado `[#Datos],[Columna]`,
 *  respetando el escape con apóstrofo. Un solo recorrido O(n). */
const bracketGroups = (inner: string): string[] => {
  const parts: string[] = [];
  let i = 0;
  while (i < inner.length) {
    if (inner[i] !== '[') {
      i++; // separadores (coma/espacio) entre grupos
      continue;
    }
    let depth = 1;
    let j = i + 1;
    let out = '';
    while (j < inner.length && depth > 0) {
      const c = inner[j]!;
      if (c === "'") {
        out += inner[j + 1] ?? '';
        j += 2;
        continue;
      }
      if (c === '[') depth++;
      else if (c === ']') {
        depth--;
        if (depth === 0) break;
      }
      out += c;
      j++;
    }
    parts.push(out);
    i = j + 1;
  }
  return parts;
};

/**
 * Interpreta el texto interior de un corchete estructurado en (columna, área,
 * fila-actual). Formas soportadas: `Columna`, `@Columna`, `#Datos`/`#Todo`/
 * `#Cabeceras`/`#Totales` (ES y EN) y la forma anidada `[#Datos],[Columna]`.
 * Función PURA (exportada para tests). Los tokens desconocidos no aportan.
 */
export const parseBracketSpec = (
  inner: string,
): { column: string | undefined; area: TableArea; thisRow: boolean } => {
  const trimmed = inner.trim();
  const parts = trimmed.startsWith('[') ? bracketGroups(trimmed) : [trimmed];
  let column: string | undefined;
  let area: TableArea = 'data';
  let thisRow = false;
  for (const p of parts) {
    const c = classifyPart(p);
    if (c.area) area = c.area;
    if (c.thisRow) thisRow = true;
    if (c.column !== undefined) column = c.column;
  }
  return { column, area, thisRow };
};

/**
 * Parsea una expresión de fórmula. Acepta un «=» inicial opcional (así puede
 * recibir el texto crudo de la celda o solo la expresión). Lanza ParseError.
 */
export function parseFormula(input: string): AstNode {
  let src = input;
  if (src.startsWith('=')) src = src.slice(1);
  return new Parser(lex(src)).parse();
}
