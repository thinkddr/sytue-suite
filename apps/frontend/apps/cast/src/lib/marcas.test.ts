// Marcas de tiempo, capítulos, «?t=» y las teclas que llevan a un sitio. Los mismos casos
// están en `apps/fyne/apps/cast/marcas_test.go`. Ejecutar: `bun test`.
import { describe, expect, test } from 'bun:test';
import { capitulosDe, destinoDeTecla, enlaceEn, inicioDeUrl, trozosDe } from './marcas.js';

describe('trozosDe', () => {
  test('parte el texto en lo llano y las marcas, con sus segundos', () => {
    expect(trozosDe('Mira el 1:23 y luego 1:02:03.')).toEqual([
      { texto: 'Mira el ' },
      { texto: '1:23', segundos: 83 },
      { texto: ' y luego ' },
      { texto: '1:02:03', segundos: 3723 },
      { texto: '.' },
    ]);
  });
  test('no confunde horas imposibles, fechas ni números pegados', () => {
    expect(trozosDe('a las 1:75')).toEqual([{ texto: 'a las 1:75' }]);
    expect(trozosDe('1:60:00')).toEqual([{ texto: '1:60:00' }]);
    expect(trozosDe('v2:30:15:00')).toEqual([{ texto: 'v2:30:15:00' }]);
    expect(trozosDe('')).toEqual([]);
  });
});

describe('capitulosDe', () => {
  test('las líneas que empiezan por una marca, desde 0:00 y en orden', () => {
    const d = 'Charla de octubre.\n\n0:00 Presentación\n- 1:30 – Demo\n(12:05) Preguntas\nGracias.';
    expect(capitulosDe(d)).toEqual([
      { segundos: 0, titulo: 'Presentación' },
      { segundos: 90, titulo: 'Demo' },
      { segundos: 725, titulo: 'Preguntas' },
    ]);
  });
  test('sin 0:00, con uno solo o desordenados no es un índice', () => {
    expect(capitulosDe('0:10 Uno\n1:00 Dos')).toEqual([]);
    expect(capitulosDe('0:00 Solo')).toEqual([]);
    expect(capitulosDe('0:00 Uno\n2:00 Dos\n1:00 Tres')).toEqual([]);
    expect(capitulosDe('Quedamos a las 0:00 y a las 1:00')).toEqual([]);
  });
});

test('enlaceEn e inicioDeUrl: ida y vuelta', () => {
  expect(enlaceEn('https://x.eu/cast/watch/v1', 83.9)).toBe('https://x.eu/cast/watch/v1?t=83');
  expect(enlaceEn('https://x.eu/cast/watch/v1?t=5', 0)).toBe('https://x.eu/cast/watch/v1');
  expect(enlaceEn('/cast/watch/v1?a=1&t=5', 90)).toBe('/cast/watch/v1?a=1&t=90');
  expect(inicioDeUrl('83')).toBe(83);
  expect(inicioDeUrl('83s')).toBe(83);
  expect(inicioDeUrl('1m23s')).toBe(83);
  expect(inicioDeUrl('1h2m3s')).toBe(3723);
  expect(inicioDeUrl('1:23')).toBe(83);
  for (const malo of [null, '', '0', 'abc', '-5', '1:75']) expect(inicioDeUrl(malo)).toBeNull();
});

test('destinoDeTecla: J y L 10 s, Inicio y Fin, 0–9 en décimas, sin salirse', () => {
  expect(destinoDeTecla('j', 30, 100)).toBe(20);
  expect(destinoDeTecla('L', 95, 100)).toBe(100);
  expect(destinoDeTecla('J', 4, 100)).toBe(0);
  expect(destinoDeTecla('Home', 50, 100)).toBe(0);
  expect(destinoDeTecla('End', 50, 100)).toBe(100);
  expect(destinoDeTecla('0', 50, 100)).toBe(0);
  expect(destinoDeTecla('7', 0, 200)).toBe(140);
  expect(destinoDeTecla('x', 0, 100)).toBeNull();
  expect(destinoDeTecla('5', 0, Number.NaN)).toBeNull();
});
