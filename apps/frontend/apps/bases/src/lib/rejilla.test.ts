// Navegación por teclado de la rejilla y conversión de valores. Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import type { BasesField, BasesRecord } from '@sytue/api-clients/bases';
import { moverFoco } from './rejilla.js';
import {
  etiquetaDeRegistro,
  puedeEscribir,
  puedeEsquema,
  textoDeValor,
  valorDeTexto,
} from './valores.js';

test('las flechas se detienen en los bordes y no salen de la rejilla', () => {
  expect(moverFoco({ fila: 0, col: 0 }, 'ArrowUp', false, 3, 4)).toEqual({ fila: 0, col: 0 });
  expect(moverFoco({ fila: 0, col: 0 }, 'ArrowLeft', false, 3, 4)).toEqual({ fila: 0, col: 0 });
  expect(moverFoco({ fila: 2, col: 3 }, 'ArrowDown', false, 3, 4)).toEqual({ fila: 2, col: 3 });
  expect(moverFoco({ fila: 2, col: 3 }, 'ArrowRight', false, 3, 4)).toEqual({ fila: 2, col: 3 });
  expect(moverFoco({ fila: 1, col: 1 }, 'ArrowDown', false, 3, 4)).toEqual({ fila: 2, col: 1 });
});

test('Inicio/Fin y Ctrl+Inicio/Fin, RePág y AvPág', () => {
  expect(moverFoco({ fila: 5, col: 2 }, 'Home', false, 20, 4)).toEqual({ fila: 5, col: 0 });
  expect(moverFoco({ fila: 5, col: 2 }, 'End', false, 20, 4)).toEqual({ fila: 5, col: 3 });
  expect(moverFoco({ fila: 5, col: 2 }, 'Home', true, 20, 4)).toEqual({ fila: 0, col: 0 });
  expect(moverFoco({ fila: 5, col: 2 }, 'End', true, 20, 4)).toEqual({ fila: 19, col: 3 });
  expect(moverFoco({ fila: 5, col: 2 }, 'PageDown', false, 20, 4)).toEqual({ fila: 15, col: 2 });
  expect(moverFoco({ fila: 5, col: 2 }, 'PageUp', false, 20, 4)).toEqual({ fila: 0, col: 2 });
});

test('una tecla que no mueve, o una rejilla vacía, no devuelven destino', () => {
  expect(moverFoco({ fila: 0, col: 0 }, 'a', false, 3, 3)).toBeNull();
  expect(moverFoco({ fila: 0, col: 0 }, 'ArrowDown', false, 0, 3)).toBeNull();
  // Una posición vieja fuera de rango (se borró una fila) se recoloca.
  expect(moverFoco({ fila: 9, col: 9 }, 'ArrowUp', false, 3, 3)).toEqual({ fila: 1, col: 2 });
});

const campo = (kind: BasesField['kind'], extra: Partial<BasesField> = {}): BasesField => ({
  id: 'f',
  table_id: 't',
  name: 'N',
  kind,
  position: 0,
  ...extra,
});

test('valorDeTexto: número con coma, fecha ISO, selección de la lista y vacío', () => {
  expect(valorDeTexto(campo('number'), '12,5')).toBe(12.5);
  expect(valorDeTexto(campo('number'), 'doce')).toBeUndefined();
  expect(valorDeTexto(campo('number'), '1e99')).toBeUndefined();
  expect(valorDeTexto(campo('date'), '2026-10-04')).toBe('2026-10-04');
  expect(valorDeTexto(campo('date'), '4/10/2026')).toBeUndefined();
  expect(valorDeTexto(campo('select', { choices: ['a', 'b'] }), 'b')).toBe('b');
  expect(valorDeTexto(campo('select', { choices: ['a', 'b'] }), 'c')).toBeUndefined();
  expect(valorDeTexto(campo('text'), '  hola ')).toBe('  hola ');
  expect(valorDeTexto(campo('text'), '   ')).toBeNull();
});

test('textoDeValor y etiquetaDeRegistro', () => {
  expect(textoDeValor(campo('checkbox'), true)).toBe('✓');
  expect(textoDeValor(campo('multiselect'), ['a', 'b'])).toBe('a, b');
  expect(textoDeValor(campo('text'), undefined)).toBe('');
  const r = { id: '12345678-aaaa', data: { n: 'Ana' }, links: {} } as unknown as BasesRecord;
  expect(etiquetaDeRegistro([campo('link', { id: 'l' }), campo('text', { id: 'n' })], r)).toBe(
    'Ana',
  );
  expect(etiquetaDeRegistro([campo('text', { id: 'x' })], r)).toBe('12345678');
});

test('los roles solo pintan la interfaz: editar es editor o propietario; esquema, propietario', () => {
  expect(puedeEscribir('editor')).toBe(true);
  expect(puedeEscribir('viewer')).toBe(false);
  expect(puedeEscribir('')).toBe(false);
  expect(puedeEsquema('owner')).toBe(true);
  expect(puedeEsquema('editor')).toBe(false);
});
