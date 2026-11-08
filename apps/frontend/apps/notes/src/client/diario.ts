// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import type { Note, NoteInput, Notebook } from '@sytue/api-clients';
import { esMac } from '@sytue/ui';
import { escapar, interpretarCaptura } from './captura.js';
import { sinEtiquetas } from './enlaces.js';
import { textoDeNota } from './extracto.js';

export const ENFOQUES = ['all', 'pending', 'done', 'pinned'] as const;
export type Enfoque = (typeof ENFOQUES)[number];

/** Día civil local: UTC cambia de día antes/después que la persona. */
export function diaLocal(d = new Date()): string {
  return `${String(d.getFullYear()).padStart(4, '0')}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
}
export function desplazarDia(dia: string, delta: number): string {
  const d = new Date(`${dia}T12:00:00`);
  d.setDate(d.getDate() + delta);
  return diaLocal(d);
}

/** Una tarea terminada: hecha o cancelada. Las canceladas se tachan como las hechas. */
export function terminada(n: Note): boolean {
  return n.kind === 'todo' && (n.checked || Boolean(n.cancelled_at));
}

/** Cuántas veces se ha trasladado de un día a otro (`data.carry_count`). */
export function vecesTrasladada(n: Note): number {
  const v = n.data?.['carry_count'];
  return typeof v === 'number' && Number.isInteger(v) && v > 0 ? v : 0;
}

/**
 * Filtra sin dejar las notas coincidentes huérfanas: incluye sus antepasados.
 * `ocultarTerminadas` quita las tareas hechas o canceladas, salvo las que hacen
 * falta como antepasado de una que sí se ve.
 */
export function enfocarNotas(
  notes: readonly Note[],
  enfoque: Enfoque,
  ocultarTerminadas = false,
): Note[] {
  const pasa = (n: Note): boolean => {
    if (ocultarTerminadas && terminada(n)) return false;
    if (enfoque === 'all') return true;
    if (enfoque === 'pinned') return n.pinned;
    if (n.kind !== 'todo' || n.cancelled_at) return false;
    return n.checked === (enfoque === 'done');
  };
  if (enfoque === 'all' && !ocultarTerminadas) return [...notes];
  const porId = new Map(notes.map((n) => [n.id, n]));
  const ids = new Set<string>();
  for (const n of notes) {
    if (!pasa(n)) continue;
    let actual: Note | undefined = n;
    while (actual && !ids.has(actual.id)) {
      ids.add(actual.id);
      actual = actual.group_id ? porId.get(actual.group_id) : undefined;
    }
  }
  return notes.filter((n) => ids.has(n.id));
}

/**
 * Los marcadores son explícitos: una frase normal nunca se
 * reclasifica. `[ ] `, `[] `, `[x] ` o `- ` al principio hacen una tarea (el guion
 * es el de una lista apuntada a mano: la línea que empieza por él es algo que hacer), y
 * `---` solo es un separador.
 */
export function capturaRapida(texto: string, tipo: 'text' | 'todo'): NoteInput | null {
  const s = texto.trim();
  if (!s) return null;
  if (s === '---') return { kind: 'text', content: '<hr>' };
  const caja = /^(?:-\s*)?\[([ xX]?)\]\s+(.+)$/s.exec(s);
  // El guion solo vale en UNA línea: un texto pegado con viñetas es un texto.
  const guion = caja ? null : /^-[ \t]+([^\n]+)$/.exec(s);
  if (tipo === 'todo' || caja || guion) {
    const title = (caja?.[2] ?? guion?.[1] ?? s).trim();
    return title ? { kind: 'todo', title, checked: Boolean(caja?.[1]?.trim()) } : null;
  }
  const c = interpretarCaptura(s);
  if (!c) return null;
  return c.kind === 'link'
    ? { kind: 'link', title: c.title, data: c.data }
    : { kind: 'text', content: c.content };
}

/** Lo más que nace de UNA vez al pegar varias líneas; lo que sobra se queda en la captura. */
export const MAX_LINEAS_PEGADAS = 50;

/**
 * Las líneas con algo de lo que hay en la captura, en orden: cada una es una nota, con
 * la misma sintaxis que al teclearla (`capturaRapida`). Pegar una lista apuntada en otra
 * parte da una fila por línea, que es lo que se ve en el papel. Las líneas en blanco no
 * son nada, y el `\r` de un portapapeles de Windows tampoco.
 */
export function lineasDeCaptura(texto: string): string[] {
  return texto
    .split(/\r\n|\r|\n/)
    .map((l) => l.trim())
    .filter(Boolean);
}

/** El título de una tarea, en runas: el tope de la captura y el `maxNoteTitle` del servicio. */
export const MAX_TITULO_TAREA = 400;

/** Los bloques que acaban una línea del cuerpo de una nota. */
const BLOQUE = 'p,div,li,h1,h2,h3,h4,h5,h6,blockquote,pre,td,th';
const FIN_DE_LINEA = `br,hr,ul,ol,table,${BLOQUE}`;

/**
 * La primera línea de un cuerpo HTML como texto, y el cuerpo sin ella con su formato.
 * Una línea acaba en un salto (`<br>`) o donde acaba su bloque; lo que queda vacío al
 * quitarla (el párrafo, la lista) se va con ella.
 */
export function primeraLinea(html: string): { linea: string; resto: string } {
  // En una `<template>`: su contenido es inerte —ni se cargan imágenes ni corre nada—,
  // como el de DOMParser, y admite rangos en cualquier DOM.
  const plantilla = document.createElement('template');
  plantilla.innerHTML = html;
  const raiz = plantilla.content;
  const textos = document.createTreeWalker(raiz, NodeFilter.SHOW_TEXT);
  let primero: Node | null = null;
  while (textos.nextNode()) {
    if (textos.currentNode.nodeValue?.trim()) {
      primero = textos.currentNode;
      break;
    }
  }
  if (!primero) return { linea: '', resto: html };
  const bloque = primero.parentElement?.closest(BLOQUE);
  const contenedor: Element | DocumentFragment = bloque && raiz.contains(bloque) ? bloque : raiz;
  // Donde acaba la línea: el primer salto o bloque que viene DETRÁS de su primer texto.
  let fin: Element | null = null;
  for (const el of contenedor.querySelectorAll(FIN_DE_LINEA)) {
    if (primero.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING) {
      fin = el;
      break;
    }
  }
  // Desde su primer texto con lo que lo envuelve (la negrita de la palabra), no desde
  // el principio del bloque: una imagen que vaya delante no es parte de la línea.
  let inicio: Node = primero;
  while (
    inicio.parentNode &&
    inicio.parentNode !== contenedor &&
    !(inicio.parentNode as Element).querySelector('img')
  )
    inicio = inicio.parentNode;
  const rango = document.createRange();
  rango.setStartBefore(inicio);
  if (fin) rango.setEndBefore(fin);
  else rango.setEnd(contenedor, contenedor.childNodes.length);
  const linea = rango.toString().replace(/\s+/g, ' ').trim();
  rango.deleteContents();
  if (fin?.tagName === 'BR') fin.remove();
  let vacio = contenedor instanceof Element ? contenedor : null;
  while (vacio && !vacio.textContent?.trim() && !vacio.querySelector('img,hr,table')) {
    const padre: Element | null = vacio.parentElement;
    vacio.remove();
    vacio = padre;
  }
  return { linea, resto: plantilla.innerHTML.trim() };
}

/** Un cuerpo sin nada que leer: vacío, o párrafos sin texto. */
function sinTexto(html: string): boolean {
  return textoDeNota(html) === '' && !/<(img|hr|table)\b/i.test(html);
}

/** Lo que cambia al convertir una línea: su tipo y, si hace falta, de dónde sale su texto. */
export interface Conversion {
  kind: 'text' | 'todo';
  title: string;
  content: string;
  checked?: boolean;
  cancelled?: boolean;
}

/**
 * Una cosa apuntada pasa a ser una tarea, o al revés, sin perder nada de lo escrito.
 *
 * - De texto a tarea: sin título, la primera línea del texto lo es y el resto se queda
 *   debajo, con su formato. Si esa línea no cabe en el título de una tarea, el título
 *   lleva su principio y el texto se queda entero.
 * - De tarea a texto: sin nada debajo, el título pasa a ser el texto; con algo, se
 *   quedan los dos. Hecha o cancelada, deja de estarlo: un texto no tiene casilla, y
 *   al volver a ser tarea sería una cosa por hacer que se vería tachada.
 */
export function convertirNota(n: {
  kind: 'text' | 'todo';
  title: string;
  content: string;
}): Conversion {
  const title = n.title.trim();
  if (n.kind === 'todo') {
    return title && sinTexto(n.content)
      ? {
          kind: 'text',
          title: '',
          content: `<p>${escapar(title)}</p>`,
          checked: false,
          cancelled: false,
        }
      : { kind: 'text', title, content: n.content, checked: false, cancelled: false };
  }
  if (title) return { kind: 'todo', title, content: n.content };
  const { linea, resto } = primeraLinea(n.content);
  // Por puntos de código y no por grafemas: es como cuenta el servicio (`RuneCountInString`).
  const runas = Array.from(linea);
  return runas.length > MAX_TITULO_TAREA
    ? {
        kind: 'todo',
        title: `${runas.slice(0, MAX_TITULO_TAREA - 1).join('')}…`,
        content: n.content,
      }
    : { kind: 'todo', title: linea, content: resto };
}

/**
 * `dd` y `tt` seguidos de un espacio se cambian por la fecha y la hora, como en
 * un cuaderno de notas rápido. Solo la palabra que se acaba de teclear (al final y suelta): `add ` o un
 * `dd` en mitad de una frase ya escrita no se tocan. Fecha ISO y hora de 24 h,
 * las mismas en la web y en Fyne y en todos los idiomas.
 */
export function expandirAtajos(texto: string, ahora = new Date()): string {
  return texto.replace(
    /(^|\s)(dd|tt) $/,
    (_, antes: string, k: string) =>
      `${antes}${k === 'dd' ? diaLocal(ahora) : `${String(ahora.getHours()).padStart(2, '0')}:${String(ahora.getMinutes()).padStart(2, '0')}`} `,
  );
}

/**
 * Lo que la captura hace con cada cambio: solo expande si lo último fue teclear un
 * espacio. Borrar «dd x» hasta «dd » no es haber escrito la palabra.
 */
export function expandirCaptura(previo: string, actual: string, ahora = new Date()): string {
  return actual === `${previo} ` ? expandirAtajos(actual, ahora) : actual;
}

export type AtajoDiario = 'today' | 'previous' | 'next' | 'capture';

/**
 * ⌘D (Ctrl+D fuera del Mac) abre hoy, ⌘⇧← y ⌘⇧→ el día anterior y el siguiente,
 * y ⌘K lleva a la captura. Solo el modificador principal del sistema, como Fyne:
 * en macOS Ctrl+D es «borrar hacia delante» dentro de cualquier campo. Solo decide
 * qué tecla es qué; dónde valen lo decide quien escucha.
 */
export function atajoDiario(
  e: Pick<KeyboardEvent, 'key' | 'metaKey' | 'ctrlKey' | 'shiftKey' | 'altKey'>,
  mac = esMac(),
): AtajoDiario | null {
  const mod = mac ? e.metaKey && !e.ctrlKey : e.ctrlKey && !e.metaKey;
  if (!mod || e.altKey) return null;
  if (e.shiftKey)
    return e.key === 'ArrowLeft' ? 'previous' : e.key === 'ArrowRight' ? 'next' : null;
  const k = e.key.toLowerCase();
  return k === 'd' ? 'today' : k === 'k' ? 'capture' : null;
}

/**
 * Los diarios PROPIOS no van al árbol: uno por día lo entierra en un mes, y un
 * día no es una lista de la barra. A un día se llega por «Hoy», por la tira
 * de la semana y sus flechas, y a los viejos por la vista «Días», que enseña solo los
 * que tienen algo y los abre sin crear diarios vacíos por el camino. (Mientras esa
 * vista no existía, los diarios con notas se quedaban en el árbol: era la única forma
 * de volver a ellos.) Se queda el que tiene cuadernos dentro, o sus hijos no tendrían
 * por dónde verse, y el de OTRA persona compartido conmigo (`sub`): ese no se abre por
 * «Hoy» —va por `owner_sub`—, así que sigue en el árbol como un cuaderno más.
 */
export function cuadernosDelArbol(notebooks: readonly Notebook[], sub: string): Notebook[] {
  const padres = new Set(notebooks.map((n) => n.parent_id).filter(Boolean));
  return notebooks.filter((n) => !(n.daily_date && n.owner_sub === sub) || padres.has(n.id));
}

/** Los siete días civiles de la semana de `dia`, de lunes a domingo. */
export function semanaDe(dia: string): string[] {
  const lunes = desplazarDia(dia, -((new Date(`${dia}T12:00:00`).getDay() + 6) % 7));
  return Array.from({ length: 7 }, (_, i) => desplazarDia(lunes, i));
}

/**
 * Una línea que dice de qué va la nota en las vistas Semana, Días y «Este día»: el
 * título o, si no hay, el texto del cuerpo sin etiquetas. Nunca HTML: se pinta como
 * texto. Las cifradas no tienen nada que enseñar aquí (el servidor ya no las manda).
 */
export function resumenNota(n: Pick<Note, 'title' | 'content'>, max = 140): string {
  const base =
    n.title?.trim() ||
    sinEtiquetas((n.content ?? '').replace(/<(br|\/p|\/li|\/h\d|\/div)\b[^>]*>/gi, ' '))
      .replace(/\s+/g, ' ')
      .trim();
  return base.length > max ? `${base.slice(0, max - 1)}…` : base;
}

/** El tipo de dato con que una nota viaja al arrastrarla sobre un día. */
export const TIPO_ARRASTRE_NOTA = 'text/x-sytue-note-id';
