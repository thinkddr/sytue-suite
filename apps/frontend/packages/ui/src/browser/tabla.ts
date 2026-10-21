/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Editar una tabla ya insertada en el cuerpo de un correo.
 *
 * `execCommand` sabe INSERTAR una tabla y nada más: no tiene comandos para
 * añadir una fila o quitar una columna. Por eso, hasta ahora, la tabla del
 * compositor era de usar y tirar — si te quedabas corto de filas, a borrarla y
 * empezar otra vez. Esto la hace editable, que es lo que hace cualquier cliente
 * de correo desde hace veinte años.
 *
 * Se trabaja sobre el DOM y no sobre el HTML como cadena a propósito: reescribir
 * la tabla entera perdería el cursor y cualquier formato que llevaran las celdas
 * dentro, y el usuario vería su texto saltar de sitio en cada clic.
 *
 * Los estilos van EN LÍNEA, como los de la tabla que se inserta: en un correo no
 * hay hoja de estilos que valga —ni Gmail ni Outlook la aplican—, así que una
 * celda nueva con clases llegaría al destinatario sin un solo borde.
 */

/** Borde y relleno de una celda, iguales a los de la tabla recién insertada. */
export const ESTILO_CELDA = 'border: 1px solid #d4d0c8; padding: 6px 8px';

/** Dónde va lo que se inserta respecto a la celda donde está el cursor. */
export type Lado = 'antes' | 'despues';

/**
 * La celda que contiene a `nodo`, o null si no está dentro de una tabla.
 *
 * Sube por los padres porque la selección casi nunca cae en la celda: cae en el
 * nodo de texto de dentro, o en un `<b>`, o en un `<p>` que alguien pegó.
 */
export function celdaDe(nodo: Node | null): HTMLTableCellElement | null {
  let n: Node | null = nodo;
  while (n) {
    if (n instanceof HTMLTableCellElement) return n;
    n = n.parentNode;
  }
  return null;
}

/** La tabla a la que pertenece la celda. */
function tablaDe(celda: HTMLTableCellElement): HTMLTableElement | null {
  return celda.closest('table');
}

/** Índice de la celda dentro de su fila. */
function columnaDe(celda: HTMLTableCellElement): number {
  const fila = celda.parentElement;
  return fila ? [...fila.children].indexOf(celda) : -1;
}

/** Todas las filas de la tabla, estén o no dentro de un `<tbody>`. */
function filasDe(tabla: HTMLTableElement): HTMLTableRowElement[] {
  return [...tabla.querySelectorAll('tr')];
}

/** Crea una celda vacía con el mismo aspecto que las demás. */
function nuevaCelda(doc: Document): HTMLTableCellElement {
  const td = doc.createElement('td');
  td.setAttribute('style', ESTILO_CELDA);
  // Sin esto la celda vacía se colapsa a nada y la fila nueva parece no existir.
  td.innerHTML = '&nbsp;';
  return td;
}

/** Añade una fila encima o debajo de la celda del cursor. */
export function insertarFila(celda: HTMLTableCellElement, lado: Lado): void {
  const fila = celda.parentElement as HTMLTableRowElement | null;
  if (!fila) return;
  const nueva = fila.ownerDocument.createElement('tr');
  for (let i = 0; i < fila.children.length; i++) nueva.append(nuevaCelda(fila.ownerDocument));
  fila.parentNode?.insertBefore(nueva, lado === 'antes' ? fila : fila.nextSibling);
}

/** Añade una columna a un lado de la celda del cursor, en TODAS las filas. */
export function insertarColumna(celda: HTMLTableCellElement, lado: Lado): void {
  const tabla = tablaDe(celda);
  const col = columnaDe(celda);
  if (!tabla || col < 0) return;
  const filas = filasDe(tabla);
  const anchura = Math.max(...filas.map((f) => f.children.length));
  for (const fila of filas) {
    // Nivelar las filas cortas ANTES de insertar, no después. Una tabla venida de
    // un pegado puede traerlas desiguales, y entonces la columna nueva caería en
    // una posición distinta en cada fila: la tabla quedaría dentada para siempre.
    while (fila.children.length < anchura) fila.append(nuevaCelda(fila.ownerDocument));
    const ref = fila.children[col] ?? null;
    const nueva = nuevaCelda(fila.ownerDocument);
    if (!ref) fila.append(nueva);
    else fila.insertBefore(nueva, lado === 'antes' ? ref : ref.nextSibling);
  }
}

/**
 * Quita la fila del cursor. Si era la última, quita la tabla entera.
 *
 * Dejar una `<table>` sin filas sería dejar un elemento invisible que el cursor
 * no puede alcanzar: la tabla parecería borrada y seguiría viajando en el correo.
 */
export function eliminarFila(celda: HTMLTableCellElement): void {
  const tabla = tablaDe(celda);
  const fila = celda.parentElement;
  if (!tabla || !fila) return;
  if (filasDe(tabla).length <= 1) {
    tabla.remove();
    return;
  }
  fila.remove();
}

/** Quita la columna del cursor de todas las filas; la última se lleva la tabla. */
export function eliminarColumna(celda: HTMLTableCellElement): void {
  const tabla = tablaDe(celda);
  const col = columnaDe(celda);
  if (!tabla || col < 0) return;
  const filas = filasDe(tabla);
  const anchura = Math.max(...filas.map((f) => f.children.length));
  if (anchura <= 1) {
    tabla.remove();
    return;
  }
  for (const fila of filas) fila.children[col]?.remove();
}

/** Quita la tabla entera. */
export function eliminarTabla(celda: HTMLTableCellElement): void {
  tablaDe(celda)?.remove();
}
