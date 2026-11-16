/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * QUÉ PASO SIGUIENTE OFRECE CADA VACÍO, SEGÚN EL ROL: el propietario crea tablas y
 * campos; quien edita añade registros; quien lee no ve botones que el servicio le
 * negaría. Volver a verlo todo desde un recorte es de cualquiera.
 */
import { expect, test } from 'bun:test';
import type { BasesView } from '@sytue/api-clients/bases';
import { SIN_LECTURA } from './lectura.js';
import { esRecorte, vacioDe } from './vacios.js';

const PROPIETARIO = { esquema: true, escribir: true };
const EDITOR = { esquema: false, escribir: true };
const LECTOR = { esquema: false, escribir: false };

test('una base sin tablas: crear o importar para el propietario, nada para los demás', () => {
  expect(vacioDe('sinTablas', PROPIETARIO).acciones).toEqual(['crearTabla', 'importarCsv']);
  expect(vacioDe('sinTablas', PROPIETARIO).descripcion).toBe('base.noTables.owner');
  for (const rol of [EDITOR, LECTOR]) {
    expect(vacioDe('sinTablas', rol).acciones).toEqual([]);
    expect(vacioDe('sinTablas', rol).descripcion).toBe('base.noTables.reader');
  }
});

test('una tabla sin campos: añadir campo solo el propietario', () => {
  expect(vacioDe('sinCampos', PROPIETARIO).acciones).toEqual(['anadirCampo']);
  expect(vacioDe('sinCampos', EDITOR).acciones).toEqual([]);
  expect(vacioDe('sinCampos', LECTOR).acciones).toEqual([]);
  expect(vacioDe('sinCampos', EDITOR).descripcion).toBe('grid.noFields.reader');
});

test('una tabla sin registros: añadir registro quien puede escribir', () => {
  expect(vacioDe('sinRegistros', PROPIETARIO).acciones).toEqual(['anadirRegistro']);
  expect(vacioDe('sinRegistros', EDITOR).acciones).toEqual(['anadirRegistro']);
  expect(vacioDe('sinRegistros', LECTOR).acciones).toEqual([]);
  expect(vacioDe('sinRegistros', LECTOR).descripcion).toBe('grid.empty.reader');
});

test('un recorte sin resultados: ver todos, sea cual sea el rol', () => {
  for (const rol of [PROPIETARIO, EDITOR, LECTOR]) {
    expect(vacioDe('sinResultados', rol).acciones).toEqual(['verTodos']);
  }
  expect(vacioDe('sinResultados', LECTOR, true).titulo).toBe('grid.noMatches');
  expect(vacioDe('sinResultados', LECTOR, false).titulo).toBe('grid.viewEmpty');
});

test('recorta una búsqueda escrita o una vista con filtros; ordenar no recorta', () => {
  const vista = (filtros: number): BasesView =>
    ({
      id: 'v',
      table_id: 't',
      name: 'Abiertos',
      kind: 'grid',
      config: filtros
        ? { filters: Array.from({ length: filtros }, () => ({ field: 'f', op: 'eq', value: 'x' })) }
        : {},
    }) as BasesView;
  expect(esRecorte(SIN_LECTURA, undefined)).toBe(false);
  expect(esRecorte({ ...SIN_LECTURA, q: '   ' }, undefined)).toBe(false);
  expect(esRecorte({ ...SIN_LECTURA, q: 'ana' }, undefined)).toBe(true);
  expect(esRecorte({ ...SIN_LECTURA, campo: 'f' }, vista(0))).toBe(false);
  expect(esRecorte(SIN_LECTURA, vista(1))).toBe(true);
});
