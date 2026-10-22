// Financieras añadidas: MIRR, MDURATION y valores con descuento (DISC/INTRATE/
// RECEIVED). Valores contrastados con Excel. `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

function setCol(e: Engine, letter: string, vals: string[]): void {
  vals.forEach((v, i) => e.setCell('Hoja1', `${letter}${i + 1}`, v));
}
function show(formula: string): string {
  const e = new Engine();
  e.setCell('Hoja1', 'Z1', formula);
  return e.getDisplay('Hoja1', 'Z1');
}

test('MIRR: ejemplo clásico y caso simétrico', () => {
  const e = new Engine();
  setCol(e, 'A', ['-120000', '39000', '30000', '21000', '37000', '46000']);
  e.setCell('Hoja1', 'B1', '=MIRR(A1:A6, 0.1, 0.12)');
  expect(e.getValue('Hoja1', 'B1') as number).toBeCloseTo(0.126094, 4);

  const e2 = new Engine();
  setCol(e2, 'A', ['-100', '60', '60']);
  e2.setCell('Hoja1', 'B1', '=MIRR(A1:A3, 0.1, 0.1)');
  expect(e2.getValue('Hoja1', 'B1') as number).toBeCloseTo(0.122497, 5);
});

test('MIRR: sin flujos de ambos signos → #DIV/0!', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '2', '3']); // todos positivos
  e.setCell('Hoja1', 'B1', '=MIRR(A1:A3, 0.1, 0.1)');
  expect(e.getDisplay('Hoja1', 'B1')).toBe('#DIV/0!');
});

test('MDURATION = DURATION / (1 + rendim/frecuencia)', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=DATE(2020,1,1)');
  e.setCell('Hoja1', 'A2', '=DATE(2025,1,1)');
  e.setCell('Hoja1', 'D1', '=DURATION(A1, A2, 0.05, 0.05, 2)');
  e.setCell('Hoja1', 'M1', '=MDURATION(A1, A2, 0.05, 0.05, 2)');
  const dur = e.getValue('Hoja1', 'D1') as number;
  expect(e.getValue('Hoja1', 'M1') as number).toBeCloseTo(dur / 1.025, 9);
  expect(e.getValue('Hoja1', 'M1') as number).toBeCloseTo(4.37606, 4);
});

test('DISC / INTRATE / RECEIVED con base 30/360 (año completo)', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=DATE(2020,1,1)');
  e.setCell('Hoja1', 'A2', '=DATE(2021,1,1)'); // yearfrac base 0 = 1.0
  e.setCell('Hoja1', 'B1', '=DISC(A1, A2, 95, 100, 0)');
  e.setCell('Hoja1', 'B2', '=INTRATE(A1, A2, 95, 100, 0)');
  e.setCell('Hoja1', 'B3', '=RECEIVED(A1, A2, 95, 0.05, 0)');
  expect(e.getValue('Hoja1', 'B1') as number).toBeCloseTo(0.05, 9);
  expect(e.getValue('Hoja1', 'B2') as number).toBeCloseTo(5 / 95, 9);
  expect(e.getValue('Hoja1', 'B3') as number).toBeCloseTo(100, 9); // 95/(1-0.05)
});

test('DISC con medio año (base 30/360)', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=DATE(2020,1,1)');
  e.setCell('Hoja1', 'A2', '=DATE(2020,7,1)'); // yearfrac = 0.5
  e.setCell('Hoja1', 'B1', '=DISC(A1, A2, 97.5, 100, 0)');
  expect(e.getValue('Hoja1', 'B1') as number).toBeCloseTo(0.05, 9); // (2.5/100)/0.5
});

test('valores con descuento: errores de dominio', () => {
  expect(show('=DISC(DATE(2021,1,1), DATE(2020,1,1), 95, 100, 0)')).toBe('#NUM!'); // vencim ≤ liquid
  expect(show('=DISC(DATE(2020,1,1), DATE(2021,1,1), 95, 100, 5)')).toBe('#NUM!'); // base inválida
  expect(show('=INTRATE(DATE(2020,1,1), DATE(2021,1,1), 0, 100, 0)')).toBe('#NUM!'); // inversión ≤ 0
});
