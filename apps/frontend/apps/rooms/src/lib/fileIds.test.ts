// Checks de sanitizeFileIds (frontera de confianza del BFF). Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { sanitizeFileIds, MAX_ATTACHMENTS } from './fileIds.js';

const A = '11111111-1111-4111-8111-111111111111';
const B = '22222222-2222-4222-8222-222222222222';

test('acepta solo UUIDs bien formados', () => {
  expect(sanitizeFileIds([A, 'no-uuid', '', 123, null, B])).toEqual([A, B]);
});

test('normaliza a minúsculas y deduplica', () => {
  expect(sanitizeFileIds([A.toUpperCase(), A, ` ${A} `])).toEqual([A]);
});

test('no-array o vacío → []', () => {
  expect(sanitizeFileIds(undefined)).toEqual([]);
  expect(sanitizeFileIds('x')).toEqual([]);
  expect(sanitizeFileIds([])).toEqual([]);
});

test('acota al máximo de adjuntos por mensaje', () => {
  // Genera MAX+5 UUIDs distintos y comprueba que se corta en MAX.
  const many = Array.from({ length: MAX_ATTACHMENTS + 5 }, (_, i) => {
    const h = i.toString(16).padStart(12, '0');
    return `00000000-0000-4000-8000-${h}`;
  });
  expect(sanitizeFileIds(many)).toHaveLength(MAX_ATTACHMENTS);
});
