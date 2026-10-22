/**
 * Self-check del estimador de coste (ruta de dinero): mapeo métrica→precio,
 * subtotal = consumo × precio, total = suma, métricas sin precio ignoradas y
 * cuotas normalizadas. Sin DOM ni framework. Vive fuera de src/ para no entrar
 * en el astro check/eslint/prettier de la app. Ejecutar: `bun test`.
 */
import { expect, test } from 'bun:test';
import { estimateCost, METRICS, describeIncluded } from '../src/lib/pricing.js';
import { appT } from '../src/i18n/index.js';

test('subtotal = consumo × precio y total = suma, redondeados a céntimos', () => {
  const { lines, total } = estimateCost([
    { metric: 'build_minutes', total: 100 }, // 100 × 0.005 = 0.50
    { metric: 'storage_gb', total: 18 }, // 18 × 0.02 = 0.36
  ]);
  expect(lines.map((l) => l.subtotal)).toEqual([0.5, 0.36]);
  expect(total).toBeCloseTo(0.86, 5);
});

test('ignora métricas sin precio y no rompe con lista vacía', () => {
  expect(estimateCost([]).total).toBe(0);
  const { lines, total } = estimateCost([
    { metric: 'egress_gb', total: 999 }, // sin precio → ignorada
    { metric: 'compute_gb_hours', total: 50 }, // 50 × 0.009 = 0.45
  ]);
  expect(lines).toHaveLength(1);
  expect(lines[0]?.metric).toBe('compute_gb_hours');
  expect(total).toBeCloseTo(0.45, 5);
});

test('cuota (share) suma ~1 cuando hay coste', () => {
  const { lines } = estimateCost([
    { metric: 'build_minutes', total: 100 },
    { metric: 'storage_gb', total: 50 },
  ]);
  expect(lines.reduce((s, l) => s + l.share, 0)).toBeCloseTo(1, 5);
});

test('todos los precios definidos son positivos', () => {
  for (const meta of Object.values(METRICS)) {
    expect(meta.pricePerUnit).toBeGreaterThan(0);
  }
});

/* ---------- describeIncluded: redacción única de la cuota incluida ---------- */
//
// La redacción sale del catálogo, así que la `t` entra por parámetro. Se prueba en
// inglés —la fuente de verdad— y con el francés a la vez en un caso, para que la
// clave siga saliendo traducida y no en el idioma base por accidente.

const t = appT('en');

test('describeIncluded: nombra solo lo que declara el backend', () => {
  const txt = describeIncluded(t, { build_minutes: 1000, compute_gb_hours: 1000, storage_gb: 25 });
  // Las cantidades se formatean por locale: en inglés europeo, «1,000».
  expect(txt).toContain('1,000');
  expect(txt).toContain('25');
  expect(txt).toContain('billed per unit');
  expect(describeIncluded(appT('fr'), { storage_gb: 25 })).toContain(
    'la facturation est à l’unité',
  );
});

test('describeIncluded: sin cuotas declaradas no inventa texto', () => {
  expect(describeIncluded(t, {})).toBe('');
  expect(describeIncluded(t, { build_minutes: 0 })).toBe('');
});

test('describeIncluded: una métrica desconocida se muestra tal cual, no se descarta', () => {
  const txt = describeIncluded(t, { metrica_futura: 7 });
  expect(txt).toContain('7');
  expect(txt).toContain('metrica_futura');
});
