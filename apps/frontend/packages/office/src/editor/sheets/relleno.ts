/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * RELLENO CON IA de una columna: a partir de los ejemplos que ya tiene (las filas con
 * valor) y, si se quiere, de una indicación, Sytue AI propone el valor de las filas que
 * faltan. Aquí vive lo que no es red ni pantalla: qué se le manda y cómo se lee lo que
 * contesta. La persona ve la propuesta y decide si se escribe.
 *
 * Es el mismo código que `apps/nucleo/formula/analisis/relleno.go` en la app nativa,
 * con los vectores de `analisis/testdata/relleno.json`: la misma hoja manda la misma
 * petición y la misma respuesta se lee igual en las dos.
 *
 * Lo que sale: el texto de las columnas del rango, a Sytue AI (Scaleway, en la UE), sin
 * anonimizar; por eso el panel lo dice antes de enviar.
 */

/** Tope del contexto, como el del asistente (`MAX_CONTEXT_CHARS`). */
export const FILL_MAX_CHARS = 12000;

export interface FillInput {
  /** Nombres de las columnas del rango (la fila de encabezados). */
  headers: readonly string[];
  /** Texto de cada fila del cuerpo, columna a columna. */
  rows: readonly (readonly string[])[];
  /** Número de fila de la hoja (1-based) de la primera fila del cuerpo. */
  firstRow: number;
  /** Columna a rellenar, índice dentro del rango. */
  target: number;
  /** Indicación de la persona; vacía = solo los ejemplos. */
  hint?: string;
}

export interface FillPrompt {
  instruction: string;
  context: string;
  /** Filas de la hoja (1-based) que se piden. */
  pending: number[];
}

/** Una celda del contexto: sin tabuladores ni saltos, que parten la tabla. */
const clean = (s: string): string => s.replace(/[\t\r\n]+/g, ' ').trim();

/**
 * La petición: la instrucción (qué columna y cómo contestar) y el contexto (la tabla en
 * texto separado por tabuladores, con «?» donde falta el valor). Sin ejemplos ni
 * indicación no hay nada que pedir; tampoco sin filas vacías. Si la tabla no cabe, se
 * mandan los ejemplos y las filas pendientes que quepan: las demás quedan sin pedir.
 */
export function buildFillPrompt(input: FillInput): FillPrompt | null {
  const { headers, rows, target, firstRow } = input;
  const hint = (input.hint ?? '').trim();
  if (target < 0 || target >= headers.length) return null;
  const full = rows.map((r) => r.some((v) => v.trim() !== ''));
  const examples = rows.filter((r, i) => full[i] && (r[target] ?? '').trim() !== '');
  const pendingIdx = rows
    .map((_, i) => i)
    .filter((i) => full[i] && (rows[i]![target] ?? '').trim() === '');
  if (pendingIdx.length === 0 || (examples.length === 0 && hint === '')) return null;

  const column = clean(headers[target] ?? '') || `#${target + 1}`;
  const instruction =
    `Rellena la columna «${column}» de la tabla del contexto. ` +
    'Las filas que ya tienen valor en esa columna son ejemplos: sigue su patrón y su formato. ' +
    (hint !== '' ? `Criterio de la persona: «${clean(hint)}». ` : '') +
    'Devuelve SOLO una línea por cada fila marcada con «?», con el número de la fila, un tabulador ' +
    'y el valor. Sin encabezados, sin comillas y sin explicaciones.';

  const line = (n: string, cells: readonly string[]): string => [n, ...cells.map(clean)].join('\t');
  const head = line('fila', headers);
  const out: string[] = [head];
  let size = head.length;
  const pending: number[] = [];
  rows.forEach((r, i) => {
    if (!full[i]) return;
    const isPending = (r[target] ?? '').trim() === '';
    const cells = headers.map((_, c) => (c === target && isPending ? '?' : (r[c] ?? '')));
    const l = line(String(firstRow + i), cells);
    // `length` cuenta unidades UTF-16: Go cuenta igual para cortar en el mismo sitio.
    if (size + 1 + l.length > FILL_MAX_CHARS) return;
    size += 1 + l.length;
    out.push(l);
    if (isPending) pending.push(firstRow + i);
  });
  if (pending.length === 0) return null;
  return { instruction, context: out.join('\n'), pending };
}

/** Las formas de una línea de respuesta, por orden: con tabulador, con «|», «:», «.» o
 *  «)», y con « - ». El tabulador va primero para que un valor negativo no pierda el
 *  signo. Clases de espacio explícitas: `\s` no es el mismo en JavaScript que en Go. */
const LINE = [
  /^[ \t]*(?:fila[ \t]*)?(\d+)\t(.*)$/i,
  /^[ \t]*(?:fila[ \t]*)?(\d+)[ \t]*[|:.)][ \t]*(.*)$/i,
  /^[ \t]*(?:fila[ \t]*)?(\d+)[ \t]+-[ \t]+(.*)$/i,
];

/** Comillas que un modelo pone alrededor de un valor y que no son parte de él. */
const QUOTES = /^[`'"“”‘’«»]+|[`'"“”‘’«»]+$/g;

/**
 * Lee la respuesta: «número<tab>valor» por línea (también con «|», «:» o «-» si el
 * modelo no respeta el tabulador), dentro o fuera de un bloque de código. Solo cuentan
 * las filas pedidas, y la primera vez que salen con un valor; nunca una fórmula.
 * Devuelve fila → valor.
 */
export function parseFillResponse(text: string, pending: readonly number[]): Map<number, string> {
  const want = new Set(pending);
  const out = new Map<number, string>();
  const body = text.replace(/```[a-zA-Z0-9_-]*[ \t\r\n]*|```/g, '\n');
  for (const raw of body.split(/\r?\n/)) {
    const m = LINE.map((re) => re.exec(raw)).find((x) => x !== null);
    if (!m) continue;
    const n = Number(m[1]);
    const value = (m[2] ?? '')
      .replace(/^[ \t]+|[ \t]+$/g, '')
      .replace(QUOTES, '')
      .replace(/^[ \t]+|[ \t]+$/g, '');
    // Una fórmula no se escribe: lo que se rellena son valores, y una fórmula del
    // modelo podría leer otras celdas de la hoja.
    if (!want.has(n) || out.has(n) || value === '' || value === '?' || value.startsWith('=')) {
      continue;
    }
    out.set(n, value);
  }
  return out;
}
