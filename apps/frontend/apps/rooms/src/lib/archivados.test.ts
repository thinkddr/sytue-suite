// Checks de la pestaña «Archivados». Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { Channel } from './chat.js';
import { claveErrorArchivado, filaTrasQuitar, nombreNuevo, soloArchivados } from './archivados.js';

const canal = (id: string, extra: Partial<Channel> = {}): Channel => ({
  id,
  name: id,
  kind: 'public',
  ...extra,
});

test('solo los archivados, sin directos y por nombre', () => {
  const cs = [
    canal('zeta', { archived: true }),
    canal('activo'),
    canal('dm', { kind: 'direct', archived: true }),
    canal('alfa', { archived: true, kind: 'private' }),
  ];
  expect(soloArchivados(cs).map((c) => c.id)).toEqual(['alfa', 'zeta']);
});

test('cada fallo con su aviso', () => {
  expect(claveErrorArchivado(403)).toBe('archived.forbidden');
  expect(claveErrorArchivado(409)).toBe('archived.nameTaken');
  expect(claveErrorArchivado(400)).toBe('archived.invalidName');
  expect(claveErrorArchivado(502)).toBe('archived.failed');
  expect(claveErrorArchivado(undefined)).toBe('archived.failed');
});

test('renombrar solo manda un nombre nuevo de verdad', () => {
  expect(nombreNuevo('ventas', '  ventas-2026 ')).toBe('ventas-2026');
  expect(nombreNuevo('ventas', ' ventas ')).toBeNull();
  expect(nombreNuevo('ventas', '   ')).toBeNull();
});

test('al desarchivar, el foco pasa a la fila siguiente, o a la anterior si era la última', () => {
  expect(filaTrasQuitar(['a', 'b', 'c'], 'b')).toBe('c');
  expect(filaTrasQuitar(['a', 'b', 'c'], 'c')).toBe('b');
  expect(filaTrasQuitar(['a'], 'a')).toBeNull();
  expect(filaTrasQuitar(['a', 'b'], 'x')).toBeNull();
});
