/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Autoformato Markdown al escribir en el folio, como «Detectar Markdown» de Google Docs:
 *
 *  - Al principio de un párrafo, «# », «## » y «### » lo hacen título; «> », cita; «- »,
 *    «* » o «+ », lista de viñetas; «1. » o «1) », numerada (desde ese número); «[ ] » o
 *    «[x] », lista de casillas. Dentro de una viñeta, «[ ] » la pasa a casillas.
 *  - En la línea, cerrar `**texto**` o `__texto__` pone negrita; `*texto*` o `_texto_`,
 *    cursiva; `~~texto~~`, tachado.
 *
 * Todo va por execCommand, así que cada paso entra en el deshacer del navegador. El
 * carácter que se teclea se inserta primero, en un paso aparte: un Ctrl+Z JUSTO después
 * deshace solo el autoformato (deshacerAutoformato) y deja «# » escrito tal cual, que es
 * lo que se esperaba escribir si no se quería el título; el siguiente Ctrl+Z ya es el del
 * navegador.
 *
 * Encendido por defecto; la preferencia (Revisar › Autoformato Markdown) es de quien
 * escribe, por navegador, como «PDF accesible».
 */
import { editingCommandState, executeEditingCommand } from '@sytue/ui/browser/editing';
import { dejarEnLista, liDelCursor, marcarComoCasillas } from './casillas';

/** Lo que el autoformato pide al editor: un comando de edición (execCommand). */
export type Editar = (comando: string, valor?: string) => boolean;

const editarNativo: Editar = (comando, valor) => executeEditingCommand(comando, valor);

/** Si el estilo de escritura en el cursor lleva ese formato (queryCommandState). */
export type Estado = (formato: FormatoEnLinea) => boolean;

const estadoNativo: Estado = (formato) => editingCommandState(formato);

/* ---------- Preferencia ---------- */

const CLAVE = 'sytue.docs.autoformato';

/** ¿Autoformato encendido? Por defecto sí; solo un «0» guardado lo apaga. */
export function autoformatoActivo(): boolean {
  try {
    return localStorage.getItem(CLAVE) !== '0';
  } catch {
    return true; // ventana privada o almacenamiento bloqueado: el valor por defecto
  }
}

export function fijarAutoformato(activo: boolean): void {
  try {
    if (activo) localStorage.removeItem(CLAVE);
    else localStorage.setItem(CLAVE, '0');
  } catch {
    // sin almacenamiento la preferencia dura lo que la página
  }
}

/* ---------- Reglas (puras) ---------- */

export type ReglaDeBloque =
  | { tipo: 'titulo'; nivel: 1 | 2 | 3 }
  | { tipo: 'cita' }
  | { tipo: 'vinetas' }
  | { tipo: 'numerada'; inicio: number }
  | { tipo: 'casillas'; hecha: boolean };

/**
 * La regla de bloque que dispara un espacio tecleado tras `antes`, el texto del párrafo
 * desde su principio hasta el cursor (sin el espacio). `enVineta`: el párrafo es un
 * elemento de una lista de viñetas, donde solo vale «[ ]» (pasarla a casillas).
 */
export function reglaDeBloque(antes: string, enVineta = false): ReglaDeBloque | null {
  const s = antes.replace(/ /g, ' ');
  const casilla = /^\[( |x|X)?\]$/.exec(s);
  if (casilla) return { tipo: 'casillas', hecha: casilla[1] === 'x' || casilla[1] === 'X' };
  if (enVineta) return null;
  if (/^#{1,3}$/.test(s)) return { tipo: 'titulo', nivel: s.length as 1 | 2 | 3 };
  if (s === '>') return { tipo: 'cita' };
  if (/^[-*+]$/.test(s)) return { tipo: 'vinetas' };
  const n = /^(\d{1,3})[.)]$/.exec(s);
  if (n) return { tipo: 'numerada', inicio: Number(n[1]) };
  return null;
}

export type FormatoEnLinea = 'bold' | 'italic' | 'strikeThrough';

export interface ReglaEnLinea {
  formato: FormatoEnLinea;
  /** Dónde empieza la marca de apertura en el texto. */
  desde: number;
  /** El texto que queda formateado, sin las marcas. */
  texto: string;
}

/*
 * Cada patrón casa el FINAL del texto: la marca de cierre recién escrita. El contenido
 * no empieza ni acaba en blanco (como en CommonMark), y la apertura no va pegada a una
 * letra (`snake_case_var` y `2*3*4` no son cursivas) ni a la misma marca (`**negrita*`
 * aún no es cursiva: es la negrita a medio cerrar).
 */
const PATRONES: ReadonlyArray<{ re: RegExp; formato: FormatoEnLinea }> = [
  { re: /(^|[^*\p{L}\p{N}])\*\*([^*\s](?:[^*]*[^*\s])?)\*\*$/u, formato: 'bold' },
  { re: /(^|[^_\p{L}\p{N}])__([^_\s](?:[^_]*[^_\s])?)__$/u, formato: 'bold' },
  { re: /(^|[^~\p{L}\p{N}])~~([^~\s](?:[^~]*[^~\s])?)~~$/u, formato: 'strikeThrough' },
  { re: /(^|[^*\p{L}\p{N}])\*([^*\s](?:[^*]*[^*\s])?)\*$/u, formato: 'italic' },
  { re: /(^|[^_\p{L}\p{N}])_([^_\s](?:[^_]*[^_\s])?)_$/u, formato: 'italic' },
];

/** La regla en línea que cierra `texto` (lo escrito hasta el cursor, marca incluida). */
export function reglaEnLinea(texto: string): ReglaEnLinea | null {
  for (const { re, formato } of PATRONES) {
    const m = re.exec(texto);
    if (!m) continue;
    const previo = m[1] ?? '';
    return { formato, desde: m.index + previo.length, texto: m[2] ?? '' };
  }
  return null;
}

/** Teclas que pueden cerrar una marca en línea. */
const CIERRES = new Set(['*', '_', '~']);

/* ---------- Deshacer inmediato ---------- */

interface Pendiente {
  /** Pasos de execCommand que hizo el autoformato, sin contar lo tecleado. */
  pasos: number;
  /** Lo que no entra en el deshacer del navegador (atributos de la lista de casillas). */
  revertir?: (() => void) | undefined;
  editar: Editar;
}

let pendiente: Pendiente | null = null;
let aplicando = false;

/** Lo siguiente que haga quien escribe ya no es «deshacer el autoformato». */
export function olvidarAutoformato(): void {
  pendiente = null;
}

export function hayAutoformatoPendiente(): boolean {
  return pendiente !== null;
}

/**
 * Atiende el `keydown` del folio: Ctrl/⌘+Z justo después de un autoformato lo deshace y
 * deja la marca escrita; cualquier otra tecla lo da por aceptado. Devuelve si la consumió.
 */
export function deshacerAutoformato(
  e: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'shiftKey' | 'altKey' | 'preventDefault'>,
): boolean {
  if (!pendiente) return false;
  if (e.key === 'Control' || e.key === 'Meta' || e.key === 'Shift' || e.key === 'Alt') return false;
  const p = pendiente;
  pendiente = null;
  if (!(e.ctrlKey || e.metaKey) || e.shiftKey || e.altKey || e.key.toLowerCase() !== 'z') {
    return false;
  }
  e.preventDefault();
  aplicando = true;
  try {
    p.revertir?.();
    for (let i = 0; i < p.pasos; i++) p.editar('undo');
  } finally {
    aplicando = false;
  }
  return true;
}

/* ---------- Aplicación sobre el folio ---------- */

/** Dentro de estos no hay autoformato: islas, campos, código, regiones y tablas. */
const VEDADO =
  '[contenteditable="false"],[data-formfield],pre,code,figure,nav,table,[data-doc-header],[data-doc-footer]';
const BLOQUES = 'p,div,li,h1,h2,h3,h4,h5,h6,blockquote,pre,td,th';

const aElemento = (n: Node): Element | null =>
  n.nodeType === Node.ELEMENT_NODE ? (n as Element) : n.parentElement;

/** El bloque del cursor: el elemento de bloque más cercano dentro del folio, o el folio. */
function bloqueDe(root: HTMLElement, nodo: Node): Element {
  const b = aElemento(nodo)?.closest(BLOQUES);
  return b && b !== root && root.contains(b) ? b : root;
}

const vedado = (root: HTMLElement, nodo: Node): boolean => {
  const v = aElemento(nodo)?.closest(VEDADO);
  return !!v && v !== root && root.contains(v);
};

/** El texto del bloque desde su principio hasta (nodo, offset). */
function textoHasta(bloque: Element, nodo: Node, offset: number): string {
  const r = document.createRange();
  r.selectNodeContents(bloque);
  r.setEnd(nodo, offset);
  return r.toString();
}

function seleccionar(r: Range): void {
  const sel = document.getSelection();
  sel?.removeAllRanges();
  sel?.addRange(r);
}

/** El cursor, si está colapsado y dentro del folio. */
function cursor(root: HTMLElement): { nodo: Node; offset: number } | null {
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0 || !sel.isCollapsed || !sel.anchorNode) return null;
  if (!root.contains(sel.anchorNode)) return null;
  return { nodo: sel.anchorNode, offset: sel.anchorOffset };
}

/**
 * Atiende lo que se va a escribir en el folio: el `keydown` de la tecla (escritorio, donde
 * execCommand vale siempre) o, si no la identifica (teclados virtuales), su `beforeinput`.
 * Si dispara una regla, se queda la tecla (`preventDefault`), la escribe él y aplica el
 * formato. Devuelve si lo hizo.
 */
export function autoformatear(
  root: HTMLElement,
  e: Pick<InputEvent, 'inputType' | 'data' | 'isComposing' | 'preventDefault'>,
  editar: Editar = editarNativo,
  estado: Estado = estadoNativo,
): boolean {
  if (aplicando || e.isComposing || e.inputType !== 'insertText' || !e.data) return false;
  if (e.data !== ' ' && !CIERRES.has(e.data)) return false;
  const c = cursor(root);
  if (!c || vedado(root, c.nodo)) return false;
  aplicando = true;
  try {
    return e.data === ' '
      ? deBloque(root, c.nodo, c.offset, e, editar)
      : enLinea(c.nodo, c.offset, e, editar, estado);
  } finally {
    aplicando = false;
  }
}

function deBloque(
  root: HTMLElement,
  nodo: Node,
  offset: number,
  e: Pick<InputEvent, 'preventDefault'>,
  editar: Editar,
): boolean {
  const bloque = bloqueDe(root, nodo);
  const enLi = bloque.tagName === 'LI';
  const lista = enLi ? bloque.parentElement : null;
  const enVineta = enLi && lista?.tagName === 'UL' && !lista.hasAttribute('data-checklist');
  // Un título, una cita o una casilla ya son lo suyo; en una numerada no se cambia nada.
  if (enLi ? !enVineta : !/^(P|DIV)$/.test(bloque.tagName) && bloque !== root) return false;
  const regla = reglaDeBloque(textoHasta(bloque, nodo, offset), enVineta);
  if (!regla) return false;

  // Lo tecleado, en su propio paso de deshacer. Si el navegador no deja editar aquí, la
  // tecla sigue su camino normal: mejor sin formato que perderla.
  if (!editar('insertText', ' ')) return false;
  e.preventDefault();
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0) return true;
  // La marca, del principio del bloque al cursor (con el espacio recién escrito).
  const marca = document.createRange();
  marca.selectNodeContents(bloque);
  marca.setEnd(sel.anchorNode ?? nodo, sel.anchorOffset);
  seleccionar(marca);
  let pasos = 0;
  if (editar('delete')) pasos++;

  let revertir: (() => void) | undefined;
  switch (regla.tipo) {
    case 'titulo':
      if (editar('formatBlock', `<h${regla.nivel}>`)) pasos++;
      break;
    case 'cita':
      if (editar('formatBlock', '<blockquote>')) pasos++;
      break;
    case 'vinetas':
      if (editar('insertUnorderedList')) pasos++;
      revertir = dejarEnLista(root, false, false);
      break;
    case 'numerada': {
      if (editar('insertOrderedList')) pasos++;
      const ol = liDelCursor(root)?.parentElement;
      if (ol?.tagName === 'OL' && regla.inicio !== 1 && ol.children.length === 1) {
        ol.setAttribute('start', String(regla.inicio));
      }
      break;
    }
    case 'casillas':
      if (enVineta && lista) {
        // La viñeta ya es lista: se pasa la lista entera, como el botón de la cinta.
        revertir = marcarComoCasillas(lista, bloque, regla.hecha);
      } else {
        if (editar('insertUnorderedList')) pasos++;
        revertir = dejarEnLista(root, true, regla.hecha);
      }
      break;
  }
  pendiente = { pasos, revertir, editar };
  root.dispatchEvent(new Event('input', { bubbles: true }));
  return true;
}

function enLinea(
  nodo: Node,
  offset: number,
  e: Pick<InputEvent, 'data' | 'preventDefault'>,
  editar: Editar,
  estado: Estado,
): boolean {
  if (!(nodo instanceof Text)) return false;
  // Solo dentro de un mismo nodo de texto: lo de dentro de las marcas tiene un solo formato.
  const regla = reglaEnLinea(nodo.data.slice(0, offset) + (e.data ?? ''));
  if (!regla) return false;
  if (!editar('insertText', e.data ?? '')) return false; // lo tecleado, en su propio paso
  e.preventDefault();
  const sel = document.getSelection();
  const t = sel?.anchorNode;
  if (!sel || !(t instanceof Text)) return true;
  const fin = sel.anchorOffset;
  const largo = fin - regla.desde;
  if (largo <= 0 || t.data.length < fin) return true;
  const r = document.createRange();
  r.setStart(t, regla.desde);
  r.setEnd(t, fin);
  seleccionar(r);
  const etiqueta = { bold: 'b', italic: 'i', strikeThrough: 's' }[regla.formato];
  const pasos = editar('insertHTML', `<${etiqueta}>${escapar(regla.texto)}</${etiqueta}>`) ? 1 : 0;
  // Lo que se escriba después ya no lleva el formato. Chrome deja el cursor al final del
  // <b> y Firefox, detrás: el comando (con el cursor colapsado solo cambia el estilo de
  // escritura, sin paso propio) va solo si el cursor aún lo lleva, o lo encendería.
  if (pasos > 0 && estado(regla.formato)) editar(regla.formato);
  pendiente = { pasos, editar };
  return true;
}

const escapar = (s: string): string =>
  s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
