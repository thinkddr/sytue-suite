/**
 * Utilidades de fecha para las vistas mes/semana/día. Sin dependencias externas.
 * También parsea DTSTART iCalendar a Date y convierte rangos a formato
 * iCalendar UTC para las consultas CalDAV.
 *
 * IDIOMA: todo lo que se ve lleva `locale` como primer parámetro y sale de
 * `Intl` (docs/CONVENTIONS.md §13.1). NO hay tablas de días ni de meses escritas
 * a mano: los nombres los da CLDR en los cinco idiomas y siempre con la
 * ortografía correcta («mié.» / «Mi.» / «mer.»).
 *
 * ponytail: la semana empieza en LUNES en las cinco lenguas (Europa
 * continental). Es lo correcto para es/fr/de/it y para en-GB; solo en EE. UU.
 * sería domingo, y cambiarlo toca la rejilla entera (monthGrid, weekGrid,
 * multiDayLayout y el índice `(getDay() + 6) % 7` de las cabeceras). Upgrade:
 * `Intl.Locale.prototype.getWeekInfo().firstDay` cuando entre una región que lo
 * necesite, tocando SOLO `startOfWeek` y ese índice.
 */

import type { CalendarEvent } from '@sytue/api-clients';
import { intlTag, type Locale } from '@sytue/i18n';
import { parseExdates, parseRecurrence, type Recurrence } from '../lib/ical.js';
import { instantFromZonedWallClock } from './tz.js';

export type ViewMode = 'month' | 'week' | 'day' | 'agenda';

/** Nº de días que abarca la vista agenda/lista desde el cursor. */
export const AGENDA_DAYS = 30;

// Caché de formateadores por (idioma, forma): construir un Intl.DateTimeFormat
// no es gratis y la rejilla del mes pide 42 celdas en cada repintado.
const formatters = new Map<string, Intl.DateTimeFormat>();

function fmt(
  locale: Locale,
  shape: string,
  options: Intl.DateTimeFormatOptions,
): Intl.DateTimeFormat {
  const key = `${locale}|${shape}`;
  let f = formatters.get(key);
  if (f === undefined) {
    f = new Intl.DateTimeFormat(intlTag(locale), options);
    formatters.set(key, f);
  }
  return f;
}

const weekdayCache = new Map<string, string[]>();

/**
 * Los siete días (lun→dom) en el idioma del usuario y con el ancho pedido. Salen de
 * `Intl` sobre una semana conocida —el 5 de enero de 2026 fue lunes—, nunca de un array
 * escrito a mano.
 */
function weekdays(locale: Locale, width: 'long' | 'short' | 'narrow'): string[] {
  const key = `${locale}|${width}`;
  let labels = weekdayCache.get(key);
  if (labels === undefined) {
    const f = fmt(locale, `weekday-${width}`, { weekday: width });
    labels = Array.from({ length: 7 }, (_, i) => f.format(new Date(2026, 0, 5 + i)));
    weekdayCache.set(key, labels);
  }
  return labels;
}

/** Cabeceras de los siete días (lun→dom): la abreviatura de `Intl`. */
export function weekdayLabels(locale: Locale): string[] {
  return weekdays(locale, 'short');
}

/**
 * Iniciales de los siete días (lun→dom): el mini-calendario de la barra lateral y las
 * cabeceras de columna estrecha. También de `Intl`: la tabla escrita a mano que había
 * aquí («L M X J V S D») solo era correcta en español.
 */
export function weekdayNarrowLabels(locale: Locale): string[] {
  return weekdays(locale, 'narrow');
}

/** Los siete días (lun→dom) con su nombre entero: lo que oye un lector de pantalla. */
export function weekdayLongLabels(locale: Locale): string[] {
  return weekdays(locale, 'long');
}

export function startOfDay(d: Date): Date {
  const x = new Date(d);
  x.setHours(0, 0, 0, 0);
  return x;
}

/** Lunes de la semana de la fecha dada. */
export function startOfWeek(d: Date): Date {
  const x = startOfDay(d);
  const day = (x.getDay() + 6) % 7; // 0 = lunes
  x.setDate(x.getDate() - day);
  return x;
}

export function addDays(d: Date, n: number): Date {
  const x = new Date(d);
  x.setDate(x.getDate() + n);
  return x;
}

export function addMonths(d: Date, n: number): Date {
  const x = new Date(d);
  x.setMonth(x.getMonth() + n);
  return x;
}

export function isSameDay(a: Date, b: Date): boolean {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}

export function isToday(d: Date): boolean {
  return isSameDay(d, new Date());
}

/**
 * ¿El intervalo [start, end) de un evento solapa el día `day` (hora local)?
 * A diferencia de `isSameDay(start, day)`, pinta en un día también los eventos
 * que lo cruzan desde el día anterior (no solo los que empiezan ese día). `end`
 * es EXCLUSIVO: un evento que termina justo a medianoche no aparece en el día
 * siguiente. Sin fin (end=null) o de duración nula, pertenece solo al día de su
 * inicio. Comparación por instantes absolutos: sólida ante cambios de hora (DST).
 */
export function overlapsDay(start: Date, end: Date | null, day: Date): boolean {
  const dayStart = startOfDay(day).getTime();
  const dayEnd = startOfDay(addDays(day, 1)).getTime();
  const startMs = start.getTime();
  if (startMs >= dayEnd) return false; // empieza después del día
  const endMs = end ? end.getTime() : startMs;
  return endMs > startMs ? endMs > dayStart : startMs >= dayStart;
}

/** Las 6 semanas (42 dias) que cubren el mes de la fecha. */
export function monthGrid(d: Date): Date[] {
  const first = new Date(d.getFullYear(), d.getMonth(), 1);
  const start = startOfWeek(first);
  return Array.from({ length: 42 }, (_, i) => addDays(start, i));
}

/** Los 7 dias (lun-dom) de la semana de la fecha. */
export function weekGrid(d: Date): Date[] {
  const start = startOfWeek(d);
  return Array.from({ length: 7 }, (_, i) => addDays(start, i));
}

/**
 * Título de la vista. El rango de la semana lo compone `formatRange` de `Intl`,
 * no una concatenación: cada lengua une los extremos a su manera («6–12 de julio
 * de 2026», «6–12 July 2026», «29 juin – 5 juil. 2026») y pegar trozos con «de»
 * en medio solo funcionaría en español.
 *
 * El modo agenda devuelve solo la fecha; la frase que la envuelve es texto de
 * interfaz y vive en el catálogo de la app, no aquí.
 */
export function titleFor(mode: ViewMode, d: Date, locale: Locale): string {
  if (mode === 'month') {
    return capitalize(fmt(locale, 'monthYear', { month: 'long', year: 'numeric' }).format(d));
  }
  if (mode === 'day') {
    return capitalize(
      fmt(locale, 'dayLong', {
        weekday: 'long',
        day: 'numeric',
        month: 'long',
        year: 'numeric',
      }).format(d),
    );
  }
  if (mode === 'agenda') return dayMonthLabel(locale, d);
  const days = weekGrid(d);
  return capitalize(
    fmt(locale, 'weekRange', { day: 'numeric', month: 'long', year: 'numeric' }).formatRange(
      days[0]!,
      days[6]!,
    ),
  );
}

/** «3 de julio» — para nombres accesibles y títulos cortos. */
export function dayMonthLabel(locale: Locale, d: Date): string {
  return fmt(locale, 'dayMonth', { day: 'numeric', month: 'long' }).format(d);
}

/** «3 de julio de 2026» — nombre accesible de un día concreto. Lo compone `Intl`
 * entero: pegar el año con « de » solo funcionaría en español. */
export function dayMonthYearLabel(locale: Locale, d: Date): string {
  return fmt(locale, 'dayMonthYear', { day: 'numeric', month: 'long', year: 'numeric' }).format(d);
}

/** «jul» — mes abreviado (columna de fecha de la agenda). */
export function monthShortLabel(locale: Locale, d: Date): string {
  return fmt(locale, 'monthShort', { month: 'short' }).format(d);
}

/** La hora, en la convención del idioma (24 h en Europa, 12 h en inglés). */
export function timeLabel(locale: Locale, d: Date): string {
  return fmt(locale, 'time', { hour: '2-digit', minute: '2-digit' }).format(d);
}

/** Parsea DTSTART/DTEND iCalendar (DATE o DATE-TIME, UTC o local) a Date. */
export function parseIcalDate(value: string): Date | null {
  if (!value) return null;
  // yyyymmdd (todo el dia)
  const dateOnly = value.match(/^(\d{4})(\d{2})(\d{2})$/);
  if (dateOnly) {
    return new Date(Number(dateOnly[1]), Number(dateOnly[2]) - 1, Number(dateOnly[3]));
  }
  // yyyymmddThhmmss(Z)
  const dt = value.match(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})(Z)?$/);
  if (dt) {
    const [, y, mo, da, h, mi, s, z] = dt;
    if (z) return new Date(Date.UTC(+y!, +mo! - 1, +da!, +h!, +mi!, +s!));
    return new Date(+y!, +mo! - 1, +da!, +h!, +mi!, +s!);
  }
  const parsed = new Date(value);
  return Number.isNaN(parsed.getTime()) ? null : parsed;
}

/**
 * TZID del VEVENT (parámetro TZID del DTSTART), o null si el DTSTART es UTC
 * (…Z), flotante o de día completo. El SDK CalDAV descarta los parámetros al
 * exponer `ev.start`, pero el iCal crudo (`ev.ical`) conserva DTSTART;TZID=…
 */
export function parseTzid(ical: string): string | null {
  const m = ical.match(/^DTSTART;[^:\r\n]*\bTZID=([^:;\r\n]+)/im);
  return m?.[1]?.trim() || null;
}

/**
 * Instante ABSOLUTO de un DTSTART/DTEND, resuelto a la ZONA DE VISUALIZACIÓN (la
 * del navegador): honra `Z` (UTC), el TZID del evento (hora-de-pared en su zona)
 * o, en su defecto, la hora local. Los de día completo (yyyymmdd) son FLOTANTES:
 * medianoche local, el mismo día natural en cualquier zona. El resultado, leído
 * con los métodos locales de Date, ya está posicionado en la zona del navegador,
 * así que timeLayout/overlapsDay reciben instantes ya resueltos a esa zona.
 */
export function eventInstant(value: string, tzid: string | null): Date | null {
  if (!value) return null;
  // UTC, día completo o local-sin-zona: parseo directo (instante ya correcto).
  if (value.endsWith('Z') || /^\d{8}$/.test(value) || !tzid) return parseIcalDate(value);
  const m = value.match(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})$/);
  if (!m) return parseIcalDate(value);
  return instantFromZonedWallClock(+m[1]!, +m[2]!, +m[3]!, +m[4]!, +m[5]!, +m[6]!, tzid);
}

/** Convierte una Date a formato iCalendar UTC para consultas CalDAV. */
export function toIcalUtc(d: Date): string {
  return d
    .toISOString()
    .replace(/[-:]/g, '')
    .replace(/\.\d{3}Z$/, 'Z');
}

/** yyyymmdd (fecha local) para DTSTART de un evento de todo el dia. */
function toIcalDateLocal(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}${pad(d.getMonth() + 1)}${pad(d.getDate())}`;
}

/** DTSTART de la i-esima ocurrencia, calculado desde el maestro (sin acumular). */
export function occurrenceStart(
  base: Date,
  i: number,
  freq: Recurrence['freq'],
  interval: number,
): Date {
  if (i === 0) return new Date(base);
  if (freq === 'DAILY') return addDays(base, i * interval);
  if (freq === 'WEEKLY') return addDays(base, i * interval * 7);
  return addMonths(base, i * interval); // MONTHLY (día del mes vía JS Date; Feb 30 → Mar)
}

/**
 * Expande los eventos recurrentes (RRULE) en ocurrencias individuales dentro de
 * [rangeStart, rangeEnd). El backend CalDAV NO expande: devuelve el VEVENT
 * maestro con su RRULE, y solo lo incluye en el rango si el maestro (su primera
 * instancia) solapa la ventana. Por eso expandimos aquí, cara al usuario.
 *
 * Cada ocurrencia es un clon del maestro con su propio start/end; conserva
 * href/uid/ical del maestro, así que editar/borrar cualquier ocurrencia opera
 * sobre la serie. Los eventos puntuales pasan tal cual.
 *
 * Respeta EXDATE: las instancias excluidas (borradas «solo este» o editadas como
 * evento suelto aparte) no se pintan, evitando el duplicado con el override.
 *
 * ponytail: solo DAILY/WEEKLY/MONTHLY (lo que crea la UI); cota de 400 iters por
 * serie acota reglas infinitas. Upgrade: expandir en el backend (Go
 * QueryCalendarObjects) para ver series cuya 1ª instancia queda fuera de la vista.
 */
export function expandOccurrences(
  events: CalendarEvent[],
  rangeStart: Date,
  rangeEnd: Date,
): CalendarEvent[] {
  const out: CalendarEvent[] = [];
  for (const ev of events) {
    const rec = ev.ical ? parseRecurrence(ev.ical) : null;
    const start = parseIcalDate(ev.start);
    if (!rec || !start) {
      out.push(ev);
      continue;
    }
    const allDay = /^\d{8}$/.test(ev.start);
    const end = ev.end ? parseIcalDate(ev.end) : null;
    const durMs = end ? end.getTime() - start.getTime() : 0;
    const interval = rec.interval && rec.interval > 0 ? rec.interval : 1;
    const untilMs = rec.until ? endOfLocalDay(rec.until).getTime() : Infinity;
    const cap = rec.count && rec.count > 0 ? Math.min(rec.count, 400) : 400;
    // Instantes excluidos (EXDATE): la instancia se editó/borró «solo este».
    const excluded = new Set<number>();
    for (const tok of ev.ical ? parseExdates(ev.ical) : []) {
      const d = parseIcalDate(tok);
      if (d) excluded.add(d.getTime());
    }
    for (let i = 0; i < cap; i++) {
      const occ = occurrenceStart(start, i, rec.freq, interval);
      const occMs = occ.getTime();
      if (occMs > untilMs || occMs >= rangeEnd.getTime()) break; // monótono: nada más cabe
      if (excluded.has(occMs)) continue; // instancia excluida (EXDATE)
      if (occMs + durMs <= rangeStart.getTime()) continue; // termina antes de la ventana
      const occEnd = new Date(occMs + durMs);
      out.push({
        ...ev,
        start: allDay ? toIcalDateLocal(occ) : toIcalUtc(occ),
        ...(end ? { end: allDay ? toIcalDateLocal(occEnd) : toIcalUtc(occEnd) } : {}),
      });
    }
  }
  return out;
}

/** 'yyyy-mm-dd' → Date del final de ese día (hora local). */
function endOfLocalDay(dateStr: string): Date {
  const [y, m, d] = dateStr.split('-').map(Number);
  return new Date(y!, m! - 1, d, 23, 59, 59, 999);
}

/** Convierte una Date a value para <input type="datetime-local">. */
export function toDatetimeLocal(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/**
 * Cambia SOLO la fecha de un valor de datetime-local, conservando su hora. Es lo
 * que hace falta cuando el evento está en «todo el día»: el control es un
 * `<input type="date">` y solo emite 'YYYY-MM-DD', pero el formulario guarda
 * siempre la forma completa, porque un valor sin hora deja en BLANCO al
 * `<input type="datetime-local">` en cuanto se desactiva «todo el día».
 */
export function withLocalDate(prev: string, date: string): string {
  if (!date) return prev;
  return `${date}T${prev.slice(11, 16) || '00:00'}`;
}

/**
 * Fin que conserva la duración cuando el inicio se mueve de `from` a `to` (lo que
 * hacen Apple Calendar y Google Calendar al reprogramar). Devuelve el fin tal cual
 * si alguno de los tres valores no es una fecha-hora legible.
 */
export function shiftEnd(from: string, to: string, end: string): string {
  const before = Date.parse(from);
  const after = Date.parse(to);
  const finish = Date.parse(end);
  if (!Number.isFinite(before) || !Number.isFinite(after) || !Number.isFinite(finish)) return end;
  return toDatetimeLocal(new Date(finish + (after - before)));
}

function capitalize(s: string): string {
  return s.charAt(0).toUpperCase() + s.slice(1);
}
