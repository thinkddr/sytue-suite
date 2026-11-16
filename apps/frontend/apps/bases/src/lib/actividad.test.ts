// La actividad de una base se cuenta en frases con nombre de persona (nunca su `sub`) y
// la búsqueda de «Mis bases» pliega mayúsculas y acentos. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { BasesAuditEntry, BasesBase, BasesPermission } from '@sytue/api-clients/bases';
import { useT as traductor } from '../i18n/index.js';
import {
  contarEntrada,
  filtrarBases,
  nombreDePersona,
  type ContextoActividad,
} from './actividad.js';

const t = traductor('es');

const permiso = (principal_id: string, principal_label: string): BasesPermission => ({
  id: principal_id,
  principal_kind: 'user',
  principal_id,
  principal_label,
  role: 'editor',
  created_by: 'yo',
  created_at: '',
});

const ctx: ContextoActividad = {
  yo: 'yo',
  tablas: [{ id: 't1', base_id: 'b', name: 'Pedidos', position: 0, fields: [] }],
  permisos: [permiso('ana', 'Ana Ruiz')],
};

const e = (action: string, extra: Partial<BasesAuditEntry> = {}): BasesAuditEntry => ({
  id: 1,
  actor: 'ana',
  action,
  at: '2026-11-12T10:00:00Z',
  ...extra,
});

test('quien mira es «Tú», una persona con permiso por su nombre y el resto sin su id', () => {
  expect(nombreDePersona('yo', ctx, t)).toBe('Tú');
  expect(nombreDePersona('ana', ctx, t)).toBe('Ana Ruiz');
  expect(nombreDePersona('0b6c-desconocido', ctx, t)).toBe('Otra persona');
});

test('un campo renombrado dice el nombre nuevo y su tabla', () => {
  const c = contarEntrada(
    e('field.rename', { table_id: 't1', field_id: 'f', after: { name: 'Importe' } }),
    ctx,
    t,
  );
  expect(c).toMatchObject({
    quien: 'Ana Ruiz',
    que: 'Renombró un campo',
    sobre: 'Importe · Pedidos',
  });
});

test('las acciones de tabla no repiten el nombre de la tabla', () => {
  const c = contarEntrada(
    e('table.rename', { table_id: 't1', after: { name: 'Pedidos' } }),
    ctx,
    t,
  );
  expect(c.sobre).toBe('Pedidos');
  // Una tabla ya borrada no está en la lista: queda sin «sobre», no con su id.
  expect(contarEntrada(e('table.delete', { table_id: 'tx' }), ctx, t).sobre).toBe('');
});

test('varias acciones técnicas comparten frase y un registro dice su tabla', () => {
  const formula = contarEntrada(e('field.formula', { table_id: 't1', after: 'A+B' }), ctx, t);
  const opciones = contarEntrada(e('field.options', { table_id: 't1', after: ['a'] }), ctx, t);
  expect(formula.que).toBe('Cambió un campo');
  expect(opciones.que).toBe(formula.que);
  const r = contarEntrada(e('record.update', { table_id: 't1', after: 3 }), ctx, t);
  expect(r).toMatchObject({ que: 'Editó un registro', sobre: 'Pedidos' });
});

test('dar acceso dice a quién, con las palabras de compartir', () => {
  const a = contarEntrada(
    e('permission.grant', {
      actor: 'yo',
      after: { kind: 'user', principal: 'ana', role: 'editor' },
    }),
    ctx,
    t,
  );
  expect(a).toMatchObject({ quien: 'Tú', que: 'Dio acceso', sobre: 'Ana Ruiz' });
  const g = contarEntrada(
    e('permission.revoke', { before: { kind: 'group', principal: 'ventas', role: 'viewer' } }),
    ctx,
    t,
  );
  expect(g.sobre).toBe(t('share.groupLabel', { name: 'ventas' }));
});

test('una acción que la interfaz no conoce se cuenta igual, sin romper', () => {
  expect(contarEntrada(e('base.archive'), ctx, t).que).toBe('Otro cambio (base.archive)');
});

const base = (id: string, name: string, description = ''): BasesBase => ({
  id,
  name,
  description,
  owner_sub: 'yo',
  role: 'owner',
  created_at: '',
  updated_at: '',
});

test('buscar en «Mis bases» pliega acentos y mira también la descripción', () => {
  const bases = [
    base('1', 'Café de la esquina'),
    base('2', 'Clientes', 'Contactos del CAFÉ y proveedores'),
    base('3', 'Almacén'),
  ];
  expect(filtrarBases(bases, 'cafe').map((b) => b.id)).toEqual(['1', '2']);
  expect(filtrarBases(bases, '  almacen ').map((b) => b.id)).toEqual(['3']);
  expect(filtrarBases(bases, '').length).toBe(3);
  expect(filtrarBases(bases, 'nada')).toEqual([]);
});
