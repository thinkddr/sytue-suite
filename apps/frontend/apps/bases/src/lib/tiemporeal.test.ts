// Ante cada cambio del flujo se pide de nuevo lo justo, y la presencia nunca cuenta la
// propia pestaña. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { BasesPresence } from '@sytue/api-clients/bases';
import { personasPresentes, planDe, presentesPorCelda } from './tiemporeal.js';

const ctx = { tabla: 't1', propias: new Set(['op-mia']) };

test('un registro de la tabla que se mira se vuelve a pedir; borrado, se quita', () => {
  expect(planDe({ action: 'record.update', table_id: 't1', record_id: 'r1' }, ctx)).toEqual({
    tipo: 'registro',
    id: 'r1',
    nuevo: false,
  });
  expect(planDe({ action: 'record.link', table_id: 't1', record_id: 'r1' }, ctx).tipo).toBe(
    'registro',
  );
  expect(planDe({ action: 'record.create', table_id: 't1', record_id: 'r2' }, ctx)).toEqual({
    tipo: 'registro',
    id: 'r2',
    nuevo: true,
  });
  expect(planDe({ action: 'record.delete', table_id: 't1', record_id: 'r1' }, ctx)).toEqual({
    tipo: 'quitar',
    id: 'r1',
  });
});

test('el eco de una operación propia no se vuelve a pedir', () => {
  expect(
    planDe({ action: 'record.update', table_id: 't1', record_id: 'r1', op_id: 'op-mia' }, ctx).tipo,
  ).toBe('nada');
  expect(
    planDe({ action: 'record.update', table_id: 't1', record_id: 'r1', op_id: 'op-ajena' }, ctx)
      .tipo,
  ).toBe('registro');
});

test('lo de otra tabla no toca lo que se ve; el esquema y el acceso recargan la estructura', () => {
  expect(planDe({ action: 'record.update', table_id: 't2', record_id: 'r9' }, ctx).tipo).toBe(
    'nada',
  );
  expect(planDe({ action: 'comment.create', table_id: 't2', record_id: 'r9' }, ctx).tipo).toBe(
    'nada',
  );
  expect(planDe({ action: 'view.create', table_id: 't2' }, ctx).tipo).toBe('nada');
  expect(planDe({ action: 'view.create', table_id: 't1' }, ctx).tipo).toBe('estructura');
  for (const action of [
    'access',
    'base.update',
    'field.create',
    'field.delete',
    'table.rename',
    'table.delete',
  ])
    expect(planDe({ action, table_id: 't2' }, ctx).tipo).toBe('estructura');
  expect(planDe({ action: 'comment.delete', table_id: 't1', record_id: 'r1' }, ctx)).toEqual({
    tipo: 'comentarios',
    registro: 'r1',
  });
  expect(planDe({ action: 'algo.desconocido', table_id: 't1', record_id: 'r1' }, ctx).tipo).toBe(
    'nada',
  );
});

const p = (conn: string, sub: string, extra: Partial<BasesPresence> = {}): BasesPresence => ({
  conn,
  sub,
  name: sub.toUpperCase(),
  ...extra,
});

test('la presencia por celda agrupa nombres y deja fuera la propia pestaña y las otras tablas', () => {
  const lista = [
    p('yo', 'ana', { table_id: 't1', record_id: 'r1', field_id: 'f1' }),
    p('c2', 'bea', { table_id: 't1', record_id: 'r1', field_id: 'f1' }),
    p('c3', 'bea', { table_id: 't1', record_id: 'r1', field_id: 'f1' }),
    p('c4', 'cleo', { table_id: 't1', record_id: 'r1', field_id: 'f1' }),
    p('c5', 'dani', { table_id: 't2', record_id: 'r1', field_id: 'f1' }),
    p('c6', 'eva', { table_id: 't1' }),
  ];
  const m = presentesPorCelda(lista, 't1', 'yo');
  expect([...m.keys()]).toEqual(['r1:f1']);
  expect(m.get('r1:f1')).toEqual(['BEA', 'CLEO']);
  expect(personasPresentes(lista, 'yo')).toEqual(['BEA', 'CLEO', 'DANI', 'EVA']);
});
