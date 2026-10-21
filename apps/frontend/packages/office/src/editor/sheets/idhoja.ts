/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Identidad estable de las hojas (SheetData.id): la co-edición (state/fusion.ts)
 * ancla en ella y no en el nombre, así renombrar una hoja mientras otra persona
 * escribe en ella no la parte en dos.
 */
import type { SheetData } from '../types';

export const nuevoIdHoja = (): string => crypto.randomUUID();

/**
 * Da `id` a las hojas que no lo tienen (o que lo repiten). Es DETERMINISTA, a
 * propósito: dos clientes que abren a la vez un libro anterior a los ids deducen
 * el mismo, y el primer guardado de cualquiera de ellos ya converge.
 */
export function conIdsDeHoja(hojas: SheetData[]): SheetData[] {
  const vistos = new Set<string>();
  return hojas.map((h, i) => {
    let id = h.id || `h:${h.name}`;
    while (vistos.has(id)) id = `${id}#${i}`;
    vistos.add(id);
    return id === h.id ? h : { ...h, id };
  });
}
