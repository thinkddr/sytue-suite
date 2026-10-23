/**
 * Construccion de cuerpos iCalendar (VEVENT) para crear/editar eventos via
 * CalDAV PUT. El SDK @sytue/api-clients/caldav parsea VEVENT; aqui generamos el
 * VCALENDAR de salida. Formato de fechas: UTC basico (yyyymmddThhmmssZ) o DATE
 * para todo el dia. Minimo viable RFC 5545.
 */

/**
 * Recurrencia iCalendar (RRULE) — subconjunto que expone la UI: FREQ diaria/
 * semanal/mensual, INTERVAL y un fin por COUNT o UNTIL (mutuamente excluyentes,
 * como exige el backend Go). `until` es 'yyyy-mm-dd' (valor de <input type=date>).
 */
export interface Recurrence {
  freq: 'DAILY' | 'WEEKLY' | 'MONTHLY';
  interval?: number | undefined;
  count?: number | undefined;
  until?: string | undefined;
}

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
 * omite cuando es 1; el fin es COUNT o UNTIL (nunca ambos). UNTIL toma el fin
 * del dia elegido en UTC para que ese ultimo dia quede incluido.
 */
export function buildRRule(rec: Recurrence): string {
  const parts = [`FREQ=${rec.freq}`];
  if (rec.interval && rec.interval > 1) parts.push(`INTERVAL=${rec.interval}`);
  if (rec.count && rec.count > 0) {
    parts.push(`COUNT=${rec.count}`);
  } else if (rec.until) {
    parts.push(`UNTIL=${rec.until.replace(/-/g, '')}T235959Z`);
  }
  return `RRULE:${parts.join(';')}`;
}

/**
 * Extrae la Recurrence de un VEVENT crudo (para reabrir un evento recurrente en
 * el modal y para expandir ocurrencias en cliente). Solo reconoce el
 * subconjunto DAILY/WEEKLY/MONTHLY que produce la UI; cualquier otra FREQ (o
 * regla con BY*) devuelve null y el evento se trata como puntual.
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
  if (freq !== 'DAILY' && freq !== 'WEEKLY' && freq !== 'MONTHLY') return null;
  // BY* (BYDAY, BYMONTHDAY, ...) no las genera esta UI: no las sabemos expandir
  // aqui, asi que degradamos a evento puntual en vez de pintar mal.
  if ([...map.keys()].some((k) => k.startsWith('BY'))) return null;
  const rec: Recurrence = { freq };
  const interval = Number(map.get('INTERVAL'));
  if (interval > 1) rec.interval = interval;
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

  // EXDATE arrastrados al desgajar una serie (RFC 5545 3.8.5.1). Tipo de valor
  // acorde al DTSTART (DATE para todo el día; DATE-TIME UTC si no).
  for (const ex of input.exdates ?? []) {
    lines.push(input.allDay ? `EXDATE;VALUE=DATE:${ex}` : `EXDATE:${ex}`);
  }

  lines.push(`SUMMARY:${escapeText(input.summary)}`);
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
    if (addr)
      lines.push(`ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:${addr}`);
  }

  // Passthrough VALARM: se reinyectan verbatim (solo se normaliza EOL a CRLF).
  for (const alarm of input.alarms ?? []) {
    lines.push(alarm.trim().replace(/\r?\n/g, '\r\n'));
  }

  lines.push('END:VEVENT', 'END:VCALENDAR');

  // CRLF segun RFC 5545.
  return lines.join('\r\n') + '\r\n';
}

/** Extrae los bloques VALARM crudos de un VEVENT (passthrough al editar). */
export function parseAlarms(ical: string): string[] {
  const out: string[] = [];
  for (const m of ical.matchAll(/^BEGIN:VALARM\r?\n[\s\S]*?^END:VALARM/gm)) {
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

/** Extrae los emails de los ATTENDEE de un VEVENT crudo (para reabrir el modal). */
export function parseAttendees(ical: string): string[] {
  const out: string[] = [];
  for (const m of ical.matchAll(/^ATTENDEE[^:]*:(?:mailto:)?(.+)$/gim)) {
    const email = m[1]?.trim();
    if (email) out.push(email);
  }
  return out;
}

/** Genera un UID estable para un evento nuevo. */
export function newUid(): string {
  const rand = globalThis.crypto.randomUUID();
  return `${rand}@sytue.eu`;
}
