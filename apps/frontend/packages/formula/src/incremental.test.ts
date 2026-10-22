// Recálculo incremental: una edición solo reevalúa a sus dependientes, y las funciones
// volátiles se reevalúan en cada edición aunque no lean nada de lo editado. `bun test`.
import { expect, test } from 'bun:test';
import { Engine, VOLATILES } from './engine.js';

const H = 'Hoja1';

/** Lee todas las celdas dadas (deja la caché caliente). */
function leer(e: Engine, refs: string[]): void {
  for (const r of refs) e.getValue(H, r);
}

test('una edición solo reevalúa a sus dependientes', () => {
  const e = new Engine();
  const refs: string[] = [];
  for (let i = 1; i <= 100; i++) {
    e.setCell(H, `A${i}`, String(i));
    e.setCell(H, `B${i}`, `=A${i}*2`);
    refs.push(`B${i}`);
  }
  e.setCell(H, 'C1', '=SUM(B1:B100)');
  refs.push('C1');
  leer(e, refs);
  const base = e.evaluaciones;
  expect(base).toBe(101); // 100 B + C1, una vez cada una

  leer(e, refs); // sin cambios: todo sale de la caché
  expect(e.evaluaciones).toBe(base);

  e.setCell(H, 'A5', '500'); // toca B5 y, por él, C1: nada más
  leer(e, refs);
  expect(e.evaluaciones - base).toBe(2);
  expect(e.getValue(H, 'B5')).toBe(1000);
  expect(e.getValue(H, 'C1')).toBe(10_100 - 10 + 1000);
});

test('una celda independiente no reevalúa nada', () => {
  const e = new Engine();
  e.setCell(H, 'A1', '1');
  e.setCell(H, 'B1', '=A1+1');
  leer(e, ['B1']);
  const base = e.evaluaciones;
  e.setCell(H, 'Z9', 'texto suelto');
  leer(e, ['B1']);
  expect(e.evaluaciones).toBe(base);
});

test('editar y vaciar una celda dejan bien a su dependiente', () => {
  const e = new Engine();
  e.setCell(H, 'A1', '2');
  e.setCell(H, 'B1', '=A1*A1');
  expect(e.getValue(H, 'B1')).toBe(4);
  e.setCell(H, 'A1', '3');
  expect(e.getValue(H, 'B1')).toBe(9);
  e.setCell(H, 'A1', '');
  expect(e.getValue(H, 'B1')).toBe(0);
});

test('un rango demasiado grande para expandirlo también se invalida al editar', () => {
  // A1:QH450 son 450 × 450 = 202.500 celdas, más que DEP_EXPAND_CAP (200.000): antes no
  // tenía ninguna dependencia y su caché quedaba rancia para siempre.
  const e = new Engine();
  e.setCell(H, 'A1', '5');
  e.setCell(H, 'QJ1', '=SUM(A1:QH450)');
  expect(e.getValue(H, 'QJ1')).toBe(5);
  e.setCell(H, 'A2', '7');
  expect(e.getValue(H, 'QJ1')).toBe(12);
  e.setCell(H, 'A1', '');
  expect(e.getValue(H, 'QJ1')).toBe(7);
  // Recorre 202.500 celdas tres veces: en un contenedor con 50 de carga pasa de 5 s. El plazo
  // vigila que termina, no cuánto tarda (eso lo mide `bun bench`).
}, 30_000);

test('VOLATILES: las siete que marca Excel', () => {
  expect([...VOLATILES].sort()).toEqual(
    ['INDIRECT', 'NOW', 'OFFSET', 'RAND', 'RANDARRAY', 'RANDBETWEEN', 'TODAY'].sort(),
  );
});

test('NOW, TODAY y RAND se reevalúan en cada edición aunque no dependan de nada', () => {
  let t = 0;
  let r = 0;
  const e = new Engine({
    now: () => new Date(Date.UTC(2026, 9, 2, 12, 0, t++)),
    random: () => ++r / 100,
  });
  e.setCell(H, 'A1', '=NOW()');
  e.setCell(H, 'A2', '=RAND()');
  e.setCell(H, 'A3', '=TODAY()');
  e.setCell(H, 'B1', '=A2*100'); // depende de una volátil: también cambia
  e.setCell(H, 'B2', '=1+1'); // no es volátil ni depende de una
  const a1 = e.getValue(H, 'A1');
  const a2 = e.getValue(H, 'A2');
  const b1 = e.getValue(H, 'B1');
  e.getValue(H, 'B2');
  const base = e.evaluaciones;

  e.setCell(H, 'Z1', 'cualquier edición');
  expect(e.getValue(H, 'A1')).not.toBe(a1);
  expect(e.getValue(H, 'A2')).not.toBe(a2);
  expect(e.getValue(H, 'B1')).not.toBe(b1);
  e.getValue(H, 'A3');
  e.getValue(H, 'B2');
  // A1, A2, A3 y B1 (por A2) se reevalúan; B2 no.
  expect(e.evaluaciones - base).toBe(4);
});

test('OFFSET e INDIRECT, que no declaran de qué leen, se reevalúan siempre', () => {
  const e = new Engine();
  e.setCell(H, 'A1', '1');
  e.setCell(H, 'A2', '2');
  e.setCell(H, 'B1', '=INDIRECT("A"&2)');
  e.setCell(H, 'B2', '=OFFSET(A1,1,0)');
  expect(e.getValue(H, 'B1')).toBe(2);
  expect(e.getValue(H, 'B2')).toBe(2);
  e.setCell(H, 'A2', '20'); // ninguna de las dos lo declaró como dependencia
  expect(e.getValue(H, 'B1')).toBe(20);
  expect(e.getValue(H, 'B2')).toBe(20);
});

test('la volatilidad llega por un nombre definido y se pierde al redefinirlo', () => {
  const e = new Engine({ random: () => 0.25 });
  e.defineName('Azar', 'RAND()');
  e.setCell(H, 'A1', '=Azar*4');
  e.getValue(H, 'A1');
  const base = e.evaluaciones;
  e.setCell(H, 'Z1', 'x');
  e.getValue(H, 'A1');
  expect(e.evaluaciones - base).toBe(1); // volátil por el nombre
  e.defineName('Azar', '7'); // ya no
  e.getValue(H, 'A1');
  const base2 = e.evaluaciones;
  e.setCell(H, 'Z2', 'y');
  e.getValue(H, 'A1');
  expect(e.evaluaciones).toBe(base2);
  expect(e.getValue(H, 'A1')).toBe(28);
});

test('una celda que deja de ser volátil se deja de reevaluar', () => {
  const e = new Engine();
  e.setCell(H, 'A1', '=RAND()');
  e.getValue(H, 'A1');
  e.setCell(H, 'A1', '=1+1');
  e.getValue(H, 'A1');
  const base = e.evaluaciones;
  e.setCell(H, 'Z1', 'x');
  e.getValue(H, 'A1');
  expect(e.evaluaciones).toBe(base);
});

test('usedRange encoge al borrar en el borde (el motor vive entre ediciones)', () => {
  const e = new Engine();
  e.setCell(H, 'B3', '1');
  e.setCell(H, 'AD5000', 'x');
  expect(e.usedRange(H)).toEqual({ maxCol: 29, maxRow: 4999 });
  e.setCell(H, 'AD5000', ''); // el borde: se recuenta
  expect(e.usedRange(H)).toEqual({ maxCol: 1, maxRow: 2 });
  e.setCell(H, 'B3', '');
  expect(e.usedRange(H)).toEqual({ maxCol: -1, maxRow: -1 });
  // Borrar en el interior no cambia nada ni obliga a recontar.
  e.setCell(H, 'A1', '1');
  e.setCell(H, 'C4', '2');
  e.setCell(H, 'A1', '');
  expect(e.usedRange(H)).toEqual({ maxCol: 2, maxRow: 3 });
});
