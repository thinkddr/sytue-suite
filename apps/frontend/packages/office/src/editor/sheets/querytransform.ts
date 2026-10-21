/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Motor de TRANSFORMACIÓN tabular (estilo Power Query «M» / «pasos aplicados») —
 * lógica PURA, determinista y sin DOM. Un pipeline es una lista de pasos
 * (`QueryStep`, definidos en types.ts) que se aplican EN ORDEN a una tabla
 * { columns, rows } (la estructura del conector: `ExecuteResult` es asignable) y
 * produce la tabla transformada. Se usa al MATERIALIZAR/REFRESCAR una tabla de
 * datos externos (SheetCanvas: comando `extdata`) y para la VISTA PREVIA por paso
 * del gestor «Pasos aplicados».
 *
 * Diseño (decisiones y topes):
 * - Los valores de celda son JSON-nativos (string | number | boolean | null | …).
 *   El texto de una celda se obtiene con `cellText` (mismo criterio que el volcado
 *   a la hoja), así que la vista previa y la hoja coinciden.
 * - `changeType` a FECHA guarda una cadena canónica es-ES (dd/m/aaaa) para que se
 *   VEA como fecha en la hoja y en la previa; las comparaciones/orden de columnas
 *   de tipo 'date' reparsean con `parseEsDate` (orden cronológico correcto).
 * - Errores por paso CLAROS (columna inexistente, etc.): `applyStep` lanza `Error`
 *   con mensaje es-ES; `runPipeline` los captura y señala el índice del paso.
 * - Escala: group-by y quitar-duplicados con Map/Set (O(n), sin bucles anidados);
 *   orden estable con desempate por índice.
 *
 * Self-test: packages/office/test/querytransform.test.ts (bun test).
 */
import type {
  QueryAddColumnStep,
  QueryAggFn,
  QueryAppendStep,
  QueryColType,
  QueryFilterStep,
  QueryGroupByStep,
  QueryJoinType,
  QueryKeepRowsStep,
  QueryMergeStep,
  QueryPivotStep,
  QueryRemoveRowsStep,
  QueryReplaceStep,
  QuerySortStep,
  QueryStep,
  QueryUnpivotStep,
} from '../types';
import { cellText } from '../dataquery';
import { applyNumFmt, parseEsDate, parseEsNumber } from './format';

/** Columna de la tabla que atraviesa el pipeline (nombre + tipo lógico). */
export interface QueryColumn {
  name: string;
  type: string;
}

/**
 * Tabla tabular sobre la que operan los pasos. Coincide en forma con el
 * `ExecuteResult` del conector (columns + filas de valores JSON-nativos), así que
 * el resultado del backend es asignable sin conversión.
 */
export interface QueryTable {
  columns: QueryColumn[];
  rows: unknown[][];
}

/** Resultado de correr un pipeline: la tabla (parcial si algún paso falló) y el
 *  primer error con el índice del paso que lo produjo (null si todo fue bien). */
export interface PipelineResult {
  table: QueryTable;
  error: { index: number; message: string } | null;
}

/* ---------- Resolución de columnas y coerción de valores ---------- */

/** Índice de la columna por nombre (primera coincidencia); lanza si no existe. */
const requireCol = (t: QueryTable, name: string): number => {
  const i = t.columns.findIndex((c) => c.name === name);
  if (i < 0) throw new Error(`La columna «${name}» no existe.`);
  return i;
};

/**
 * Valor numérico de una celda para comparar/ordenar/agregar, según el tipo de su
 * columna ('date' → serial de fecha vía parseEsDate; resto → número es-ES). Los
 * booleanos cuentan como 1/0. null si no es interpretable como número.
 */
const toNumeric = (v: unknown, colType: string): number | null => {
  if (typeof v === 'number') return Number.isFinite(v) ? v : null;
  if (typeof v === 'boolean') return v ? 1 : 0;
  const t = cellText(v);
  return colType === 'date' ? parseEsDate(t) : parseEsNumber(t);
};

/** Número de un OPERANDO de texto según el tipo de la columna (fecha vs número). */
const operandNumeric = (operand: string, colType: string): number | null =>
  colType === 'date' ? parseEsDate(operand) : parseEsNumber(operand);

/** Compara una celda con un operando textual: numérico si ambos lo son; si no,
 *  comparación de texto local (es). Devuelve el signo (-1/0/1). */
const cmpCellOperand = (cell: unknown, operand: string, colType: string): number => {
  const nc = toNumeric(cell, colType);
  const no = operandNumeric(operand, colType);
  if (nc !== null && no !== null) return nc === no ? 0 : nc < no ? -1 : 1;
  return cellText(cell).localeCompare(operand, 'es');
};

/** Compara dos celdas de la MISMA columna (para ordenar): numérico si ambas lo
 *  son (números antes que texto); si no, texto local. Devuelve el signo. */
const cmpCells = (a: unknown, b: unknown, colType: string): number => {
  const na = toNumeric(a, colType);
  const nb = toNumeric(b, colType);
  if (na !== null && nb !== null) return na === nb ? 0 : na < nb ? -1 : 1;
  if (na !== null) return -1;
  if (nb !== null) return 1;
  return cellText(a).localeCompare(cellText(b), 'es');
};

/** ¿Son iguales la celda y el operando? Igualdad numérica si ambos son número
 *  (así «5» y 5 coinciden); si no, igualdad exacta del texto de la celda. */
const cellEqualsOperand = (cell: unknown, operand: string, colType: string): boolean => {
  const nc = toNumeric(cell, colType);
  const no = operandNumeric(operand, colType);
  if (nc !== null && no !== null) return nc === no;
  return cellText(cell) === operand;
};

/* ---------- Conversión de tipos (paso changeType) ---------- */

const TRUE_WORDS = new Set(['true', 'verdadero', 'sí', 'si', 'yes', '1', 'x']);
const FALSE_WORDS = new Set(['false', 'falso', 'no', '0']);

/** Booleano a partir de un valor cualquiera; null si no es reconocible. */
const toBoolean = (v: unknown): boolean | null => {
  if (typeof v === 'boolean') return v;
  const t = cellText(v).trim().toLocaleLowerCase('es');
  if (TRUE_WORDS.has(t)) return true;
  if (FALSE_WORDS.has(t)) return false;
  return null;
};

/** Convierte un valor al tipo pedido; null si no se puede parsear (celda vacía). */
const convertValue = (v: unknown, type: QueryColType): unknown => {
  switch (type) {
    case 'text':
      return cellText(v);
    case 'number':
      return typeof v === 'number' ? v : parseEsNumber(cellText(v));
    case 'boolean':
      return toBoolean(v);
    case 'date': {
      const serial = parseEsDate(cellText(v));
      return serial === null ? null : applyNumFmt({ id: 'date' }, serial);
    }
  }
};

/* ---------- Columna añadida: expresión (constante / interpolación / aritmética) ---------- */

/** Referencia a una columna dentro de una expresión: `{Nombre}`. */
const REF_RE = /\{([^{}]+)\}/g;

/** Nodo del árbol aritmético de una expresión de columna añadida. */
type ExprNode =
  | { t: 'num'; v: number }
  | { t: 'ref'; i: number; type: string }
  | { t: 'bin'; op: '+' | '-' | '*' | '/'; l: ExprNode; r: ExprNode }
  | { t: 'neg'; e: ExprNode };

type ExprToken =
  { t: 'num'; v: number } | { t: 'ref'; i: number; type: string } | { t: 'op'; v: string };

/** Trocea la expresión en tokens (números es-ES, refs `{col}` y operadores). */
const tokenizeExpr = (expr: string, table: QueryTable): ExprToken[] => {
  const toks: ExprToken[] = [];
  let i = 0;
  while (i < expr.length) {
    const ch = expr[i]!;
    if (ch === ' ' || ch === '\t') {
      i++;
      continue;
    }
    if (ch === '{') {
      const end = expr.indexOf('}', i);
      if (end < 0) throw new Error('La expresión tiene un «{» sin cerrar.');
      const idx = requireCol(table, expr.slice(i + 1, end));
      toks.push({ t: 'ref', i: idx, type: table.columns[idx]!.type });
      i = end + 1;
      continue;
    }
    if ('+-*/()'.includes(ch)) {
      toks.push({ t: 'op', v: ch });
      i++;
      continue;
    }
    const m = /^[\d.,]+/.exec(expr.slice(i));
    if (m) {
      const n = parseEsNumber(m[0]);
      if (n === null) throw new Error('La expresión tiene un número no válido.');
      toks.push({ t: 'num', v: n });
      i += m[0].length;
      continue;
    }
    throw new Error(`La expresión tiene un carácter no válido: «${ch}».`);
  }
  return toks;
};

/** Analiza los tokens a un árbol aritmético (descenso recursivo con precedencia). */
const parseExprTokens = (toks: ExprToken[]): ExprNode => {
  let pos = 0;
  const peek = (): ExprToken | undefined => toks[pos];
  const eatOp = (v: string): boolean => {
    const tk = toks[pos];
    if (tk && tk.t === 'op' && tk.v === v) {
      pos++;
      return true;
    }
    return false;
  };
  // primary := num | ref | '(' expr ')'
  const primary = (): ExprNode => {
    const tk = peek();
    if (!tk) throw new Error('La expresión está incompleta.');
    if (tk.t === 'num') {
      pos++;
      return { t: 'num', v: tk.v };
    }
    if (tk.t === 'ref') {
      pos++;
      return { t: 'ref', i: tk.i, type: tk.type };
    }
    if (eatOp('(')) {
      const e = expr();
      if (!eatOp(')')) throw new Error('Falta un «)» en la expresión.');
      return e;
    }
    throw new Error('La expresión no es válida.');
  };
  // unary := '-' unary | '+' unary | primary
  const unary = (): ExprNode => {
    if (eatOp('-')) return { t: 'neg', e: unary() };
    if (eatOp('+')) return unary();
    return primary();
  };
  // term := unary (('*'|'/') unary)*
  const term = (): ExprNode => {
    let node = unary();
    for (;;) {
      if (eatOp('*')) node = { t: 'bin', op: '*', l: node, r: unary() };
      else if (eatOp('/')) node = { t: 'bin', op: '/', l: node, r: unary() };
      else return node;
    }
  };
  // expr := term (('+'|'-') term)*
  const expr = (): ExprNode => {
    let node = term();
    for (;;) {
      if (eatOp('+')) node = { t: 'bin', op: '+', l: node, r: term() };
      else if (eatOp('-')) node = { t: 'bin', op: '-', l: node, r: term() };
      else return node;
    }
  };
  const root = expr();
  if (pos !== toks.length) throw new Error('La expresión tiene tokens sobrantes.');
  return root;
};

/** Evalúa el árbol para una fila; null si algún ref no es numérico o hay /0. */
const evalNode = (node: ExprNode, row: unknown[]): number | null => {
  switch (node.t) {
    case 'num':
      return node.v;
    case 'ref':
      return toNumeric(row[node.i], node.type);
    case 'neg': {
      const e = evalNode(node.e, row);
      return e === null ? null : -e;
    }
    case 'bin': {
      const l = evalNode(node.l, row);
      const r = evalNode(node.r, row);
      if (l === null || r === null) return null;
      const out =
        node.op === '+' ? l + r : node.op === '-' ? l - r : node.op === '*' ? l * r : l / r;
      return Number.isFinite(out) ? out : null;
    }
  }
};

/** Evaluador de columna añadida ya compilado: tipo de salida + función por fila. */
interface CompiledExpr {
  type: 'number' | 'text';
  run: (row: unknown[]) => unknown;
}

/**
 * Compila la expresión de una columna añadida (una vez por paso). Modos:
 *  - ARITMÉTICO si contiene un operador y solo caracteres aritméticos tras quitar
 *    los `{ref}` (p. ej. «{precio} * 1,21»); o si es un número constante.
 *  - TEXTO en otro caso: interpola cada `{col}` con su texto (constante o
 *    concatenación, p. ej. «{nombre} {apellido}»).
 * Valida que cada columna referida exista (lanza si no).
 */
const compileExpr = (rawExpr: string, table: QueryTable): CompiledExpr => {
  const expr = rawExpr.trim();
  const hasRef = /\{[^{}]+\}/.test(expr);
  const skeleton = expr.replace(REF_RE, ' 0 ');
  const numericConst = !hasRef && /^[+-]?[\d.,]+$/.test(expr) && parseEsNumber(expr) !== null;
  const arithmetic =
    numericConst || (/[-+*/]/.test(skeleton) && /^[\s\d.,+\-*/()]*$/.test(skeleton));

  if (arithmetic) {
    const ast = parseExprTokens(tokenizeExpr(expr, table));
    return { type: 'number', run: (row) => evalNode(ast, row) };
  }
  // Modo texto: precompila los tramos (literal / ref) para interpolar por fila.
  const parts: ({ lit: string } | { ref: number })[] = [];
  let last = 0;
  for (const m of expr.matchAll(REF_RE)) {
    if (m.index > last) parts.push({ lit: expr.slice(last, m.index) });
    parts.push({ ref: requireCol(table, m[1]!) });
    last = m.index + m[0].length;
  }
  if (last < expr.length) parts.push({ lit: expr.slice(last) });
  return {
    type: 'text',
    run: (row) => parts.map((p) => ('lit' in p ? p.lit : cellText(row[p.ref]))).join(''),
  };
};

/* ---------- Filtrado de filas ---------- */

/** ¿Pasa la celda la condición del filtro? */
const passesFilter = (cell: unknown, step: QueryFilterStep, colType: string): boolean => {
  const value = step.value ?? '';
  switch (step.op) {
    case 'empty':
      return cellText(cell) === '';
    case 'nonempty':
      return cellText(cell) !== '';
    case 'contains':
      return cellText(cell).toLocaleLowerCase('es').includes(value.toLocaleLowerCase('es'));
    case 'eq':
      return cellEqualsOperand(cell, value, colType);
    case 'neq':
      return !cellEqualsOperand(cell, value, colType);
    case 'gt':
      return cmpCellOperand(cell, value, colType) > 0;
    case 'lt':
      return cmpCellOperand(cell, value, colType) < 0;
    case 'between':
      return (
        cmpCellOperand(cell, value, colType) >= 0 &&
        cmpCellOperand(cell, step.value2 ?? '', colType) <= 0
      );
  }
};

/* ---------- Agrupación (group-by) ---------- */

/** Agrega los valores de una columna en un grupo según la función pedida. Para
 *  min/max devuelve el VALOR de celda original del extremo (conserva la fecha). */
const aggregate = (
  fn: QueryAggFn,
  rows: readonly unknown[][],
  colIdx: number,
  colType: string,
): unknown => {
  if (fn === 'count') return rows.length;
  if (fn === 'first') return rows.length === 0 ? null : rows[0]![colIdx]; // valor de celda original
  if (fn === 'min' || fn === 'max') {
    let best: { n: number; v: unknown } | null = null;
    for (const r of rows) {
      const n = toNumeric(r[colIdx], colType);
      if (n === null) continue;
      if (!best || (fn === 'min' ? n < best.n : n > best.n)) best = { n, v: r[colIdx] };
    }
    return best ? best.v : null;
  }
  let sum = 0;
  let count = 0;
  for (const r of rows) {
    const n = toNumeric(r[colIdx], colType);
    if (n === null) continue;
    sum += n;
    count++;
  }
  if (fn === 'sum') return sum;
  return count === 0 ? null : sum / count; // avg
};

/** Tipo de salida de una columna de agregación (min/max/first heredan el de su
 *  origen; count/sum/avg son numéricos). */
const aggColType = (fn: QueryAggFn, sourceType: string): string =>
  fn === 'min' || fn === 'max' || fn === 'first' ? sourceType : 'number';

/* ---------- Aplicación de un paso ---------- */

/** Proyecta la tabla a un subconjunto/reorden de columnas por índice de origen. */
const projectColumns = (t: QueryTable, idxs: number[]): QueryTable => ({
  columns: idxs.map((i) => t.columns[i]!),
  rows: t.rows.map((r) => idxs.map((i) => r[i])),
});

const sortStep = (t: QueryTable, step: QuerySortStep): QueryTable => {
  if (step.by.length === 0) return t;
  const keys = step.by.map((k) => {
    const i = requireCol(t, k.column);
    return { i, dir: k.dir, type: t.columns[i]!.type };
  });
  const indexed = t.rows.map((r, idx) => ({ r, idx }));
  indexed.sort((a, b) => {
    for (const k of keys) {
      const c = cmpCells(a.r[k.i], b.r[k.i], k.type);
      if (c !== 0) return k.dir === 'asc' ? c : -c;
    }
    return a.idx - b.idx; // desempate estable
  });
  return { columns: t.columns, rows: indexed.map((x) => x.r) };
};

const groupByStep = (t: QueryTable, step: QueryGroupByStep): QueryTable => {
  const keyIdx = step.columns.map((c) => requireCol(t, c));
  const aggMeta = step.aggs.map((a) => {
    const src = a.fn === 'count' && a.column === '' ? -1 : requireCol(t, a.column);
    return { fn: a.fn, as: a.as, i: src, srcType: src >= 0 ? t.columns[src]!.type : 'number' };
  });
  // Un grupo por clave (texto de las columnas clave); orden de PRIMERA aparición.
  const groups = new Map<string, { key: unknown[]; rows: unknown[][] }>();
  for (const row of t.rows) {
    const kk = JSON.stringify(keyIdx.map((i) => cellText(row[i])));
    let g = groups.get(kk);
    if (!g) {
      g = { key: keyIdx.map((i) => row[i]), rows: [] };
      groups.set(kk, g);
    }
    g.rows.push(row);
  }
  const columns: QueryColumn[] = [
    ...keyIdx.map((i) => t.columns[i]!),
    ...aggMeta.map((a) => ({ name: a.as, type: aggColType(a.fn, a.srcType) })),
  ];
  const rows = [...groups.values()].map((g) => [
    ...g.key,
    ...aggMeta.map((a) => aggregate(a.fn, g.rows, a.i, a.srcType)),
  ]);
  return { columns, rows };
};

const addColumnStep = (t: QueryTable, step: QueryAddColumnStep): QueryTable => {
  if (step.name.trim() === '') throw new Error('El nombre de la columna nueva está vacío.');
  const compiled = compileExpr(step.expr, t); // valida refs (lanza si faltan)
  return {
    columns: [...t.columns, { name: step.name, type: compiled.type }],
    rows: t.rows.map((r) => [...r, compiled.run(r)]),
  };
};

const replaceStep = (t: QueryTable, step: QueryReplaceStep): QueryTable => {
  const i = requireCol(t, step.column);
  if (step.find === '') return t; // sin patrón no hay nada que reemplazar (evita no-op costoso)
  return {
    columns: t.columns.map((c, j) => (j === i ? { ...c, type: 'text' } : c)),
    rows: t.rows.map((r) =>
      r.map((v, j) => (j === i ? cellText(v).split(step.find).join(step.replace) : v)),
    ),
  };
};

const keepRowsStep = (t: QueryTable, step: QueryKeepRowsStep): QueryTable => {
  const n = Math.max(0, Math.trunc(step.count));
  const rows =
    step.end === 'first' ? t.rows.slice(0, n) : t.rows.slice(Math.max(0, t.rows.length - n));
  return { columns: t.columns, rows };
};

const removeRowsStep = (t: QueryTable, step: QueryRemoveRowsStep): QueryTable => {
  const n = Math.max(0, Math.trunc(step.count));
  const rows =
    step.end === 'first' ? t.rows.slice(n) : t.rows.slice(0, Math.max(0, t.rows.length - n));
  return { columns: t.columns, rows };
};

/* ---------- Combinar/reestructurar: unpivot / pivot / append / merge ---------- */

/**
 * Resuelve la tabla de OTRA consulta (append/merge) por su referencia (nombre
 * único de la ExternalTable) a su instantánea { columns, rows }. `undefined` = no
 * encontrada. PUEDE LANZAR (p. ej. un resolutor transitivo que detecta un ciclo):
 * el error se propaga como error del paso. El resolutor de producción usa el
 * SNAPSHOT ya materializado de la otra tabla (no re-ejecuta su pipeline en cadena;
 * la re-evaluación transitiva es un follow-up), así que no puede colgarse.
 */
export type QueryResolver = (ref: string) => QueryTable | undefined;

/** Resuelve la otra tabla o lanza un error de paso CLARO (sin resolutor / no existe). */
const resolveOther = (ref: string, resolve: QueryResolver | undefined): QueryTable => {
  const name = ref.trim();
  if (name === '') throw new Error('No se ha indicado la otra tabla.');
  if (!resolve) throw new Error(`No se puede resolver la tabla «${name}» (falta el resolutor).`);
  const other = resolve(name); // puede lanzar (ciclo en un resolutor transitivo)
  if (!other) throw new Error(`La tabla «${name}» no existe o no está materializada.`);
  return other;
};

/** Nombre único que no esté en `used`: si colisiona antepone `prefix`; si aún
 *  colisiona (o sin prefijo), añade « (2)», « (3)»… */
const uniqueName = (name: string, used: ReadonlySet<string>, prefix: string): string => {
  let candidate = used.has(name) && prefix !== '' ? `${prefix}${name}` : name;
  const base = candidate;
  let n = 2;
  while (used.has(candidate)) {
    candidate = `${base} (${n})`;
    n++;
  }
  return candidate;
};

/**
 * ANEXAR: apila las filas de la otra tabla debajo, alineando por NOMBRE de columna.
 * Columnas = unión (las de esta tabla + las de la otra que no aparezcan); las
 * columnas ausentes en una de las dos se rellenan con null. Colisión de nombre → se
 * conserva el TIPO de la columna izquierda (documentado).
 */
const appendStep = (t: QueryTable, step: QueryAppendStep, resolve?: QueryResolver): QueryTable => {
  const other = resolveOther(step.from, resolve);
  const columns: QueryColumn[] = t.columns.map((c) => ({ ...c }));
  const indexByName = new Map<string, number>();
  t.columns.forEach((c, i) => {
    if (!indexByName.has(c.name)) indexByName.set(c.name, i);
  });
  for (const c of other.columns) {
    if (!indexByName.has(c.name)) {
      indexByName.set(c.name, columns.length);
      columns.push({ name: c.name, type: c.type });
    }
  }
  const width = columns.length;
  const topRows = t.rows.map((r) => {
    const out = new Array<unknown>(width).fill(null);
    for (let i = 0; i < t.columns.length; i++) out[i] = r[i];
    return out;
  });
  const otherIdx = other.columns.map((c) => indexByName.get(c.name)!);
  const botRows = other.rows.map((r) => {
    const out = new Array<unknown>(width).fill(null);
    for (let j = 0; j < other.columns.length; j++) out[otherIdx[j]!] = r[j];
    return out;
  });
  return { columns, rows: [...topRows, ...botRows] };
};

/** Índice de una columna de la tabla DERECHA por nombre; lanza si no existe. */
const requireRightCol = (right: QueryTable, name: string): number => {
  const i = right.columns.findIndex((c) => c.name === name);
  if (i < 0) throw new Error(`La columna «${name}» no existe en la otra tabla.`);
  return i;
};

/**
 * COMBINAR (merge/join): empareja por pares de columnas clave (comparación por
 * TEXTO de celda, como group-by/dedup: «5» coincide con 5). Semántica de múltiples
 * coincidencias = PRODUCTO (una fila por par). Tipos:
 *  - inner: solo filas con coincidencia.
 *  - left:  todas las de la izquierda (sin par → columnas traídas a null).
 *  - right: todas las de la derecha (sin par → columnas de la izquierda a null).
 *  - full:  unión de left y right.
 *  - antiLeft: SOLO filas de la izquierda SIN coincidencia (no trae columnas).
 */
const mergeStep = (t: QueryTable, step: QueryMergeStep, resolve?: QueryResolver): QueryTable => {
  const right = resolveOther(step.from, resolve);
  if (step.on.length === 0)
    throw new Error('La combinación necesita al menos un par de columnas clave.');
  const leftKeys = step.on.map((p) => requireCol(t, p.left));
  const rightKeys = step.on.map((p) => requireRightCol(right, p.right));
  const isAnti = step.join === 'antiLeft';

  // Columnas de la derecha a traer: las pedidas (que existan) o todas menos las
  // usadas como clave. antiLeft no trae ninguna (es un filtro de la izquierda).
  const bringIdx: number[] = [];
  if (!isAnti) {
    if (step.bring.length > 0)
      for (const name of step.bring) bringIdx.push(requireRightCol(right, name));
    else {
      const keySet = new Set(rightKeys);
      right.columns.forEach((_, i) => {
        if (!keySet.has(i)) bringIdx.push(i);
      });
    }
  }
  // Nombres de salida: los de la izquierda + los traídos (prefijo/sufijo ante colisión).
  const usedNames = new Set(t.columns.map((c) => c.name));
  const prefix = step.prefix ?? '';
  const broughtCols: QueryColumn[] = bringIdx.map((i) => {
    const src = right.columns[i]!;
    const name = uniqueName(src.name, usedNames, prefix);
    usedNames.add(name);
    return { name, type: src.type };
  });
  const columns: QueryColumn[] = [...t.columns, ...broughtCols];

  // Multimap de la derecha por clave (texto). O(n+m), sin bucles anidados.
  const keyText = (row: unknown[], idxs: number[]): string =>
    JSON.stringify(idxs.map((i) => cellText(row[i])));
  const rightByKey = new Map<string, unknown[][]>();
  for (const r of right.rows) {
    const k = keyText(r, rightKeys);
    const arr = rightByKey.get(k);
    if (arr) arr.push(r);
    else rightByKey.set(k, [r]);
  }
  const matched = new Set<string>();
  const bringVals = (r: unknown[] | null): unknown[] => bringIdx.map((i) => (r ? r[i] : null));
  const leftNulls = (): unknown[] => new Array<unknown>(t.columns.length).fill(null);

  const rows: unknown[][] = [];
  for (const lr of t.rows) {
    const k = keyText(lr, leftKeys);
    const ms = rightByKey.get(k);
    if (ms && ms.length > 0) {
      matched.add(k);
      if (isAnti) continue; // anti-left: solo filas SIN coincidencia
      for (const rr of ms) rows.push([...lr, ...bringVals(rr)]); // producto (multi-match)
    } else if (isAnti || step.join === 'left' || step.join === 'full') {
      rows.push(isAnti ? [...lr] : [...lr, ...bringVals(null)]);
    }
    // inner / right: la fila izquierda sin par se descarta aquí.
  }
  // Cola: filas de la derecha sin par (en su orden original) para right/full.
  if (step.join === 'right' || step.join === 'full') {
    for (const rr of right.rows) {
      if (!matched.has(keyText(rr, rightKeys))) rows.push([...leftNulls(), ...bringVals(rr)]);
    }
  }
  return { columns, rows };
};

/**
 * PIVOTAR: los valores distintos de la columna clave (orden de APARICIÓN) pasan a
 * columnas; cada celda agrega (`fn`) los valores de la columna de valor de las
 * filas que comparten esa clave. Las filas se agrupan por las columnas RESTANTES
 * (orden de primera aparición). Combinación clave×grupo ausente → celda vacía (null).
 */
const pivotStep = (t: QueryTable, step: QueryPivotStep): QueryTable => {
  const keyIdx = requireCol(t, step.key);
  const valIdx = requireCol(t, step.value);
  if (keyIdx === valIdx) throw new Error('La columna clave y la de valor deben ser distintas.');
  const restIdx = t.columns.map((_, i) => i).filter((i) => i !== keyIdx && i !== valIdx);
  const valType = t.columns[valIdx]!.type;
  // Nombres de columna nuevos = valores distintos de la clave, en orden de aparición.
  const pivotNames: string[] = [];
  const pivotPos = new Map<string, number>();
  for (const r of t.rows) {
    const kt = cellText(r[keyIdx]);
    if (!pivotPos.has(kt)) {
      pivotPos.set(kt, pivotNames.length);
      pivotNames.push(kt);
    }
  }
  // Un grupo por las columnas restantes; en cada grupo, celdas-valor por columna nueva.
  const groups = new Map<string, { key: unknown[]; buckets: unknown[][][] }>();
  for (const r of t.rows) {
    const gk = JSON.stringify(restIdx.map((i) => cellText(r[i])));
    let g = groups.get(gk);
    if (!g) {
      g = { key: restIdx.map((i) => r[i]), buckets: pivotNames.map(() => []) };
      groups.set(gk, g);
    }
    g.buckets[pivotPos.get(cellText(r[keyIdx]))!]!.push([r[valIdx]]); // fila mínima [valor]
  }
  const columns: QueryColumn[] = [
    ...restIdx.map((i) => t.columns[i]!),
    ...pivotNames.map((n) => ({ name: n, type: aggColType(step.fn, valType) })),
  ];
  const rows = [...groups.values()].map((g) => [
    ...g.key,
    ...g.buckets.map((cells) =>
      cells.length === 0 ? null : aggregate(step.fn, cells, 0, valType),
    ),
  ]);
  return { columns, rows };
};

/**
 * DESPIVOTAR: colapsa las columnas «atributo» en dos columnas (nombre + valor),
 * repitiendo las columnas ancla. Conserva TODAS las celdas (incluidos null/vacío;
 * no descarta como sí hace el «Unpivot» por defecto de Power Query) para que sea
 * reversible con `pivot`. Tipo de la columna valor = el común de las despivotadas
 * si es uniforme; si no, 'text'.
 */
const unpivotStep = (t: QueryTable, step: QueryUnpivotStep): QueryTable => {
  if (step.columns.length === 0) throw new Error('Elige al menos una columna para despivotar.');
  const attrName = step.attribute.trim();
  const valName = step.value.trim();
  if (attrName === '' || valName === '')
    throw new Error('Los nombres de las columnas atributo y valor no pueden estar vacíos.');
  const unpivotIdx = step.columns.map((c) => requireCol(t, c));
  const unpivotSet = new Set(unpivotIdx);
  const anchorIdx = t.columns.map((_, i) => i).filter((i) => !unpivotSet.has(i));
  const types = new Set(unpivotIdx.map((i) => t.columns[i]!.type));
  const valType = types.size === 1 ? [...types][0]! : 'text';
  const columns: QueryColumn[] = [
    ...anchorIdx.map((i) => t.columns[i]!),
    { name: attrName, type: 'text' },
    { name: valName, type: valType },
  ];
  const rows: unknown[][] = [];
  for (const r of t.rows) {
    const anchorVals = anchorIdx.map((i) => r[i]);
    for (const ui of unpivotIdx) rows.push([...anchorVals, t.columns[ui]!.name, r[ui]]);
  }
  return { columns, rows };
};

/**
 * Aplica UN paso a la tabla y devuelve la tabla resultante (NUEVA; no muta la
 * entrada). Lanza `Error` con mensaje es-ES si el paso no es aplicable (columna
 * inexistente, expresión inválida, otra tabla no resoluble, etc.). `resolve` es el
 * resolutor de OTRAS tablas para append/merge (opcional; los demás pasos lo ignoran).
 */
export const applyStep = (
  table: QueryTable,
  step: QueryStep,
  resolve?: QueryResolver,
): QueryTable => {
  switch (step.kind) {
    case 'filter': {
      const i = requireCol(table, step.column);
      const type = table.columns[i]!.type;
      return {
        columns: table.columns,
        rows: table.rows.filter((r) => passesFilter(r[i], step, type)),
      };
    }
    case 'select':
      return projectColumns(
        table,
        step.columns.map((c) => requireCol(table, c)),
      );
    case 'removeColumns': {
      const drop = new Set(step.columns.map((c) => requireCol(table, c)));
      return projectColumns(
        table,
        table.columns.map((_, i) => i).filter((i) => !drop.has(i)),
      );
    }
    case 'reorder': {
      const lead: number[] = [];
      for (const c of step.columns) {
        const i = requireCol(table, c);
        if (!lead.includes(i)) lead.push(i);
      }
      const leadSet = new Set(lead);
      const rest = table.columns.map((_, i) => i).filter((i) => !leadSet.has(i));
      return projectColumns(table, [...lead, ...rest]);
    }
    case 'rename': {
      const i = requireCol(table, step.column);
      if (step.to.trim() === '') throw new Error('El nuevo nombre de columna está vacío.');
      return {
        columns: table.columns.map((c, j) => (j === i ? { ...c, name: step.to } : c)),
        rows: table.rows,
      };
    }
    case 'changeType': {
      const i = requireCol(table, step.column);
      return {
        columns: table.columns.map((c, j) => (j === i ? { ...c, type: step.type } : c)),
        rows: table.rows.map((r) => r.map((v, j) => (j === i ? convertValue(v, step.type) : v))),
      };
    }
    case 'sort':
      return sortStep(table, step);
    case 'groupBy':
      return groupByStep(table, step);
    case 'addColumn':
      return addColumnStep(table, step);
    case 'replace':
      return replaceStep(table, step);
    case 'dedupe': {
      const idxs =
        step.columns && step.columns.length > 0
          ? step.columns.map((c) => requireCol(table, c))
          : table.columns.map((_, i) => i);
      const seen = new Set<string>();
      const rows: unknown[][] = [];
      for (const r of table.rows) {
        const key = JSON.stringify(idxs.map((i) => cellText(r[i])));
        if (seen.has(key)) continue;
        seen.add(key);
        rows.push(r);
      }
      return { columns: table.columns, rows };
    }
    case 'keepRows':
      return keepRowsStep(table, step);
    case 'removeRows':
      return removeRowsStep(table, step);
    case 'unpivot':
      return unpivotStep(table, step);
    case 'pivot':
      return pivotStep(table, step);
    case 'append':
      return appendStep(table, step, resolve);
    case 'merge':
      return mergeStep(table, step, resolve);
  }
};

/**
 * Aplica la lista de pasos EN ORDEN. Si un paso lanza, se detiene y devuelve la
 * tabla hasta el paso anterior + el error con su índice (para la vista previa y
 * los mensajes del gestor). Sin pasos = tabla de entrada (retrocompatible).
 */
export const runPipeline = (
  input: QueryTable,
  steps: readonly QueryStep[],
  resolve?: QueryResolver,
): PipelineResult => {
  let table = input;
  for (let i = 0; i < steps.length; i++) {
    try {
      table = applyStep(table, steps[i]!, resolve);
    } catch (e) {
      return {
        table,
        error: { index: i, message: e instanceof Error ? e.message : 'Paso no válido.' },
      };
    }
  }
  return { table, error: null };
};

/**
 * Aplica el pipeline y devuelve SOLO la tabla transformada; si algún paso falla,
 * devuelve la tabla de ENTRADA sin transformar (el materializado/refresco nunca
 * deja la hoja a medias; el error se ve en el gestor «Pasos aplicados»).
 */
export const applyTransform = (
  input: QueryTable,
  steps: readonly QueryStep[],
  resolve?: QueryResolver,
): QueryTable => {
  const { table, error } = runPipeline(input, steps, resolve);
  return error ? input : table;
};

/* ---------- Presentación (para el gestor de pasos) ---------- */

const FILTER_OP_TEXT: Record<QueryFilterStep['op'], string> = {
  eq: '=',
  neq: '≠',
  contains: 'contiene',
  gt: '>',
  lt: '<',
  between: 'entre',
  empty: 'vacío',
  nonempty: 'no vacío',
};

const AGG_FN_TEXT: Record<QueryAggFn, string> = {
  sum: 'suma',
  count: 'contar',
  avg: 'promedio',
  min: 'mín',
  max: 'máx',
  first: 'primero',
};

const JOIN_TEXT: Record<QueryJoinType, string> = {
  inner: 'interna',
  left: 'izquierda',
  right: 'derecha',
  full: 'completa',
  antiLeft: 'anti-izquierda',
};

const COL_TYPE_TEXT: Record<QueryColType, string> = {
  text: 'texto',
  number: 'número',
  date: 'fecha',
  boolean: 'booleano',
};

/** Descripción breve y legible (es-ES) de un paso, para la lista del gestor. */
export const describeStep = (step: QueryStep): string => {
  switch (step.kind) {
    case 'filter': {
      const op = FILTER_OP_TEXT[step.op];
      if (step.op === 'empty' || step.op === 'nonempty') return `Filtrar «${step.column}» ${op}`;
      if (step.op === 'between')
        return `Filtrar «${step.column}» entre ${step.value ?? ''} y ${step.value2 ?? ''}`;
      return `Filtrar «${step.column}» ${op} ${step.value ?? ''}`;
    }
    case 'select':
      return `Elegir columnas: ${step.columns.join(', ')}`;
    case 'removeColumns':
      return `Quitar columnas: ${step.columns.join(', ')}`;
    case 'rename':
      return `Renombrar «${step.column}» → «${step.to}»`;
    case 'reorder':
      return `Reordenar columnas: ${step.columns.join(', ')}`;
    case 'changeType':
      return `Tipo de «${step.column}» → ${COL_TYPE_TEXT[step.type]}`;
    case 'sort':
      return `Ordenar por ${step.by.map((b) => `${b.column} ${b.dir === 'asc' ? '↑' : '↓'}`).join(', ')}`;
    case 'groupBy':
      return `Agrupar por ${step.columns.join(', ')} (${step.aggs
        .map((a) => `${AGG_FN_TEXT[a.fn]} → ${a.as}`)
        .join(', ')})`;
    case 'addColumn':
      return `Añadir columna «${step.name}» = ${step.expr}`;
    case 'replace':
      return `Reemplazar en «${step.column}»: «${step.find}» → «${step.replace}»`;
    case 'dedupe':
      return step.columns && step.columns.length > 0
        ? `Quitar duplicados por ${step.columns.join(', ')}`
        : 'Quitar filas duplicadas';
    case 'keepRows':
      return `Mantener ${step.end === 'first' ? 'primeras' : 'últimas'} ${step.count} filas`;
    case 'removeRows':
      return `Quitar ${step.end === 'first' ? 'primeras' : 'últimas'} ${step.count} filas`;
    case 'unpivot':
      return `Despivotar ${step.columns.join(', ')} → «${step.attribute}»/«${step.value}»`;
    case 'pivot':
      return `Pivotar «${step.key}» (${AGG_FN_TEXT[step.fn]} de «${step.value}»)`;
    case 'append':
      return `Anexar «${step.from}»`;
    case 'merge':
      return `Combinar «${step.from}» (${JOIN_TEXT[step.join]}) por ${step.on
        .map((p) => `${p.left}=${p.right}`)
        .join(', ')}`;
  }
};
