/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL PARSER DE HTML DE NOTES, BAJO DEMANDA.
 *
 * Todo lo que lee el HTML de una nota sin tocar el DOM —el texto de la lista y de las
 * tarjetas, los títulos de los enlaces, el saneado de lo que se pega— va sobre parse5
 * (con entities, ~225 KB). Estaba en el JS inicial de Notes aunque la app, al
 * arrancar, solo pinta su armazón: sin notas no hay nada que leer.
 *
 * Ahora se baja aparte, en cuanto arranca la isla (`cargarHtml`), y en paralelo con la
 * bóveda y con la primera petición. La regla que lo hace seguro: NINGUNA nota llega a
 * la interfaz antes que el parser. `notesApi.pedir` (todo lo que viene del BFF) y las
 * copias del dispositivo (`cola.leerCuadernos`, `cola.leerNotas`) esperan a
 * `cargarHtml()` antes de devolver nada; desde ahí, todo sigue siendo síncrono igual
 * que antes. Si una función de aquí se llama antes, es un error de programación y lo
 * dice (no hay una lectura «de reserva» que sanee distinto).
 *
 * La página pública (`/p/<token>`) se pinta en el servidor con las notas dentro: su isla
 * registra los módulos de forma estática (`html-estatico.ts`), y las pruebas también.
 */
import type { DefaultTreeAdapterMap } from 'parse5';
import type * as Portapapeles from '@sytue/ui/browser/portapapeles';

export interface ModulosHtml {
  parse5: { parseFragment: (html: string) => DefaultTreeAdapterMap['documentFragment'] };
  portapapeles: Pick<typeof Portapapeles, 'htmlATexto' | 'pegadoConFormato' | 'textoPegado'>;
}

let modulos: ModulosHtml | undefined;
let cargando: Promise<void> | undefined;

/** Los módulos ya cargados de forma estática (la página pública, las pruebas). */
export function registrarHtml(m: ModulosHtml): void {
  modulos = m;
}

/** Baja parse5 y el saneador del pegado. Idempotente; si falla, el siguiente intento repite. */
export function cargarHtml(): Promise<void> {
  if (modulos) return Promise.resolve();
  cargando ??= Promise.all([import('parse5'), import('@sytue/ui/browser/portapapeles')]).then(
    ([parse5, portapapeles]) => {
      modulos ??= { parse5, portapapeles };
    },
    (error: unknown) => {
      cargando = undefined;
      throw error;
    },
  );
  return cargando;
}

/** ¿Ya se puede leer HTML sin esperar? */
export function htmlCargado(): boolean {
  return modulos !== undefined;
}

function cargados(): ModulosHtml {
  if (!modulos) throw new Error('notes: HTML leído antes de cargarHtml()');
  return modulos;
}

/** `parse5.parseFragment`: el árbol de un trozo de HTML, sin DOM ni cargas. */
export function parseFragment(html: string): DefaultTreeAdapterMap['documentFragment'] {
  return cargados().parse5.parseFragment(html);
}

/** HTML → texto plano (`@sytue/ui/browser/portapapeles`). */
export function htmlATexto(html: string): string {
  return cargados().portapapeles.htmlATexto(html);
}

/** El texto de un pegado, con el HTML como plan B (Safari). */
export function textoPegado(datos: Pick<DataTransfer, 'getData'> | null | undefined): string {
  return cargados().portapapeles.textoPegado(datos);
}

/** El saneado del pegado: la estructura del origen con una lista blanca. */
export function pegadoConFormato(html: string, opciones?: Portapapeles.OpcionesDePegado): string {
  return cargados().portapapeles.pegadoConFormato(html, opciones);
}
