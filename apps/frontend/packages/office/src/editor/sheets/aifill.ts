/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Dónde escribe el asistente de IA su fórmula, y cómo se adapta al bajar.
 *
 * El asistente devuelve UNA fórmula, escrita para la primera celda del destino.
 * Pegarla tal cual en veinte celdas deja veinte veces `=SUMA(A1:B1)`, que es
 * justo lo que se reportó: «debería insertar la función adaptada en toda la
 * columna, no solo generar funciones de una celda».
 *
 * Sin DOM ni motor de fórmulas: el desplazamiento de referencias y la lectura de
 * celdas se inyectan, así que esto se prueba sin abrir una hoja.
 */

/** Rectángulo de celdas, en índices 0-based. */
export interface Rect {
  c1: number;
  c2: number;
  r1: number;
  r2: number;
}

/**
 * Columnas (0-based) que MENCIONA una fórmula: de `=SUMA(A1:B1)` saca A y B.
 *
 * Deliberadamente TOSCA: una expresión regular de referencias A1, sin entender
 * cadenas ni nombres definidos. Un falso positivo aquí solo alarga o acorta el
 * relleno propuesto; nunca escribe una fórmula incorrecta, porque eso lo decide
 * el desplazador de referencias, que sí parsea de verdad.
 */
export function columnasCitadas(formula: string): number[] {
  const cols = new Set<number>();
  for (const m of formula.matchAll(/\$?([A-Z]{1,2})\$?\d{1,3}/g)) {
    const letras = m[1];
    if (!letras) continue;
    let n = 0;
    for (const ch of letras) n = n * 26 + (ch.charCodeAt(0) - 64);
    cols.add(n - 1);
  }
  return [...cols];
}

export interface Relleno {
  formula: string;
  /** Selección actual. */
  rect: Rect;
  /** Filas de la rejilla (tope de la extensión automática). */
  filas: number;
  /** ¿Tiene contenido esa celda? Para saber hasta dónde llegan los datos. */
  tieneDato: (col: number, fila: number) => boolean;
  /** Desplaza las referencias relativas de la fórmula. */
  desplazar: (formula: string, dc: number, dr: number) => string;
}

/**
 * Última fila hasta la que extender cuando solo hay UNA celda seleccionada.
 *
 * Se mira cuántas filas seguidas tienen datos en las columnas que la fórmula
 * menciona, que es lo que hace un doble clic en el tirador de relleno. No es
 * adivinar: es la convención de cualquier hoja de cálculo, y sin ella pedir
 * «suma A y B en toda la columna C» seguiría escribiendo una sola celda.
 */
export function hastaDondeLleganLosDatos(
  formula: string,
  desde: number,
  filas: number,
  tieneDato: (col: number, fila: number) => boolean,
): number {
  let hasta = desde;
  for (const col of columnasCitadas(formula)) {
    for (let r = desde; r < filas; r++) {
      if (!tieneDato(col, r)) break;
      hasta = Math.max(hasta, r);
    }
  }
  return hasta;
}

/**
 * Limpia el resultado devuelto por la IA para convertirlo en una fórmula válida de hoja de cálculo.
 *
 * Elimina:
 * - Bloques de código Markdown (```excel ... ``` o ``` ... ```)
 * - Tildes graves/agudas, comillas simples/dobles y tipográficas (`, ´, ", ', «, », “, ”, ‘, ’, „, ‟)
 * - Texto introductorio previo a la fórmula (ej. "La fórmula es: ")
 * - Asegura que empiece por '=' si el modelo devolvió solo el nombre de la función (ej. SUM(A1:A2))
 */
export function cleanFormula(raw: string): string {
  let s = (raw ?? '').trim();
  if (!s) return '';

  // 1. Extraer de bloques de código markdown: ```excel ... ``` o ``` ... ```
  const fenceMatch = s.match(/```(?:[a-zA-Z0-9_-]+)?\s*([\s\S]*?)\s*```/);
  if (fenceMatch && fenceMatch[1]) {
    s = fenceMatch[1].trim();
  }

  // 2. Si hay varias líneas, localizar la línea que contiene la fórmula
  const lines = s
    .split(/\r?\n/)
    .map((l) => l.trim())
    .filter(Boolean);
  if (lines.length > 1) {
    const eqLine = lines.find((l) => l.includes('='));
    if (eqLine) {
      s = eqLine;
    } else {
      s = lines[0] ?? '';
    }
  }

  // 3. Extraer a partir del primer '=' si viene precedido de texto explicativo
  const eqIdx = s.indexOf('=');
  if (eqIdx !== -1) {
    s = s.slice(eqIdx);
  }

  // 4. Limpiar caracteres envolventes en los extremos: comillas, tildes y tipográficas
  const wrapChars = /^[`'´"“”‘’«»„‟\s]+|[`'´"“”‘’«»„‟\s]+$/g;
  s = s.replace(wrapChars, '');

  // 5. Si tras limpiar empieza por '=', limpiar comillas internas tras el '=' (ej: =`SUM(A1)`)
  if (s.startsWith('=')) {
    const rest = s.slice(1).replace(/^[`'´"“”‘’«»„‟\s]+|[`'´"“”‘’«»„‟\s]+$/g, '');
    s = '=' + rest;
  } else {
    // Si no empieza por '=' pero tiene forma de llamada a función (ej: SUM(A1:A2))
    if (/^[A-Za-z_][A-Za-z0-9_.]*\(/.test(s)) {
      s = '=' + s;
    }
  }

  return s.trim();
}

/**
 * Detecta si la fórmula contiene rangos de celdas que abarcan múltiples filas (ej. A1:A10, A1:B2, A:A).
 * Un rango en la misma fila (ej. A1:B1) NO es multifila (es una operación fila a fila extensible).
 */
export function tieneRangoMultiFila(formula: string): boolean {
  for (const m of formula.matchAll(
    /\$?([A-Z]{1,2})\$?(\d{1,5})\s*:\s*\$?([A-Z]{1,2})\$?(\d{1,5})/gi,
  )) {
    const fila1 = m[2] ? parseInt(m[2], 10) : 0;
    const fila2 = m[4] ? parseInt(m[4], 10) : 0;
    if (fila1 !== fila2) return true;
  }
  // Columnas completas A:A o A:B
  if (/\b\$?[A-Z]{1,2}\s*:\s*\$?[A-Z]{1,2}\b/i.test(formula)) {
    return true;
  }
  return false;
}

/**
 * Funciones dinámicas matriciales / spill que se derraman solas en la hoja.
 */
export function esSpillFormula(formula: string): boolean {
  return /\b(SEQUENCE|SECUENCIA|FILTER|FILTRAR|UNIQUE|UNICOS|ÚNICOS|SORT|ORDENAR|SORTBY|ORDENARPOR|TRANSPOSE|TRANSPONER|CHOOSEROWS|CHOOSECOLS|EXPAND|TAKE|DROP|ARRAYFORMULA|MATRIZ)\s*\(/i.test(
    formula,
  );
}

/**
 * Indica si la fórmula es multi-celda: o bien agrega verticalmente múltiples celdas
 * (rango multifila como =SUM(A1:A2)), o bien es una función matricial dinámica (spill).
 */
export function esFormulaMultiCelda(formula: string): boolean {
  return tieneRangoMultiFila(formula) || esSpillFormula(formula);
}

/**
 * Celdas a escribir, como desplazamientos `{ dc, dr }` desde la esquina superior
 * izquierda de la selección, con la fórmula ya adaptada a cada una.
 *
 * Devuelve desplazamientos y no referencias A1 para no tener que conocer aquí el
 * nombre de las columnas: quien llama ya sabe convertirlos.
 */
export function celdasARellenar(r: Relleno): { dc: number; dr: number; formula: string }[] {
  const formula = cleanFormula(r.formula);
  if (formula === '') return [];

  // Las funciones matriciales/spill solo deben escribirse en la celda ancla (dc=0, dr=0)
  // para permitir que el motor de fórmulas las derrame naturalmente sin provocar #SPILL!.
  if (esSpillFormula(formula)) {
    return [{ dc: 0, dr: 0, formula }];
  }

  const multiCelda = esFormulaMultiCelda(formula);
  const unaSola = r.rect.r1 === r.rect.r2 && r.rect.c1 === r.rect.c2;

  // Si solo hay una celda destino seleccionada:
  // - Si es una fórmula multi-celda (ej. =SUM(A1:A2)), NO se extiende hacia abajo por la columna.
  // - Si es una fórmula fila a fila (ej. =SUMA(A1:B1) o =A1+B1), se extiende hasta donde lleguen los datos.
  const r2 = unaSola
    ? multiCelda
      ? r.rect.r1
      : hastaDondeLleganLosDatos(formula, r.rect.r1, r.filas, r.tieneDato)
    : r.rect.r2;

  const salida: { dc: number; dr: number; formula: string }[] = [];
  for (let c = r.rect.c1; c <= r.rect.c2; c++) {
    for (let f = r.rect.r1; f <= r2; f++) {
      const dc = c - r.rect.c1;
      const dr = f - r.rect.r1;
      const desplazada = r.desplazar(formula, dc, dr);
      // El desplazador devuelve `#…` cuando la referencia se sale de la rejilla:
      // esa celda se deja sin tocar en vez de escribirle un #¡REF!.
      if (desplazada.startsWith('#')) continue;
      salida.push({ dc, dr, formula: desplazada });
    }
  }
  return salida;
}
