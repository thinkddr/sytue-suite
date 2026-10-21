/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de las funciones puras de la ola «paridad OnlyOffice» de Sheets:
 * formato condicional avanzado (buildCondDecor: relleno/barra/icono, prioridad,
 * top-N, duplicados, escala de color, barra de datos, iconos, fórmula),
 * sparklines (geometría + SVG). La tabla dinámica tiene su test (sheets-pivot). Además del
 * saneado de las reglas/sparklines nuevas.
 * Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import type { CellValue } from '@sytue/formula';
import {
  buildCondDecor,
  colorScaleAt,
  condRuleLabel,
  dataBarBackground,
  iconSetSvg,
  lerpHex,
  sanitizeCondRule,
  sanitizeSparkline,
  sparklineGeom,
  sparklineSvg,
} from '../src/editor/sheets/format';
import type { CondRule } from '../src/editor/types';
import { docsT } from '../src/i18n';

/** Getter de celda sobre un mapa A1→valor (celda vacía = ''). */
function getterOf(grid: Record<string, CellValue>): (c: number, r: number) => CellValue {
  return (c, r) => grid[`${String.fromCharCode(65 + c)}${r + 1}`] ?? '';
}

const NEVER = (): boolean => false;

/* ---------- Formato condicional: reglas de RELLENO ---------- */

test('buildCondDecor cell: rellena solo las celdas que cumplen', () => {
  const get = getterOf({ A1: 10, A2: 3, A3: 20 });
  const rule: CondRule = { range: 'A1:A3', op: 'gt', value: '5', fill: '#FEF08A' };
  const m = buildCondDecor([rule], get, NEVER);
  expect(m.get('A1')?.fill).toBe('#FEF08A');
  expect(m.get('A2')).toBeUndefined(); // 3 no supera 5
  expect(m.get('A3')?.fill).toBe('#FEF08A');
});

// Una regla de un libro importado puede pintar solo la letra («en rojo si es negativo»): antes
// se perdía al sanear y no se pintaba.
test('formato condicional: el color de letra, con relleno o sin él', () => {
  const soloLetra = { range: 'A1:A2', op: 'lt', value: '0', textColor: '#C00000' };
  expect(sanitizeCondRule(soloLetra)).toEqual(soloLetra as CondRule);
  expect(sanitizeCondRule({ ...soloLetra, textColor: 'red' })).toBeNull(); // solo #RRGGBB
  const get = getterOf({ A1: -1, A2: 2 });
  const conRelleno: CondRule = {
    range: 'A1:A2',
    op: 'gt',
    value: '-5',
    fill: '#FEF08A',
    textColor: '#1D4ED8',
  };
  const m = buildCondDecor([soloLetra as CondRule, conRelleno], get, NEVER);
  // A1 cumple las dos: la letra es la de la primera, el relleno el de la segunda.
  expect(m.get('A1')).toEqual({ textColor: '#C00000', fill: '#FEF08A' });
  expect(m.get('A2')).toEqual({ fill: '#FEF08A', textColor: '#1D4ED8' });
});

test('buildCondDecor prioridad: gana la PRIMERA regla de relleno', () => {
  const get = getterOf({ A1: 10 });
  const green: CondRule = { range: 'A1', op: 'gt', value: '0', fill: '#BBF7D0' };
  const yellow: CondRule = { range: 'A1', op: 'gt', value: '0', fill: '#FEF08A' };
  expect(buildCondDecor([green, yellow], get, NEVER).get('A1')?.fill).toBe('#BBF7D0');
  expect(buildCondDecor([yellow, green], get, NEVER).get('A1')?.fill).toBe('#FEF08A');
});

test('buildCondDecor top-N: incluye empates en el umbral', () => {
  const get = getterOf({ A1: 10, A2: 20, A3: 10, A4: 5 });
  const rule: CondRule = { range: 'A1:A4', kind: 'top', rank: 2, fill: '#FED7AA' };
  const m = buildCondDecor([rule], get, NEVER);
  // sorted desc 20,10,10,5 → umbral = 2.º = 10 → todo ≥10 (empates) se pinta
  expect(m.get('A2')?.fill).toBe('#FED7AA');
  expect(m.get('A1')?.fill).toBe('#FED7AA');
  expect(m.get('A3')?.fill).toBe('#FED7AA');
  expect(m.get('A4')).toBeUndefined();
});

test('buildCondDecor duplicados/únicos', () => {
  const get = getterOf({ A1: 10, A2: 20, A3: 10, A4: 5 });
  const dup = buildCondDecor([{ range: 'A1:A4', kind: 'dup', fill: '#FBCFE8' }], get, NEVER);
  expect(dup.get('A1')?.fill).toBe('#FBCFE8'); // 10 aparece dos veces
  expect(dup.get('A2')).toBeUndefined();
  const uniq = buildCondDecor([{ range: 'A1:A4', kind: 'unique', fill: '#BFDBFE' }], get, NEVER);
  expect(uniq.get('A2')?.fill).toBe('#BFDBFE'); // 20 es único
  expect(uniq.get('A1')).toBeUndefined();
});

test('buildCondDecor colorScale: interpola por valor', () => {
  const get = getterOf({ A1: 5, A2: 20, A3: 12.5 });
  const rule: CondRule = { range: 'A1:A3', kind: 'colorScale', scale: ['#000000', '#ffffff'] };
  const m = buildCondDecor([rule], get, NEVER);
  expect(m.get('A1')?.fill).toBe('#000000'); // mínimo
  expect(m.get('A2')?.fill).toBe('#ffffff'); // máximo
  expect(m.get('A3')?.fill).toBe('#808080'); // punto medio
});

test('buildCondDecor dataBar: fracción respecto al mayor |valor|, cero como eje', () => {
  const get = getterOf({ A1: 10, A2: 20, A3: -20 });
  const m = buildCondDecor([{ range: 'A1:A3', kind: 'dataBar', color: '#2563EB' }], get, NEVER);
  expect(m.get('A2')?.bar?.frac).toBe(1);
  expect(m.get('A2')?.bar?.neg).toBe(false);
  expect(m.get('A3')?.bar?.neg).toBe(true);
  expect(m.get('A1')?.bar?.frac).toBeCloseTo(0.5, 5);
});

test('buildCondDecor iconSet: nivel por tercios', () => {
  const get = getterOf({ A1: 5, A2: 20, A3: 12.5 });
  const m = buildCondDecor([{ range: 'A1:A3', kind: 'iconSet', icons: 'arrows' }], get, NEVER);
  expect(m.get('A1')?.icon).toBe('arrows:0'); // bajo
  expect(m.get('A2')?.icon).toBe('arrows:2'); // alto
  expect(m.get('A3')?.icon).toBe('arrows:1'); // medio
});

test('buildCondDecor formula: usa el evaluador inyectado (refs relativas por celda)', () => {
  const get = getterOf({ A1: 1, A2: 1 });
  // Sólo la primera fila del rango dispara (dr === 0).
  const evalFormula = (_f: string, _dc: number, dr: number): boolean => dr === 0;
  const m = buildCondDecor(
    [{ range: 'A1:A2', kind: 'formula', formula: '=X', fill: '#E9D5FF' }],
    get,
    evalFormula,
  );
  expect(m.get('A1')?.fill).toBe('#E9D5FF');
  expect(m.get('A2')).toBeUndefined();
});

test('barra e icono son canales independientes del relleno', () => {
  const get = getterOf({ A1: 10, A2: 20 });
  const rules: CondRule[] = [
    { range: 'A1:A2', op: 'gt', value: '0', fill: '#FEF08A' },
    { range: 'A1:A2', kind: 'dataBar', color: '#2563EB' },
    { range: 'A1:A2', kind: 'iconSet', icons: 'traffic' },
  ];
  const d = buildCondDecor(rules, get, NEVER).get('A2');
  expect(d?.fill).toBe('#FEF08A');
  expect(d?.bar?.frac).toBe(1);
  expect(d?.icon).toBe('traffic:2');
});

/* ---------- Color / barra / icono: helpers ---------- */

test('lerpHex y colorScaleAt', () => {
  expect(lerpHex('#000000', '#ffffff', 0)).toBe('#000000');
  expect(lerpHex('#000000', '#ffffff', 1)).toBe('#ffffff');
  expect(lerpHex('#000000', '#ffffff', 0.5)).toBe('#808080');
  // Escala de 3 colores: t=0.25 cae en el primer tramo (negro→rojo).
  expect(colorScaleAt(['#000000', '#ff0000', '#ffffff'], 0.25)).toBe('#800000');
});

test('dataBarBackground: barra sólida con cortes duros (sin degradado visible)', () => {
  const css = dataBarBackground({ frac: 1, axis: 0, neg: false, color: '#2563EB' });
  expect(css).toContain('linear-gradient(90deg');
  expect(css).toContain('#2563EB');
  expect(css).toContain('100%'); // llena hasta el final
});

test('iconSetSvg: SVG válido y saneo del nivel', () => {
  expect(iconSetSvg('traffic:2')).toContain('<svg');
  expect(iconSetSvg('traffic:2')).toContain('circle');
  expect(iconSetSvg('symbols:9')).toContain('<svg'); // nivel fuera de rango no rompe
});

/* ---------- Sparklines ---------- */

test('sparklineGeom línea: un punto por valor, ascendente sube', () => {
  const g = sparklineGeom([1, 2, 3], 'line');
  expect(g.points.length).toBe(3);
  expect(g.points[0]![0]).toBe(0); // primer x
  // y menor = más arriba; valor mayor → y menor
  expect(g.points[2]![1]).toBeLessThan(g.points[0]![1]);
});

test('sparklineGeom columnas y win/loss', () => {
  const cols = sparklineGeom([1, -1], 'column');
  expect(cols.bars.length).toBe(2);
  expect(cols.bars[1]!.neg).toBe(true);
  const wl = sparklineGeom([2, 0, -3], 'winloss');
  expect(wl.bars[1]!.h).toBe(0); // cero = sin barra
  expect(wl.bars[2]!.neg).toBe(true);
  expect(wl.bars[0]!.neg).toBe(false);
});

test('sparklineSvg: línea dibuja path; columnas dibujan rects', () => {
  expect(sparklineSvg([1, 2, 3], 'line', '#2563EB')).toContain('<path');
  expect(sparklineSvg([1, 2, 3], 'column')).toContain('<rect');
  expect(sparklineSvg([], 'line')).toContain('<svg'); // rango vacío no rompe
});

/* ---------- Saneado ---------- */

test('sanitizeCondRule: retrocompatible (celda sin kind) y tipos nuevos', () => {
  const cell = sanitizeCondRule({ range: 'A1', op: 'gt', value: '5', fill: '#FEF08A' });
  expect(cell).toEqual({ range: 'A1', op: 'gt', value: '5', fill: '#FEF08A' }); // kind omitido
  expect(sanitizeCondRule({ range: 'A1:A3', kind: 'dataBar', color: '#2563EB' })).toEqual({
    range: 'A1:A3',
    kind: 'dataBar',
    color: '#2563EB',
  });
  expect(sanitizeCondRule({ range: 'A1:A3', kind: 'iconSet', icons: 'traffic' })?.icons).toBe(
    'traffic',
  );
  expect(sanitizeCondRule({ range: 'A1', kind: 'top', rank: 3, fill: '#FED7AA' })?.rank).toBe(3);
  expect(
    sanitizeCondRule({ range: 'A1', kind: 'formula', formula: '=A1>1', fill: '#FEF08A' })?.formula,
  ).toBe('=A1>1');
  // Inválidos → null
  expect(sanitizeCondRule({ range: 'A1', kind: 'top', rank: 0, fill: '#FEF08A' })).toBeNull();
  expect(sanitizeCondRule({ range: 'A1', kind: 'colorScale', scale: ['#000000'] })).toBeNull();
  expect(sanitizeCondRule({ range: 'nope', kind: 'dataBar', color: '#2563EB' })).toBeNull();
});

test('sanitizeSparkline', () => {
  expect(sanitizeSparkline({ cell: 'B2', range: 'A1:A5', type: 'line' })).toEqual({
    cell: 'B2',
    range: 'A1:A5',
    type: 'line',
  });
  expect(sanitizeSparkline({ cell: 'B2', range: 'A1:A5', type: 'nope' })).toBeNull();
  expect(sanitizeSparkline({ cell: 'B2', type: 'line' })).toBeNull(); // sin rango
});

test('condRuleLabel legible por tipo y en el idioma de la interfaz', () => {
  const es = docsT('es');
  expect(condRuleLabel({ range: 'A1', kind: 'dataBar', color: '#2563EB' }, es)).toBe(
    'Barra de datos',
  );
  expect(condRuleLabel({ range: 'A1', kind: 'top', rank: 5, percent: true }, es)).toBe(
    '5 % superiores',
  );
  // El operador sale traducido: antes salía su CLAVE («cond.gt 5»).
  expect(condRuleLabel({ range: 'A1', op: 'gt', value: '5' }, es)).toBe('Mayor que 5');
  expect(
    condRuleLabel({ range: 'A1', kind: 'colorScale', scale: ['#fff', '#000'] }, docsT('de')),
  ).toBe('2-Farben-Skala');
});
