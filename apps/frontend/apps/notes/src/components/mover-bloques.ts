/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los bloques de primer nivel de una nota (párrafo, título, lista, tabla, panel…), como
 * los mueve el asa ⋮⋮ de Confluence: subir, bajar, duplicar, quitar y arrastrar a otro
 * sitio. Trabaja sobre el DOM del campo; quien llama entrega la nota después.
 */

/** El bloque de primer nivel (hijo directo del campo) que contiene a `nodo`. */
export function bloqueDe(nodo: Node | null, raiz: HTMLElement): HTMLElement | null {
  let n: Node | null = nodo;
  while (n && n.parentNode !== raiz) n = n.parentNode;
  return n instanceof HTMLElement ? n : null;
}

/** Sube (−1) o baja (1) el bloque un puesto. `false` si ya está en el extremo. */
export function moverBloque(bloque: HTMLElement, paso: -1 | 1): boolean {
  const vecino = paso < 0 ? bloque.previousElementSibling : bloque.nextElementSibling;
  if (!vecino) return false;
  if (paso < 0) vecino.before(bloque);
  else vecino.after(bloque);
  return true;
}

/** Pone una copia justo debajo y la devuelve. */
export function duplicarBloque(bloque: HTMLElement): HTMLElement {
  const copia = bloque.cloneNode(true) as HTMLElement;
  bloque.after(copia);
  return copia;
}

/** Quita el bloque; si era el último, deja un párrafo vacío donde seguir escribiendo. */
export function quitarBloque(bloque: HTMLElement, raiz: HTMLElement): void {
  bloque.remove();
  if (!raiz.firstElementChild) raiz.innerHTML = '<p><br></p>';
}

/**
 * Lleva `bloque` junto a `destino`: delante si `antes`, detrás si no. Es el soltar de un
 * arrastre; soltarlo sobre sí mismo no hace nada.
 */
export function llevarBloque(bloque: HTMLElement, destino: HTMLElement, antes: boolean): boolean {
  if (bloque === destino || bloque.contains(destino)) return false;
  if (antes) destino.before(bloque);
  else destino.after(bloque);
  return true;
}

/** Envuelve el bloque en un panel (`<div data-panel>`), el bloque compartido con Fyne. */
export function enPanel(bloque: HTMLElement, tipo = 'info'): HTMLElement {
  const panel = bloque.ownerDocument.createElement('div');
  panel.setAttribute('data-panel', tipo);
  bloque.replaceWith(panel);
  panel.append(bloque);
  return panel;
}
