/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Lo que la búsqueda de la lista mira ADEMÁS del nombre, los correos, la organización
 * y el teléfono: el apodo («Pili»), el cargo («fontanero»), la ciudad de sus
 * direcciones y la nota. Es como se recuerda a mucha gente —«el de Zaragoza», «la
 * abogada»— y hasta ahora no la encontraba nadie.
 *
 * Fyne mira los mismos campos (`coincide` en apps/fyne/contactos/app.go).
 */
import { parseVcardDetails } from '../lib/vcard.js';

/** Los textos de más de un contacto, en uno; vacío si el vCard no se puede leer. */
export function textoBuscable(vcard: string): string {
  try {
    const d = parseVcardDetails(vcard);
    return [d.nickname, d.title, d.note, ...d.addresses.map((a) => a.city ?? '')]
      .filter(Boolean)
      .join('\n');
  } catch {
    return '';
  }
}

/** El apellido del N del vCard, para «Ordenar por apellido»; vacío si no lo trae. */
export function apellidoDe(vcard: string): string {
  try {
    return parseVcardDetails(vcard).lastName.trim();
  } catch {
    return '';
  }
}
