// Fórmulas en la interfaz: de nombres a ids y vuelta. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { BasesField, BasesTable } from '@sytue/api-clients/bases';
import { efectivo, formulaAIds, formulaANombres, referenciasDeFormula } from './formulas.js';

const campo = (id: string, name: string, kind: BasesField['kind'], mas: Partial<BasesField> = {}) =>
  ({ id, table_id: 't', name, kind, position: 0, ...mas }) as BasesField;

const lineas: BasesTable = {
  id: 't2',
  base_id: 'b',
  name: 'Líneas',
  position: 1,
  fields: [campo('imp', 'Importe', 'number'), campo('nota', 'Nota', 'text')],
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
