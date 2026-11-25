/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los comandos de «/»: al escribir una barra al principio de una línea, o
 * detrás de un blanco, sale un menú con los bloques que se pueden poner ahí
 * —título, lista, tarea, cita, código, panel, tabla, estado, fecha…— y lo que se
 * sigue escribiendo lo filtra. Es el mismo catálogo que el menú «+» de la barra. Es lo que hacen los editores de notas, y aplica las MISMAS
 * órdenes que la barra del editor: nada nuevo que deshacer ni que sanear.
 *
 * Aquí solo vive la decisión (qué comandos hay, cuándo se abre el menú y qué
 * deja cada consulta); el DOM lo toca RichText. Su espejo en el escritorio es
 * `comandos.go` de apps/fyne/apps/notes, con la misma lista en el mismo orden.
 *
 * Y la de su vecino `[[`, que abre la lista de notas que enlazar: el menú es el
 * mismo, lo que cambia es qué ofrece.
 */
import { contiene, plegar } from '@sytue/i18n';
import type { NotesMessageKey } from '../i18n/index.js';

export type IdComando =
  | 'p'
  | 'h1'
  | 'h2'
  | 'h3'
  | 'vinetas'
  | 'numeros'
  | 'tarea'
  | 'decision'
  | 'cita'
  | 'codigo'
  | 'panelInfo'
  | 'panelNota'
  | 'panelExito'
  | 'panelAviso'
  | 'panelError'
  | 'expandir'
  | 'tabla'
  | 'columnas2'
  | 'columnas3'
  | 'imagen'
  | 'archivo'
  | 'regla'
  | 'estado'
  | 'fecha'
  | 'hoy'
  | 'enlace'
  | 'mencion'
  | 'simbolo'
  | 'emoji';

export interface Comando {
  id: IdComando;
  clave: NotesMessageKey;
  /** Una línea que dice qué hace, bajo el nombre (en el menú «+» y en «/»). */
  descripcion: NotesMessageKey;
  /** Nombres cortos que también valen, en cualquier idioma: «/h1», «/todo», «/---». */
  alias: string[];
  /** Lo que se teclea para lo mismo (el atajo de Markdown), a la derecha del menú. */
  pista?: string;
}

/**
 * EL CATÁLOGO DE LO QUE SE INSERTA: el mismo en «/» y en el menú «+» de la barra, como en
 * Confluence. El orden es el de los dos menús. Los bloques nuevos (panel, expandir,
 * estado, decisiones, fecha, columnas) escriben el HTML compartido con Fyne que deja
 * pasar el saneador (`BLOQUES_DE_NOTAS`).
 */
export const COMANDOS: readonly Comando[] = [
  { id: 'p', clave: 'editor.paragraph', descripcion: 'editor.paragraphDesc', alias: ['p', 'text'] },
  {
    id: 'h1',
    clave: 'editor.heading1',
    descripcion: 'editor.h1Desc',
    alias: ['h1', '#'],
    pista: '#',
  },
  {
    id: 'h2',
    clave: 'editor.heading2',
    descripcion: 'editor.h2Desc',
    alias: ['h2', '##'],
    pista: '##',
  },
  {
    id: 'h3',
    clave: 'editor.heading3',
    descripcion: 'editor.h3Desc',
    alias: ['h3', '###'],
    pista: '###',
  },
  {
    id: 'vinetas',
    clave: 'editor.bulletList',
    descripcion: 'editor.bulletListDesc',
    alias: ['ul', 'list', '-'],
    pista: '-',
  },
  {
    id: 'numeros',
    clave: 'editor.numberedList',
    descripcion: 'editor.numberedListDesc',
    alias: ['ol', '1.'],
    pista: '1.',
  },
  {
    id: 'tarea',
    clave: 'editor.taskList',
    descripcion: 'editor.taskListDesc',
    alias: ['todo', 'task', '[]'],
    pista: '[]',
  },
  {
    id: 'decision',
    clave: 'editor.decision',
    descripcion: 'editor.decisionDesc',
    alias: ['decision', '<>'],
  },
  {
    id: 'cita',
    clave: 'editor.quote',
    descripcion: 'editor.quoteDesc',
    alias: ['quote', '>'],
    pista: '>',
  },
  {
    id: 'codigo',
    clave: 'editor.codeBlock',
    descripcion: 'editor.codeBlockDesc',
    alias: ['code', '```'],
    pista: '```',
  },
  {
    id: 'panelInfo',
    clave: 'editor.panelInfo',
    descripcion: 'editor.panelDesc',
    alias: ['panel', 'info'],
  },
  {
    id: 'panelNota',
    clave: 'editor.panelNote',
    descripcion: 'editor.panelDesc',
    alias: ['panel', 'note'],
  },
  {
    id: 'panelExito',
    clave: 'editor.panelSuccess',
    descripcion: 'editor.panelDesc',
    alias: ['panel', 'success'],
  },
  {
    id: 'panelAviso',
    clave: 'editor.panelWarning',
    descripcion: 'editor.panelDesc',
    alias: ['panel', 'warning'],
  },
  {
    id: 'panelError',
    clave: 'editor.panelError',
    descripcion: 'editor.panelDesc',
    alias: ['panel', 'error'],
  },
  {
    id: 'expandir',
    clave: 'editor.expand',
    descripcion: 'editor.expandDesc',
    alias: ['expand', 'details', 'toggle'],
  },
  { id: 'tabla', clave: 'editor.table', descripcion: 'editor.tableDesc', alias: ['table'] },
  {
    id: 'columnas2',
    clave: 'editor.columns2',
    descripcion: 'editor.columnsDesc',
    alias: ['columns', 'layout', '2'],
  },
  {
    id: 'columnas3',
    clave: 'editor.columns3',
    descripcion: 'editor.columnsDesc',
    alias: ['columns', 'layout', '3'],
  },
  // Una imagen o un fichero: se guardan como adjuntos de la nota y el texto los nombra
  // (`adjunto:ID`, components/adjuntos-texto.ts). Solo donde hay nota a la que adjuntar.
  {
    id: 'imagen',
    clave: 'editor.image',
    descripcion: 'editor.imageDesc',
    alias: ['image', 'img', 'picture'],
  },
  {
    id: 'archivo',
    clave: 'editor.file',
    descripcion: 'editor.fileDesc',
    alias: ['file', 'attach'],
  },
  {
    id: 'regla',
    clave: 'editor.divider',
    descripcion: 'editor.hrDesc',
    alias: ['hr', 'divider', '---'],
    pista: '---',
  },
  { id: 'estado', clave: 'editor.status', descripcion: 'editor.statusDesc', alias: ['status'] },
  { id: 'fecha', clave: 'editor.date', descripcion: 'editor.dateDesc', alias: ['date'] },
  { id: 'hoy', clave: 'editor.today', descripcion: 'editor.todayDesc', alias: ['today', 'now'] },
  {
    id: 'enlace',
    clave: 'editor.linkNote',
    descripcion: 'editor.linkNoteDesc',
    alias: ['link', 'note', '[['],
    pista: '[[',
  },
  {
    id: 'mencion',
    clave: 'editor.mention',
    descripcion: 'editor.mentionDesc',
    alias: ['mention', '@'],
    pista: '@',
  },
  {
    id: 'simbolo',
    clave: 'editor.symbols',
    descripcion: 'editor.symbolsDesc',
    alias: ['symbol', 'char'],
  },
  {
    id: 'emoji',
    clave: 'editor.emoji',
    descripcion: 'editor.emojiDesc',
    alias: ['emoji', ':'],
    pista: ':',
  },
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

/**
 * Los comandos que deja la consulta: por su nombre en el idioma de la app, por su
 * descripción (en el menú «+», que se busca con frases) o por un alias. `disponibles`
 * quita lo que aquí no hay (sin cuaderno no se enlazan notas; sin gente, nadie a quien
 * mencionar).
 */
export function filtrarComandos(
  consulta: string,
  rotulo: (c: Comando) => string,
  descripcion?: (c: Comando) => string,
  disponibles: (c: Comando) => boolean = () => true,
): Comando[] {
  const q = consulta.trim();
  const todos = COMANDOS.filter(disponibles);
  if (!q) return todos;
  const pq = plegar(q);
  return todos.filter(
    (c) =>
      contiene(rotulo(c), q) ||
      (descripcion !== undefined && contiene(descripcion(c), q)) ||
      c.alias.some((a) => plegar(a).startsWith(pq)),
  );
}

/** El tope de lo que se escribe tras `[[`: el nombre de una nota cabe; más largo ya no es buscarla. */
const MAX_CONSULTA_ENLACE = 60;

/**
 * Si el texto del bloque antes del cursor acaba en un `[[` sin cerrar, lo escrito tras
 * los corchetes (la consulta, quizá vacía); si no, null. Vale en cualquier sitio de la
 * línea —«ver [[Presupuesto»— y la consulta admite espacios, que es como se llaman las
 * notas. Un corchete la cierra: `[[x]]` ya no está buscando nada.
 */
export function consultaDeEnlace(antes: string): string | null {
  const m = /\[\[([^[\]\n]*)$/.exec(antes.replace(/\u00a0/g, ' '));
  if (!m) return null;
  const q = m[1] ?? '';
  return q.length <= MAX_CONSULTA_ENLACE ? q : null;
}
