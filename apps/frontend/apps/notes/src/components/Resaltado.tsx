/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Un texto con lo buscado resaltado: el fragmento de un resultado de búsqueda o
 * el nombre de algo en «Ir a». Con la regla de búsqueda de la suite
 * (`rangosDeConsulta` de @sytue/i18n): «reunion» marca «reunión» entera.
 */
import type { JSX } from 'preact';
import { rangosDeConsulta } from '@sytue/i18n';

export function Resaltado({ texto, consulta }: { texto: string; consulta: string }): JSX.Element {
  const trozos: (string | JSX.Element)[] = [];
  let desde = 0;
  for (const [a, b] of rangosDeConsulta(texto, consulta)) {
    if (a > desde) trozos.push(texto.slice(desde, a));
    trozos.push(
      <mark key={a} class="notes-resalte">
        {texto.slice(a, b)}
      </mark>,
    );
    desde = b;
  }
  if (desde < texto.length) trozos.push(texto.slice(desde));
  return <>{trozos}</>;
}
