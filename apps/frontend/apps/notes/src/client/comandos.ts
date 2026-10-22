/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los comandos de «/»: al escribir una barra al principio de una línea, o
 * detrás de un blanco, sale un menú con los bloques que se pueden poner ahí
 * —título, tarea, lista, cita, código, divisor, tabla— y lo que se sigue
 * escribiendo lo filtra. Es lo que hacen Twos y Notion, y aplica las MISMAS
 * órdenes que la barra del editor: nada nuevo que deshacer ni que sanear.
 *
 * Aquí solo vive la decisión (qué comandos hay, cuándo se abre el menú y qué
 * deja cada consulta); el DOM lo toca RichText. Su espejo en el escritorio es
 * `comandos.go` de apps/fyne/apps/notes, con la misma lista en el mismo orden.
 */
import { contiene, plegar } from '@sytue/i18n';
import type { NotesMessageKey } from '../i18n/index.js';

export type IdComando =
  | 'p'
  | 'h1'
  | 'h2'
  | 'h3'
  | 'tarea'
  | 'vinetas'
  | 'numeros'
  | 'cita'
  | 'codigo'
  | 'regla'
  | 'tabla';

export interface Comando {
  id: IdComando;
  clave: NotesMessageKey;
  /** Nombres cortos que también valen, en cualquier idioma: «/h1», «/todo», «/---». */
  alias: string[];
}

export const COMANDOS: readonly Comando[] = [
  { id: 'p', clave: 'editor.paragraph', alias: ['p', 'text'] },
  { id: 'h1', clave: 'editor.h1', alias: ['h1', '#'] },
  { id: 'h2', clave: 'editor.h2', alias: ['h2', '##'] },
  { id: 'h3', clave: 'editor.h3', alias: ['h3', '###'] },
  { id: 'tarea', clave: 'note.newTodo', alias: ['todo', 'task', '[]'] },
  { id: 'vinetas', clave: 'editor.bulletList', alias: ['ul', 'list', '-'] },
  { id: 'numeros', clave: 'editor.numberedList', alias: ['ol', '1.'] },
  { id: 'cita', clave: 'editor.quote', alias: ['quote', '>'] },
  { id: 'codigo', clave: 'editor.codeBlock', alias: ['code', '```'] },
  { id: 'regla', clave: 'editor.hr', alias: ['hr', 'divider', '---'] },
  { id: 'tabla', clave: 'editor.table', alias: ['table'] },
];

/** El tope de lo que se escribe tras la barra: más largo ya no es un comando. */
const MAX_CONSULTA = 20;

/**
 * Si el texto del bloque antes del cursor acaba en una barra que abre el menú,
 * lo escrito tras ella (la consulta, quizá vacía); si no, null. La barra vale al
 * principio o detrás de un blanco: «a/b» o una URL no son comandos.
 */
export function consultaDeBarra(antes: string): string | null {
  const m = /(?:^|\s)\/([^\s/]*)$/.exec(antes.replace(/\u00a0/g, ' '));
  if (!m) return null;
  const q = m[1] ?? '';
  return q.length <= MAX_CONSULTA ? q : null;
}

/** Los comandos que deja la consulta: por su nombre en el idioma de la app o por un alias. */
export function filtrarComandos(consulta: string, rotulo: (c: Comando) => string): Comando[] {
  const q = consulta.trim();
  if (!q) return [...COMANDOS];
  const pq = plegar(q);
  return COMANDOS.filter(
    (c) => contiene(rotulo(c), q) || c.alias.some((a) => plegar(a).startsWith(pq)),
  );
}
