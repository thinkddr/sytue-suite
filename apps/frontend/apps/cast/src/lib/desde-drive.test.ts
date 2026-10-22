import { expect, test } from 'bun:test';
import { idDeDrive, libresDe, motivoDeDrive, reintentable } from './desde-drive.js';
import { FalloDelServicio } from './fallo.js';

const ID = '7d1c6a3e-0b8e-4f39-9d3a-4a3e8a1f2b10';

test('sin ?drive= es la subida de siempre', () => {
  expect(idDeDrive('')).toBeNull();
  expect(idDeDrive('?otra=1')).toBeNull();
});

test('el identificador de Drive se lee de la URL', () => {
  expect(idDeDrive(`?drive=${ID}`)).toBe(ID);
  expect(idDeDrive(`?drive=${ID.toUpperCase()}`)).toBe(ID);
});

test('lo que no es un identificador no llega al servicio: es el estado vacío', () => {
  expect(idDeDrive('?drive=')).toBe('');
  expect(idDeDrive('?drive=..%2Fquota')).toBe('');
  expect(idDeDrive(`?drive=${ID}x`)).toBe('');
});

test('cada respuesta del servicio tiene su motivo', () => {
  expect(motivoDeDrive(new FalloDelServicio(404))).toBe('sinPermiso');
  expect(motivoDeDrive(new FalloDelServicio(403))).toBe('sinPermiso');
  expect(motivoDeDrive(new FalloDelServicio(415, 'not_video'))).toBe('noVideo');
  expect(motivoDeDrive(new FalloDelServicio(413, 'too_large'))).toBe('grande');
  expect(motivoDeDrive(new FalloDelServicio(507, 'quota_exceeded'))).toBe('cuota');
  expect(motivoDeDrive(new FalloDelServicio(503))).toBe('drive');
  expect(motivoDeDrive(new FalloDelServicio(500))).toBe('error');
});

test('sin conexión manda sobre cualquier otra cosa', () => {
  expect(motivoDeDrive(new FalloDelServicio(415), false)).toBe('offline');
  expect(motivoDeDrive(new TypeError('Failed to fetch'))).toBe('offline');
});

test('los bytes libres salen del detalle del 507', () => {
  expect(libresDe(new FalloDelServicio(507, 'quota_exceeded', { available_bytes: 42 }))).toBe(42);
  expect(libresDe(new FalloDelServicio(507))).toBeNull();
  expect(libresDe(new Error('x'))).toBeNull();
});

test('solo se ofrece reintentar lo que no depende del fichero', () => {
  expect(reintentable('drive')).toBe(true);
  expect(reintentable('noVideo')).toBe(false);
  expect(reintentable('cuota')).toBe(false);
});
