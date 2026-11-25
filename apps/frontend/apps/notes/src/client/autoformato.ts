/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Markdown EN LA LÍNEA al escribir: `**negrita**`, `*cursiva*` o `_cursiva_`, `` `código` ``
 * y `~~tachado~~` se convierten al teclear la marca que cierra, como en Confluence o
 * Notion. Los de principio de línea (`# `, `- `, `1. `…) viven en `markdown.ts`.
 *
 * Aquí solo vive la decisión, sobre el texto que hay antes del cursor (con la marca que
 * cierra ya escrita); el DOM lo toca RichText, que además no lo hace dentro del código.
 *
 * LOS NEGATIVOS IMPORTAN MÁS: una marca que convierte donde no debe se come caracteres
 * que alguien escribió («2*3*4», «snake_case_», una URL con `__`).
 */

export type EtiquetaEnLinea = 'b' | 'i' | 'code' | 's';

export interface AtajoEnLinea {
  /** Dónde empieza la marca que abre, en el texto dado. */
  desde: number;
  etiqueta: EtiquetaEnLinea;
  /** Lo que queda entre las marcas. */
  texto: string;
}

/** En el orden en que se prueban: las dobles antes que las sencillas. */
const MARCAS: readonly (readonly [string, EtiquetaEnLinea])[] = [
  ['**', 'b'],
  ['~~', 's'],
  ['`', 'code'],
  ['*', 'i'],
  ['_', 'i'],
];

const PALABRA = /[\p{L}\p{N}]/u;
const BLANCO = /\s/;

/**
 * Qué convierte la marca recién tecleada al final de `antes`; `null` si nada. La marca
 * que abre va al principio o tras algo que no es letra ni número, lo de dentro no empieza
 * ni acaba en blanco, y una marca sencilla no forma parte de una doble.
 */
export function atajoEnLinea(antes: string): AtajoEnLinea | null {
  const texto = antes.replace(/ /g, ' ');
  for (const [marca, etiqueta] of MARCAS) {
    if (!texto.endsWith(marca)) continue;
    const fin = texto.length - marca.length;
    const sencilla = marca.length === 1;
    // `**` a medio cerrar: el `*` de ahora es el primero de los dos.
    if (sencilla && texto[fin - 1] === marca) continue;
    const desde = texto.lastIndexOf(marca, fin - 1);
    if (desde < 0) continue;
    const dentro = texto.slice(desde + marca.length, fin);
    if (!dentro || BLANCO.test(dentro[0]!) || BLANCO.test(dentro.at(-1)!)) continue;
    if (sencilla && dentro.includes(marca)) continue;
    const previo = texto[desde - 1];
    if (previo !== undefined && (PALABRA.test(previo) || previo === marca[0])) continue;
    if (sencilla && texto[desde + 1] === marca) continue;
    return { desde, etiqueta, texto: dentro };
  }
  return null;
}
