/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Anclas de fila y de columna: identidad estable de las filas y columnas de una hoja para
 * que el cursor y la selección de otra persona sigan a SU fila aunque alguien inserte o
 * borre por encima. Las celdas se direccionan por referencia («B2»), que cambia al
 * insertar, así que el índice no sirve como identidad.
 *
 * Modelo: `SheetData.rowIds` y `colIds` son listas de `{ id }` con la identidad de las
 * primeras filas o columnas; las que quedan más allá son IMPLÍCITAS y se llaman
 * `t<posición - longitud de la lista + base>` (`rowBase`/`colBase`, 0 si falta). Una hoja
 * sin listas (cualquier libro anterior, cualquier importación) tiene todas implícitas:
 * `t0`, `t1`… Insertar o borrar solo materializa lo que hace falta, conservando el nombre
 * que ya tenían, así que el libro no crece mientras nadie toque la estructura y la
 * identidad no cambia al hacerlo. Se guardan como lista con `id` para que la fusión a tres bandas
 * (state/fusion.ts) las trate por clave: dos inserciones concurrentes se conservan las dos.
 *
 * ponytail: dos personas que materializan a la vez hasta distinto punto fusionan la lista
 * (mismos nombres) pero `base` es un número suelto y gana el propio; en ese caso raro un
 * cursor de la cola puede caer en la fila vecina hasta que su dueño vuelva a moverse.
 * Mover filas sin insertar ni borrar (ordenar) no cambia ids.
 */
import type { SheetData } from '../types';

/** Identidad de una fila o de una columna. */
export interface Ancla {
  id: string;
}
/** Un eje de la hoja: la lista materializada y el desplazamiento del nombre de la cola. */
export interface Eje {
  ids?: readonly Ancla[] | undefined;
  base?: number | undefined;
}

/** Tope de la lista materializada (entrada hostil). */
const MAX_ANCLAS = 20_000;

/** Id de nueva inserción: aleatorio, único entre clientes sin coordinarse. */
export const nuevaAncla = (): string => `n${crypto.randomUUID().slice(0, 8)}`;

/** Los ejes de una hoja. */
export const ejeFilas = (s: Pick<SheetData, 'rowIds' | 'rowBase'>): Eje => ({
  ids: s.rowIds,
  base: s.rowBase,
});
export const ejeColumnas = (s: Pick<SheetData, 'colIds' | 'colBase'>): Eje => ({
  ids: s.colIds,
  base: s.colBase,
});

/** Identidad de la posición `i` (0-based). */
export const idDe = (e: Eje, i: number): string =>
  e.ids?.[i]?.id ?? `t${i - (e.ids?.length ?? 0) + (e.base ?? 0)}`;

/** Posición actual de una identidad, o null si ya no existe (borrada) o no es válida. */
export function indiceDe(e: Eje, id: string): number | null {
  const i = e.ids ? e.ids.findIndex((x) => x.id === id) : -1;
  if (i >= 0) return i;
  const m = /^t(\d{1,7})$/.exec(id);
  const k = m ? Number(m[1]) - (e.base ?? 0) : -1;
  return k >= 0 ? (e.ids?.length ?? 0) + k : null;
}

/** Materializa hasta `hasta` posiciones con el nombre que ya tenían las implícitas. */
function materializar(e: Eje, hasta: number): { ids: Ancla[]; base: number } {
  const ids = e.ids ? [...e.ids] : [];
  let base = e.base ?? 0;
  const falta = Math.min(hasta, MAX_ANCLAS) - ids.length;
  for (let k = 0; k < falta; k += 1) ids.push({ id: `t${base + k}` });
  if (falta > 0) base += falta;
  return { ids, base };
}

/** Inserta `n` posiciones nuevas en `desde`. */
export function insertar(
  e: Eje,
  desde: number,
  n: number,
  nueva = nuevaAncla,
): { ids: Ancla[]; base: number } {
  const m = materializar(e, desde);
  m.ids.splice(desde, 0, ...Array.from({ length: n }, () => ({ id: nueva() })));
  return m;
}

/** Quita `n` posiciones desde `desde`. */
export function quitar(e: Eje, desde: number, n: number): { ids: Ancla[]; base: number } {
  const m = materializar(e, desde + n);
  m.ids.splice(desde, n);
  return m;
}

/** Extremos de un rango con la identidad de cada uno. */
export const anclarRango = (e: Eje, i1: number, i2: number): [string, string] => [
  idDe(e, i1),
  idDe(e, i2),
];

/** Dónde caen hoy los extremos anclados; si alguno desapareció, los índices que traía. */
export const resolverRango = (
  e: Eje,
  ids: readonly [string, string] | undefined,
  i1: number,
  i2: number,
): [number, number] => {
  const x = ids ? indiceDe(e, ids[0]) : null;
  const y = ids ? indiceDe(e, ids[1]) : null;
  return x !== null && y !== null ? [Math.min(x, y), Math.max(x, y)] : [i1, i2];
};

/** Los campos de anclas de la hoja que cambian con una operación estructural sobre un eje. */
export function conEstructura(
  s: SheetData,
  eje: 'fila' | 'columna',
  desde: number,
  cantidad: number,
): Pick<SheetData, 'rowIds' | 'rowBase' | 'colIds' | 'colBase'> {
  const e = eje === 'fila' ? ejeFilas(s) : ejeColumnas(s);
  const r = cantidad > 0 ? insertar(e, desde, cantidad) : quitar(e, desde, -cantidad);
  return eje === 'fila' ? { rowIds: r.ids, rowBase: r.base } : { colIds: r.ids, colBase: r.base };
}
