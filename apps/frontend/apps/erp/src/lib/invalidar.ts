/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Invalidar lo cacheado de UN modelo después de escribir en él, mirando la lista que
 * está abierta AHORA y no la de cuando se escribió.
 *
 * Los «Deshacer» de los avisos (papelera.ts, importacion.ts) se pulsan hasta diez
 * segundos después: en ese rato se puede haber abierto otro modelo, cambiado el
 * filtro o empezado otra ficha. Un callback creado en el render del borrado recargaba
 * la lista con el modelo y los filtros de ENTONCES —la de Facturas se rellenaba con
 * los Clientes— y cerraba la ficha que hubiera abierta. Espejo en Fyne:
 * `recargarModelo` de `apps/fyne/apps/erp/acciones.go`.
 */

/** La lista abierta: el modelo y sus filtros, tal como están en este momento. */
export interface ListaAbierta<M extends { clave: string }> {
  modelo: M | null;
  busqueda: string;
  estado: string;
  orden: { campo: string; asc: boolean } | null;
  papelera: boolean;
}

/**
 * Olvida lo cacheado de `clave` —todas sus búsquedas— y, SOLO si es el modelo que
 * está abierto, lo vuelve a pedir con los filtros de ahora. No toca la ficha.
 */
export function invalidarModelo<M extends { clave: string }>(
  clave: string,
  cache: Map<string, unknown>,
  abierta: ListaAbierta<M>,
  recargar: (lista: ListaAbierta<M> & { modelo: M }) => void,
): void {
  // Borrar durante la iteración de un `Map` es seguro: la entrada retirada
  // simplemente no se visita.
  for (const k of cache.keys()) {
    if (k.startsWith(`${clave}|`)) cache.delete(k);
  }
  const { modelo } = abierta;
  if (modelo?.clave === clave) recargar({ ...abierta, modelo });
}
