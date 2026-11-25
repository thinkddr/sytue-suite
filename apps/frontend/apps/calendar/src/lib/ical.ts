/**
 * Construccion de cuerpos iCalendar (VEVENT) para crear/editar eventos via
 * CalDAV PUT. El SDK @sytue/api-clients/caldav parsea VEVENT; aqui generamos el
 * VCALENDAR de salida. Formato de fechas: UTC basico (yyyymmddThhmmssZ) o DATE
 * para todo el dia. Minimo viable RFC 5545.
 */
import { instantFromZonedWallClock, zoneOffsetMinutes } from '../components/tz.js';

/** Día de la semana de un BYDAY (RFC 5545 3.3.10), sin ordinal. */
export type Weekday = 'MO' | 'TU' | 'WE' | 'TH' | 'FR' | 'SA' | 'SU';

/** Los siete, de lunes a domingo: el orden de las columnas y de `weekdayNarrowLabels`. */
export const WEEKDAYS: readonly Weekday[] = ['MO', 'TU', 'WE', 'TH', 'FR', 'SA', 'SU'];

/** «Cada día laborable»: de lunes a viernes, como lo guarda Google Calendar. */
export const WORKWEEK: readonly Weekday[] = ['MO', 'TU', 'WE', 'TH', 'FR'];

/**
 * Recurrencia iCalendar (RRULE) — subconjunto que expone la UI: FREQ diaria/
 * semanal/mensual/anual, INTERVAL, los días de una semanal (`byday`: «lunes y
 * miércoles», «cada día laborable») y un fin por COUNT o UNTIL (mutuamente
 * excluyentes, como exige el backend Go). `until` es 'yyyy-mm-dd' (valor de
 * <input type=date>).
 */
export interface Recurrence {
  freq: 'DAILY' | 'WEEKLY' | 'MONTHLY' | 'YEARLY';
  interval?: number | undefined;
  /** Solo con WEEKLY: los días de la semana en que cae, de lunes a domingo. */
  byday?: Weekday[] | undefined;
  count?: number | undefined;
  until?: string | undefined;
}

/**
 * Los días válidos de `days`, sin repetir y de lunes a domingo. Ordenarlos hace que
 * «miércoles y lunes» y «lunes y miércoles» sean la misma regla, y filtrar impide que
 * un valor ajeno (viene del navegador) se cuele en la línea RRULE.
 */
export function normalizeWeekdays(days: readonly string[]): Weekday[] {
  return WEEKDAYS.filter((d) => days.includes(d));
}

/** Cómo cuenta un evento en la disponibilidad: ocupado (lo de siempre) o disponible. */
export type Transp = 'OPAQUE' | 'TRANSPARENT';

export interface EventInput {
  uid: string;
  summary: string;
  /** ISO 8601 (local datetime-local del formulario). */
  start: string;
  end?: string | undefined;
  allDay?: boolean | undefined;
  /**
   * Zona horaria IANA del evento (p. ej. «Europe/Madrid»). El start/end del
   * formulario es la hora-de-pared EN ESTA ZONA; se emite DTSTART;TZID=… con esa
   * hora local (sin convertir a UTC), para que el backend expanda con DST
   * correcto y el TZID haga round-trip. Ausente o «UTC» => DTSTART UTC (…Z).
   */
  tzid?: string | undefined;
  location?: string | undefined;
  description?: string | undefined;
  recurrence?: Recurrence | undefined;
  /**
   * Color del evento (hex #RRGGBB de la paleta de la suite). Se emite como
   * X-APPLE-CALENDAR-COLOR (extension estandar, round-trip CalDAV). Vacio =>
   * el evento hereda el color de su calendario.
   */
  color?: string | undefined;
  /**
   * URL de la videollamada (RFC 7986 CONFERENCE). Es la propiedad que hace que Apple
   * Calendar y Outlook pinten «Unirse»; en LOCATION sería solo texto.
   */
  conferenceUrl?: string | undefined;
  /** Emails de los invitados (ATTENDEE). Se emiten con RSVP=TRUE. */
  attendees?: string[] | undefined;
  /** Email del organizador (ORGANIZER). Normalmente, el usuario que crea. */
  organizer?: string | undefined;
  /**
   * Respuesta YA DADA de cada invitado, por email en minúsculas (lo que devuelve
   * `guestPartstats`). Sin ella, cada guardado volvía a poner a todos en
   * NEEDS-ACTION: mover una reunión media hora borraba quién había dicho que sí.
   * Los que no están aquí —los recién añadidos— salen a la espera.
   */
  partstats?: Record<string, string> | undefined;
  /**
   * Bloques VALARM crudos (BEGIN:VALARM..END:VALARM) del evento: los que la UI
   * edita (recordatorios, ver buildAlarm) más los que conserva verbatim de un
   * cliente CalDAV externo (email/absolutos). Nunca inventados en creacion.
   */
  alarms?: string[] | undefined;
  /**
   * EXDATE a emitir (tokens iCal ya formateados: yyyymmddThhmmssZ, o yyyymmdd si
   * allDay). Se usan al desgajar una serie recurrente («este y siguientes»): las
   * excepciones posteriores al corte se arrastran a la serie nueva.
   */
  exdates?: string[] | undefined;
  /**
   * La regla de repetición del evento tal cual, sin «RRULE:» (`FREQ=WEEKLY;BYDAY=MO,WE`),
   * cuando el formulario no sabe editarla (`parseRecurrence` → null). Viaja para que una
   * serie de Apple, Google o un .ics importado no se quede en un evento suelto al
   * guardarla. `recurrence` manda si vienen las dos.
   */
  rruleRaw?: string | undefined;
  /**
   * «Mostrar como» (TRANSP, RFC 5545 §3.8.2.7): 'TRANSPARENT' es «disponible» y el
   * evento no cuenta como ocupado en free/busy ni en «buscar hueco» (los dos los
   * calcula el servicio con `CountsAsBusy`). Sin valor, el guardado no toca el TRANSP
   * que traiga el recurso: un arrastre no sabe de esto y no debe borrarlo.
   */
  transp?: Transp | undefined;
  /**
   * El usuario no ha tocado la repetición. Al editar un recurso que ya existe, el BFF
   * conserva entonces la RRULE guardada byte a byte; sin esto, que no venga
   * `recurrence` quiere decir que la ha quitado.
   */
  recurrenceUnchanged?: boolean | undefined;
}

/**
 * Recordatorios que ofrece la UI: minutos ANTES del inicio (0 = en el momento).
 * Aquí solo viven las CANTIDADES; la etiqueta («10 minutos antes» / «10 Minuten
 * vorher») la compone la UI desde el catálogo con la forma plural que toque, y
 * por eso este módulo —que genera iCalendar— no tiene texto de interfaz.
 */
export const REMINDER_PRESETS: readonly number[] = [0, 5, 10, 15, 30, 60, 120, 1440, 2880, 10080];

/** Convierte ISO/datetime-local a formato iCalendar UTC. */
function toIcalUtc(iso: string): string {
  const d = new Date(iso);
  return d
    .toISOString()
    .replace(/[-:]/g, '')
    .replace(/\.\d{3}Z$/, 'Z');
}

/**
 * Reformatea un datetime-local ('yyyy-mm-ddThh:mm[:ss]') a la forma DATE-TIME
 * FLOTANTE de iCalendar ('yyyymmddThhmmss') SIN convertir a UTC: es la
 * hora-de-pared que acompaña a un DTSTART;TZID=. Fallback defensivo a UTC.
 */
function toIcalLocal(iso: string): string {
  const m = iso.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?/);
  return m ? `${m[1]}${m[2]}${m[3]}T${m[4]}${m[5]}${m[6] ?? '00'}` : toIcalUtc(iso);
}

/** Convierte a DATE (yyyymmdd) para eventos de todo el dia. */
function toIcalDate(iso: string): string {
  const d = new Date(iso);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, '0');
  const day = String(d.getDate()).padStart(2, '0');
  return `${y}${m}${day}`;
}

/** Escapa texto segun RFC 5545 (comas, puntos y coma, saltos de linea). */
function escapeText(value: string): string {
  return value
    .replace(/\\/g, '\\\\')
    .replace(/;/g, '\\;')
    .replace(/,/g, '\\,')
    .replace(/\n/g, '\\n');
}

/**
 * Genera la linea RRULE (RFC 5545 3.8.5.3) desde una Recurrence. INTERVAL se
 * omite cuando es 1; BYDAY solo acompaña a una semanal; el fin es COUNT o UNTIL
 * (nunca ambos). UNTIL toma el fin del dia elegido en UTC para que ese ultimo dia
 * quede incluido. Mismo orden de partes que `Recurrencia.RRule` en Fyne.
 */
export function buildRRule(rec: Recurrence): string {
  const parts = [`FREQ=${rec.freq}`];
  if (rec.interval && rec.interval > 1) parts.push(`INTERVAL=${rec.interval}`);
  const byday = rec.freq === 'WEEKLY' && rec.byday ? normalizeWeekdays(rec.byday) : [];
  if (byday.length) parts.push(`BYDAY=${byday.join(',')}`);
  if (rec.count && rec.count > 0) {
    parts.push(`COUNT=${rec.count}`);
  } else if (rec.until) {
    parts.push(`UNTIL=${rec.until.replace(/-/g, '')}T235959Z`);
  }
  return `RRULE:${parts.join(';')}`;
}

/**
 * Extrae la Recurrence de un VEVENT crudo (para reabrir un evento recurrente en
 * el modal). Solo reconoce el subconjunto que produce la UI: DAILY/WEEKLY/MONTHLY/
 * YEARLY y, en una semanal, BYDAY con días sueltos («MO,WE»). Cualquier otra FREQ,
 * otro BY* (BYMONTH, BYMONTHDAY, «2MO»…) devuelve null: el formulario la enseña
 * como «Personalizada» y la conserva tal cual (`rruleCruda`).
 */
export function parseRecurrence(ical: string): Recurrence | null {
  const line = ical.match(/^RRULE:(.*)$/m);
  if (!line?.[1]) return null;
  const map = new Map<string, string>();
  for (const kv of line[1].trim().split(';')) {
    const [k, v] = kv.split('=');
    if (k && v) map.set(k.toUpperCase(), v.trim());
  }
  const freq = map.get('FREQ');
  if (freq !== 'DAILY' && freq !== 'WEEKLY' && freq !== 'MONTHLY' && freq !== 'YEARLY') {
    return null;
  }
  // De los BY* el formulario solo sabe editar el BYDAY de una semanal sin ordinales;
  // reescribir cualquier otro lo cambiaría (o lo perdería) al guardar.
  if ([...map.keys()].some((k) => k.startsWith('BY') && !(k === 'BYDAY' && freq === 'WEEKLY'))) {
    return null;
  }
  const rec: Recurrence = { freq };
  const interval = Number(map.get('INTERVAL'));
  if (interval > 1) rec.interval = interval;
  const bydayRaw = map.get('BYDAY');
  if (bydayRaw !== undefined) {
    const tokens = bydayRaw.toUpperCase().split(',');
    if (!tokens.every((d) => (WEEKDAYS as readonly string[]).includes(d))) return null;
    // Con INTERVAL > 1 la semana en que empieza cada salto depende de WKST, que el
    // formulario no guarda: rehacerla sin él movería las repeticiones.
    const wkst = map.get('WKST')?.toUpperCase();
    if (interval > 1 && wkst && wkst !== 'MO') return null;
    rec.byday = normalizeWeekdays(tokens);
  }
  const count = Number(map.get('COUNT'));
  if (count > 0) rec.count = count;
  const until = map.get('UNTIL');
  if (until && /^\d{8}/.test(until))
    rec.until = `${until.slice(0, 4)}-${until.slice(4, 6)}-${until.slice(6, 8)}`;
  return rec;
}

/** Genera el cuerpo VCALENDAR/VEVENT para un PUT CalDAV. */
export function buildVevent(input: EventInput): string {
  const now = toIcalUtc(new Date().toISOString());
  const lines = [
    'BEGIN:VCALENDAR',
    'VERSION:2.0',
    'PRODID:-//Sytue One//Sytue Calendar//ES',
    'CALSCALE:GREGORIAN',
    'BEGIN:VEVENT',
    `UID:${input.uid}`,
    `DTSTAMP:${now}`,
  ];

  if (input.allDay) {
    // Día completo: FLOTANTE (VALUE=DATE, sin zona) — mismo día en cualquier zona.
    lines.push(`DTSTART;VALUE=DATE:${toIcalDate(input.start)}`);
    if (input.end) lines.push(`DTEND;VALUE=DATE:${toIcalDate(input.end)}`);
  } else if (input.tzid && input.tzid !== 'UTC') {
    // Con zona: DTSTART;TZID=<IANA>:<hora local>. La hora del formulario es la
    // hora-de-pared en esa zona; el backend (go-ical + tzdata) la resuelve a UTC.
    lines.push(`DTSTART;TZID=${input.tzid}:${toIcalLocal(input.start)}`);
    if (input.end) lines.push(`DTEND;TZID=${input.tzid}:${toIcalLocal(input.end)}`);
  } else {
    lines.push(`DTSTART:${toIcalUtc(input.start)}`);
    if (input.end) lines.push(`DTEND:${toIcalUtc(input.end)}`);
  }

  if (input.recurrence) lines.push(buildRRule(input.recurrence));
  else if (input.rruleRaw && esRRuleSegura(input.rruleRaw)) lines.push(`RRULE:${input.rruleRaw}`);

  // EXDATE arrastrados al desgajar una serie (RFC 5545 3.8.5.1). Tipo de valor
  // acorde al DTSTART (DATE para todo el día; DATE-TIME UTC si no).
  for (const ex of input.exdates ?? []) {
    lines.push(input.allDay ? `EXDATE;VALUE=DATE:${ex}` : `EXDATE:${ex}`);
  }

  lines.push(`SUMMARY:${escapeText(input.summary)}`);
  // OPAQUE es el valor por defecto del RFC: solo se escribe el «disponible», así un
  // evento normal sale igual que siempre y volver a «ocupado» es quitar la línea.
  if (input.transp === 'TRANSPARENT') lines.push('TRANSP:TRANSPARENT');
  if (input.location) lines.push(`LOCATION:${escapeText(input.location)}`);
  if (input.description) lines.push(`DESCRIPTION:${escapeText(input.description)}`);

  // Color del evento (RFC 7986 / X-APPLE-CALENDAR-COLOR). Solo hex #RRGGBB de la
  // paleta: el guardado (regex) impide inyectar lineas/parametros en el iCal.
  if (input.color && /^#[0-9a-fA-F]{6}$/.test(input.color)) {
    lines.push(`X-APPLE-CALENDAR-COLOR:${input.color.toUpperCase()}`);
  }

  // Videollamada (RFC 7986 CONFERENCE). Solo http(s) y sin ';', ',' ni controles: el
  // ICS es un formato de lineas y una URL con esos caracteres cerraria la propiedad e
  // inyectaria otras nuevas. El backend vuelve a sanear (domain.SanitizeConferenceURL):
  // este ICS tambien puede llegar por CalDAV desde un cliente cualquiera.
  const conf = input.conferenceUrl?.trim() ?? '';
  // Un CR/LF dentro de la URL cerraría la línea CONFERENCE y abriría otra propiedad,
  // así que vetar el rango de control es justo el objetivo. La directiva va en UNA
  // línea a propósito: eslint-disable-next-line solo alcanza la línea siguiente, y
  // partida en dos se silenciaba a sí misma en vez de a la expresión regular.
  // eslint-disable-next-line no-control-regex -- vetar C0 es el objetivo
  if (/^https?:\/\/[^\s;,\x00-\x1f]+$/.test(conf)) {
    lines.push(`CONFERENCE;VALUE=URI;FEATURE=VIDEO;LABEL=Sytue Rooms:${conf}`);
  }

  // ORGANIZER/ATTENDEE (RFC 5545 3.8.4). Un VEVENT con asistentes debe declarar
  // organizador. El backend persiste los ATTENDEE en su tabla y, por la API REST
  // con send=true, entrega invitaciones iTIP a la bandeja del destinatario. Nota:
  // el PUT CalDAV guarda los asistentes pero NO auto-agenda (ver docs/api/event).
  if (input.organizer) lines.push(`ORGANIZER:mailto:${input.organizer}`);
  for (const email of input.attendees ?? []) {
    const addr = email.trim();
    // Solo un token iCalendar (letras y guiones) vuelve al .ics: lo que no lo sea
    // —o no esté— sale como NEEDS-ACTION, que es lo que pedía el invitado nuevo.
    const previa = input.partstats?.[addr.toLowerCase()]?.toUpperCase() ?? '';
    const partstat = /^[A-Z-]{1,32}$/.test(previa) ? previa : 'NEEDS-ACTION';
    if (addr)
      lines.push(`ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=${partstat};RSVP=TRUE:mailto:${addr}`);
  }

  // Passthrough VALARM: se reinyectan verbatim (solo se normaliza EOL a CRLF).
  for (const alarm of input.alarms ?? []) {
    lines.push(alarm.trim().replace(/\r?\n/g, '\r\n'));
  }

  lines.push('END:VEVENT', 'END:VCALENDAR');

  // CRLF segun RFC 5545.
  return lines.join('\r\n') + '\r\n';
}

/**
 * Extrae los bloques VALARM crudos del VEVENT maestro (passthrough al editar). Los de
 * una excepción (otro VEVENT con RECURRENCE-ID en el mismo recurso) son de esa
 * ocurrencia: sumarlos al maestro los repetiría en toda la serie.
 */
export function parseAlarms(ical: string): string[] {
  const out: string[] = [];
  const vevent = ical.match(/^BEGIN:VEVENT\r?\n[\s\S]*?^END:VEVENT/m)?.[0] ?? ical;
  for (const m of vevent.matchAll(/^BEGIN:VALARM\r?\n[\s\S]*?^END:VALARM/gm)) {
    out.push(m[0].replace(/\r?\n/g, '\r\n'));
  }
  return out;
}

/**
 * Serializa un recordatorio (minutos ANTES del inicio) a un bloque VALARM DISPLAY.
 * El disparo es una DURATION relativa negativa al DTSTART (RFC 5545 3.8.6.3), que
 * es lo que el backend precalcula en su cola de avisos.
 */
export function buildAlarm(minutesBefore: number, description: string): string {
  return [
    'BEGIN:VALARM',
    'ACTION:DISPLAY',
    `DESCRIPTION:${escapeText(description || 'Recordatorio')}`,
    `TRIGGER:${triggerFromMinutes(minutesBefore)}`,
    'END:VALARM',
  ].join('\r\n');
}

/** minutos-antes → DURATION iCal negativa ('-PT15M', '-P1D', '-PT0M' en el momento). */
function triggerFromMinutes(m: number): string {
  if (m <= 0) return '-PT0M';
  if (m % 1440 === 0) return `-P${m / 1440}D`;
  if (m % 60 === 0) return `-PT${m / 60}H`;
  return `-PT${m}M`;
}

/**
 * Separa los VALARM de un VEVENT en recordatorios EDITABLES por la UI (DISPLAY con
 * TRIGGER relativo negativo al inicio → minutos antes) y el resto (email, disparo
 * absoluto, RELATED=END…), que se conservan verbatim (passthrough) para no perder
 * lo que puso un cliente CalDAV externo.
 */
export function parseReminders(ical: string): { minutes: number[]; passthrough: string[] } {
  const minutes: number[] = [];
  const passthrough: string[] = [];
  for (const block of parseAlarms(ical)) {
    const m = editableAlarmMinutes(block);
    if (m === null) passthrough.push(block);
    else minutes.push(m);
  }
  return { minutes, passthrough };
}

/** Minutos-antes de un VALARM editable, o null si no lo es (se conserva verbatim). */
function editableAlarmMinutes(block: string): number | null {
  const action = block
    .match(/^ACTION:(.+)$/im)?.[1]
    ?.trim()
    .toUpperCase();
  if (action && action !== 'DISPLAY') return null; // sin ACTION = DISPLAY por defecto
  const trig = block.match(/^TRIGGER([^:\r\n]*):(.+)$/im);
  if (!trig) return null;
  const params = (trig[1] ?? '').toUpperCase();
  if (params.includes('VALUE=DATE-TIME') || params.includes('RELATED=END')) return null;
  return durationToMinutesBefore(trig[2]!.trim());
}

/** '-PT15M' | '-P1D' | 'PT0S' → minutos ANTES (>=0); null si es "después" o ilegible. */
function durationToMinutesBefore(d: string): number | null {
  const m = d.match(/^(-)?P(?:(\d+)D)?(?:T(?:(\d+)H)?(?:(\d+)M)?(?:(\d+)S)?)?$/);
  if (!m) return null;
  const total =
    Number(m[2] ?? 0) * 1440 +
    Number(m[3] ?? 0) * 60 +
    Number(m[4] ?? 0) +
    Math.round(Number(m[5] ?? 0) / 60);
  if (total === 0) return 0; // en el momento del evento
  return m[1] ? total : null; // solo el signo negativo ("antes") es editable
}

/**
 * Extrae los EXDATE de un VEVENT como tokens iCal crudos (yyyymmdd o
 * yyyymmddThhmmss[Z]). Reúne varias propiedades EXDATE y listas separadas por
 * comas (RFC 5545 3.8.5.1). Sirve para excluir instancias al expandir y para
 * partir las excepciones al desgajar una serie.
 */
export function parseExdates(ical: string): string[] {
  const out: string[] = [];
  for (const m of ical.matchAll(/^EXDATE[^:\r\n]*:(.+)$/gim)) {
    for (const tok of m[1]!.split(',')) {
      const t = tok.trim();
      if (t) out.push(t);
    }
  }
  return out;
}

/**
 * Extrae el color del evento de un VEVENT crudo: X-APPLE-CALENDAR-COLOR (hex, con
 * o sin canal alfa) o, si no, COLOR (nombre CSS3, RFC 7986). Devuelve un valor
 * usable como color CSS, o undefined. Se valida el formato para no arrastrar
 * basura a un `style` (Preact no interpola CSS, pero mantenemos el valor limpio).
 */
export function parseColor(ical: string): string | undefined {
  const apple = ical.match(
    /^X-APPLE-CALENDAR-COLOR[^:\r\n]*:\s*(#[0-9a-fA-F]{6}(?:[0-9a-fA-F]{2})?)/im,
  );
  if (apple?.[1]) return apple[1];
  const named = ical.match(/^COLOR[^:\r\n]*:\s*([a-zA-Z]{1,40})\s*$/im);
  return named?.[1];
}

/**
 * «Mostrar como» del maestro: 'TRANSPARENT' si el evento se marcó como disponible y,
 * en cualquier otro caso (sin TRANSP, OPAQUE o algo ilegible), 'OPAQUE', que es lo
 * que el servicio cuenta como ocupado.
 */
export function parseTransp(ical: string | undefined): Transp {
  if (!ical) return 'OPAQUE';
  const v = propLines(maestro(ical), 'TRANSP')[0]?.value.toUpperCase();
  return v === 'TRANSPARENT' ? 'TRANSPARENT' : 'OPAQUE';
}

/**
 * Los emails de los ATTENDEE de un VEVENT crudo (para reabrir el modal y para
 * conservarlos al mover un evento). Son los de `parseGuests`: los del maestro, sin
 * los destinatarios de sus VALARM, que si no pasarían a ser invitados al guardar.
 */
export function parseAttendees(ical: string): string[] {
  return parseGuests(ical).map((g) => g.email);
}

/**
 * La respuesta de un invitado tal y como la enseña la UI. El PARTSTAT de RFC 5545
 * tiene más valores (DELEGATED, COMPLETED…), pero para una reunión solo importan
 * cuatro: sí, quizá, no y «aún no ha contestado», que es también lo que vale para
 * un valor que no se conoce.
 */
export type GuestStatus = 'accepted' | 'tentative' | 'declined' | 'pending';

export interface Guest {
  email: string;
  /** El CN del ATTENDEE/ORGANIZER, si lo trae: «Ada Lovelace». */
  name?: string | undefined;
  status: GuestStatus;
}

/** PARTSTAT (cualquier caja) → la respuesta que pinta la UI. */
export function guestStatus(partstat: string | undefined): GuestStatus {
  switch ((partstat ?? '').trim().toUpperCase()) {
    case 'ACCEPTED':
      return 'accepted';
    case 'TENTATIVE':
      return 'tentative';
    case 'DECLINED':
      return 'declined';
    default:
      return 'pending';
  }
}

/** Deshace el plegado de líneas de RFC 5545 §3.1 (CRLF + espacio o tabulador). */
function unfold(ical: string): string {
  return ical.replace(/\r?\n[ \t]/g, '');
}

/** Valor de un parámetro (`CN`, `PARTSTAT`) en la parte de parámetros de una línea. */
function param(params: string, name: string): string | undefined {
  const m = params.match(new RegExp(`;${name}=("[^"]*"|[^;:]*)`, 'i'));
  const v = m?.[1]?.replace(/^"|"$/g, '').trim();
  return v || undefined;
}

/** Las líneas `NAME;params:valor` de una propiedad, ya desplegadas. */
function propLines(ical: string, name: string): { params: string; value: string }[] {
  const out: { params: string; value: string }[] = [];
  // Los parámetros pueden llevar «:» entre comillas (CN="Ada: equipo"): se saltan.
  // La clase de fuera excluye también «"», para que un entrecomillado solo pueda
  // leerse de una manera: si no, una línea que no casa (un valor vacío tras «:»)
  // retrocede por todas las particiones posibles y congela la pestaña.
  const re = new RegExp(`^${name}((?:;(?:"[^"]*"|[^;:"\r\n])*)*):(.+)$`, 'gim');
  for (const m of unfold(ical).matchAll(re)) out.push({ params: m[1] ?? '', value: m[2]!.trim() });
  return out;
}

function sinMailto(v: string): string {
  return v.replace(/^mailto:/i, '').trim();
}

/**
 * El primer VEVENT —el maestro—, desplegado y sin sus VALARM. Un VALARM con
 * ACTION:EMAIL lleva ATTENDEE (a quién se manda el aviso), que no es un invitado: el
 * servicio tampoco lo cuenta (go-ical lee solo las propiedades del VEVENT).
 */
function maestro(ical: string): string {
  const plano = unfold(ical);
  const vevent = plano.match(/BEGIN:VEVENT[\s\S]*?END:VEVENT/)?.[0] ?? plano;
  return vevent.replace(/^BEGIN:VALARM[\s\S]*?^END:VALARM[^\n]*\n?/gm, '');
}

/**
 * Los invitados de un VEVENT crudo con su respuesta: la lista de la vista rápida y
 * de las píldoras del formulario. Solo el primer VEVENT —el maestro—; las
 * excepciones de una serie llevan sus propios ATTENDEE y no se suman.
 */
export function parseGuests(ical: string): Guest[] {
  const out: Guest[] = [];
  for (const { params, value } of propLines(maestro(ical), 'ATTENDEE')) {
    const email = sinMailto(value);
    if (!email || out.some((g) => g.email.toLowerCase() === email.toLowerCase())) continue;
    out.push({ email, name: param(params, 'CN'), status: guestStatus(param(params, 'PARTSTAT')) });
  }
  return out;
}

/** El ORGANIZER del VEVENT crudo, o null si no lo declara. */
export function parseOrganizer(ical: string): { email: string; name?: string | undefined } | null {
  const linea = propLines(maestro(ical), 'ORGANIZER')[0];
  if (!linea) return null;
  const email = sinMailto(linea.value);
  return email ? { email, name: param(linea.params, 'CN') } : null;
}

/**
 * Las respuestas ya dadas, por email en minúsculas, para devolverlas al .ics al
 * guardar (`EventInput.partstats`). Solo las que no son «a la espera».
 */
export function guestPartstats(ical: string | undefined): Record<string, string> {
  const out: Record<string, string> = {};
  for (const g of ical ? parseGuests(ical) : []) {
    if (g.status !== 'pending') out[g.email.toLowerCase()] = g.status.toUpperCase();
  }
  return out;
}

/**
 * Lo que ha contestado `email` a la invitación del evento, o null si no figura
 * entre sus asistentes. Es lo que decide si la vista rápida pregunta «¿vas a ir?»
 * y qué respuesta sale marcada: se compara la DIRECCIÓN entera y no un trozo de la
 * línea, que «ana@sytue.eu» está dentro de «juana@sytue.eu».
 */
export function miRespuesta(
  ical: string | undefined,
  email: string | undefined,
): GuestStatus | null {
  const mio = email?.trim().toLowerCase();
  if (!ical || !mio) return null;
  return parseGuests(ical).find((g) => g.email.toLowerCase() === mio)?.status ?? null;
}

/**
 * La RRULE del maestro tal cual (sin «RRULE:»), solo si el formulario NO la sabe
 * editar: `FREQ=MONTHLY;BYDAY=2MO`, `FREQ=YEARLY;BYMONTH=3`… Con una regla que sí entiende
 * (`parseRecurrence`) devuelve undefined: esa viaja como `recurrence`.
 */
export function rruleCruda(ical: string | undefined): string | undefined {
  if (!ical || parseRecurrence(ical)) return undefined;
  const v = propLines(maestro(ical), 'RRULE')[0]?.value;
  return v && esRRuleSegura(v) ? v : undefined;
}

/** El evento se repite (RRULE en el maestro), la entienda el formulario o no. */
export function tieneRepeticion(ical: string | undefined): boolean {
  return Boolean(ical && propLines(maestro(ical), 'RRULE').length > 0);
}

/**
 * Una RRULE que puede volver al .ics sin abrir otra propiedad: pares NOMBRE=valor
 * separados por «;», con FREQ, y en el valor solo lo que usa RFC 5545 §3.3.10
 * (dígitos, letras, «,», «+», «-», y «:» no hace falta: UNTIL es yyyymmddThhmmssZ).
 */
export function esRRuleSegura(regla: string): boolean {
  return (
    regla.length <= 512 &&
    /^[A-Z][A-Z-]*=[A-Z0-9,+-]+(?:;[A-Z][A-Z-]*=[A-Z0-9,+-]+)*$/i.test(regla) &&
    /(?:^|;)FREQ=/i.test(regla)
  );
}

// --- Fundir un guardado con el recurso que ya existe -------------------------------

/** Una línea lógica del .ics: las físicas (con su plegado) y el texto desplegado. */
interface Linea {
  fisicas: string[];
  texto: string;
  nombre: string;
}

function lineasDe(ical: string): Linea[] {
  const out: Linea[] = [];
  for (const fisica of ical.split(/\r?\n/)) {
    const ultima = out[out.length - 1];
    if (/^[ \t]/.test(fisica) && ultima) {
      ultima.fisicas.push(fisica);
      ultima.texto += fisica.slice(1);
      continue;
    }
    if (!fisica.trim()) continue;
    out.push({
      fisicas: [fisica],
      texto: fisica,
      nombre: (fisica.match(/^[A-Za-z0-9-]+/)?.[0] ?? '').toUpperCase(),
    });
  }
  return out;
}

/** Parámetros (con su «;» inicial) y valor de una línea lógica. */
function partes(texto: string): { params: string; value: string } {
  const m = texto.match(/^[A-Za-z0-9-]+((?:;(?:"[^"]*"|[^;:"])*)*):([\s\S]*)$/);
  return { params: m?.[1] ?? '', value: m?.[2] ?? '' };
}

interface Bloque {
  /** Índice de BEGIN:VEVENT y de END:VEVENT en las líneas. */
  ini: number;
  fin: number;
  /** Propiedades del propio VEVENT (no las de sus VALARM). */
  props: Linea[];
  /** Subcomponentes (VALARM y cualquier otro), cada uno con sus líneas. */
  hijos: { nombre: string; lineas: Linea[] }[];
}

function veventsDe(lineas: Linea[]): Bloque[] {
  const out: Bloque[] = [];
  let actual: Bloque | null = null;
  let hijo: { nombre: string; lineas: Linea[]; prof: number } | null = null;
  lineas.forEach((l, i) => {
    const t = l.texto.trim().toUpperCase();
    if (!actual) {
      if (t === 'BEGIN:VEVENT') actual = { ini: i, fin: i, props: [], hijos: [] };
      return;
    }
    if (hijo) {
      hijo.lineas.push(l);
      if (t.startsWith('BEGIN:')) hijo.prof++;
      else if (t.startsWith('END:') && --hijo.prof === 0) {
        actual.hijos.push({ nombre: hijo.nombre, lineas: hijo.lineas });
        hijo = null;
      }
      return;
    }
    if (t === 'END:VEVENT') {
      actual.fin = i;
      out.push(actual);
      actual = null;
    } else if (t.startsWith('BEGIN:')) {
      hijo = { nombre: t.slice(6), lineas: [l], prof: 1 };
    } else {
      actual.props.push(l);
    }
  });
  return out;
}

/**
 * Lo que el formulario (o el arrastre) decide de un evento. Todo lo demás —CLASS,
 * TRANSP, STATUS, CATEGORIES, ATTACH, URL, RDATE, EXDATE, X-*, las excepciones de la
 * serie…— se queda como estaba en el recurso guardado.
 */
const GESTIONADAS = new Set([
  'DTSTAMP',
  'DTSTART',
  'DTEND',
  'DURATION',
  'SUMMARY',
  'LOCATION',
  'DESCRIPTION',
  'X-APPLE-CALENDAR-COLOR',
  'CONFERENCE',
  'ORGANIZER',
  'ATTENDEE',
  'LAST-MODIFIED',
  'SEQUENCE',
]);

/** Un CONFERENCE es del formulario solo si es un enlace web: el de teléfono no se ve. */
function esConferenciaWeb(l: Linea): boolean {
  return l.nombre === 'CONFERENCE' && /^https?:\/\//i.test(partes(l.texto).value.trim());
}

/**
 * Clave de un DTSTART/DTEND por su INSTANTE, no por cómo se escribe: el formulario
 * reabre un `…Z` en la zona del navegador, y sin esto cambiar solo el título
 * reescribía la hora (y subía SEQUENCE) sin moverla.
 */
function claveHora(l: Linea): string {
  const { params, value } = partes(l.texto);
  const v = value.trim();
  if (/^\d{8}$/.test(v)) return `${l.nombre}:D${v}`;
  const m = v.match(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})(Z)?$/);
  if (!m) return `${l.nombre}:${l.texto.replace(/"/g, '').toUpperCase()}`;
  const n = m.slice(1, 7).map(Number) as [number, number, number, number, number, number];
  if (m[7]) return `${l.nombre}:${Date.UTC(n[0], n[1] - 1, n[2], n[3], n[4], n[5])}`;
  const tz = param(params, 'TZID');
  if (!tz) return `${l.nombre}:F${v}`;
  try {
    return `${l.nombre}:${instantFromZonedWallClock(...n, tz).getTime()}`;
  } catch {
    return `${l.nombre}:${tz}|${v}`; // zona que Intl no conoce: se compara como texto
  }
}

/** Un valor de fecha del .ics leído como instante: día (D) o fecha-hora resoluble (T). */
type Instante = { tipo: 'D' | 'T'; ms: number };

function instanteDe(valor: string, tz: string | undefined): Instante | null {
  const v = valor.trim();
  const d = v.match(/^(\d{4})(\d{2})(\d{2})$/);
  if (d) return { tipo: 'D', ms: Date.UTC(+d[1]!, +d[2]! - 1, +d[3]!) };
  const m = v.match(/^(\d{4})(\d{2})(\d{2})T(\d{2})(\d{2})(\d{2})(Z)?$/);
  if (!m) return null;
  const n = m.slice(1, 7).map(Number) as [number, number, number, number, number, number];
  if (m[7]) return { tipo: 'T', ms: Date.UTC(n[0], n[1] - 1, n[2], n[3], n[4], n[5]) };
  if (!tz) return null; // hora flotante: en el servidor no hay zona con la que leerla
  try {
    return { tipo: 'T', ms: instantFromZonedWallClock(...n, tz).getTime() };
  } catch {
    return null;
  }
}

function dosCifras(n: number): string {
  return String(n).padStart(2, '0');
}

/** Un valor desplazado `delta` ms, escrito de la misma forma (día, …Z o en su TZID). */
function desplazarValor(valor: string, tz: string | undefined, delta: number): string {
  const v = valor.trim();
  const i = instanteDe(v, tz);
  if (!i) return valor;
  let ms = i.ms + delta;
  if (!v.endsWith('Z') && i.tipo === 'T' && tz) ms += zoneOffsetMinutes(new Date(ms), tz) * 60_000;
  const f = new Date(ms);
  const dia = `${f.getUTCFullYear()}${dosCifras(f.getUTCMonth() + 1)}${dosCifras(f.getUTCDate())}`;
  if (i.tipo === 'D') return dia;
  const hora = `${dosCifras(f.getUTCHours())}${dosCifras(f.getUTCMinutes())}${dosCifras(f.getUTCSeconds())}`;
  return `${dia}T${hora}${v.endsWith('Z') ? 'Z' : ''}`;
}

/** La línea con todos sus valores (lista por comas) desplazados. */
function desplazarLinea(l: Linea, delta: number): Linea {
  const { params, value } = partes(l.texto);
  const tz = param(params, 'TZID');
  const valores = value.split(',').map((v) => desplazarValor(v, tz, delta));
  const texto = `${l.texto.match(/^[A-Za-z0-9-]+/)?.[0] ?? l.nombre}${params}:${valores.join(',')}`;
  return texto === l.texto ? l : { fisicas: [texto], texto, nombre: l.nombre };
}

/** Instante de la primera (única) fecha de una línea DTSTART/RECURRENCE-ID. */
function instanteLinea(l: Linea | undefined): Instante | null {
  if (!l) return null;
  const { params, value } = partes(l.texto);
  return instanteDe(value.split(',')[0] ?? '', param(params, 'TZID'));
}

/** Lo que cambia la serie para un invitado (RFC 5545 §3.8.7.4): hora y repetición. */
function claveSecuencia(props: Linea[]): string {
  return props
    .filter((l) => ['DTSTART', 'DTEND', 'DURATION', 'RRULE'].includes(l.nombre))
    .map((l) => (l.nombre === 'DTSTART' || l.nombre === 'DTEND' ? claveHora(l) : l.texto))
    .sort()
    .join('\n');
}

/**
 * Funde un guardado sobre el recurso que ya existe: sustituye SOLO lo que el
 * formulario gestiona y deja el resto tal cual.
 *
 * Regenerar el VEVENT entero desde `EventInput` borraba en silencio lo que la web no
 * sabe expresar: una serie con BYDAY de Apple se quedaba en un evento, se perdían las
 * excepciones (otros VEVENT con RECURRENCE-ID del mismo recurso), CLASS:PRIVATE, las
 * categorías, los adjuntos, el CN de los invitados…
 *
 * - Lo que no cambia de valor conserva su línea original, con sus parámetros
 *   (LANGUAGE de un SUMMARY, CN y ROLE de un ATTENDEE, la RRULE byte a byte…).
 * - La RRULE se conserva tal cual con `recurrenceUnchanged`; si no, manda la del
 *   formulario, y sin ninguna se quita (RDATE y EXDATE se quedan).
 * - Los VALARM del maestro son los de `input.alarms` (los editables más los que el
 *   cliente arrastró verbatim); los de las excepciones no se tocan.
 * - SEQUENCE sube cuando cambia la hora o la repetición; DTSTAMP y LAST-MODIFIED se
 *   renuevan.
 *
 * Sin un maestro reconocible en `original`, devuelve `buildVevent(input)`.
 */
export function fundirVevent(original: string, input: EventInput): string {
  const viejas = lineasDe(original);
  const bloques = veventsDe(viejas);
  const maestroViejo = bloques.find((b) => !b.props.some((l) => l.nombre === 'RECURRENCE-ID'));
  const nuevo = veventsDe(lineasDe(buildVevent(input)))[0];
  if (!maestroViejo || !nuevo) return buildVevent(input);

  const gestionadas = new Set(GESTIONADAS);
  if (!input.recurrenceUnchanged) gestionadas.add('RRULE');
  // «Mostrar como» solo manda cuando el formulario lo dice: el arrastre no lo lleva y
  // tiene que conservar el TRANSP que hubiera (de este formulario o de Apple/Outlook).
  if (input.transp) gestionadas.add('TRANSP');
  const esGestionada = (l: Linea): boolean =>
    l.nombre === 'CONFERENCE' ? esConferenciaWeb(l) : gestionadas.has(l.nombre);
  const mismoValor = (a: Linea, b: Linea): boolean => {
    const va = partes(a.texto).value.trim();
    const vb = partes(b.texto).value.trim();
    return a.nombre === 'X-APPLE-CALENDAR-COLOR'
      ? va.toUpperCase() === vb.toUpperCase()
      : va === vb;
  };
  const deVieja = (nombre: string): Linea[] =>
    maestroViejo.props.filter((l) => l.nombre === nombre);

  // COLOR (nombre CSS, RFC 7986) no lo edita el formulario, pero `parseColor` lo lee
  // cuando no hay X-APPLE-CALENDAR-COLOR: si quien guarda cambia o quita el color que
  // traía el evento, el COLOR viejo no puede quedarse mandando.
  const appleViejo = deVieja('X-APPLE-CALENDAR-COLOR')[0];
  const appleNuevo = nuevo.props.find((l) => l.nombre === 'X-APPLE-CALENDAR-COLOR');
  if (appleViejo && !(appleNuevo && mismoValor(appleViejo, appleNuevo))) gestionadas.add('COLOR');

  const props: Linea[] = deVieja('UID').slice(0, 1);
  for (const l of nuevo.props) {
    if (l.nombre === 'UID') continue;
    if (l.nombre === 'EXDATE') {
      // Solo las que añade quien guarda; las del recurso ya van entre las conservadas.
      if (!deVieja('EXDATE').some((v) => v.texto === l.texto)) props.push(l);
      continue;
    }
    if (!esGestionada(l)) continue;
    let igual: Linea | undefined;
    if (l.nombre === 'ATTENDEE' || l.nombre === 'ORGANIZER') {
      // El mismo invitado conserva su línea: su CN, su ROLE, su PARTSTAT real.
      const email = sinMailto(partes(l.texto).value).toLowerCase();
      igual = deVieja(l.nombre).find(
        (v) => sinMailto(partes(v.texto).value).toLowerCase() === email,
      );
      // El ORGANIZER de un evento que otro organizó no pasa a ser quien guarda.
      if (l.nombre === 'ORGANIZER') igual ??= deVieja('ORGANIZER')[0];
    } else if (l.nombre === 'DTSTART' || l.nombre === 'DTEND') {
      igual = deVieja(l.nombre).find((v) => claveHora(v) === claveHora(l));
    } else if (l.nombre !== 'DTSTAMP') {
      igual = deVieja(l.nombre).find((v) => esGestionada(v) && mismoValor(v, l));
    }
    props.push(igual ?? l);
  }
  for (const l of maestroViejo.props) {
    if (l.nombre !== 'UID' && !esGestionada(l)) props.push(l);
  }

  const secuenciaVieja = Number(partes(deVieja('SEQUENCE')[0]?.texto ?? '').value) || 0;
  const cambia = claveSecuencia(maestroViejo.props) !== claveSecuencia(props);
  const ahora = partes(nuevo.props.find((l) => l.nombre === 'DTSTAMP')?.texto ?? '').value;
  const extra: string[] = [];
  if (cambia || deVieja('SEQUENCE').length) {
    extra.push(`SEQUENCE:${secuenciaVieja + (cambia ? 1 : 0)}`);
  }
  if (ahora) extra.push(`LAST-MODIFIED:${ahora}`);

  // La serie entera se mueve («todos», o arrastrar la serie): las excepciones y los
  // EXDATE apuntan a ocurrencias por su hora de inicio, así que se mueven con ella; si
  // no, la ocurrencia excluida volvía a la hora nueva y la movida salía dos veces. Con
  // otra regla (o sin ella) esas excepciones ya no son de ninguna ocurrencia y se quitan,
  // como hace el backend al editar por REST (`caldav.CarryOver`).
  const reglaDe = (ps: Linea[]): string =>
    ps
      .filter((l) => l.nombre === 'RRULE')
      .map((l) => l.texto.trim())
      .join('\n');
  const reglaVieja = reglaDe(maestroViejo.props);
  const mismaSerie = reglaVieja !== '' && reglaVieja === reglaDe(props);
  const iniViejo = instanteLinea(deVieja('DTSTART')[0]);
  const iniNuevo = instanteLinea(props.find((l) => l.nombre === 'DTSTART'));
  const delta =
    mismaSerie && iniViejo && iniNuevo && iniViejo.tipo === iniNuevo.tipo
      ? iniNuevo.ms - iniViejo.ms
      : 0;
  const finalProps = delta
    ? props.map((l) =>
        l.nombre === 'EXDATE' && deVieja('EXDATE').includes(l) ? desplazarLinea(l, delta) : l,
      )
    : props;
  const excepciones = new Map<number, Linea[]>();
  const excepcionesViejas = bloques.filter(
    (b) => b !== maestroViejo && b.props.some((l) => l.nombre === 'RECURRENCE-ID'),
  );
  for (const b of excepcionesViejas) {
    if (reglaVieja && !mismaSerie) {
      excepciones.set(b.ini, []);
      continue;
    }
    if (!delta) continue;
    const rid = b.props.find((l) => l.nombre === 'RECURRENCE-ID');
    // Una excepción que no se había movido (solo otro título, otro aviso…) sigue a la
    // serie; una que sí, se queda donde la dejaron.
    const sinMover =
      instanteLinea(rid)?.ms === instanteLinea(b.props.find((l) => l.nombre === 'DTSTART'))?.ms;
    const mover = (l: Linea): Linea =>
      l.nombre === 'RECURRENCE-ID' || (sinMover && (l.nombre === 'DTSTART' || l.nombre === 'DTEND'))
        ? desplazarLinea(l, delta)
        : l;
    const lineas = viejas.slice(b.ini, b.fin + 1);
    // Solo las propiedades del propio VEVENT, no las de sus VALARM.
    const propias = new Set(b.props);
    excepciones.set(
      b.ini,
      lineas.map((l) => (propias.has(l) ? mover(l) : l)),
    );
  }

  const hijos = [
    ...maestroViejo.hijos.filter((h) => h.nombre !== 'VALARM'),
    ...nuevo.hijos.filter((h) => h.nombre === 'VALARM'),
  ];
  const cuerpo = [
    ...viejas[maestroViejo.ini]!.fisicas,
    ...finalProps.flatMap((l) => l.fisicas),
    ...extra,
    ...hijos.flatMap((h) => h.lineas.flatMap((l) => l.fisicas)),
    ...viejas[maestroViejo.fin]!.fisicas,
  ];
  // Fuera del maestro, todo como estaba salvo las excepciones movidas o quitadas.
  const resto = (desde: number, hasta: number): string[] => {
    const out: string[] = [];
    for (let i = desde; i < hasta; i++) {
      const exc = excepciones.get(i);
      if (exc) {
        out.push(...exc.flatMap((l) => l.fisicas));
        i = excepcionesViejas.find((b) => b.ini === i)!.fin;
        continue;
      }
      out.push(...viejas[i]!.fisicas);
    }
    return out;
  };
  const salida = [
    ...resto(0, maestroViejo.ini),
    ...cuerpo,
    ...resto(maestroViejo.fin + 1, viejas.length),
  ];
  return salida.join('\r\n') + '\r\n';
}

/** Genera un UID estable para un evento nuevo. */
export function newUid(): string {
  const rand = globalThis.crypto.randomUUID();
  return `${rand}@sytue.eu`;
}
