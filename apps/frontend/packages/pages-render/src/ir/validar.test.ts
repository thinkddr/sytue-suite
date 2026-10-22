/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import { describe, expect, test } from 'bun:test';
import * as fs from 'node:fs';
import * as path from 'node:path';
import { LOCALES } from '@sytue/i18n';
import { IDIOMAS } from './tipos';
import { validar, type ModoValidacion, type Problema } from './validar';

describe('Validador IR (TS)', () => {
  test('LOCALES de @sytue/i18n es subconjunto de IDIOMAS del IR', () => {
    const setIdiomas = new Set(IDIOMAS as readonly string[]);
    for (const loc of LOCALES) {
      expect(setIdiomas.has(loc)).toBe(true);
    }
  });

  test('valida todos los ficheros emitidos por Go sin errores', () => {
    const dir = path.resolve(
      __dirname,
      '../../../../../../services/pages/internal/ir/testdata/emitidos',
    );
    if (!fs.existsSync(dir)) {
      throw new Error(`no existe el directorio de emitidos: ${dir}`);
    }
    const archivos = fs.readdirSync(dir).filter((f) => f.endsWith('.json'));
    expect(archivos.length).toBeGreaterThan(0);

    for (const arch of archivos) {
      const ruta = path.join(dir, arch);
      const crudo = JSON.parse(fs.readFileSync(ruta, 'utf-8'));
      const probs = validar(crudo, { modo: 'publicar' });
      const errores = probs.filter((p) => p.severity === 'error');
      expect(errores).toEqual([]);
    }
  });

  test('detecta los defectos esperados en todas las fixtures inválidas', () => {
    const dir = path.resolve(
      __dirname,
      '../../../../../../services/pages/internal/ir/testdata/invalidos',
    );
    if (!fs.existsSync(dir)) {
      throw new Error(`no existe el directorio de inválidos: ${dir}`);
    }
    const archivos = fs.readdirSync(dir).filter((f) => f.endsWith('.json'));
    expect(archivos.length).toBeGreaterThan(0);

    for (const arch of archivos) {
      const ruta = path.join(dir, arch);
      const wrapper = JSON.parse(fs.readFileSync(ruta, 'utf-8')) as {
        modo: ModoValidacion;
        ir: unknown;
        espera: Problema[];
      };

      const probs = validar(wrapper.ir, { modo: wrapper.modo });

      for (const exp of wrapper.espera) {
        const encontrado = probs.some(
          (p) => p.path === exp.path && p.code === exp.code && p.severity === exp.severity,
        );
        if (!encontrado) {
          throw new Error(
            `En ${arch}: esperaba problema ${JSON.stringify(exp)}, pero se obtuvieron: ${JSON.stringify(probs)}`,
          );
        }
      }
    }
  });
});
