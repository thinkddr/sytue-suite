/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de las FORMAS y GRÁFICOS del EDITOR DE DOCUMENTOS
 * (src/editor/docblocks.ts): saneado de configuración, serializado/parseo de la
 * rejilla del gráfico y horneado del bloque <figure>/SVG (escape anti-XSS,
 * atributos data-*, reuso de la geometría de slides/shapes.ts y del pipeline de
 * datos de sheets/Chart.tsx). No re-cubre la geometría pura en sí (la validan
 * slides-shapes.test.ts y sheets-format.test.ts).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import {
  chartFigureHtml,
  chartSvg,
  DEFAULT_SHAPE_H,
  DEFAULT_SHAPE_W,
  docChartData,
  newChartGrid,
  parseChartGrid,
  sanitizeChartType,
  sanitizeFill,
  sanitizeShape,
  sanitizeShapeConfig,
  sanitizeSize,
  sanitizeStroke,
  serializeChartGrid,
  shapeFigureHtml,
  shapeSvg,
} from '../src/editor/docblocks';

/* ---------- Formas: saneado ---------- */

test('sanitizeShape: válida pasa, desconocida/nula → rect', () => {
  expect(sanitizeShape('star')).toBe('star');
  expect(sanitizeShape('cylinder')).toBe('cylinder');
  expect(sanitizeShape('inventada')).toBe('rect');
  expect(sanitizeShape(null)).toBe('rect');
});

test('sanitizeFill: hex de la paleta o transparent; si no, el defecto (transparent)', () => {
  expect(sanitizeFill('#2563EB')).toBe('#2563EB');
  expect(sanitizeFill('transparent')).toBe('transparent');
  expect(sanitizeFill('#123456')).toBe('transparent'); // fuera de paleta
  expect(sanitizeFill(null)).toBe('transparent');
});

test('sanitizeStroke: hex de la paleta; si no, la tinta por defecto', () => {
  expect(sanitizeStroke('#DC2626')).toBe('#DC2626');
  expect(sanitizeStroke('transparent')).toBe('#0A0A0A'); // el borde no admite transparent
  expect(sanitizeStroke('rojo')).toBe('#0A0A0A');
});

test('sanitizeSize: entero acotado; NaN/fuera de rango → el defecto dado', () => {
  expect(sanitizeSize('240', DEFAULT_SHAPE_W)).toBe(240);
  expect(sanitizeSize(10, DEFAULT_SHAPE_W)).toBe(24); // clamp al mínimo
  expect(sanitizeSize(99999, DEFAULT_SHAPE_H)).toBe(1600); // clamp al máximo
  expect(sanitizeSize('abc', DEFAULT_SHAPE_H)).toBe(DEFAULT_SHAPE_H);
});

test('sanitizeShapeConfig: rellena defectos y castea el texto', () => {
  expect(sanitizeShapeConfig({})).toEqual({
    shape: 'rect',
    w: DEFAULT_SHAPE_W,
    h: DEFAULT_SHAPE_H,
    fill: 'transparent',
    stroke: '#0A0A0A',
    text: '',
  });
  const cfg = sanitizeShapeConfig({
    shape: 'star',
    w: '200',
    h: '120',
    fill: '#16A34A',
    text: 'Hola',
  });
  expect(cfg).toEqual({
    shape: 'star',
    w: 200,
    h: 120,
    fill: '#16A34A',
    stroke: '#0A0A0A',
    text: 'Hola',
  });
});

/* ---------- Formas: horneado figure/SVG ---------- */

test('shapeFigureHtml lleva data-* (fuente de verdad) + SVG + contenteditable=false', () => {
  const html = shapeFigureHtml({
    shape: 'ellipse',
    w: 300,
    h: 160,
    fill: '#2563EB',
    stroke: '#0A0A0A',
    text: 'Etiqueta',
  });
  expect(html).toContain('data-shape');
  expect(html).toContain('data-shape-type="ellipse"');
  expect(html).toContain('data-w="300"');
  expect(html).toContain('data-h="160"');
  expect(html).toContain('data-fill="#2563EB"');
  expect(html).toContain('contenteditable="false"');
  expect(html).toContain('<ellipse');
  expect(html).toContain('Etiqueta');
});

test('shapeSvg reutiliza la geometría de slides: polígono, cilindro, elipse, línea/flecha, rect', () => {
  const base = { w: 200, h: 120, fill: 'transparent', stroke: '#0A0A0A', text: '' };
  expect(shapeSvg({ ...base, shape: 'star' })).toContain('<polygon');
  expect(shapeSvg({ ...base, shape: 'triangle' })).toContain('<polygon');
  expect(shapeSvg({ ...base, shape: 'cylinder' })).toContain('<path');
  expect(shapeSvg({ ...base, shape: 'ellipse' })).toContain('<ellipse');
  const arrow = shapeSvg({ ...base, shape: 'arrow' });
  expect(arrow).toContain('<line');
  expect(arrow).toContain('<path'); // punta de flecha
  expect(shapeSvg({ ...base, shape: 'rect' })).toContain('<rect');
});

test('shapeSvg: línea/flecha NO llevan capa de texto aunque se pida', () => {
  const cfg = { w: 200, h: 120, fill: 'transparent', stroke: '#0A0A0A', text: 'no visible' };
  expect(shapeSvg({ ...cfg, shape: 'line' })).not.toContain('foreignObject');
  expect(shapeSvg({ ...cfg, shape: 'rect' })).toContain('foreignObject'); // rect sí
});

test('shapeFigureHtml escapa el texto del usuario (anti-XSS)', () => {
  const html = shapeFigureHtml({
    shape: 'rect',
    w: 200,
    h: 120,
    fill: 'transparent',
    stroke: '#0A0A0A',
    text: '<script>alert(1)</script> a"b&c',
  });
  expect(html).not.toContain('<script>');
  expect(html).toContain('&lt;script&gt;');
  expect(html).toContain('&amp;');
});

/* ---------- Gráficos: tipo + rejilla ---------- */

test('sanitizeChartType: válido pasa, desconocido/nulo → bar', () => {
  expect(sanitizeChartType('line')).toBe('line');
  expect(sanitizeChartType('pie')).toBe('pie');
  expect(sanitizeChartType('scatter')).toBe('bar'); // no soportado en el documento
  expect(sanitizeChartType(null)).toBe('bar');
});

test('serializar → parsear conserva la rejilla (round-trip)', () => {
  const grid = [
    ['', 'A', 'B'],
    ['Ene', '10', '3'],
    ['Feb', '15', '7'],
  ];
  expect(parseChartGrid(serializeChartGrid(grid))).toEqual(grid);
});

test('parseChartGrid es tolerante: JSON inválido/no-array → rejilla de arranque', () => {
  expect(parseChartGrid('no es json')).toEqual(newChartGrid());
  expect(parseChartGrid('{"a":1}')).toEqual(newChartGrid());
  expect(parseChartGrid('[]')).toEqual(newChartGrid());
  expect(parseChartGrid(null)).toEqual(newChartGrid());
});

test('parseChartGrid rectangulariza (rellena filas cortas al ancho máximo)', () => {
  const grid = parseChartGrid(
    JSON.stringify([
      ['', 'A', 'B'],
      ['x', '1'],
    ]),
  );
  expect(grid).toEqual([
    ['', 'A', 'B'],
    ['x', '1', ''],
  ]);
});

test('parseChartGrid castea celdas no-string y acota el tamaño', () => {
  const grid = parseChartGrid(
    JSON.stringify([
      [1, 2],
      [null, 'x'],
    ]),
  );
  expect(grid).toEqual([
    ['1', '2'],
    ['', 'x'],
  ]);
});

test('docChartData interpreta la rejilla como un rango de sheets (cabecera + rótulos)', () => {
  const data = docChartData([
    ['', 'Ventas'],
    ['Ene', '10'],
    ['Feb', '15,5'],
  ]);
  expect(data.labels).toEqual(['Ene', 'Feb']);
  expect(data.series).toHaveLength(1);
  expect(data.series[0]!.name).toBe('Ventas');
  expect(data.series[0]!.values).toEqual([10, 15.5]); // coma decimal es-ES
  expect(
    docChartData([
      ['', 'Ventas'],
      ['Ene', '2%5%'],
    ]).series[0]!.values,
  ).toEqual([25]);
});

/* ---------- Gráficos: horneado figure/SVG ---------- */

test('chartFigureHtml lleva data-* (fuente de verdad) + SVG + contenteditable=false', () => {
  const grid = newChartGrid();
  const html = chartFigureHtml('bar', grid);
  expect(html).toContain('data-chart');
  expect(html).toContain('data-chart-type="bar"');
  expect(html).toContain('contenteditable="false"');
  expect(html).toContain('<svg');
  // El data-chart-grid horneado (escapado) decodifica a la misma rejilla.
  const decoded = html
    .match(/data-chart-grid="([^"]*)"/)![1]!
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&lt;/g, '<')
    .replace(/&gt;/g, '>')
    .replace(/&amp;/g, '&');
  expect(parseChartGrid(decoded)).toEqual(grid);
});

test('chartSvg: barras/líneas dibujan marcas; circular, sectores', () => {
  const grid = newChartGrid();
  expect(chartSvg('bar', grid)).toContain('<rect');
  expect(chartSvg('line', grid)).toContain('<path');
  const pie = chartSvg('pie', grid);
  expect(pie.startsWith('<svg')).toBe(true);
  expect(pie).toContain('<path'); // sectores
});

test('chartSvg sin datos muestra aviso sobrio (rejilla vacía o circular sin positivos)', () => {
  // Igual que el gráfico de diapositiva: rejilla vacía o circular sin valores > 0.
  expect(chartSvg('bar', [])).toContain('Sin datos numéricos');
  expect(
    chartSvg('pie', [
      ['', 'A'],
      ['Ene', '0'],
    ]),
  ).toContain('Sin datos numéricos');
});

test('chartFigureHtml escapa los datos del usuario (anti-XSS)', () => {
  const html = chartFigureHtml('bar', [
    ['', '<img src=x onerror=alert(1)>'],
    ['a"b&c', '5'],
  ]);
  expect(html).not.toContain('<img src=x');
  expect(html).toContain('&lt;img');
  expect(html).toContain('&amp;');
});
