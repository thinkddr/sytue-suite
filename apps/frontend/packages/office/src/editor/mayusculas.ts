/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Cambiar mayúsculas y minúsculas» de la pestaña Inicio: las cuatro transformaciones
 * de un procesador de textos (tipo oración, minúsculas, MAYÚSCULAS y Cada Palabra).
 *
 * Módulo PURO (sin DOM): `exec('changeCase')` lo aplica nodo de texto a nodo de texto y
 * la app nativa tiene su gemelo en `apps/fyne/editor/docus/rico/mayusculas.go`. Los dos
 * recorren los mismos vectores (`test/vectores/docus-mayusculas.json`): el mismo
 * documento se edita en las dos y una frase no puede salir distinta según dónde se pulse.
 *
 * Dos reglas que no son evidentes:
 *  - Carácter a carácter y SIN cambiar la longitud: «ß» no pasa a «SS» e «İ» pasa a «i»
 *    y no a «i̇» (el mapeo simple de Unicode, el de Go).
 *    Así cada nodo de texto conserva su largo, los rangos de la selección siguen valiendo
 *    y el formato no se desplaza ni una letra; Go hace lo mismo (`unicode.ToUpper`).
 *  - El estado cruza los nodos de texto de un mismo párrafo (una palabra partida por una
 *    negrita sigue siendo una palabra) y se reinicia en cada párrafo nuevo.
 */

/** Las cuatro, en el orden del menú. */
export const MODOS_MAYUSCULAS = ['sentence', 'lower', 'upper', 'title'] as const;
export type ModoMayusculas = (typeof MODOS_MAYUSCULAS)[number];

/**
 * Mayús+F3 de Word: el modo siguiente para el texto elegido, en el ciclo minúsculas →
 * MAYÚSCULAS → Cada Palabra → minúsculas. Lo decide el propio texto: todo en minúscula
 * pasa a mayúsculas; todo en mayúscula, a cada palabra; lo demás, a minúsculas. Se mide
 * con las mismas transformaciones de abajo, no con `toLowerCase`, para que la app nativa
 * (`rico.SiguienteModoMayusculas`) decida lo mismo con la misma frase.
 */
export function siguienteModoMayusculas(texto: string): ModoMayusculas {
  const con = (m: ModoMayusculas) => {
    const c = cambioDeMayusculas(m);
    c.parrafo();
    return c.tramo(texto);
  };
  if (con('lower') === texto) return 'upper';
  if (con('upper') === texto) return 'title';
  return 'lower';
}

export function esModoMayusculas(v: unknown): v is ModoMayusculas {
  return typeof v === 'string' && (MODOS_MAYUSCULAS as readonly string[]).includes(v);
}

const LETRA = /\p{L}/u;
/** Lo que sigue una palabra: letras, cifras, marcas combinantes y el apóstrofo («l’été»). */
const DE_PALABRA = /[\p{L}\p{N}\p{M}'’]/u;
const CIFRA = /\p{N}/u;
/** Lo que cierra una frase; la siguiente empieza tras un blanco. */
const FIN_DE_FRASE = /[.!?…]/u;
const BLANCO = /\s/u;

/**
 * Las letras que el `toUpperCase`/`toLowerCase` de JS alargan (el mapeo COMPLETO de
 * SpecialCasing) pero tienen un equivalente de una sola letra en el mapeo SIMPLE, que es
 * el de Go (`unicode.ToUpper`). Sin esto la «İ» turca seguiría «İ» en la web y pasaría a
 * «i» en la app nativa, y lo mismo la iota suscrita griega («ᾳ» → «ᾼ»).
 */
function simple(cp: number, mayuscula: boolean): number | null {
  if (!mayuscula) return cp === 0x130 ? 0x69 : null;
  // ᾀ-ᾇ, ᾐ-ᾗ, ᾠ-ᾧ → ᾈ-ᾏ, ᾘ-ᾟ, ᾨ-ᾯ
  if ((cp >= 0x1f80 && cp <= 0x1f87) || (cp >= 0x1f90 && cp <= 0x1f97)) return cp + 8;
  if (cp >= 0x1fa0 && cp <= 0x1fa7) return cp + 8;
  if (cp === 0x1fb3 || cp === 0x1fc3 || cp === 0x1ff3) return cp + 9;
  return null;
}

/** Mayúscula o minúscula de UN carácter, sin cambiar su longitud (ver arriba). */
function cambiar(ch: string, mayuscula: boolean): string {
  const otro = mayuscula ? ch.toUpperCase() : ch.toLowerCase();
  if (otro.length === ch.length && Array.from(otro).length === 1) return otro;
  const s = simple(ch.codePointAt(0) ?? 0, mayuscula);
  return s === null ? ch : String.fromCodePoint(s);
}

/** El que transforma los trozos de un párrafo seguidos, con su estado. */
export interface CambioDeMayusculas {
  /** Transforma el siguiente trozo de texto del párrafo en curso. */
  tramo: (texto: string) => string;
  /** Empieza un párrafo nuevo: frase y palabra nuevas. */
  parrafo: () => void;
  /**
   * Lo que hay en el párrafo ANTES de la selección, que no se cambia pero decide cómo
   * empieza: «la» elegido dentro de «hola» no es principio de palabra, y «MUNDO» en
   * «hola MUNDO» no es principio de frase.
   */
  previo: (texto: string) => void;
}

export function cambioDeMayusculas(modo: ModoMayusculas): CambioDeMayusculas {
  let inicioDeFrase = true; // la próxima letra abre frase
  let trasFin = false; // ha habido un «.» y falta el blanco que lo cierra
  let enPalabra = false;
  const caracter = (ch: string): string => {
    if (modo === 'upper') return cambiar(ch, true);
    if (modo === 'lower') return cambiar(ch, false);
    if (modo === 'title') {
      const out = LETRA.test(ch) ? cambiar(ch, !enPalabra) : ch;
      enPalabra = DE_PALABRA.test(ch);
      return out;
    }
    // Tipo oración: todo en minúscula salvo la primera letra de cada frase.
    if (LETRA.test(ch)) {
      const out = cambiar(ch, inicioDeFrase);
      inicioDeFrase = false;
      trasFin = false;
      return out;
    }
    if (FIN_DE_FRASE.test(ch)) trasFin = true;
    else if (BLANCO.test(ch)) inicioDeFrase ||= trasFin;
    // Una cifra no abre ni cierra frase («3.5» no es un final), pero sí anula el punto.
    else if (CIFRA.test(ch)) trasFin = false;
    return ch;
  };
  return {
    tramo: (texto) => Array.from(texto, caracter).join(''),
    previo: (texto) => {
      for (const ch of texto) caracter(ch);
    },
    parrafo: () => {
      inicioDeFrase = true;
      trasFin = false;
      enPalabra = false;
    },
  };
}
