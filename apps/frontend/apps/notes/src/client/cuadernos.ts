/* SPDX-License-Identifier: Apache-2.0 */
import type { Notebook, NotebookInput } from '@sytue/api-clients';

export type ColocacionCuaderno = 'before' | 'after' | 'inside';
export interface CambioCuaderno {
  notebook: Notebook;
  input: NotebookInput;
}

export const ordenDeCuaderno = (a: Notebook, b: Notebook): number =>
  a.position - b.position || a.created_at.localeCompare(b.created_at) || a.id.localeCompare(b.id);

/** PUT sustituye todos los ajustes: mover nunca cambia acceso, formato ni cifrado. */
export function ajustesDelCuaderno(nb: Notebook): NotebookInput {
  return {
    title: nb.title,
    parent_id: nb.parent_id ?? null,
    emoji: nb.emoji ?? '',
    color: nb.color,
    layout: nb.layout,
    visibility: nb.visibility,
    indexable: nb.indexable,
    encrypted: nb.encrypted,
    crypto: nb.crypto ?? null,
    room_id: nb.room_id ?? '',
    position: nb.position,
  };
}

/** Un plan de posiciones enteras sin empates; deja espacio entre ellas para el siguiente gesto. */
export function colocarCuaderno(
  notebooks: readonly Notebook[],
  id: string,
  destino: string,
  lado: ColocacionCuaderno,
): CambioCuaderno[] | null {
  const origen = notebooks.find((n) => n.id === id);
  const target = notebooks.find((n) => n.id === destino);
  if (!origen || !target || origen === target || origen.read_only || target.read_only) return null;
  const padre = lado === 'inside' ? target.id : (target.parent_id ?? null);
  const vistos = new Set<string>();
  let subiendo = padre;
  while (subiendo) {
    if (subiendo === id || vistos.has(subiendo)) return null;
    vistos.add(subiendo);
    subiendo = notebooks.find((n) => n.id === subiendo)?.parent_id ?? null;
  }
  const hermanos = notebooks
    .filter((n) => (n.parent_id ?? null) === padre && n.id !== id)
    .sort(ordenDeCuaderno);
  const indice =
    lado === 'inside'
      ? hermanos.length
      : hermanos.findIndex((n) => n.id === destino) + (lado === 'after' ? 1 : 0);
  if (indice < 0) return null;
  const anterior = hermanos[indice - 1]?.position;
  const siguiente = hermanos[indice]?.position;
  const posicion =
    anterior === undefined
      ? (siguiente ?? 2048) - 1024
      : siguiente === undefined
        ? anterior + 1024
        : Math.floor((anterior + siguiente) / 2);
  if (
    Number.isSafeInteger(posicion) &&
    Math.abs(posicion) < 2_000_000_000 &&
    (anterior === undefined || posicion > anterior) &&
    (siguiente === undefined || posicion < siguiente)
  ) {
    if ((origen.parent_id ?? null) === padre && origen.position === posicion) return [];
    return [
      {
        notebook: origen,
        input: { ...ajustesDelCuaderno(origen), parent_id: padre, position: posicion },
      },
    ];
  }
  hermanos.splice(indice, 0, origen);
  // Solo se renumera cuando no queda un entero libre entre los vecinos.
  const cambios = hermanos.flatMap((nb, i) => {
    const position = (i + 1) * 1024;
    if (nb.position === position && (nb.parent_id ?? null) === padre) return [];
    return [{ notebook: nb, input: { ...ajustesDelCuaderno(nb), parent_id: padre, position } }];
  });
  return cambios.some((c) => c.notebook.read_only) ? null : cambios;
}
