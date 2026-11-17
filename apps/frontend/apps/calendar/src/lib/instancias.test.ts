/**
 * La web pinta las repeticiones que calcula el servicio, como Fyne.
 *
 * El BFF `/api/events` pedía solo el `REPORT` CalDAV (los maestros con su RRULE) y la
 * web los expandía con un motor propio sin BYDAY ni YEARLY, y con un «cada mes el 31»
 * que caía en el 1–3 del mes siguiente. Ahora las fechas salen de
 * `/calendars/{id}/instances` (el motor Go de Fyne) y el `REPORT` solo pone el
 * href/etag/iCal con el que se edita. Aquí se finge el servicio con las instancias
 * que calcula su motor para cada regla (comprobadas contra `domain.RRule.Expand`).
 *
 * Ejecutar: `bun test`.
 */
import { afterEach, describe, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';
import type { CalendarEvent } from '@sytue/api-clients';

mock.module('./session.js', () => ({
  getApiSession: () => Promise.resolve({ accessToken: 't', user: { id: 'ana' } }),
}));

const { GET } = await import('../pages/api/events.js');
const { ocurrenciasDeInstancias, rfc3339DeIcal } = await import('./instancias.js');
const { listEvents } = await import('../client/calendarApi.js');

const fetchReal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchReal;
});

/** Un recurso .ics con un VEVENT. */
function ics(uid: string, lineas: string[]): string {
  return [
    'BEGIN:VCALENDAR',
    'VERSION:2.0',
    'BEGIN:VEVENT',
    `UID:${uid}`,
    ...lineas,
    'END:VEVENT',
    'END:VCALENDAR',
  ].join('\r\n');
}

function multistatus(recursos: { href: string; etag: string; ical: string }[]): string {
  const respuestas = recursos
    .map(
      (r) => `<d:response><d:href>${r.href}</d:href><d:propstat><d:prop>
<d:getetag>${r.etag}</d:getetag><c:calendar-data>${r.ical}</c:calendar-data>
</d:prop></d:propstat></d:response>`,
    )
    .join('');
  return `<?xml version="1.0"?><d:multistatus xmlns:d="DAV:" xmlns:c="urn:ietf:params:xml:ns:caldav">${respuestas}</d:multistatus>`;
}

interface Pedida {
  method: string;
  url: string;
}

/** El servicio: el REPORT da los maestros y `/instances` las fechas. */
function servicio(
  recursos: { href: string; etag: string; ical: string }[],
  instancias: unknown[],
  estadoInstancias = 200,
): Pedida[] {
  const pedidas: Pedida[] = [];
  globalThis.fetch = ((url: string | URL | Request, init?: RequestInit) => {
    const u = String(url instanceof Request ? url.url : url);
    const method = init?.method ?? (url instanceof Request ? url.method : 'GET');
    pedidas.push({ method, url: u });
    if (method === 'REPORT')
      return Promise.resolve(new Response(multistatus(recursos), { status: 207 }));
    return Promise.resolve(
      new Response(JSON.stringify({ instances: instancias }), { status: estadoInstancias }),
    );
  }) as unknown as typeof fetch;
  return pedidas;
}

async function eventos(
  params: Record<string, string>,
): Promise<{ status: number; events: CalendarEvent[] }> {
  const url = new URL('https://calendar.sytue.eu/calendar/api/events');
  for (const [k, v] of Object.entries(params)) url.searchParams.set(k, v);
  const res = (await GET({ url, request: new Request(url) } as unknown as APIContext)) as Response;
  const body = (await res.json()) as { events?: CalendarEvent[] };
  return { status: res.status, events: body.events ?? [] };
}

const RANGO = {
  calendarUrl: '/dav/calendars/ana/personal/',
  calendarId: 'cal-1',
  start: '20260101T000000Z',
  end: '20261231T000000Z',
};

const inst = (uid: string, start: string, end: string, extra: object = {}) => ({
  event: { uid, summary: 'x', ...extra },
  start,
  end,
});

describe('/api/events con el motor del servicio', () => {
  test('pide /instances del calendario con el rango en RFC 3339', async () => {
    const pedidas = servicio([], []);
    await eventos(RANGO);
    const rest = pedidas.find((p) => p.method === 'GET');
    expect(rest?.url).toBe(
      'https://calendar.sytue.eu/api/v1/calendars/cal-1/instances?start=2026-01-01T00%3A00%3A00Z&end=2026-12-31T00%3A00%3A00Z',
    );
  });

  test('un BYDAY=TU,TH se pinta martes y jueves, con el href/etag/iCal del maestro', async () => {
    const ical = ics('clase@x', [
      'DTSTART:20261103T170000Z',
      'DTEND:20261103T180000Z',
      'RRULE:FREQ=WEEKLY;BYDAY=TU,TH',
      'SUMMARY:Clase',
    ]);
    servicio(
      [{ href: '/dav/calendars/ana/personal/clase.ics', etag: '"e1"', ical }],
      [
        inst('clase@x', '2026-11-03T17:00:00Z', '2026-11-03T18:00:00Z'),
        inst('clase@x', '2026-11-05T17:00:00Z', '2026-11-05T18:00:00Z'),
        inst('clase@x', '2026-11-10T17:00:00Z', '2026-11-10T18:00:00Z'),
      ],
    );
    const { events } = await eventos(RANGO);
    expect(events.map((e) => e.start)).toEqual([
      '20261103T170000Z',
      '20261105T170000Z',
      '20261110T170000Z',
    ]);
    // Los días de la semana: martes (2) y jueves (4).
    expect(events.map((e) => new Date(rfc3339DeIcal(e.start)!).getUTCDay())).toEqual([2, 4, 2]);
    for (const e of events) {
      expect(e.href).toBe('/dav/calendars/ana/personal/clase.ics');
      expect(e.etag).toBe('"e1"');
      expect(e.ical).toContain('RRULE:FREQ=WEEKLY;BYDAY=TU,TH');
      expect(e.summary).toBe('Clase');
    }
    expect(events[1]!.end).toBe('20261105T180000Z');
  });

  test('un MONTHLY del 31 salta los meses sin 31 (no cae el 1–3 del siguiente)', async () => {
    const ical = ics('cierre@x', [
      'DTSTART:20260131T090000Z',
      'DTEND:20260131T100000Z',
      'RRULE:FREQ=MONTHLY;COUNT=4',
    ]);
    servicio(
      [{ href: '/c.ics', etag: '"e"', ical }],
      [
        inst('cierre@x', '2026-01-31T09:00:00Z', '2026-01-31T10:00:00Z'),
        inst('cierre@x', '2026-03-31T09:00:00Z', '2026-03-31T10:00:00Z'),
        inst('cierre@x', '2026-05-31T09:00:00Z', '2026-05-31T10:00:00Z'),
        inst('cierre@x', '2026-07-31T09:00:00Z', '2026-07-31T10:00:00Z'),
      ],
    );
    const { events } = await eventos(RANGO);
    expect(events.map((e) => e.start.slice(0, 8))).toEqual([
      '20260131',
      '20260331',
      '20260531',
      '20260731',
    ]);
  });

  test('un YEARLY de día completo (festivo de una suscripción) cae en su fecha', async () => {
    const ical = ics('navidad@feed', [
      'DTSTART;VALUE=DATE:20201225',
      'DTEND;VALUE=DATE:20201226',
      'RRULE:FREQ=YEARLY',
      'SUMMARY:Navidad',
    ]);
    servicio(
      [{ href: '/dav/calendars/ana/festivos/n.ics', etag: '"f"', ical }],
      [inst('navidad@feed', '2026-12-25T00:00:00Z', '2026-12-26T00:00:00Z', { all_day: true })],
    );
    const { events } = await eventos(RANGO);
    expect(events).toHaveLength(1);
    expect(events[0]!.start).toBe('20261225');
    expect(events[0]!.end).toBe('20261226');
  });

  test('un 403 de /instances sale como 403: la vista aparta ese calendario', async () => {
    servicio([], [], 403);
    expect((await eventos(RANGO)).status).toBe(403);
  });

  test('un 5xx de /instances es un 502', async () => {
    servicio([], [], 500);
    expect((await eventos(RANGO)).status).toBe(502);
  });

  test('sin calendarId salen los maestros del REPORT, como antes', async () => {
    const ical = ics('a@x', ['DTSTART:20260105T090000Z', 'RRULE:FREQ=DAILY']);
    const pedidas = servicio([{ href: '/a.ics', etag: '"a"', ical }], []);
    const { events } = await eventos({
      calendarUrl: RANGO.calendarUrl,
      start: RANGO.start,
      end: RANGO.end,
    });
    expect(events.map((e) => e.start)).toEqual(['20260105T090000Z']);
    expect(pedidas.some((p) => p.method === 'GET')).toBe(false);
  });
});

describe('ocurrenciasDeInstancias', () => {
  const maestro = (start: string, lineas: string[] = []): CalendarEvent => ({
    href: '/m.ics',
    etag: '"m"',
    uid: 'm@x',
    summary: 'Reunión',
    start,
    end: undefined,
    ical: ics('m@x', [`DTSTART${lineas[0] ?? ''}:${start}`, ...lineas.slice(1)]),
  });

  test('con TZID, la hora de pared en su zona (como la entrega el REPORT)', () => {
    const [o] = ocurrenciasDeInstancias(
      [maestro('20260105T090000', [';TZID=Europe/Madrid'])],
      // 9:00 en Madrid en julio (verano) son las 7:00 UTC.
      [inst('m@x', '2026-07-06T07:00:00Z', '2026-07-06T08:00:00Z')],
    );
    expect(o!.start).toBe('20260706T090000');
    expect(o!.end).toBe('20260706T100000');
  });

  test('sin Z ni TZID (flotante), la hora escrita, sin Z: la web la lee en local', () => {
    // El servicio guarda la hora flotante como UTC: su reloj UTC es la del .ics.
    const [o] = ocurrenciasDeInstancias(
      [maestro('20260105T100000')],
      [inst('m@x', '2026-01-12T10:00:00Z', '2026-01-12T11:00:00Z')],
    );
    expect(o!.start).toBe('20260112T100000');
    expect(o!.end).toBe('20260112T110000');
  });

  test('una repetición cambiada en otro cliente (RECURRENCE-ID) lleva su título', () => {
    const [o] = ocurrenciasDeInstancias(
      [maestro('20260105T090000Z')],
      [
        {
          ...inst('m@x', '2026-01-12T11:00:00Z', '2026-01-12T12:00:00Z', {
            summary: 'Reunión (movida)',
            location: 'Sala 2',
          }),
          is_override: true,
        },
      ],
    );
    expect(o!.summary).toBe('Reunión (movida)');
    expect(o!.location).toBe('Sala 2');
    expect(o!.start).toBe('20260112T110000Z');
  });

  test('una instancia sin maestro en el REPORT no se pinta (no se podría editar)', () => {
    expect(
      ocurrenciasDeInstancias([], [inst('otro@x', '2026-01-12T11:00:00Z', '2026-01-12T12:00:00Z')]),
    ).toEqual([]);
  });
});

describe('listEvents (navegador)', () => {
  test('manda el id REST del calendario para que el BFF expanda con el servicio', async () => {
    let pedida = '';
    globalThis.fetch = ((url: string) => {
      pedida = url;
      return Promise.resolve(new Response('{"events":[]}', { status: 200 }));
    }) as unknown as typeof fetch;
    await listEvents(
      { url: '/dav/calendars/ana/personal/', id: 'cal-1' },
      { start: 'a', end: 'b' },
    );
    const q = new URL(pedida, 'https://x').searchParams;
    expect(q.get('calendarId')).toBe('cal-1');
    expect(q.get('calendarUrl')).toBe('/dav/calendars/ana/personal/');
  });
});
