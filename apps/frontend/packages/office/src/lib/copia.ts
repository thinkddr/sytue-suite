/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Hacer una copia» de un documento, desde la portada (la tesela) y desde el menú
 * Archivo del editor: un solo camino para los dos. El SERVICIO copia el contenido de
 * ahora del original (`copy_of` en POST /documents) en la misma petición; aquí no se
 * lee ni se siembra nada. Quien llama abre la copia con el id que vuelve.
 *
 * El editor guarda ANTES lo que tenga en cola (`useDocument.makeCopy`): la copia sale
 * de lo que el servicio tiene, y sin eso le faltarían los últimos segundos tecleados.
 */
import type { DocKind } from '@sytue/api-clients';

/** El original: su id, su tipo y su título. */
export interface Original {
  id: string;
  kind: DocKind;
  title: string;
}

/**
 * Crea la copia con el título `titulo` (ya traducido: «Copia de …») y devuelve su id.
 * Lanza si el BFF dice que no o si la respuesta no trae id: sin id no hay a dónde ir.
 */
export async function hacerCopia(original: Original, titulo: string): Promise<string> {
  const res = await fetch('/api/create', {
    method: 'POST',
    credentials: 'include',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ kind: original.kind, title: titulo, copy_of: original.id }),
  });
  if (!res.ok) throw new Error(`copia: ${res.status}`);
  const data = (await res.json()) as { id?: unknown };
  if (typeof data.id !== 'string' || data.id === '') throw new Error('copia: respuesta sin id');
  return data.id;
}
