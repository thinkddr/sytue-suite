/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Listas de casillas del folio: una `<ul data-checklist>` cuyos `<li>` llevan
 * `data-checked="1"` (hecha) o `"0"`. Es una lista de viñetas con dos atributos, así que
 * quien no la conoce (el editor de antes, otro procesador al pegar) la ve como viñetas y
 * no pierde nada. La casilla la pinta editor.css; la exportación (services/docs) y la app
 * nativa (`rico`) ponen su marca ☐/☑.
 *
 * Se marca con un clic en la casilla o con Ctrl+Alt+Intro (el de Google Docs) con el
 * cursor en el elemento; Ctrl+Mayús+9 (`listCheck`) convierte la lista o el párrafo.
 */

export const LISTA = 'ul[data-checklist]';
export const ELEMENTO = 'ul[data-checklist] > li';

const aElemento = (n: Node | null | undefined): Element | null =>
  !n ? null : n.nodeType === Node.ELEMENT_NODE ? (n as Element) : n.parentElement;

/** El elemento de una lista de casillas en el que está `n`, dentro de `root`. */
export function elementoDeCasilla(root: HTMLElement, n: Node | null | undefined): Element | null {
  const li = aElemento(n)?.closest(ELEMENTO);
  return li && root.contains(li) ? li : null;
}

export const estaHecha = (li: Element): boolean => li.getAttribute('data-checked') === '1';

/**
 * Pasa `ul` a lista de casillas: todas sin hacer, salvo `li` si `hecha`. Devuelve cómo
 * dejarla como estaba (los atributos no entran en el deshacer del navegador).
 */
export function marcarComoCasillas(ul: Element, li: Element | null, hecha: boolean): () => void {
  const antes = Array.from(ul.children).map((h) => [h, h.getAttribute('data-checked')] as const);
  const teniaMarca = ul.hasAttribute('data-checklist');
  ul.setAttribute('data-checklist', '');
  for (const h of Array.from(ul.children)) {
    if (h.tagName === 'LI' && !h.hasAttribute('data-checked')) h.setAttribute('data-checked', '0');
  }
  if (li) li.setAttribute('data-checked', hecha ? '1' : '0');
  return () => {
    if (!teniaMarca) ul.removeAttribute('data-checklist');
    for (const [h, v] of antes) {
      if (v === null) h.removeAttribute('data-checked');
      else h.setAttribute('data-checked', v);
    }
  };
}

/** Quita la marca de casillas: queda una lista de viñetas. */
export function quitarCasillas(ul: Element): void {
  ul.removeAttribute('data-checklist');
  for (const h of Array.from(ul.children)) h.removeAttribute('data-checked');
}

/** Marca o desmarca un elemento. */
export function alternarCasilla(li: Element): void {
  li.setAttribute('data-checked', estaHecha(li) ? '0' : '1');
}

/**
 * Marca o desmarca los elementos de casilla que toca la selección (Ctrl+Alt+Intro).
 * Si todos estaban hechos se desmarcan; si no, se marcan todos. Devuelve si había alguno.
 */
export function alternarCasillasDeLaSeleccion(root: HTMLElement): boolean {
  const sel = document.getSelection();
  if (!sel || sel.rangeCount === 0) return false;
  const rango = sel.getRangeAt(0);
  if (!root.contains(rango.commonAncestorContainer)) return false;
  const propio = elementoDeCasilla(root, rango.startContainer);
  const tocados = Array.from(root.querySelectorAll(ELEMENTO)).filter(
    (li) => li === propio || (!rango.collapsed && rango.intersectsNode(li)),
  );
  if (tocados.length === 0) return false;
  const marcar = !tocados.every(estaHecha);
  for (const li of tocados) li.setAttribute('data-checked', marcar ? '1' : '0');
  return true;
}

/**
 * ¿El clic cae en la casilla de un elemento? La casilla va en el margen, a la izquierda
 * del texto del `<li>` (su `::before`), así que es el clic sobre el `<li>` a la izquierda
 * de su caja. Devuelve el elemento, o null.
 */
export function casillaDelClic(
  root: HTMLElement,
  objetivo: EventTarget | null,
  x: number,
): Element | null {
  const li = elementoDeCasilla(root, objetivo instanceof Node ? objetivo : null);
  if (!li || (objetivo as Element) !== li) return null;
  return x < li.getBoundingClientRect().left ? li : null;
}

/**
 * Tras un Intro en una lista de casillas, el navegador clona el elemento con sus
 * atributos: el nuevo nacería hecho si el de arriba lo estaba. El que queda vacío es el
 * nuevo (Intro al final, o al principio, que deja el vacío arriba); si no, el del cursor.
 */
export function trasNuevoElemento(root: HTMLElement): void {
  const li = elementoDeCasilla(root, document.getSelection()?.anchorNode);
  if (!li) return;
  const previo = li.previousElementSibling;
  const nuevo = previo && previo.tagName === 'LI' && previo.textContent === '' ? previo : li;
  nuevo.setAttribute('data-checked', '0');
}

/* ---------- Crear la lista ---------- */

export function liDelCursor(root: HTMLElement): Element | null {
  const n = document.getSelection()?.anchorNode;
  const li = n ? aElemento(n)?.closest('li') : null;
  return li && root.contains(li) ? li : null;
}

/**
 * Deja el elemento del cursor en una lista del tipo pedido. execCommand junta la lista
 * nueva con una vecina del mismo tipo (una viñeta escrita bajo una lista de casillas
 * caería en ella, y al revés): si pasa, se separa. Devuelve cómo deshacer lo que
 * execCommand no ve.
 */
export function dejarEnLista(
  root: HTMLElement,
  casillas: boolean,
  hecha: boolean,
): (() => void) | undefined {
  const li = liDelCursor(root);
  const ul = li?.parentElement;
  if (!li || !ul || ul.tagName !== 'UL') return undefined;
  if (ul.hasAttribute('data-checklist') === casillas) {
    if (casillas) li.setAttribute('data-checked', hecha ? '1' : '0');
    return undefined;
  }
  const sola = separarElemento(li);
  if (!casillas) {
    quitarCasillas(sola);
    return undefined;
  }
  return marcarComoCasillas(sola, li, hecha);
}

/**
 * Pasa a casillas los elementos de lista que toca la selección tras hacer la lista con
 * execCommand (varios párrafos seleccionados dan varios elementos). Si la lista es solo
 * de ellos se marca entera; si execCommand los juntó con una vecina de viñetas, salen a
 * una lista propia y la vecina sigue de viñetas. Con uno solo es dejarEnLista.
 */
export function dejarSeleccionEnCasillas(root: HTMLElement, rango: Range | null): void {
  const propio = liDelCursor(root);
  const ul = propio?.parentElement;
  if (!propio || !ul || ul.tagName !== 'UL') return;
  const elementos = Array.from(ul.children).filter(
    (h) =>
      h.tagName === 'LI' &&
      (h === propio || (!!rango && !rango.collapsed && rango.intersectsNode(h))),
  );
  if (elementos.length <= 1) {
    dejarEnLista(root, true, false);
    return;
  }
  marcarComoCasillas(separarElementos(elementos), null, false);
}

/** Saca `li` a una lista propia del mismo tipo, partiendo la suya en dos si hace falta. */
function separarElemento(li: Element): Element {
  return separarElementos([li]);
}

/**
 * Saca los elementos `lis` (seguidos, de una misma lista) a una lista propia del mismo
 * tipo, partiendo la suya en dos si hace falta.
 */
function separarElementos(lis: Element[]): Element {
  const primero = lis[0] as Element;
  const ultimo = lis[lis.length - 1] as Element;
  const lista = primero.parentElement as Element;
  if (lista.children.length === lis.length) return lista;
  const nueva = document.createElement(lista.tagName.toLowerCase());
  if (!primero.previousSibling) lista.before(nueva);
  else if (!ultimo.nextSibling) lista.after(nueva);
  else {
    // En medio: lo de detrás pasa a una tercera lista, gemela de la de arriba.
    const resto = document.createElement(lista.tagName.toLowerCase());
    for (const a of Array.from(lista.attributes)) resto.setAttribute(a.name, a.value);
    for (let n = ultimo.nextSibling; n; n = ultimo.nextSibling) resto.append(n);
    lista.after(nueva, resto);
  }
  const sel = document.getSelection();
  // Mover los nodos saca de ellos la selección (también a un Range clonado, que sigue
  // vivo): se apunta dónde estaba y se repone.
  const dentro = (n: Node | null): n is Node => n !== null && lis.some((li) => li.contains(n));
  const anclaN = sel?.anchorNode ?? null;
  const focoN = sel?.focusNode ?? null;
  const ancla = dentro(anclaN) ? ([anclaN, sel?.anchorOffset ?? 0] as const) : null;
  const foco = dentro(focoN) ? ([focoN, sel?.focusOffset ?? 0] as const) : null;
  nueva.append(...lis);
  if (sel && ancla) sel.setBaseAndExtent(ancla[0], ancla[1], ...(foco ?? ancla));
  return nueva;
}
