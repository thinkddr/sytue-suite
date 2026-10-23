/**
 * Passthrough VALARM: editar un evento desde la UI no debe perder los
 * recordatorios que puso un cliente CalDAV externo (parseAlarms extrae los
 * bloques crudos y buildVevent los reinyecta verbatim), y crear un evento
 * nuevo no debe inventarse ninguno.
 */
import { describe, expect, test } from 'bun:test';
import {
  buildAlarm,
  buildVevent,
  newUid,
  parseAlarms,
  parseColor,
  parseExdates,
  parseReminders,
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
