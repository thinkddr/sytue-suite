/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de la lógica pura añadida a sheets/format.ts: portapapeles TSV
 * (serializar/parsear con interop Excel/Sheets), desplazamiento de referencias
 * al pegar/autorrellenar (shiftCellHtml), formato visual de celda
 * (sanitizeCellStyle + cellStyleCss) y quitar duplicados (dedupeRowIndices).
 * Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import {
  cellStyleCss,
  dedupeRowIndices,
  parseCell,
  parseTsv,
  renderCellHtml,
  sanitizeCellStyle,
  serializeTsv,
  shiftCellHtml,
} from '../src/editor/sheets/format';

/* ---------- Portapapeles TSV ---------- */

test('serializeTsv/parseTsv: ida y vuelta, tabs/saltos internos y bordes', () => {
  const grid = [
    ['A', 'B', 'C'],
    ['1', '2', '3'],
  ];
  const tsv = serializeTsv(grid);
  expect(tsv).toBe('A\tB\tC\n1\t2\t3');
  expect(parseTsv(tsv)).toEqual(grid);
  // Un tab/salto dentro de una celda se aplana (las celdas son de una línea).
  expect(serializeTsv([['a\tb', 'c\nd']])).toBe('a b\tc d');
  // Salto final (lo añaden los portapapeles) se ignora; una sola celda; vacío.
  expect(parseTsv('a\tb\n')).toEqual([['a', 'b']]);
  expect(parseTsv('x')).toEqual([['x']]);
  expect(parseTsv('')).toEqual([['']]);
  expect(parseTsv('a\r\nb')).toEqual([['a'], ['b']]); // CRLF de Windows
});

/* ---------- shiftCellHtml (pegado + autorelleno) ---------- */

test('shiftCellHtml: ajusta refs de fórmula conservando formato; literal intacto', () => {
  expect(shiftCellHtml('<b>=A1+B1</b>', 1, 2)).toBe('<b>=B3+C3</b>');
  expect(shiftCellHtml('=SUMA(A1:A3)', 2, 0)).toBe('=SUMA(C1:C3)');
  expect(shiftCellHtml('hola', 3, 3)).toBe('hola'); // literal no se toca
  expect(shiftCellHtml('', 1, 1)).toBe(''); // vacía
  expect(shiftCellHtml('=A1', 0, -1)).toBe('#REF!'); // fuera de rejilla → literal #REF!
  // Alineación + cursiva se conservan al desplazar.
  expect(shiftCellHtml('<div style="text-align: right;"><i>=A1</i></div>', 1, 0)).toBe(
    '<div style="text-align: right;"><i>=B1</i></div>',
  );
});

/* ---------- Formato visual de celda ---------- */

test('sanitizeCellStyle: acepta lo válido, clampa tamaño, rechaza lo arbitrario', () => {
  expect(
    sanitizeCellStyle({
      font: 'Arial',
      size: 14,
      color: '#FF0000',
      fill: '#00FF00',
      u: true,
      s: true,
      valign: 'middle',
      wrap: true,
      border: { t: true, x: 9 },
    }),
  ).toEqual({
    font: 'Arial',
    size: 14,
    color: '#FF0000',
    fill: '#00FF00',
    u: true,
    s: true,
    valign: 'middle',
    wrap: true,
    border: { t: true },
  });
  expect(sanitizeCellStyle({ size: 200 })).toEqual({ size: 96 }); // clamp alto
  expect(sanitizeCellStyle({ size: 2 })).toEqual({ size: 6 }); // clamp bajo
  expect(sanitizeCellStyle({ font: 'Comic Sans' })).toBeNull(); // familia fuera del catálogo
  expect(sanitizeCellStyle({ color: 'red' })).toBeNull(); // solo #RRGGBB
  expect(sanitizeCellStyle({ valign: 'diagonal' })).toBeNull();
  expect(sanitizeCellStyle({ border: {} })).toBeNull(); // borde sin lados → nada
  expect(sanitizeCellStyle({})).toBeNull(); // objeto vacío no se guarda
  // Del borde, el color y el trazo de los lados que se ven; nada raro llega al style inline.
  expect(
    sanitizeCellStyle({
      border: {
        t: true,
        tc: '#FF0000',
        ts: 'double',
        b: false,
        bc: '#00FF00',
        r: true,
        rc: 'red',
        rs: 'groove',
      },
    }),
  ).toEqual({ border: { t: true, tc: '#FF0000', ts: 'double', r: true } });
  expect(sanitizeCellStyle('cadena')).toBeNull();
});

test('cellStyleCss: propiedades CSS correctas y SIN background (lo gestiona el lienzo)', () => {
  expect(cellStyleCss({ font: 'Georgia' })).toEqual({ fontFamily: 'Georgia, serif' });
  expect(cellStyleCss({ size: 18 })).toEqual({ fontSize: '18pt' });
  expect(cellStyleCss({ color: '#123456' })).toEqual({ color: '#123456' });
  expect(cellStyleCss({ u: true, s: true })).toEqual({
    textDecorationLine: 'underline line-through',
  });
  expect(cellStyleCss({ wrap: true })).toEqual({ whiteSpace: 'normal', wordBreak: 'break-word' });
  expect(cellStyleCss({ valign: 'bottom' })).toEqual({
    display: 'flex',
    flexDirection: 'column',
    justifyContent: 'flex-end',
  });
  expect(cellStyleCss({ border: { t: true, b: true } })).toEqual({
    borderTop: '1px solid var(--sytue-border-strong)',
    borderBottom: '1px solid var(--sytue-border-strong)',
  });
  // El trazo y el color de cada lado, de un libro importado; sin ellos, la línea fina de serie.
  expect(
    cellStyleCss({
      border: {
        t: true,
        ts: 'medium',
        tc: '#FF0000',
        r: true,
        rs: 'dashed',
        l: true,
        lc: '#00B050',
      },
    }),
  ).toEqual({
    borderTop: '2px solid #FF0000',
    borderRight: '1px dashed var(--sytue-border-strong)',
    borderLeft: '1px solid #00B050',
  });
  // El relleno NUNCA sale como background aquí (el lienzo lo mezcla con selección/condicional).
  expect(cellStyleCss({ fill: '#FFEEDD' })).toEqual({});
});

/* ---------- Quitar duplicados ---------- */

test('dedupeRowIndices: conserva la primera aparición de cada clave', () => {
  expect(dedupeRowIndices(['a', 'b', 'a', 'c', 'b'])).toEqual([0, 1, 3]);
  expect(dedupeRowIndices(['x', 'x', 'x'])).toEqual([0]);
  expect(dedupeRowIndices(['p', 'q', 'r'])).toEqual([0, 1, 2]); // sin duplicados
  expect(dedupeRowIndices([])).toEqual([]);
});

/* ---------- Envoltorio HTML de celda (movido a format.ts) ---------- */

test('parseCell/renderCellHtml siguen siendo inversas tras mover a format.ts', () => {
  const html = '<div style="text-align: center;"><b><i>Hola</i></b></div>';
  const p = parseCell(html);
  expect(p).toEqual({ align: 'center', b: true, i: true, inner: 'Hola' });
  expect(renderCellHtml(p)).toBe(html);
  expect(renderCellHtml(parseCell(''))).toBe(''); // vacía
});
