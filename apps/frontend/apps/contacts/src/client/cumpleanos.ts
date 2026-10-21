/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Próximos cumpleaños, leídos del BDAY del vCard de cada contacto. Se admiten
 * las cuatro formas que circulan —«19900215», «1990-02-15», «--0215» y
 * «--02-15» (sin año, vCard 4)— y las demás se ignoran sin ruido: un
 * cumpleaños mal escrito no es un fallo de la libreta.
 */
import type { Contact } from '@sytue/api-clients';

export interface Cumpleanos {
  contact: Contact;
  /** La próxima vez que cae, a medianoche local. */
  fecha: Date;
  /** Días que faltan (0 = hoy). */
  dias: number;
  /** Los años que cumple, si el vCard trae el de nacimiento. */
  edad?: number;
}

const BDAY = /^BDAY(?:;[^:]*)?:(.+)$/im;

/** Mes (1-12), día y, si lo hay, año de nacimiento; `null` si no hay BDAY legible. */
export function cumpleanosDe(vcard: string): { mes: number; dia: number; anyo?: number } | null {
  const m = BDAY.exec(vcard);
  if (!m) return null;
  const v = (m[1] ?? '').trim();
  let r = /^(\d{4})-?(\d{2})-?(\d{2})/.exec(v);
  if (r) {
    const [, a, mm, dd] = r;
    return valida(Number(mm), Number(dd), Number(a));
  }
  r = /^--(\d{2})-?(\d{2})/.exec(v);
  if (r) return valida(Number(r[1]), Number(r[2]));
  return null;
}

function valida(mes: number, dia: number, anyo?: number) {
  if (mes < 1 || mes > 12 || dia < 1 || dia > 31) return null;
  return anyo && anyo > 1000 ? { mes, dia, anyo } : { mes, dia };
}

/**
 * Los cumpleaños de `contacts` en los próximos `ventana` días desde `hoy`, del
 * más cercano al más lejano. Un 29 de febrero en año no bisiesto cae el 1 de
 * marzo, que es lo que hace el propio `Date`.
 */
export function proximosCumpleanos(
  contacts: readonly Contact[],
  hoy: Date,
  ventana = 365,
): Cumpleanos[] {
  const inicio = new Date(hoy.getFullYear(), hoy.getMonth(), hoy.getDate());
  const out: Cumpleanos[] = [];
  for (const contact of contacts) {
    const c = cumpleanosDe(contact.vcard);
    if (!c) continue;
    let fecha = new Date(inicio.getFullYear(), c.mes - 1, c.dia);
    if (fecha < inicio) fecha = new Date(inicio.getFullYear() + 1, c.mes - 1, c.dia);
    const dias = Math.round((fecha.getTime() - inicio.getTime()) / 86_400_000);
    if (dias > ventana) continue;
    out.push({ contact, fecha, dias, ...(c.anyo ? { edad: fecha.getFullYear() - c.anyo } : {}) });
  }
  return out.sort(
    (a, b) => a.dias - b.dias || a.contact.fullName.localeCompare(b.contact.fullName),
  );
}
