// El tablero reparte sin perder registros y el borrador de una vista solo se envía si
// está completo. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { BasesAttachment, BasesField, BasesRecord, BasesView } from '@sytue/api-clients/bases';
import {
  SIN_VALOR,
  borradorDeVista,
  camposDeFormulario,
  columnasDeTablero,
  configDeBorrador,
  datosDeColumna,
  portadaDe,
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

test('una tarjeta añadida al pie de una columna nace con el valor de esa columna', () => {
  expect(datosDeColumna(sel, 'hecho')).toEqual({ s: 'hecho' });
  // La de «sin valor» nace vacía: no se escribe una cadena vacía en un campo de selección.
  expect(datosDeColumna(sel, SIN_VALOR)).toEqual({});
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

const vista = (kind: BasesView['kind'], config: BasesView['config']): BasesView => ({
  id: 'v',
  table_id: 't',
  name: 'Mía',
  kind,
  config,
  created_by: 'ana',
});

test('editar una vista: el borrador de lo guardado vuelve a dar lo guardado', () => {
  const casilla: BasesField = {
    id: 'c',
    table_id: 't',
    name: 'Hecho',
    kind: 'checkbox',
    position: 2,
  };
  for (const cfg of [
    {
      filters: [{ field: 'n', op: 'gte', value: 10.5 }],
      sort: [{ field: 'n', dir: 'desc' as const }],
    },
    { filters: [{ field: 'c', op: 'eq', value: false }] },
    { filters: [{ field: 's', op: 'empty' }] },
    {},
  ]) {
    const b = borradorDeVista(vista('grid', cfg), [sel, num, casilla]);
    expect(b.nombre).toBe('Mía');
    expect(configDeBorrador(b, [sel, num, casilla])).toEqual(cfg);
  }
  const k = borradorDeVista(vista('kanban', { group_by: 's' }), [num, sel]);
  expect(configDeBorrador(k, [num, sel])).toEqual({ group_by: 's' });
});

test('editar una vista no pierde los filtros y órdenes que el diálogo no enseña', () => {
  const cfg: BasesView['config'] = {
    filters: [
      { field: 'n', op: 'gt', value: 1 },
      { field: 's', op: 'eq', value: 'hecho' },
      { field: 'borrado', op: 'empty' },
    ],
    sort: [
      { field: 's', dir: 'asc' },
      { field: 'n', dir: 'desc' },
    ],
  };
  const b = borradorDeVista(vista('grid', cfg), [sel, num]);
  expect(b.filtroCampo).toBe('n');
  expect(b.ordenCampo).toBe('s');
  // Se cambia el primer filtro; lo demás sigue, salvo lo del campo que ya no existe.
  expect(configDeBorrador({ ...b, filtroValor: '5' }, [sel, num])).toEqual({
    filters: [
      { field: 'n', op: 'gt', value: 5 },
      { field: 's', op: 'eq', value: 'hecho' },
    ],
    sort: [
      { field: 's', dir: 'asc' },
      { field: 'n', dir: 'desc' },
    ],
  });
});

test('un formulario guardado sin lista de campos se abre con todos los que admite marcados', () => {
  const b = borradorDeVista(vista('form', {}), [sel, num]);
  expect(b.campos).toEqual(['s', 'n']);
  expect(configDeBorrador(b, [sel, num])).toEqual({ fields: ['s', 'n'] });
  expect(borradorDeVista(vista('form', { fields: ['n'] }), [sel, num]).campos).toEqual(['n']);
});

const adjunto = (id: string, mime: string, has_thumb: boolean): BasesAttachment => ({
  id,
  record_id: 'r',
  field_id: '',
  name: id,
  mime,
  size: 1,
  has_thumb,
  created_by: '',
  created_at: '',
});

test('la portada de la galería es la primera imagen con miniatura, por orden de campos', () => {
  const docs: BasesField = {
    id: 'd',
    table_id: 't',
    name: 'Docs',
    kind: 'attachment',
    position: 2,
  };
  const fotos: BasesField = {
    id: 'f',
    table_id: 't',
    name: 'Fotos',
    kind: 'attachment',
    position: 3,
  };
  const r = {
    ...reg('r', {}),
    attachments: {
      // Un PDF (aunque tuviera miniatura) y una imagen sin miniatura no son portada.
      d: [adjunto('pdf', 'application/pdf', true), adjunto('grande', 'image/png', false)],
      f: [adjunto('a', 'image/jpeg', true), adjunto('b', 'image/png', true)],
    },
  };
  expect(portadaDe([num, docs, fotos], r)?.id).toBe('a');
  // El orden es el de los campos, no el del mapa de adjuntos.
  const r2 = { ...r, attachments: { f: r.attachments.f, d: [adjunto('c', 'image/gif', true)] } };
  expect(portadaDe([num, docs, fotos], r2)?.id).toBe('c');
  expect(portadaDe([num, docs, fotos], reg('vacío', {}))).toBeNull();
});

test('una galería se guarda como la rejilla: filtro y orden, sin agrupar', () => {
  const b: BorradorVista = {
    nombre: 'Fotos',
    tipo: 'gallery',
    agrupar: 's',
    campos: [],
    filtroCampo: 'n',
    filtroOp: 'gt',
    filtroValor: '3',
    ordenCampo: 'n',
    ordenDir: 'desc',
  };
  expect(configDeBorrador(b, [sel, num])).toEqual({
    filters: [{ field: 'n', op: 'gt', value: 3 }],
    sort: [{ field: 'n', dir: 'desc' }],
  });
});

const fecha: BasesField = { id: 'f', table_id: 't', name: 'Alta', kind: 'date', position: 2 };

test('un formulario guarda como obligatorios solo los que enseña, en su orden', () => {
  expect(
    configDeBorrador({ ...base, tipo: 'form', campos: ['n', 's'], obligatorios: ['s', 'x'] }, [
      sel,
      num,
    ]),
  ).toEqual({ fields: ['n', 's'], required: ['s'] });
  // Desmarcar un campo del formulario se lleva su «obligatorio».
  expect(
    configDeBorrador({ ...base, tipo: 'form', campos: ['n'], obligatorios: ['s'] }, [sel, num]),
  ).toEqual({ fields: ['n'] });
  const guardada = vista('form', { fields: ['s', 'n'], required: ['n'] });
  expect(configDeBorrador(borradorDeVista(guardada, [sel, num]), [sel, num])).toEqual(
    guardada.config,
  );
});

test('los filtros de fecha relativos: hoy y este mes sin valor, N días entre 1 y 3650', () => {
  const con = (filtroOp: string, filtroValor = '') =>
    configDeBorrador({ ...base, filtroCampo: 'f', filtroOp, filtroValor }, [fecha]);
  expect(con('today')).toEqual({ filters: [{ field: 'f', op: 'today' }] });
  expect(con('this_month', 'basura')).toEqual({ filters: [{ field: 'f', op: 'this_month' }] });
  expect(con('past_days', '7')).toEqual({ filters: [{ field: 'f', op: 'past_days', value: 7 }] });
  expect(con('next_days', '30')).toEqual({
    filters: [{ field: 'f', op: 'next_days', value: 30 }],
  });
  for (const malo of ['', '0', '1.5', 'siete', '3651']) expect(con('past_days', malo)).toBeNull();
  // Un número no tiene «hoy».
  expect(configDeBorrador({ ...base, filtroCampo: 'n', filtroOp: 'today' }, [num])).toBeNull();
});
