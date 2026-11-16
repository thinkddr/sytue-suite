/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Un texto con lo buscado resaltado: el fragmento de un resultado de búsqueda, el
 * nombre de algo en «Ir a», el asunto de un correo o el nombre de un contacto. Con la
 * regla de búsqueda de la suite (`rangosDeConsulta` de @sytue/i18n): «reunion» marca
 * «reunión» entera. Nació en Notes y subió aquí para que Mail y Contacts marquen lo
 * buscado igual, con el mismo amarillo (`.sytue-resalte` de styles.css).
 */
import type { JSX } from 'preact';
import { rangosDeConsulta } from '@sytue/i18n';

export interface ResaltadoProps {
  texto: string;
  /** Lo buscado. Vacía, el texto sale tal cual. */
  consulta: string;
}

export function Resaltado({ texto, consulta }: ResaltadoProps): JSX.Element {
  const trozos: (string | JSX.Element)[] = [];
  let desde = 0;
  for (const [a, b] of rangosDeConsulta(texto, consulta)) {
    if (a > desde) trozos.push(texto.slice(desde, a));
    trozos.push(
      <mark key={a} class="sytue-resalte">
        {texto.slice(a, b)}
      </mark>,
    );
    desde = b;
  }
  if (desde < texto.length) trozos.push(texto.slice(desde));
  return <>{trozos}</>;
}
