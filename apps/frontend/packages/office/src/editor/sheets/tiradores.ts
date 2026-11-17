/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La hoja con el dedo. En el teléfono, arrastrar sobre la rejilla la desplaza (lo pone el
 * navegador), así que el rango no se puede marcar arrastrando como con el ratón: se marca
 * con los dos tiradores circulares de las esquinas de la selección, como en las hojas de
 * Android e iOS. Aquí está la cuenta, sin DOM ni estado, para poder probarla: a qué rango
 * lleva soltar un tirador en una celda y qué celda hay bajo el dedo.
 */

/** Celda y rectángulo de la hoja, en índices 0-based (los de `SheetCanvas`). */
export interface PosTirador {
  c: number;
  r: number;
}
export interface RectTirador {
  c1: number;
  c2: number;
  r1: number;
  r2: number;
}

/** El tirador de arriba a la izquierda del rango o el de abajo a la derecha. */
export type Tirador = 'inicio' | 'fin';

/** El radio del círculo que se ve, en px de maqueta, y el de la zona donde lo coge un dedo,
 *  que es mayor: 2 × 22 son los 44 px de objetivo táctil de la suite. */
export const RADIO_TIRADOR = 7;
export const ZONA_TIRADOR = 22;

/**
 * El rango tras llevar `tirador` hasta `hasta`. La esquina CONTRARIA no se mueve y pasa a
 * ser la celda ancla, de modo que cruzar el tirador al otro lado da la vuelta al rango en
 * vez de deshacerlo. Llegar a la propia ancla deja solo esa celda (sin «B2:B2»).
 */
export function destinoTirador(
  rect: RectTirador,
  tirador: Tirador,
  hasta: PosTirador,
  fusionEn?: (c: number, r: number) => RectTirador | null,
): { sel: PosTirador; ext: PosTirador | null } {
  const sel = tirador === 'fin' ? { c: rect.c1, r: rect.r1 } : { c: rect.c2, r: rect.r2 };
  const ext = hasta.c === sel.c && hasta.r === sel.r ? null : { c: hasta.c, r: hasta.r };
  // El tirador de arriba de una combinada deja de ancla su esquina de abajo, una celda
  // CUBIERTA. Si el rango no sale de esa combinada, es la combinada sola, por su ancla: con
  // la cubierta de celda activa, lo que se escribiera iría a una celda que no se ve.
  const fu = fusionEn?.(sel.c, sel.r);
  if (fu && (fu.c1 !== sel.c || fu.r1 !== sel.r)) {
    const e = ext ?? sel;
    const dentro =
      Math.min(sel.c, e.c) >= fu.c1 &&
      Math.min(sel.r, e.r) >= fu.r1 &&
      Math.max(sel.c, e.c) <= fu.c2 &&
      Math.max(sel.r, e.r) <= fu.r2;
    if (dentro) return { sel: { c: fu.c1, r: fu.r1 }, ext: null };
  }
  return { sel, ext };
}

/**
 * El punto de la rejilla que señala el dedo que lleva un tirador. El círculo va centrado en
 * la esquina, y el dedo no lo coge por el centro: se suma lo que había entre el dedo y la
 * esquina al empezar (`agarre`) y se mete un poco hacia dentro del rango, para que la
 * esquina caiga en SU celda y no en la vecina con la que comparte borde.
 */
export function puntoDeTirador(
  tirador: Tirador,
  dedo: { x: number; y: number },
  agarre: { x: number; y: number },
  dentro = 2,
): { x: number; y: number } {
  const d = tirador === 'fin' ? -dentro : dentro;
  return { x: dedo.x + agarre.x + d, y: dedo.y + agarre.y + d };
}

/**
 * La celda de la rejilla bajo un punto, de entre lo que `elementsFromPoint` da de arriba
 * abajo: la primera con `[data-ref]`. Saltarse lo que no es celda es lo que deja ver la de
 * debajo del propio tirador (que tiene el puntero capturado) o de un gráfico flotante.
 */
export function celdaEnElementos(elementos: Iterable<Pick<Element, 'closest'>>): PosTirador | null {
  for (const el of elementos) {
    const celda = el.closest<HTMLElement>('[data-ref]');
    if (!celda) continue;
    // `Number('')` es 0: sin el dato no hay celda, no la A1.
    const c = Number(celda.dataset['c'] || NaN);
    const r = Number(celda.dataset['r'] || NaN);
    if (Number.isInteger(c) && Number.isInteger(r)) return { c, r };
  }
  return null;
}
