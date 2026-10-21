// La carga por ventanas de la rejilla: nunca más de MAX_VENTANAS páginas en memoria, y
// se vuelve atrás con el cursor con el que se pidió cada una. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { BasesRecord } from '@sytue/api-clients/bases';
import {
  MAX_VENTANAS,
  anadir,
  avanzar,
  cursorAnterior,
  editar,
  empezar,
  retroceder,
} from './ventanas.js';

const reg = (id: string) => ({ id }) as BasesRecord;
/** La página i de una tabla de 6 páginas de 2 registros; su cursor de entrada es `c<i>`. */
const pagina = (i: number) => ({
  records: [reg(`r${i}a`), reg(`r${i}b`)],
  next_cursor: i < 5 ? `c${i + 1}` : '',
});
const ids = (v: { paginas: BasesRecord[][] }) => v.paginas.flat().map((r) => r.id);

test('avanzar acumula hasta MAX_VENTANAS y luego suelta la primera', () => {
  let v = empezar(pagina(0));
  for (let i = 1; i <= 5; i++) {
    expect(v.siguiente).toBe(`c${i}`);
    v = avanzar(v, pagina(i));
    expect(v.paginas.length).toBeLessThanOrEqual(MAX_VENTANAS);
  }
  expect(v.primera).toBe(3);
  expect(ids(v)).toEqual(['r3a', 'r3b', 'r4a', 'r4b', 'r5a', 'r5b']);
  expect(v.siguiente).toBe('');
});

test('retroceder pide la anterior con su cursor y deja el siguiente donde toca', () => {
  let v = empezar(pagina(0));
  for (let i = 1; i <= 4; i++) v = avanzar(v, pagina(i));
  expect(v.primera).toBe(2);
  expect(cursorAnterior(v)).toBe('c1');
  v = retroceder(v, pagina(1));
  expect(ids(v)).toEqual(['r1a', 'r1b', 'r2a', 'r2b', 'r3a', 'r3b']);
  // La 4 se soltó: se vuelve a pedir con el cursor con el que se pidió.
  expect(v.siguiente).toBe('c4');
  expect(cursorAnterior(v)).toBe('');
  v = retroceder(v, pagina(0));
  expect(v.primera).toBe(0);
  expect(v.siguiente).toBe('c3');
  expect(cursorAnterior(v)).toBeNull();
  expect(retroceder(v, pagina(0))).toBe(v);
});

test('los cambios locales no descuadran las ventanas', () => {
  let v = empezar(pagina(0));
  v = avanzar(v, pagina(1));
  v = editar(v, (rs) => rs.filter((r) => r.id !== 'r0b'));
  v = anadir(v, reg('nuevo'));
  expect(ids(v)).toEqual(['r0a', 'r1a', 'r1b', 'nuevo']);
  expect(v.paginas.length).toBe(2);
  expect(ids(anadir(empezar({ records: [] }), reg('x')))).toEqual(['x']);
});
