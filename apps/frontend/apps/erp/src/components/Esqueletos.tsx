/* SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos */
/**
 * Lo que el ERP pinta mientras llega lo suyo: la FORMA de la lista, de los indicadores
 * de la portada y de la ficha, no una rueda en medio de un hueco. Con la rueda la tabla
 * saltaba al cargar —de un bloque de 150 px a seis filas—, y el resto de la suite ya
 * pinta esqueletos (§3.18 de la guía de diseño).
 *
 * Las medidas las copia Fyne (`apps/fyne/apps/erp/esqueletos.go`) y un guardián de allí
 * las compara con estas: si una cambia aquí, cámbiala allí.
 */
import type { JSX } from 'preact';
import { Card, Skeleton, cx } from '@sytue/ui';

/** Las filas de la lista mientras carga: las de una página corta, sin desbordar. */
export const FILAS_ESQUELETO_LISTA = 6;

/**
 * Los anchos de cada barra, por columna y desplazados en cada fila: todas iguales
 * parecían una rejilla de ladrillos, y con esto se leen como texto de largo variable.
 */
export const ANCHOS_ESQUELETO: readonly string[] = ['70%', '45%', '85%', '55%'];

/** El ancho de la barra `i`, dando la vuelta a la lista. */
export function anchoEsqueleto(i: number): string {
  return ANCHOS_ESQUELETO[i % ANCHOS_ESQUELETO.length] ?? '100%';
}

/** En el teléfono la fila de resumen enseña dos datos, no la tabla entera. */
export const COLUMNAS_ESQUELETO_MOVIL = 2;

/**
 * La tabla que llega: una caja como la de la tabla y seis filas con una barra por cada
 * columna de `modelo.lista`. Fila: `py-3` + barra de 1rem = 2.5rem; entre filas, el
 * filete de `divide-y`.
 */
export function EsqueletoLista({ columnas }: { columnas: number }): JSX.Element {
  const n = Math.max(columnas, 1);
  return (
    <div
      data-esqueleto="lista"
      aria-hidden="true"
      class="border-border bg-surface rounded-card divide-border-subtle divide-y overflow-hidden border shadow-xs"
    >
      {Array.from({ length: FILAS_ESQUELETO_LISTA }, (_, f) => (
        <div key={f} class="flex items-center gap-4 px-3 py-3">
          {Array.from({ length: n }, (_, c) => (
            <div
              key={c}
              class={cx('min-w-0 flex-1', c >= COLUMNAS_ESQUELETO_MOVIL && 'max-md:hidden')}
            >
              <Skeleton width={anchoEsqueleto(f + c)} />
            </div>
          ))}
        </div>
      ))}
    </div>
  );
}

/**
 * Una tarjeta del resumen sin datos: la misma caja que `Card p-4` y sus tres líneas a
 * su altura —la fila de la insignia (`h-5`), la cifra `text-xl` (`h-7.5`) y la pista
 * `text-xs` (`h-4`)—, así que nada se mueve cuando llegan.
 */
export function EsqueletoKPI(): JSX.Element {
  return (
    <Card class="p-4" aria-hidden="true">
      <div class="mb-1 flex h-5 items-center">
        <Skeleton width="40%" height="0.75rem" />
      </div>
      <div class="flex h-7.5 items-center">
        <Skeleton width="60%" height="1.25rem" />
      </div>
      <div class="mt-1 flex h-4 items-center">
        <Skeleton width="80%" height="0.75rem" />
      </div>
    </Card>
  );
}

/**
 * La cifra de un indicador mientras llega el resumen: la caja de línea de `text-xl`
 * (`h-7.5`) con una barra dentro, en vez de un «0 €» que parece un dato.
 */
export function CifraCargando(): JSX.Element {
  return (
    <div data-esqueleto="cifra" aria-hidden="true" class="flex h-7.5 items-center">
      <Skeleton width="50%" height="1.25rem" />
    </div>
  );
}

/** Los tres indicadores de la portada, en su rejilla de siempre. */
export function EsqueletoKPIs(): JSX.Element {
  return (
    <div data-esqueleto="kpis" class="grid grid-cols-1 gap-3 sm:grid-cols-3">
      <EsqueletoKPI />
      <EsqueletoKPI />
      <EsqueletoKPI />
    </div>
  );
}

/** Los bloques de campos de la ficha: un rótulo corto y su caja de control. */
export const CAMPOS_ESQUELETO_FICHA = 5;

export function EsqueletoFicha(): JSX.Element {
  return (
    <div data-esqueleto="ficha" aria-hidden="true" class="flex flex-col gap-4">
      <div class="flex flex-col gap-1">
        <Skeleton width="55%" height="1.5rem" />
        <Skeleton width="25%" height="0.75rem" />
      </div>
      {Array.from({ length: CAMPOS_ESQUELETO_FICHA }, (_, i) => (
        <div key={i} class="flex flex-col gap-1.5">
          <Skeleton width="35%" height="0.75rem" />
          <Skeleton height="2.5rem" />
        </div>
      ))}
    </div>
  );
}
