// Checks del deshacer/rehacer del lienzo de la hoja. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import {
  anotar,
  deshacer,
  nuevoHistorial,
  rehacer,
  sePuedeDeshacer,
  sePuedeRehacer,
} from '../src/editor/sheets/historial';

/** Simula la vida de verdad: se aplica un cambio anotando el estado anterior. */
function aplicar<T>(h: ReturnType<typeof nuevoHistorial<T>>, anterior: T, tope?: number) {
  return anotar(h, anterior, tope);
}

test('sin nada hecho no hay nada que deshacer ni que rehacer', () => {
  const h = nuevoHistorial<string>();
  expect(sePuedeDeshacer(h)).toBe(false);
  expect(sePuedeRehacer(h)).toBe(false);
  expect(deshacer(h, 'a')).toBeNull();
  expect(rehacer(h, 'a')).toBeNull();
});

test('deshacer devuelve el estado anterior, y rehacer lo trae de vuelta', () => {
  let h = nuevoHistorial<string>();
  h = aplicar(h, 'a'); // a → b
  h = aplicar(h, 'b'); // b → c
  const atras = deshacer(h, 'c');
  expect(atras?.modelo).toBe('b');
  const atras2 = deshacer(atras!.historial, 'b');
  expect(atras2?.modelo).toBe('a');
  // Y de vuelta.
  const alante = rehacer(atras2!.historial, 'a');
  expect(alante?.modelo).toBe('b');
  expect(rehacer(alante!.historial, 'b')?.modelo).toBe('c');
});

test('escribir algo nuevo tras deshacer BORRA lo que había por delante', () => {
  // Es la parte que se hace mal. Si tras deshacer se escribe otra cosa, la
  // historia se bifurcó: la rama vieja no lleva a ninguna parte, y dejarla haría
  // que Ctrl+Y saltara a un estado que nunca existió.
  let h = nuevoHistorial<string>();
  h = aplicar(h, 'a');
  h = aplicar(h, 'b');
  const atras = deshacer(h, 'c')!;
  expect(sePuedeRehacer(atras.historial)).toBe(true);
  // Ahora se escribe algo nuevo estando en 'b'.
  const tras = aplicar(atras.historial, 'b');
  expect(sePuedeRehacer(tras)).toBe(false);
  // Y hacia atrás se sigue pudiendo ir.
  expect(deshacer(tras, 'z')?.modelo).toBe('b');
});

test('el tope recorta por el principio: se pierde lo más viejo, no lo reciente', () => {
  let h = nuevoHistorial<number>();
  for (let i = 0; i < 10; i++) h = aplicar(h, i, 3);
  expect(h.pasado).toEqual([7, 8, 9]);
  // Los tres pasos que quedan siguen sirviendo, en orden.
  const a = deshacer(h, 10)!;
  expect(a.modelo).toBe(9);
  expect(deshacer(a.historial, 9)?.modelo).toBe(8);
});

test('deshacer hasta el fondo y volver: la ida y la vuelta cuadran', () => {
  // El caso que descubre un `slice` mal puesto: recorrer entero en los dos
  // sentidos tiene que devolver exactamente la misma secuencia.
  let h = nuevoHistorial<string>();
  const pasos = ['a', 'b', 'c', 'd'];
  for (const p of pasos) h = aplicar(h, p);
  let actual = 'e';
  const atras: string[] = [];
  for (;;) {
    const r = deshacer(h, actual);
    if (!r) break;
    h = r.historial;
    actual = r.modelo;
    atras.push(actual);
  }
  expect(atras).toEqual(['d', 'c', 'b', 'a']);
  const alante: string[] = [];
  for (;;) {
    const r = rehacer(h, actual);
    if (!r) break;
    h = r.historial;
    actual = r.modelo;
    alante.push(actual);
  }
  expect(alante).toEqual(['b', 'c', 'd', 'e']);
});
