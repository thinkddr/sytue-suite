// A dónde va cada denuncia. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { peticionDeDenuncia } from './denuncia.js';

const datos = { reason: 'spam', details: '' } as const;

test('con sesión va a la ruta con sesión y sin prueba de trabajo', () => {
  expect(peticionDeDenuncia({ kind: 'video', videoId: 'v1' }, datos, null)).toEqual({
    ruta: '/videos/v1/reports',
    cuerpo: { kind: 'video', reason: 'spam', details: '' },
  });
});

test('sin sesión va a la pública, con la prueba resuelta', () => {
  const p = peticionDeDenuncia({ kind: 'comment', videoId: 'v1', target: 'c9' }, datos, 'carga');
  expect(p.ruta).toBe('/public/videos/v1/reports');
  expect(p.cuerpo).toEqual({
    kind: 'comment',
    target: 'c9',
    reason: 'spam',
    details: '',
    altcha: 'carga',
  });
});

test('a una persona se la nombra por su sub, no por su nombre', () => {
  const p = peticionDeDenuncia(
    { kind: 'person', videoId: 'v1', target: 'sub-de-bea', nombre: 'Bea' },
    datos,
    null,
  );
  expect(p.cuerpo).toEqual({ kind: 'person', target: 'sub-de-bea', reason: 'spam', details: '' });
});

test('el canal tiene su ruta y escapa el slug', () => {
  expect(peticionDeDenuncia({ kind: 'channel', org: 'a/b' }, datos, 'carga').ruta).toBe(
    '/public/channels/a%2Fb/reports',
  );
});
