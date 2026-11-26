/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Limpiar texto» de la pestaña Datos: quitar los espacios que sobran y cambiar las
 * mayúsculas del texto de las celdas elegidas, EN SU SITIO. Es lo que se hace con datos
 * pegados de otra parte —nombres en mayúsculas, códigos con un espacio delante que no
 * casan en un BUSCARV— y en una hoja solo había la vía larga: una columna auxiliar con
 * `=ESPACIOS()`, copiarla, pegar valores y borrarla.
 *
 * Solo se tocan los LITERALES: una fórmula calcula su texto y reescribirla la rompería.
 * Las mayúsculas son las de «Cambiar mayúsculas y minúsculas» de Docus (`mayusculas.ts`),
 * no otra regla: la misma frase sale igual en un documento y en una celda. Los espacios,
 * con el `\s` de JavaScript, que es el que Go replica (`editor.EsBlancoJS`). La app nativa
 * hace lo mismo en `apps/fyne/calcus/texto.go`, con los mismos vectores
 * (`test/vectores/calcus-herramientas.json`).
 */
import { cambioDeMayusculas, MODOS_MAYUSCULAS, type ModoMayusculas } from '../mayusculas';

/** «Recortar espacios» y los cuatro de las mayúsculas, en el orden del menú. */
export const MODOS_TEXTO_CELDAS = ['trim', ...MODOS_MAYUSCULAS] as const;
export type ModoTextoCeldas = 'trim' | ModoMayusculas;

export function esModoTextoCeldas(v: unknown): v is ModoTextoCeldas {
  return typeof v === 'string' && (MODOS_TEXTO_CELDAS as readonly string[]).includes(v);
}

/**
 * ¿Se puede reescribir el texto de esta celda sin perder nada? Solo si dentro del envoltorio
 * (`parseCell(...).inner`) no hay marcado ni más entidades que las cinco de `stripHtml`: un
 * `<br>` o un `&eacute;` se leerían de una manera y se escribirían de otra. Es la regla de
 * `celdaEntendida` de la app nativa.
 */
export function literalReescribible(inner: string): boolean {
  return !inner.includes('<') && !/&(?!(?:amp|lt|gt|quot|nbsp);)/.test(inner);
}

/* Los blancos de una línea: el `\s` de JS sin los saltos, que separan líneas. */
const BLANCOS_DE_LINEA = /[^\S\n\r]+/u;

/**
 * El texto de una celda transformado. «Recortar» es el ESPACIOS de las hojas de cálculo
 * línea a línea: fuera los blancos de los extremos, y los de dentro, de uno en uno (un
 * tabulador o un espacio duro también cuentan). Las líneas de una celda con saltos se
 * conservan, menos las vacías del principio y del final.
 */
export function transformarTextoCelda(texto: string, modo: ModoTextoCeldas): string {
  if (modo !== 'trim') {
    const cambio = cambioDeMayusculas(modo);
    cambio.parrafo();
    return cambio.tramo(texto);
  }
  const lineas = texto.split(/\r\n|\r|\n/).map((l) =>
    l
      .split(BLANCOS_DE_LINEA)
      .filter((p) => p !== '')
      .join(' '),
  );
  while (lineas.length > 0 && lineas[0] === '') lineas.shift();
  while (lineas.length > 0 && lineas[lineas.length - 1] === '') lineas.pop();
  return lineas.join('\n');
}
