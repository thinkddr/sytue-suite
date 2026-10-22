/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';

const ruta = new URL('./editar-ia.ts', import.meta.url).pathname;
const fuente = await Bun.file(ruta).text();

test('la instrucción del usuario viaja en context y la directiva de sistema es fija', () => {
  expect(fuente).toContain('context: contextoFinal');
  const bloque = fuente.slice(fuente.indexOf('const INSTRUCCION'), fuente.indexOf('].join'));
  expect(bloque).not.toContain('${');
  expect(fuente).toContain('instruction: INSTRUCCION');
});

test('la directiva de sistema ordena no identificar personas y responder estrictamente en JSON', () => {
  expect(fuente).toContain('Do not name or identify people');
  expect(fuente).toContain('Answer ONLY with a JSON object');
});

test('las validaciones de entrada ocurren antes de descargar la imagen', () => {
  const sinId = fuente.indexOf("'missing_id'");
  const sinInstruccion = fuente.indexOf("'missing_instruction'");
  const larga = fuente.indexOf("'instruction_too_long'");
  const imagen = fuente.indexOf('await miniaturaEnBase64(');

  expect(sinId).toBeGreaterThan(0);
  expect(sinInstruccion).toBeGreaterThan(0);
  expect(larga).toBeGreaterThan(0);
  expect(imagen).toBeGreaterThan(sinId);
  expect(imagen).toBeGreaterThan(sinInstruccion);
  expect(imagen).toBeGreaterThan(larga);
});

test('no modifica el fichero original en Drive (edición no destructiva)', () => {
  expect(fuente).not.toContain('guardarFoto');
  expect(fuente).not.toContain('driveClient');
  expect(fuente).not.toContain('PUT');
});
