/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Ordenar por apellido»: la libreta en el orden de la guía de teléfonos, con las
 * cabeceras A–Z por la inicial del apellido. Quien guarda «Ana García» y «Luis
 * García» los busca juntos, en la G. Un contacto sin apellido (una empresa, un
 * «Fontanero») se ordena por su nombre, como en Contactos de Apple.
 *
 * Es una preferencia de quien mira y de este navegador, por persona, como las de
 * Calendar; Fyne guarda la misma en su almacén por persona (`contactos/orden.go`).
 */
import { intlTag, type Locale } from '@sytue/i18n';

export type OrdenLista = 'nombre' | 'apellido';

const clave = (userId: string): string => `sytue.contacts.${userId}.orden`;

export function leerOrden(userId: string): OrdenLista {
  try {
    return localStorage.getItem(clave(userId)) === 'apellido' ? 'apellido' : 'nombre';
  } catch {
    return 'nombre';
  }
}

export function guardarOrden(userId: string, orden: OrdenLista): void {
  try {
    if (orden === 'apellido') localStorage.setItem(clave(userId), orden);
    else localStorage.removeItem(clave(userId));
  } catch {
    // Sin almacenamiento vale para esta sesión.
  }
}

/** Por qué se ordena (y se agrupa) un contacto: su apellido o, sin él, su nombre. */
export function claveDeOrden(nombre: string, apellido: string, orden: OrdenLista): string {
  return orden === 'apellido' && apellido.trim() ? apellido.trim() : nombre;
}

/** Los contactos en el orden pedido, sin tocar los de entrada (estable a igualdad). */
export function ordenar<T>(
  items: readonly T[],
  clave: (item: T) => string,
  nombre: (item: T) => string,
  locale: Locale,
): T[] {
  const col = new Intl.Collator(intlTag(locale), { sensitivity: 'base', numeric: true });
  return [...items].sort(
    (a, b) => col.compare(clave(a), clave(b)) || col.compare(nombre(a), nombre(b)),
  );
}
