/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA BARRA DEL EDITOR CABE EN UNA FILA: lo que no cabe va al menú «Más».
 *
 * Antes se envolvía en dos o tres filas en el escritorio y se desplazaba de lado en el
 * teléfono, donde los controles del final no se veían y nada decía que estaban ahí. Una
 * fila de altura fija tampoco mueve la nota al cambiar de una a otra.
 *
 * NO SE CORTA POR EL FINAL, SE CORTA POR LO MENOS USADO. Cada pieza lleva su prioridad
 * y se esconden primero las de prioridad más baja; las que quedan conservan su orden y
 * sus grupos. Cortar por el final dejaba las listas en «Más» en el teléfono y el
 * asistente a la vista.
 *
 * Aquí solo vive la cuenta; las medidas las toma RichText con lo que el navegador
 * pinta (y, sin maquetación, con anchos de reserva).
 */

/** Una pieza de la barra tal como cuenta para caber. */
export interface PiezaDeBarra {
  /** Ancho pintado, en px. */
  ancho: number;
  /** El grupo: entre dos piezas de grupos distintos va un separador. */
  grupo: string;
  /** Más alta, más tarde se esconde. */
  prioridad: number;
}

export interface MedidasDeBarra {
  /** Lo que mide por dentro la fila, en px. Con 0 no se sabe y no se esconde nada. */
  disponible: number;
  /** El botón «Más». */
  anchoMas: number;
  /** El `gap` entre dos piezas. */
  hueco: number;
  /** Lo que añade un separador, su margen incluido (el hueco de al lado va aparte). */
  separador: number;
}

/** Lo que ocupa la fila con las piezas `visibles`, separadores incluidos. */
export function anchoDeFila(
  piezas: readonly PiezaDeBarra[],
  visibles: readonly boolean[],
  m: Pick<MedidasDeBarra, 'hueco' | 'separador'>,
): number {
  let total = 0;
  let previa: PiezaDeBarra | null = null;
  piezas.forEach((p, i) => {
    if (!visibles[i]) return;
    if (previa) {
      total += m.hueco;
      if (previa.grupo !== p.grupo) total += m.separador + m.hueco;
    }
    total += p.ancho;
    previa = p;
  });
  return total;
}

/**
 * Qué piezas se quedan en la fila (`true`) y cuáles van a «Más» (`false`). Si caben
 * todas, todas, y no hace falta «Más». Si no, se esconden por prioridad —la más baja
 * primero y, a igual prioridad, la que va más a la derecha— hasta que lo que queda y el
 * botón «Más» caben.
 */
export function repartirBarra(piezas: readonly PiezaDeBarra[], m: MedidasDeBarra): boolean[] {
  const visibles = piezas.map(() => true);
  if (m.disponible <= 0 || anchoDeFila(piezas, visibles, m) <= m.disponible) return visibles;
  const orden = piezas
    .map((p, i) => ({ p, i }))
    .sort((a, b) => a.p.prioridad - b.p.prioridad || b.i - a.i)
    .map(({ i }) => i);
  const conMas = () => {
    const fila = anchoDeFila(piezas, visibles, m);
    return (fila > 0 ? fila + m.hueco : 0) + m.anchoMas;
  };
  for (const i of orden) {
    if (conMas() <= m.disponible) break;
    visibles[i] = false;
  }
  return visibles;
}
