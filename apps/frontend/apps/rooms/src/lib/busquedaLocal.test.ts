/*
 * BUSCAR EN LOS CANALES CIFRADOS, EN EL DISPOSITIVO (D-ROOM-01).
 *
 * El servidor no puede leer los mensajes: se busca en la historia ya descifrada que tiene
 * el cliente, con los mismos operadores que el backend y sin acentos ni mayúsculas.
 */
import { expect, test } from 'bun:test';
import type { ChatMessage } from './chat.js';
import {
  buscarEnLocal,
  cargarHistoriaDelCanal,
  cargarHistoriaLocal,
  MAX_ARCHIVOS_CANAL,
  MENSAJES_ARCHIVOS_CANAL,
  MENSAJES_POR_CANAL,
} from './busquedaLocal.js';

function msg(id: string, body: string, extra: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id,
    channelId: 'c1',
    author: { id: 'ana', name: 'Ana Pérez' },
    body,
    createdAt: `2026-05-0${id.slice(-1)}T10:00:00Z`,
    ...extra,
  };
}

const canales = { c1: 'equipo', c2: 'ventas' };
const todos = [
  msg('m1', 'La Reunión es mañana'),
  msg('m2', 'reunion movida', { channelId: 'c2', author: { id: 'luis', name: 'Luis' } }),
  msg('m3', 'nada que ver'),
  msg('m4', 'reunión secreta', { decryptionFailed: true }),
  msg('m5', 'el informe', {
    attachments: [{ fileId: 'f', name: 'acta.pdf', contentType: 'application/pdf', sizeBytes: 1 }],
  }),
];

test('encuentra sin acentos ni mayúsculas, lo más reciente primero, y salta lo no descifrado', () => {
  expect(buscarEnLocal('REUNION', todos, canales).map((m) => m.id)).toEqual(['m2', 'm1']);
});

test('respeta in:, from:, has:file y las fechas', () => {
  expect(buscarEnLocal('reunion in:#ventas', todos, canales).map((m) => m.id)).toEqual(['m2']);
  expect(buscarEnLocal('from:@ana reunion', todos, canales).map((m) => m.id)).toEqual(['m1']);
  expect(buscarEnLocal('has:file', todos, canales).map((m) => m.id)).toEqual(['m5']);
  expect(buscarEnLocal('acta', todos, canales).map((m) => m.id)).toEqual(['m5']);
  expect(buscarEnLocal('reunion after:2026-05-01', todos, canales).map((m) => m.id)).toEqual([
    'm2',
  ]);
});

test('carga la última página de cada canal y un canal caído no tumba los demás', async () => {
  const muchos = Array.from({ length: MENSAJES_POR_CANAL + 5 }, (_, i) => msg(`x${i}`, 'a'));
  const h = await cargarHistoriaLocal(['c1', 'roto'], (id) =>
    id === 'roto' ? Promise.reject(new Error('x')) : Promise.resolve({ messages: muchos }),
  );
  expect(h.length).toBe(MENSAJES_POR_CANAL);
  expect(h.at(-1)?.id).toBe(`x${MENSAJES_POR_CANAL + 4}`);
});

// D-ROOM-03: «Archivos del canal» recorre hacia atrás SOLO ese canal, hasta mil mensajes.
function canalDe(n: number): ChatMessage[] {
  return Array.from({ length: n }, (_, i) => ({
    id: `x${i}`,
    channelId: 'c1',
    author: { id: 'ana', name: 'Ana' },
    body: `m${i}`,
    createdAt: new Date(Date.UTC(2026, 0, 1) + i * 60_000).toISOString(),
    ...(i % 100 === 0
      ? {
          attachments: [
            { fileId: `f${i}`, name: `f${i}.pdf`, contentType: 'application/pdf', sizeBytes: 1 },
          ],
        }
      : {}),
  }));
}

function paginador(historia: ChatMessage[], llamadas: (string | undefined)[]) {
  return (_id: string, before?: string) => {
    llamadas.push(before);
    const previos = before ? historia.filter((m) => m.createdAt < before) : historia;
    return Promise.resolve({ messages: previos.slice(-MENSAJES_POR_CANAL) });
  };
}

test('archivos del canal: pagina hacia atrás hasta el tope de mil y encuentra adjuntos viejos', async () => {
  const llamadas: (string | undefined)[] = [];
  const reunidos = await cargarHistoriaDelCanal('c1', paginador(canalDe(1500), llamadas));
  expect(reunidos).toHaveLength(MENSAJES_ARCHIVOS_CANAL);
  expect(reunidos[0]!.id).toBe('x500');
  expect(reunidos.at(-1)!.id).toBe('x1499');
  expect(llamadas).toHaveLength(MENSAJES_ARCHIVOS_CANAL / MENSAJES_POR_CANAL);
  const ficheros = buscarEnLocal('in:#equipo has:file', reunidos, canales, MAX_ARCHIVOS_CANAL);
  // La última página sola (50 mensajes) no tendría ninguno: estos están muy atrás.
  expect(ficheros.map((m) => m.id)).toEqual([
    'x1400',
    'x1300',
    'x1200',
    'x1100',
    'x1000',
    'x900',
    'x800',
    'x700',
    'x600',
    'x500',
  ]);
});

test('archivos del canal: se para al llegar al principio y aguanta un fallo a mitad', async () => {
  const llamadas: (string | undefined)[] = [];
  expect(await cargarHistoriaDelCanal('c1', paginador(canalDe(120), llamadas))).toHaveLength(120);
  let n = 0;
  const fallaLaSegunda = (_id: string, before?: string) =>
    ++n === 2 ? Promise.reject(new Error('red')) : paginador(canalDe(120), [])(_id, before);
  expect(await cargarHistoriaDelCanal('c1', fallaLaSegunda)).toHaveLength(MENSAJES_POR_CANAL);
  await expect(
    cargarHistoriaDelCanal('c1', () => Promise.reject(new Error('red'))),
  ).rejects.toThrow('red');
});
