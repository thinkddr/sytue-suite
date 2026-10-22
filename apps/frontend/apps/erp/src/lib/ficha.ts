/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La IDENTIDAD de la ficha abierta. El formulario de la ficha guarda en su estado lo que
 * se está escribiendo, y ese estado nace de su registro UNA vez, al montarse. Si la ficha
 * no se vuelve a montar cuando cambia de registro, lo escrito —o lo leído— de uno se
 * queda en el formulario del siguiente: «Nuevo» con una ficha abierta daba de alta una
 * copia del registro abierto, y abrir otra fila guardaba en ella los valores de la
 * anterior.
 *
 * Esta clave va en el `key` de `<Ficha>`: cada registro, cada alta en blanco y cada
 * duplicado es una ficha DISTINTA, con su formulario recién hecho.
 */

/**
 * @param modelo  Clave del modelo (`contacto`, `factura_venta`…).
 * @param ficha   El registro abierto, o `'nueva'` para un alta.
 * @param semilla De dónde copia sus valores un alta (duplicar); nulo si parte en blanco.
 */
export function claveDeFicha(
  modelo: string,
  ficha: { id: string } | 'nueva',
  semilla: { id: string } | null,
): string {
  if (ficha !== 'nueva') return `${modelo}:registro:${ficha.id}`;
  return semilla ? `${modelo}:copia:${semilla.id}` : `${modelo}:nueva`;
}
