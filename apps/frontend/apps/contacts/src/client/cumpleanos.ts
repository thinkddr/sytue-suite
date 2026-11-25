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
  /** Es un aniversario (ANNIVERSARY), no un cumpleaños: la lista lo dice. */
  aniversario?: true;
}

// Con prefijo de grupo opcional (`item1.BDAY`, como lo escriben algunos clientes)
// y sus parámetros aparte: la ficha leía antes el BDAY con `parseVcardDetails`,
// que ya respetaba los grupos, y no debe dejar de enseñarlo por leerlo aquí.
const BDAY = /^(?:[A-Za-z0-9-]+\.)?BDAY((?:;[^:]*)?):(.+)$/im;
// El aniversario (vCard 4) se escribe con las mismas formas que el cumpleaños.
const ANNIVERSARY = /^(?:[A-Za-z0-9-]+\.)?ANNIVERSARY((?:;[^:]*)?):(.+)$/im;

/** Mes (1-12), día y, si lo hay, año de nacimiento; `null` si no hay BDAY legible. */
export function cumpleanosDe(vcard: string): { mes: number; dia: number; anyo?: number } | null {
  return fechaDe(vcard, BDAY);
}

/** Lo mismo con el ANNIVERSARY: una boda, el día que se conocieron. */
export function aniversarioDe(vcard: string): { mes: number; dia: number; anyo?: number } | null {
  return fechaDe(vcard, ANNIVERSARY);
}

function fechaDe(
  vcard: string,
  propiedad: RegExp,
): { mes: number; dia: number; anyo?: number } | null {
  const m = propiedad.exec(vcard.replace(/\r?\n[ \t]/g, ''));
  if (!m) return null;
  const v = (m[2] ?? '').trim();
  let r = /^(\d{4})-?(\d{2})-?(\d{2})/.exec(v);
  if (r) {
    const [, a, mm, dd] = r;
    // Contactos de Apple guarda el cumpleaños sin año como un año ficticio y lo
    // marca con `X-APPLE-OMIT-YEAR=<ese año>`: tomarlo por real daría «… de 1604»
    // en la ficha y una edad de cuatro siglos en «Próximos cumpleaños».
    const omitido = /;X-APPLE-OMIT-YEAR=(\d{4})/i.exec(m[1] ?? '')?.[1];
    return valida(Number(mm), Number(dd), omitido === a ? undefined : Number(a));
  }
  r = /^--(\d{2})-?(\d{2})/.exec(v);
  if (r) return valida(Number(r[1]), Number(r[2]));
  return null;
}

/**
 * El cumpleaños en el formato largo del idioma (`tag` BCP 47): «15 de febrero de
 * 1990» o, si el vCard no trae el año, «15 de febrero». La fecha se construye por
 * partes y no con `new Date('1990-02-15')`, que es medianoche UTC y al oeste de
 * Greenwich se pinta como el día 14. Sin año se usa uno bisiesto para que un
 * 29 de febrero no se convierta en 1 de marzo.
 */
export function formatearCumpleanos(
  c: { mes: number; dia: number; anyo?: number | undefined },
  tag: string,
): string {
  if (c.anyo) {
    return new Intl.DateTimeFormat(tag, { day: 'numeric', month: 'long', year: 'numeric' }).format(
      new Date(c.anyo, c.mes - 1, c.dia),
    );
  }
  return new Intl.DateTimeFormat(tag, { day: 'numeric', month: 'long' }).format(
    new Date(2000, c.mes - 1, c.dia),
  );
}

function valida(mes: number, dia: number, anyo?: number) {
  const conAnyo = anyo && anyo > 1000 ? anyo : undefined;
  // Un 31 de febrero no es un cumpleaños: `Date` lo correría al 2 de marzo. Sin año
  // se mide contra uno bisiesto para que el 29 de febrero valga.
  const diasDelMes = new Date(conAnyo ?? 2000, mes, 0).getDate();
  if (mes < 1 || mes > 12 || dia < 1 || dia > diasDelMes) return null;
  return conAnyo ? { mes, dia, anyo: conAnyo } : { mes, dia };
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
    // El cumpleaños y, si lo hay, el aniversario: los dos son «el día de alguien» que
    // no se quiere olvidar, y la lista ya está para eso.
    for (const [c, aniversario] of [
      [cumpleanosDe(contact.vcard), false],
      [aniversarioDe(contact.vcard), true],
    ] as const) {
      if (!c) continue;
      let fecha = new Date(inicio.getFullYear(), c.mes - 1, c.dia);
      if (fecha < inicio) fecha = new Date(inicio.getFullYear() + 1, c.mes - 1, c.dia);
      const dias = Math.round((fecha.getTime() - inicio.getTime()) / 86_400_000);
      if (dias > ventana) continue;
      out.push({
        contact,
        fecha,
        dias,
        ...(aniversario
          ? { aniversario: true as const }
          : c.anyo
            ? { edad: fecha.getFullYear() - c.anyo }
            : {}),
      });
    }
  }
  return out.sort(
    (a, b) => a.dias - b.dias || a.contact.fullName.localeCompare(b.contact.fullName),
  );
}
