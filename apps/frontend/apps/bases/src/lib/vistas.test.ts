// El tablero reparte sin perder registros y el borrador de una vista solo se envía si
// está completo. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { BasesField, BasesRecord } from '@sytue/api-clients/bases';
import {
  SIN_VALOR,
  camposDeFormulario,
  columnasDeTablero,
  configDeBorrador,
  type BorradorVista,
} from './vistas.js';

const sel: BasesField = {
  id: 's',
  table_id: 't',
  name: 'Estado',
  kind: 'select',
  choices: ['nuevo', 'hecho'],
  position: 0,
};
const num: BasesField = { id: 'n', table_id: 't', name: 'Importe', kind: 'number', position: 1 };
const reg = (id: string, data: BasesRecord['data']): BasesRecord => ({
  id,
  table_id: 't',
  data,
  links: {},
  rev: 1,
  created_by: '',
  updated_by: '',
  created_at: '',
  updated_at: '',
});

test('el tablero tiene una columna por opción y una de «sin valor» al final', () => {
  const cols = columnasDeTablero(sel, [
    reg('1', { s: 'hecho' }),
    reg('2', {}),
    reg('3', { s: 'nuevo' }),
  ]);
  expect(cols.map((c) => c.valor)).toEqual(['nuevo', 'hecho', SIN_VALOR]);
  expect(cols.map((c) => c.registros.map((r) => r.id))).toEqual([['3'], ['1'], ['2']]);
});

test('un valor que ya no es una opción cae en «sin valor»: ningún registro se pierde', () => {
  const regs = [reg('1', { s: 'opción borrada' }), reg('2', { s: 5 as unknown as string })];
  const cols = columnasDeTablero(sel, regs);
  expect(cols.reduce((n, c) => n + c.registros.length, 0)).toBe(2);
  expect(cols.at(-1)!.registros).toHaveLength(2);
});

test('el formulario público no admite enlaces', () => {
  const link: BasesField = {
    id: 'l',
    table_id: 't',
    name: 'L',
    kind: 'link',
    link_table_id: 'x',
    position: 2,
  };
  expect(camposDeFormulario([sel, num, link]).map((c) => c.id)).toEqual(['s', 'n']);
});

const base: BorradorVista = {
  nombre: 'V',
  tipo: 'grid',
  agrupar: '',
  campos: [],
  filtroCampo: '',
  filtroOp: 'eq',
  filtroValor: '',
  ordenCampo: '',
  ordenDir: 'asc',
};

test('configDeBorrador: kanban pide campo, formulario pide campos, filtro pide valor del tipo', () => {
  expect(configDeBorrador({ ...base, tipo: 'kanban' }, [sel])).toBeNull();
  expect(configDeBorrador({ ...base, tipo: 'kanban', agrupar: 's' }, [sel])).toEqual({
    group_by: 's',
  });
  expect(configDeBorrador({ ...base, tipo: 'form' }, [sel])).toBeNull();
  expect(configDeBorrador({ ...base, tipo: 'form', campos: ['s', 'n'] }, [sel, num])).toEqual({
    fields: ['s', 'n'],
  });
  expect(
    configDeBorrador({ ...base, filtroCampo: 'n', filtroOp: 'gt', filtroValor: 'diez' }, [num]),
  ).toBeNull();
  expect(
    configDeBorrador({ ...base, filtroCampo: 'n', filtroOp: 'gt', filtroValor: '10,5' }, [num]),
  ).toEqual({
    filters: [{ field: 'n', op: 'gt', value: 10.5 }],
  });
  expect(configDeBorrador({ ...base, filtroCampo: 'n', filtroOp: 'empty' }, [num])).toEqual({
    filters: [{ field: 'n', op: 'empty' }],
  });
  // Un operador que el tipo no admite nunca sale.
  expect(
    configDeBorrador({ ...base, filtroCampo: 'n', filtroOp: 'contains', filtroValor: 'x' }, [num]),
  ).toBeNull();
  expect(configDeBorrador({ ...base, ordenCampo: 'n', ordenDir: 'desc' }, [num])).toEqual({
    sort: [{ field: 'n', dir: 'desc' }],
  });
  expect(configDeBorrador(base, [num])).toEqual({});
});
