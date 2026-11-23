/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La línea de tiempo con las fotos del servidor y las cifradas (ADR 0086) juntas por
 * fecha. Las del servidor llegan por páginas, de la más reciente a la más antigua;
 * las cifradas están todas en el navegador. Para no pintar una cifrada de 2019 antes
 * de que hayan llegado las de 2024 del servidor, una cifrada solo entra cuando ya se
 * ha cargado hasta su fecha (o la línea del servidor ha terminado).
 *
 * Con una búsqueda, el servidor ordena por lo que encuentra: las cifradas que
 * coinciden (búsqueda en el navegador) van delante, por fecha, y luego las suyas.
 */
import type { FotoDeGaleria } from '../client/galeriaCifrada.js';

export type ElementoLinea<S> =
  | { tipo: 'servidor'; id: string; takenAt: string; foto: S }
  | { tipo: 'cifrada'; id: string; takenAt: string; foto: FotoDeGaleria };

export function mezclarLinea<S extends { id: string; takenAt: string }>(
  servidor: readonly S[],
  cifradas: readonly FotoDeGaleria[],
  opciones: {
    /** La línea del servidor ya no tiene más páginas. */
    fin: boolean;
    /** Con un año elegido, solo lo de ese año y anteriores. */
    anyo?: number | null;
    /** Hay una búsqueda: las cifradas (ya filtradas) van delante. */
    busqueda?: boolean;
  },
): ElementoLinea<S>[] {
  const s: ElementoLinea<S>[] = servidor.map((foto) => ({
    tipo: 'servidor',
    id: foto.id,
    takenAt: foto.takenAt,
    foto,
  }));
  const tope = opciones.anyo != null ? Date.UTC(opciones.anyo + 1, 0, 1) : Infinity;
  const ultima = servidor.at(-1)?.takenAt;
  const desde = opciones.fin ? -Infinity : ultima ? Date.parse(ultima) : Infinity;
  const c: ElementoLinea<S>[] = cifradas
    .filter((f) => {
      const ms = Date.parse(f.entrada.tomada);
      return ms < tope && (opciones.busqueda ? true : ms >= desde);
    })
    .map((foto) => ({ tipo: 'cifrada', id: foto.id, takenAt: foto.entrada.tomada, foto }));
  if (opciones.busqueda) return [...c, ...s];
  const out: ElementoLinea<S>[] = [];
  let i = 0;
  let j = 0;
  while (i < s.length || j < c.length) {
    const a = s[i];
    const b = c[j];
    if (b && (!a || Date.parse(b.takenAt) > Date.parse(a.takenAt))) {
      out.push(b);
      j++;
    } else if (a) {
      out.push(a);
      i++;
    }
  }
  return out;
}
