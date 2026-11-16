/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

/**
 * El «Deshacer» de la papelera de Fotos (docs/design.md §5.4-5.5).
 *
 * Mandar a la papelera NO pregunta: es reversible, y lo que lo deshace es el aviso de
 * 10 s, no un diálogo antes. Para que ese botón valga, tiene que devolver ESA tanda —no
 * la última que se tiró— y a SU sitio en la galería, no al final de la lista. Y si en
 * esos 10 s la galería ya es otra (otra búsqueda, otra sección), no se toca: pintar las
 * fotos devueltas en una lista que no las pidió sería peor que no pintarlas.
 */
import { enTandas } from '@sytue/ui';

interface ConId {
  id: string;
}

/**
 * Lo que se tiró de una vez, con lo necesario para devolverlo: las fotos tal como se
 * veían, el orden de la galería en ese momento y qué galería era.
 */
export interface Tirada<F extends ConId> {
  fotos: readonly F[];
  /** Los ids de la galería en el orden en que se veían al tirar. */
  orden: readonly string[];
  /** La línea de tiempo que se veía (cambia con cada búsqueda nueva). */
  generacion: number;
}

/** Guarda la tanda que acaba de ir a la papelera: solo las que el servicio aceptó. */
export function tirada<F extends ConId>(
  lista: readonly F[],
  hechas: ReadonlySet<string>,
  generacion: number,
): Tirada<F> {
  return {
    fotos: lista.filter((f) => hechas.has(f.id)),
    orden: lista.map((f) => f.id),
    generacion,
  };
}

/**
 * Pone las devueltas donde estaban: cada una detrás de la última foto que la precedía
 * y sigue en la lista. Lo que se ha movido o cargado entretanto se queda donde está, y
 * una que ya está (por una recarga) no se repite.
 */
export function reponer<F extends ConId>(
  actual: readonly F[],
  devueltas: readonly F[],
  orden: readonly string[],
): F[] {
  const pos = new Map(orden.map((id, i) => [id, i]));
  const lista = [...actual];
  const presentes = new Set(lista.map((f) => f.id));
  const enOrden = [...devueltas].sort(
    (a, b) => (pos.get(a.id) ?? Infinity) - (pos.get(b.id) ?? Infinity),
  );
  for (const f of enOrden) {
    if (presentes.has(f.id)) continue;
    const suya = pos.get(f.id) ?? Infinity;
    let detras = -1;
    lista.forEach((x, i) => {
      const p = pos.get(x.id);
      if (p !== undefined && p < suya) detras = i;
    });
    lista.splice(detras + 1, 0, f);
    presentes.add(f.id);
  }
  return lista;
}

/** Lo que el «Deshacer» necesita saber de la galería AL PULSARLO, no al tirar. */
export interface GaleriaViva {
  montada: boolean;
  generacion: number;
}

/**
 * Restaura cada foto de la tanda (el servicio lo hace por id, de una en una, en tandas
 * como Drive) y, si la galería que se ve es la misma de cuando se tiró, las repone en
 * su sitio. Devuelve cuántas volvieron y cuántas siguen en la papelera.
 */
export async function deshacerTirada<F extends ConId>(
  t: Tirada<F>,
  restaurar: (id: string) => Promise<unknown>,
  galeria: { readonly current: GaleriaViva },
  reponerEn: (cambio: (actual: F[]) => F[]) => void,
): Promise<{ devueltas: number; fallidas: number }> {
  const { hechos, fallidos } = await enTandas(t.fotos, (f) => restaurar(f.id));
  const vista = galeria.current;
  if (hechos.length > 0 && vista.montada && vista.generacion === t.generacion) {
    reponerEn((actual) => reponer(actual, hechos, t.orden));
  }
  return { devueltas: hechos.length, fallidas: fallidos.length };
}

/**
 * Manda a la papelera las que no estén ya de camino. Sin la pregunta de antes, nada
 * frenaba un segundo toque (o Supr mantenido) con la primera llamada en vuelo: salían
 * dos avisos con «Deshacer» y un error por la misma foto. `nuevas` son las que esta
 * llamada ha mandado de verdad; vacía, no hay nada que avisar.
 */
export async function tirarSinRepetir(
  ids: readonly string[],
  tirar: (id: string) => Promise<unknown>,
  enVuelo: Set<string>,
): Promise<{ nuevas: string[]; hechas: Set<string>; fallo?: PromiseRejectedResult }> {
  const nuevas = [...new Set(ids)].filter((id) => !enVuelo.has(id));
  for (const id of nuevas) enVuelo.add(id);
  try {
    const resultados = await Promise.allSettled(nuevas.map((id) => tirar(id)));
    const fallo = resultados.find((r): r is PromiseRejectedResult => r.status === 'rejected');
    return {
      nuevas,
      hechas: new Set(nuevas.filter((_, i) => resultados[i]?.status === 'fulfilled')),
      ...(fallo ? { fallo } : {}),
    };
  } finally {
    for (const id of nuevas) enVuelo.delete(id);
  }
}
