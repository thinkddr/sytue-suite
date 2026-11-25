/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las funciones bajo demanda (perezosas.ts): los nombres sueltos son los del registro, el
 * motor sin funciones da `#BUSY!` y nunca un valor equivocado, y `./motor` no las trae.
 */
import { describe, expect, test } from 'bun:test';
import { readFileSync, writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { buildRegistry } from './functions/index.js';
import { nombresDeFunciones } from './catalogo.js';
import { Engine } from './engine.js';
import { NOMBRES_REGISTRO, nombresDeFuncionesSinRegistro } from './nombres-funciones.js';
import { cargarFunciones, funcionesCargadas, instalarFunciones } from './perezosas.js';
import type { Registry } from './registry.js';
import { ERR } from './values.js';

describe('las funciones bajo demanda', () => {
  test('los nombres sueltos son los del registro (GENERAR=1 los reescribe)', () => {
    const nombres = buildRegistry().names();
    if (process.env['GENERAR'] === '1') {
      const ruta = fileURLToPath(new URL('./nombres-funciones.ts', import.meta.url));
      const fuente = readFileSync(ruta, 'utf8');
      const inicio = fuente.indexOf('= [\n') + 4;
      const fin = fuente.indexOf('\n];', inicio);
      const lista = nombres.map((n) => `  '${n}',`).join('\n');
      writeFileSync(ruta, fuente.slice(0, inicio) + lista + fuente.slice(fin));
    }
    expect([...NOMBRES_REGISTRO]).toEqual(nombres);
    expect(nombresDeFuncionesSinRegistro()).toEqual(nombresDeFunciones());
  });

  test('sin las funciones, una fórmula con una da #BUSY! y una sin ninguna se calcula', () => {
    // El estado del editor antes de `cargarFunciones`; se repone la fábrica al acabar.
    instalarFunciones(undefined as unknown as () => Registry);
    try {
      expect(funcionesCargadas()).toBe(false);
      const e = new Engine();
      expect(e.esperaFunciones).toBe(true);
      e.setCell('Hoja1', 'A1', '2');
      e.setCell('Hoja1', 'A2', '=A1*3');
      e.setCell('Hoja1', 'A3', '=SUM(A1,3)*2');
      e.setCell('Hoja1', 'A4', '=NOEXISTE(1)');
      expect(e.getValue('Hoja1', 'A2')).toBe(6);
      expect(String(e.getValue('Hoja1', 'A3'))).toBe(ERR.BUSY.code);
      expect(String(e.getValue('Hoja1', 'A4'))).toBe(ERR.NAME.code);
    } finally {
      instalarFunciones(buildRegistry);
    }
  });

  test('un motor construido con las funciones calcula', async () => {
    await cargarFunciones();
    expect(funcionesCargadas()).toBe(true);
    const e = new Engine();
    expect(e.esperaFunciones).toBe(false);
    e.setCell('Hoja1', 'A1', '2');
    e.setCell('Hoja1', 'A2', '=SUM(A1,3)*2');
    expect(e.getValue('Hoja1', 'A2')).toBe(10);
  });

  test('`./motor` no importa el registro de funciones', () => {
    for (const fichero of ['./motor.ts', './engine.ts', './ligero.ts', './perezosas.ts']) {
      const fuente = readFileSync(new URL(fichero, import.meta.url), 'utf8');
      expect(fuente).not.toMatch(/from '\.\/functions\/index\.js'/);
      expect(fuente).not.toMatch(/from '\.\/catalogo\.js'/);
      expect(fuente).not.toMatch(/from '\.\/index\.js'/);
    }
  });
});
