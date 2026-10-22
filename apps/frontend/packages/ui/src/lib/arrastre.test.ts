// Checks del núcleo puro de arrastre y redimensión en @sytue/ui. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import {
  clamp,
  calcularEnvolvente,
  calcularLimitesArrastre,
  calcularArrastre,
  calcularRedimension,
  calcularPasoTeclado,
} from './arrastre';

test('clamp acota valores numéricos', () => {
  expect(clamp(5, 0, 10)).toBe(5);
  expect(clamp(-5, 0, 10)).toBe(0);
  expect(clamp(15, 0, 10)).toBe(10);
});

test('calcularEnvolvente calcula correctamente el bounding box', () => {
  expect(calcularEnvolvente([])).toEqual({ x: 0, y: 0, w: 0, h: 0 });

  const cajas = [
    { x: 10, y: 20, w: 30, h: 40 },
    { x: 50, y: 15, w: 20, h: 10 },
  ];
  // minX = 10, minY = 15, maxX = 70, maxY = 60
  // w = 60, h = 45
  expect(calcularEnvolvente(cajas)).toEqual({ x: 10, y: 15, w: 60, h: 45 });
});

test('calcularLimitesArrastre no permite que la envolvente salga del lienzo', () => {
  const env = { x: 10, y: 20, w: 50, h: 40 };
  const limites = calcularLimitesArrastre(env, { w: 100, h: 100 });
  expect(limites.min).toEqual({ x: -10, y: -20 });
  expect(limites.max).toEqual({ x: 40, y: 40 }); // 100 - (10 + 50) = 40, 100 - (20 + 40) = 40
});

test('calcularArrastre mueve cajas en grupo manteniendo posiciones relativas y topando deltas', () => {
  const origs = [
    { id: 'b1', x: 10, y: 10, w: 20, h: 20 },
    { id: 'b2', x: 40, y: 40, w: 20, h: 20 },
  ];
  const min = { x: -10, y: -10 };
  const max = { x: 40, y: 40 };

  // Movimiento dentro de límites
  const res1 = calcularArrastre(origs, 15, 20, min, max);
  expect(res1.dx).toBe(15);
  expect(res1.dy).toBe(20);
  expect(res1.nuevasPosiciones[0]).toEqual({ id: 'b1', x: 25, y: 30, w: 20, h: 20 });
  expect(res1.nuevasPosiciones[1]).toEqual({ id: 'b2', x: 55, y: 60, w: 20, h: 20 });

  // Movimiento sobrepasando límites hacia la derecha/abajo
  const res2 = calcularArrastre(origs, 50, 100, min, max);
  expect(res2.dx).toBe(40);
  expect(res2.dy).toBe(40);
  expect(res2.nuevasPosiciones[0]).toEqual({ id: 'b1', x: 50, y: 50, w: 20, h: 20 });
  expect(res2.nuevasPosiciones[1]).toEqual({ id: 'b2', x: 80, y: 80, w: 20, h: 20 });

  // Movimiento sobrepasando límites hacia la izquierda/arriba
  const res3 = calcularArrastre(origs, -50, -50, min, max);
  expect(res3.dx).toBe(-10);
  expect(res3.dy).toBe(-10);
  expect(res3.nuevasPosiciones[0]).toEqual({ id: 'b1', x: 0, y: 0, w: 20, h: 20 });
  expect(res3.nuevasPosiciones[1]).toEqual({ id: 'b2', x: 30, y: 30, w: 20, h: 20 });
});

test('calcularRedimension redimensiona con topes mínimos y de borde', () => {
  const orig = { x: 10, y: 10, w: 30, h: 40 };

  // Redimension normal 'se'
  const r1 = calcularRedimension(orig, 10, 15, 4, 100, 'se');
  expect(r1).toEqual({ x: 10, y: 10, w: 40, h: 55 });

  // Redimensión empequeñeciendo bajo el mínimo (4)
  const r2 = calcularRedimension(orig, -35, -45, 4, 100, 'se');
  expect(r2).toEqual({ x: 10, y: 10, w: 4, h: 4 });

  // Redimensión sobrepasando el borde del escenario (100)
  const r3 = calcularRedimension(orig, 100, 100, 4, 100, 'se');
  expect(r3).toEqual({ x: 10, y: 10, w: 90, h: 90 }); // 100 - x = 90
});

test('calcularPasoTeclado calcula deltas de flechas con multiplicador', () => {
  expect(calcularPasoTeclado('ArrowLeft')).toEqual({ dx: -1, dy: 0 });
  expect(calcularPasoTeclado('ArrowRight', 5)).toEqual({ dx: 5, dy: 0 });
  expect(calcularPasoTeclado('ArrowUp', 10)).toEqual({ dx: 0, dy: -10 });
  expect(calcularPasoTeclado('ArrowDown')).toEqual({ dx: 0, dy: 1 });
  expect(calcularPasoTeclado('Enter')).toBeNull();
});
