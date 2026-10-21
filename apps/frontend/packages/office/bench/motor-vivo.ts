/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Benchmark de lo que cuesta UNA edición en el editor de Calcus con 10.000 y 100.000
 * celdas: reconstruir el motor entero (lo que se hacía antes) frente a ponerlo al día con
 * la diferencia (`sincronizarMotor`), con y sin la pista de la celda editada. Es el
 * camino del editor; el del motor solo está en packages/formula/bench/calcus.ts.
 *
 *   cd apps/frontend/packages/office && bun bench/motor-vivo.ts [--tamanos 10000,100000]
 *
 * Un script, no un test: los tiempos dependen de la máquina.
 */
import { anotarCambio, construirMotor, sincronizarMotor } from '../src/editor/sheets/motor-vivo';
import type { SheetModel } from '../src/editor/types';

const H = 'Hoja1';

function hoja(n: number): { cells: Record<string, string>; filas: number } {
  const filas = Math.max(1, Math.floor(n / 4));
  const cells: Record<string, string> = {};
  for (let i = 1; i <= filas; i++) {
    cells[`A${i}`] = String((i * 37) % 1000);
    cells[`B${i}`] = `=A${i}*1.21`;
    cells[`C${i}`] = `=IF(B${i}>100,1,0)`;
    cells[`D${i}`] = i === 1 ? '=B1' : `=D${i - 1}+B${i}`;
  }
  cells['F1'] = `=SUM(B1:B${filas})`;
  return { cells, filas };
}

const modelo = (cells: Record<string, string>): SheetModel => ({
  active: 0,
  sheets: [{ name: H, cells }],
});

function mediana(f: () => void, veces = 5): number {
  const t: number[] = [];
  for (let i = 0; i < veces; i++) {
    const t0 = performance.now();
    f();
    t.push(performance.now() - t0);
  }
  return t.sort((a, b) => a - b)[Math.floor(veces / 2)];
}

function linea(texto: string): void {
  process.stdout.write(`${texto}\n`);
}

const a = process.argv.indexOf('--tamanos');
const tamanos = (a >= 0 ? process.argv[a + 1] : '10000,100000').split(',').map(Number);

for (const n of tamanos) {
  const { cells, filas } = hoja(n);
  /** Lo que lee el editor tras una edición: la ventana visible y el resumen. */
  const leerVentana = (e: { getValue: (h: string, r: string) => unknown }): void => {
    for (let i = 1; i <= 60; i++) {
      e.getValue(H, `B${i}`);
      e.getValue(H, `C${i}`);
      e.getValue(H, `D${i}`);
    }
    e.getValue(H, 'F1');
  };
  linea(`# ${n} celdas (${filas} filas) · bun ${process.versions.bun} · mediana de 5`);
  linea('escenario'.padEnd(46) + 'ms'.padStart(9));

  // Edición de una celda suelta (H1), como si se tecleara una nota al lado de los datos.
  const reconstruir = mediana(() => {
    const v = construirMotor(modelo({ ...cells, H1: 'nota' }));
    leerVentana(v.engine);
  });
  linea(
    'edicion suelta · reconstruir entero (antes)'.padEnd(46) + reconstruir.toFixed(1).padStart(9),
  );

  let vivo = construirMotor(modelo(cells));
  leerVentana(vivo.engine);
  let prev = cells;
  let k = 0;
  const sinPista = mediana(() => {
    const nuevas = { ...prev, H1: `n${k++}` };
    vivo = sincronizarMotor(vivo, modelo(nuevas)).vivo;
    leerVentana(vivo.engine);
    prev = nuevas;
  });
  linea('edicion suelta · sincronizar sin pista'.padEnd(46) + sinPista.toFixed(1).padStart(9));

  const conPista = mediana(() => {
    const nuevas = { ...prev, H1: `n${k++}` };
    anotarCambio(nuevas, prev, ['H1']);
    vivo = sincronizarMotor(vivo, modelo(nuevas)).vivo;
    leerVentana(vivo.engine);
    prev = nuevas;
  });
  linea('edicion suelta · sincronizar con pista'.padEnd(46) + conPista.toFixed(1).padStart(9));

  // Lo que cuesta el modelo inmutable por sí solo: copiar las celdas.
  linea(
    '(copiar las celdas del modelo, suelo)'.padEnd(46) +
      mediana(() => ({ ...prev, H2: 'x' }))
        .toFixed(1)
        .padStart(9),
  );
}
