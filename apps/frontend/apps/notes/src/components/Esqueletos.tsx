/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Esqueletos de Notes: lo que se ve mientras llegan las notas de un cuaderno, con la
 * GEOMETRÍA de lo que va a llegar. Una rueda centrada hacía que todo saltase al
 * llegar los datos —la columna de lectura aparecía de golpe donde antes había un
 * hueco— y no decía qué se estaba cargando. Es lo que ya hacen la rejilla y el
 * lateral de Calendar (`GridSkeleton`).
 *
 * Tres formas, una por manera de leer un cuaderno:
 *   - `filas`: la lista y el diario, líneas con su marca en la columna de lectura.
 *   - `tarjetas`: columnas y lienzo libre, tarjetas repartidas en las tres columnas.
 *   - `hoja`: la nota que llena el panel en la vista de árbol, título y párrafo.
 *
 * Los bloques son decorativos (`aria-hidden` en `Skeleton`); el contenedor lleva
 * `aria-busy` para que el lector de pantalla sepa que la zona está cargando. El pulso
 * lo apaga `prefers-reduced-motion` en la hoja de estilos de `@sytue/ui`.
 */
import type { JSX } from 'preact';
import { Skeleton } from '@sytue/ui';

export type FormaEsqueleto = 'filas' | 'tarjetas' | 'hoja';

// Anchos distintos por fila: con todas iguales parece una tabla vacía, no texto.
const ANCHOS_FILA = ['62%', '46%', '74%', '38%', '56%', '68%'];
// Altos de tarjeta por columna: las notas de verdad no miden todas lo mismo.
const ALTOS_TARJETA = [['7rem', '4.5rem'], ['5rem', '8rem'], ['6rem']];
const ANCHOS_PARRAFO = ['100%', '94%', '97%', '88%', '62%'];

export function EsqueletoCuaderno({ forma }: { forma: FormaEsqueleto }): JSX.Element {
  if (forma === 'filas') {
    // La misma columna y el mismo sangrado que `notes-stream`: la marca cae en el
    // borde de la columna y el texto empieza donde empezará el título.
    return (
      <div class="notes-col" aria-busy="true" data-esqueleto="filas">
        <div class="notes-stream -mx-2">
          {ANCHOS_FILA.map((ancho, i) => (
            <div key={i} class="flex h-8 items-center gap-2.5 px-2">
              <Skeleton width="1.25rem" height="0.375rem" class="shrink-0" />
              <Skeleton width={ancho} height="0.875rem" />
            </div>
          ))}
        </div>
      </div>
    );
  }
  if (forma === 'hoja') {
    // `notes-sheet`: la nota a todo el panel, sin tarjeta alrededor.
    return (
      <div class="notes-sheet grid content-start gap-6" aria-busy="true" data-esqueleto="hoja">
        <Skeleton width="45%" height="1.75rem" />
        <div class="grid gap-3">
          {ANCHOS_PARRAFO.map((ancho, i) => (
            <Skeleton key={i} width={ancho} />
          ))}
        </div>
      </div>
    );
  }
  // La rejilla de columnas tal cual: en el teléfono es una, y en escritorio, tres.
  return (
    <div
      class="grid grid-cols-1 items-start gap-3 p-4 md:grid-cols-2 lg:grid-cols-3"
      aria-busy="true"
      data-esqueleto="tarjetas"
    >
      {ALTOS_TARJETA.map((altos, i) => (
        <div key={i} class="grid content-start gap-2.5">
          {altos.map((alto, j) => (
            <Skeleton key={j} height={alto} class="rounded-lg!" />
          ))}
        </div>
      ))}
    </div>
  );
}

/**
 * Las filas del panel de notas de la vista de árbol: líneas a la altura de una rama,
 * con alguna sangrada para que se lea como árbol y no como lista.
 */
export function EsqueletoRamas(): JSX.Element {
  const ramas: Array<[nivel: number, ancho: string]> = [
    [0, '70%'],
    [1, '55%'],
    [1, '62%'],
    [0, '48%'],
    [0, '66%'],
  ];
  return (
    <div class="grid gap-0.5 p-2" aria-busy="true" data-esqueleto="ramas">
      {ramas.map(([nivel, ancho], i) => (
        <div
          key={i}
          class="flex h-8 items-center px-2"
          style={{ paddingLeft: `${0.5 + nivel * 1.25}rem` }}
        >
          <Skeleton width={ancho} height="0.875rem" />
        </div>
      ))}
    </div>
  );
}

/**
 * Los cuadernos del menú mientras se cargan: filas con la altura de una fila del
 * árbol, tintadas para el cromo oscuro (un `bg-subtle` ahí sería un destello claro),
 * como el lateral de Calendar.
 */
export function EsqueletoCuadernos(): JSX.Element {
  return (
    <div class="grid gap-0.5" aria-busy="true" data-esqueleto="cuadernos">
      {[0, 1, 2].map((i) => (
        <Skeleton
          key={i}
          height="2rem"
          class="rounded-nav!"
          style={{ backgroundColor: 'var(--sytue-sidebar-hover-bg)' }}
        />
      ))}
    </div>
  );
}
