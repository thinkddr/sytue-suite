/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Motor de fórmulas: mantiene el grid de una o varias hojas, parsea las celdas
 * «=», resuelve referencias y rangos, evalúa en orden topológico (por recursión
 * memorizada) recalculando solo lo sucio, y detecta ciclos → #CIRC!.
 * Código original (clean-room), semántica OpenFormula / ECMA-376.
 */
import {
  parseFormula,
  parseCellWord,
  numToCol,
  type AstNode,
  type BinOp,
  type CellRef,
} from './parser.js';
import {
  ERR,
  isError,
  coerceNumber,
  coerceText,
  compareValues,
  formatNumber,
  FormulaError,
  type CellValue,
} from './values.js';
import { buildRegistry } from './functions/index.js';
import { groupBy, pivotBy } from './functions/agrupar.js';
import type { Agregador } from './functions/agrupar.js';
import type { Arg, FnCtx, FnImpl, RefRange, Registry, TableResolver } from './registry.js';

/** Opciones del motor. Reloj y RNG inyectables para NOW/TODAY/RAND deterministas. */
export interface EngineOptions {
  sheets?: string[];
  now?: () => Date;
  random?: () => number;
  /** Resolutor de referencias estructuradas `Tabla[Columna]` (mapa tabla→rango
   *  del editor). Sin él, toda referencia estructurada da #REF!. */
  resolveTable?: TableResolver;
}

interface Cell {
  sheet: string;
  col: number;
  row: number;
  raw: string;
  /** AST si es fórmula; null si es literal; 'ERR' si la fórmula no parsea. */
  ast: AstNode | null | 'ERR';
  /** Claves de celda de las que depende (rangos expandidos), para invalidar. */
  deps: string[];
  /** Fórmula de matriz CSE `{=…}` (Ctrl+Mayús+Entrar): evalúa elementwise. */
  array: boolean;
  /** ¿Lleva un SUBTOTAL o un AGGREGATE? Se calcula la primera vez que se pregunta. */
  sub?: boolean;
  /** ¿Es volátil (NOW, RAND, OFFSET…)? Se reevalúa en cada edición, no solo si cambia
   *  algo de lo que lee. Ver VOLATILES. */
  volatile?: boolean;
}

/**
 * Funciones VOLÁTILES: su valor puede cambiar sin que cambie ninguna celda que lean.
 * NOW/TODAY/RAND/RANDBETWEEN/RANDARRAY dependen del reloj o del azar; OFFSET e INDIRECT
 * construyen la referencia al evaluar, así que el motor no puede saber de antemano de
 * qué dependen. Como en Excel, una fórmula que llama a alguna de ellas, directa o por
 * un nombre definido, se recalcula en cada edición.
 */
export const VOLATILES: ReadonlySet<string> = new Set([
  'NOW',
  'TODAY',
  'RAND',
  'RANDBETWEEN',
  'RANDARRAY',
  'OFFSET',
  'INDIRECT',
]);

/** Las dos funciones que se saltan entre sí (y a sí mismas) al resumir un rango. */
const RESUMEN = new Set(['SUBTOTAL', 'AGGREGATE']);

/** ¿El árbol llama a SUBTOTAL o a AGGREGATE en algún punto? */
function llevaResumen(n: AstNode): boolean {
  switch (n.kind) {
    case 'call':
      return RESUMEN.has(n.name.toUpperCase()) || n.args.some(llevaResumen);
    case 'unary':
    case 'percent':
      return llevaResumen(n.operand);
    case 'binary':
      return llevaResumen(n.left) || llevaResumen(n.right);
    default:
      return false;
  }
}

/**
 * Funciones de ORDEN SUPERIOR: reciben un LAMBDA y por eso no pueden vivir en el
 * Registry, que entrega valores ya evaluados. Se despachan en evalCall, como LET.
 */
const ORDEN_SUPERIOR = new Set(['MAP', 'REDUCE', 'SCAN', 'MAKEARRAY', 'BYROW', 'BYCOL']);
/** GROUPBY y PIVOTBY: reciben una función (un LAMBDA o el nombre de una) como argumento. */
const AGRUPADORAS = new Set(['GROUPBY', 'PIVOTBY']);

interface Env {
  sheet: string;
  col: number;
  row: number;
  ctx: FnCtx;
  /** Variables locales de LET (nombre en mayúsculas → matriz). */
  locals?: Map<string, CellValue[][]> | undefined;
  /** Contexto de matriz CSE: las funciones escalares se difunden elementwise. */
  arrayMode?: boolean;
  /** Parámetros del LAMBDA en curso que no recibieron argumento (ISOMITTED). */
  omitidos?: Set<string> | undefined;
}

/** Un array derramado (spill): su rectángulo y la matriz que lo llena. */
interface SpillInfo {
  sheet: string;
  c1: number;
  r1: number;
  c2: number;
  r2: number;
  matrix: CellValue[][];
}

/** Definición de un nombre: su AST y la hoja base para refs sin cualificar. */
interface NameDef {
  ast: AstNode;
  sheet: string;
}

const SEP = '\u0000';
/** Tope de celdas al expandir un rango como dependencias (evita explotar). */
const DEP_EXPAND_CAP = 200_000;

/** Nombre válido de rango/variable: identificador que NO parezca una referencia. */
const NAME_RE = /^[A-Za-z_][A-Za-z0-9_.]*$/;
const CELLISH_RE = /^\$?[A-Za-z]{1,3}\$?[0-9]+$/;
/** Lo que el motor acepta como nombre definido; el editor valida con esto mismo. */
export function isValidName(name: string): boolean {
  if (!NAME_RE.test(name) || CELLISH_RE.test(name)) return false;
  const up = name.toUpperCase();
  return up !== 'TRUE' && up !== 'FALSE' && up !== 'R' && up !== 'C';
}

/**
 * Funciones cuyo resultado de nivel superior suele ser un array. Sirve para
 * «cebar» el ancla de derrame antes de su 1ª evaluación, y así poder resolver
 * referencias a celdas derramadas sin haber renderizado antes el ancla. Los
 * falsos positivos se autolimpian al evaluar (si dan escalar salen del conjunto).
 */
const ARRAY_FUNCS = new Set([
  'SEQUENCE',
  'RANDARRAY',
  'FILTER',
  'SORT',
  'SORTBY',
  'UNIQUE',
  'TRANSPOSE',
  'XLOOKUP',
  'INDEX',
  'OFFSET',
  'CHOOSE',
  'MMULT',
  'LET',
  'TEXTSPLIT',
  'INDIRECT',
  'MINVERSE',
  'MUNIT',
  'TREND',
]);
function mightSpill(ast: AstNode): boolean {
  if (ast.kind === 'range' || ast.kind === 'name' || ast.kind === 'structured') return true;
  if (ast.kind === 'array') return ast.rows.length > 1 || ast.rows[0]!.length > 1;
  if (ast.kind === 'call') return ARRAY_FUNCS.has(ast.name.toUpperCase());
  return false;
}

/**
 * Funciones ESCALARES puras que, en contexto de matriz CSE (`{=…}`), se difunden
 * elementwise sobre sus argumentos-array (p. ej. `{=SQRT(A1:A3)}` o
 * `{=IF(A1:A3>0,A1:A3,0)}`). Es un ALLOWLIST deliberado: lo que no esté aquí se
 * evalúa normal (los reductores como SUM/AND/CONCAT siguen consumiendo el rango
 * entero; un escalar desconocido simplemente no se vectoriza, sin resultados
 * erróneos). Nunca incluir funciones que agregan un rango.
 */
const ELEMENTWISE = new Set([
  'IF',
  'IFS',
  'IFERROR',
  'IFNA',
  'NOT',
  'SWITCH',
  'ABS',
  'SIGN',
  'SQRT',
  'POWER',
  'MOD',
  'QUOTIENT',
  'INT',
  'TRUNC',
  'ROUND',
  'ROUNDUP',
  'ROUNDDOWN',
  'MROUND',
  'CEILING',
  'FLOOR',
  'EXP',
  'LN',
  'LOG',
  'LOG10',
  'FACT',
  'SIN',
  'COS',
  'TAN',
  'ASIN',
  'ACOS',
  'ATAN',
  'ATAN2',
  'SINH',
  'COSH',
  'TANH',
  'DEGREES',
  'RADIANS',
  'COMBIN',
  'PERMUT',
  'LEN',
  'LEFT',
  'RIGHT',
  'MID',
  'UPPER',
  'LOWER',
  'PROPER',
  'TRIM',
  'TEXT',
  'VALUE',
  'CHAR',
  'CODE',
  'UNICHAR',
  'UNICODE',
  'EXACT',
  'CLEAN',
  'REPT',
  'SUBSTITUTE',
  'REPLACE',
  'FIND',
  'SEARCH',
  'NUMBERVALUE',
  'T',
  'N',
  'ISEVEN',
  'ISODD',
  'YEAR',
  'MONTH',
  'DAY',
  'HOUR',
  'MINUTE',
  'SECOND',
  'WEEKDAY',
  'WEEKNUM',
  'DATE',
  'TIME',
  'EDATE',
  'EOMONTH',
  'DATEVALUE',
  'TIMEVALUE',
  'ISOWEEKNUM',
  'DATESTR',
]);

/** Envuelve un valor escalar como Arg (para difundir una función elementwise). */
function scalarArg(v: CellValue): Arg {
  return { value: () => v, matrix: () => [[v]], ref: undefined };
}

/** Una matriz de valores como Arg (para llamar a una función de la librería por su nombre). */
function matrizArg(m: CellValue[][]): Arg {
  return { value: () => collapse(m), matrix: () => m, ref: undefined };
}

/** ¿Es el nodo una llamada a LAMBDA? */
function esLambda(n: AstNode): n is Extract<AstNode, { kind: 'call' }> {
  return n.kind === 'call' && n.name.toUpperCase() === 'LAMBDA';
}

function keyOf(sheet: string, col: number, row: number): string {
  return sheet + SEP + col + SEP + row;
}

/**
 * Claves SINTÉTICAS de dependencia de columna/fila entera: quien referencia A:A
 * (o 1:1) las registra como deps y `setCell` las invalida al escribir en esa
 * columna/fila, para que SUM(A:A) recalcule cuando cambia CUALQUIER celda de A —
 * incluso una nueva más abajo del último contenido (los rangos de eje entero se
 * acotan al rango usado, así que sin esto no vigilarían las filas futuras). La
 * letra c/r tras el SEP las mantiene fuera del espacio de claves de celda
 * (donde tras el SEP va la columna, un dígito): ninguna clave de celda vale
 * sheet+SEP+'c'+n ni sheet+SEP+'r'+n, así que no colisionan.
 */
function colDepKey(sheet: string, col: number): string {
  return sheet + SEP + 'c' + col;
}
function rowDepKey(sheet: string, row: number): string {
  return sheet + SEP + 'r' + row;
}

/**
 * Clave sintética de «rango demasiado grande para expandirlo» (> DEP_EXPAND_CAP celdas):
 * quien lo lee la registra y cualquier escritura en la hoja la invalida. Es grosero a
 * propósito —no sabemos qué celdas caen dentro sin recorrerlas—, pero correcto: sin ella
 * el rango no tenía ninguna dependencia y su caché quedaba rancia para siempre. Tras el
 * SEP va una `g`, fuera del espacio de claves de celda, como las de columna y fila.
 */
function grandeDepKey(sheet: string): string {
  return sheet + SEP + 'g';
}

/** Parsea «A1» → {col,row} 0-based (ignora los «$»). */
function parseA1(a1: string): { col: number; row: number } {
  const r = parseCellWord(a1, undefined);
  return { col: r.col, row: r.row };
}

export class Engine {
  private readonly cells = new Map<string, Cell>();
  private readonly cache = new Map<string, CellValue>();
  /** Índice inverso: clave de celda → fórmulas que dependen de ella. */
  private readonly revDeps = new Map<string, Set<string>>();
  private readonly computing = new Set<string>();
  private readonly names: string[];
  private readonly clock: () => Date;
  private readonly rng: () => number;
  private readonly registry: Registry;
  /** Resolutor de tablas inyectado por el editor (o undefined → tablas = #REF!). */
  private readonly tableResolver: TableResolver | undefined;
  /** Nombres definidos (rangos/valores/fórmulas), clave en mayúsculas. */
  private readonly definedNames = new Map<string, NameDef>();
  /** Derrames activos: clave del ancla → rectángulo + matriz. */
  private readonly spills = new Map<string, SpillInfo>();
  /** Celda derramada (no ancla) → clave del ancla que la cubre. */
  private readonly coverage = new Map<string, string>();
  /** Candidatas a ancla de array (para resolver refs a celdas derramadas). */
  private readonly arrayAnchors = new Map<string, { sheet: string; col: number; row: number }>();
  /** Guarda contra ciclos al resolver un nombre que se referencia a sí mismo. */
  private readonly resolvingNames = new Set<string>();
  /**
   * Marca de agua del rango USADO por hoja (última col/fila con contenido). Acota
   * las referencias de columna/fila entera (A:A, 1:1) para NO recorrer 1.048.576
   * filas: SUM(A:A) itera solo hasta `maxRow`. Solo CRECE (no encoge al borrar):
   * deja el máximo histórico… salvo que se borre en el borde: entonces la hoja se marca
   * en `usadoSucio` y se recuenta, perezosamente, la próxima vez que alguien lo pregunte.
   * (Un motor que vive entre ediciones no puede quedarse con el máximo histórico: la
   * rejilla del editor se estiraría para siempre tras borrar.)
   */
  private readonly used = new Map<string, { maxCol: number; maxRow: number }>();
  private readonly usadoSucio = new Set<string>();
  /** Filas ocultas por hoja: 1 a mano (esquema), 2 por el filtro. Ver setHiddenRows. */
  private readonly hiddenRows = new Map<string, Map<number, 1 | 2>>();
  /** Claves de las celdas volátiles (ver VOLATILES), para invalidarlas en cada edición
   *  sin recorrer todas las celdas. */
  private readonly volatiles = new Set<string>();
  /** Hubo una edición desde la última lectura: las volátiles hay que reevaluarlas. */
  private volatilesSucias = false;
  /** Fórmulas evaluadas de verdad desde que se creó el motor (no las servidas de la
   *  caché). Es la medida de «una edición solo reevalúa a sus dependientes». */
  private evaluadas = 0;

  constructor(opts: EngineOptions = {}) {
    this.names = opts.sheets && opts.sheets.length > 0 ? [...opts.sheets] : ['Hoja1'];
    this.clock = opts.now ?? (() => new Date());
    this.rng = opts.random ?? Math.random;
    this.registry = buildRegistry();
    this.tableResolver = opts.resolveTable;
  }

  /** Nombres de las hojas, en orden. */
  sheetNames(): string[] {
    return [...this.names];
  }

  /** Escribe el texto crudo de una celda (fórmula o literal). '' la vacía. */
  setCell(sheet: string, a1: string, raw: string): void {
    if (!this.names.includes(sheet)) this.names.push(sheet);
    const { col, row } = parseA1(a1);
    const k = keyOf(sheet, col, row);

    const old = this.cells.get(k);
    if (old) for (const d of old.deps) this.revDeps.get(d)?.delete(k);
    this.volatiles.delete(k);

    const hadArrays = this.arrayAnchors.size > 0;
    this.arrayAnchors.delete(k);
    if (raw === '') {
      this.cells.delete(k);
      // Borrar en el borde del rango usado puede encogerlo: se recuenta al preguntarlo.
      const u = this.used.get(sheet);
      if (u && (col >= u.maxCol || row >= u.maxRow)) this.usadoSucio.add(sheet);
    } else {
      this.growUsed(sheet, col, row); // amplía el rango usado (acota los A:A / 1:1)
      let ast: AstNode | null | 'ERR' = null;
      let deps: string[] = [];
      // Fórmula normal «=…» o de matriz clásica CSE «{=…}» (se le quitan las llaves).
      const arrayForm = raw.startsWith('{=') && raw.endsWith('}');
      if (raw.startsWith('=') || arrayForm) {
        try {
          ast = parseFormula(arrayForm ? raw.slice(1, -1) : raw);
          deps = this.collectDeps(ast, { sheet, col, row });
          if (arrayForm || mightSpill(ast)) this.arrayAnchors.set(k, { sheet, col, row });
        } catch {
          ast = 'ERR';
        }
      }
      const volatile = ast !== null && ast !== 'ERR' && this.esVolatil(ast, new Set());
      if (volatile) this.volatiles.add(k);
      this.cells.set(k, { sheet, col, row, raw, ast, deps, array: arrayForm, volatile });
      for (const d of deps) {
        let s = this.revDeps.get(d);
        if (!s) {
          s = new Set();
          this.revDeps.set(d, s);
        }
        s.add(k);
      }
    }
    this.invalidate(k);
    // Invalida a quien vigila la COLUMNA o la FILA entera de esta celda (A:A / 1:1).
    this.invalidate(colDepKey(sheet, col));
    this.invalidate(rowDepKey(sheet, row));
    // Y a quien lee un rango demasiado grande para vigilarlo celda a celda.
    this.invalidate(grandeDepKey(sheet));
    // Las volátiles se reevalúan en cada edición, aunque no lean nada de lo editado. Se
    // invalidan al LEER, no aquí: cargar 100.000 celdas con 10.000 RAND() serían mil
    // millones de invalidaciones.
    if (this.volatiles.size > 0) this.volatilesSucias = true;
    if (!hadArrays && this.arrayAnchors.size > 0) {
      // Aparece el 1er array de la hoja: purga la caché por si algún '' vacío
      // quedó cacheado (antes no había arrays) y ahora un derrame lo cubrirá.
      this.cache.clear();
      this.spills.clear();
      this.coverage.clear();
    } else if (this.arrayAnchors.size) {
      // Con arrays ya presentes, cualquier edición puede crear/romper una colisión
      // de derrame: revalida las anclas para que recomputen su área/#SPILL!.
      // ponytail: revalida TODAS las anclas por edición; índice de solapamiento si
      // algún día hay muchísimos arrays en una misma hoja.
      // La copia del iterador es DELIBERADA: `invalidate` desmonta derrames y puede
      // acabar tocando el propio mapa de anclas; recorrer el iterador vivo mientras
      // se modifica se salta claves en silencio.
      // oxlint-disable-next-line unicorn/no-useless-spread
      for (const ak of [...this.arrayAnchors.keys()]) this.invalidate(ak);
    }
  }

  /**
   * Dice qué filas de la hoja no se ven: `filtered`, las que esconde el autofiltro, y
   * `manual`, las del detalle de un grupo contraído. Solo lo leen SUBTOTAL y
   * AGGREGATE; el resto de funciones cuenta todas las filas, como en cualquier hoja.
   * Una fila en las dos listas cuenta como filtrada.
   *
   * ponytail: vacía la caché ENTERA en vez de solo las celdas con SUBTOTAL. La rejilla
   * es de 26×100 y esto se llama al contraer un grupo o filtrar, no al teclear; si
   * algún día pesara, el camino es un índice de las celdas con resumen.
   */
  setHiddenRows(sheet: string, filtered: Iterable<number>, manual: Iterable<number>): void {
    const m = new Map<number, 1 | 2>();
    for (const r of manual) m.set(r, 1);
    for (const r of filtered) m.set(r, 2);
    if (m.size === 0) this.hiddenRows.delete(sheet);
    else this.hiddenRows.set(sheet, m);
    this.cache.clear();
    this.spills.clear();
    this.coverage.clear();
  }

  /* ---------- Nombres definidos (rangos con nombre) ---------- */

  /**
   * Registra (o redefine) un nombre. `definition` es una expresión de fórmula:
   * un rango («Hoja1!A1:B10»), un valor («0,21» → usar «0.21»), o una fórmula
   * («SUM(A1:A10)»). `scopeSheet` = hoja base para las refs sin cualificar (por
   * defecto, la 1ª hoja). Devuelve false si `name` no es un identificador válido
   * (o parece una referencia) o si `definition` no parsea.
   */
  defineName(name: string, definition: string, scopeSheet?: string, local = false): boolean {
    if (!isValidName(name)) return false;
    let ast: AstNode;
    try {
      ast = parseFormula(definition);
    } catch {
      return false;
    }
    const sheet = scopeSheet && this.names.includes(scopeSheet) ? scopeSheet : this.names[0]!;
    // `local`: el nombre solo existe en las fórmulas de `scopeSheet` y allí tapa al del
    // libro que se llame igual (el «ámbito: hoja» de Excel). Clave «hoja\0NOMBRE».
    this.definedNames.set(local ? sheet + SEP + name.toUpperCase() : name.toUpperCase(), {
      ast,
      sheet,
    });
    this.rebuildAllDeps();
    return true;
  }

  /** Borra un nombre definido (el de la hoja `localSheet`, si se da). True si existía. */
  deleteName(name: string, localSheet?: string): boolean {
    const up = name.toUpperCase();
    const had = this.definedNames.delete(localSheet ? localSheet + SEP + up : up);
    if (had) this.rebuildAllDeps();
    return had;
  }

  /** Nombres definidos actualmente (en mayúsculas, sin repetir los de hoja), ordenados. */
  definedNameList(): string[] {
    return [
      ...new Set([...this.definedNames.keys()].map((k) => k.slice(k.indexOf(SEP) + 1))),
    ].sort();
  }

  /** El nombre `up` visto desde una fórmula de `sheet`: el de esa hoja gana al del libro. */
  private lookupName(up: string, sheet: string): { key: string; def: NameDef } | undefined {
    const localKey = sheet + SEP + up;
    const local = this.definedNames.get(localKey);
    if (local) return { key: localKey, def: local };
    const def = this.definedNames.get(up);
    return def ? { key: up, def } : undefined;
  }

  /** Texto crudo tal cual se escribió (fuente de verdad). '' si vacía. */
  getRaw(sheet: string, a1: string): string {
    const { col, row } = parseA1(a1);
    return this.cells.get(keyOf(sheet, col, row))?.raw ?? '';
  }

  /** Valor calculado (número, texto, booleano o error). '' si la celda vacía. */
  getValue(sheet: string, a1: string): CellValue {
    const { col, row } = parseA1(a1);
    this.refrescarVolatiles();
    return this.evalCell(sheet, col, row);
  }

  /** Texto a mostrar en el grid (errores como su código, números formateados). */
  getDisplay(sheet: string, a1: string): string {
    const v = this.getValue(sheet, a1);
    if (v instanceof FormulaError) return v.code;
    if (typeof v === 'boolean') return v ? 'TRUE' : 'FALSE';
    if (typeof v === 'number') return formatNumber(v);
    return v;
  }

  /** Última columna y fila con contenido de la hoja (0-based; -1 si está vacía). Es la
   *  marca de agua de `used`, que encoge al borrar en el borde. */
  usedRange(sheet: string): { maxCol: number; maxRow: number } {
    const u = this.usado(sheet);
    return u ? { maxCol: u.maxCol, maxRow: u.maxRow } : { maxCol: -1, maxRow: -1 };
  }

  /**
   * Evalúa una fórmula SIN escribirla en ninguna celda, como si estuviera en
   * (`col`, `row`) de `sheet`: no deja caché, dependencias ni derrame, y no amplía el
   * rango usado. Es para lo que pregunta el editor —una regla de formato condicional,
   * una validación—, que antes ocupaba una celda de apoyo y de paso estiraba los A:A
   * hasta ella. Un resultado matricial da su primer valor; si no parsea, #NAME?.
   */
  evaluate(sheet: string, formula: string, col = 0, row = 0): CellValue {
    this.refrescarVolatiles();
    let ast: AstNode;
    try {
      ast = parseFormula(formula);
    } catch {
      return ERR.NAME;
    }
    const env: Env = { sheet, col, row, ctx: this.makeCtx(sheet, col, row) };
    return this.evalNodeMatrix(ast, env)[0]?.[0] ?? '';
  }

  /** Fórmulas que el motor ha evaluado de verdad desde que existe (las que sirve la caché
   *  no cuentan). Sirve para comprobar, y medir, que una edición solo reevalúa lo que
   *  depende de ella. */
  get evaluaciones(): number {
    return this.evaluadas;
  }

  /** Si hubo una edición desde la última lectura, descarta la caché de las volátiles y de
   *  lo que depende de ellas. Dentro de una misma lectura son consistentes: se calculan
   *  una vez y se cachean, como en un recálculo de Excel. */
  private refrescarVolatiles(): void {
    if (!this.volatilesSucias) return;
    this.volatilesSucias = false;
    for (const v of this.volatiles) this.invalidate(v);
  }

  /** ¿El árbol llama a una función volátil, directamente o a través de un nombre
   *  definido? `vistos` corta los nombres que se referencian a sí mismos. */
  private esVolatil(n: AstNode, vistos: Set<string>): boolean {
    switch (n.kind) {
      case 'call': {
        const nombre = n.name.toUpperCase();
        if (VOLATILES.has(nombre) || n.args.some((a) => this.esVolatil(a, vistos))) return true;
        // Un nombre definido que se llama como función (LAMBDA con nombre) es volátil si lo es su cuerpo.
        const def = this.definedNames.get(nombre);
        if (!def || vistos.has(nombre) || this.registry.has(nombre)) return false;
        vistos.add(nombre);
        return this.esVolatil(def.ast, vistos);
      }
      case 'name': {
        const nombre = n.name.toUpperCase();
        const def = this.definedNames.get(nombre);
        if (!def || vistos.has(nombre)) return false;
        vistos.add(nombre);
        return this.esVolatil(def.ast, vistos);
      }
      case 'unary':
      case 'percent':
        return this.esVolatil(n.operand, vistos);
      case 'binary':
        return this.esVolatil(n.left, vistos) || this.esVolatil(n.right, vistos);
      default:
        return false;
    }
  }

  /** Fuerza el cálculo de todas las celdas de fórmula (deja el display listo). */
  recalc(): void {
    this.refrescarVolatiles();
    for (const cell of this.cells.values()) {
      if (cell.ast !== null) this.evalCell(cell.sheet, cell.col, cell.row);
    }
  }

  /* ---------- Interno ---------- */

  private invalidate(k: string): void {
    // Borra la caché de k y, recursivamente, la de todo lo que dependa de él.
    const stack = [k];
    const seen = new Set<string>();
    while (stack.length) {
      const cur = stack.pop()!;
      if (seen.has(cur)) continue;
      seen.add(cur);
      this.cache.delete(cur);
      // Si `cur` era el ancla de un derrame, desmóntalo: libera sus celdas
      // cubiertas (que vuelven a '' o a otro derrame) y propágales la invalidación.
      const sp = this.spills.get(cur);
      if (sp) {
        this.spills.delete(cur);
        for (let r = sp.r1; r <= sp.r2; r++)
          for (let c = sp.c1; c <= sp.c2; c++) {
            if (r === sp.r1 && c === sp.c1) continue;
            const ck = keyOf(sp.sheet, c, r);
            if (this.coverage.get(ck) === cur) {
              this.coverage.delete(ck);
              stack.push(ck);
            }
          }
      }
      const deps = this.revDeps.get(cur);
      if (deps) for (const d of deps) stack.push(d);
    }
  }

  /** Recomputa deps + revDeps de todas las celdas (tras cambiar un nombre) y
   *  purga caché/derrames: los valores se recalculan perezosamente. */
  private rebuildAllDeps(): void {
    this.revDeps.clear();
    this.volatiles.clear();
    for (const cell of this.cells.values()) {
      if (cell.ast && cell.ast !== 'ERR') {
        // Un nombre redefinido puede hacer volátil (o dejar de serlo) a quien lo usa.
        cell.volatile = this.esVolatil(cell.ast, new Set());
        if (cell.volatile) this.volatiles.add(keyOf(cell.sheet, cell.col, cell.row));
        cell.deps = this.collectDeps(cell.ast, { sheet: cell.sheet, col: cell.col, row: cell.row });
        const k = keyOf(cell.sheet, cell.col, cell.row);
        for (const d of cell.deps) {
          let s = this.revDeps.get(d);
          if (!s) {
            s = new Set();
            this.revDeps.set(d, s);
          }
          s.add(k);
        }
      } else {
        cell.deps = [];
      }
    }
    this.cache.clear();
    this.spills.clear();
    this.coverage.clear();
  }

  private evalCell(sheet: string, col: number, row: number): CellValue {
    const k = keyOf(sheet, col, row);
    const hit = this.cache.get(k);
    if (hit !== undefined) return hit;
    if (this.computing.has(k)) return ERR.CIRC; // ciclo: no se cachea aquí

    const cell = this.cells.get(k);
    if (!cell) {
      const v = this.coveredValue(sheet, col, row, k);
      // Un '' vacío NO se cachea si hay arrays vivos: un derrame posterior podría
      // cubrir esta celda y un '' cacheado quedaría obsoleto. Sin arrays, se
      // cachea (camino común y rápido). Los valores derramados sí se cachean (se
      // purgan al invalidar el ancla).
      if (v !== '' || this.arrayAnchors.size === 0) this.cache.set(k, v);
      return v;
    }
    if (cell.ast === null) {
      const v = parseLiteral(cell.raw);
      this.cache.set(k, v);
      return v;
    }
    if (cell.ast === 'ERR') {
      this.cache.set(k, ERR.NAME);
      return ERR.NAME;
    }

    this.computing.add(k);
    this.evaluadas += 1;
    let v: CellValue;
    try {
      const env: Env = {
        sheet,
        col,
        row,
        ctx: this.makeCtx(sheet, col, row),
        arrayMode: cell.array,
      };
      const m = this.evalNodeMatrix(cell.ast, env);
      v = this.finishCell(sheet, col, row, k, m);
    } finally {
      this.computing.delete(k);
    }
    this.cache.set(k, v);
    return v;
  }

  /**
   * Resultado de nivel superior de una celda: un array 1×1 (o escalar envuelto)
   * colapsa a su único valor; un array multi-celda se DERRAMA (spill) hacia
   * abajo/derecha desde el ancla. Si el área destino choca con contenido de
   * usuario u otro derrame → #SPILL! (no derrama nada).
   */
  private finishCell(
    sheet: string,
    col: number,
    row: number,
    k: string,
    m: CellValue[][],
  ): CellValue {
    const R = m.length;
    const C = R === 0 ? 0 : m[0]!.length;
    if (R <= 1 && C <= 1) {
      this.arrayAnchors.delete(k);
      return R === 0 || C === 0 ? '' : m[0]![0]!;
    }
    this.arrayAnchors.set(k, { sheet, col, row }); // es (o quiere ser) un array
    const c2 = col + C - 1;
    const r2 = row + R - 1;
    for (let r = row; r <= r2; r++)
      for (let c = col; c <= c2; c++) {
        if (r === row && c === col) continue; // el ancla no cuenta como choque
        const tk = keyOf(sheet, c, r);
        const busy =
          this.cells.has(tk) ||
          (this.coverage.has(tk) && this.coverage.get(tk) !== k) ||
          this.spills.has(tk);
        if (busy) return ERR.SPILL;
      }
    this.spills.set(k, { sheet, c1: col, r1: row, c2, r2, matrix: m });
    this.growUsed(sheet, c2, r2); // el derrame ocupa celdas → cuenta como rango usado
    for (let r = row; r <= r2; r++)
      for (let c = col; c <= c2; c++) {
        if (r === row && c === col) continue;
        this.coverage.set(keyOf(sheet, c, r), k);
      }
    return m[0]![0]!;
  }

  /** Valor de una celda SIN fórmula propia: el de un array que la cubra, o ''. */
  private coveredValue(sheet: string, col: number, row: number, k: string): CellValue {
    let anchor = this.coverage.get(k);
    if (anchor === undefined && this.arrayAnchors.size) {
      // Puede que un array aún no evaluado derrame aquí: evalúa las candidatas
      // arriba-izquierda de la misma hoja (memorizado: barato tras la 1ª vez).
      for (const a of this.arrayAnchors.values()) {
        if (a.sheet !== sheet || a.col > col || a.row > row) continue;
        const ak = keyOf(a.sheet, a.col, a.row);
        if (ak !== k && !this.spills.has(ak) && !this.computing.has(ak)) {
          this.evalCell(a.sheet, a.col, a.row);
        }
      }
      anchor = this.coverage.get(k);
    }
    if (anchor !== undefined) {
      const sp = this.spills.get(anchor);
      if (sp) return sp.matrix[row - sp.r1]?.[col - sp.c1] ?? '';
    }
    return '';
  }

  private makeCtx(sheet: string, col: number, row: number): FnCtx {
    return {
      now: this.clock,
      random: this.rng,
      here: { sheet, col, row },
      resolve: (rr) => this.resolveRange(rr),
      parseRef: (text) => this.parseRefText(text, sheet),
      isFormula: (rr) => {
        const cell = this.cells.get(keyOf(rr.sheet, rr.c1, rr.r1));
        return !!cell && cell.ast !== null; // '=' que parsea o no (ast real o 'ERR')
      },
      resolveTable: (q) => this.tableResolver?.(q, { sheet, col, row }),
      formulaText: (rr) => {
        const cell = this.cells.get(keyOf(rr.sheet, rr.c1, rr.r1));
        return cell && cell.ast !== null ? cell.raw : undefined;
      },
      sheetNames: () => [...this.names],
      subtotalCell: (sh, c, r) => {
        const cell = this.cells.get(keyOf(sh, c, r));
        if (!cell || cell.ast === null || cell.ast === 'ERR') return false;
        cell.sub ??= llevaResumen(cell.ast);
        return cell.sub;
      },
      rowHidden: (sh, r) => this.hiddenRows.get(sh)?.get(r) ?? 0,
    };
  }

  /**
   * Resuelve una referencia estructurada `Tabla[Columna]` a un RefRange usando
   * el resolutor inyectado (mapa tabla→rango del editor), anclado en la celda
   * `here`. Sin resolutor o si la tabla/columna no existe → #REF!; el resolutor
   * puede devolver otro error (p. ej. #VALUE! para `@` fuera de la tabla), que se
   * propaga tal cual.
   */
  private resolveTableRef(
    node: Extract<AstNode, { kind: 'structured' }>,
    here: { sheet: string; col: number; row: number },
  ): RefRange | FormulaError {
    const r = this.tableResolver?.(
      { table: node.table, column: node.column, area: node.area, thisRow: node.thisRow },
      here,
    );
    return r ?? ERR.REF;
  }

  private parseRefText(text: string, defSheet: string): RefRange | undefined {
    try {
      const node = parseFormula(text);
      if (node.kind === 'ref') return this.toRefRange(node.ref, defSheet);
      if (node.kind === 'range') return this.rangeToRefRange(node.a, node.b, defSheet, node.whole);
    } catch {
      /* no es una referencia */
    }
    return undefined;
  }

  private toRefRange(ref: CellRef, defSheet: string): RefRange {
    const s = ref.sheet ?? defSheet;
    return { sheet: s, c1: ref.col, r1: ref.row, c2: ref.col, r2: ref.row };
  }

  /** El rango usado de la hoja, recontado si se borró algo en su borde desde la última
   *  vez. Cuenta las celdas con contenido y lo que cubren los derrames vivos. */
  private usado(sheet: string): { maxCol: number; maxRow: number } | undefined {
    if (this.usadoSucio.delete(sheet)) {
      let maxCol = -1;
      let maxRow = -1;
      for (const c of this.cells.values()) {
        if (c.sheet !== sheet) continue;
        if (c.col > maxCol) maxCol = c.col;
        if (c.row > maxRow) maxRow = c.row;
      }
      for (const sp of this.spills.values()) {
        if (sp.sheet !== sheet) continue;
        if (sp.c2 > maxCol) maxCol = sp.c2;
        if (sp.r2 > maxRow) maxRow = sp.r2;
      }
      if (maxCol < 0) this.used.delete(sheet);
      else this.used.set(sheet, { maxCol, maxRow });
    }
    return this.used.get(sheet);
  }

  /** Amplía la marca de agua del rango usado de una hoja (solo crece). */
  private growUsed(sheet: string, col: number, row: number): void {
    const u = this.used.get(sheet);
    if (!u) this.used.set(sheet, { maxCol: col, maxRow: row });
    else {
      if (col > u.maxCol) u.maxCol = col;
      if (row > u.maxRow) u.maxRow = row;
    }
  }

  /**
   * Traduce un nodo de rango a rectángulo resuelto. Para un rango de celdas
   * normal, min/max de ambos extremos. Para columna entera (`whole==='col'`),
   * las filas van de 0 al `maxRow` USADO de la hoja (JAMÁS 1.048.575: invariante
   * de escala); para fila entera, las columnas de 0 al `maxCol` usado. En una
   * hoja vacía el límite es -1 → rango vacío (SUM=0, como una columna en blanco).
   */
  private rangeToRefRange(
    a: CellRef,
    b: CellRef,
    defSheet: string,
    whole?: 'col' | 'row',
  ): RefRange {
    const s = a.sheet ?? defSheet;
    if (whole === 'col') {
      const maxRow = this.usado(s)?.maxRow ?? -1;
      return {
        sheet: s,
        c1: Math.min(a.col, b.col),
        r1: 0,
        c2: Math.max(a.col, b.col),
        r2: maxRow,
      };
    }
    if (whole === 'row') {
      const maxCol = this.usado(s)?.maxCol ?? -1;
      return {
        sheet: s,
        c1: 0,
        r1: Math.min(a.row, b.row),
        c2: maxCol,
        r2: Math.max(a.row, b.row),
      };
    }
    return {
      sheet: s,
      c1: Math.min(a.col, b.col),
      r1: Math.min(a.row, b.row),
      c2: Math.max(a.col, b.col),
      r2: Math.max(a.row, b.row),
    };
  }

  private resolveRange(rr: RefRange): CellValue[][] {
    const out: CellValue[][] = [];
    for (let r = rr.r1; r <= rr.r2; r++) {
      const row: CellValue[] = [];
      for (let c = rr.c1; c <= rr.c2; c++) row.push(this.evalCell(rr.sheet, c, r));
      out.push(row);
    }
    return out;
  }

  /** Expande las referencias del AST a claves de celda (para invalidar). Un
   *  nodo `name` se expande a las deps de su definición (anclada en su hoja),
   *  con guarda contra ciclos, para que cambiar una celda del rango con nombre
   *  invalide a quien lo usa. */
  private collectDeps(node: AstNode, here: { sheet: string; col: number; row: number }): string[] {
    const keys: string[] = [];
    const nameGuard = new Set<string>();
    const walk = (n: AstNode, sheet: string): void => {
      switch (n.kind) {
        case 'ref': {
          const rr = this.toRefRange(n.ref, sheet);
          keys.push(keyOf(rr.sheet, rr.c1, rr.r1));
          break;
        }
        case 'range': {
          const rr = this.rangeToRefRange(n.a, n.b, sheet, n.whole);
          // Eje entero: se vigila por COLUMNA/FILA (clave sintética), no celda a
          // celda, para invalidar aunque el contenido crezca luego (ver colDepKey).
          if (n.whole === 'col') {
            for (let c = rr.c1; c <= rr.c2; c++) keys.push(colDepKey(rr.sheet, c));
            break;
          }
          if (n.whole === 'row') {
            for (let r = rr.r1; r <= rr.r2; r++) keys.push(rowDepKey(rr.sheet, r));
            break;
          }
          const count = (rr.c2 - rr.c1 + 1) * (rr.r2 - rr.r1 + 1);
          if (count > DEP_EXPAND_CAP) {
            // ponytail: se vigila la hoja entera, no el rectángulo; si importara, un
            // índice de rectángulos en setCell.
            keys.push(grandeDepKey(rr.sheet));
            break;
          }
          for (let r = rr.r1; r <= rr.r2; r++)
            for (let c = rr.c1; c <= rr.c2; c++) keys.push(keyOf(rr.sheet, c, r));
          break;
        }
        case 'structured': {
          // Resuelve la tabla AHORA (con el resolutor del editor) y registra sus
          // celdas como deps, para que editarlas invalide a quien la referencia.
          // ponytail: si la tabla se REDIMENSIONA sin editar esta fórmula, el
          // editor (dueño del mapa) debe invalidar/recalcular; el motor resuelve
          // perezosamente en cada eval, así que el valor siempre queda al día.
          const rr = this.resolveTableRef(n, { sheet, col: here.col, row: here.row });
          if (rr instanceof FormulaError) break;
          const count = (rr.c2 - rr.c1 + 1) * (rr.r2 - rr.r1 + 1);
          if (count > DEP_EXPAND_CAP) {
            keys.push(grandeDepKey(rr.sheet));
            break;
          }
          for (let r = rr.r1; r <= rr.r2; r++)
            for (let c = rr.c1; c <= rr.c2; c++) keys.push(keyOf(rr.sheet, c, r));
          break;
        }
        case 'name': {
          const hit = this.lookupName(n.name.toUpperCase(), sheet);
          if (hit && !nameGuard.has(hit.key)) {
            nameGuard.add(hit.key);
            walk(hit.def.ast, hit.def.sheet);
          }
          break;
        }
        case 'unary':
        case 'percent':
          walk(n.operand, sheet);
          break;
        case 'binary':
          walk(n.left, sheet);
          walk(n.right, sheet);
          break;
        case 'call': {
          for (const a of n.args) walk(a, sheet);
          // Una LAMBDA con nombre llamada como función: lo que lee su cuerpo también cuenta.
          const up = n.name.toUpperCase();
          const def = this.definedNames.get(up);
          if (def && !nameGuard.has(up) && !this.registry.has(up)) {
            nameGuard.add(up);
            walk(def.ast, def.sheet);
          }
          break;
        }
        default:
          break;
      }
    };
    walk(node, here.sheet);
    return keys;
  }

  /* ---------- Evaluación del AST ---------- */

  private evalNode(node: AstNode, env: Env): CellValue {
    switch (node.kind) {
      case 'num':
        return node.value;
      case 'str':
        return node.value;
      case 'bool':
        return node.value;
      case 'array':
        return node.rows[0]![0]!;
      case 'name': {
        const m = this.evalNameNode(node, env);
        return m === undefined ? ERR.NAME : collapse(m);
      }
      case 'ref': {
        const s = node.ref.sheet ?? env.sheet;
        return this.evalCell(s, node.ref.col, node.ref.row);
      }
      case 'range':
        return collapse(this.evalNodeMatrix(node, env));
      case 'structured':
        return collapse(this.evalNodeMatrix(node, env));
      case 'unary': {
        const v = coerceNumber(this.evalNode(node.operand, env));
        if (isError(v)) return v;
        return node.op === '-' ? -v : v;
      }
      case 'percent': {
        const v = coerceNumber(this.evalNode(node.operand, env));
        return isError(v) ? v : v / 100;
      }
      case 'binary':
        return this.evalBinary(node, env);
      case 'call': {
        const r = this.evalCall(node, env);
        return Array.isArray(r) ? collapse(r) : r;
      }
      case 'empty':
        return ''; // argumento omitido: en blanco
    }
  }

  private evalBinary(node: Extract<AstNode, { kind: 'binary' }>, env: Env): CellValue {
    return applyBinary(node.op, this.evalNode(node.left, env), this.evalNode(node.right, env));
  }

  /**
   * Operador binario con difusión (broadcast) elementwise sobre arrays: un
   * escalar (1×1) se difunde; un vector fila/columna se estira contra una matriz;
   * dimensiones incompatibles → #N/A en esa celda. Permite «A1:A3*2» o
   * «A1:A3>2» (esencial para FILTER/SORT sobre condiciones).
   */
  private evalBinaryMatrix(node: Extract<AstNode, { kind: 'binary' }>, env: Env): CellValue[][] {
    const L = this.evalNodeMatrix(node.left, env);
    const R = this.evalNodeMatrix(node.right, env);
    const Lr = L.length;
    const Lc = L[0]?.length ?? 0;
    const Rr = R.length;
    const Rc = R[0]?.length ?? 0;
    if (Lr <= 1 && Lc <= 1 && Rr <= 1 && Rc <= 1) {
      return [[applyBinary(node.op, L[0]?.[0] ?? '', R[0]?.[0] ?? '')]];
    }
    const rows = Math.max(Lr, Rr);
    const cols = Math.max(Lc, Rc);
    const out: CellValue[][] = [];
    for (let r = 0; r < rows; r++) {
      const row: CellValue[] = [];
      for (let c = 0; c < cols; c++) {
        const lv = L[Lr === 1 ? 0 : r]?.[Lc === 1 ? 0 : c] ?? ERR.NA;
        const rv = R[Rr === 1 ? 0 : r]?.[Rc === 1 ? 0 : c] ?? ERR.NA;
        row.push(applyBinary(node.op, lv, rv));
      }
      out.push(row);
    }
    return out;
  }

  private evalCall(node: Extract<AstNode, { kind: 'call' }>, env: Env): CellValue | CellValue[][] {
    const nombre = node.name.toUpperCase();
    if (nombre === 'LET') return this.evalLet(node, env);
    // Las de orden superior reciben el AST de su LAMBDA, no un valor: una funcion no
    // cabe en CellValue (number|string|boolean|FormulaError) y no hace falta que quepa.
    if (ORDEN_SUPERIOR.has(nombre)) return this.evalOrdenSuperior(nombre, node, env);
    if (AGRUPADORAS.has(nombre)) return this.evalAgrupar(nombre, node, env);
    if (nombre === 'ISOMITTED') return this.evalIsOmitted(node, env);
    // LAMBDA suelta no es un valor: sin funcion que la aplique no hay nada que calcular.
    if (nombre === 'LAMBDA') return ERR.CALC;
    const fn = this.registry.get(node.name);
    if (!fn) {
      // Un nombre definido que guarda un LAMBDA se llama como una función: =DOBLE(A1).
      const def = this.definedNames.get(nombre);
      if (def && esLambda(def.ast)) {
        const valores = node.args.map((a) => this.evalNodeMatrix(a, env));
        const omitidos = node.args.map((a) => a.kind === 'empty');
        return this.ejecutarLambda(
          def.ast,
          valores,
          omitidos,
          { ...env, sheet: def.sheet, locals: undefined, omitidos: undefined },
          undefined,
        );
      }
      return ERR.NAME;
    }
    // En una fórmula de matriz CSE, las funciones escalares se difunden elementwise.
    if (env.arrayMode && ELEMENTWISE.has(node.name.toUpperCase())) {
      return this.vectorizeCall(fn, node.args, env);
    }
    return fn(
      env.ctx,
      node.args.map((a) => this.makeArg(a, env)),
    );
  }

  /**
   * Difunde una función escalar sobre sus argumentos-array (semántica CSE): toma
   * la matriz de cada argumento, calcula las dimensiones de difusión (máx. filas
   * × máx. columnas) y llama a la función celda a celda con los escalares
   * correspondientes; un vector fila/columna se estira, dimensiones que faltan →
   * #N/A. Si ningún argumento es array, es una llamada escalar normal.
   */
  private vectorizeCall(fn: FnImpl, argNodes: AstNode[], env: Env): CellValue | CellValue[][] {
    const mats = argNodes.map((n) => this.evalNodeMatrix(n, env));
    let R = 1;
    let C = 1;
    for (const m of mats) {
      R = Math.max(R, m.length);
      C = Math.max(C, m[0]?.length ?? 0);
    }
    if (R <= 1 && C <= 1) {
      return fn(
        env.ctx,
        argNodes.map((n) => this.makeArg(n, env)),
      );
    }
    const out: CellValue[][] = [];
    for (let r = 0; r < R; r++) {
      const row: CellValue[] = [];
      for (let c = 0; c < C; c++) {
        const cellArgs = mats.map((m) => {
          const rr = m.length === 1 ? 0 : r;
          const cc = (m[0]?.length ?? 1) === 1 ? 0 : c;
          return scalarArg(m[rr]?.[cc] ?? ERR.NA);
        });
        const res = fn(env.ctx, cellArgs);
        row.push(Array.isArray(res) ? collapse(res) : res);
      }
      out.push(row);
    }
    return out;
  }

  /**
   * LET(nombre1, valor1, [nombre2, valor2, …], cálculo): liga nombres locales a
   * valores (cada uno ve los definidos antes) y evalúa el cálculo con ellos. Los
   * valores se guardan como matriz, así una variable puede ser un array entero.
   */
  private evalLet(node: Extract<AstNode, { kind: 'call' }>, env: Env): CellValue | CellValue[][] {
    const a = node.args;
    if (a.length < 3 || a.length % 2 === 0) return ERR.VALUE; // pares nombre/valor + cálculo
    const locals = new Map(env.locals ?? []);
    const childEnv: Env = { ...env, locals };
    for (let i = 0; i + 1 < a.length - 1; i += 2) {
      const nameNode = a[i]!;
      if (nameNode.kind !== 'name') return ERR.VALUE; // el nombre debe ser un identificador
      locals.set(nameNode.name.toUpperCase(), this.evalNodeMatrix(a[i + 1]!, childEnv));
    }
    return this.evalNodeMatrix(a[a.length - 1]!, childEnv);
  }

  /** Tope de llamadas anidadas a LAMBDA (una con nombre puede llamarse a sí misma). */
  private profundidadLambda = 0;

  /**
   * Aplica una función a unos argumentos ya evaluados: un LAMBDA literal (cierra sobre las
   * variables de su sitio), el nombre de un LAMBDA definido o el de una función de la
   * librería (`BYROW(A1:C3, SUM)`). #VALUE! si el nodo no es nada de eso o la aridad no
   * cuadra; #NAME? si el nombre no existe.
   */
  private aplicarLambda(
    lam: AstNode,
    valores: CellValue[][][],
    env: Env,
  ): CellValue[][] | FormulaError {
    if (lam.kind === 'name') return this.aplicarPorNombre(lam.name, valores, env);
    if (!esLambda(lam)) return ERR.VALUE;
    return this.ejecutarLambda(lam, valores, [], env, env.locals);
  }

  private aplicarPorNombre(
    nombre: string,
    valores: CellValue[][][],
    env: Env,
  ): CellValue[][] | FormulaError {
    const def = this.definedNames.get(nombre.toUpperCase());
    if (def && esLambda(def.ast)) {
      const limpio: Env = { ...env, sheet: def.sheet, locals: undefined, omitidos: undefined };
      return this.ejecutarLambda(def.ast, valores, [], limpio, undefined);
    }
    const fn = this.registry.get(nombre);
    if (!fn) return ERR.NAME;
    const r = fn(env.ctx, valores.map(matrizArg));
    if (r instanceof FormulaError) return r;
    return Array.isArray(r) ? r : [[r]];
  }

  /**
   * Cuerpo de un LAMBDA con sus parámetros ligados, como LET. Un parámetro entre corchetes,
   * `[x]`, es opcional: si falta el argumento (o llega omitido, `F(1,,3)`) vale en blanco y
   * ISOMITTED(x) da VERDADERO. Los opcionales van al final; faltar uno obligatorio o sobrar
   * argumentos es #VALUE!. `locales` son las variables que el LAMBDA ve además de las suyas:
   * las del sitio donde se escribió si es literal, ninguna si viene de un nombre definido.
   */
  private ejecutarLambda(
    lam: Extract<AstNode, { kind: 'call' }>,
    valores: CellValue[][][],
    omitidosPorPosicion: readonly boolean[],
    env: Env,
    locales: Map<string, CellValue[][]> | undefined,
  ): CellValue[][] | FormulaError {
    const partes = lam.args;
    if (partes.length < 1) return ERR.VALUE;
    const params = partes.slice(0, -1);
    const cuerpo = partes[partes.length - 1]!;
    if (valores.length > params.length) return ERR.VALUE; // sobran argumentos
    const nombres: { nombre: string; opcional: boolean }[] = [];
    for (const pn of params) {
      if (pn.kind === 'name') nombres.push({ nombre: pn.name.toUpperCase(), opcional: false });
      else if (pn.kind === 'structured' && pn.table === '' && pn.column !== undefined)
        nombres.push({ nombre: pn.column.toUpperCase(), opcional: true });
      else return ERR.VALUE; // el parametro debe ser un identificador
    }
    // Un opcional no puede ir antes de uno obligatorio.
    const primerOpcional = nombres.findIndex((n) => n.opcional);
    if (primerOpcional >= 0 && nombres.slice(primerOpcional).some((n) => !n.opcional)) {
      return ERR.VALUE;
    }
    if (this.profundidadLambda >= 256) return ERR.NUM;
    const ligadas = new Map(locales ?? []);
    const omitidos = new Set(env.omitidos ?? []);
    for (let i = 0; i < nombres.length; i += 1) {
      const { nombre, opcional } = nombres[i]!;
      const faltaOOmitido = i >= valores.length || omitidosPorPosicion[i] === true;
      if (faltaOOmitido && !opcional) return ERR.VALUE;
      if (faltaOOmitido) {
        ligadas.set(nombre, [['']]);
        omitidos.add(nombre);
      } else {
        ligadas.set(nombre, valores[i]!);
        omitidos.delete(nombre);
      }
    }
    this.profundidadLambda += 1;
    try {
      return this.evalNodeMatrix(cuerpo, { ...env, locals: ligadas, omitidos });
    } finally {
      this.profundidadLambda -= 1;
    }
  }

  /** ISOMITTED(parámetro): ¿el LAMBDA en curso recibió ese parámetro? Solo vale con un
   *  nombre que sea parámetro del LAMBDA (o variable de LET): otra cosa es #NAME?. */
  private evalIsOmitted(node: Extract<AstNode, { kind: 'call' }>, env: Env): CellValue {
    const n = node.args[0];
    if (node.args.length !== 1 || !n) return ERR.VALUE;
    if (n.kind !== 'name') return ERR.VALUE;
    const up = n.name.toUpperCase();
    if (!env.locals?.has(up)) return ERR.NAME;
    return env.omitidos?.has(up) ?? false;
  }

  /** Primer escalar de una matriz (los lambda de MAP/BYROW/… devuelven un valor). */
  private static escalarDe(m: CellValue[][] | FormulaError): CellValue {
    if (m instanceof FormulaError) return m;
    return m[0]?.[0] ?? ERR.VALUE;
  }

  /**
   * MAP, REDUCE, SCAN, MAKEARRAY, BYROW y BYCOL: las que toman una funcion.
   *
   * Todas comparten la misma idea —recorrer algo aplicando un LAMBDA— y se diferencian
   * en QUE recorren y que hacen con el resultado. El limite de celdas es el mismo que
   * usa SEQUENCE: una lambda que genere un millon de celdas cuelga la hoja, y el tope
   * convierte eso en un #NUM! que se entiende.
   */
  private evalOrdenSuperior(
    nombre: string,
    node: Extract<AstNode, { kind: 'call' }>,
    env: Env,
  ): CellValue | CellValue[][] {
    const a = node.args;
    const TOPE = 1_000_000;

    if (nombre === 'MAP') {
      if (a.length !== 2) return ERR.VALUE;
      const m = this.evalNodeMatrix(a[0]!, env);
      const out: CellValue[][] = [];
      for (const fila of m) {
        const nueva: CellValue[] = [];
        for (const v of fila) nueva.push(Engine.escalarDe(this.aplicarLambda(a[1]!, [[[v]]], env)));
        out.push(nueva);
      }
      return out;
    }

    if (nombre === 'REDUCE' || nombre === 'SCAN') {
      if (a.length !== 3) return ERR.VALUE;
      let acc = this.evalNodeMatrix(a[0]!, env);
      const m = this.evalNodeMatrix(a[1]!, env);
      const parciales: CellValue[][] = [];
      for (const fila of m) {
        const linea: CellValue[] = [];
        for (const v of fila) {
          const r = this.aplicarLambda(a[2]!, [acc, [[v]]], env);
          if (r instanceof FormulaError) return r;
          acc = r;
          linea.push(Engine.escalarDe(r));
        }
        parciales.push(linea);
      }
      // REDUCE devuelve SOLO el acumulado final; SCAN, el rastro entero. Esa es toda la
      // diferencia entre las dos, y confundirlas da una forma de resultado distinta.
      return nombre === 'SCAN' ? parciales : Engine.escalarDe(acc);
    }

    if (nombre === 'MAKEARRAY') {
      if (a.length !== 3) return ERR.VALUE;
      const filas = coerceNumber(this.evalNode(a[0]!, env));
      if (isError(filas)) return filas;
      const cols = coerceNumber(this.evalNode(a[1]!, env));
      if (isError(cols)) return cols;
      const R = Math.trunc(filas);
      const C = Math.trunc(cols);
      if (R < 1 || C < 1) return ERR.VALUE;
      if (R * C > TOPE) return ERR.NUM;
      const out: CellValue[][] = [];
      for (let r = 1; r <= R; r += 1) {
        const fila: CellValue[] = [];
        // Los indices que ve la lambda son 1-based, como los de una hoja.
        for (let c = 1; c <= C; c += 1) {
          fila.push(Engine.escalarDe(this.aplicarLambda(a[2]!, [[[r]], [[c]]], env)));
        }
        out.push(fila);
      }
      return out;
    }

    if (nombre === 'BYROW' || nombre === 'BYCOL') {
      if (a.length !== 2) return ERR.VALUE;
      const m = this.evalNodeMatrix(a[0]!, env);
      if (nombre === 'BYROW') {
        // Cada fila entra ENTERA en la lambda y sale un valor: el resultado es una
        // COLUMNA, no una fila. Devolverlo tumbado es el error tipico y solo se ve al
        // derramar junto a los datos.
        return m.map((fila) => [Engine.escalarDe(this.aplicarLambda(a[1]!, [[fila]], env))]);
      }
      const anchura = m[0]?.length ?? 0;
      const salida: CellValue[] = [];
      for (let c = 0; c < anchura; c += 1) {
        const columna = m.map((fila) => fila[c] ?? ERR.NA);
        salida.push(Engine.escalarDe(this.aplicarLambda(a[1]!, [[columna]], env)));
      }
      return [salida]; // BYCOL devuelve una FILA
    }

    return ERR.NAME;
  }

  /**
   * GROUPBY y PIVOTBY (núcleo en functions/agrupar.ts). Lo de aquí es leer los argumentos:
   * las matrices, el agregador —un LAMBDA o el nombre de una función, que también puede
   * ser PERCENTOF, la única que necesita el total— y las opciones, que se pueden omitir
   * (`GROUPBY(A:A,B:B,SUM,,0)`).
   */
  private evalAgrupar(
    nombre: string,
    node: Extract<AstNode, { kind: 'call' }>,
    env: Env,
  ): CellValue | CellValue[][] {
    const a = node.args;
    const esPivot = nombre === 'PIVOTBY';
    // GROUPBY(filas, valores, función, [cabeceras], [total], [orden], [filtro], [relación])
    // PIVOTBY(filas, columnas, valores, función, [cabeceras], [totalF], [ordenF],
    //         [totalC], [ordenC], [filtro], [relativo_a])
    const nObl = esPivot ? 4 : 3;
    const nMax = esPivot ? 11 : 8;
    if (a.length < nObl || a.length > nMax) return ERR.VALUE;
    const dado = (i: number): AstNode | undefined => {
      const x = a[i];
      return x && x.kind !== 'empty' ? x : undefined;
    };
    const matriz = (i: number): CellValue[][] | undefined => {
      const n = dado(i);
      return n ? this.evalNodeMatrix(n, env) : undefined;
    };
    const entero = (i: number): number | FormulaError | undefined => {
      const n = dado(i);
      if (!n) return undefined;
      const v = coerceNumber(this.evalNode(n, env));
      return isError(v) ? v : Math.trunc(v);
    };
    const lista = (i: number): number[] | FormulaError | undefined => {
      const m = matriz(i);
      if (!m) return undefined;
      const out: number[] = [];
      for (const fila of m) {
        for (const v of fila) {
          const x = coerceNumber(v);
          if (isError(x)) return x;
          out.push(Math.trunc(x));
        }
      }
      return out;
    };
    const fnNodo = dado(nObl - 1);
    if (!fnNodo) return ERR.VALUE;
    const pideTodos = fnNodo.kind === 'name' && fnNodo.name.toUpperCase() === 'PERCENTOF';
    const agregar: Agregador = (grupo, todos) => {
      const r = this.aplicarLambda(fnNodo, pideTodos ? [grupo, todos] : [grupo], env);
      return Engine.escalarDe(r);
    };
    const errores = (...xs: unknown[]): FormulaError | undefined =>
      xs.find((x): x is FormulaError => x instanceof FormulaError);

    const filasM = matriz(0);
    if (!filasM) return ERR.VALUE;
    if (esPivot) {
      const colsM = matriz(1);
      const valM = matriz(2);
      if (!colsM || !valM) return ERR.VALUE;
      const cab = entero(4);
      const totF = entero(5);
      const ordF = lista(6);
      const totC = entero(7);
      const ordC = lista(8);
      const err = errores(cab, totF, ordF, totC, ordC);
      if (err) return err;
      const r = pivotBy({
        filas: filasM,
        columnas: colsM,
        valores: valM,
        agregar,
        cabeceras: cab as number | undefined,
        totalFilas: totF as number | undefined,
        ordenFilas: ordF as number[] | undefined,
        totalColumnas: totC as number | undefined,
        ordenColumnas: ordC as number[] | undefined,
        filtro: matriz(9),
      });
      return r;
    }
    const valM = matriz(1);
    if (!valM) return ERR.VALUE;
    const cab = entero(3);
    const tot = entero(4);
    const ord = lista(5);
    const err = errores(cab, tot, ord);
    if (err) return err;
    return groupBy({
      filas: filasM,
      valores: valM,
      agregar,
      cabeceras: cab as number | undefined,
      totales: tot as number | undefined,
      orden: ord as number[] | undefined,
      filtro: matriz(6),
    });
  }

  /**
   * Resuelve un nodo `name` a una matriz: 1º variable local de LET, 2º nombre
   * definido (evaluado anclado en su hoja base). undefined si el nombre no existe.
   */
  private evalNameNode(
    node: Extract<AstNode, { kind: 'name' }>,
    env: Env,
  ): CellValue[][] | undefined {
    const up = node.name.toUpperCase();
    const local = env.locals?.get(up);
    if (local) return local;
    const hit = this.lookupName(up, env.sheet);
    if (!hit) return undefined;
    const { key, def } = hit;
    if (this.resolvingNames.has(key)) return [[ERR.CIRC]]; // nombre recursivo
    this.resolvingNames.add(key);
    try {
      const nEnv: Env = { sheet: def.sheet, col: 0, row: 0, ctx: env.ctx };
      return this.evalNodeMatrix(def.ast, nEnv);
    } finally {
      this.resolvingNames.delete(key);
    }
  }

  private evalNodeMatrix(node: AstNode, env: Env): CellValue[][] {
    if (node.kind === 'array') return node.rows.map((r) => [...r]);
    if (node.kind === 'range') {
      return this.resolveRange(this.rangeToRefRange(node.a, node.b, env.sheet, node.whole));
    }
    if (node.kind === 'structured') {
      const rr = this.resolveTableRef(node, { sheet: env.sheet, col: env.col, row: env.row });
      return rr instanceof FormulaError ? [[rr]] : this.resolveRange(rr);
    }
    if (node.kind === 'ref') {
      const s = node.ref.sheet ?? env.sheet;
      return [[this.evalCell(s, node.ref.col, node.ref.row)]];
    }
    if (node.kind === 'call') {
      const r = this.evalCall(node, env);
      return Array.isArray(r) ? r : [[r]];
    }
    if (node.kind === 'name') {
      return this.evalNameNode(node, env) ?? [[ERR.NAME]];
    }
    if (node.kind === 'binary') {
      return this.evalBinaryMatrix(node, env);
    }
    if (node.kind === 'unary' || node.kind === 'percent') {
      const m = this.evalNodeMatrix(node.operand, env);
      if (m.length <= 1 && (m[0]?.length ?? 0) <= 1) return [[this.evalNode(node, env)]];
      return m.map((row) =>
        row.map((v) => {
          const n = coerceNumber(v);
          if (isError(n)) return n;
          if (node.kind === 'percent') return n / 100;
          return node.op === '-' ? -n : n;
        }),
      );
    }
    return [[this.evalNode(node, env)]];
  }

  private makeArg(node: AstNode, env: Env): Arg {
    let sv: CellValue | undefined;
    let mv: CellValue[][] | undefined;
    let refCache: RefRange | undefined;
    let refComputed = false;
    // Arrow functions para capturar `this` (el motor) léxicamente, sin alias.
    const computeRef = (): RefRange | undefined => {
      if (!refComputed) {
        refComputed = true;
        if (node.kind === 'ref') refCache = this.toRefRange(node.ref, env.sheet);
        else if (node.kind === 'range')
          refCache = this.rangeToRefRange(node.a, node.b, env.sheet, node.whole);
        else if (node.kind === 'structured') {
          const r = this.resolveTableRef(node, { sheet: env.sheet, col: env.col, row: env.row });
          if (!(r instanceof FormulaError)) refCache = r;
        }
      }
      return refCache;
    };
    return {
      value: (): CellValue => {
        if (sv === undefined) sv = collapse(this.evalNodeMatrix(node, env));
        return sv;
      },
      matrix: (): CellValue[][] => {
        if (mv === undefined) mv = this.evalNodeMatrix(node, env);
        return mv;
      },
      get ref(): RefRange | undefined {
        return computeRef();
      },
      literal:
        node.kind === 'str' ||
        node.kind === 'binary' ||
        node.kind === 'unary' ||
        node.kind === 'percent',
    };
  }
}

/** Operador binario sobre dos valores ya evaluados (semántica ECMA-376). */
function applyBinary(op: BinOp, lv: CellValue, rv: CellValue): CellValue {
  if (op === '&') {
    const l = coerceText(lv);
    if (isError(l)) return l;
    const r = coerceText(rv);
    return isError(r) ? r : l + r;
  }
  if (op === '=' || op === '<>' || op === '<' || op === '>' || op === '<=' || op === '>=') {
    // Dos números que coinciden en 15 cifras significativas son iguales (0,1+0,2 = 0,3), como en Excel.
    const cmp =
      typeof lv === 'number' && typeof rv === 'number' && mismoA15(lv, rv)
        ? 0
        : compareValues(lv, rv);
    if (isError(cmp)) return cmp;
    switch (op) {
      case '=':
        return cmp === 0;
      case '<>':
        return cmp !== 0;
      case '<':
        return cmp < 0;
      case '>':
        return cmp > 0;
      case '<=':
        return cmp <= 0;
      case '>=':
        return cmp >= 0;
    }
  }
  const l = coerceNumber(lv);
  if (isError(l)) return l;
  const r = coerceNumber(rv);
  if (isError(r)) return r;
  switch (op) {
    case '+':
      return finito(l + r);
    case '-':
      return finito(l - r);
    case '*':
      return finito(l * r);
    case '/':
      return r === 0 ? ERR.DIV0 : finito(l / r);
    case '^':
      return finito(Math.pow(l, r));
  }
  return ERR.VALUE; // inalcanzable (todos los operadores cubiertos)
}

/** Un desbordamiento (±∞) o un NaN es #NUM!, como en Excel. */
function finito(p: number): CellValue {
  return Number.isFinite(p) ? p : ERR.NUM;
}

/** ¿Iguales a 15 cifras significativas? El atajo descarta casi todo sin formatear. */
function mismoA15(a: number, b: number): boolean {
  if (a === b) return true;
  if (Math.abs(a - b) > 1e-10 * Math.max(Math.abs(a), Math.abs(b))) return false;
  return Number(a.toPrecision(15)) === Number(b.toPrecision(15));
}

/** Colapsa una matriz a escalar: 1×1 → su valor; multi-celda → #VALUE!. */
function collapse(m: CellValue[][]): CellValue {
  if (m.length === 1 && m[0]!.length === 1) return m[0]![0]!;
  if (m.length === 0 || m[0]!.length === 0) return ERR.VALUE;
  return ERR.VALUE;
}

/**
 * Interpreta el texto crudo de una celda literal: número si parsea limpio
 * (admite «%» final), TRUE/FALSE booleano, o el texto tal cual (incl. HTML).
 */
function parseLiteral(raw: string): CellValue {
  const t = raw.trim();
  if (t === '') return '';
  const up = t.toUpperCase();
  if (up === 'TRUE') return true;
  if (up === 'FALSE') return false;
  let s = t;
  let pct = false;
  if (s.endsWith('%')) {
    pct = true;
    s = s.slice(0, -1).trim();
  }
  if (/^[-+]?(\d+\.?\d*|\.\d+)([eE][-+]?\d+)?$/.test(s)) {
    const n = Number(s);
    return pct ? n / 100 : n;
  }
  return raw;
}

// Reexport de conveniencia: convertir índice de columna a letra (para agentes).
export { numToCol };
