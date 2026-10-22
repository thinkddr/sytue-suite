/**
 * Self-check de las ayudas de la pantalla de escala. Lo que importa: el coste que
 * se le enseña a alguien antes de que confirme, y que la traducción de recursos no
 * invente nada cuando el catálogo no los trae. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import {
  HOURS_PER_MONTH,
  humanResources,
  monthlyComputeEur,
  typicalPortFor,
} from '../src/lib/runtime.js';

test('el coste mensual sale de la métrica que se factura de verdad', () => {
  // 0,5 GiB × 1 instancia × 730 h × 0,009 €/GB·h = 3,285 -> 3,29 €
  expect(monthlyComputeEur(0.5, 1)).toBe(3.29);
  // Escalar multiplica: el triple de instancias es el triple de coste.
  expect(monthlyComputeEur(0.5, 3)).toBe(9.86);
  // Y el tamaño también: 4 GiB es 8 veces 0,5 GiB.
  expect(monthlyComputeEur(4, 1)).toBe(26.28);
  expect(HOURS_PER_MONTH).toBe(730);
});

test('sin datos no se inventa un coste', () => {
  // El catálogo de demostración no trae memoria: mejor 0 que un número falso.
  expect(monthlyComputeEur(0, 3)).toBe(0);
  expect(monthlyComputeEur(2, 0)).toBe(0);
  expect(monthlyComputeEur(-1, 1)).toBe(0);
});

test('los recursos se enseñan en unidades humanas', () => {
  expect(humanResources('es', '1000m', '512Mi')).toBe('1 vCPU · 512 MB');
  expect(humanResources('es', '500m', '1Gi')).toBe('0,5 vCPU · 1,0 GB');
  expect(humanResources('es', '2000m', '2Gi')).toBe('2 vCPU · 2,0 GB');
  expect(humanResources('en', '4000m', '1536Mi')).toBe('4 vCPU · 1.5 GB');
});

test('un catálogo sin recursos no produce texto basura', () => {
  expect(humanResources('es', '', '')).toBe('');
  expect(humanResources('es', 'rarísimo', 'raro')).toBe('');
  // Uno de los dos ausente: se enseña el que sí hay.
  expect(humanResources('es', '1000m', '')).toBe('1 vCPU');
});

test('solo se sugiere puerto donde la convención es real', () => {
  expect(typicalPortFor('astro')).toBe(4321);
  expect(typicalPortFor('next')).toBe(3000);
  // Los que respetan $PORT no necesitan sugerencia: sugerir sería ruido.
  expect(typicalPortFor('node')).toBeUndefined();
  expect(typicalPortFor('static')).toBeUndefined();
});
