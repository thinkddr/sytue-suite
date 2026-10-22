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
import { occurrenceStart, parseIcalDate, toDatetimeLocal } from '../components/dates.js';
import { parseExdates } from './ical.js';
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
  const editedStartMs = new Date(edited.start).getTime();
  const deltaMs = masterStart ? editedStartMs - t.recurrenceId.getTime() : 0;
  const durationMs = edited.end ? new Date(edited.end).getTime() - editedStartMs : 0;
  const newStart = masterStart ? new Date(masterStart.getTime() + deltaMs) : new Date(edited.start);

  const eventInput: EventInput = { ...contentCopy(edited), uid: t.uid };
  if (rule) eventInput.recurrence = rule;
  eventInput.start = toDatetimeLocal(newStart);
  if (edited.end) eventInput.end = toDatetimeLocal(new Date(newStart.getTime() + durationMs));
  await saveEvent({
    calendarUrl: t.calendarUrl,
    eventInput,
    href: t.href,
    ...(t.etag ? { etag: t.etag } : {}),
  });
}

// --- Helpers -----------------------------------------------------------------

/** Copia los campos de CONTENIDO para un objeto independiente (sin uid/recurrencia). */
function contentCopy(e: EventInput): EventInput {
  const out: EventInput = { summary: e.summary, start: e.start };
  if (e.end !== undefined) out.end = e.end;
  if (e.allDay !== undefined) out.allDay = e.allDay;
  if (e.location !== undefined) out.location = e.location;
  if (e.description !== undefined) out.description = e.description;
  if (e.attendees !== undefined) out.attendees = e.attendees;
  if (e.alarms !== undefined) out.alarms = e.alarms;
  return out;
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
  let n = 0;
  for (let i = 0; i < cap; i++) {
    if (occurrenceStart(start, i, rec.freq, interval).getTime() >= boundary.getTime()) break;
    n++;
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
