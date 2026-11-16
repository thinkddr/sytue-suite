/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * Moverse con las flechas por una lista o una cuadrícula de elementos enfocables: la de
 * Drive (lista y teselas) y la galería de Photos. Vive aquí para que las dos cuenten igual,
 * y tiene su espejo en Fyne (`apps/fyne/ui/navegacionrejilla.go`) con los mismos casos.
 *
 * LA CUADRÍCULA SE MIDE, NO SE CUENTA. Las columnas de Drive cambian con el ancho
 * (`grid-cols-2 … xl:grid-cols-5`) y las filas justificadas de Photos tienen un número
 * distinto de fotos cada una, partidas además por meses. En vez de suponer «n columnas»,
 * ↑ y ↓ buscan en la fila de encima o de debajo la caja cuyo centro está más cerca en
 * horizontal: es lo que hace un explorador de escritorio y lo único que sirve en las
 * filas justificadas. ← y → siguen el orden de lectura y pasan de una fila a la
 * siguiente, como Google Photos y el Finder.
 */

/** La caja de un elemento en pantalla: lo que da `getBoundingClientRect`. */
export interface CajaRejilla {
  x: number;
  y: number;
  ancho: number;
  alto: number;
}

/** Un píxel de holgura: con zoom y fracciones, dos cajas de la misma fila no casan al céntimo. */
const HOLGURA = 1;

const centroX = (c: CajaRejilla): number => c.x + c.ancho / 2;

/**
 * El índice al que lleva `tecla` desde `actual` en una lista de UNA columna, o `null` si la
 * tecla no es de moverse o ya está en el borde.
 */
export function vecinoEnLista(actual: number, total: number, tecla: string): number | null {
  let destino: number;
  switch (tecla) {
    case 'ArrowDown':
      destino = actual + 1;
      break;
    case 'ArrowUp':
      destino = actual - 1;
      break;
    case 'Home':
      destino = 0;
      break;
    case 'End':
      destino = total - 1;
      break;
    default:
      return null;
  }
  return destino >= 0 && destino < total && destino !== actual ? destino : null;
}

/**
 * El índice al que lleva `tecla` desde `actual` en una cuadrícula cuyas cajas, en orden de
 * lectura, son `cajas`; `null` si la tecla no es de moverse o no hay a dónde ir.
 */
export function vecinoEnRejilla(
  cajas: readonly CajaRejilla[],
  actual: number,
  tecla: string,
): number | null {
  const total = cajas.length;
  const yo = cajas[actual];
  if (!yo) return null;
  switch (tecla) {
    case 'ArrowRight':
      return actual + 1 < total ? actual + 1 : null;
    case 'ArrowLeft':
      return actual > 0 ? actual - 1 : null;
    case 'Home':
    case 'End':
      return vecinoEnLista(actual, total, tecla);
    case 'ArrowDown':
    case 'ArrowUp':
      break;
    default:
      return null;
  }
  const abajo = tecla === 'ArrowDown';
  // La fila más cercana en esa dirección: la de arriba del todo de las que empiezan bajo
  // mi borde inferior, o la de abajo del todo de las que acaban sobre mi borde superior.
  let filaMasCerca: number | null = null;
  for (const c of cajas) {
    if (abajo ? c.y >= yo.y + yo.alto - HOLGURA : c.y + c.alto <= yo.y + HOLGURA) {
      const borde = abajo ? c.y : c.y + c.alto;
      if (filaMasCerca === null || (abajo ? borde < filaMasCerca : borde > filaMasCerca)) {
        filaMasCerca = borde;
      }
    }
  }
  if (filaMasCerca === null) return null;
  const mio = centroX(yo);
  let mejor: number | null = null;
  let distancia = Infinity;
  for (let i = 0; i < total; i++) {
    const c = cajas[i]!;
    const borde = abajo ? c.y : c.y + c.alto;
    if (Math.abs(borde - filaMasCerca) > HOLGURA) continue;
    const d = Math.abs(centroX(c) - mio);
    // Con un empate (dos cajas a la misma distancia) gana la primera en el orden de lectura.
    if (d < distancia) {
      distancia = d;
      mejor = i;
    }
  }
  return mejor;
}

const conModificador = (e: KeyboardEvent): boolean =>
  e.altKey || e.ctrlKey || e.metaKey || e.shiftKey;

/** Enfoca el elemento y lo trae a la vista lo justo, sin centrarlo. */
export function enfocarALaVista(el: HTMLElement): void {
  el.focus({ preventScroll: true });
  el.scrollIntoView?.({ block: 'nearest', inline: 'nearest' });
}

/**
 * El `onKeyDown` de una cuadrícula: si la tecla sale de uno de `elementos` (él mismo, no un
 * mando que lleve dentro) y lleva a otro, lo enfoca y se queda la tecla. Con un modificador
 * no hace nada: Mayús + flecha, Ctrl + Inicio y compañía son de otros. Devuelve si se movió.
 */
export function moverFocoEnRejilla(e: KeyboardEvent, elementos: readonly HTMLElement[]): boolean {
  if (conModificador(e)) return false;
  const actual = elementos.indexOf(e.target as HTMLElement);
  if (actual < 0) return false;
  const cajas = elementos.map((el) => {
    const r = el.getBoundingClientRect();
    return { x: r.left, y: r.top, ancho: r.width, alto: r.height };
  });
  const destino = vecinoEnRejilla(cajas, actual, e.key);
  const el = destino === null ? undefined : elementos[destino];
  if (!el) return false;
  e.preventDefault();
  enfocarALaVista(el);
  return true;
}

/**
 * El `onKeyDown` de la fila `indice` de una lista VIRTUAL de `total` filas: lleva la fila
 * de destino a la vista con `llevarA` (el `scrollToIndex` del `VirtualListHandle`) y la
 * enfoca en cuanto el virtualizador la pinta, que puede ser uno o dos fotogramas después.
 * `buscar(i)` encuentra el elemento de la fila `i` si está en el DOM.
 */
export function moverFocoEnListaVirtual(
  e: KeyboardEvent,
  indice: number,
  total: number,
  llevarA: (i: number) => void,
  buscar: (i: number) => HTMLElement | null,
): boolean {
  if (conModificador(e) || e.target !== e.currentTarget) return false;
  const destino = vecinoEnLista(indice, total, e.key);
  if (destino === null) return false;
  e.preventDefault();
  llevarA(destino);
  let intentos = 0;
  const enfocar = (): void => {
    const el = buscar(destino);
    if (el) enfocarALaVista(el);
    else if (++intentos < 10) requestAnimationFrame(enfocar);
  };
  enfocar();
  return true;
}
