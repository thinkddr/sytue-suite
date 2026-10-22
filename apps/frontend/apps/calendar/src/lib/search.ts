/**
 * Helpers PUROS para presentar un resultado de búsqueda de eventos (sin estado ni
 * DOM): detectar si es una serie recurrente y formatear su «cuándo» y su fragmento
 * en la zona de VISUALIZACIÓN (la del navegador). Las fechas llegan en RFC3339
 * (instante UTC); un evento de todo el día se formatea por su fecha de calendario
 * (sin desplazamiento por zona), como en EventPopover.
 */
import type { EventSearchResult } from '../client/calendarApi.js';
import type { Locale } from '@sytue/i18n';
// `useT` no es un hook (es `createAppTranslator` atado a un idioma), pero su
// nombre lo parece: se importa renombrado para que la regla react-hooks no lo
// confunda al llamarlo desde una función pura.
import { useT as calendarTranslator, type T } from '../i18n/index.js';
import { dayMonthLabel, timeLabel, isSameDay, addDays } from '../components/dates.js';

/** ¿El resultado es una serie recurrente? (lleva RRULE o RDATE). */
export function isRecurring(r: EventSearchResult): boolean {
  return Boolean(r.rrule) || Boolean(r.rdate && r.rdate.length > 0);
}

/** Fecha (mediodía local) del componente de fecha de un instante ISO, sin que la
 * zona desplace el día (para eventos de todo el día). */
function calendarDay(iso: string): Date {
  const [y, m, d] = iso.slice(0, 10).split('-').map(Number);
  return new Date(y ?? 1970, (m ?? 1) - 1, d ?? 1, 12, 0, 0, 0);
}

/**
 * Línea de «cuándo» de un evento, en el idioma del usuario. Es la ÚNICA: la usan
 * tanto el resultado de búsqueda como la vista rápida de la rejilla, que antes
 * repetían las mismas cinco ramas.
 *
 * Cada rama es una PLANTILLA COMPLETA del catálogo («Todo el día · {date}»,
 * «{date}, {start} – {end}»), no trozos pegados con `+`: el separador, el orden
 * y la preposición cambian entre lenguas. Las fechas y las horas ya vienen de
 * `Intl` (dates.ts) en la convención de cada idioma.
 */
export function formatWhen(
  t: T,
  locale: Locale,
  start: Date,
  end: Date | null,
  allDay: boolean,
): string {
  if (allDay) {
    // DTEND de un todo-el-día es EXCLUSIVO: el último día es el anterior al fin.
    const last = end && end.getTime() > start.getTime() ? addDays(end, -1) : start;
    return isSameDay(start, last)
      ? t('when.allDay', { date: dayMonthLabel(locale, start) })
      : t('when.allDayRange', {
          start: dayMonthLabel(locale, start),
          end: dayMonthLabel(locale, last),
        });
  }
  if (!end) {
    return t('when.at', { date: dayMonthLabel(locale, start), time: timeLabel(locale, start) });
  }
  // Mismo día: «10:00 – 11:30»; varios días: la fecha va en cada extremo.
  return isSameDay(start, end)
    ? t('when.range', {
        date: dayMonthLabel(locale, start),
        start: timeLabel(locale, start),
        end: timeLabel(locale, end),
      })
    : t('when.rangeAcross', {
        startDate: dayMonthLabel(locale, start),
        startTime: timeLabel(locale, start),
        endDate: dayMonthLabel(locale, end),
        endTime: timeLabel(locale, end),
      });
}

/**
 * Línea de «cuándo» de un resultado de búsqueda, en la zona de visualización.
 * Se le pide en un idioma CONCRETO (el que baja del middleware), no lo detecta.
 */
export function whenLabel(locale: Locale, r: EventSearchResult): string {
  const t = calendarTranslator(locale);
  return r.all_day
    ? formatWhen(t, locale, calendarDay(r.dtstart), r.dtend ? calendarDay(r.dtend) : null, true)
    : formatWhen(t, locale, new Date(r.dtstart), r.dtend ? new Date(r.dtend) : null, false);
}

/** Fragmento secundario (lugar o descripción) del resultado; «» si no hay. */
export function snippet(r: EventSearchResult): string {
  return (r.location || r.description || '').trim();
}

/** Instante de inicio del resultado como Date (para navegar el calendario). */
export function startDate(r: EventSearchResult): Date {
  return r.all_day ? calendarDay(r.dtstart) : new Date(r.dtstart);
}

/**
 * El día al que lleva el ENLACE de un evento. `/events/{id}` —el que compone el
 * buscador global de la suite— redirige a la vista con `?date=<dtstart>` (y
 * `all_day=1` si es de todo el día), y aquí se abre como un resultado de la búsqueda
 * propia: en ese día. null si la URL no trae una fecha válida.
 */
export function fechaDelEnlace(search: string): Date | null {
  const p = new URLSearchParams(search);
  const dtstart = p.get('date') ?? '';
  if (!/^\d{4}-\d{2}-\d{2}/.test(dtstart)) return null;
  const dia = startDate({ dtstart, all_day: p.get('all_day') === '1' } as EventSearchResult);
  return Number.isNaN(dia.getTime()) ? null : dia;
}
