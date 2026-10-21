/**
 * EventModal — crear/editar un evento con el design system Sytue (Modal + Form de
 * @sytue/ui). Guarda via BFF CalDAV (el servidor genera el .ics) en el calendario
 * destino elegido; al editar, cambiar de calendario mueve el evento. Permite
 * borrar eventos existentes. docs/design.md §9.
 */
import type { JSX } from 'preact';
import type { Locale } from '@sytue/i18n';
import { useEffect, useMemo, useState } from 'preact/hooks';
import {
  Button,
  FieldGroup,
  Icon,
  Input,
  Select,
  Textarea,
  Switch,
  Modal,
  useToast,
  describedBy,
} from '@sytue/ui';
import {
  saveEvent,
  deleteEvent,
  bookResource,
  createConference,
  type Calendar,
  type EventInput,
  type Recurrence,
  type RecurrenceScope,
} from '../client/calendarApi.js';
import { buildAlarm, buildRRule, parseReminders, REMINDER_PRESETS } from '../lib/ical.js';
import { applyRecurringEdit, applyRecurringDelete, type RecurTarget } from '../lib/recurrence.js';
import { ScopeDialog } from './ScopeDialog.js';
import { AsistenteEvento } from './AsistenteEvento.js';
import type { EventoPropuesto } from '../lib/ai.js';
import { ColorSwatches } from './CalendarModal.js';
import { FindTimePanel } from './FindTimePanel.js';
import { ResourcePicker, type BookingStatus } from './ResourcePicker.js';
import { shiftEnd, toDatetimeLocal, withLocalDate } from './dates.js';
import { browserZone, listTimeZones, zoneLabel, instantFromZonedWallClock } from './tz.js';
import type { T } from '../i18n/index.js';

type RepeatFreq = '' | Recurrence['freq'];
type RepeatEnd = 'never' | 'count' | 'until';

export interface EditingEvent {
  /** href existente (edicion) o null (creacion). */
  href: string | null;
  etag?: string | undefined;
  uid?: string | undefined;
  summary: string;
  start: Date;
  end?: Date | undefined;
  allDay?: boolean | undefined;
  /** Zona horaria IANA del evento (del DTSTART;TZID); por defecto la del navegador. */
  tzid?: string | undefined;
  location?: string | undefined;
  description?: string | undefined;
  recurrence?: Recurrence | undefined;
  /** Color del evento (hex #RRGGBB); vacio => hereda el del calendario. */
  color?: string | undefined;
  /** URL de la videollamada del evento (RFC 7986 CONFERENCE). */
  conferenceUrl?: string | undefined;
  /** Emails de los invitados existentes (para reabrir el modal). */
  invitees?: string[] | undefined;
  /** Bloques VALARM crudos del evento original (recordatorios + passthrough). */
  alarms?: string[] | undefined;
  /** URL del calendario al que pertenece el evento (edicion). */
  calendarUrl?: string | undefined;
  /** iCal crudo del maestro (para el recorte por alcance de una serie). */
  ical?: string | undefined;
  /** Solo lectura: el evento viene de una suscripción ICS externa. Se muestra en
   * modo consulta (campos deshabilitados, sin Guardar/Eliminar). */
  readOnly?: boolean | undefined;
}

interface Props {
  t: T;
  locale: Locale;
  open: boolean;
  /** Calendarios del usuario (selector de destino). */
  calendars: Calendar[];
  editing: EditingEvent | null;
  onClose: () => void;
  onSaved: () => void;
}

export function EventModal({
  t,
  locale,
  open,
  calendars,
  editing,
  onClose,
  onSaved,
}: Props): JSX.Element | null {
  const toast = useToast();
  // Destino por defecto: el calendario del evento (edicion) o el por defecto.
  const defaultUrl = calendars.find((c) => c.isDefault)?.url ?? calendars[0]?.url ?? '';
  const [calUrl, setCalUrl] = useState(editing?.calendarUrl ?? defaultUrl);
  const [summary, setSummary] = useState(editing?.summary ?? '');
  const [start, setStart] = useState(() => toDatetimeLocal(editing?.start ?? new Date()));
  const [end, setEnd] = useState(() =>
    toDatetimeLocal(editing?.end ?? editing?.start ?? new Date()),
  );
  const [allDay, setAllDay] = useState(Boolean(editing?.allDay));
  // Zona horaria del evento: por defecto la del navegador (zona de visualización).
  const [tzid, setTzid] = useState(editing?.tzid ?? browserZone());
  const [location, setLocation] = useState(editing?.location ?? '');
  // Videollamada del evento. La sala la crea Rooms y aquí solo se guarda su enlace:
  // el evento no «contiene» la reunión, la referencia (RFC 7986 CONFERENCE).
  const [conferenceUrl, setConferenceUrl] = useState(editing?.conferenceUrl ?? '');
  const [conferenceBusy, setConferenceBusy] = useState(false);
  // UID generado en esta sesión del modal, si hizo falta uno. Lo comparten la reserva
  // de recursos y la videollamada: son dos cosas que se ligan AL MISMO evento.
  const [draftUid, setDraftUid] = useState('');
  const [conferenceError, setConferenceError] = useState(false);
  const [description, setDescription] = useState(editing?.description ?? '');
  const [color, setColor] = useState(editing?.color ?? '');
  // Panel «buscar hueco» (asistente de programacion): plegado por defecto.
  const [findTimeOpen, setFindTimeOpen] = useState(false);
  const [repeatFreq, setRepeatFreq] = useState<RepeatFreq>(editing?.recurrence?.freq ?? '');
  const [repeatEnd, setRepeatEnd] = useState<RepeatEnd>(repeatEndOf(editing?.recurrence));
  const [repeatCount, setRepeatCount] = useState(String(editing?.recurrence?.count ?? 4));
  const [repeatUntil, setRepeatUntil] = useState(editing?.recurrence?.until ?? '');
  const [invitees, setInvitees] = useState<string[]>(editing?.invitees ?? []);
  const [inviteeDraft, setInviteeDraft] = useState('');
  const [inviteeError, setInviteeError] = useState<string | null>(null);
  // Salas y recursos a reservar (patrón resource-calendar): IDs elegidos + su estado
  // confirmado tras guardar (aceptada/rechazada). La disponibilidad en vivo y el anti
  // doble-reserva atómico los resuelve el servicio Go.
  const [resourceIds, setResourceIds] = useState<string[]>([]);
  const [bookingStatus, setBookingStatus] = useState<Record<string, BookingStatus>>({});
  // Recordatorios editables (minutos antes) + VALARM no editables (passthrough).
  const [reminders, setReminders] = useState<number[]>(
    () => parseReminders((editing?.alarms ?? []).join('\r\n')).minutes,
  );
  const [passthroughAlarms, setPassthroughAlarms] = useState<string[]>(
    () => parseReminders((editing?.alarms ?? []).join('\r\n')).passthrough,
  );
  // Alcance pendiente al editar/borrar una serie recurrente (this/following/all).
  const [scopePrompt, setScopePrompt] = useState<{
    op: 'edit' | 'delete';
    edited?: EventInput;
    rule?: Recurrence | undefined;
  } | null>(null);
  // Accion en curso: solo el boton pulsado muestra loading; el resto se
  // deshabilita para un estado ocupado coherente.
  const [busy, setBusy] = useState<false | 'save' | 'delete'>(false);
  const [summaryError, setSummaryError] = useState<string | null>(null);
  /**
   * Vuelca lo que propuso el asistente en el formulario. NO GUARDA: de eso se
   * encarga quien pulsa «Guardar» después de mirarlo.
   *
   * Lo que el asistente no diga NO SE TOCA. Si alguien ya tenía escrito el
   * lugar y la frase no lo menciona, borrarlo sería perder trabajo hecho a mano
   * por no haber sabido leerlo.
   */
  const aplicarPropuesta = (p: EventoPropuesto): void => {
    setSummary(p.titulo ?? summary);
    setSummaryError(null);
    if (p.inicio !== undefined) {
      const nuevoInicio = toDatetimeLocal(new Date(p.inicio));
      // El fin ARRASTRA al inicio conservando la duración, igual que al mover la
      // hora a mano: sin esto, mover el inicio al jueves deja el fin el martes.
      setEnd(
        p.fin !== undefined ? toDatetimeLocal(new Date(p.fin)) : shiftEnd(start, nuevoInicio, end),
      );
      setStart(nuevoInicio);
      setEndError(null);
    } else if (p.fin !== undefined) {
      setEnd(toDatetimeLocal(new Date(p.fin)));
      setEndError(null);
    }
    if (p.lugar !== undefined) setLocation(p.lugar);
    if (p.descripcion !== undefined) setDescription(p.descripcion);
  };

  const [endError, setEndError] = useState<string | null>(null);
  // Lista IANA (nativa) con etiqueta + offset actual; calculada una vez.
  const tzOptions = useMemo(
    () => listTimeZones().map((z) => ({ value: z, label: zoneLabel(z) })),
    [],
  );
  // Instantes reales (tz-correctos) del inicio/fin elegidos: la disponibilidad de
  // recursos y la reserva usan el MISMO instante que persiste el evento.
  const startInstant = useMemo(() => localInstant(start, tzid, allDay), [start, tzid, allDay]);
  const endInstant = useMemo(
    () => localInstant(end || start, tzid, allDay),
    [end, start, tzid, allDay],
  );

  // Al (re)abrir, sincroniza el formulario con el evento en edicion.
  useEffect(() => {
    if (!open) return;
    setCalUrl(editing?.calendarUrl ?? '');
    setSummary(editing?.summary ?? '');
    setStart(toDatetimeLocal(editing?.start ?? new Date()));
    setEnd(toDatetimeLocal(editing?.end ?? editing?.start ?? new Date()));
    setAllDay(Boolean(editing?.allDay));
    setTzid(editing?.tzid ?? browserZone());
    setLocation(editing?.location ?? '');
    setConferenceUrl(editing?.conferenceUrl ?? '');
    setConferenceError(false);
    setDescription(editing?.description ?? '');
    setColor(editing?.color ?? '');
    setFindTimeOpen(false);
    setRepeatFreq(editing?.recurrence?.freq ?? '');
    setRepeatEnd(repeatEndOf(editing?.recurrence));
    setRepeatCount(String(editing?.recurrence?.count ?? 4));
    setRepeatUntil(editing?.recurrence?.until ?? '');
    setInvitees(editing?.invitees ?? []);
    setInviteeDraft('');
    setInviteeError(null);
    setResourceIds([]);
    setBookingStatus({});
    setSummaryError(null);
    setEndError(null);
    const parsed = parseReminders((editing?.alarms ?? []).join('\r\n'));
    setReminders(parsed.minutes);
    setPassthroughAlarms(parsed.passthrough);
    setScopePrompt(null);
  }, [open, editing]);

  // Si los calendarios llegan con el modal ya abierto, fija el destino por
  // defecto sin resetear el resto del formulario.
  useEffect(() => {
    if (open && !calUrl && defaultUrl) setCalUrl(defaultUrl);
  }, [open, calUrl, defaultUrl]);

  const isEdit = Boolean(editing?.href);
  // Solo lectura (suscripción ICS): consulta sin edición. Un <fieldset disabled>
  // nativo deshabilita TODOS los controles del formulario de una vez.
  const readOnly = Boolean(editing?.readOnly);

  /** Añade un email a la lista de invitados (Enter/coma/blur). Devuelve el resto no consumido. */
  function addInvitee(raw: string): void {
    const email = raw
      .trim()
      .replace(/[,;]+$/, '')
      .trim();
    if (!email) return;
    if (!isEmail(email)) {
      setInviteeError(t('event.guestInvalid'));
      return;
    }
    if (!invitees.some((e) => e.toLowerCase() === email.toLowerCase())) {
      setInvitees((list) => [...list, email]);
    }
    setInviteeDraft('');
    setInviteeError(null);
  }

  function removeInvitee(email: string): void {
    setInvitees((list) => list.filter((e) => e !== email));
  }

  /**
   * Cambiar el inicio ARRASTRA el fin conservando la duración, como Apple Calendar
   * y Google Calendar. Sin esto, mover una reunión de las 10:00 a las 12:00 dejaba
   * el fin en las 11:00 y el evento se guardaba terminando antes de empezar: el
   * `min` del campo no valida un valor que ya estaba escrito, y aquí no hay envío
   * nativo de formulario que lo bloquee.
   */
  function changeStart(next: string): void {
    setEnd(shiftEnd(start, next, end));
    setStart(next);
    if (endError) setEndError(null);
  }

  function addReminder(): void {
    setReminders((list) => [...list, 10]);
  }
  function setReminderAt(i: number, minutes: number): void {
    setReminders((list) => list.map((m, j) => (j === i ? minutes : m)));
  }
  function removeReminderAt(i: number): void {
    setReminders((list) => list.filter((_, j) => j !== i));
  }

  /**
   * UID estable del evento: el de edición, el ya generado en esta sesión, o uno nuevo.
   * Que sea el mismo para todo lo que se ligue al evento es la condición de que la
   * videollamada y la reserva de recursos apunten al evento que se acaba guardando.
   */
  function ensureUid(): string {
    if (editing?.uid) return editing.uid;
    if (draftUid) return draftUid;
    const nuevo = `${crypto.randomUUID()}@calendar.sytue.eu`;
    setDraftUid(nuevo);
    return nuevo;
  }

  /**
   * Añade la videollamada. El UID tiene que ser el MISMO que se enviará al guardar,
   * porque es lo que liga la sala al evento: si aquí se generase uno y al guardar otro,
   * cada reapertura crearía una sala distinta y el enlace repartido dejaría de servir.
   */
  async function handleAddConference(): Promise<void> {
    if (conferenceBusy) return;
    setConferenceBusy(true);
    setConferenceError(false);
    try {
      const uid = ensureUid();
      const { url } = await createConference(
        uid,
        summary.trim() || t('event.conferenceDefaultSummary'),
      );
      setConferenceUrl(url);
    } catch {
      setConferenceError(true);
    } finally {
      setConferenceBusy(false);
    }
  }

  async function handleSave(): Promise<void> {
    if (!calUrl) {
      toast.notify({ tone: 'danger', title: t('toast.selectCalendar') });
      return;
    }
    // Validacion inline (no toast): error junto al campo, docs/design.md §5.3.
    if (!summary.trim()) {
      setSummaryError(t('event.titleRequired'));
      document.getElementById('ev-summary')?.focus();
      return;
    }
    // Fin anterior al inicio: el `min` del campo marca el valor como inválido pero
    // no impide guardarlo (esto no es un envío nativo), y el evento acababa en el
    // calendario con duración negativa, invisible en la rejilla.
    if (Date.parse(end) < Date.parse(start)) {
      setEndError(t('event.endBeforeStart'));
      document.getElementById('ev-end')?.focus();
      return;
    }
    // Consume un email tecleado pero no confirmado; aborta si es inválido.
    const draft = inviteeDraft.trim();
    if (draft && !isEmail(draft)) {
      setInviteeError(t('event.guestInvalid'));
      document.getElementById('ev-invitee')?.focus();
      return;
    }
    const finalInvitees =
      draft && !invitees.some((e) => e.toLowerCase() === draft.toLowerCase())
        ? [...invitees, draft]
        : invitees;
    const recurrence = buildRecurrence(repeatFreq, repeatEnd, repeatCount, repeatUntil);
    // VALARM = recordatorios editados (minutos antes) + los no editables preservados.
    const alarms = [...reminders.map((m) => buildAlarm(m, summary.trim())), ...passthroughAlarms];

    // Editar una INSTANCIA de serie recurrente: preguntar el alcance y aplicar el
    // split (this/following/all) en vez del PUT directo, para no corromper la serie.
    if (isEdit && editing?.recurrence) {
      const edited: EventInput = {
        summary: summary.trim(),
        start,
        allDay,
        ...(allDay ? {} : { tzid }),
        ...(end ? { end } : {}),
        ...(location ? { location } : {}),
        ...(description ? { description } : {}),
        ...(color ? { color } : {}),
        ...(finalInvitees.length ? { attendees: finalInvitees } : {}),
        ...(alarms.length ? { alarms } : {}),
      };
      setScopePrompt({ op: 'edit', edited, rule: recurrence });
      return;
    }

    setBusy('save');
    // UID del evento: al reservar recursos hace falta uno estable que enlace evento↔
    // reserva. En edición ya existe; en creación con recursos se genera aquí y se
    // envía en el .ics para que ambos compartan identidad.
    const bookingUid =
      editing?.uid ??
      (resourceIds.length > 0 || conferenceUrl || draftUid ? ensureUid() : undefined);
    const eventInput: EventInput = {
      summary: summary.trim(),
      start,
      allDay,
      ...(allDay ? {} : { tzid }),
      ...(bookingUid ? { uid: bookingUid } : {}),
      ...(end ? { end } : {}),
      ...(location ? { location } : {}),
      ...(description ? { description } : {}),
      ...(color ? { color } : {}),
      ...(conferenceUrl ? { conferenceUrl } : {}),
      ...(recurrence ? { recurrence } : {}),
      ...(finalInvitees.length ? { attendees: finalInvitees } : {}),
      ...(alarms.length ? { alarms } : {}),
    };
    // Mover de calendario = crear en el destino (mismo UID) y borrar el original.
    // Primero se crea: si el borrado falla queda un duplicado, nunca se pierde.
    const moveFrom =
      editing?.href && editing.calendarUrl && calUrl !== editing.calendarUrl ? editing.href : null;
    try {
      if (moveFrom) {
        await saveEvent({ calendarUrl: calUrl, eventInput });
        await deleteEvent(moveFrom);
      } else {
        await saveEvent({
          calendarUrl: calUrl,
          eventInput,
          ...(editing?.href ? { href: editing.href } : {}),
          ...(editing?.etag ? { etag: editing.etag } : {}),
        });
      }
      const bookingToast = await bookSelectedResources(bookingUid, recurrence);
      toast.notify(
        bookingToast ?? {
          tone: 'success',
          title: t(isEdit ? 'toast.eventUpdated' : 'toast.eventCreated'),
        },
      );
      onSaved();
      onClose();
    } catch {
      toast.notify({ tone: 'danger', title: t('cal.error.saveEvent') });
    } finally {
      setBusy(false);
    }
  }

  /**
   * Reserva los recursos elegidos para el evento recién guardado (por su UID). El
   * servicio Go comprueba la disponibilidad de forma ATÓMICA y responde aceptada/
   * rechazada; devuelve un toast de resumen si algo quedó ocupado, o null (éxito).
   */
  async function bookSelectedResources(
    uid: string | undefined,
    recurrence: Recurrence | undefined,
  ): Promise<{ tone: 'success' | 'warning'; title: string } | null> {
    if (!uid || resourceIds.length === 0) return null;
    const rrule = recurrence ? buildRRule(recurrence).replace(/^RRULE:/, '') : undefined;
    const statuses: Record<string, BookingStatus> = {};
    await Promise.all(
      resourceIds.map(async (id) => {
        try {
          const { status } = await bookResource(id, {
            uid,
            summary: summary.trim(),
            start: startInstant.toISOString(),
            end: endInstant.toISOString(),
            ...(allDay ? { all_day: true } : { tzid }),
            ...(rrule ? { rrule } : {}),
          });
          statuses[id] = status;
        } catch {
          statuses[id] = 'declined'; // conservador: sin confirmación, no dar por reservada
        }
      }),
    );
    setBookingStatus(statuses);
    const booked = resourceIds.filter((id) => statuses[id] === 'accepted').length;
    const busy = resourceIds.filter((id) => statuses[id] === 'declined').length;
    // Plantillas completas del catálogo, anidadas: la forma plural de «reservado»
    // y el orden de la frase no son los mismos en las cinco lenguas.
    const bookedLabel = t('booking.booked', { count: booked });
    if (busy > 0) {
      return {
        tone: 'warning',
        title: t('booking.savedBusy', {
          booked: bookedLabel,
          busy: t('booking.busy', { count: busy }),
        }),
      };
    }
    return { tone: 'success', title: t('booking.saved', { booked: bookedLabel }) };
  }

  async function handleDelete(): Promise<void> {
    if (!editing?.href) return;
    // Serie recurrente: preguntar el alcance del borrado (this/following/all).
    if (editing.recurrence) {
      setScopePrompt({ op: 'delete' });
      return;
    }
    setBusy('delete');
    try {
      await deleteEvent(editing.href);
      toast.notify({ tone: 'success', title: t('toast.eventDeleted') });
      onSaved();
      onClose();
    } catch {
      toast.notify({ tone: 'danger', title: t('cal.error.deleteEvent') });
    } finally {
      setBusy(false);
    }
  }

  /** Ejecuta la edición/borrado por alcance elegido en el ScopeDialog. */
  async function performScoped(scope: RecurrenceScope): Promise<void> {
    const prompt = scopePrompt;
    if (!prompt || !editing?.href) return;
    const cal = calendars.find((c) => c.url === editing.calendarUrl);
    const target: RecurTarget = {
      calendarId: cal?.id ?? '',
      calendarUrl: editing.calendarUrl ?? '',
      uid: editing.uid ?? '',
      href: editing.href,
      etag: editing.etag,
      masterIcal: editing.ical ?? '',
      recurrenceId: editing.start,
      allDay: Boolean(editing.allDay),
    };
    setBusy(prompt.op === 'delete' ? 'delete' : 'save');
    try {
      if (prompt.op === 'delete') await applyRecurringDelete(target, scope);
      else await applyRecurringEdit(target, scope, prompt.edited!, prompt.rule);
      toast.notify({
        tone: 'success',
        title: prompt.op === 'delete' ? t('toast.eventDeleted') : t('toast.eventUpdated'),
      });
      onSaved();
      onClose();
    } catch {
      toast.notify({
        tone: 'danger',
        title: prompt.op === 'delete' ? t('toast.eventDeleteFailed') : t('toast.eventSaveFailed'),
      });
    } finally {
      setBusy(false);
      setScopePrompt(null);
    }
  }

  return (
    <>
      <Modal
        open={open}
        onClose={onClose}
        title={
          readOnly ? t('event.viewTitle') : isEdit ? t('event.editTitle') : t('event.newTitle')
        }
        size="md"
        footer={
          readOnly ? (
            <Button variant="secondary" onClick={onClose}>
              {t('common.close')}
            </Button>
          ) : (
            <>
              {/* «Eliminar» a la izquierda en escritorio (`md:mr-auto`); en el teléfono,
                  sola arriba y la pareja Cancelar | Guardar debajo (el pie del Modal). */}
              {isEdit ? (
                <Button
                  variant="danger"
                  class="md:mr-auto"
                  onClick={() => void handleDelete()}
                  loading={busy === 'delete'}
                  disabled={busy !== false}
                >
                  {t('common.delete')}
                </Button>
              ) : null}
              <Button variant="secondary" onClick={onClose} disabled={busy !== false}>
                {t('common.cancel')}
              </Button>
              <Button
                variant="primary"
                onClick={() => void handleSave()}
                loading={busy === 'save'}
                disabled={busy !== false}
              >
                {t('common.save')}
              </Button>
            </>
          )
        }
      >
        {readOnly ? <p class="text-text-muted mb-3 text-sm">{t('event.readOnlyNotice')}</p> : null}
        {/* fieldset disabled: deshabilita nativamente todos los controles en modo consulta. */}
        <fieldset disabled={readOnly} class="m-0 flex min-w-0 flex-col gap-4 border-0 p-0">
          {/* El asistente va ARRIBA DEL TODO porque es el atajo al formulario
              entero, no un campo más: se escribe la frase, se rellenan cinco
              campos y se revisa hacia abajo. En consulta no hay nada que
              rellenar y no se pinta. */}
          {readOnly ? null : <AsistenteEvento t={t} onPropuesta={aplicarPropuesta} />}
          <FieldGroup
            label={t('event.title')}
            htmlFor="ev-summary"
            required
            {...(summaryError ? { error: summaryError } : {})}
          >
            <Input
              id="ev-summary"
              value={summary}
              placeholder={t('event.titlePlaceholder')}
              invalid={Boolean(summaryError)}
              aria-describedby={describedBy('ev-summary', undefined, summaryError ?? undefined)}
              onInput={(e) => {
                setSummary(e.currentTarget.value);
                if (summaryError) setSummaryError(null);
              }}
            />
          </FieldGroup>
          <FieldGroup label={t('event.calendar')} htmlFor="ev-calendar">
            <Select
              id="ev-calendar"
              value={calUrl}
              onChange={(e) => setCalUrl(e.currentTarget.value)}
            >
              {calendars.map((c) => (
                <option key={c.url} value={c.url}>
                  {c.displayName}
                </option>
              ))}
            </Select>
          </FieldGroup>
          <div class="border-border bg-bg-subtle rounded-md border px-3 py-2">
            <Switch
              checked={allDay}
              onChange={() => setAllDay((v) => !v)}
              label={t('event.allDay')}
            />
          </div>
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <FieldGroup label={t('event.start')} htmlFor="ev-start">
              <Input
                id="ev-start"
                type={allDay ? 'date' : 'datetime-local'}
                value={allDay ? start.slice(0, 10) : start}
                onInput={(e) =>
                  changeStart(
                    allDay ? withLocalDate(start, e.currentTarget.value) : e.currentTarget.value,
                  )
                }
              />
            </FieldGroup>
            <FieldGroup
              label={t('event.end')}
              htmlFor="ev-end"
              {...(endError ? { error: endError } : {})}
            >
              <Input
                id="ev-end"
                type={allDay ? 'date' : 'datetime-local'}
                value={allDay ? end.slice(0, 10) : end}
                // Restricción nativa: el selector no ofrece un fin anterior al inicio.
                min={allDay ? start.slice(0, 10) : start}
                invalid={Boolean(endError)}
                aria-describedby={describedBy('ev-end', undefined, endError ?? undefined)}
                onInput={(e) => {
                  setEnd(
                    allDay ? withLocalDate(end, e.currentTarget.value) : e.currentTarget.value,
                  );
                  if (endError) setEndError(null);
                }}
              />
            </FieldGroup>
          </div>
          {/* Zona horaria del evento: solo con hora (los de día completo son flotantes). */}
          {!allDay ? (
            <FieldGroup label={t('event.timeZone')} htmlFor="ev-tz">
              <Select id="ev-tz" value={tzid} onChange={(e) => setTzid(e.currentTarget.value)}>
                {tzOptions.map((o) => (
                  <option key={o.value} value={o.value}>
                    {o.label}
                  </option>
                ))}
              </Select>
            </FieldGroup>
          ) : null}
          {/* Buscar hueco (asistente de programacion): huecos libres comunes en la
              franja laboral; al elegir uno, fija el inicio/fin del evento. */}
          {!allDay ? (
            <div class="flex flex-col gap-2">
              {/* Discreto y a la izquierda, bajo las horas: es una ayuda para
                  elegirlas, no una acción del evento; como caja de ancho completo
                  parecía otro campo del formulario. */}
              <Button
                type="button"
                variant="ghost"
                size="sm"
                class="self-start"
                aria-expanded={findTimeOpen}
                leadingIcon={<Icon name="search" size={16} />}
                onClick={() => setFindTimeOpen((v) => !v)}
              >
                {findTimeOpen ? t('event.findTimeHide') : t('event.findTimeShow')}
              </Button>
              {findTimeOpen ? (
                <FindTimePanel
                  t={t}
                  locale={locale}
                  anchorStart={new Date(start)}
                  durationMin={durationMinutesOf(start, end)}
                  tzid={tzid}
                  onPick={(s, e) => {
                    setStart(toDatetimeLocal(s));
                    setEnd(toDatetimeLocal(e));
                    setFindTimeOpen(false);
                  }}
                />
              ) : null}
            </div>
          ) : null}
          <div class="grid grid-cols-1 gap-4 sm:grid-cols-2">
            <FieldGroup label={t('event.repeat')} htmlFor="ev-repeat">
              <Select
                id="ev-repeat"
                value={repeatFreq}
                onChange={(e) => setRepeatFreq(e.currentTarget.value as RepeatFreq)}
              >
                <option value="">{t('event.repeatNever')}</option>
                <option value="DAILY">{t('event.repeatDaily')}</option>
                <option value="WEEKLY">{t('event.repeatWeekly')}</option>
                <option value="MONTHLY">{t('event.repeatMonthly')}</option>
              </Select>
            </FieldGroup>
            {repeatFreq ? (
              <FieldGroup label={t('event.repeatEnds')} htmlFor="ev-repeat-end">
                <Select
                  id="ev-repeat-end"
                  value={repeatEnd}
                  onChange={(e) => setRepeatEnd(e.currentTarget.value as RepeatEnd)}
                >
                  <option value="never">{t('event.repeatEndNever')}</option>
                  <option value="count">{t('event.repeatEndCount')}</option>
                  <option value="until">{t('event.repeatEndUntil')}</option>
                </Select>
              </FieldGroup>
            ) : null}
          </div>
          {repeatFreq && repeatEnd === 'count' ? (
            <FieldGroup label={t('event.repeatCount')} htmlFor="ev-repeat-count">
              <Input
                id="ev-repeat-count"
                type="number"
                min={1}
                value={repeatCount}
                onInput={(e) => setRepeatCount(e.currentTarget.value)}
              />
            </FieldGroup>
          ) : null}
          {repeatFreq && repeatEnd === 'until' ? (
            <FieldGroup label={t('event.repeatUntil')} htmlFor="ev-repeat-until">
              <Input
                id="ev-repeat-until"
                type="date"
                min={start.slice(0, 10)}
                value={repeatUntil}
                onInput={(e) => setRepeatUntil(e.currentTarget.value)}
              />
            </FieldGroup>
          ) : null}
          <FieldGroup label={t('event.location')} htmlFor="ev-location">
            <Input
              id="ev-location"
              value={location}
              placeholder={t('event.locationPlaceholder')}
              onInput={(e) => setLocation(e.currentTarget.value)}
            />
          </FieldGroup>
          {/* Videollamada: una sala de Sytue Rooms ligada a ESTE evento. Se guarda en la
              propiedad CONFERENCE (RFC 7986), que es la que hace que Apple Calendar y
              Outlook pinten «Unirse» al sincronizar por CalDAV — en LOCATION la URL
              sería solo texto. Un clic, sin formularios ni elegir proveedor. */}
          <div class="flex flex-col gap-1.5">
            <span class="text-text text-sm font-semibold">{t('event.conference')}</span>
            {conferenceUrl ? (
              <div class="border-border-subtle flex items-center justify-between gap-2 rounded-md border px-3 py-2">
                <a
                  href={conferenceUrl}
                  target="_blank"
                  rel="noreferrer"
                  class="text-primary-text min-w-0 flex-1 truncate text-sm underline underline-offset-2"
                >
                  {conferenceUrl}
                </a>
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => setConferenceUrl('')}
                  aria-label={t('event.conferenceRemoveAria')}
                >
                  {t('common.remove')}
                </Button>
              </div>
            ) : (
              <div class="flex flex-col gap-1">
                <div>
                  <Button
                    size="sm"
                    variant="secondary"
                    onClick={() => void handleAddConference()}
                    disabled={conferenceBusy}
                  >
                    {conferenceBusy ? t('event.conferenceCreating') : t('event.conferenceAdd')}
                  </Button>
                </div>
                <span class="sytue-meta">{t('event.conferenceHint')}</span>
                {conferenceError ? (
                  <span role="alert" class="text-danger-text text-xs">
                    {t('event.conferenceError')}
                  </span>
                ) : null}
              </div>
            )}
          </div>
          <ResourcePicker
            t={t}
            titulo={t('event.resources')}
            start={startInstant}
            end={endInstant}
            selected={resourceIds}
            onChange={setResourceIds}
            status={bookingStatus}
          />
          <div class="flex flex-col gap-1.5">
            <span id="ev-color-label" class="text-text text-sm font-semibold">
              {t('common.colour')}
            </span>
            <ColorSwatches
              t={t}
              value={color}
              onChange={setColor}
              idPrefix="ev-color"
              allowNone
              labelledBy="ev-color-label"
            />
          </div>
          <FieldGroup
            label={t('event.guests')}
            htmlFor="ev-invitee"
            {...(inviteeError ? { error: inviteeError } : {})}
          >
            {invitees.length > 0 ? (
              <ul class="mb-2 flex flex-wrap gap-1.5" aria-label={t('event.guestsAdded')}>
                {invitees.map((email) => (
                  <li
                    key={email}
                    class="rounded-pill border-border bg-bg-subtle text-text inline-flex items-center gap-1 border py-0.5 pr-1 pl-2.5 text-sm"
                  >
                    <span class="max-w-[16rem] truncate">{email}</span>
                    <button
                      type="button"
                      aria-label={t('event.guestRemove', { email })}
                      onClick={() => removeInvitee(email)}
                      class="text-text-muted hover:bg-danger-bg hover:text-danger-text sytue-focus-ring rounded-control grid h-5 w-5 place-items-center transition-colors"
                    >
                      <Icon name="close" size={14} />
                    </button>
                  </li>
                ))}
              </ul>
            ) : null}
            <Input
              id="ev-invitee"
              type="email"
              value={inviteeDraft}
              placeholder={t('event.guestPlaceholder')}
              invalid={Boolean(inviteeError)}
              aria-describedby={describedBy('ev-invitee', undefined, inviteeError ?? undefined)}
              onInput={(e) => {
                setInviteeDraft(e.currentTarget.value);
                if (inviteeError) setInviteeError(null);
              }}
              onKeyDown={(e) => {
                if (e.key === 'Enter' || e.key === ',') {
                  e.preventDefault();
                  addInvitee(inviteeDraft);
                }
              }}
              onBlur={() => addInvitee(inviteeDraft)}
            />
          </FieldGroup>
          <FieldGroup label={t('event.description')} htmlFor="ev-description">
            <Textarea
              id="ev-description"
              rows={4}
              value={description}
              placeholder={t('event.descriptionPlaceholder')}
              onInput={(e) => setDescription(e.currentTarget.value)}
            />
          </FieldGroup>
          <FieldGroup label={t('event.reminders')} htmlFor="ev-reminder-add">
            {reminders.length > 0 ? (
              <ul class="mb-2 flex flex-col gap-2" aria-label={t('event.remindersList')}>
                {reminders.map((minutes, i) => (
                  <li key={i} class="flex items-center gap-2">
                    <ReminderRow
                      t={t}
                      minutes={minutes}
                      index={i}
                      onChange={(m) => setReminderAt(i, m)}
                      onRemove={() => removeReminderAt(i)}
                    />
                  </li>
                ))}
              </ul>
            ) : (
              <p class="text-text-subtle mb-2 text-sm">{t('event.remindersNone')}</p>
            )}
            <Button
              id="ev-reminder-add"
              type="button"
              variant="secondary"
              size="sm"
              leadingIcon={<Icon name="plus" size={16} />}
              onClick={addReminder}
            >
              {t('event.reminderAdd')}
            </Button>
          </FieldGroup>
        </fieldset>
      </Modal>
      <ScopeDialog
        t={t}
        open={scopePrompt !== null}
        op={scopePrompt?.op ?? 'edit'}
        busy={busy !== false}
        onConfirm={(scope) => void performScoped(scope)}
        onClose={() => setScopePrompt(null)}
      />
    </>
  );
}

const UNIT_FACTOR = { minutes: 1, hours: 60, days: 1440 } as const;
type ReminderUnit = keyof typeof UNIT_FACTOR;

/** ¿`m` minutos coincide con un preset (para mostrar el selector, no el editor)? */
function isPreset(m: number): boolean {
  return REMINDER_PRESETS.includes(m);
}

function reminderLabel(t: T, minutes: number): string {
  if (minutes === 0) return t('reminder.atTime');
  if (minutes % 10080 === 0) return t('reminder.weeks', { count: minutes / 10080 });
  if (minutes % 1440 === 0) return t('reminder.days', { count: minutes / 1440 });
  if (minutes % 60 === 0) return t('reminder.hours', { count: minutes / 60 });
  return t('reminder.minutes', { count: minutes });
}

/** Descompone minutos en {cantidad, unidad} para el editor personalizado. */
function decompose(m: number): { value: number; unit: ReminderUnit } {
  if (m > 0 && m % 1440 === 0) return { value: m / 1440, unit: 'days' };
  if (m > 0 && m % 60 === 0) return { value: m / 60, unit: 'hours' };
  return { value: m, unit: 'minutes' };
}

/**
 * Fila de un recordatorio: preset o personalizado (cantidad + unidad) + quitar.
 *
 * Cada control va en su `<div>` con las clases de la fila: `Select` pone su `class` en
 * el `<select>` y no en el envoltorio que participa en el flex, y `Input` suma `w-full`
 * (que gana a `w-20`). Con las clases puestas en el control, el personalizado aplastaba
 * el selector a 44 px y sacaba del diálogo la unidad y el aspa de quitar.
 *
 * En el teléfono, el personalizado parte en dos líneas: el selector entero arriba y
 * cantidad, unidad y quitar debajo. En una sola le quedaban 4 px a «Personalizado».
 */
function ReminderRow({
  t,
  minutes,
  index,
  onChange,
  onRemove,
}: {
  t: T;
  minutes: number;
  index: number;
  onChange: (m: number) => void;
  onRemove: () => void;
}): JSX.Element {
  const preset = isPreset(minutes);
  const { value, unit } = decompose(minutes);
  return (
    <div class="flex flex-1 flex-wrap items-center gap-2">
      <div class={preset ? 'min-w-0 flex-1' : 'min-w-0 flex-1 max-md:basis-full'}>
        <Select
          aria-label={t('event.reminderSelect', { index: index + 1 })}
          value={preset ? String(minutes) : 'custom'}
          onChange={(e) => {
            const v = e.currentTarget.value;
            onChange(v === 'custom' ? (preset ? 45 : minutes) : Number(v));
          }}
        >
          {REMINDER_PRESETS.map((presetMinutes) => (
            <option key={presetMinutes} value={String(presetMinutes)}>
              {reminderLabel(t, presetMinutes)}
            </option>
          ))}
          <option value="custom">{t('event.reminderCustom')}</option>
        </Select>
      </div>
      {!preset ? (
        <>
          <div class="w-20 shrink-0">
            <Input
              type="number"
              min={0}
              aria-label={t('event.reminderAmount', { index: index + 1 })}
              value={String(value)}
              onInput={(e) =>
                onChange(Math.max(0, Number(e.currentTarget.value) || 0) * UNIT_FACTOR[unit])
              }
            />
          </div>
          <div class="shrink-0 max-md:min-w-0 max-md:flex-1">
            <Select
              aria-label={t('event.reminderUnit', { index: index + 1 })}
              value={unit}
              onChange={(e) => onChange(value * UNIT_FACTOR[e.currentTarget.value as ReminderUnit])}
            >
              <option value="minutes">{t('event.unitMinutes')}</option>
              <option value="hours">{t('event.unitHours')}</option>
              <option value="days">{t('event.unitDays')}</option>
            </Select>
          </div>
        </>
      ) : null}
      <button
        type="button"
        aria-label={t('event.reminderRemove', { index: index + 1 })}
        onClick={onRemove}
        class="text-text-muted hover:bg-danger-bg hover:text-danger-text sytue-focus-ring rounded-control grid h-8 w-8 shrink-0 place-items-center transition-colors pointer-coarse:h-11 pointer-coarse:w-11"
      >
        <Icon name="close" size={16} />
      </button>
    </div>
  );
}

/** Validacion mínima de email (no RFC completa; el backend valida de verdad). */
function isEmail(v: string): boolean {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(v);
}

/**
 * Convierte una fecha-hora local del formulario (datetime-local o date, hora-de-pared
 * en `tzid`) al instante real (UTC). Para eventos de día completo (flotantes) usa las
 * 00:00 UTC del día. Es el MISMO instante que persiste el evento, así la
 * disponibilidad y la reserva de recursos cuadran con lo que se guarda.
 */
function localInstant(local: string, tzid: string, allDay: boolean): Date {
  const m = local.match(/^(\d{4})-(\d{2})-(\d{2})(?:T(\d{2}):(\d{2}))?/);
  if (!m) return new Date(local);
  const [, y, mo, d, h, mi] = m;
  if (allDay) return new Date(Date.UTC(Number(y), Number(mo) - 1, Number(d)));
  return instantFromZonedWallClock(
    Number(y),
    Number(mo),
    Number(d),
    Number(h ?? '0'),
    Number(mi ?? '0'),
    0,
    tzid,
  );
}

/** Minutos entre dos datetime-local; 60 por defecto si falta el fin o es <= inicio. */
function durationMinutesOf(start: string, end: string): number {
  const min = (new Date(end).getTime() - new Date(start).getTime()) / 60_000;
  return Number.isFinite(min) && min >= 15 ? Math.round(min) : 60;
}

/** Deriva el modo de fin del selector desde una Recurrence existente. */
function repeatEndOf(rec: Recurrence | undefined): RepeatEnd {
  if (rec?.count) return 'count';
  if (rec?.until) return 'until';
  return 'never';
}

/** Ensambla la Recurrence a enviar, o undefined si el evento no se repite. */
function buildRecurrence(
  freq: RepeatFreq,
  end: RepeatEnd,
  count: string,
  until: string,
): Recurrence | undefined {
  if (!freq) return undefined;
  if (end === 'count') return { freq, count: Math.max(1, parseInt(count, 10) || 1) };
  if (end === 'until' && until) return { freq, until };
  return { freq };
}
