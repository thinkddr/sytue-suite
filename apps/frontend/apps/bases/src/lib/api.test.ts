// El mensaje que ve la persona sale de la clave de error del servicio, nunca del texto
// crudo, y cada código de la API de Bases tiene el suyo. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { HttpError } from '@sytue/api-clients';
import type { T } from '../i18n/index.js';
import { mensajeDeError } from './api.js';

const t = ((k: string) => k) as unknown as T;
const err = (status: number, code?: string) =>
  new HttpError(status, 'x', 'http://x', code ? JSON.stringify({ code }) : undefined);

test('cada estado y código de la API tiene su mensaje', () => {
  expect(mensajeDeError(err(401), t)).toBe('err.session');
  expect(mensajeDeError(err(403), t)).toBe('err.forbidden');
  expect(mensajeDeError(err(404), t)).toBe('err.notFound');
  expect(mensajeDeError(err(409, 'rev_conflict'), t)).toBe('err.conflict');
  expect(mensajeDeError(err(409, 'referenced'), t)).toBe('err.referenced');
  expect(mensajeDeError(err(409, 'limit_reached'), t)).toBe('err.limit');
  expect(mensajeDeError(err(422, 'invalid_input'), t)).toBe('err.invalid');
  expect(mensajeDeError(err(422, 'not_a_member'), t)).toBe('err.notMember');
  expect(mensajeDeError(err(422, 'is_member'), t)).toBe('err.isMember');
});

test('un fallo desconocido, o que no es de la API, sale genérico y sin filtrar texto', () => {
  expect(mensajeDeError(err(500), t)).toBe('err.generic');
  expect(mensajeDeError(new Error('SELECT * FROM bases'), t)).toBe('err.generic');
  expect(mensajeDeError('texto', t)).toBe('err.generic');
});
