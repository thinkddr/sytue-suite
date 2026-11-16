/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Cabeceras A–Z de la lista (design.md §2.4: «List A–Z con Avatar»). Es la gemela de
 * `GruposPorInicial` en apps/fyne/contactos/iniciales.go y las dos pasan los mismos
 * vectores (apps/fyne/contactos/testdata/iniciales.json).
 *
 * La inicial NO es «la primera letra sin tilde»: qué es una letra propia lo decide la
 * colación del idioma, la misma que ordena la lista. En español la Ñ tiene cabecera y la
 * Á va con la A; en sueco y finés la Å, la Ä y la Ö van al final, cada una con la suya;
 * en alemán la Ä va con la A. Una tabla a mano se habría quedado corta con 26 idiomas:
 * se pregunta al cotejador si la letra y su base son la MISMA a nivel primario.
 */
import { intlTag, type Locale } from '@sytue/i18n';

/** Cabecera de lo que no empieza por letra (cifras, signos, vacío). Siempre al final. */
export const SIN_LETRA = '#';

export interface GrupoInicial<T> {
  inicial: string;
  items: T[];
}

const LETRA = /^\p{L}$/u;
const LATINA = /^\p{Script=Latin}$/u;
const A_Z = Array.from({ length: 26 }, (_, i) => String.fromCharCode(65 + i));

/** La primera letra de `s` (un punto de código, no una unidad UTF-16). */
function primera(s: string): string {
  return s.codePointAt(0) === undefined ? '' : String.fromCodePoint(s.codePointAt(0) as number);
}

/**
 * La cabecera de una letra: en mayúscula, y reducida a su base si la colación dice que
 * es la misma (Á → A en español, pero la Ñ se queda). Las que no se descomponen (Ø, Ł)
 * se prueban contra la A–Z latina: en inglés la Ø es una O; en danés, no.
 */
function inicialDeLetra(r: string, base: Intl.Collator): string {
  // Una sola letra: «ß».toUpperCase() son dos («SS»), y Go deja la ß como está.
  const mayus = r.toUpperCase();
  const u = primera(mayus) === mayus ? mayus : r;
  const b = primera(u.normalize('NFD'));
  if (b !== u && base.compare(u, b) === 0) return b;
  if (!A_Z.includes(u) && LATINA.test(u)) {
    const x = A_Z.find((l) => base.compare(u, l) === 0);
    if (x) return x;
  }
  return u;
}

/**
 * Agrupa por inicial en el orden en que llegan (la lista ya viene ordenada por la
 * colación del idioma), con «#» al final. Un grupo que la colación dejara partido se
 * junta en el sitio de su primera aparición.
 */
export function gruposPorInicial<T>(
  items: readonly T[],
  nombre: (item: T) => string,
  locale: Locale,
): GrupoInicial<T>[] {
  const base = new Intl.Collator(intlTag(locale), { sensitivity: 'base' });
  // Por primera letra: en una libreta de miles, las iniciales distintas son decenas.
  const cache = new Map<string, string>();
  const grupos: GrupoInicial<T>[] = [];
  const donde = new Map<string, GrupoInicial<T>>();
  const sinLetra: T[] = [];
  for (const item of items) {
    const r = primera(nombre(item).trim().normalize('NFC'));
    if (!LETRA.test(r)) {
      sinLetra.push(item);
      continue;
    }
    let inicial = cache.get(r);
    if (inicial === undefined) {
      inicial = inicialDeLetra(r, base);
      cache.set(r, inicial);
    }
    let g = donde.get(inicial);
    if (!g) {
      g = { inicial, items: [] };
      donde.set(inicial, g);
      grupos.push(g);
    }
    g.items.push(item);
  }
  if (sinLetra.length > 0) grupos.push({ inicial: SIN_LETRA, items: sinLetra });
  return grupos;
}

/** Una fila de la lista virtual: la cabecera de una inicial, o un contacto. */
export type FilaLista =
  { tipo: 'cabecera'; inicial: string } | { tipo: 'contacto'; indice: number };

export interface ListaConCabeceras<T> {
  /** Los contactos en el orden en que se ven: el cursor y j/k hablan en estos índices. */
  items: T[];
  /** Las filas de la lista virtual, cabeceras incluidas. */
  filas: FilaLista[];
  /** La fila de cada contacto (por su índice en `items`), para desplazar hasta él. */
  filaDe: number[];
}

/**
 * Las filas de la lista. Con `conCabeceras` (sin búsqueda, etiqueta ni destacados) van
 * por iniciales y lo que no empieza por letra pasa al final; sin él, una fila por
 * contacto y en el mismo orden. Con un filtro la lista es un resultado, no la libreta, y
 * una «A» con un solo contacto debajo no sitúa a nadie.
 */
export function listaConCabeceras<T>(
  items: readonly T[],
  nombre: (item: T) => string,
  locale: Locale,
  conCabeceras: boolean,
): ListaConCabeceras<T> {
  if (!conCabeceras) {
    return {
      items: [...items],
      filas: items.map((_, indice) => ({ tipo: 'contacto', indice })),
      filaDe: items.map((_, i) => i),
    };
  }
  const fuera: T[] = [];
  const filas: FilaLista[] = [];
  const filaDe: number[] = [];
  for (const g of gruposPorInicial(items, nombre, locale)) {
    filas.push({ tipo: 'cabecera', inicial: g.inicial });
    for (const item of g.items) {
      filaDe.push(filas.length);
      filas.push({ tipo: 'contacto', indice: fuera.length });
      fuera.push(item);
    }
  }
  return { items: fuera, filas, filaDe };
}
