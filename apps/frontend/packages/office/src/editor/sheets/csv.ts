/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Descargar la hoja como CSV» del menú Archivo. Calcus importaba CSV pero no lo sabía
 * escribir, y el CSV es lo que piden un banco, una gestoría o el importador de cualquier
 * otra herramienta: hasta ahora había que pasar por Excel para sacarlo.
 *
 * Se escribe la hoja ACTIVA desde A1 hasta la última fila y columna con algo, con lo que
 * cada celda ENSEÑA (el valor calculado con su formato, lo mismo que copiar), como hacen
 * Excel y Sheets. El formato es el del RFC 4180: coma, comillas dobles solo donde hacen
 * falta (con la comilla doblada) y fin de línea CRLF. Delante va la marca de orden de
 * bytes de UTF-8, sin la cual Excel abre «Año» como «AÃ±o».
 *
 * Se genera en el dispositivo, sin pedir nada al servicio: funciona sin conexión y con
 * el Drive en la bóveda. La app nativa lo escribe igual (`apps/fyne/calcus/csv.go`), y
 * los dos recorren los mismos vectores (`test/vectores/calcus-herramientas.json`).
 */

/** Marca de orden de bytes de UTF-8, para que Excel lea los acentos. */
export const BOM_UTF8 = '\uFEFF';

/** Un campo del CSV: entre comillas si lleva coma, comillas o salto de línea. */
function campo(v: string): string {
  return /[",\r\n]/.test(v) ? `"${v.replace(/"/g, '""')}"` : v;
}

/**
 * Las filas, ya con el texto de cada celda, como CSV (sin la marca de orden de bytes).
 * Las filas se igualan al ancho de la más larga, que es lo que esperan los importadores
 * que cuentan columnas.
 */
export function csvDeFilas(filas: readonly (readonly string[])[]): string {
  const ancho = filas.reduce((m, f) => Math.max(m, f.length), 0);
  return filas
    .map((f) => Array.from({ length: ancho }, (_, i) => campo(f[i] ?? '')).join(','))
    .map((l) => `${l}\r\n`)
    .join('');
}

/** El nombre de la descarga: «Título - Hoja.csv», como Sheets. */
export function nombreCsv(base: string, hoja: string): string {
  const limpio = hoja.replace(/[\\/:*?"<>|]+/g, '_').trim();
  return `${base}${limpio === '' ? '' : ` - ${limpio}`}.csv`;
}
