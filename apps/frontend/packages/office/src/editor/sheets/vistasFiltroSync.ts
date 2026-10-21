/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Sincroniza las vistas de filtro personales con el SERVIDOR (services/docs, 0017). Hasta
 * ahora vivían en el localStorage del navegador; ahora la fuente de verdad es el servidor
 * (por persona y documento) y el navegador queda solo como BUZÓN DE SALIDA: lo que aún no
 * se ha subido (sin red, o una versión anterior que ya las tenía en local). Sin pérdida:
 *
 *  - `guardarVistas` escribe primero en local y solo borra la copia local cuando el
 *    servidor ha confirmado.
 *  - `cargarVistas` une lo que hay en el servidor con lo pendiente en local (id a id) y
 *    lo sube; la copia local se borra al confirmarse. Así la migración desde localStorage
 *    es la propia carga: no hay un paso aparte que se pueda dejar a medias.
 *
 * Lo que sale por la red es solo el estado `{views, active}` de ESTA persona: el servicio
 * toma el `sub` de la sesión, así que no hay manera de pedir las de otra.
 *
 * ponytail: último gana por persona (dos dispositivos que editan a la vez: gana el último
 * en guardar; la unión de la carga solo evita perder lo pendiente de un dispositivo sin
 * red). Si hiciera falta fusionar borrados, la fila pediría una revisión.
 */
import {
  emptyFilterViews,
  mergeFilterViews,
  parseFilterViews,
  type FilterViewsState,
} from './filtro';

/** Lo que necesita del entorno (inyectable: los tests no tienen red ni `localStorage`). */
export interface EntornoVistas {
  fetch: (url: string, init?: RequestInit) => Promise<Response>;
  storage: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'> | null;
}

const url = (docId: string): string => `/api/filter-views?id=${encodeURIComponent(docId)}`;

const leerLocal = (io: EntornoVistas, key: string): string | null => {
  try {
    return io.storage?.getItem(key) ?? null;
  } catch {
    return null;
  }
};
const escribirLocal = (io: EntornoVistas, key: string, v: string | null): void => {
  try {
    if (v === null) io.storage?.removeItem(key);
    else io.storage?.setItem(key, v);
  } catch {
    /* cuota llena o almacenamiento bloqueado: la vista no sobrevive a la pestaña */
  }
};

/** Sube el estado. true si el servidor lo confirmó (y entonces el buzón local se vacía). */
export async function guardarVistas(
  io: EntornoVistas,
  docId: string,
  key: string,
  st: FilterViewsState,
): Promise<boolean> {
  escribirLocal(io, key, JSON.stringify(st)); // primero el buzón: cerrar la pestaña no pierde nada
  try {
    const r = await io.fetch(url(docId), {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: JSON.stringify({ state: st }),
    });
    if (!r.ok) return false;
  } catch {
    return false;
  }
  escribirLocal(io, key, null);
  return true;
}

/**
 * El estado de la persona: el del servidor unido a lo que hubiera pendiente en local.
 * null si el servidor no contesta (el llamador se queda con lo local).
 */
export async function cargarVistas(
  io: EntornoVistas,
  docId: string,
  key: string,
): Promise<FilterViewsState | null> {
  let servidor: FilterViewsState;
  try {
    const r = await io.fetch(url(docId));
    if (!r.ok) return null;
    const body = (await r.json()) as { state?: unknown };
    servidor = parseFilterViews(body.state ? JSON.stringify(body.state) : null);
  } catch {
    return null;
  }
  const crudo = leerLocal(io, key);
  if (crudo === null) return servidor;
  const local = parseFilterViews(crudo);
  if (emptyFilterViews(local)) {
    // Buzón vacío o ilegible: no hay nada que subir. Se vacía solo si SE LEYÓ vacío.
    escribirLocal(io, key, null);
    return servidor;
  }
  const unido = mergeFilterViews(servidor, local);
  // Si la subida falla, el buzón se queda y se reintentará en la próxima carga.
  await guardarVistas(io, docId, key, unido);
  return unido;
}
