/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «Compartir mi disponibilidad»: los huecos libres de los próximos días, dentro de la
 * jornada de los ajustes, escritos como texto para pegarlos en un correo o un chat.
 * Es lo que se hace a mano cuando alguien de fuera pregunta «¿cuándo te viene bien?»:
 * abrir la semana, mirar los huecos y escribirlos uno a uno.
 *
 * Los huecos los calcula el servicio («buscar hueco», el mismo de la ficha del evento),
 * así que cuenta lo que cuenta el free/busy: un evento «disponible» no ocupa, uno
 * rechazado tampoco. Aquí solo se agrupan por día y se escriben, en la zona y el idioma
 * de quien comparte, con la zona dicha en la cabecera para que el otro no adivine.
 *
 * Fyne escribe el MISMO texto en `apps/fyne/calendario/disponibilidad.go`.
 */
import { intlTag, type Locale } from '@sytue/i18n';
import type { FreeSlot } from '../client/calendarApi.js';
import type { T } from '../i18n/index.js';

/** Duraciones mínimas que se ofrecen, en minutos. */
export const DURACIONES_DISPONIBILIDAD: readonly number[] = [30, 60];
/** Días hacia delante que se ofrecen. */
export const VENTANAS_DISPONIBILIDAD: readonly number[] = [7, 14];

export interface DiaDisponible {
  /** «lun, 30 nov» en el idioma y la zona de quien comparte. */
  dia: string;
  /** «09:00–10:30», uno por hueco y en orden. */
  tramos: string[];
}

/** Agrupa los huecos por día natural en `zona` (un hueco no cruza la medianoche: la
 *  franja laboral lo corta antes). */
export function diasDisponibles(
  huecos: readonly FreeSlot[],
  zona: string,
  locale: Locale,
): DiaDisponible[] {
  const tag = intlTag(locale);
  const fmtDia = new Intl.DateTimeFormat(tag, {
    weekday: 'short',
    day: 'numeric',
    month: 'short',
    timeZone: zona,
  });
  const fmtHora = new Intl.DateTimeFormat(tag, {
    hour: '2-digit',
    minute: '2-digit',
    timeZone: zona,
  });
  const out: DiaDisponible[] = [];
  const ordenados = [...huecos].sort((a, b) => Date.parse(a.start) - Date.parse(b.start));
  for (const h of ordenados) {
    const ini = new Date(h.start);
    const fin = new Date(h.end);
    if (Number.isNaN(ini.getTime()) || Number.isNaN(fin.getTime()) || !(fin > ini)) continue;
    const dia = fmtDia.format(ini);
    const tramo = `${fmtHora.format(ini)}–${fmtHora.format(fin)}`;
    const ultimo = out[out.length - 1];
    if (ultimo?.dia === dia) ultimo.tramos.push(tramo);
    else out.push({ dia, tramos: [tramo] });
  }
  return out;
}

/** El texto que se copia: la cabecera con la zona y una línea por día. Vacío si no hay
 *  ningún hueco (no se copia una cabecera sin nada debajo). */
export function textoDisponibilidad(t: T, dias: readonly DiaDisponible[], zona: string): string {
  if (dias.length === 0) return '';
  return [
    t('availability.textHeader', { zone: zona }),
    ...dias.map((d) => t('availability.textLine', { day: d.dia, slots: d.tramos.join(', ') })),
  ].join('\n');
}
