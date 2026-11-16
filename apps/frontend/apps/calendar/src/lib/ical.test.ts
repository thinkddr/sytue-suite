/**
 * Passthrough VALARM: editar un evento desde la UI no debe perder los
 * recordatorios que puso un cliente CalDAV externo (parseAlarms extrae los
 * bloques crudos y buildVevent los reinyecta verbatim), y crear un evento
 * nuevo no debe inventarse ninguno.
 */
import { describe, expect, test } from 'bun:test';
import {
  buildAlarm,
  buildRRule,
  buildVevent,
  fundirVevent,
  guestPartstats,
  miRespuesta,
  guestStatus,
  newUid,
  parseAlarms,
  parseAttendees,
  parseColor,
  parseExdates,
  parseGuests,
  parseOrganizer,
  parseRecurrence,
  parseReminders,
  rruleCruda,
  tieneRepeticion,
  type Recurrence,
  type Weekday,
} from './ical.js';

const ALARM_DISPLAY =
  'BEGIN:VALARM\r\nACTION:DISPLAY\r\nDESCRIPTION:Recordatorio\r\nTRIGGER:-PT15M\r\nEND:VALARM';
const ALARM_EMAIL = 'BEGIN:VALARM\r\nACTION:EMAIL\r\nTRIGGER:-PT1H\r\nEND:VALARM';

const EXTERNAL_VEVENT =
  [
    'BEGIN:VCALENDAR',
    'VERSION:2.0',
    'BEGIN:VEVENT',
    'UID:ext-1@example.com',
    'DTSTART:20260710T100000Z',
    'DTEND:20260710T110000Z',
    'SUMMARY:Cita externa',
    ALARM_DISPLAY,
    ALARM_EMAIL,
    'END:VEVENT',
    'END:VCALENDAR',
  ].join('\r\n') + '\r\n';

test('newUid genera un identificador aleatorio UUID', () => {
  expect(newUid()).toMatch(/^[0-9a-f-]{36}@sytue\.eu$/i);
});

describe('color por evento', () => {
  test('buildVevent emite X-APPLE-CALENDAR-COLOR y parseColor lo recupera (round-trip)', () => {
    const ics = buildVevent({
      uid: 'c1@sytue.eu',
      summary: 'Con color',
      start: '2026-07-10T10:00',
      color: '#6b3e9e',
    });
    expect(ics).toContain('X-APPLE-CALENDAR-COLOR:#6B3E9E');
    expect(parseColor(ics)).toBe('#6B3E9E');
  });

  test('buildVevent descarta un color no-hex (sin inyeccion en el iCal)', () => {
    const ics = buildVevent({
      uid: 'c2@sytue.eu',
      summary: 'Sin color',
      start: '2026-07-10T10:00',
      color: '#fff\r\nSUMMARY:evil',
    });
    expect(ics).not.toContain('X-APPLE-CALENDAR-COLOR');
    expect(ics).not.toContain('evil');
  });

  test('parseColor lee un COLOR con nombre CSS3 (RFC 7986) y devuelve undefined sin color', () => {
    expect(parseColor('BEGIN:VEVENT\r\nCOLOR:cornflowerblue\r\nEND:VEVENT')).toBe('cornflowerblue');
    expect(parseColor('BEGIN:VEVENT\r\nSUMMARY:x\r\nEND:VEVENT')).toBeUndefined();
  });
});

describe('parseAlarms', () => {
  test('extrae cada bloque VALARM verbatim', () => {
    expect(parseAlarms(EXTERNAL_VEVENT)).toEqual([ALARM_DISPLAY, ALARM_EMAIL]);
  });

  test('normaliza fuentes con LF a CRLF y devuelve [] sin VALARM', () => {
    expect(parseAlarms(EXTERNAL_VEVENT.replace(/\r\n/g, '\n'))).toEqual([
      ALARM_DISPLAY,
      ALARM_EMAIL,
    ]);
    expect(parseAlarms('BEGIN:VEVENT\r\nSUMMARY:Sin alarmas\r\nEND:VEVENT')).toEqual([]);
  });
});

describe('buildVevent', () => {
  const base = { uid: 'ext-1@example.com', summary: 'Cita externa', start: '2026-07-10T12:00' };

  test('round-trip: reinyecta los VALARM extraidos, dentro del VEVENT', () => {
    const rebuilt = buildVevent({ ...base, alarms: parseAlarms(EXTERNAL_VEVENT) });
    // Los bloques sobreviven identicos al ciclo editar->guardar...
    expect(parseAlarms(rebuilt)).toEqual([ALARM_DISPLAY, ALARM_EMAIL]);
    // ...y quedan antes de END:VEVENT (dentro del componente).
    expect(rebuilt.indexOf(ALARM_EMAIL)).toBeLessThan(rebuilt.indexOf('END:VEVENT'));
  });

  test('creacion sin alarms no emite ningun VALARM', () => {
    expect(buildVevent(base)).not.toContain('VALARM');
    expect(buildVevent({ ...base, alarms: [] })).not.toContain('VALARM');
  });

  test('emite EXDATE de los tokens (timed y todo el día)', () => {
    const timed = buildVevent({ ...base, exdates: ['20260717T120000Z'] });
    expect(timed).toContain('EXDATE:20260717T120000Z');
    const allDay = buildVevent({ ...base, allDay: true, exdates: ['20260717'] });
    expect(allDay).toContain('EXDATE;VALUE=DATE:20260717');
  });
});

describe('recordatorios (VALARM editable)', () => {
  test('buildAlarm serializa minutos-antes a DURATION iCal', () => {
    expect(buildAlarm(15, 'Reunión')).toContain('TRIGGER:-PT15M');
    expect(buildAlarm(60, 'x')).toContain('TRIGGER:-PT1H');
    expect(buildAlarm(1440, 'x')).toContain('TRIGGER:-P1D');
    expect(buildAlarm(0, 'x')).toContain('TRIGGER:-PT0M');
    expect(buildAlarm(15, 'x')).toContain('ACTION:DISPLAY');
  });

  test('parseReminders separa editables (minutos) de passthrough', () => {
    const ical =
      'BEGIN:VEVENT\r\n' +
      'BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-PT15M\r\nEND:VALARM\r\n' +
      'BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:-P1D\r\nEND:VALARM\r\n' +
      'BEGIN:VALARM\r\nACTION:EMAIL\r\nTRIGGER:-PT1H\r\nEND:VALARM\r\n' +
      'END:VEVENT';
    const { minutes, passthrough } = parseReminders(ical);
    expect(minutes).toEqual([15, 1440]); // display relativos negativos
    expect(passthrough).toHaveLength(1); // el EMAIL se conserva verbatim
    expect(passthrough[0]).toContain('ACTION:EMAIL');
  });

  test('round-trip: buildAlarm → parseReminders recupera los minutos', () => {
    const blocks = [15, 60, 1440].map((m) => buildAlarm(m, 'x'));
    expect(parseReminders(blocks.join('\r\n')).minutes).toEqual([15, 60, 1440]);
  });

  test('un TRIGGER "después" (positivo) no es editable', () => {
    const ical = 'BEGIN:VALARM\r\nACTION:DISPLAY\r\nTRIGGER:PT15M\r\nEND:VALARM';
    const { minutes, passthrough } = parseReminders(ical);
    expect(minutes).toEqual([]);
    expect(passthrough).toHaveLength(1);
  });
});

describe('parseExdates', () => {
  test('reúne varias propiedades y listas por comas', () => {
    const ical =
      'BEGIN:VEVENT\r\n' +
      'EXDATE:20260708T090000Z,20260715T090000Z\r\n' +
      'EXDATE;VALUE=DATE:20260722\r\n' +
      'END:VEVENT';
    expect(parseExdates(ical)).toEqual(['20260708T090000Z', '20260715T090000Z', '20260722']);
  });
});

describe('invitados y su respuesta', () => {
  // Un VEVENT como lo devuelve el servicio: líneas plegadas a 75 octetos (RFC 5545
  // §3.1), CN entre comillas con «:» dentro y PARTSTAT en cualquier caja.
  const REUNION = [
    'BEGIN:VCALENDAR',
    'BEGIN:VEVENT',
    'UID:r-1@sytue.eu',
    'DTSTART:20261110T090000Z',
    'ORGANIZER;CN="Ada: equipo":mailto:ada@sytue.eu',
    'ATTENDEE;ROLE=REQ-PARTICIPANT;PARTSTAT=ACCEPTED;RSVP=TRUE:mailto:bob@examp',
    ' le.com',
    'ATTENDEE;CN=Carla;PARTSTAT=tentative:mailto:carla@sytue.eu',
    'ATTENDEE;PARTSTAT=DECLINED:mailto:dani@sytue.eu',
    'ATTENDEE;RSVP=TRUE:mailto:eva@sytue.eu',
    'ATTENDEE;PARTSTAT=DELEGATED:mailto:fede@sytue.eu',
    'END:VEVENT',
    'END:VCALENDAR',
  ].join('\r\n');

  test('lee cada invitado con su respuesta, también si la línea viene plegada', () => {
    expect(parseGuests(REUNION)).toEqual([
      { email: 'bob@example.com', name: undefined, status: 'accepted' },
      { email: 'carla@sytue.eu', name: 'Carla', status: 'tentative' },
      { email: 'dani@sytue.eu', name: undefined, status: 'declined' },
      { email: 'eva@sytue.eu', name: undefined, status: 'pending' },
      { email: 'fede@sytue.eu', name: undefined, status: 'pending' },
    ]);
  });

  test('el organizador, con el CN aunque lleve «:» entre comillas', () => {
    expect(parseOrganizer(REUNION)).toEqual({ email: 'ada@sytue.eu', name: 'Ada: equipo' });
    expect(parseOrganizer('BEGIN:VEVENT\r\nUID:x\r\nEND:VEVENT')).toBeNull();
  });

  test('guestStatus: lo que no es sí, quizá o no es «a la espera»', () => {
    expect(guestStatus('Accepted')).toBe('accepted');
    expect(guestStatus('NEEDS-ACTION')).toBe('pending');
    expect(guestStatus(undefined)).toBe('pending');
  });

  test('miRespuesta: lo que contesté yo, por mi dirección entera', () => {
    expect(miRespuesta(REUNION, 'Carla@Sytue.eu')).toBe('tentative');
    expect(miRespuesta(REUNION, 'bob@example.com')).toBe('accepted');
    expect(miRespuesta(REUNION, 'eva@sytue.eu')).toBe('pending');
    // Quien organiza no es invitado de su propia reunión, ni lo es quien no figura,
    // aunque su dirección esté DENTRO de la de otro («arla@sytue.eu» en «carla@…»).
    expect(miRespuesta(REUNION, 'ada@sytue.eu')).toBeNull();
    expect(miRespuesta(REUNION, 'arla@sytue.eu')).toBeNull();
    expect(miRespuesta(REUNION, '')).toBeNull();
    expect(miRespuesta(undefined, 'carla@sytue.eu')).toBeNull();
  });

  test('guardar conserva la respuesta de quien ya contestó', () => {
    const partstats = guestPartstats(REUNION);
    expect(partstats).toEqual({
      'bob@example.com': 'ACCEPTED',
      'carla@sytue.eu': 'TENTATIVE',
      'dani@sytue.eu': 'DECLINED',
    });
    const ics = buildVevent({
      uid: 'r-1@sytue.eu',
      summary: 'Revisión',
      start: '2026-11-10T09:00:00Z',
      organizer: 'ada@sytue.eu',
      attendees: ['Bob@Example.com', 'nuevo@sytue.eu'],
      partstats,
    });
    expect(ics).toContain('PARTSTAT=ACCEPTED;RSVP=TRUE:mailto:Bob@Example.com');
    // El recién añadido entra a la espera, como siempre.
    expect(ics).toContain('PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:nuevo@sytue.eu');
  });

  test('el destinatario de un recordatorio por correo no es un invitado', () => {
    // Thunderbird y Apple guardan así un aviso por email: el ATTENDEE va DENTRO del
    // VALARM. El servicio no lo cuenta, y la web tampoco debe pintarlo ni, al mover
    // el evento, convertirlo en invitado.
    const personal = [
      'BEGIN:VCALENDAR',
      'BEGIN:VEVENT',
      'UID:p-1@example.com',
      'DTSTART:20261110T090000Z',
      'SUMMARY:Dentista',
      'BEGIN:VALARM',
      'ACTION:EMAIL',
      'TRIGGER:-PT1H',
      'SUMMARY:Aviso',
      'ATTENDEE:mailto:yo@sytue.eu',
      'END:VALARM',
      'END:VEVENT',
      'END:VCALENDAR',
    ].join('\r\n');
    expect(parseGuests(personal)).toEqual([]);
    expect(parseAttendees(personal)).toEqual([]);
    // Con invitados de verdad, salen ellos y no el del aviso.
    const conInvitado = personal.replace(
      'SUMMARY:Dentista',
      'SUMMARY:Dentista\r\nATTENDEE;PARTSTAT=ACCEPTED:mailto:bob@sytue.eu',
    );
    expect(parseAttendees(conInvitado)).toEqual(['bob@sytue.eu']);
  });

  test('una línea con muchos parámetros entrecomillados y sin valor no se atasca', () => {
    // Sin la clase excluyente, cada «"a"» podía leerse entrecomillado o letra a
    // letra, y una línea que no casa probaba todas las combinaciones: con 18
    // segmentos JSC tardaba más de un segundo y luego abandonaba.
    const linea = `ATTENDEE;CN=${'"a"'.repeat(40)}:`;
    const ics = `BEGIN:VEVENT\r\n${linea}\r\nEND:VEVENT`;
    const t0 = performance.now();
    expect(parseGuests(ics)).toEqual([]);
    expect(performance.now() - t0).toBeLessThan(200);
  });

  test('un PARTSTAT que no es un token no se cuela en el .ics', () => {
    const ics = buildVevent({
      uid: 'r-2@sytue.eu',
      summary: 'x',
      start: '2026-11-10T09:00:00Z',
      attendees: ['bob@example.com'],
      partstats: { 'bob@example.com': 'ACCEPTED\r\nX-MAL:1' },
    });
    expect(ics).toContain('PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:bob@example.com');
    expect(ics).not.toContain('X-MAL');
  });
});

describe('fundirVevent: guardar no borra lo que el formulario no edita', () => {
  // Una serie de Apple: regla con BYDAY (el formulario no la sabe editar), una
  // excepción en el mismo recurso, propiedades que la web no enseña y un invitado
  // con su CN y su ROLE.
  const SERIE_APPLE =
    [
      'BEGIN:VCALENDAR',
      'VERSION:2.0',
      'PRODID:-//Apple Inc.//macOS 15//EN',
      'BEGIN:VTIMEZONE',
      'TZID:Europe/Madrid',
      'END:VTIMEZONE',
      'BEGIN:VEVENT',
      'UID:serie-1@icloud.com',
      'DTSTAMP:20260101T000000Z',
      'DTSTART:20261109T090000Z',
      'DTEND:20261109T100000Z',
      'RRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR;WKST=MO',
      'EXDATE:20261111T090000Z',
      'SEQUENCE:3',
      'SUMMARY;LANGUAGE=es:Diario de equipo',
      'CLASS:PRIVATE',
      'TRANSP:TRANSPARENT',
      'CATEGORIES:Trabajo,Equipo',
      'ATTACH;FMTTYPE=application/pdf:https://example.com/orden.pdf',
      'X-APPLE-TRAVEL-ADVISORY-BEHAVIOR:AUTOMATIC',
      'ORGANIZER;CN=Ada Lovelace:mailto:ada@example.com',
      'ATTENDEE;CN=Grace Hopper;ROLE=OPT-PARTICIPANT;PARTSTAT=ACCEPTED:mailto:grace@exam',
      ' ple.com',
      ALARM_DISPLAY,
      'END:VEVENT',
      'BEGIN:VEVENT',
      'UID:serie-1@icloud.com',
      'RECURRENCE-ID:20261113T090000Z',
      'DTSTART:20261113T110000Z',
      'DTEND:20261113T120000Z',
      'SUMMARY:Diario (movido)',
      ALARM_EMAIL,
      'END:VEVENT',
      'END:VCALENDAR',
    ].join('\r\n') + '\r\n';

  /** Lo que manda el formulario al reabrir la serie y cambiar solo el título. */
  const formulario = (extra: Partial<Parameters<typeof fundirVevent>[1]> = {}) => ({
    uid: 'serie-1@icloud.com',
    summary: 'Diario de equipo',
    start: '2026-11-09T09:00:00Z',
    end: '2026-11-09T10:00:00Z',
    rruleRaw: 'FREQ=WEEKLY;BYDAY=MO,WE,FR;WKST=MO',
    recurrenceUnchanged: true,
    attendees: ['grace@example.com'],
    organizer: 'yo@sytue.eu',
    partstats: { 'grace@example.com': 'ACCEPTED' },
    alarms: parseAlarms(SERIE_APPLE),
    ...extra,
  });

  const maestroDe = (ics: string) => ics.split('BEGIN:VEVENT')[1]!.split('END:VEVENT')[0]!;

  test('cambiar el título conserva la RRULE con BYDAY byte a byte', () => {
    const ics = fundirVevent(SERIE_APPLE, formulario({ summary: 'Diario del equipo' }));
    expect(ics).toContain('\r\nRRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR;WKST=MO\r\n');
    expect(ics.match(/^RRULE:/gm)).toHaveLength(1);
    expect(ics).toContain('SUMMARY:Diario del equipo');
    expect(ics).not.toContain('SUMMARY;LANGUAGE=es:Diario de equipo');
  });

  test('lo que la web no edita se queda: CLASS, TRANSP, CATEGORIES, ATTACH, X-*, EXDATE', () => {
    const ics = fundirVevent(SERIE_APPLE, formulario({ summary: 'Otro' }));
    for (const linea of [
      'CLASS:PRIVATE',
      'TRANSP:TRANSPARENT',
      'CATEGORIES:Trabajo,Equipo',
      'ATTACH;FMTTYPE=application/pdf:https://example.com/orden.pdf',
      'X-APPLE-TRAVEL-ADVISORY-BEHAVIOR:AUTOMATIC',
      'EXDATE:20261111T090000Z',
      'PRODID:-//Apple Inc.//macOS 15//EN',
      'TZID:Europe/Madrid',
    ]) {
      expect(ics).toContain(`\r\n${linea}\r\n`);
    }
  });

  test('la excepción de la serie (RECURRENCE-ID) y su VALARM siguen en el recurso', () => {
    const ics = fundirVevent(SERIE_APPLE, formulario());
    expect(ics.match(/^BEGIN:VEVENT/gm)).toHaveLength(2);
    expect(ics).toContain('RECURRENCE-ID:20261113T090000Z\r\nDTSTART:20261113T110000Z');
    expect(ics).toContain('SUMMARY:Diario (movido)');
    // El aviso por correo es de la excepción: no se cuela en el maestro.
    expect(maestroDe(ics)).not.toContain('ACTION:EMAIL');
    expect(maestroDe(ics)).toContain('ACTION:DISPLAY');
  });

  test('el invitado conserva su CN y su ROLE, y el organizador no pasa a ser quien guarda', () => {
    const ics = fundirVevent(SERIE_APPLE, formulario());
    expect(ics).toContain(
      'ATTENDEE;CN=Grace Hopper;ROLE=OPT-PARTICIPANT;PARTSTAT=ACCEPTED:mailto:grace@exam\r\n ple.com',
    );
    expect(ics).toContain('ORGANIZER;CN=Ada Lovelace:mailto:ada@example.com');
    expect(ics).not.toContain('yo@sytue.eu');
  });

  test('un invitado nuevo entra con su línea, y quitar uno lo quita', () => {
    const ics = fundirVevent(
      SERIE_APPLE,
      formulario({ attendees: ['linus@example.com'], partstats: {} }),
    );
    expect(ics).not.toContain('grace@exam');
    expect(ics).toContain('PARTSTAT=NEEDS-ACTION;RSVP=TRUE:mailto:linus@example.com');
  });

  test('sin cambiar la hora no sube SEQUENCE; moverla sí', () => {
    const igual = fundirVevent(SERIE_APPLE, formulario({ summary: 'Otro' }));
    expect(igual).toContain('\r\nSEQUENCE:3\r\n');
    expect(igual).toContain('\r\nDTSTART:20261109T090000Z\r\n');
    const movida = fundirVevent(
      SERIE_APPLE,
      formulario({ start: '2026-11-09T10:00:00Z', end: '2026-11-09T11:00:00Z' }),
    );
    expect(movida).toContain('\r\nSEQUENCE:4\r\n');
    expect(movida).toContain('\r\nDTSTART:20261109T100000Z\r\n');
    expect(movida.match(/^SEQUENCE:/gm)).toHaveLength(1);
  });

  test('la misma hora escrita en otra zona no cuenta como cambio', () => {
    const ics = fundirVevent(
      SERIE_APPLE,
      formulario({ start: '2026-11-09T10:00', end: '2026-11-09T11:00', tzid: 'Europe/Madrid' }),
    );
    expect(ics).toContain('\r\nDTSTART:20261109T090000Z\r\n');
    expect(ics).toContain('\r\nSEQUENCE:3\r\n');
  });

  test('«No se repite» quita la RRULE (y sube SEQUENCE); otra regla la sustituye', () => {
    const sin = fundirVevent(
      SERIE_APPLE,
      formulario({ rruleRaw: undefined, recurrenceUnchanged: false }),
    );
    expect(sin).not.toContain('RRULE:');
    expect(sin).toContain('\r\nSEQUENCE:4\r\n');
    const otra = fundirVevent(
      SERIE_APPLE,
      formulario({
        rruleRaw: undefined,
        recurrenceUnchanged: false,
        recurrence: { freq: 'DAILY' },
      }),
    );
    expect(otra).toContain('\r\nRRULE:FREQ=DAILY\r\n');
    expect(otra.match(/^RRULE:/gm)).toHaveLength(1);
  });

  test('arrastrar un evento con CLASS:PRIVATE y una excepción conserva ambos', () => {
    const ics = fundirVevent(SERIE_APPLE, {
      uid: 'serie-1@icloud.com',
      summary: 'Diario de equipo',
      start: '2026-11-09T09:30:00Z',
      end: '2026-11-09T10:30:00Z',
      recurrenceUnchanged: true,
      attendees: ['grace@example.com'],
      organizer: 'yo@sytue.eu',
      alarms: parseAlarms(SERIE_APPLE),
    });
    expect(ics).toContain('\r\nCLASS:PRIVATE\r\n');
    // La serie se mueve media hora: la excepción y el EXDATE siguen a su ocurrencia.
    // La excepción ya estaba movida a las 11:00 y ahí se queda.
    expect(ics).toContain('RECURRENCE-ID:20261113T093000Z\r\nDTSTART:20261113T110000Z');
    expect(ics).toContain('\r\nEXDATE:20261111T093000Z\r\n');
    expect(ics).toContain('\r\nRRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR;WKST=MO\r\n');
    expect(ics).toContain('\r\nSEQUENCE:4\r\n');
  });

  test('mover la serie mueve una excepción sin mover con ella, también en su TZID', () => {
    const conTz = SERIE_APPLE.replace(
      'RECURRENCE-ID:20261113T090000Z\r\nDTSTART:20261113T110000Z\r\nDTEND:20261113T120000Z',
      'RECURRENCE-ID;TZID=Europe/Madrid:20261113T100000\r\n' +
        'DTSTART;TZID=Europe/Madrid:20261113T100000\r\nDTEND;TZID=Europe/Madrid:20261113T110000',
    );
    const ics = fundirVevent(
      conTz,
      formulario({ start: '2026-11-09T10:00:00Z', end: '2026-11-09T11:00:00Z' }),
    );
    expect(ics).toContain(
      'RECURRENCE-ID;TZID=Europe/Madrid:20261113T110000\r\n' +
        'DTSTART;TZID=Europe/Madrid:20261113T110000\r\nDTEND;TZID=Europe/Madrid:20261113T120000',
    );
    expect(ics).toContain('SUMMARY:Diario (movido)');
  });

  test('con otra regla, o sin ella, las excepciones de la serie vieja se van', () => {
    const otra = fundirVevent(
      SERIE_APPLE,
      formulario({
        rruleRaw: undefined,
        recurrenceUnchanged: false,
        recurrence: { freq: 'DAILY' },
      }),
    );
    expect(otra.match(/^BEGIN:VEVENT/gm)).toHaveLength(1);
    expect(otra).not.toContain('RECURRENCE-ID');
    expect(otra).toContain('\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n');
    const sin = fundirVevent(
      SERIE_APPLE,
      formulario({ rruleRaw: undefined, recurrenceUnchanged: false }),
    );
    expect(sin).not.toContain('RECURRENCE-ID');
  });

  test('cambiar o quitar el color quita también el COLOR por nombre que traía', () => {
    const conColor = SERIE_APPLE.replace(
      'CLASS:PRIVATE',
      'CLASS:PRIVATE\r\nX-APPLE-CALENDAR-COLOR:#FF0000\r\nCOLOR:red',
    );
    const igual = fundirVevent(conColor, formulario({ color: '#ff0000' }));
    expect(igual).toContain('\r\nCOLOR:red\r\n');
    expect(parseColor(fundirVevent(conColor, formulario()))).toBeUndefined();
    expect(parseColor(fundirVevent(conColor, formulario({ color: '#00AA00' })))).toBe('#00AA00');
    // Sin X-APPLE que lo acompañe, el COLOR por nombre no lo toca el formulario.
    const soloNombre = SERIE_APPLE.replace('CLASS:PRIVATE', 'CLASS:PRIVATE\r\nCOLOR:red');
    expect(fundirVevent(soloNombre, formulario())).toContain('\r\nCOLOR:red\r\n');
  });

  test('sin maestro reconocible, sale lo mismo que un evento nuevo', () => {
    const input = { uid: 'nuevo@sytue.eu', summary: 'Nuevo', start: '2026-11-09T09:00:00Z' };
    const sinDtstamp = (s: string) => s.replace(/^DTSTAMP:.*\r\n/m, '');
    expect(sinDtstamp(fundirVevent('', input))).toBe(sinDtstamp(buildVevent(input)));
  });
});

describe('reglas que el formulario no sabe editar', () => {
  const con = (regla: string) =>
    `BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:x\r\nDTSTART:20261109T090000Z\r\nRRULE:${regla}\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n`;

  test('rruleCruda devuelve la regla tal cual solo si parseRecurrence no la entiende', () => {
    expect(rruleCruda(con('FREQ=MONTHLY;BYDAY=2MO'))).toBe('FREQ=MONTHLY;BYDAY=2MO');
    expect(rruleCruda(con('FREQ=YEARLY;BYMONTH=3'))).toBe('FREQ=YEARLY;BYMONTH=3');
    expect(rruleCruda(con('FREQ=WEEKLY;INTERVAL=2'))).toBeUndefined();
    // Desde CAL-11 el formulario edita estas dos: ya no son «Personalizada».
    expect(rruleCruda(con('FREQ=WEEKLY;BYDAY=MO,WE'))).toBeUndefined();
    expect(rruleCruda(con('FREQ=YEARLY'))).toBeUndefined();
    expect(rruleCruda('BEGIN:VEVENT\r\nUID:x\r\nEND:VEVENT')).toBeUndefined();
  });

  test('tieneRepeticion cuenta cualquier RRULE', () => {
    expect(tieneRepeticion(con('FREQ=YEARLY;BYMONTH=3'))).toBe(true);
    expect(tieneRepeticion(con('FREQ=DAILY'))).toBe(true);
    expect(tieneRepeticion('BEGIN:VEVENT\r\nUID:x\r\nEND:VEVENT')).toBe(false);
  });

  test('buildVevent emite rruleRaw si no hay recurrence, y nunca una que abra otra línea', () => {
    const base = { uid: 'r@sytue.eu', summary: 'R', start: '2026-11-09T09:00:00Z' };
    expect(buildVevent({ ...base, rruleRaw: 'FREQ=YEARLY;BYMONTH=3' })).toContain(
      '\r\nRRULE:FREQ=YEARLY;BYMONTH=3\r\n',
    );
    const mala = buildVevent({ ...base, rruleRaw: 'FREQ=DAILY\r\nCLASS:PUBLIC' });
    expect(mala).not.toContain('RRULE:');
    expect(mala).not.toContain('CLASS:PUBLIC');
    expect(
      buildVevent({ ...base, recurrence: { freq: 'DAILY' }, rruleRaw: 'FREQ=YEARLY' }),
    ).toContain('RRULE:FREQ=DAILY\r\n');
  });

  test('parseAlarms solo lee los VALARM del maestro', () => {
    const ics = `BEGIN:VEVENT\r\nUID:x\r\n${ALARM_DISPLAY}\r\nEND:VEVENT\r\nBEGIN:VEVENT\r\nUID:x\r\nRECURRENCE-ID:20261113T090000Z\r\n${ALARM_EMAIL}\r\nEND:VEVENT\r\n`;
    expect(parseAlarms(ics)).toEqual([ALARM_DISPLAY]);
  });
});

/*
 * Las reglas que el formulario sabe escribir y leer. Mismos vectores que
 * TestLaReglaEsLaDeIcalTs en apps/fyne/calendario: las dos plataformas guardan la
 * misma RRULE para la misma elección.
 */
describe('Repetir: cada año, cada día laborable y ciertos días (CAL-11)', () => {
  const con = (regla: string) =>
    `BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:x\r\nDTSTART:20261109T090000Z\r\nRRULE:${regla}\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n`;

  const vectores: [Recurrence, string][] = [
    [{ freq: 'WEEKLY', byday: ['MO', 'TU', 'WE', 'TH', 'FR'] }, 'FREQ=WEEKLY;BYDAY=MO,TU,WE,TH,FR'],
    [
      { freq: 'WEEKLY', interval: 2, byday: ['MO', 'WE'], count: 6 },
      'FREQ=WEEKLY;INTERVAL=2;BYDAY=MO,WE;COUNT=6',
    ],
    [{ freq: 'YEARLY' }, 'FREQ=YEARLY'],
    [{ freq: 'YEARLY', until: '2030-12-31' }, 'FREQ=YEARLY;UNTIL=20301231T235959Z'],
  ];

  test('buildRRule escribe BYDAY y YEARLY', () => {
    for (const [rec, regla] of vectores) expect(buildRRule(rec)).toBe(`RRULE:${regla}`);
  });

  test('parseRecurrence las lee de vuelta tal cual', () => {
    for (const [rec, regla] of vectores) expect(parseRecurrence(con(regla))).toEqual(rec);
  });

  test('BYDAY sale ordenado de lunes a domingo, sin repetir ni valores ajenos', () => {
    expect(buildRRule({ freq: 'WEEKLY', byday: ['FR', 'MO', 'FR'] })).toBe(
      'RRULE:FREQ=WEEKLY;BYDAY=MO,FR',
    );
    expect(buildRRule({ freq: 'WEEKLY', byday: ['MO', 'XX\r\nCLASS:PUBLIC' as Weekday] })).toBe(
      'RRULE:FREQ=WEEKLY;BYDAY=MO',
    );
    // BYDAY solo acompaña a una semanal.
    expect(buildRRule({ freq: 'DAILY', byday: ['MO'] })).toBe('RRULE:FREQ=DAILY');
    expect(parseRecurrence(con('FREQ=WEEKLY;BYDAY=we,MO;WKST=MO'))).toEqual({
      freq: 'WEEKLY',
      byday: ['MO', 'WE'],
    });
  });

  test('lo que no sabe reescribir sin cambiarlo sigue siendo «Personalizada»', () => {
    for (const regla of [
      'FREQ=MONTHLY;BYDAY=2MO',
      'FREQ=WEEKLY;BYDAY=1MO',
      'FREQ=WEEKLY;BYDAY=MO;BYMONTH=3',
      'FREQ=DAILY;BYDAY=MO,TU',
      'FREQ=YEARLY;BYMONTH=3;BYMONTHDAY=8',
      // Cada dos semanas con la semana empezando en domingo: sin WKST cambiaría.
      'FREQ=WEEKLY;INTERVAL=2;BYDAY=MO;WKST=SU',
    ]) {
      expect(parseRecurrence(con(regla))).toBeNull();
      expect(rruleCruda(con(regla))).toBe(regla);
    }
  });
});
