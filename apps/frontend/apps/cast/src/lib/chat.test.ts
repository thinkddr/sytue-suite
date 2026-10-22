import { expect, test } from 'bun:test';
import { MAX_MENSAJES, aplicar, leerTrama, unir, urlDelChat } from './chat.js';
import type { MensajeChat } from './tipos.js';

const m = (id: string, t = '2026-09-27T10:00:00Z'): MensajeChat => ({
  id,
  author: { sub: 's', name: 'Ana' },
  body: 'hola',
  created_at: t,
});

test('lee las cuatro tramas del servidor y descarta lo que no entiende', () => {
  expect(leerTrama(JSON.stringify({ type: 'message', ...m('1') }))?.type).toBe('message');
  expect(leerTrama('{"type":"deleted","id":"1"}')).toEqual({ type: 'deleted', id: '1' });
  expect(leerTrama('{"type":"viewers","count":12}')).toEqual({ type: 'viewers', count: 12 });
  expect(leerTrama('{"type":"error","code":"too_fast"}')).toEqual({
    type: 'error',
    code: 'too_fast',
  });
  expect(leerTrama('no es json')).toBeNull();
  expect(leerTrama('{"type":"message"}')).toBeNull();
  expect(leerTrama(new ArrayBuffer(2))).toBeNull();
});

test('un mensaje repetido (historial y socket) no se duplica; un borrado se va', () => {
  let l = aplicar([m('1')], { type: 'message', ...m('1') });
  expect(l).toHaveLength(1);
  l = aplicar(l, { type: 'message', ...m('2') });
  l = aplicar(l, { type: 'deleted', id: '1' });
  expect(l.map((x) => x.id)).toEqual(['2']);
});

test('la lista no crece sin fin', () => {
  let l: readonly MensajeChat[] = [];
  for (let i = 0; i < MAX_MENSAJES + 10; i++) l = aplicar(l, { type: 'message', ...m(String(i)) });
  expect(l).toHaveLength(MAX_MENSAJES);
  expect(l[0]!.id).toBe('10');
});

test('unir ordena por fecha y quita repetidos', () => {
  const u = unir(
    [m('b', '2026-09-27T10:00:02Z'), m('a', '2026-09-27T10:00:01Z')],
    [m('b', '2026-09-27T10:00:02Z')],
  );
  expect(u.map((x) => x.id)).toEqual(['a', 'b']);
});

test('la URL del socket lleva el token codificado', () => {
  expect(urlDelChat('wss://cast.sytue.eu/', 'v1', 'a.b+c')).toBe(
    'wss://cast.sytue.eu/ws/videos/v1/chat?access_token=a.b%2Bc',
  );
});
