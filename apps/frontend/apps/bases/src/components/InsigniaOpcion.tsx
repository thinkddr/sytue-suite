/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Una opción de un campo de selección tal como se pinta en una celda o en la ficha. Si
 * ya no es una opción del campo (la retiraron), el registro la conserva: sale en gris,
 * con borde discontinuo y el texto «opción retirada», no solo con color.
 */
import type { JSX } from 'preact';
import { Badge } from '@sytue/ui';
import type { BasesField } from '@sytue/api-clients/bases';
import type { T } from '../i18n/index.js';
import { tonoDeOpcion } from '../lib/aspecto.js';
import { esOpcionRetirada } from '../lib/esquema.js';

export function InsigniaOpcion(p: { t: T; campo: BasesField; opcion: string }): JSX.Element {
  if (!esOpcionRetirada(p.campo, p.opcion)) {
    return (
      <Badge tone={tonoDeOpcion(p.campo, p.opcion)} class="max-w-full">
        <span class="break-words whitespace-normal">{p.opcion}</span>
      </Badge>
    );
  }
  const retirada = p.t('field.retiredOption');
  return (
    <Badge
      tone="neutral"
      class="border-border-strong max-w-full gap-1 border border-dashed"
      title={retirada}
      data-retirada=""
    >
      <span class="break-words whitespace-normal">{p.opcion}</span>
      <span class="font-normal italic">· {retirada}</span>
    </Badge>
  );
}
