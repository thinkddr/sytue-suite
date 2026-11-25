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
import { BLOQUES_DE_NOTAS } from '@sytue/ui/browser/bloques-de-notas';
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

/*
 * COMBINAR Y SEPARAR CELDAS: `colspan` y `rowspan`, con los topes del saneador del
 * servicio (`BLOQUES_DE_NOTAS.numericos`). Lo mismo, con las mismas reglas, que el
 * editor de Fyne (`combinar.go`):
 *
 *   - con la de la derecha: la celda se queda con la siguiente de su fila si ocupan las
 *     mismas filas; el texto de la otra va detrás del suyo;
 *   - con la de abajo: igual con la que empieza justo debajo, en la misma sección, y
 *     ocupa las mismas columnas;
 *   - separar: la celda vuelve a una ranura y las que tapaba vuelven, vacías.
 */

const TOPE = BLOQUES_DE_NOTAS.numericos['td'] ?? {};

/** Un `colspan` o un `rowspan` de la celda: 1 si no lo trae o no es un número. */
function tramo(celda: Element, k: 'colspan' | 'rowspan'): number {
  const n = Number(celda.getAttribute(k));
  return Number.isInteger(n) && n >= 1 ? n : 1;
}

function ponerTramo(celda: Element, k: 'colspan' | 'rowspan', n: number): void {
  if (n > 1) celda.setAttribute(k, String(n));
  else celda.removeAttribute(k);
}

/** Dónde empieza cada celda en la rejilla: su fila y su columna, contando lo que tapan
 *  las combinadas de arriba. */
function ranuras(tabla: HTMLTableElement): Map<HTMLTableCellElement, [number, number]> {
  const filas = filasDe(tabla);
  const ocupada = new Set<string>();
  const out = new Map<HTMLTableCellElement, [number, number]>();
  filas.forEach((fila, f) => {
    let col = 0;
    for (const c of celdasDe(fila)) {
      while (ocupada.has(`${f},${col}`)) col++;
      out.set(c, [f, col]);
      const alto = Math.min(tramo(c, 'rowspan'), filas.length - f);
      for (let df = 0; df < alto; df++)
        for (let dc = 0; dc < tramo(c, 'colspan'); dc++) ocupada.add(`${f + df},${col + dc}`);
      col += tramo(c, 'colspan');
    }
  });
  return out;
}

const vacia = (c: Element): boolean =>
  (c.textContent ?? '').replace(/ /g, '').trim() === '' && !c.querySelector('img');

/** Pasa lo de `de` detrás de lo de `a` (si `a` está vacía, en su lugar). */
function juntar(a: HTMLTableCellElement, de: HTMLTableCellElement): void {
  if (vacia(de)) return;
  if (vacia(a)) a.replaceChildren();
  a.append(...de.childNodes);
}

/** La celda con la que se combinaría `celda` a la derecha, o null. */
export function vecinaDerecha(celda: HTMLTableCellElement): HTMLTableCellElement | null {
  const tabla = celda.closest('table');
  const fila = celda.parentElement;
  if (!tabla || !fila) return null;
  const otra = celdasDe(fila)[celdasDe(fila).indexOf(celda) + 1];
  if (!otra || tramo(otra, 'rowspan') !== tramo(celda, 'rowspan')) return null;
  if (tramo(celda, 'colspan') + tramo(otra, 'colspan') > (TOPE['colspan'] ?? 1)) return null;
  const r = ranuras(tabla);
  return r.get(otra)?.[1] === (r.get(celda)?.[1] ?? -1) + tramo(celda, 'colspan') ? otra : null;
}

/** La celda con la que se combinaría `celda` por abajo, o null. */
export function vecinaAbajo(celda: HTMLTableCellElement): HTMLTableCellElement | null {
  const tabla = celda.closest('table');
  if (!tabla) return null;
  const r = ranuras(tabla);
  const [f, col] = r.get(celda) ?? [-1, -1];
  const fila = filasDe(tabla)[f + tramo(celda, 'rowspan')];
  if (!fila || fila.parentElement !== celda.parentElement?.parentElement) return null;
  const otra = celdasDe(fila).find((c) => r.get(c)?.[1] === col);
  if (!otra || tramo(otra, 'colspan') !== tramo(celda, 'colspan')) return null;
  return tramo(celda, 'rowspan') + tramo(otra, 'rowspan') > (TOPE['rowspan'] ?? 1) ? null : otra;
}

/** Si la celda ocupa más de una ranura. */
export function esCombinada(celda: HTMLTableCellElement): boolean {
  return tramo(celda, 'colspan') > 1 || tramo(celda, 'rowspan') > 1;
}

export function combinarDerecha(celda: HTMLTableCellElement): void {
  const otra = vecinaDerecha(celda);
  if (!otra) return;
  juntar(celda, otra);
  ponerTramo(celda, 'colspan', tramo(celda, 'colspan') + tramo(otra, 'colspan'));
  otra.remove();
}

export function combinarAbajo(celda: HTMLTableCellElement): void {
  const otra = vecinaAbajo(celda);
  if (!otra) return;
  juntar(celda, otra);
  ponerTramo(celda, 'rowspan', tramo(celda, 'rowspan') + tramo(otra, 'rowspan'));
  otra.remove();
}

/** Separa una celda combinada: vuelve a una ranura y las que tapaba vuelven vacías, del
 *  mismo tipo que ella (`th` con su `scope`, si lo era). */
export function separarCelda(celda: HTMLTableCellElement): void {
  const tabla = celda.closest('table');
  if (!tabla || !esCombinada(celda)) return;
  const r = ranuras(tabla);
  const filas = filasDe(tabla);
  const [f, col] = r.get(celda) ?? [0, 0];
  const ancho = tramo(celda, 'colspan');
  const alto = Math.min(tramo(celda, 'rowspan'), filas.length - f);
  ponerTramo(celda, 'colspan', 1);
  ponerTramo(celda, 'rowspan', 1);
  const nueva = () => {
    const c = celda.ownerDocument.createElement(celda.tagName.toLowerCase() === 'th' ? 'th' : 'td');
    const scope = celda.getAttribute('scope');
    if (scope) c.setAttribute('scope', scope);
    c.innerHTML = '<br>';
    return c;
  };
  for (let g = f; g < f + alto; g++) {
    const fila = filas[g];
    if (!fila) continue;
    const n = g === f ? ancho - 1 : ancho;
    // En su fila, detrás de ella; en las de debajo, delante de la primera que empiece
    // más a la derecha.
    const antes =
      g === f
        ? celda.nextElementSibling
        : (celdasDe(fila).find((c) => (r.get(c)?.[1] ?? -1) > col) ?? null);
    for (let k = 0; k < n; k++) fila.insertBefore(nueva(), antes);
  }
}
