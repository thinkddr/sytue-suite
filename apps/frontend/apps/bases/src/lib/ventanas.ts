/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Carga por ventanas de la rejilla. El servicio pagina por cursor (sin techo de
 * registros), así que la interfaz no puede acumular la tabla entera: guarda como mucho
 * MAX_VENTANAS páginas consecutivas y recuerda el cursor con el que pidió cada una, que
 * es lo que permite volver atrás sin que el servicio sepa paginar hacia atrás. Pura,
 * para probarla sin navegador.
 */
import type { BasesRecord } from '@sytue/api-clients/bases';

export const MAX_VENTANAS = 3;

export interface Ventanas {
  /** Las páginas en memoria, consecutivas. */
  paginas: BasesRecord[][];
  /** Índice (desde 0) de la primera página en memoria. */
  primera: number;
  /** cursores[i] = cursor con el que se pide la página i ('' la primera). */
  cursores: string[];
  /** Cursor de la página que sigue a la última en memoria; '' = no hay más. */
  siguiente: string;
}

interface Pagina {
  records: BasesRecord[];
  next_cursor?: string;
}

export const SIN_VENTANAS: Ventanas = { paginas: [], primera: 0, cursores: [''], siguiente: '' };

/** La primera página de una tabla o vista. */
export function empezar(p: Pagina): Ventanas {
  return { paginas: [p.records], primera: 0, cursores: [''], siguiente: p.next_cursor ?? '' };
}

/** Añade la página pedida con `v.siguiente`; si ya hay MAX_VENTANAS, suelta la primera. */
export function avanzar(v: Ventanas, p: Pagina): Ventanas {
  const indice = v.primera + v.paginas.length;
  const cursores = v.cursores.slice();
  cursores[indice] = v.siguiente;
  const paginas = [...v.paginas, p.records];
  const sobra = paginas.length > MAX_VENTANAS;
  return {
    paginas: sobra ? paginas.slice(1) : paginas,
    primera: sobra ? v.primera + 1 : v.primera,
    cursores,
    siguiente: p.next_cursor ?? '',
  };
}

/** El cursor con el que se pide la página anterior a la primera en memoria, o null. */
export function cursorAnterior(v: Ventanas): string | null {
  return v.primera > 0 ? (v.cursores[v.primera - 1] ?? null) : null;
}

/** Pone delante la página pedida con `cursorAnterior`; si sobra una, suelta la última. */
export function retroceder(v: Ventanas, p: Pagina): Ventanas {
  if (v.primera === 0) return v;
  const paginas = [p.records, ...v.paginas];
  const primera = v.primera - 1;
  if (paginas.length <= MAX_VENTANAS) return { ...v, paginas, primera };
  return {
    paginas: paginas.slice(0, MAX_VENTANAS),
    primera,
    cursores: v.cursores,
    // La que se suelta se volverá a pedir con el cursor con el que se pidió.
    siguiente: v.cursores[primera + MAX_VENTANAS] ?? '',
  };
}

/** Aplica un cambio local (editar, borrar) a los registros de cada página. */
export function editar(v: Ventanas, fn: (rs: BasesRecord[]) => BasesRecord[]): Ventanas {
  return { ...v, paginas: v.paginas.map(fn) };
}

/** Un registro recién creado se enseña al final de lo que hay en pantalla. */
export function anadir(v: Ventanas, r: BasesRecord): Ventanas {
  const paginas = v.paginas.length > 0 ? v.paginas.slice() : [[]];
  paginas[paginas.length - 1] = [...paginas[paginas.length - 1]!, r];
  return { ...v, paginas };
}
