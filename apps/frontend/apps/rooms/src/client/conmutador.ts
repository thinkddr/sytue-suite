/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El orden del conmutador rápido de canales (Ctrl+G): con la caja vacía, lo que
 * tiene algo sin leer primero y luego por nombre; escribiendo, primero lo que
 * EMPIEZA por lo escrito, después lo que lo contiene, y dentro de cada grupo lo
 * no leído por delante. Sin acentos ni mayúsculas: «facturacion» encuentra
 * «Facturación».
 */
import { plegar } from '@sytue/i18n';
export interface CanalConmutable {
  id: string;
  name: string;
  unread?: number | undefined;
  mentions?: number | undefined;
}

export function normalizar(s: string): string {
  return plegar(s).trim();
}

export function ordenarCanales<C extends CanalConmutable>(canales: readonly C[], q: string): C[] {
  const texto = normalizar(q);
  const pendiente = (c: C) => (c.unread ?? 0) + (c.mentions ?? 0);
  const peso = (c: C): number => {
    if (!texto) return 0;
    const n = normalizar(c.name);
    if (n.startsWith(texto)) return 2;
    if (n.includes(texto)) return 1;
    return -1;
  };
  return canales
    .map((c) => ({ c, peso: peso(c) }))
    .filter((x) => x.peso >= 0)
    .sort(
      (a, b) =>
        b.peso - a.peso ||
        Number(pendiente(b.c) > 0) - Number(pendiente(a.c) > 0) ||
        a.c.name.localeCompare(b.c.name, undefined, { sensitivity: 'base' }),
    )
    .map((x) => x.c);
}
