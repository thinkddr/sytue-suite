/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los valores que un comando de la cinta admite cuando no son texto libre: las familias
 * de la lista, las tallas, los interlineados, los colores y las URL de imagen.
 *
 * Los usan DOS sitios y por eso viven aparte: las pestañas Inicio de Docus y de Diapos
 * (que antes llevaban cada una su copia de la lista) y la validación de las macros, que
 * no puede dejar pasar una fuente que la lista no ofrece. Módulo puro, sin DOM.
 */

/** Familias ofrecidas (las 6 de §6.3, en su orden; «Times» se declara como
 *  «Times New Roman» para coincidir con queryCommandValue('fontName')). */
export const FUENTES: readonly string[] = [
  'Inter',
  'Arial',
  'Georgia',
  'Times New Roman',
  'Courier New',
  'JetBrains Mono',
];

/** Escala `fontSize` de execCommand, la única que `exec('size')` admite. */
export const TALLAS: readonly string[] = ['1', '2', '3', '4', '5', '6', '7'];

/** Interlineados admitidos por `exec('lineHeight')`. */
export const INTERLINEADOS: readonly string[] = ['1', '1.15', '1.5', '2'];

/** Estilos de bloque de `exec('heading')`. */
export const BLOQUES: readonly string[] = ['p', 'h1', 'h2', 'h3', 'blockquote'];

/** Alineaciones verticales de una celda de tabla. */
export const ALINEACION_VERTICAL: readonly string[] = ['top', 'middle', 'bottom'];

const HEX = /^#(?:[0-9a-f]{3}|[0-9a-f]{6})$/i;

/** ¿Es un color de contenido? `#rgb` o `#rrggbb`; con `transparente`, también «transparent». */
export const esColor = (v: string, transparente = false): boolean =>
  HEX.test(v) || (transparente && v === 'transparent');

/** Sitios de los que puede venir una imagen o un medio sin salir de la casa. */
export interface OrigenesPermitidos {
  /** Origen de la página (`location.origin`). */
  readonly propio: string;
  /** Orígenes de Sytue Drive (los ficheros que se arrastran desde allí). */
  readonly drive: readonly string[];
}

const DATA_MEDIA = /^data:(?:image|audio|video)\/[a-z0-9.+-]+;base64,[a-z0-9+/=\s]*$/i;
const DATA_IMAGEN = /^data:image\/(?:png|jpe?g|gif|webp|avif|bmp);base64,[a-z0-9+/=\s]*$/i;

/**
 * ¿Es esta URL una fuente permitida para una imagen (o, con `medio`, un audio o vídeo)?
 *
 * Solo `data:` del tipo que toca, `blob:` de esta página, una ruta del propio origen o un
 * origen de Drive. Una macro es texto guardado y reproducido más tarde, así que una
 * dirección externa sería una petición a un tercero que nadie ve —y que un documento
 * compartido luego repetiría en cada lector—. SVG queda fuera a propósito: es código.
 */
export function urlPermitida(valor: string, origenes: OrigenesPermitidos, medio = false): boolean {
  if (valor === '') return false;
  if (valor.startsWith('data:')) return (medio ? DATA_MEDIA : DATA_IMAGEN).test(valor);
  if (valor.startsWith('blob:')) {
    try {
      return new URL(valor.slice(5)).origin === origenes.propio;
    } catch {
      return false;
    }
  }
  // «//otro-sitio» es una URL con esquema heredado y empieza por «/».
  if (valor.startsWith('/') && !valor.startsWith('//')) return true;
  try {
    const u = new URL(valor);
    if (u.protocol !== 'https:' && u.protocol !== 'http:') return false;
    return u.origin === origenes.propio || origenes.drive.includes(u.origin);
  } catch {
    return false;
  }
}
