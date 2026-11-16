/**
 * «Deshacer» tras mover, redimensionar o borrar un evento: qué se puede deshacer,
 * cómo se vuelve al estado de antes y qué .ics acepta el BFF para recrear uno
 * borrado (PUT /api/event).
 *
 * Ejecutar: `bun test`.
 */
import { afterEach, describe, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';
import {
  MAX_ICS_RESTAURADO,
  entradaConHorario,
  horarioAnterior,
  icsRestaurable,
  puedeDeshacerBorrado,
  type EventoDeshacible,
} from './deshacer.js';

mock.module('./session.js', () => ({
  getApiSession: () =>
    Promise.resolve({ accessToken: 't', user: { id: 'yo', email: 'yo@sytue.eu' } }),
}));

const { PUT } = await import('../pages/api/event.js');

const HREF = '/dav/calendars/yo/personal/dentista-1.ics';

/** Un evento suelto con un recordatorio, como lo guarda el servicio. */
const SUELTO =
  [
    'BEGIN:VCALENDAR',
    'VERSION:2.0',
    'PRODID:-//Sytue One//Sytue Calendar//ES',
    'BEGIN:VEVENT',
    'UID:dentista-1',
    'DTSTART:20261109T090000Z',
    'DTEND:20261109T100000Z',
    'SUMMARY:Dentista',
    'BEGIN:VALARM',
    'ACTION:DISPLAY',
    'TRIGGER:-PT15M',
    'END:VALARM',
    'END:VEVENT',
    'END:VCALENDAR',
  ].join('\r\n') + '\r\n';

const CON_INVITADOS = SUELTO.replace(
  'SUMMARY:Dentista',
  'SUMMARY:Reunión\r\nORGANIZER:mailto:yo@sytue.eu\r\nATTENDEE;PARTSTAT=ACCEPTED:mailto:ana@ejemplo.eu',
);
const SERIE = SUELTO.replace('SUMMARY:Dentista', 'SUMMARY:Dentista\r\nRRULE:FREQ=WEEKLY');

function evento(ical: string, extra: Partial<EventoDeshacible> = {}): EventoDeshacible {
  return {
    uid: 'dentista-1',
    summary: 'Dentista',
    start: '20261109T090000Z',
    end: '20261109T100000Z',
    ical,
    href: HREF,
    calendarUrl: '/dav/calendars/yo/personal/',
    ...extra,
  };
}

describe('deshacer un movimiento', () => {
  test('devuelve el inicio y el fin que tenía antes del arrastre', () => {
    const antes = horarioAnterior(
      evento(SUELTO),
      new Date(Date.UTC(2026, 10, 9, 14)),
      new Date(Date.UTC(2026, 10, 9, 15)),
    );
    expect(antes?.start.toISOString()).toBe('2026-11-09T09:00:00.000Z');
    expect(antes?.end.toISOString()).toBe('2026-11-09T10:00:00.000Z');
  });

  test('sin DTEND, conserva la duración del bloque', () => {
    const antes = horarioAnterior(
      evento(SUELTO, { end: '' }),
      new Date(Date.UTC(2026, 10, 9, 14)),
      new Date(Date.UTC(2026, 10, 9, 14, 30)),
    );
    expect(antes?.end.toISOString()).toBe('2026-11-09T09:30:00.000Z');
  });

  test('un día completo vuelve como día completo, con su fin exclusivo', () => {
    // Arrastrado en el mes del 9 al 11: deshacer lo devuelve al 9 sin volverlo «con hora».
    const diaCompleto = evento(SUELTO, { start: '20261109', end: '20261110' });
    const antes = horarioAnterior(diaCompleto, new Date(2026, 10, 11), new Date(2026, 10, 12));
    expect(antes?.start).toEqual(new Date(2026, 10, 9));
    expect(antes?.end).toEqual(new Date(2026, 10, 10));
    const entrada = entradaConHorario(diaCompleto, antes!.start, antes!.end);
    expect(entrada.allDay).toBe(true);
    expect(entrada.start).toBe('2026-11-09T00:00');
    expect(entrada.end).toBe('2026-11-10T00:00');
  });

  test('un evento con hora no se marca como día completo', () => {
    const entrada = entradaConHorario(
      evento(SUELTO),
      new Date(2026, 10, 9, 9),
      new Date(2026, 10, 9, 10),
    );
    expect(entrada.allDay).toBeUndefined();
  });

  test('la entrada de deshacer conserva invitados, respuestas y recordatorios', () => {
    const inicio = new Date(2026, 10, 9, 9);
    const fin = new Date(2026, 10, 9, 10);
    const entrada = entradaConHorario(evento(CON_INVITADOS), inicio, fin);
    expect(entrada.start).toBe('2026-11-09T09:00');
    expect(entrada.end).toBe('2026-11-09T10:00');
    expect(entrada.uid).toBe('dentista-1');
    expect(entrada.attendees).toEqual(['ana@ejemplo.eu']);
    expect(entrada.partstats).toEqual({ 'ana@ejemplo.eu': 'ACCEPTED' });
    expect(entrada.alarms?.length).toBe(1);
    expect(entrada.recurrenceUnchanged).toBe(true);
  });
});

describe('deshacer un borrado', () => {
  test('se ofrece para un evento suelto sin invitados', () => {
    expect(puedeDeshacerBorrado(evento(SUELTO))).toBe(true);
  });

  test('no se ofrece con invitados: recrearlo les mandaría otra invitación', () => {
    expect(puedeDeshacerBorrado(evento(CON_INVITADOS))).toBe(false);
  });

  test('no se ofrece para una serie, ni para lo que no es tuyo o no tiene .ics', () => {
    expect(puedeDeshacerBorrado(evento(SERIE))).toBe(false);
    expect(puedeDeshacerBorrado(evento(SUELTO, { readOnly: true }))).toBe(false);
    expect(puedeDeshacerBorrado(evento(SUELTO, { href: undefined }))).toBe(false);
    expect(puedeDeshacerBorrado(evento(''))).toBe(false);
  });

  test('el .ics restaurable es el mismo, recordatorios incluidos', () => {
    expect(icsRestaurable(SUELTO)).toBe(SUELTO);
  });

  test('un VEVENT suelto se envuelve en un VCALENDAR', () => {
    const solo = SUELTO.slice(SUELTO.indexOf('BEGIN:VEVENT'), SUELTO.indexOf('END:VCALENDAR'));
    const ics = icsRestaurable(solo);
    expect(ics?.startsWith('BEGIN:VCALENDAR\r\n')).toBe(true);
    expect(ics).toContain('UID:dentista-1');
    expect(ics?.endsWith('END:VCALENDAR\r\n')).toBe(true);
  });

  test('rechaza invitados, METHOD, lo que no es un evento y lo desmedido', () => {
    expect(icsRestaurable(CON_INVITADOS)).toBeNull();
    expect(
      icsRestaurable(SUELTO.replace('VERSION:2.0', 'VERSION:2.0\r\nMETHOD:REQUEST')),
    ).toBeNull();
    expect(icsRestaurable('BEGIN:VCALENDAR\r\nVERSION:2.0\r\nEND:VCALENDAR')).toBeNull();
    expect(icsRestaurable(42)).toBeNull();
    expect(icsRestaurable('x'.repeat(MAX_ICS_RESTAURADO + 1))).toBeNull();
  });
});

describe('PUT /api/event (recrear lo borrado)', () => {
  const fetchReal = globalThis.fetch;
  afterEach(() => {
    globalThis.fetch = fetchReal;
  });

  function servicio(): { method: string; path: string; headers: Headers; body: string }[] {
    const vistas: { method: string; path: string; headers: Headers; body: string }[] = [];
    globalThis.fetch = (async (url: string | URL, init?: RequestInit) => {
      vistas.push({
        method: init?.method ?? 'GET',
        path: new URL(String(url)).pathname,
        headers: new Headers(init?.headers),
        body: typeof init?.body === 'string' ? init.body : '',
      });
      return new Response(null, { status: 201 });
    }) as unknown as typeof fetch;
    return vistas;
  }

  function restaurar(cuerpo: unknown): Promise<Response> {
    const request = new Request('https://calendar.sytue.eu/calendar/api/event', {
      method: 'PUT',
      body: JSON.stringify(cuerpo),
    });
    return PUT({ request } as unknown as APIContext) as Promise<Response>;
  }

  test('vuelve a escribir el .ics en su href, sin pisar nada que ya esté ahí', async () => {
    const vistas = servicio();
    const res = await restaurar({ href: HREF, ical: SUELTO });
    expect(res.status).toBe(200);
    expect(vistas).toHaveLength(1);
    expect(vistas[0]!.method).toBe('PUT');
    expect(vistas[0]!.path).toBe(HREF);
    expect(vistas[0]!.headers.get('if-none-match')).toBe('*');
    expect(vistas[0]!.headers.get('if-match')).toBeNull();
    expect(vistas[0]!.body).toBe(SUELTO);
  });

  test('con invitados o fuera del servicio, 400 y no se escribe nada', async () => {
    const vistas = servicio();
    expect((await restaurar({ href: HREF, ical: CON_INVITADOS })).status).toBe(400);
    expect((await restaurar({ href: 'https://otro.eu/x.ics', ical: SUELTO })).status).toBe(400);
    expect((await restaurar({ href: '//otro.eu/x.ics', ical: SUELTO })).status).toBe(400);
    expect((await restaurar({ href: HREF })).status).toBe(400);
    expect(vistas).toHaveLength(0);
  });
});
