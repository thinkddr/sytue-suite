/**
 * Edición/borrado por ALCANCE de una instancia de serie recurrente («solo este» /
 * «este y los siguientes» / «todos»), sobre el mismo PUT CalDAV + el endpoint de
 * recorte del maestro (amend-recurrence). El maestro se recorta en el backend
 * (EXDATE o UNTIL, reutilizando su motor RRULE y preservando VALARM/EXDATE/
 * overrides); la instancia/serie nueva se crea aquí como objeto independiente.
 *
 * Orden de operaciones: crear SIEMPRE antes de recortar el maestro, de modo que
 * un fallo a mitad deje —como mucho— un duplicado, nunca una pérdida (mismo
 * criterio que mover un evento entre calendarios).
 */
import { addDays, occurrenceStart, parseIcalDate, toDatetimeLocal } from '../components/dates.js';
import { parseExdates, WEEKDAYS } from './ical.js';
import {
  amendRecurrence,
  deleteEvent,
  saveEvent,
  type EventInput,
  type Recurrence,
  type RecurrenceScope,
} from '../client/calendarApi.js';

/** Datos del maestro + la instancia objetivo para una edición por alcance. */
export interface RecurTarget {
  /** ID REST del calendario (para amend-recurrence). */
  calendarId: string;
  /** URL de la colección CalDAV (para crear/editar objetos). */
  calendarUrl: string;
  /** UID del maestro. */
  uid: string;
  /** href del objeto maestro (para editar/borrar «todos»). */
  href: string;
  etag?: string | undefined;
  /** iCal del maestro (RRULE + EXDATE previos). */
  masterIcal: string;
  /** Inicio ORIGINAL de la instancia objetivo (recurrence-id). */
  recurrenceId: Date;
  allDay: boolean;
}

/**
 * Aplica una edición de la instancia según el alcance elegido. `rule` es la regla
 * (posiblemente editada) del formulario: se aplica a «todos» y a la serie nueva de
 * «este y siguientes»; undefined = el usuario quitó la recurrencia.
 */
export async function applyRecurringEdit(
  t: RecurTarget,
  scope: RecurrenceScope,
  edited: EventInput,
  rule: Recurrence | undefined,
): Promise<void> {
  if (scope === 'all') return editAll(t, edited, rule);
  if (scope === 'following') return editFollowing(t, edited, rule);
  return editThis(t, edited);
}

/** Aplica un borrado de la instancia según el alcance elegido. */
export async function applyRecurringDelete(t: RecurTarget, scope: RecurrenceScope): Promise<void> {
  if (scope === 'all') {
    await deleteEvent(t.href);
    return;
  }
  await amendRecurrence({
    calendarId: t.calendarId,
    uid: t.uid,
    recurrenceId: recurrenceIso(t),
    mode: scope === 'following' ? 'until' : 'exclude',
    ...(t.etag ? { etag: t.etag } : {}),
  });
}

// --- «Solo este» -------------------------------------------------------------

async function editThis(t: RecurTarget, edited: EventInput): Promise<void> {
  // 1) Evento suelto editado (uid nuevo, sin recurrencia). 2) EXDATE en el maestro.
  await saveEvent({ calendarUrl: t.calendarUrl, eventInput: contentCopy(edited) });
  await amendRecurrence({
    calendarId: t.calendarId,
    uid: t.uid,
    recurrenceId: recurrenceIso(t),
    mode: 'exclude',
    ...(t.etag ? { etag: t.etag } : {}),
  });
}

// --- «Este y los siguientes» -------------------------------------------------

async function editFollowing(
  t: RecurTarget,
  edited: EventInput,
  rule: Recurrence | undefined,
): Promise<void> {
  const base = contentCopy(edited);
  if (rule) base.recurrence = carryRule(rule, masterDtstart(t.masterIcal), t.recurrenceId);
  // Regla que el formulario no sabe editar (BYMONTH, 2MO…): pasa tal cual. Con COUNT
  // la serie nueva saldría con más repeticiones, y por eso la interfaz no ofrece este
  // alcance en ese caso (`permiteSiguientes`).
  else if (edited.rruleRaw) base.rruleRaw = edited.rruleRaw;
  const carried = carriedExdates(t.masterIcal, t.recurrenceId);
  if (carried.length) base.exdates = carried;
  // 1) Serie nueva desde esta instancia. 2) UNTIL en el maestro (justo antes).
  await saveEvent({ calendarUrl: t.calendarUrl, eventInput: base });
  await amendRecurrence({
    calendarId: t.calendarId,
    uid: t.uid,
    recurrenceId: recurrenceIso(t),
    mode: 'until',
    ...(t.etag ? { etag: t.etag } : {}),
  });
}

/** Regla arrastrada a la serie nueva: mismo patrón, fin ajustado (COUNT/UNTIL). */
export function carryRule(
  masterRec: Recurrence,
  masterStart: Date | null,
  boundary: Date,
): Recurrence {
  const rec: Recurrence = { freq: masterRec.freq };
  if (masterRec.interval && masterRec.interval > 1) rec.interval = masterRec.interval;
  // Los días de la semana son el patrón: «lunes y miércoles» sigue siéndolo en la nueva.
  if (masterRec.byday?.length) rec.byday = [...masterRec.byday];
  if (masterRec.count && masterRec.count > 0) {
    const kept = masterStart ? countBefore(masterStart, masterRec, boundary) : 0;
    rec.count = Math.max(1, masterRec.count - kept);
  } else if (masterRec.until) {
    rec.until = masterRec.until;
  }
  return rec;
}

// --- «Todos» -----------------------------------------------------------------

async function editAll(
  t: RecurTarget,
  edited: EventInput,
  rule: Recurrence | undefined,
): Promise<void> {
  // Un cambio de hora en «todos» DESPLAZA la serie: se aplica el mismo delta al
  // DTSTART del maestro (no se ancla la serie a esta instancia). Sin cambio de
  // hora, delta = 0 y el maestro conserva su inicio.
  const masterStart = masterDtstart(t.masterIcal);
  const editedStart = new Date(edited.start);
  const editedEnd = edited.end ? new Date(edited.end) : undefined;
  let newStart: Date;
  let newEnd: Date | undefined;
  if (edited.allDay) {
    // En día completo el desfase y la duración van en DÍAS de calendario: en
    // milisegundos, un maestro que cae en un cambio de hora acababa a las 23:00 del día
    // anterior y con el DTEND (exclusivo) igual al DTSTART.
    const base = masterStart ?? editedStart;
    newStart = addDays(base, masterStart ? diasEntre(t.recurrenceId, editedStart) : 0);
    newEnd = editedEnd ? addDays(newStart, diasEntre(editedStart, editedEnd)) : undefined;
  } else {
    const deltaMs = masterStart ? editedStart.getTime() - t.recurrenceId.getTime() : 0;
    newStart = masterStart ? new Date(masterStart.getTime() + deltaMs) : editedStart;
    newEnd = editedEnd
      ? new Date(newStart.getTime() + editedEnd.getTime() - editedStart.getTime())
      : undefined;
  }

  const eventInput: EventInput = { ...contentCopy(edited), uid: t.uid };
  if (rule) eventInput.recurrence = rule;
  else if (edited.rruleRaw) eventInput.rruleRaw = edited.rruleRaw;
  // Es el mismo recurso: sin tocar la repetición, el BFF conserva la RRULE guardada.
  if (edited.recurrenceUnchanged) eventInput.recurrenceUnchanged = true;
  eventInput.start = toDatetimeLocal(newStart);
  if (newEnd) eventInput.end = toDatetimeLocal(newEnd);
  await saveEvent({
    calendarUrl: t.calendarUrl,
    eventInput,
    href: t.href,
    ...(t.etag ? { etag: t.etag } : {}),
  });
}

// --- Helpers -----------------------------------------------------------------

/** Días de calendario (locales) de `a` a `b`, sin la hora ni los cambios de hora. */
function diasEntre(a: Date, b: Date): number {
  const dia = (d: Date) => Date.UTC(d.getFullYear(), d.getMonth(), d.getDate());
  return Math.round((dia(b) - dia(a)) / 86_400_000);
}

/** Copia los campos de CONTENIDO para un objeto independiente (sin uid/recurrencia). */
function contentCopy(e: EventInput): EventInput {
  const out: EventInput = { summary: e.summary, start: e.start };
  if (e.end !== undefined) out.end = e.end;
  if (e.allDay !== undefined) out.allDay = e.allDay;
  if (e.location !== undefined) out.location = e.location;
  if (e.description !== undefined) out.description = e.description;
  if (e.color !== undefined) out.color = e.color;
  if (e.conferenceUrl !== undefined) out.conferenceUrl = e.conferenceUrl;
  if (e.attendees !== undefined) out.attendees = e.attendees;
  if (e.partstats !== undefined) out.partstats = e.partstats;
  if (e.alarms !== undefined) out.alarms = e.alarms;
  return out;
}

/**
 * «Este y los siguientes» se puede ofrecer: con una regla que la interfaz no sabe
 * contar (BY*) y un fin por COUNT, la serie nueva no sabría cuántas le quedan.
 */
export function permiteSiguientes(rruleRaw: string | undefined): boolean {
  return !rruleRaw || !/(?:^|;)COUNT=/i.test(rruleRaw);
}

/** recurrence-id como RFC3339 para el backend. Para todo el día, medianoche UTC
 *  del día (para que el backend derive la fecha correcta sin sesgo de zona). */
function recurrenceIso(t: RecurTarget): string {
  const d = t.recurrenceId;
  if (t.allDay) {
    return new Date(Date.UTC(d.getFullYear(), d.getMonth(), d.getDate())).toISOString();
  }
  return d.toISOString();
}

/** DTSTART del maestro (primer DTSTART del recurso). */
function masterDtstart(ical: string): Date | null {
  const m = ical.match(/^DTSTART[^:\r\n]*:(.+)$/im);
  return m ? parseIcalDate(m[1]!.trim()) : null;
}

/** Nº de ocurrencias del maestro con inicio anterior a `boundary` (para COUNT).
 *  Acotado por el COUNT del maestro (no cuenta más allá de las que existen). */
export function countBefore(start: Date, rec: Recurrence, boundary: Date): number {
  const interval = rec.interval && rec.interval > 0 ? rec.interval : 1;
  const cap = rec.count && rec.count > 0 ? rec.count : 1000;
  if (rec.freq === 'WEEKLY' && rec.byday?.length) {
    return countBeforeByDay(start, rec.byday, interval, cap, boundary);
  }
  let n = 0;
  for (let i = 0; i < cap; i++) {
    if (occurrenceStart(start, i, rec.freq, interval).getTime() >= boundary.getTime()) break;
    n++;
  }
  return n;
}

/**
 * `countBefore` de una semanal con BYDAY, como la expande el servicio (`instancesIn`,
 * semana de lunes a domingo): cada `interval` semanas desde la del inicio, los días
 * elegidos en orden, sin contar los anteriores al inicio. Sin esto, «lunes y miércoles,
 * 10 veces» partido en la tercera semana dejaba a la serie nueva con 8 en vez de 6.
 */
function countBeforeByDay(
  start: Date,
  byday: readonly string[],
  interval: number,
  cap: number,
  boundary: Date,
): number {
  const offsets = WEEKDAYS.flatMap((d, i) => (byday.includes(d) ? [i] : []));
  if (!offsets.length) return 0;
  const monday = addDays(start, -((start.getDay() + 6) % 7));
  let n = 0;
  for (let week = 0; n < cap; week += interval) {
    for (const off of offsets) {
      const t = addDays(monday, week * 7 + off);
      if (t.getTime() < start.getTime()) continue;
      if (t.getTime() >= boundary.getTime() || n >= cap) return n;
      n++;
    }
  }
  return n;
}

/** EXDATE del maestro que caen en/después del corte (se arrastran a la serie nueva). */
export function carriedExdates(ical: string, boundary: Date): string[] {
  return parseExdates(ical).filter((tok) => {
    const d = parseIcalDate(tok);
    return d !== null && d.getTime() >= boundary.getTime();
  });
}
