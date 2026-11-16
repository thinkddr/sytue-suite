/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La muestra redonda de un fondo del lienzo, delante de su rótulo en el menú del visor
 * y en el panel del editor. Una sola pieza para los dos: antes cada uno tenía su copia
 * de los colores, y una se quedaba atrás en cuanto se tocaba la otra.
 */
import type { JSX } from 'preact';
import { cx } from '@sytue/ui';
import { clasesMuestraFondo, type FondoLienzo } from '../lib/fondoLienzo.js';

export interface MuestraFondoProps {
  fondo: FondoLienzo;
  /** `sm` (12 px) en el menú del visor y `md` (14 px) en el panel del editor. */
  tamanyo?: 'sm' | 'md';
}

export function MuestraFondo({ fondo, tamanyo = 'sm' }: MuestraFondoProps): JSX.Element {
  return (
    <span
      aria-hidden="true"
      data-fondo={fondo}
      class={cx(
        'shrink-0 rounded-full border',
        tamanyo === 'md' ? 'h-3.5 w-3.5' : 'h-3 w-3',
        clasesMuestraFondo(fondo),
      )}
    />
  );
}
