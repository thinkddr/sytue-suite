/**
 * POST /api/event al EDITAR: lee el recurso guardado y funde los cambios encima, en
 * vez de regenerarlo entero desde el formulario.
 *
 * Antes, mover media hora una serie de Apple con BYDAY la dejaba en un evento suelto
 * y borraba sus excepciones y su CLASS:PRIVATE: el PUT llevaba solo lo que el
 * formulario sabe expresar.
 *
 * Ejecutar: `bun test`.
 */
import { afterEach, describe, expect, mock, test } from 'bun:test';
import type { APIContext } from 'astro';

mock.module('./session.js', () => ({
  getApiSession: () =>
    Promise.resolve({ accessToken: 't', user: { id: 'yo', email: 'yo@sytue.eu' } }),
}));

const { POST } = await import('../pages/api/event.js');

const HREF = '/dav/calendars/yo/personal/serie-1.ics';
const GUARDADO =
  [
    'BEGIN:VCALENDAR',
    'VERSION:2.0',
    'BEGIN:VEVENT',
    'UID:serie-1@icloud.com',
    'DTSTART:20261109T090000Z',
    'DTEND:20261109T100000Z',
    'RRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR',
    'CLASS:PRIVATE',
    'SUMMARY:Diario',
    'END:VEVENT',
    'BEGIN:VEVENT',
    'UID:serie-1@icloud.com',
    'RECURRENCE-ID:20261113T090000Z',
    'DTSTART:20261113T110000Z',
    'SUMMARY:Diario (movido)',
    'END:VEVENT',
    'END:VCALENDAR',
  ].join('\r\n') + '\r\n';

interface Peticion {
  method: string;
  path: string;
  headers: Headers;
  body: string;
}

const fetchReal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchReal;
});

/** Servicio CalDAV falso: el GET devuelve `guardado` (o 404) y apunta lo que llega. */
function servicio(guardado: string | null): Peticion[] {
  const vistas: Peticion[] = [];
  globalThis.fetch = (async (url: string | URL, init?: RequestInit) => {
    const method = init?.method ?? 'GET';
    vistas.push({
      method,
      path: new URL(String(url)).pathname,
      headers: new Headers(init?.headers),
      body: typeof init?.body === 'string' ? init.body : '',
    });
    if (method === 'GET') {
      return guardado === null
        ? new Response('no', { status: 404 })
        : new Response(guardado, { status: 200, headers: { etag: '"leido"' } });
    }
    return new Response(null, { status: 201 });
  }) as unknown as typeof fetch;
  return vistas;
}

function guardar(cuerpo: unknown): Promise<Response> {
  const request = new Request('https://calendar.sytue.eu/calendar/api/event', {
    method: 'POST',
    body: JSON.stringify(cuerpo),
  });
  return POST({ request } as unknown as APIContext) as Promise<Response>;
}

const arrastre = {
  calendarUrl: '/dav/calendars/yo/personal/',
  href: HREF,
  eventInput: {
    uid: 'serie-1@icloud.com',
    summary: 'Diario',
    start: '2026-11-09T09:30:00Z',
    end: '2026-11-09T10:30:00Z',
    recurrenceUnchanged: true,
  },
};

describe('POST /api/event al editar', () => {
  test('funde sobre lo guardado: la RRULE con BYDAY, CLASS y la excepción siguen', async () => {
    const vistas = servicio(GUARDADO);
    const res = await guardar({ ...arrastre, etag: '"del-navegador"' });
    expect(res.status).toBe(200);
    expect(vistas.map((v) => `${v.method} ${v.path}`)).toEqual([`GET ${HREF}`, `PUT ${HREF}`]);
    const put = vistas[1]!;
    expect(put.body).toContain('\r\nRRULE:FREQ=WEEKLY;BYDAY=MO,WE,FR\r\n');
    expect(put.body).toContain('\r\nCLASS:PRIVATE\r\n');
    // La serie se mueve media hora y su excepción sigue apuntando a su ocurrencia.
    expect(put.body).toContain('RECURRENCE-ID:20261113T093000Z');
    expect(put.body).toContain('\r\nDTSTART:20261109T093000Z\r\n');
    expect(put.body).toContain('\r\nSEQUENCE:1\r\n');
    // Manda el ETag que vio el navegador: un cambio ajeno posterior da 412, no se pisa.
    expect(put.headers.get('if-match')).toBe('"del-navegador"');
  });

  test('sin ETag del navegador, el PUT lleva el de lo que se acaba de leer', async () => {
    const vistas = servicio(GUARDADO);
    await guardar(arrastre);
    expect(vistas[1]!.headers.get('if-match')).toBe('"leido"');
  });

  test('si el recurso ya no está, se escribe como un evento nuevo', async () => {
    const vistas = servicio(null);
    const res = await guardar({
      ...arrastre,
      eventInput: { ...arrastre.eventInput, rruleRaw: 'FREQ=YEARLY;BYMONTH=11' },
    });
    expect(res.status).toBe(200);
    expect(vistas[1]!.method).toBe('PUT');
    expect(vistas[1]!.body).toContain('\r\nRRULE:FREQ=YEARLY;BYMONTH=11\r\n');
    expect(vistas[1]!.body).not.toContain('CLASS:');
  });

  test('crear (sin href) no lee nada y sale igual que hasta ahora', async () => {
    const vistas = servicio(GUARDADO);
    await guardar({ calendarUrl: '/dav/calendars/yo/personal/', eventInput: arrastre.eventInput });
    expect(vistas.map((v) => v.method)).toEqual(['PUT']);
    expect(vistas[0]!.headers.get('if-none-match')).toBe('*');
  });

  test('un rruleRaw que abriría otra línea se descarta', async () => {
    const vistas = servicio(null);
    await guardar({
      ...arrastre,
      eventInput: { ...arrastre.eventInput, rruleRaw: 'FREQ=DAILY\r\nCLASS:PUBLIC' },
    });
    expect(vistas[1]!.body).not.toContain('CLASS:PUBLIC');
  });

  test('otro fallo al leer no escribe nada', async () => {
    const vistas: string[] = [];
    globalThis.fetch = (async (_u: string | URL, init?: RequestInit) => {
      vistas.push(init?.method ?? 'GET');
      return new Response('x', { status: 403 });
    }) as unknown as typeof fetch;
    const res = await guardar(arrastre);
    expect(res.status).toBe(403);
    expect(vistas).toEqual(['GET']);
  });
});
