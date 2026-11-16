// El indicador de hilo y «También enviar al canal» (D-ROOM-05): el mapeo del servicio
// y cómo cambia la raíz al llegar o fallar una respuesta.
import { expect, test } from 'bun:test';
import type { ChatMessage } from './chat.js';
import {
  anotarRespuesta,
  deshacerRespuesta,
  esDifundida,
  MAX_PARTICIPANTES,
  threadInfo,
} from './hilos.js';

function msg(over: Partial<ChatMessage>): ChatMessage {
  return {
    id: 'm',
    channelId: 'c',
    author: { id: 'ana', name: 'Ana' },
    body: 'x',
    createdAt: '2026-11-10T10:00:00Z',
    ...over,
  };
}

test('threadInfo mapea el indicador del servicio y omite lo que no viene', () => {
  expect(threadInfo({})).toEqual({});
  expect(
    threadInfo({
      last_reply_at: '2026-11-10T10:05:00Z',
      thread_participants: [{ sub: 'bea', name: 'Bea' }, { sub: 'carl' }],
      also_in_channel: true,
    }),
  ).toEqual({
    lastReplyAt: '2026-11-10T10:05:00Z',
    threadParticipants: [
      { id: 'bea', name: 'Bea' },
      { id: 'carl', name: 'carl' },
    ],
    alsoInChannel: true,
  });
});

test('solo una respuesta marcada es «también al canal»', () => {
  expect(esDifundida(msg({ alsoInChannel: true }))).toBe(false);
  expect(esDifundida(msg({ parentId: 'r' }))).toBe(false);
  expect(esDifundida(msg({ parentId: 'r', alsoInChannel: true }))).toBe(true);
});

test('una respuesta nueva pone a su autor el primero, sin repetir y con tope', () => {
  const raiz = msg({
    id: 'r',
    replyCount: 3,
    lastReplyAt: '2026-11-10T10:03:00Z',
    threadParticipants: [
      { id: 'bea', name: 'Bea' },
      { id: 'carl', name: 'Carl' },
      { id: 'dani', name: 'Dani' },
    ],
  });
  const r = anotarRespuesta(
    raiz,
    msg({
      id: 'x',
      parentId: 'r',
      author: { id: 'carl', name: 'Carl' },
      createdAt: '2026-11-10T10:09:00Z',
    }),
  );
  expect(r.replyCount).toBe(4);
  expect(r.lastReplyAt).toBe('2026-11-10T10:09:00Z');
  expect(r.threadParticipants?.map((p) => p.id)).toEqual(['carl', 'bea', 'dani']);

  const nueva = anotarRespuesta(
    r,
    msg({
      id: 'y',
      parentId: 'r',
      author: { id: 'eva', name: 'Eva' },
      createdAt: '2026-11-10T10:10:00Z',
    }),
  );
  expect(nueva.threadParticipants).toHaveLength(MAX_PARTICIPANTES);
  expect(nueva.threadParticipants?.[0]).toEqual({ id: 'eva', name: 'Eva' });
});

test('una respuesta más antigua que la última solo suma a la cuenta', () => {
  const raiz = msg({
    id: 'r',
    replyCount: 1,
    lastReplyAt: '2026-11-10T10:05:00Z',
    threadParticipants: [{ id: 'bea', name: 'Bea' }],
  });
  const r = anotarRespuesta(
    raiz,
    msg({ parentId: 'r', author: { id: 'eva', name: 'Eva' }, createdAt: '2026-11-10T10:01:00Z' }),
  );
  expect(r.replyCount).toBe(2);
  expect(r.lastReplyAt).toBe('2026-11-10T10:05:00Z');
  expect(r.threadParticipants).toEqual([{ id: 'bea', name: 'Bea' }]);
});

test('deshacer devuelve la cuenta y el indicador de antes', () => {
  const sinHilo = msg({ id: 'r' });
  const con = anotarRespuesta(sinHilo, msg({ parentId: 'r', author: { id: 'eva', name: 'Eva' } }));
  const vuelta = deshacerRespuesta(con, sinHilo);
  expect(vuelta.replyCount).toBe(0);
  expect(vuelta.lastReplyAt).toBeUndefined();
  expect(vuelta.threadParticipants).toBeUndefined();
});
