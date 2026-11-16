import { expect, test } from 'bun:test';
import {
  correosDeInvitados,
  enlaceProgramarReunion,
  MAX_INVITADOS,
  abrirEnCalendar,
  participantesDeReunion,
} from './reunion.js';
import { channelLink } from '../client/chatApi.js';

test('en un canal invita a su roster sin mí ni las invitaciones pendientes', () => {
  const roster = [
    { sub: 'yo' },
    { sub: 'ana' },
    { sub: 'beto', pending: true },
    { sub: 'caro' },
    { sub: 'ana' },
  ];
  expect(participantesDeReunion({ kind: 'public' }, roster, 'yo')).toEqual(['ana', 'caro']);
});

test('en un chat invita a sus otros miembros, aunque no haya roster', () => {
  expect(participantesDeReunion({ kind: 'direct', peerSubs: ['ana', 'beto'] }, [], 'yo')).toEqual([
    'ana',
    'beto',
  ]);
  expect(participantesDeReunion({ kind: 'direct' }, [], 'yo')).toEqual([]);
});

test('solo invita a quien tiene correo en el directorio, sin repetir y acotado', () => {
  const fichas = new Map([
    ['ana', { email: ' Ana@Acme.sytue.eu ' }],
    ['ana2', { email: 'ana@acme.sytue.eu' }],
    ['caro', { email: 'caro@acme.sytue.eu' }],
  ]);
  expect(correosDeInvitados(['ana', 'externo', 'ana2', 'caro'], fichas)).toEqual([
    'ana@acme.sytue.eu',
    'caro@acme.sytue.eu',
  ]);

  const muchos = Array.from({ length: MAX_INVITADOS + 20 }, (_, i) => `u${i}`);
  const todos = new Map(muchos.map((s) => [s, { email: `${s}@acme.sytue.eu` }]));
  expect(correosDeInvitados(muchos, todos)).toHaveLength(MAX_INVITADOS);
});

test('el enlace es el intent de creación que Calendar ya lee', () => {
  const url = new URL(
    enlaceProgramarReunion('https://calendar.sytue.eu/acme/', {
      titulo: 'Reunión · #diseño & co',
      descripcion: 'Conversación: https://rooms.sytue.eu/acme/channels/c1',
      invitados: ['ana@acme.sytue.eu', 'caro@acme.sytue.eu'],
    }),
  );
  expect(url.origin + url.pathname).toBe('https://calendar.sytue.eu/acme/');
  expect(url.searchParams.get('action')).toBe('create');
  expect(url.searchParams.get('title')).toBe('Reunión · #diseño & co');
  expect(url.searchParams.get('description')).toBe(
    'Conversación: https://rooms.sytue.eu/acme/channels/c1',
  );
  expect(url.searchParams.get('attendees')).toBe('ana@acme.sytue.eu,caro@acme.sytue.eu');
});

test('sin invitados ni descripción no manda parámetros vacíos', () => {
  const url = new URL(enlaceProgramarReunion('https://calendar.sytue.eu', { titulo: 'X' }));
  expect(url.searchParams.has('attendees')).toBe(false);
  expect(url.searchParams.has('description')).toBe(false);
});

test('el enlace al canal vuelve a la conversación con prefijo y organización', () => {
  expect(channelLink('c 1', 'https://sytue.eu/rooms/acme')).toBe(
    'https://sytue.eu/rooms/acme/channels/c%201',
  );
});

function entorno(fichas: () => Promise<ReadonlyMap<string, { sub: string; email: string }>>) {
  const pasos: string[] = [];
  const ventana = { location: { href: '' }, opener: {} as unknown };
  return {
    pasos,
    ventana,
    e: {
      abrir: () => {
        pasos.push('abrir');
        return ventana as unknown as Window;
      },
      irAqui: (url: string) => pasos.push(`aquí ${url}`),
      fichas: () => {
        pasos.push('directorio');
        return fichas();
      },
    },
  };
}

test('abre la pestaña antes de esperar al directorio y la lleva a Calendar con invitados', async () => {
  const { pasos, ventana, e } = entorno(() =>
    Promise.resolve(new Map([['ana', { sub: 'ana', email: 'ana@acme.sytue.eu' }]])),
  );
  await abrirEnCalendar('https://calendar.sytue.eu', { titulo: 'Reunión' }, ['ana'], e);
  expect(pasos).toEqual(['abrir', 'directorio']);
  expect(ventana.opener).toBeNull();
  const url = new URL(ventana.location.href);
  expect(url.searchParams.get('attendees')).toBe('ana@acme.sytue.eu');
});

test('si el directorio falla, Calendar se abre igual sin invitados', async () => {
  const { ventana, e } = entorno(() => Promise.reject(new Error('502')));
  await abrirEnCalendar('https://calendar.sytue.eu', { titulo: 'Reunión' }, ['ana'], e);
  const url = new URL(ventana.location.href);
  expect(url.searchParams.get('title')).toBe('Reunión');
  expect(url.searchParams.has('attendees')).toBe(false);
});

test('sin pestaña nueva (bloqueada) va en esta, y sin nadie a quien invitar no pide el directorio', async () => {
  const { pasos, e } = entorno(() => Promise.resolve(new Map()));
  await abrirEnCalendar('https://calendar.sytue.eu', { titulo: 'R' }, [], {
    ...e,
    abrir: () => null,
  });
  expect(pasos).toEqual(['aquí https://calendar.sytue.eu/?action=create&title=R']);
});

test('si el directorio no contesta, Calendar se abre igual pasado el plazo', async () => {
  const { ventana, e } = entorno(() => new Promise(() => {}));
  await abrirEnCalendar('https://calendar.sytue.eu', { titulo: 'Reunión' }, ['ana'], {
    ...e,
    esperaMs: 5,
  });
  const url = new URL(ventana.location.href);
  expect(url.searchParams.get('title')).toBe('Reunión');
  expect(url.searchParams.has('attendees')).toBe(false);
});
