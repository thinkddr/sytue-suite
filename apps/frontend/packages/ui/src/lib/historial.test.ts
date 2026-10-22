// Checks del deshacer/rehacer compartido en @sytue/ui. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import {
  anotar,
  deshacer,
  nuevoHistorial,
  rehacer,
  sePuedeDeshacer,
  sePuedeRehacer,
  TOPE,
} from './historial';

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

test('escribir algo nuevo tras deshacer borra el futuro', () => {
  let h = nuevoHistorial<string>();
  h = aplicar(h, 'a');
  h = aplicar(h, 'b');
  const atras = deshacer(h, 'c')!;
  expect(sePuedeRehacer(atras.historial)).toBe(true);
  const tras = aplicar(atras.historial, 'b');
  expect(sePuedeRehacer(tras)).toBe(false);
  expect(deshacer(tras, 'z')?.modelo).toBe('b');
});

test('el tope recorta por el principio y admite tope configurable (100 para Pages)', () => {
  let h = nuevoHistorial<number>();
  for (let i = 0; i < 10; i++) h = aplicar(h, i, 3);
  expect(h.pasado).toEqual([7, 8, 9]);

  // Probar tope 100
  let h100 = nuevoHistorial<number>();
  for (let i = 0; i < 150; i++) h100 = aplicar(h100, i, 100);
  expect(h100.pasado.length).toBe(100);
  expect(h100.pasado[0]).toBe(50);
  expect(h100.pasado[99]).toBe(149);

  // Probar tope por defecto
  expect(TOPE).toBe(50);
});

test('deshacer hasta el fondo y volver: la ida y la vuelta cuadran', () => {
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
