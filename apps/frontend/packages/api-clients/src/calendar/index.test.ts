// Qué pide el SDK de Calendar y qué entiende de lo que le contestan.
//
// Todo lo de aquí es regresión de fallos que NO daban error en la consola: el
// SDK hablaba un idioma y el servicio otro, así que la pantalla salía vacía o
// la acción no ocurría, y ninguna de las dos cosas parece un fallo del cliente.
// Los cuatro casos se midieron contra `services/calendar/internal/http`.
// Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { CalendarClient, parseVevent, type Event } from './index.js';

interface Llamada {
  method: string;
  path: string;
  body: unknown;
}

/** Cliente con un fetch que apunta lo que se le pide y contesta por ruta. */
function clienteQueApunta(respuestas: Record<string, unknown>): {
  cliente: CalendarClient;
  llamadas: Llamada[];
} {
  const llamadas: Llamada[] = [];
  const fetchImpl = (async (url: string | URL, init?: RequestInit) => {
    const u = new URL(String(url));
    const path = u.pathname.replace('/api/v1', '');
    llamadas.push({
      method: init?.method ?? 'GET',
      path,
      body: init?.body ? JSON.parse(String(init.body)) : undefined,
    });
    const cuerpo = respuestas[`${init?.method ?? 'GET'} ${path}`] ?? respuestas[path];
    if (cuerpo === undefined) {
      return new Response(JSON.stringify({ error: 'not found' }), { status: 404 });
    }
    return new Response(JSON.stringify(cuerpo), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as unknown as typeof fetch;
  return {
    cliente: new CalendarClient({
      baseUrl: 'https://calendar.test',
      auth: { getAccessToken: () => 'tok' },
      fetchImpl,
    }),
    llamadas,
  };
}

const CRUDO = {
  id: 'e1',
  uid: 'u1',
  calendar_id: 'c1',
  summary: 'Comité',
  dtstart: '2026-09-09T10:00:00+02:00',
  dtend: '2026-09-09T11:00:00+02:00',
  location: 'Sala 2',
  all_day: false,
};

test('createEvent crea DENTRO del calendario, que es la única ruta que existe', async () => {
  const { cliente, llamadas } = clienteQueApunta({ 'POST /calendars/c1/events': CRUDO });
  const ev = await cliente.createEvent({
    calendarId: 'c1',
    summary: 'Comité',
    start: '2026-09-09T10:00:00+02:00',
    end: '2026-09-09T11:00:00+02:00',
    attendees: ['ana@sytue.eu'],
  });
  expect(llamadas[0]!.path).toBe('/calendars/c1/events');
  expect(llamadas[0]!.method).toBe('POST');
  // El calendario va en la ruta, no en el cuerpo; los asistentes son objetos.
  const cuerpo = llamadas[0]!.body as Record<string, unknown>;
  expect(cuerpo['calendarId']).toBeUndefined();
  expect(cuerpo['attendees']).toEqual([{ email: 'ana@sytue.eu' }]);
  expect(ev.start).toBe('2026-09-09T10:00:00+02:00');
  expect(ev.calendarId).toBe('c1');
});

test('una fecha sin zona se rechaza aquí y no con un 400 del servicio', () => {
  const { cliente } = clienteQueApunta({});
  expect(
    cliente.createEvent({ calendarId: 'c1', summary: 'x', start: '2026-09-09T10:00:00' }),
  ).rejects.toThrow(/zona horaria/);
});

test('updateEvent lee antes de escribir: cambiar el título no borra la hora', async () => {
  const { cliente, llamadas } = clienteQueApunta({
    'GET /events/e1': CRUDO,
    'PUT /events/e1': { ...CRUDO, summary: 'Comité anual' },
  });
  const ev = await cliente.updateEvent('e1', { summary: 'Comité anual' });
  expect(llamadas.map((l) => l.method)).toEqual(['GET', 'PUT']);
  const enviado = llamadas[1]!.body as Record<string, unknown>;
  expect(enviado['summary']).toBe('Comité anual');
  expect(enviado['start']).toBe('2026-09-09T10:00:00+02:00');
  expect(enviado['location']).toBe('Sala 2');
  expect(ev.summary).toBe('Comité anual');
});

test('searchEvents traduce dtstart: quien lee .start no recibe undefined', async () => {
  const { cliente } = clienteQueApunta({ '/events/search': { events: [CRUDO], next_cursor: '' } });
  const [ev] = (await cliente.searchEvents('comité')) as [Event];
  expect(ev.start).toBe('2026-09-09T10:00:00+02:00');
  expect(ev.end).toBe('2026-09-09T11:00:00+02:00');
  expect(ev.calendarId).toBe('c1');
  expect(ev.summary).toBe('Comité');
});

test('listEvents pide instancias por calendario y ordena por fecha', async () => {
  const tarde = { ...CRUDO, id: 'e2', dtstart: '2026-09-09T16:00:00+02:00' };
  const { cliente, llamadas } = clienteQueApunta({
    '/calendars': { calendars: [{ id: 'c1' }, { id: 'c2' }] },
    '/calendars/c1/instances': { instances: [{ event: tarde, start: tarde.dtstart }] },
    '/calendars/c2/instances': { instances: [{ event: CRUDO, start: CRUDO.dtstart }] },
  });
  const out = await cliente.listEvents({
    from: '2026-09-09T00:00:00Z',
    to: '2026-09-10T00:00:00Z',
  });
  expect(out.map((e) => e.id)).toEqual(['e1', 'e2']);
  expect(llamadas.some((l) => l.path === '/events')).toBe(false);
});

test('listCalendars lee { calendars } y marca el de por defecto', async () => {
  const { cliente } = clienteQueApunta({
    '/calendars': {
      calendars: [
        { id: 'c1', name: 'personal', display_name: 'Personal', is_default: true },
        { id: 'c2', name: 'festivos', read_only: true },
      ],
    },
  });
  expect(await cliente.listCalendars()).toEqual([
    { id: 'c1', displayName: 'Personal', isDefault: true },
    { id: 'c2', displayName: 'festivos', readOnly: true },
  ]);
});

test('parseVevent solo da por enlace de reunión un CONFERENCE http(s)', () => {
  const con = (valor: string): string | undefined =>
    parseVevent(`BEGIN:VEVENT\nUID:1\nCONFERENCE;VALUE=URI;FEATURE=VIDEO:${valor}\nEND:VEVENT`)
      .conferenceUrl;
  expect(con('https://rooms.sytue.eu/rooms/acme/r/abc')).toBe(
    'https://rooms.sytue.eu/rooms/acme/r/abc',
  );
  // Acaba como `href` de «Unirse»: lo que no sea un enlace web no llega.
  expect(con('javascript:alert(1)')).toBeUndefined();
  expect(con('data:text/html,<script>alert(1)</script>')).toBeUndefined();
  expect(con('zoommtg://zoom.us/join?confno=1')).toBeUndefined();
  expect(con('https://ok.example/a b')).toBeUndefined();
});
