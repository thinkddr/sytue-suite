/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * ¿Se puede leer este extracto? El texto de una página sale de la capa de
 * texto del PDF, y hay PDFs —los que llevan fuentes con tabla de caracteres
 * rota, típicos de formularios rellenables— cuya capa dice `,035(62 '( 0$75Ê&8/$`
 * donde la página pone «IMPRESO DE MATRÍCULA». Enseñar eso en la miniatura es
 * peor que no enseñar nada: parece que el visor ha fallado.
 *
 * Heurística deliberadamente simple: si menos de dos de cada tres caracteres
 * son letras, dígitos o espacios, se da por ilegible. Con texto real la
 * proporción ronda el 90 %; con la tabla rota, el 40 %.
 */
export function extractoLegible(texto: string | undefined | null): string | null {
  const t = (texto ?? '').trim();
  if (t.length < 4) return null;
  let buenos = 0;
  let total = 0;
  for (const c of t) {
    total++;
    if (/[\p{L}\p{N}\s]/u.test(c)) buenos++;
  }
  return buenos / total >= 2 / 3 ? t : null;
}

/**
 * ¿Es un escaneo sin capa de texto? Se sabe sin pedir nada más: el contenido trae
 * el extracto de las páginas que al motor le dio tiempo a leer —la primera
 * siempre— y en un escaneo están todos vacíos. Las que no leyó (`undefined`) no
 * cuentan: sin leer no es sin texto.
 */
export function sinCapaDeTexto(paginas: readonly { extracto?: string | undefined }[]): boolean {
  const leidas = paginas.filter((p) => p.extracto !== undefined);
  return leidas.length > 0 && leidas.every((p) => p.extracto === '');
}
