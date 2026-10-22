/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las organizaciones de confianza para las peticiones de firma de fuera (ADR 0059),
 * tal como se escriben en la consola.
 */

/** Las organizaciones, una por línea o separadas por comas o espacios, sin repetir. */
export function organizacionesDe(texto: string): string[] {
  const out: string[] = [];
  for (const x of texto.split(/[\s,;]+/)) {
    const o = x.trim().toLowerCase();
    if (o && !out.includes(o)) out.push(o);
  }
  return out;
}
