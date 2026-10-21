/**
 * La matemática de color es el CONTRATO entre lo que se ve y lo que se
 * exportará: se fija con vectores. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { ajustarColor, aplicarColor, luminancia, type RGB } from './motor.js';

const cerca = (a: RGB, b: RGB) => a.forEach((x, i) => expect(x).toBeCloseTo(b[i]!, 5));

test('un paso de exposición dobla la luz; −1 la halva', () => {
  cerca(
    ajustarColor([0.25, 0.25, 0.25], { tipo: 'exposicion', params: { valor: 1 } }),
    [0.5, 0.5, 0.5],
  );
  cerca(
    ajustarColor([0.5, 0.5, 0.5], { tipo: 'exposicion', params: { valor: -1 } }),
    [0.25, 0.25, 0.25],
  );
});

test('el contraste gira alrededor del gris medio, que no se mueve', () => {
  cerca(
    ajustarColor([0.5, 0.5, 0.5], { tipo: 'contraste', params: { valor: 1 } }),
    [0.5, 0.5, 0.5],
  );
  cerca(ajustarColor([0.75, 0.25, 0.5], { tipo: 'contraste', params: { valor: 1 } }), [1, 0, 0.5]);
});

test('saturación −1 deja gris de la misma luminancia; +0 no toca', () => {
  const c: RGB = [0.9, 0.2, 0.1];
  const gris = ajustarColor(c, { tipo: 'saturacion', params: { valor: -1 } });
  expect(gris[0]).toBeCloseTo(gris[1], 6);
  expect(gris[1]).toBeCloseTo(gris[2], 6);
  expect(gris[0]).toBeCloseTo(luminancia(c), 6);
  cerca(ajustarColor(c, { tipo: 'saturacion', params: { valor: 0 } }), c);
});

test('la temperatura cálida sube el rojo y baja el azul; el verde no cambia', () => {
  const c = ajustarColor([0.5, 0.5, 0.5], { tipo: 'temperatura', params: { valor: 1 } });
  expect(c[0]).toBeGreaterThan(0.5);
  expect(c[1]).toBeCloseTo(0.5, 6);
  expect(c[2]).toBeLessThan(0.5);
});

// El recorte a 0..1 va AL FINAL: entre pasos se conserva lo que el siguiente
// puede recuperar.
test('se recorta al final, no entre pasos', () => {
  const r = {
    version: 1,
    pasos: [
      { tipo: 'exposicion' as const, params: { valor: 2 } }, // 0.5 → 2.0, fuera de rango
      { tipo: 'exposicion' as const, params: { valor: -2 } }, // y vuelve a 0.5
    ],
  };
  cerca(aplicarColor([0.5, 0.5, 0.5], r), [0.5, 0.5, 0.5]);
  cerca(aplicarColor([0.9, 0.9, 0.9], { version: 1, pasos: [r.pasos[0]!] }), [1, 1, 1]);
});

test('la geometría no altera el color', () => {
  cerca(ajustarColor([0.3, 0.6, 0.9], { tipo: 'giro', params: { grados: 90 } }), [0.3, 0.6, 0.9]);
});
