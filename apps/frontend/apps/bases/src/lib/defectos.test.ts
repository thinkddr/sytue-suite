// El diálogo del valor por defecto: abre con el que el campo tiene y envía lo que el
// servicio espera (null para quitarlo, 'today' en una fecha). Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { BasesField } from '@sytue/api-clients/bases';
import { DEFECTO_HOY, admiteDefecto, borradorDeDefecto, valorDeDefecto } from './defectos.js';

const campo = (kind: BasesField['kind'], extra: Partial<BasesField> = {}): BasesField => ({
  id: 'c',
  table_id: 't',
  name: 'C',
  kind,
  position: 0,
  ...extra,
});

test('solo admiten defecto los campos que se escriben', () => {
  expect(admiteDefecto(campo('text'))).toBe(true);
  expect(admiteDefecto(campo('date'))).toBe(true);
  for (const k of ['link', 'formula', 'attachment'] as const) {
    expect(admiteDefecto(campo(k))).toBe(false);
  }
});

test('una fecha puede valer «el día del alta» o un día fijo', () => {
  const f = campo('date', { default: DEFECTO_HOY });
  const b = borradorDeDefecto(f);
  expect(b.hoy).toBe(true);
  expect(valorDeDefecto(f, b)).toBe('today');
  expect(valorDeDefecto(f, { ...b, hoy: false, texto: '2026-12-31' })).toBe('2026-12-31');
  expect(valorDeDefecto(f, { ...b, hoy: false, texto: '31/12/2026' })).toBeUndefined();
  expect(valorDeDefecto(f, { ...b, hoy: false, texto: '' })).toBeNull();
});

test('opción, casilla, varias y número: lo de cada tipo, y vacío lo quita', () => {
  const s = campo('select', { choices: ['Nuevo', 'Hecho'], default: 'Nuevo' });
  expect(borradorDeDefecto(s).texto).toBe('Nuevo');
  expect(valorDeDefecto(s, { ...borradorDeDefecto(s), texto: 'Otro' })).toBeUndefined();
  const c = campo('checkbox', { default: true });
  expect(valorDeDefecto(c, borradorDeDefecto(c))).toBe(true);
  expect(valorDeDefecto(c, { ...borradorDeDefecto(c), marcado: false })).toBeNull();
  const m = campo('multiselect', { choices: ['a', 'b'], default: ['b'] });
  expect(valorDeDefecto(m, borradorDeDefecto(m))).toEqual(['b']);
  const n = campo('number');
  expect(valorDeDefecto(n, { ...borradorDeDefecto(n), texto: '2,5' })).toBe(2.5);
});
