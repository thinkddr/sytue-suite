/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LOS ESQUELETOS DE FOTOS: lo que se ve mientras llega la primera página de una
 * pantalla, con la geometría de lo que va a llegar (design §3.18).
 *
 * Antes era un `Spinner` suelto arriba a la izquierda: la pantalla estaba en blanco,
 * y al llegar las fotos todo lo de debajo bajaba de golpe. Un esqueleto con LA MISMA
 * MAQUETA —las filas justificadas de `.filas`, con su alto de fila, o la rejilla de
 * tarjetas 4:3 de los álbumes— reserva el sitio y la página no salta.
 *
 * ACCESIBLE UNA SOLA VEZ. Las teselas son `aria-hidden` (`Skeleton` ya lo es); el
 * contenedor lleva `aria-busy` y dice «Cargando» con un único `role="status"`: veinte
 * teselas no son veinte avisos. Con `prefers-reduced-motion` el pulso de
 * `.sytue-skeleton` queda a cero (theme.css) y el esqueleto se queda quieto.
 *
 * La rueda se queda para lo que sí es «un poco más»: la página siguiente de la
 * galería, al pie, cuando ya hay fotos encima.
 */
import type { ComponentChildren, JSX } from 'preact';
import { Skeleton, cx } from '@sytue/ui';
import { useT } from '@sytue/i18n/preact';

/**
 * Proporciones de una biblioteca corriente, en el orden en que suelen salir: muchas
 * 4:3 y 3:2 apaisadas, algún retrato de móvil (3:4, 9:16), un cuadrado y una 16:9.
 * Variadas a propósito: con todas iguales el esqueleto era una cuadrícula, y al
 * llegar las fotos las filas se recomponían de otra forma.
 */
export const PROPORCIONES_ESQUELETO: readonly number[] = [
  1.3333, 0.75, 1.5, 1, 1.7778, 0.5625, 1.3333, 1.5, 0.75, 1.3333, 1, 1.5, 0.75, 1.7778, 1.3333,
  0.6667, 1.5, 1.3333,
];

/** El envoltorio común: ocupado, con un solo aviso para el lector de pantalla. */
function Ocupado({
  class: cls,
  children,
}: {
  class?: string;
  children: ComponentChildren;
}): JSX.Element {
  const t = useT();
  return (
    <div aria-busy="true" class={cx('flex flex-col', cls)} data-esqueleto="">
      <span role="status" class="sr-only">
        {t('ui.loading')}
      </span>
      {children}
    </div>
  );
}

/**
 * El radio de la tesela lo pone quien la envuelve (`rounded-sm`, el de la foto) y el
 * bloque gris lo hereda: `Skeleton` trae `rounded-md` y, sin fusión de clases, cuál
 * de los dos ganaba dependía del orden de la hoja de estilos.
 */
const HEREDA_RADIO = { borderRadius: 'inherit' };

/** Teselas grises en `.filas`: el alto de fila es el de la galería real. */
function FilasEsqueleto({ cuantas }: { cuantas: number }): JSX.Element {
  return (
    <ul class="filas" aria-hidden="true">
      {Array.from({ length: cuantas }, (_, i) => (
        <li
          key={i}
          style={{ '--r': PROPORCIONES_ESQUELETO[i % PROPORCIONES_ESQUELETO.length] }}
          class="rounded-sm"
        >
          <Skeleton height="100%" style={HEREDA_RADIO} />
        </li>
      ))}
    </ul>
  );
}

/**
 * La galería y las pantallas de filas (un álbum, la papelera, «Parecidas», «Cerca»,
 * un álbum compartido). `mes` pinta además la cabecera de un mes, como la de la
 * galería: su caja mide lo mismo (`py-2` y una línea de `text-sm`).
 */
export function EsqueletoFilas({
  cuantas = 12,
  mes = false,
}: {
  cuantas?: number;
  mes?: boolean;
}): JSX.Element {
  return (
    <Ocupado>
      {mes ? (
        <div class="py-2" aria-hidden="true">
          <Skeleton width="8rem" height="1.25rem" />
        </div>
      ) : null}
      <FilasEsqueleto cuantas={cuantas} />
    </Ocupado>
  );
}

/**
 * La rejilla de álbumes: tarjetas 4:3 con su título y su cuenta, como `TarjetaAlbum`.
 * `cuadradas` es la de «Compartidos conmigo», con portadas cuadradas y menos hueco.
 */
export function EsqueletoAlbumes({
  cuantas = 8,
  cuadradas = false,
}: {
  cuantas?: number;
  cuadradas?: boolean;
}): JSX.Element {
  return (
    <Ocupado>
      <ul
        aria-hidden="true"
        class={cx(
          'grid grid-cols-[repeat(auto-fill,minmax(11rem,1fr))]',
          cuadradas ? 'gap-3' : 'gap-6',
        )}
      >
        {Array.from({ length: cuantas }, (_, i) => (
          <li key={i}>
            <div class={cx('rounded-sm', cuadradas ? 'aspect-square' : 'aspect-[4/3]')}>
              <Skeleton height="100%" style={HEREDA_RADIO} />
            </div>
            {/* Cada línea en la caja de su texto (`text-sm`, 20; `text-xs`, 16), con el
                bloque centrado: la tarjeta mide lo mismo que la de verdad. */}
            <div class={cx('flex h-5 items-center', cuadradas ? 'mt-1' : 'mt-2')}>
              <Skeleton width="60%" height="1rem" />
            </div>
            <div class="flex h-4 items-center">
              <Skeleton width="35%" height="0.75rem" />
            </div>
          </li>
        ))}
      </ul>
    </Ocupado>
  );
}

/** «Duplicadas»: grupos con un borde y miniaturas cuadradas de 160 con su botón. */
export function EsqueletoDuplicadas({ grupos = 2 }: { grupos?: number }): JSX.Element {
  return (
    <Ocupado class="gap-6">
      {Array.from({ length: grupos }, (_, g) => (
        <div key={g} aria-hidden="true" class="border-border rounded-md border p-3">
          <div class="flex flex-wrap gap-3">
            {Array.from({ length: g === 0 ? 3 : 2 }, (_, i) => (
              <div key={i} class="w-40">
                <div class="h-40 w-40 rounded-sm">
                  <Skeleton height="100%" style={HEREDA_RADIO} />
                </div>
                <Skeleton height="2rem" class="mt-2" />
              </div>
            ))}
          </div>
        </div>
      ))}
    </Ocupado>
  );
}
