// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import type { DefaultTreeAdapterMap } from 'parse5';
import { parseFragment } from './html.js';

const HIDDEN = new Set(['script', 'style', 'template', 'noscript', 'iframe', 'object', 'embed']);
const BLOCKS = new Set([
  'br',
  'hr',
  'p',
  'div',
  'li',
  'ul',
  'ol',
  'blockquote',
  'pre',
  'h1',
  'h2',
  'h3',
  'h4',
  'h5',
  'h6',
  'table',
  'thead',
  'tbody',
  'tfoot',
  'tr',
  'td',
  'th',
]);

/** Plain text of a note body, never HTML: parse without a browser DOM or resource loads. */
export function textoDeNota(html: string): string {
  const pending: (DefaultTreeAdapterMap['childNode'] | string)[] = [
    ...parseFragment(html).childNodes,
  ].reverse();
  const parts: string[] = [];
  while (pending.length) {
    const node = pending.pop()!;
    if (typeof node === 'string') {
      parts.push(node);
    } else if ('value' in node) {
      parts.push(node.value);
    } else if ('tagName' in node && !HIDDEN.has(node.tagName)) {
      if (BLOCKS.has(node.tagName)) {
        parts.push(' ');
        pending.push(' ');
      }
      for (let i = node.childNodes.length - 1; i >= 0; i--) pending.push(node.childNodes[i]!);
    }
  }
  return parts.join('').replace(/\s+/g, ' ').trim();
}

/** Plain-text preview, never HTML: parse without a browser DOM or resource loads. */
export function extracto(html: string, max = 72): string {
  const text = textoDeNota(html);
  return text.length > max ? `${text.slice(0, max - 1)}…` : text;
}

/** Lo que dice «Información» de una nota: cuánto texto hay y cuánto se tarda en leer. */
export interface Estadisticas {
  palabras: number;
  caracteres: number;
  /** Minutos de lectura a {@link PALABRAS_POR_MINUTO}, redondeando hacia arriba. */
  minutos: number;
}

/** El ritmo de lectura silenciosa que usan Medium, Bear o Notion para el «x min». */
export const PALABRAS_POR_MINUTO = 200;

// Han, hiragana y katakana se escriben sin espacios: cada carácter cuenta como una
// palabra, como hace Word con el texto de Asia oriental. El coreano (hangul) sí separa
// las palabras con espacios y cuenta como cualquier otro alfabeto. No se usa
// `Intl.Segmenter`: necesita un diccionario que Go no tiene, y las dos superficies
// tienen que dar los MISMOS números (vectores en estadisticas.json).
const CJK = /[\p{Script=Han}\p{Script=Hiragana}\p{Script=Katakana}]/u;
const LETRA = /[\p{L}\p{N}]/u;
// Una marca combinante (acento suelto, dakuten de un kana en NFD, el selector de
// variante de «❤️») va con lo que tiene delante y nunca empieza palabra: si no, un emoji
// o un «が» descompuesto contarían de más.
const MARCA = /\p{M}/u;
const DIGITO = /\p{Nd}/u;
// Lo que une dos trozos de una misma palabra si va entre letras («don't», «e-mail»,
// «l’hiver») o entre cifras («1.250», «3,5»).
const UNE_LETRAS = new Set(["'", '’', '-', '‐']);
const UNE_CIFRAS = new Set(['.', ',']);

/**
 * Palabras, caracteres y minutos de lectura de un texto PLANO (el de
 * {@link textoDeNota}). Los caracteres son puntos de código del texto con los blancos
 * juntados en un espacio —no unidades UTF-16: un emoji cuenta uno—. Espejo de
 * `htmltext.Stats` en `apps/nucleo`.
 */
export function estadisticas(texto: string): Estadisticas {
  const cps = Array.from(texto.replace(/\s+/g, ' ').trim());
  let palabras = 0;
  let enPalabra = false;
  for (let i = 0; i < cps.length; i++) {
    const c = cps[i]!;
    if (MARCA.test(c)) {
      // Sigue lo que hubiera: ni empieza ni corta una palabra.
    } else if (CJK.test(c)) {
      palabras++;
      enPalabra = false;
    } else if (LETRA.test(c)) {
      if (!enPalabra) palabras++;
      enPalabra = true;
    } else if (enPalabra && i + 1 < cps.length && une(cps[i - 1]!, c, cps[i + 1]!)) {
      // Sigue la misma palabra.
    } else {
      enPalabra = false;
    }
  }
  return {
    palabras,
    caracteres: cps.length,
    minutos: Math.ceil(palabras / PALABRAS_POR_MINUTO),
  };
}

function une(antes: string, c: string, despues: string): boolean {
  if (CJK.test(despues) || !LETRA.test(despues)) return false;
  if (UNE_LETRAS.has(c)) return true;
  return UNE_CIFRAS.has(c) && DIGITO.test(antes) && DIGITO.test(despues);
}
