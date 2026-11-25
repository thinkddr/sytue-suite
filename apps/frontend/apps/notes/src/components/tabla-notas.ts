/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que la tabla de una nota hace además de lo común (`@sytue/ui/browser/tabla`, que
 * comparte con el redactor de Mail): cabecera de fila y de columna, filas numeradas,
 * fondo de celda, alineación de columna y moverse de celda en celda con Tab.
 *
 * EL HTML ES EL COMPARTIDO CON FYNE: la fila de cabecera es un `<thead>` de `<th>`, la
 * columna de cabecera son `<th scope="row">`, las filas numeradas
 * `data-numerada` en la tabla y el fondo `data-fondo` con el nombre de un token
 * (`gris|azul|…`), nunca un color crudo. Todo pasa el saneador del servicio.
 */
import type { ColorDeBloque } from './bloques.js';

// Sin las colecciones de tabla (`rows`, `tBodies`, `cells`): consultas del DOM, que
// valen igual en el navegador y en el DOM de las pruebas.
const hijos = <E extends Element>(el: Element, etiquetas: string): E[] =>
  [...el.children].filter((h): h is E => etiquetas.split(',').includes(h.tagName.toLowerCase()));
const celdasDe = (fila: Element) => hijos<HTMLTableCellElement>(fila, 'td,th');
const filasDeSeccion = (s: Element) => hijos<HTMLTableRowElement>(s, 'tr');
const cabezaDe = (tabla: HTMLTableElement) => hijos<HTMLElement>(tabla, 'thead')[0] ?? null;

/** Las filas de la tabla, las de la cabecera incluidas, en orden. */
function filasDe(tabla: HTMLTableElement): HTMLTableRowElement[] {
  return [...tabla.querySelectorAll<HTMLTableRowElement>('tr')].filter(
    (f) => f.closest('table') === tabla,
  );
}

/** La posición de la celda en su fila. */
function columnaDe(celda: HTMLTableCellElement): number {
  return celda.parentElement ? celdasDe(celda.parentElement).indexOf(celda) : -1;
}

/** Cambia una celda de etiqueta (`td` ⇄ `th`) conservando hijos y atributos. */
function cambiarCelda(celda: HTMLTableCellElement, etiqueta: 'td' | 'th'): HTMLTableCellElement {
  if (celda.tagName.toLowerCase() === etiqueta) return celda;
  const nueva = celda.ownerDocument.createElement(etiqueta);
  for (const a of celda.attributes) nueva.setAttribute(a.name, a.value);
  while (celda.firstChild) nueva.append(celda.firstChild);
  celda.replaceWith(nueva);
  return nueva;
}

/** El cuerpo de la tabla, creándolo si las filas cuelgan directamente de ella. */
function cuerpoDe(tabla: HTMLTableElement): HTMLElement {
  let cuerpo = hijos<HTMLElement>(tabla, 'tbody')[0] ?? null;
  if (!cuerpo) {
    cuerpo = tabla.ownerDocument.createElement('tbody');
    for (const f of filasDeSeccion(tabla)) cuerpo.append(f);
    tabla.append(cuerpo);
  }
  return cuerpo;
}

export function tieneCabeceraDeFila(tabla: HTMLTableElement): boolean {
  const cabeza = cabezaDe(tabla);
  return !!cabeza && filasDeSeccion(cabeza).length > 0;
}

/** Pone o quita la fila de cabecera: la primera fila pasa a un `<thead>` de `<th>`. */
export function alternarCabeceraDeFila(tabla: HTMLTableElement): void {
  const cuerpo = cuerpoDe(tabla);
  const cabeza = cabezaDe(tabla);
  if (cabeza) {
    for (const fila of filasDeSeccion(cabeza).reverse()) {
      for (const c of celdasDe(fila)) cambiarCelda(c, 'td').removeAttribute('scope');
      cuerpo.prepend(fila);
    }
    cabeza.remove();
    return;
  }
  const primera = filasDeSeccion(cuerpo)[0];
  if (!primera) return;
  const nueva = tabla.ownerDocument.createElement('thead');
  tabla.insertBefore(nueva, cuerpo);
  nueva.append(primera);
  for (const c of celdasDe(primera)) cambiarCelda(c, 'th').removeAttribute('scope');
}

export function tieneCabeceraDeColumna(tabla: HTMLTableElement): boolean {
  const filas = filasDeSeccion(cuerpoDe(tabla));
  return (
    filas.length > 0 &&
    filas.every((f) => {
      const c = celdasDe(f)[0];
      return c?.tagName === 'TH' && c.getAttribute('scope') === 'row';
    })
  );
}

/** Pone o quita la columna de cabecera: la primera celda de cada fila del cuerpo, `<th scope="row">`. */
export function alternarCabeceraDeColumna(tabla: HTMLTableElement): void {
  const quitar = tieneCabeceraDeColumna(tabla);
  for (const fila of filasDeSeccion(cuerpoDe(tabla))) {
    const c = celdasDe(fila)[0];
    if (!c) continue;
    if (quitar) cambiarCelda(c, 'td').removeAttribute('scope');
    else cambiarCelda(c, 'th').setAttribute('scope', 'row');
  }
}

export function alternarFilasNumeradas(tabla: HTMLTableElement): void {
  tabla.toggleAttribute('data-numerada');
  // El atributo va sin valor, como lo escribe el saneador.
  if (tabla.hasAttribute('data-numerada')) tabla.setAttribute('data-numerada', '');
}

/** El fondo de una celda con el nombre de un token, o ninguno. */
export function fondoDeCelda(celda: HTMLTableCellElement, color: ColorDeBloque | null): void {
  if (color) celda.setAttribute('data-fondo', color);
  else celda.removeAttribute('data-fondo');
}

/** Alinea la columna de la celda: `text-align` en cada celda, que el saneador conserva. */
export function alinearColumna(
  celda: HTMLTableCellElement,
  lado: 'left' | 'center' | 'right',
): void {
  const tabla = celda.closest('table');
  if (!tabla) return;
  const col = columnaDe(celda);
  for (const fila of filasDe(tabla)) {
    const c = celdasDe(fila)[col];
    if (!c) continue;
    if (lado === 'left') c.style.removeProperty('text-align');
    else c.style.setProperty('text-align', lado);
    if (!c.getAttribute('style')) c.removeAttribute('style');
  }
}

/** La alineación de la columna de la celda (la de la propia celda). */
export function alineacionDe(celda: HTMLTableCellElement): 'left' | 'center' | 'right' {
  const a = celda.style.textAlign;
  return a === 'center' || a === 'right' ? a : 'left';
}

/**
 * La celda de al lado al pulsar Tab (`paso` 1) o Mayús+Tab (−1), en orden de lectura.
 * `null` en los extremos: después de la última celda, Tab añade una fila (quien llama).
 */
export function celdaVecina(
  celda: HTMLTableCellElement,
  paso: 1 | -1,
): HTMLTableCellElement | null {
  const tabla = celda.closest('table');
  if (!tabla) return null;
  const todas = filasDe(tabla).flatMap(celdasDe);
  return todas[todas.indexOf(celda) + paso] ?? null;
}

/** Pone el cursor en `celda`, con su contenido seleccionado (para escribir encima). */
export function entrarEnCelda(celda: HTMLTableCellElement): void {
  const r = celda.ownerDocument.createRange();
  r.selectNodeContents(celda);
  const vacia = (celda.textContent ?? '').replace(/ /g, '').trim() === '';
  if (vacia) r.collapse(true);
  const sel = celda.ownerDocument.getSelection();
  sel?.removeAllRanges();
  sel?.addRange(r);
}

/** Dónde está la celda en la rejilla: fila y columna (cabecera incluida). */
export function posicionDe(celda: HTMLTableCellElement): [number, number] {
  const tabla = celda.closest('table');
  const fila = celda.parentElement;
  if (!tabla || !fila) return [-1, -1];
  return [filasDe(tabla).indexOf(fila as HTMLTableRowElement), celdasDe(fila).indexOf(celda)];
}

/** La celda en esa fila y columna, o la más cercana que quede (tras quitar una fila). */
export function celdaEn(
  tabla: HTMLTableElement,
  [fila, col]: [number, number],
): HTMLTableCellElement | null {
  const filas = filasDe(tabla);
  const f = filas[Math.min(Math.max(fila, 0), filas.length - 1)];
  if (!f) return null;
  const celdas = celdasDe(f);
  return celdas[Math.min(Math.max(col, 0), celdas.length - 1)] ?? null;
}
