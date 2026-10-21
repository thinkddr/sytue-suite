/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Combinación de correspondencia (mail merge) — AYUDAS PURAS (sin DOM). El
 * pegamento con el folio (leer/escribir el origen embebido, insertar marcadores,
 * vista previa y combinar) vive en ribbon/tabs/Correspondencia.tsx; aquí solo la
 * lógica no trivial y testeable: parsear el origen CSV/TSV pegado y sustituir los
 * marcadores de campo de un HTML por los valores de un registro (con escape, que
 * es la frontera de confianza: los datos del origen son de usuario → anti-XSS).
 * Ejecutar el self-test: cd apps/frontend && bun test packages/office/test/mailmerge.test.ts
 */

/** Origen de datos de la combinación: cabecera + un registro por fila. */
export interface MergeSource {
  /** Nombres de campo (1.ª fila del origen), en orden. */
  fields: string[];
  /** Un registro por fila siguiente: campo → valor (todas las claves de `fields`). */
  records: Record<string, string>[];
}

const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
};

/** Escapa texto para inyectarlo como HTML (mismo criterio que exec.ts/ribbon). */
export function escapeHtml(text: string): string {
  return text.replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch] ?? ch);
}

const HTML_UNESCAPES: Record<string, string> = {
  '&amp;': '&',
  '&lt;': '<',
  '&gt;': '>',
  '&quot;': '"',
  '&#39;': "'",
  '&apos;': "'",
};

/** Revierte el escape de un valor de atributo (el nombre de campo del marcador). */
function unescapeHtml(text: string): string {
  return text.replace(/&(?:amp|lt|gt|quot|apos|#39);/g, (e) => HTML_UNESCAPES[e] ?? e);
}

/* ---------- Parseo del origen (CSV/TSV → {fields, records}) ---------- */

/**
 * Delimitador dominante de la cabecera: TAB si aparece (TSV pegado desde una
 * hoja), si no el más frecuente entre coma y punto y coma (CSV europeo). Ignora
 * los separadores que caen dentro de comillas.
 */
function detectDelimiter(headerLine: string): string {
  let tab = 0;
  let comma = 0;
  let semi = 0;
  let quoted = false;
  for (const ch of headerLine) {
    if (ch === '"') quoted = !quoted;
    else if (!quoted) {
      if (ch === '\t') tab += 1;
      else if (ch === ',') comma += 1;
      else if (ch === ';') semi += 1;
    }
  }
  if (tab > 0) return '\t';
  return semi > comma ? ';' : ',';
}

/**
 * Divide texto delimitado en filas de celdas (RFC 4180 básico): comillas dobles
 * para entrecomillar, `""` escapa una comilla y los saltos de línea dentro de
 * comillas no cortan la fila. Una sola pasada O(n) sobre el texto.
 */
function splitRows(text: string, delim: string): string[][] {
  const rows: string[][] = [];
  let row: string[] = [];
  let field = '';
  let quoted = false;
  const s = text.replace(/\r\n?/g, '\n'); // normaliza fin de línea
  for (let i = 0; i < s.length; i += 1) {
    const ch = s[i]!;
    if (quoted) {
      if (ch === '"') {
        if (s[i + 1] === '"') {
          field += '"';
          i += 1; // comilla escapada
        } else {
          quoted = false;
        }
      } else {
        field += ch;
      }
    } else if (ch === '"') {
      quoted = true;
    } else if (ch === delim) {
      row.push(field);
      field = '';
    } else if (ch === '\n') {
      row.push(field);
      rows.push(row);
      row = [];
      field = '';
    } else {
      field += ch;
    }
  }
  if (field !== '' || row.length > 0) {
    row.push(field);
    rows.push(row);
  }
  return rows;
}

/**
 * Parsea un origen pegado (CSV/TSV) a {fields, records}. 1.ª fila = nombres de
 * campo; filas siguientes = registros. Detecta el delimitador, respeta comillas,
 * descarta las filas vacías y rellena con '' las celdas que falten. Una cabecera
 * vacía se nombra «Campo N» para no perder la columna ni chocar con «».
 */
export function parseMergeSource(text: string): MergeSource {
  const clean = text.replace(/^\uFEFF/, ''); // quita BOM de un pegado desde Excel
  if (clean.trim() === '') return { fields: [], records: [] };
  const firstLine = clean.split('\n', 1)[0] ?? '';
  const delim = detectDelimiter(firstLine);
  const rows = splitRows(clean, delim);
  if (rows.length === 0) return { fields: [], records: [] };
  const fields = (rows[0] ?? []).map((h, i) => h.trim() || `Campo ${i + 1}`);
  const records = rows
    .slice(1)
    .filter((r) => r.some((c) => c.trim() !== '')) // descarta filas en blanco
    .map((r) => {
      const rec: Record<string, string> = {};
      fields.forEach((f, i) => {
        rec[f] = r[i] ?? '';
      });
      return rec;
    });
  return { fields, records };
}

/* ---------- Marcadores de campo ---------- */

/**
 * HTML del marcador de un campo de combinación (FUENTE ÚNICA del formato, para
 * que insertar y sustituir no diverjan). `contenteditable=false` lo hace una
 * ficha atómica; el nombre va escapado en el atributo y en el texto visible «».
 */
export function mergeFieldHtml(field: string): string {
  const safe = escapeHtml(field);
  return `<span data-mergefield="${safe}" contenteditable="false">«${safe}»</span>`;
}

/** Marcador `<span data-mergefield="X" …>…</span>`; el nombre (escapado) en el g1. */
const MERGE_FIELD_RE = /<span\b[^>]*\bdata-mergefield="([^"]*)"[^>]*>.*?<\/span>/gi;

/**
 * Sustituye cada marcador de campo del HTML por el valor del registro, ESCAPADO
 * (los datos del origen son de usuario → anti-XSS). Un campo que no está en el
 * registro se sustituye por cadena vacía. Función pura: es el núcleo de
 * «finalizar y combinar» (una pasada por registro) y la cubre el self-test.
 */
export function substituteMergeFields(html: string, record: Record<string, string>): string {
  return html.replace(MERGE_FIELD_RE, (_m, rawField: string) => {
    const value = record[unescapeHtml(rawField)];
    return value === undefined ? '' : escapeHtml(value);
  });
}
