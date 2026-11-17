/**
 * Matemática del desgaje de series («este y los siguientes»): la serie nueva
 * hereda el patrón con el fin recalculado (COUNT restante / UNTIL) y arrastra los
 * EXDATE posteriores al corte. El recorte del maestro (EXDATE/UNTIL) se prueba en
 * Go (services/calendar/internal/caldav). Aquí, las funciones puras del cliente.
 */
import { afterEach, describe, expect, test } from 'bun:test';
import {
  applyRecurringEdit,
  carriedExdates,
  carryRule,
  countBefore,
  permiteSiguientes,
  type RecurTarget,
} from './recurrence.js';
import type { EventInput, Recurrence } from '../client/calendarApi.js';

const masterStart = new Date('2026-07-01T09:00:00Z'); // 1-jul (miércoles)
const cut = new Date('2026-07-15T09:00:00Z'); // corte en la 3ª ocurrencia semanal

describe('countBefore', () => {
  test('cuenta las ocurrencias semanales anteriores al corte', () => {
    expect(countBefore(masterStart, { freq: 'WEEKLY' }, cut)).toBe(2); // 1-jul, 8-jul
  });
  test('no cuenta más allá del COUNT del maestro', () => {
    expect(countBefore(masterStart, { freq: 'WEEKLY', count: 2 }, cut)).toBe(2);
  });
  test('una semanal con días cuenta cada día elegido, no una por semana', () => {
    // Mi 1, Lu 6, Mi 8, Lu 13: el lunes 29-jun es anterior al inicio y no cuenta.
    expect(countBefore(masterStart, { freq: 'WEEKLY', byday: ['MO', 'WE'] }, cut)).toBe(4);
    const laborables: Recurrence = { freq: 'WEEKLY', byday: ['MO', 'TU', 'WE', 'TH', 'FR'] };
    expect(countBefore(masterStart, laborables, cut)).toBe(10); // 1-3, 6-10 y 13-14 jul
    // Cada dos semanas: Mi 1 y Lu 13 (la semana del 6 se salta; el 15 es el corte).
    expect(
      countBefore(masterStart, { freq: 'WEEKLY', interval: 2, byday: ['MO', 'WE'] }, cut),
    ).toBe(2);
  });
  test('una anual cuenta años', () => {
    const corte = new Date('2028-08-01T09:00:00Z');
    expect(countBefore(masterStart, { freq: 'YEARLY' }, corte)).toBe(3); // 2026, 2027, 2028
  });
});

describe('carryRule', () => {
  test('COUNT: la serie nueva hereda el remanente (10 − 2 = 8)', () => {
    const rec: Recurrence = { freq: 'WEEKLY', count: 10 };
    expect(carryRule(rec, masterStart, cut)).toEqual({ freq: 'WEEKLY', count: 8 });
  });
  test('BYDAY: la serie nueva conserva los días y descuenta cada uno de ellos', () => {
    const rec: Recurrence = { freq: 'WEEKLY', byday: ['MO', 'WE'], count: 10 };
    expect(carryRule(rec, masterStart, cut)).toEqual({
      freq: 'WEEKLY',
      byday: ['MO', 'WE'],
      count: 6,
    });
  });
  test('UNTIL: la serie nueva conserva el UNTIL del maestro', () => {
    const rec: Recurrence = { freq: 'WEEKLY', until: '2026-12-31' };
    expect(carryRule(rec, masterStart, cut)).toEqual({ freq: 'WEEKLY', until: '2026-12-31' });
  });
  test('infinita: solo freq (+ interval si >1)', () => {
    expect(carryRule({ freq: 'DAILY', interval: 2 }, masterStart, cut)).toEqual({
      freq: 'DAILY',
      interval: 2,
    });
  });
  test('remanente agotado se acota a 1 (nunca 0 ni negativo)', () => {
    const rec: Recurrence = { freq: 'WEEKLY', count: 2 };
    expect(carryRule(rec, masterStart, new Date('2026-08-05T09:00:00Z')).count).toBe(1);
  });
});

describe('carriedExdates', () => {
  test('arrastra solo los EXDATE en/después del corte', () => {
    const ical =
      'BEGIN:VEVENT\r\n' +
      'EXDATE:20260708T090000Z\r\n' + // antes del corte → se queda en el maestro
      'EXDATE:20260722T090000Z\r\n' + // después → se arrastra a la serie nueva
      'END:VEVENT';
    expect(carriedExdates(ical, cut)).toEqual(['20260722T090000Z']);
  });
});

describe('series con una regla que el formulario no sabe editar', () => {
  const fetchReal = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = fetchReal;
  });

  /** Lo que el cliente manda al BFF, por ruta. */
  function apuntar(): { url: string; body: { eventInput?: EventInput } }[] {
    const vistas: { url: string; body: { eventInput?: EventInput } }[] = [];
    globalThis.fetch = (async (url: string | URL, init?: RequestInit) => {
      vistas.push({ url: String(url), body: JSON.parse(String(init?.body ?? '{}')) });
      return new Response('{"ok":true}', { status: 200 });
    }) as unknown as typeof fetch;
    return vistas;
  }

  const target: RecurTarget = {
    calendarId: 'c1',
    calendarUrl: '/dav/calendars/yo/personal/',
    uid: 'serie-1',
    href: '/dav/calendars/yo/personal/serie-1.ics',
    masterIcal:
      'BEGIN:VEVENT\r\nDTSTART:20261109T090000Z\r\nRRULE:FREQ=WEEKLY;BYDAY=MO,WE\r\nEND:VEVENT',
    recurrenceId: new Date('2026-11-11T09:00:00Z'),
    allDay: false,
  };
  const edited: EventInput = {
    summary: 'Diario',
    start: '2026-11-11T09:30:00Z',
    end: '2026-11-11T10:30:00Z',
    color: '#6B3E9E',
    rruleRaw: 'FREQ=WEEKLY;BYDAY=MO,WE',
    recurrenceUnchanged: true,
  };

  test('«todos» edita el mismo recurso sin tocar la regla', async () => {
    const vistas = apuntar();
    await applyRecurringEdit(target, 'all', edited, undefined);
    const input = vistas[0]!.body.eventInput!;
    expect(input.recurrenceUnchanged).toBe(true);
    expect(input.rruleRaw).toBe('FREQ=WEEKLY;BYDAY=MO,WE');
    expect(input.color).toBe('#6B3E9E');
  });

  test('«este y los siguientes» arrastra la regla tal cual a la serie nueva', async () => {
    const vistas = apuntar();
    await applyRecurringEdit(target, 'following', edited, undefined);
    const nueva = vistas[0]!.body.eventInput!;
    expect(nueva.rruleRaw).toBe('FREQ=WEEKLY;BYDAY=MO,WE');
    // Es un recurso nuevo: no hay regla guardada que conservar.
    expect(nueva.recurrenceUnchanged).toBeUndefined();
    expect(vistas[1]!.url).toContain('amend-recurrence');
  });

  test('«solo este» sale sin repetición', async () => {
    const vistas = apuntar();
    await applyRecurringEdit(target, 'this', edited, undefined);
    expect(vistas[0]!.body.eventInput!.rruleRaw).toBeUndefined();
  });

  test('con COUNT no se ofrece «este y los siguientes»', () => {
    expect(permiteSiguientes('FREQ=WEEKLY;BYDAY=MO;COUNT=10')).toBe(false);
    expect(permiteSiguientes('FREQ=WEEKLY;BYDAY=MO;UNTIL=20261231T235959Z')).toBe(true);
    expect(permiteSiguientes(undefined)).toBe(true);
  });
});

/**
 * «Todos» en una serie de día completo: el desfase y la duración van en días de
 * calendario. En milisegundos, un maestro que cae en el cambio de hora (25 oct 2026 en
 * Madrid) se guardaba con el fin a las 23:00 de su mismo día, y el DTEND exclusivo
 * acababa igual al DTSTART.
 */
describe('«todos» en una serie de día completo', () => {
  const fetchReal = globalThis.fetch;
  const tzReal = process.env.TZ;
  afterEach(() => {
    globalThis.fetch = fetchReal;
    if (tzReal === undefined) delete process.env.TZ;
    else process.env.TZ = tzReal;
  });

  test('cruzar un cambio de hora no recorta el día', async () => {
    process.env.TZ = 'Europe/Madrid';
    const vistas: EventInput[] = [];
    globalThis.fetch = (async (_url: string | URL, init?: RequestInit) => {
      vistas.push(JSON.parse(String(init?.body ?? '{}')).eventInput);
      return new Response('{"ok":true}', { status: 200 });
    }) as unknown as typeof fetch;
    const serie: RecurTarget = {
      calendarId: 'c1',
      calendarUrl: '/dav/calendars/yo/personal/',
      uid: 'serie-dia',
      href: '/dav/calendars/yo/personal/serie-dia.ics',
      masterIcal: 'BEGIN:VEVENT\r\nDTSTART;VALUE=DATE:20261025\r\nRRULE:FREQ=WEEKLY\r\nEND:VEVENT',
      recurrenceId: new Date(2026, 10, 8),
      allDay: true,
    };
    const dia: EventInput = {
      summary: 'Guardia',
      allDay: true,
      start: '2026-11-08T00:00',
      end: '2026-11-09T00:00',
    };
    await applyRecurringEdit(serie, 'all', dia, { freq: 'WEEKLY' });
    expect(vistas[0]!.start).toBe('2026-10-25T00:00');
    expect(vistas[0]!.end).toBe('2026-10-26T00:00');

    // Mover la instancia un día mueve la serie un día, también sobre el cambio de hora.
    await applyRecurringEdit(
      serie,
      'all',
      { ...dia, start: '2026-11-09T00:00', end: '2026-11-11T00:00' },
      { freq: 'WEEKLY' },
    );
    expect(vistas[1]!.start).toBe('2026-10-26T00:00');
    expect(vistas[1]!.end).toBe('2026-10-28T00:00');
  });
});
