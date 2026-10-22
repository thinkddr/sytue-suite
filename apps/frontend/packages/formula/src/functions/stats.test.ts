// Checks de estadística descriptiva (stats.ts). Valores contrastados con los
// de una hoja de cálculo real (Excel/LibreOffice). `bun test`.
import { expect, test } from 'bun:test';
import { Engine } from '../engine.js';

/** Rellena una columna de Hoja1 desde la fila 1 con los textos crudos dados. */
function setCol(e: Engine, letter: string, vals: string[]): void {
  vals.forEach((v, i) => e.setCell('Hoja1', `${letter}${i + 1}`, v));
}

test('AVERAGE: números, ignora texto/blanco en rango, #DIV/0! sin números', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '2', '3', '4', '5']);
  e.setCell('Hoja1', 'B1', '=AVERAGE(A1:A5)');
  expect(e.getValue('Hoja1', 'B1')).toBe(3);
  // texto y blanco en el rango se ignoran
  setCol(e, 'C', ['10', 'texto', '', '20']);
  e.setCell('Hoja1', 'B2', '=AVERAGE(C1:C4)');
  expect(e.getValue('Hoja1', 'B2')).toBe(15);
  // booleano directo cuenta; en rango se ignora
  e.setCell('Hoja1', 'B3', '=AVERAGE(1, TRUE)');
  expect(e.getValue('Hoja1', 'B3')).toBe(1);
  e.setCell('Hoja1', 'D1', 'TRUE');
  e.setCell('Hoja1', 'B4', '=AVERAGE(D1)');
  expect(e.getDisplay('Hoja1', 'B4')).toBe('#DIV/0!');
});

test('AVERAGEA: texto=0 y booleanos cuentan también en rango', () => {
  const e = new Engine();
  setCol(e, 'A', ['5', 'texto', 'TRUE']); // 5, 0, 1
  e.setCell('Hoja1', 'B1', '=AVERAGEA(A1:A3)');
  expect(e.getValue('Hoja1', 'B1')).toBe(2); // (5+0+1)/3
  // blanco se ignora (no entra en el denominador)
  setCol(e, 'C', ['4', '', '8']);
  e.setCell('Hoja1', 'B2', '=AVERAGEA(C1:C3)');
  expect(e.getValue('Hoja1', 'B2')).toBe(6);
});

test('COUNT / COUNTA / COUNTBLANK', () => {
  const e = new Engine();
  // A1 número, A2 texto, A3 blanco, A4 booleano, A5 error, A6 número
  setCol(e, 'A', ['1', 'texto', '', 'TRUE', '=1/0', '7']);
  e.setCell('Hoja1', 'B1', '=COUNT(A1:A6)'); // solo 1 y 7
  expect(e.getValue('Hoja1', 'B1')).toBe(2);
  e.setCell('Hoja1', 'B2', '=COUNTA(A1:A6)'); // todo menos el blanco A3
  expect(e.getValue('Hoja1', 'B2')).toBe(5);
  e.setCell('Hoja1', 'B3', '=COUNTBLANK(A1:A6)'); // solo A3
  expect(e.getValue('Hoja1', 'B3')).toBe(1);
  // COUNT de literales directos
  e.setCell('Hoja1', 'B4', '=COUNT(1, 2, 3)');
  expect(e.getValue('Hoja1', 'B4')).toBe(3);
});

test('MAX / MIN: ignoran texto y booleanos de rango; vacío → 0', () => {
  const e = new Engine();
  setCol(e, 'A', ['3', '-1', '2', 'texto']);
  e.setCell('Hoja1', 'B1', '=MAX(A1:A4)');
  e.setCell('Hoja1', 'B2', '=MIN(A1:A4)');
  expect(e.getValue('Hoja1', 'B1')).toBe(3);
  expect(e.getValue('Hoja1', 'B2')).toBe(-1);
  // rango totalmente vacío → 0
  e.setCell('Hoja1', 'B3', '=MAX(Z1:Z9)');
  expect(e.getValue('Hoja1', 'B3')).toBe(0);
  // booleano directo cuenta en MAX
  e.setCell('Hoja1', 'B4', '=MAX(FALSE, 0.5)');
  expect(e.getValue('Hoja1', 'B4')).toBe(0.5);
});

test('MAXA / MINA: el texto vale 0, los booleanos cuentan', () => {
  const e = new Engine();
  setCol(e, 'A', ['-5', 'texto']); // -5 y 0
  e.setCell('Hoja1', 'B1', '=MAXA(A1:A2)');
  expect(e.getValue('Hoja1', 'B1')).toBe(0);
  e.setCell('Hoja1', 'B2', '=MINA(TRUE, 2)'); // min(1,2)
  expect(e.getValue('Hoja1', 'B2')).toBe(1);
});

test('MEDIAN: par promedia el centro; impar el central; vacío → #NUM!', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=MEDIAN(1, 2, 3, 4)');
  expect(e.getValue('Hoja1', 'A1')).toBe(2.5);
  e.setCell('Hoja1', 'A2', '=MEDIAN(1, 3, 2)');
  expect(e.getValue('Hoja1', 'A2')).toBe(2);
  e.setCell('Hoja1', 'A3', '=MEDIAN(Z1:Z9)');
  expect(e.getDisplay('Hoja1', 'A3')).toBe('#NUM!');
});

test('MODE: más frecuente, primero en empate, #N/A si nada se repite', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=MODE(1, 2, 2, 3)');
  expect(e.getValue('Hoja1', 'A1')).toBe(2);
  e.setCell('Hoja1', 'A2', '=MODE(3, 3, 2, 2)'); // empate → el primero (3)
  expect(e.getValue('Hoja1', 'A2')).toBe(3);
  e.setCell('Hoja1', 'A3', '=MODE(1, 2, 3)');
  expect(e.getDisplay('Hoja1', 'A3')).toBe('#N/A');
});

test('STDEV/STDEVP/VAR/VARP con el dataset canónico {2,4,4,4,5,5,7,9}', () => {
  const e = new Engine();
  setCol(e, 'A', ['2', '4', '4', '4', '5', '5', '7', '9']); // media 5, ΣΣ=32
  e.setCell('Hoja1', 'B1', '=VARP(A1:A8)');
  e.setCell('Hoja1', 'B2', '=STDEVP(A1:A8)');
  e.setCell('Hoja1', 'B3', '=VAR(A1:A8)');
  e.setCell('Hoja1', 'B4', '=STDEV(A1:A8)');
  expect(e.getValue('Hoja1', 'B1')).toBe(4); // 32/8
  expect(e.getValue('Hoja1', 'B2')).toBe(2); // sqrt(4)
  expect(e.getValue('Hoja1', 'B3') as number).toBeCloseTo(32 / 7, 10);
  expect(e.getValue('Hoja1', 'B4') as number).toBeCloseTo(Math.sqrt(32 / 7), 10);
});

test('STDEV con <2 valores → #DIV/0!; STDEVP con 1 → 0', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=STDEV(5)');
  expect(e.getDisplay('Hoja1', 'A1')).toBe('#DIV/0!');
  e.setCell('Hoja1', 'A2', '=STDEVP(5)');
  expect(e.getValue('Hoja1', 'A2')).toBe(0);
  e.setCell('Hoja1', 'A3', '=VARP(5)');
  expect(e.getValue('Hoja1', 'A3')).toBe(0);
});

test('LARGE / SMALL y límites de k → #NUM!', () => {
  const e = new Engine();
  setCol(e, 'A', ['3', '1', '4', '1', '5']); // desc: 5,4,3,1,1 | asc: 1,1,3,4,5
  e.setCell('Hoja1', 'B1', '=LARGE(A1:A5, 1)');
  e.setCell('Hoja1', 'B2', '=LARGE(A1:A5, 2)');
  e.setCell('Hoja1', 'B3', '=SMALL(A1:A5, 1)');
  e.setCell('Hoja1', 'B4', '=SMALL(A1:A5, 3)');
  expect(e.getValue('Hoja1', 'B1')).toBe(5);
  expect(e.getValue('Hoja1', 'B2')).toBe(4);
  expect(e.getValue('Hoja1', 'B3')).toBe(1);
  expect(e.getValue('Hoja1', 'B4')).toBe(3);
  e.setCell('Hoja1', 'B5', '=LARGE(A1:A5, 0)');
  e.setCell('Hoja1', 'B6', '=SMALL(A1:A5, 6)');
  expect(e.getDisplay('Hoja1', 'B5')).toBe('#NUM!');
  expect(e.getDisplay('Hoja1', 'B6')).toBe('#NUM!');
});

test('RANK: descendente por defecto, ascendente con orden≠0, #N/A si falta', () => {
  const e = new Engine();
  setCol(e, 'A', ['3', '1', '4', '1', '5']);
  e.setCell('Hoja1', 'B1', '=RANK(4, A1:A5)'); // desc: 5>4 → 2
  expect(e.getValue('Hoja1', 'B1')).toBe(2);
  e.setCell('Hoja1', 'B2', '=RANK(1, A1:A5)'); // desc: 5,4,3 por encima → 4 (empate)
  expect(e.getValue('Hoja1', 'B2')).toBe(4);
  e.setCell('Hoja1', 'B3', '=RANK(1, A1:A5, 1)'); // asc: ninguno menor → 1
  expect(e.getValue('Hoja1', 'B3')).toBe(1);
  e.setCell('Hoja1', 'B4', '=RANK(9, A1:A5)'); // no está → #N/A
  expect(e.getDisplay('Hoja1', 'B4')).toBe('#N/A');
});

test('PERCENTILE / QUARTILE (interpolación inclusiva) y dominio', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '2', '3', '4']);
  e.setCell('Hoja1', 'B1', '=PERCENTILE(A1:A4, 0.25)');
  e.setCell('Hoja1', 'B2', '=PERCENTILE(A1:A4, 0.5)');
  expect(e.getValue('Hoja1', 'B1')).toBe(1.75);
  expect(e.getValue('Hoja1', 'B2')).toBe(2.5);
  e.setCell('Hoja1', 'B3', '=QUARTILE(A1:A4, 1)'); // = percentil 0.25
  e.setCell('Hoja1', 'B4', '=QUARTILE(A1:A4, 2)'); // mediana
  e.setCell('Hoja1', 'B5', '=QUARTILE(A1:A4, 0)'); // mínimo
  e.setCell('Hoja1', 'B6', '=QUARTILE(A1:A4, 4)'); // máximo
  expect(e.getValue('Hoja1', 'B3')).toBe(1.75);
  expect(e.getValue('Hoja1', 'B4')).toBe(2.5);
  expect(e.getValue('Hoja1', 'B5')).toBe(1);
  expect(e.getValue('Hoja1', 'B6')).toBe(4);
  // dominio fuera de rango
  e.setCell('Hoja1', 'B7', '=PERCENTILE(A1:A4, 1.5)');
  e.setCell('Hoja1', 'B8', '=QUARTILE(A1:A4, 5)');
  expect(e.getDisplay('Hoja1', 'B7')).toBe('#NUM!');
  expect(e.getDisplay('Hoja1', 'B8')).toBe('#NUM!');
});

test('GEOMEAN / HARMEAN y su dominio (valores > 0)', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=GEOMEAN(4, 9)'); // sqrt(36)
  expect(e.getValue('Hoja1', 'A1')).toBe(6);
  e.setCell('Hoja1', 'A2', '=HARMEAN(1, 4, 4)'); // 3 / (1 + 0.25 + 0.25)
  expect(e.getValue('Hoja1', 'A2')).toBe(2);
  e.setCell('Hoja1', 'A3', '=GEOMEAN(1, 0)'); // 0 no permitido
  expect(e.getDisplay('Hoja1', 'A3')).toBe('#NUM!');
  e.setCell('Hoja1', 'A4', '=HARMEAN(-1, 2)'); // negativo no permitido
  expect(e.getDisplay('Hoja1', 'A4')).toBe('#NUM!');
});

test('los errores dentro de un rango se propagan (salvo COUNT*)', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '=1/0', '3']);
  e.setCell('Hoja1', 'B1', '=AVERAGE(A1:A3)');
  e.setCell('Hoja1', 'B2', '=MAX(A1:A3)');
  expect(e.getDisplay('Hoja1', 'B1')).toBe('#DIV/0!');
  expect(e.getDisplay('Hoja1', 'B2')).toBe('#DIV/0!');
  // COUNT/COUNTA no propagan el error
  e.setCell('Hoja1', 'B3', '=COUNT(A1:A3)'); // cuenta 1 y 3
  expect(e.getValue('Hoja1', 'B3')).toBe(2);
  e.setCell('Hoja1', 'B4', '=COUNTA(A1:A3)'); // incluye la celda de error
  expect(e.getValue('Hoja1', 'B4')).toBe(3);
});

// ---- Endurecimiento: bordes de interpolación e identidades (contrastados) ----

test('PERCENTILE en los extremos p=0/p=1 → mínimo/máximo', () => {
  const e = new Engine();
  setCol(e, 'A', ['4', '1', '3', '2']); // desordenado a propósito
  e.setCell('Hoja1', 'B1', '=PERCENTILE(A1:A4, 0)');
  e.setCell('Hoja1', 'B2', '=PERCENTILE(A1:A4, 1)');
  expect(e.getValue('Hoja1', 'B1')).toBe(1);
  expect(e.getValue('Hoja1', 'B2')).toBe(4);
});

test('PERCENTILE/QUARTILE interpolan sobre 8 valores (2.75, 3.7) e identidad con MEDIAN', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '2', '3', '4', '5', '6', '7', '8']);
  e.setCell('Hoja1', 'B1', '=QUARTILE(A1:A8, 1)'); // percentil 0.25 → 2.75
  e.setCell('Hoja1', 'B2', '=QUARTILE(A1:A8, 2)'); // == MEDIAN
  e.setCell('Hoja1', 'B3', '=MEDIAN(A1:A8)');
  expect(e.getValue('Hoja1', 'B1') as number).toBeCloseTo(2.75, 10);
  expect(e.getValue('Hoja1', 'B2')).toBe(e.getValue('Hoja1', 'B3'));
  setCol(e, 'C', ['1', '2', '3', '4', '5', '6', '7', '8', '9', '10']);
  e.setCell('Hoja1', 'B4', '=PERCENTILE(C1:C10, 0.3)');
  expect(e.getValue('Hoja1', 'B4') as number).toBeCloseTo(3.7, 10);
});

test('MODE: con modas entrelazadas gana la que aparece primero', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'A1', '=MODE(1, 1, 2, 2, 3)'); // 1 y 2 empatan → 1
  expect(e.getValue('Hoja1', 'A1')).toBe(1);
});

test('k-funciones sobre rango vacío → error de dominio', () => {
  const e = new Engine();
  e.setCell('Hoja1', 'B1', '=LARGE(Z1:Z9, 1)');
  e.setCell('Hoja1', 'B2', '=SMALL(Z1:Z9, 1)');
  e.setCell('Hoja1', 'B3', '=PERCENTILE(Z1:Z9, 0.5)');
  e.setCell('Hoja1', 'B4', '=GEOMEAN(Z1:Z9)');
  e.setCell('Hoja1', 'B5', '=HARMEAN(Z1:Z9)');
  for (const c of ['B1', 'B2', 'B3', 'B4', 'B5']) {
    expect(e.getDisplay('Hoja1', c)).toBe('#NUM!');
  }
});

test('GEOMEAN/HARMEAN con varios valores (medias exactas)', () => {
  const e = new Engine();
  setCol(e, 'A', ['1', '2', '3', '4', '5']);
  e.setCell('Hoja1', 'B1', '=GEOMEAN(A1:A5)'); // 120^(1/5)
  e.setCell('Hoja1', 'B2', '=HARMEAN(A1:A5)'); // 5 / Σ(1/x)
  expect(e.getValue('Hoja1', 'B1') as number).toBeCloseTo(Math.pow(120, 1 / 5), 10);
  expect(e.getValue('Hoja1', 'B2') as number).toBeCloseTo(
    5 / (1 + 1 / 2 + 1 / 3 + 1 / 4 + 1 / 5),
    10,
  );
});

test('LARGE/SMALL/QUARTILE truncan el índice fraccional (decisión OpenFormula)', () => {
  const e = new Engine();
  setCol(e, 'A', ['10', '20', '30']);
  e.setCell('Hoja1', 'B1', '=LARGE(A1:A3, 1.9)'); // trunc→1 → 30
  e.setCell('Hoja1', 'B2', '=SMALL(A1:A3, 2.9)'); // trunc→2 → 20
  expect(e.getValue('Hoja1', 'B1')).toBe(30);
  expect(e.getValue('Hoja1', 'B2')).toBe(20);
  setCol(e, 'C', ['1', '2', '3', '4']);
  e.setCell('Hoja1', 'B3', '=QUARTILE(C1:C4, 1.9)'); // trunc→1 → 1.75
  expect(e.getValue('Hoja1', 'B3')).toBe(1.75);
});
