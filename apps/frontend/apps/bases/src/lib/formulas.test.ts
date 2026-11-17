// Fórmulas en la interfaz: de nombres a ids y vuelta. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { BasesField, BasesTable } from '@sytue/api-clients/bases';
import {
  avisoBorrarCampo,
  efectivo,
  formulaAIds,
  formulaANombres,
  formulaEditada,
  formulasQueUsan,
  referenciasDeFormula,
} from './formulas.js';

const campo = (id: string, name: string, kind: BasesField['kind'], mas: Partial<BasesField> = {}) =>
  ({ id, table_id: 't', name, kind, position: 0, ...mas }) as BasesField;

const lineas: BasesTable = {
  id: 't2',
  base_id: 'b',
  name: 'Líneas',
  position: 1,
  fields: [
    campo('imp', 'Importe', 'number', { table_id: 't2' }),
    campo('nota', 'Nota', 'text', { table_id: 't2' }),
  ],
};
const propios = [
  campo('pre', 'Precio', 'number'),
  campo('ver', 'Versión 1.0', 'text'),
  campo('lin', 'Líneas', 'link', { link_table_id: 't2' }),
];

test('de nombres a ids, sin tocar lo que va entre comillas', () => {
  expect(formulaAIds('{precio} * 2 & "{Precio}"', propios, [lineas])).toBe(
    '{pre} * 2 & "{Precio}"',
  );
  expect(formulaAIds('SUM({Líneas.Importe}) + {Líneas}', propios, [lineas])).toBe(
    'SUM({lin.imp}) + {lin}',
  );
  // Un nombre con punto es primero un campo propio.
  expect(formulaAIds('{Versión 1.0}', propios, [lineas])).toBe('{ver}');
});

test('un nombre que no existe no se envía', () => {
  expect(formulaAIds('{Nada}', propios, [lineas])).toBeNull();
  expect(formulaAIds('{Precio.Importe}', propios, [lineas])).toBeNull();
  expect(formulaAIds('{Líneas.Nada}', propios, [lineas])).toBeNull();
  expect(formulaAIds('{Precio', propios, [lineas])).toBeNull();
});

test('de ids a nombres, y lo desconocido se deja', () => {
  expect(formulaANombres('SUM({lin.imp}) + {pre} + {zzz}', propios, [lineas])).toBe(
    'SUM({Líneas.Importe}) + {Precio} + {zzz}',
  );
});

test('las referencias disponibles: los campos y lo agregable de cada enlace', () => {
  expect(referenciasDeFormula(propios, [lineas])).toEqual([
    '{Precio}',
    '{Versión 1.0}',
    '{Líneas}',
    '{Líneas.Importe}',
  ]);
});

test('una fórmula se comporta como su resultado', () => {
  expect(efectivo(campo('f', 'Total', 'formula', { result: 'text' })).kind).toBe('text');
  expect(efectivo(campo('f', 'Total', 'formula')).kind).toBe('number');
  expect(efectivo(propios[0]!).kind).toBe('number');
});

test('al editar una fórmula se envía con ids, y solo si cambia', () => {
  const total = campo('tot', 'Total', 'formula', { formula: 'SUM({lin.imp}) * {pre}' });
  // Lo que se enseña al abrir es la guardada con nombres, y tal cual no hay nada que enviar.
  const abierta = formulaANombres(total.formula!, propios, [lineas]);
  expect(abierta).toBe('SUM({Líneas.Importe}) * {Precio}');
  expect(formulaEditada(abierta, total, propios, [lineas])).toEqual({
    ids: 'SUM({lin.imp}) * {pre}',
    cambia: false,
  });
  expect(formulaEditada('SUM({líneas.importe}) * {Precio} * 2', total, propios, [lineas])).toEqual({
    ids: 'SUM({lin.imp}) * {pre} * 2',
    cambia: true,
  });
  // Un nombre que no existe o una fórmula vacía no se envían.
  expect(formulaEditada('{Nada} + 1', total, propios, [lineas])).toEqual({
    ids: null,
    cambia: false,
  });
  expect(formulaEditada('   ', total, propios, [lineas])).toEqual({ ids: null, cambia: false });
});

const pedidos: BasesTable = {
  id: 't',
  base_id: 'b',
  name: 'Pedidos',
  position: 0,
  fields: [
    ...propios,
    campo('tot', 'Total', 'formula', { formula: 'SUM({lin.imp}) & "{pre}"' }),
    campo('iva', 'IVA', 'formula', { formula: '{pre} * 0.21' }),
  ],
};
const lineasConFormula: BasesTable = {
  ...lineas,
  fields: [
    ...lineas.fields,
    campo('dbl', 'Doble', 'formula', { table_id: 't2', formula: '{imp} * 2' }),
  ],
};

test('las fórmulas que usan un campo, aquí y a través de un enlace', () => {
  const tablas = [pedidos, lineasConFormula];
  // `{pre}` entre comillas es texto, no una referencia.
  expect(formulasQueUsan(propios[0]!, tablas)).toEqual(['IVA']);
  expect(formulasQueUsan(propios[2]!, tablas)).toEqual(['Total']);
  expect(formulasQueUsan(lineas.fields[0]!, tablas)).toEqual(['Pedidos.Total', 'Doble']);
  expect(formulasQueUsan(propios[1]!, tablas)).toEqual([]);
});

test('el aviso de borrar un campo dice que se lleva sus datos, o qué fórmula lo impide', () => {
  const t = ((k: string, v?: Record<string, unknown>) =>
    v ? `${k}:${JSON.stringify(v)}` : k) as never;
  expect(avisoBorrarCampo(t, propios[1]!, [pedidos])).toEqual({
    mensaje: 'field.deleteConfirm:{"name":"Versión 1.0"}',
    bloqueado: false,
  });
  expect(avisoBorrarCampo(t, propios[0]!, [pedidos, lineasConFormula])).toEqual({
    mensaje: 'field.deleteBlocked:{"name":"Precio","formulas":"IVA"}',
    bloqueado: true,
  });
});
