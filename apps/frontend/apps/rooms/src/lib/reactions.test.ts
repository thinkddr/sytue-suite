// Checks de la etiqueta «quién reaccionó». Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { reactedByLabel } from './reactions.js';
import { roomsT } from '../i18n/index.js';

const NAMES = { a: 'Ana', l: 'Luis', m: 'Marta', p: 'Pau', r: 'Rita' };

/** Atajo: la etiqueta en el idioma base (inglés, docs/CONVENTIONS.md §13.1). */
function label(emoji: string, subs: string[] | undefined): string | undefined {
  return reactedByLabel('en', roomsT('en'), emoji, subs, NAMES);
}

test('sin subs (servicio antiguo u optimista) no hay etiqueta', () => {
  expect(label('👍', undefined)).toBeUndefined();
  expect(label('👍', [])).toBeUndefined();
});

test('un solo reactor va en singular', () => {
  expect(label('👍', ['a'])).toBe('Ana reacted with 👍');
});

test('dos y tres reactores se listan enteros', () => {
  expect(label('👍', ['a', 'l'])).toBe('Ana and Luis reacted with 👍');
  expect(label('🎉', ['a', 'l', 'm'])).toBe('Ana, Luis and Marta reacted with 🎉');
});

test('a partir del cuarto se agrupa en «y N más»', () => {
  expect(label('👍', ['a', 'l', 'm', 'p', 'r'])).toBe(
    'Ana, Luis, Marta and 2 more reacted with 👍',
  );
});

test('los subs que no se resuelven se cuentan, nunca se inventa un nombre', () => {
  const out = label('👍', ['a', 'desconocido', 'otro']);
  expect(out).toBe('Ana and 2 more reacted with 👍');
  expect(out).not.toContain('desconocido');
});

test('sin ningún nombre resoluble se informa del número', () => {
  expect(label('👍', ['x'])).toBe('1 person reacted with 👍');
  expect(reactedByLabel('en', roomsT('en'), '👍', ['x', 'y'], {})).toBe('2 people reacted with 👍');
});

// El verbo concuerda con la cantidad en cada lengua: es justo lo que un `n === 1`
// en el código no puede garantizar y por lo que la forma la elige Intl.PluralRules.
test('la concordancia del verbo la resuelve el catálogo, no el código', () => {
  expect(reactedByLabel('es', roomsT('es'), '👍', ['a'], NAMES)).toBe('Ana reaccionó con 👍');
  expect(reactedByLabel('es', roomsT('es'), '👍', ['a', 'l'], NAMES)).toBe(
    'Ana y Luis reaccionaron con 👍',
  );
  expect(reactedByLabel('de', roomsT('de'), '👍', ['a'], NAMES)).toBe('Ana hat mit 👍 reagiert');
  expect(reactedByLabel('de', roomsT('de'), '👍', ['a', 'l'], NAMES)).toBe(
    'Ana und Luis haben mit 👍 reagiert',
  );
});
