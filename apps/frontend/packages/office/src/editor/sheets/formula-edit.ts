/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Helpers PUROS de la UX de edición de fórmulas in-cell (sin DOM ni estado de
 * UI): reciben (texto, posición del cursor) y devuelven datos para el
 * autocompletado, el hint de argumentos, el point-and-click de referencias, el
 * ciclado F4 y el resaltado por color.
 *
 * El AST de `parseFormula` (@sytue/formula) NO lleva offsets de origen, que aquí
 * son imprescindibles (tintar el texto de cada ref, localizar la ref bajo el
 * cursor). Por eso se replica el tokenizador del parser (parser.ts) añadiendo
 * posiciones; las reglas léxicas son las MISMAS (mismos operadores, mismas
 * palabras, mismas comillas), así no diverge. Para normalizar una ref a
 * rectángulo se reutiliza `parseRangeRef` de ./format y `parseCellWord`/
 * `numToCol` del motor (nunca regex de refs a mano).
 *
 * Self-test: packages/office/test/formula-edit.test.ts (bun test).
 */
import { numToCol, parseBracketSpec, parseCellWord, parseRefEnd } from '@sytue/formula';
import type { CellRef, RefEnd, TableArea } from '@sytue/formula';
import { parseRangeRef } from './format';
import type { RangeRect } from './format';

/* ---------- Tokenizador con posiciones (réplica del lexer del parser) ---------- */

type TokType =
  | 'num'
  | 'str'
  | 'qsheet'
  | 'word'
  | 'op'
  | 'lparen'
  | 'rparen'
  | 'sep'
  | 'colon'
  | 'bang'
  | 'percent';

/** Token de la fórmula con su posición absoluta en el texto (incluido el «=»). */
export interface FxTok {
  t: TokType;
  /** Texto del token tal cual (con comillas en str/qsheet). */
  v: string;
  start: number;
  /** Offset exclusivo (fin). */
  end: number;
}

const WORD_START = /[A-Za-z_$]/;
const WORD_REST = /[A-Za-z0-9_$.]/;
const DIGIT = /[0-9]/;

/**
 * Tokeniza el texto (con o sin «=» inicial) con offsets absolutos. Nunca lanza:
 * un carácter inesperado o una cadena/comilla sin cerrar se toleran (el texto
 * está a medio teclear). Espejo de `lex()` del parser salvo por eso.
 */
export function tokenizeFx(src: string): FxTok[] {
  const toks: FxTok[] = [];
  const n = src.length;
  let i = 0;
  const push = (t: TokType, start: number, end: number): void => {
    toks.push({ t, v: src.slice(start, end), start, end });
  };
  while (i < n) {
    const c = src[i]!;
    if (c === ' ' || c === '\t' || c === '\n' || c === '\r') {
      i++;
      continue;
    }
    if (DIGIT.test(c) || (c === '.' && DIGIT.test(src[i + 1] ?? ''))) {
      let j = i + 1;
      while (j < n && /[0-9.]/.test(src[j]!)) j++;
      if (src[j] === 'e' || src[j] === 'E') {
        j++;
        if (src[j] === '+' || src[j] === '-') j++;
        while (j < n && DIGIT.test(src[j]!)) j++;
      }
      push('num', i, j);
      i = j;
      continue;
    }
    if (c === '"') {
      let j = i + 1;
      while (j < n) {
        if (src[j] === '"') {
          if (src[j + 1] === '"') {
            j += 2;
            continue;
          }
          j++;
          break;
        }
        j++;
      }
      push('str', i, j);
      i = j;
      continue;
    }
    if (c === "'") {
      let j = i + 1;
      while (j < n) {
        if (src[j] === "'") {
          if (src[j + 1] === "'") {
            j += 2;
            continue;
          }
          j++;
          break;
        }
        j++;
      }
      push('qsheet', i, j);
      i = j;
      continue;
    }
    const two = src.slice(i, i + 2);
    if (two === '<=' || two === '>=' || two === '<>') {
      push('op', i, i + 2);
      i += 2;
      continue;
    }
    if ('+-*/^&=<>'.includes(c)) {
      push('op', i, i + 1);
      i++;
      continue;
    }
    if (c === '(') {
      push('lparen', i, i + 1);
      i++;
      continue;
    }
    if (c === ')') {
      push('rparen', i, i + 1);
      i++;
      continue;
    }
    if (c === ',' || c === ';') {
      push('sep', i, i + 1);
      i++;
      continue;
    }
    if (c === ':') {
      push('colon', i, i + 1);
      i++;
      continue;
    }
    if (c === '!') {
      push('bang', i, i + 1);
      i++;
      continue;
    }
    if (c === '%') {
      push('percent', i, i + 1);
      i++;
      continue;
    }
    if (WORD_START.test(c)) {
      let j = i + 1;
      while (j < n && WORD_REST.test(src[j]!)) j++;
      push('word', i, j);
      i = j;
      continue;
    }
    i++; // carácter desconocido: se ignora (fórmula a medio escribir)
  }
  return toks;
}

/** Fila/columna «entera» de una ref de eje entero: el render de rangos la ACOTA
 *  (Math.min con ROWS/COLS), así el resaltado abarca lo visible sin fijar aquí el
 *  tamaño real de la hoja. 2^20-1 = última fila/columna posible (paridad Excel). */
const WHOLE_AXIS = 1_048_575;

/** Tipo de referencia de un token (word o num) como extremo: celda/columna/fila,
 *  o null si no lo es. Delega en `parseRefEnd` del motor (única fuente de verdad;
 *  una fila desnuda «3» llega como el texto del token numérico). */
const endpointKind = (tok: FxTok | undefined): RefEnd['kind'] | null =>
  tok && (tok.t === 'word' || tok.t === 'num') ? (parseRefEnd(tok.v)?.kind ?? null) : null;

/**
 * Rectángulo 0-based de una referencia CRUDA (sin prefijo de hoja): celda o rango
 * A1 (vía parseRangeRef), o de EJE ENTERO — A:A/A:C → columnas (filas libres a
 * WHOLE_AXIS), 1:1/3:5 → filas (columnas libres). Reutiliza parseRefEnd del motor
 * (nunca regex de refs a mano). null si no parsea o si mezcla tipos de extremo.
 */
const rangeRectOf = (bare: string): RangeRect | null => {
  const i = bare.indexOf(':');
  if (i < 0) return parseRangeRef(bare); // celda sola (retrocompat)
  const a = parseRefEnd(bare.slice(0, i));
  const b = parseRefEnd(bare.slice(i + 1));
  if (!a || !b || a.kind !== b.kind) return null;
  if (a.kind === 'col')
    return { c1: Math.min(a.col, b.col), r1: 0, c2: Math.max(a.col, b.col), r2: WHOLE_AXIS };
  if (a.kind === 'row')
    return { c1: 0, r1: Math.min(a.row, b.row), c2: WHOLE_AXIS, r2: Math.max(a.row, b.row) };
  return {
    c1: Math.min(a.col, b.col),
    r1: Math.min(a.row, b.row),
    c2: Math.max(a.col, b.col),
    r2: Math.max(a.row, b.row),
  };
};

/** Nombre de hoja «limpio» de un token word/qsheet (quita comillas de qsheet). */
function sheetNameOf(tok: FxTok): string {
  if (tok.t === 'qsheet') return tok.v.replace(/^'/, '').replace(/'$/, '').replace(/''/g, "'");
  return tok.v;
}

/* ---------- (a) Token/función/argumento bajo el cursor ---------- */

/** Identificador (posible nombre de función) que el cursor está tecleando. */
export interface FuncToken {
  /** Prefijo tecleado hasta el cursor (tal cual, respetando la caja). */
  prefix: string;
  /** Inicio del identificador (para reemplazarlo entero al aceptar). */
  start: number;
  /** Fin del identificador. */
  end: number;
}

/**
 * Identificador bajo el cursor apto para autocompletar por nombre. Null si el
 * cursor no está sobre una palabra, si el prefijo está vacío o si la palabra es
 * la referencia tras un «!» (nombre cualificado por hoja). No filtra contra el
 * registro: eso lo hace quien consume (con `buildRegistry().names()` en vivo).
 */
export function funcTokenAt(text: string, caret: number): FuncToken | null {
  if (!text.startsWith('=')) return null;
  const toks = tokenizeFx(text);
  for (let k = 0; k < toks.length; k++) {
    const t = toks[k]!;
    if (t.t !== 'word') continue;
    if (caret < t.start || caret > t.end) continue; // dentro o al final del identificador
    if (toks[k - 1]?.t === 'bang') return null; // referencia cualificada por hoja
    const prefix = text.slice(t.start, caret);
    if (prefix === '') return null;
    return { prefix, start: t.start, end: t.end };
  }
  return null;
}

/** Función que envuelve al cursor y el argumento actual. */
export interface ArgHint {
  /** Nombre de la función (EN MAYÚSCULAS). */
  name: string;
  /** Índice 0-based del argumento donde está el cursor (por separadores de nivel). */
  argIndex: number;
  /** Offset del «(» de la llamada. */
  open: number;
}

/**
 * Llamada a función que ENVUELVE al cursor, contando los separadores de su
 * nivel para saber el argumento actual. Los paréntesis de agrupación anidados
 * cuentan profundidad pero no dan hint; se devuelve la función nombrada más
 * interna que rodea al cursor, o null si ninguna.
 */
export function argHintAt(text: string, caret: number): ArgHint | null {
  if (!text.startsWith('=')) return null;
  const toks = tokenizeFx(text);
  const stack: { name: string; argIndex: number; open: number }[] = [];
  for (let k = 0; k < toks.length; k++) {
    const t = toks[k]!;
    if (t.start >= caret) break; // solo lo anterior al cursor
    if (t.t === 'lparen') {
      const prev = toks[k - 1];
      const name = prev?.t === 'word' ? prev.v.toUpperCase() : '';
      stack.push({ name, argIndex: 0, open: t.start });
    } else if (t.t === 'rparen') {
      stack.pop();
    } else if (t.t === 'sep') {
      const top = stack[stack.length - 1];
      if (top) top.argIndex++;
    }
  }
  for (let s = stack.length - 1; s >= 0; s--) {
    const f = stack[s]!;
    if (f.name !== '') return { name: f.name, argIndex: f.argIndex, open: f.open };
  }
  return null;
}

/* ---------- (b) ¿El cursor espera una referencia? ---------- */

/**
 * ¿La posición del cursor espera una referencia? Cierto cuando el token
 * significativo previo es el «=» inicial, un operador, un separador, un «(» o un
 * «!» (posición de operando). Falso si no es una fórmula o si el cursor va justo
 * tras un operando (número, texto, «)», «%» o una referencia ya escrita).
 */
export function expectsRef(text: string, caret: number): boolean {
  if (!text.startsWith('=')) return false;
  const toks = tokenizeFx(text);
  let prev: FxTok | null = null;
  for (const t of toks) {
    if (t.end <= caret) prev = t;
    else break;
  }
  if (!prev) return false;
  return prev.t === 'op' || prev.t === 'sep' || prev.t === 'lparen' || prev.t === 'bang';
}

/* ---------- (c) Ciclado absoluto/relativo (F4) ---------- */

/** Banderas ($col, $fila) de cada estado del ciclo A1→$A$1→A$1→$A1→A1. */
const STATE_FLAGS: readonly [boolean, boolean][] = [
  [false, false], // 0: A1
  [true, true], // 1: $A$1
  [false, true], // 2: A$1
  [true, false], // 3: $A1
];

function absState(colAbs: boolean, rowAbs: boolean): number {
  if (!colAbs && !rowAbs) return 0;
  if (colAbs && rowAbs) return 1;
  if (!colAbs && rowAbs) return 2;
  return 3;
}

/**
 * Cicla el patrón de anclaje de una referencia: A1 → $A$1 → A$1 → $A1 → A1.
 * Conserva el prefijo de hoja y aplica el MISMO estado a ambos extremos de un
 * rango (A1:B2 → $A$1:$B$2 → …). Devuelve la ref intacta si no parsea como A1.
 */
export function cycleRefAbs(ref: string): string {
  const bang = ref.lastIndexOf('!');
  const prefix = bang >= 0 ? ref.slice(0, bang + 1) : '';
  const body = bang >= 0 ? ref.slice(bang + 1) : ref;
  const parts = body.split(':');
  let first: CellRef;
  try {
    first = parseCellWord(parts[0]!, undefined);
  } catch {
    return ref;
  }
  const [colAbs, rowAbs] = STATE_FLAGS[(absState(first.colAbs, first.rowAbs) + 1) % 4]!;
  const emit = (p: string): string => {
    let r: CellRef;
    try {
      r = parseCellWord(p, undefined);
    } catch {
      return p;
    }
    return `${colAbs ? '$' : ''}${numToCol(r.col)}${rowAbs ? '$' : ''}${r.row + 1}`;
  };
  return prefix + parts.map(emit).join(':');
}

/**
 * Cicla la referencia que toca el cursor dentro del texto (F4). Devuelve el
 * texto nuevo y el cursor recolocado al final de la ref (su longitud cambia);
 * null si el cursor no está sobre ninguna referencia.
 */
export function cycleRefAt(text: string, caret: number): { text: string; caret: number } | null {
  const hit = extractRefs(text).find((r) => caret >= r.start && caret <= r.end);
  if (!hit) return null;
  const cycled = cycleRefAbs(hit.text);
  return {
    text: text.slice(0, hit.start) + cycled + text.slice(hit.end),
    caret: hit.start + cycled.length,
  };
}

/**
 * Referencia cuyo span TOCA el cursor (inclusive en ambos extremos): la que un
 * clic de point-and-click debe SUSTITUIR en vez de añadir otra. Devuelve null si
 * el cursor no toca ninguna. Es lo que hace que, tras insertar una celda con un
 * clic (el cursor queda al final de la ref) o al recolocar el cursor sobre ella,
 * el siguiente clic la REEMPLACE (paridad Excel/Sheets), no la duplique.
 * Si dos refs comparten el offset (fin de una = inicio de otra), gana la 1.ª.
 */
export function refAtCaret(text: string, caret: number): FormulaRef | null {
  return extractRefs(text).find((r) => caret >= r.start && caret <= r.end) ?? null;
}

/* ---------- (d) Extracción de referencias + color por ref ---------- */

/** Referencia hallada en una fórmula, con su posición, rectángulo y color. */
export interface FormulaRef {
  /** Texto tal cual aparece (con prefijo de hoja si lo tiene). */
  text: string;
  start: number;
  end: number;
  /** Nombre de hoja si la referencia lo cualifica; undefined si es local. */
  sheet?: string;
  /** Rectángulo 0-based de la parte A1 (sin hoja), normalizado. */
  rect: RangeRect;
  /** Índice de color, estable por referencia DISTINTA (misma zona → mismo color). */
  colorIndex: number;
}

/** Resuelve una referencia estructurada `Tabla[Columna]` a hoja+rectángulo para
 *  resaltarla en la rejilla; null si es irresoluble (tabla/columna desconocida).
 *  Lo provee SheetCanvas (dueño del mapa tabla→rango). */
export type StructuredResolver = (spec: {
  table: string;
  column: string | undefined;
  area: TableArea;
  thisRow: boolean;
}) => { sheet: string; rect: RangeRect } | null;

/** Ref recolectada antes de asignarle color (A1 o estructurada). */
type RawRef = {
  text: string;
  start: number;
  end: number;
  sheet?: string;
  rect: RangeRect;
  key: string;
};

/** Caracteres válidos en un nombre de tabla (mismo criterio que el lexer de palabras). */
const TABLE_NAME_CH = /[A-Za-z0-9_$.]/;

/**
 * Extrae las referencias y rangos de una fórmula («=…») con sus posiciones y un
 * color por referencia DISTINTA (misma celda/rango y hoja → mismo color,
 * ignorando el anclaje «$»). Ignora lo que va dentro de cadenas y los nombres de
 * función (palabra seguida de «(»). Si se pasa `resolveStructured`, también
 * reconoce las referencias estructuradas `Tabla[Columna]` y las resalta como el
 * rango de esa columna (sin resolutor se ignoran: retrocompat). Orden por
 * aparición; el color se asigna en ese orden.
 */
export function extractRefs(text: string, resolveStructured?: StructuredResolver): FormulaRef[] {
  if (!text.startsWith('=')) return [];
  const toks = tokenizeFx(text);
  const raw: RawRef[] = [];

  // Tramos de corchete de tabla `[...]` (UN barrido lineal, ANTES de la ruta (1)):
  // los reutilizan (2) las refs estructuradas y —SIEMPRE, aunque no haya resolutor—
  // la ruta (1) para NO confundir un nombre de columna con forma de celda (p. ej.
  // `Ventas[T1]`) con una ref A1 fantasma sobre la celda real T1 (rompería el
  // resaltado y el point-and-click, que llaman a extractRefs sin resolutor).
  const brackets = tableBracketSpans(text, toks);
  const insideBrackets = (pos: number): boolean =>
    brackets.some((s) => pos > s.open && pos < s.close);

  // (1) Referencias A1, rangos y ejes enteros (A:A, 1:1) por recorrido de tokens.
  for (let k = 0; k < toks.length; k++) {
    const t = toks[k]!;
    const kind = endpointKind(t);
    if (kind === null) continue; // no es un extremo de referencia
    if (insideBrackets(t.start)) continue; // nombre de columna de `Tabla[…]`, no una ref A1
    if (toks[k + 1]?.t === 'lparen') continue; // nombre de función (p. ej. LOG10()
    if (toks[k + 1]?.t === 'bang') continue; // es un nombre de hoja, no una ref

    // Prefijo de hoja: (word|qsheet) «!» justo antes.
    let start = t.start;
    let sheet: string | undefined;
    const prev = toks[k - 1];
    const prev2 = toks[k - 2];
    if (prev?.t === 'bang' && (prev2?.t === 'word' || prev2?.t === 'qsheet')) {
      start = prev2.start;
      sheet = sheetNameOf(prev2);
    }

    // Extensión de rango: «:» + segundo extremo del MISMO tipo (celda/col/fila);
    // el extremo derecho no puede ser un nombre de función (seguido de «(»).
    let end = t.end;
    const nx = toks[k + 1];
    const nx2 = toks[k + 2];
    if (nx?.t === 'colon' && endpointKind(nx2) === kind && toks[k + 3]?.t !== 'lparen') {
      end = nx2!.end;
      k += 2;
    } else if (kind !== 'cell') {
      continue; // una columna/fila SUELTA («A», «3») no es una referencia
    }

    const bare = text.slice(t.start, end); // parte A1 sin prefijo de hoja
    const rect = rangeRectOf(bare);
    if (!rect) continue;
    const key = `${sheet ?? ''}!${rect.c1},${rect.r1},${rect.c2},${rect.r2}`;
    raw.push({
      text: text.slice(start, end),
      start,
      end,
      ...(sheet !== undefined ? { sheet } : {}),
      rect,
      key,
    });
  }

  // (2) Referencias estructuradas Tabla[Columna] (sobre los tramos ya emparejados).
  if (resolveStructured) collectStructuredRefs(text, brackets, resolveStructured, raw);

  // (3) Color por clave DISTINTA, en orden de aparición.
  raw.sort((a, b) => a.start - b.start);
  const colorByKey = new Map<string, number>();
  return raw.map((r) => {
    let colorIndex = colorByKey.get(r.key);
    if (colorIndex === undefined) {
      colorIndex = colorByKey.size;
      colorByKey.set(r.key, colorIndex);
    }
    return {
      text: r.text,
      start: r.start,
      end: r.end,
      ...(r.sheet !== undefined ? { sheet: r.sheet } : {}),
      rect: r.rect,
      colorIndex,
    };
  });
}

/** Tramo de un corchete de tabla de nivel superior: `open` es el índice del «[» y
 *  `close` el del «]» que lo cierra; `close === text.length` si quedó sin cerrar
 *  (fórmula a medio teclear). */
interface BracketSpan {
  open: number;
  close: number;
}

/**
 * Empareja los corchetes de tabla `[...]` de NIVEL SUPERIOR con UN barrido lineal
 * (el lexer de fórmulas ignora «[]@#», por eso se escanea el texto crudo). Cuenta
 * profundidad para tragar la forma anidada `Tabla[[#Datos],[Col]]` en un solo
 * tramo. Ignora los corchetes dentro de cadenas. Un «[» sin cerrar produce un
 * tramo hasta el final (así el resaltado/point-and-click ya protegen la columna
 * a medio teclear). Lineal O(n): las cadenas (en orden y sin solapar) se saltan
 * con un puntero monótono, no rebuscando en cada carácter.
 */
function tableBracketSpans(text: string, toks: readonly FxTok[]): BracketSpan[] {
  const strs = toks.filter((t) => t.t === 'str'); // tokens: ordenados y sin solapar
  const spans: BracketSpan[] = [];
  let depth = 0;
  let open = -1;
  let si = 0;
  for (let i = 0; i < text.length; i++) {
    while (si < strs.length && strs[si]!.end <= i) si++;
    if (si < strs.length && i >= strs[si]!.start) continue; // dentro de una cadena
    const c = text[i]!;
    if (c === '[') {
      if (depth === 0) open = i;
      depth++;
    } else if (c === ']' && depth > 0) {
      depth--;
      if (depth === 0) spans.push({ open, close: i });
    }
  }
  if (depth > 0) spans.push({ open, close: text.length }); // corchete sin cerrar
  return spans;
}

/** Añade a `raw` las referencias estructuradas `Tabla[…]` de los tramos ya
 *  emparejados, resueltas a hoja+rectángulo. Los tramos sin cerrar se omiten (no
 *  hay especificador completo); las irresolubles también (no se resaltan). */
function collectStructuredRefs(
  text: string,
  brackets: readonly BracketSpan[],
  resolveStructured: StructuredResolver,
  raw: RawRef[],
): void {
  for (const { open, close } of brackets) {
    if (text[close] !== ']') continue; // sin cerrar: no resoluble
    // Nombre de tabla justo antes del «[» (vacío = implícita).
    let ns = open;
    while (ns > 0 && TABLE_NAME_CH.test(text[ns - 1]!)) ns--;
    const spec = parseBracketSpec(text.slice(open + 1, close));
    const hit = resolveStructured({
      table: text.slice(ns, open),
      column: spec.column,
      area: spec.area,
      thisRow: spec.thisRow,
    });
    if (hit) {
      const key = `${hit.sheet}!${hit.rect.c1},${hit.rect.r1},${hit.rect.c2},${hit.rect.r2}`;
      raw.push({
        text: text.slice(ns, close + 1),
        start: ns,
        end: close + 1,
        sheet: hit.sheet,
        rect: hit.rect,
        key,
      });
    }
  }
}

/* ---------- (e) Corchete de tabla bajo el cursor (autocompletado de columnas) ---------- */

/** Columna que se teclea dentro de `Tabla[…` bajo el cursor. */
export interface TableBracket {
  /** Nombre de la tabla antes del «[»; '' si es implícita («[@Col]»/«[Col]»). */
  table: string;
  /** Prefijo de columna tecleado tras «[» (y el «@» opcional). */
  prefix: string;
  /** Inicio del prefijo (para sustituirlo al aceptar). */
  start: number;
  /** Fin del prefijo (= cursor). */
  end: number;
  /** Había un «@» (fila actual) tras el «[». */
  thisRow: boolean;
}

/**
 * Detecta si el cursor teclea el nombre de columna dentro de un corchete de tabla
 * `Tabla[prefijo`. Escanea el texto crudo (el lexer de fórmulas ignora «[]@#», por
 * eso no se reutiliza). Solo la forma SIMPLE `Tabla[col` y la implícita
 * `[@col]`/`[col]`; las anidadas `[[#Datos],[col]]` o de área `[#…]` devuelven null
 * (sin autocompletado). Null si el cursor no está en ese contexto.
 */
export function tableBracketAt(text: string, caret: number): TableBracket | null {
  if (!text.startsWith('=')) return null;
  // «[» abierto que envuelve al cursor (sin «]» entre él y el cursor).
  let lb = -1;
  for (let i = caret - 1; i >= 0; i--) {
    const ch = text[i]!;
    if (ch === ']') return null; // corchete anterior ya cerrado
    if (ch === '[') {
      lb = i;
      break;
    }
  }
  if (lb < 0) return null;
  // Nombre de tabla justo antes del «[».
  let ni = lb - 1;
  while (ni >= 0 && TABLE_NAME_CH.test(text[ni]!)) ni--;
  const pre = ni >= 0 ? text[ni]! : '';
  if (pre === '[' || pre === ',') return null; // forma anidada [[#..],[..]]
  // Contenido tras «[» (y «@» opcional) hasta el cursor.
  let j = lb + 1;
  let thisRow = false;
  if (text[j] === '@') {
    thisRow = true;
    j++;
  }
  const content = text.slice(j, caret);
  if (/[[\],#]/.test(content)) return null; // área/anidado → sin autocompletado simple
  return { table: text.slice(ni + 1, lb), prefix: content, start: j, end: caret, thisRow };
}
