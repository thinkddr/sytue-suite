// Checks de a quién interrumpe una llamada entrante. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { avisoDeLlamada } from './llamadaEntrante.js';
import type { Channel } from './chat.js';

function canal(over: Partial<Channel> = {}): Channel {
  return { id: 'c1', name: 'general', kind: 'public', ...over } as Channel;
}

test('llamada en un canal que no estás mirando: avisa', () => {
  const a = avisoDeLlamada({
    channelId: 'c1',
    activeId: 'otro',
    enLlamada: false,
    canales: [canal()],
  });
  expect(a).toEqual({ channelId: 'c1', label: 'general', direct: false });
});

test('un directo se marca como tal: es el único que además suena', () => {
  const a = avisoDeLlamada({
    channelId: 'd1',
    activeId: null,
    enLlamada: false,
    canales: [canal({ id: 'd1', name: 'Ana', kind: 'direct' })],
  });
  expect(a?.direct).toBe(true);
});

test('el canal que ya está abierto no se avisa: lo cuenta el banner de la cabecera', () => {
  expect(
    avisoDeLlamada({ channelId: 'c1', activeId: 'c1', enLlamada: false, canales: [canal()] }),
  ).toBeNull();
});

test('estando ya en una llamada, no entra otra por encima', () => {
  expect(
    avisoDeLlamada({ channelId: 'c1', activeId: 'otro', enLlamada: true, canales: [canal()] }),
  ).toBeNull();
});

test('un canal silenciado no interrumpe: ya se dijo que no', () => {
  expect(
    avisoDeLlamada({
      channelId: 'c1',
      activeId: 'otro',
      enLlamada: false,
      canales: [canal({ muted: true })],
    }),
  ).toBeNull();
});

test('un canal que no está en la lista se ignora en vez de inventar el nombre', () => {
  expect(
    avisoDeLlamada({ channelId: 'ajeno', activeId: 'otro', enLlamada: false, canales: [canal()] }),
  ).toBeNull();
  expect(
    avisoDeLlamada({ channelId: '', activeId: null, enLlamada: false, canales: [canal()] }),
  ).toBeNull();
});
