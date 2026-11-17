import { describe, expect, it } from 'bun:test';
import type { SheetModel } from '../types';
import {
  accionDeTeclaEnPestana,
  conColorDePestana,
  hojaGemela,
  nombreLibreDeHoja,
  nombresGemelos,
  opcionesDePestana,
} from './pestana';

const libro = (n: number, extra: Partial<SheetModel> = {}): SheetModel => ({
  active: 0,
  sheets: Array.from({ length: n }, (_, i) => ({ name: `H${i + 1}`, cells: {} })),
  ...extra,
});

describe('conColorDePestana', () => {
  it('pone el color en tabColor solo de esa hoja, y null lo quita', () => {
    const con = conColorDePestana(libro(2), 1, '#dc2626');
    expect(con.sheets[1]?.tabColor).toBe('#DC2626');
    expect(con.sheets[0]?.tabColor).toBeUndefined();
    const sin = conColorDePestana(con, 1, null);
    expect('tabColor' in (sin.sheets[1] ?? {})).toBe(false);
  });

  it('devuelve el mismo modelo si no cambia nada o el color no es #RRGGBB', () => {
    const con = conColorDePestana(libro(1), 0, '#2563EB');
    expect(conColorDePestana(con, 0, '#2563eb')).toBe(con);
    const m = libro(1);
    expect(conColorDePestana(m, 0, null)).toBe(m);
    expect(conColorDePestana(m, 0, 'red')).toBe(m);
    expect(conColorDePestana(m, 5, '#2563EB')).toBe(m);
  });
});

describe('hojas gemelas', () => {
  it('son el mismo nombre salvo mayúsculas, como EqualFold de Go en el servidor', () => {
    expect(nombresGemelos('Ventas', 'VENTAS')).toBe(true);
    expect(nombresGemelos('Año 2026', 'AÑO 2026')).toBe(true);
    expect(nombresGemelos('Ventas', 'Ventas ')).toBe(false);
    // Letras que EqualFold da por la misma y las minúsculas no: la web no puede ser
    // más laxa que el servidor, o el guardado fallaría sin aviso.
    expect(nombresGemelos('ſ', 's')).toBe(true);
    expect(nombresGemelos('ϑ', 'θ')).toBe(true);
    expect(nombresGemelos('\u212a', 'k')).toBe(true);
  });

  it('la gemela es otra hoja: la propia no cuenta', () => {
    const hojas = ['Ventas', 'Resumen'];
    expect(hojaGemela(hojas, 'VENTAS')).toBe(0);
    expect(hojaGemela(hojas, 'ventas', 1)).toBe(0);
    expect(hojaGemela(hojas, 'VENTAS', 0)).toBe(-1);
    expect(hojaGemela(hojas, 'Compras')).toBe(-1);
  });

  it('el nombre libre se salta los ocupados con otras mayúsculas', () => {
    const hoja = (n: number): string => `Hoja ${n}`;
    expect(nombreLibreDeHoja([], hoja, 1)).toBe('Hoja 1');
    expect(nombreLibreDeHoja(['Hoja 1', 'Hoja 2'], hoja, 3)).toBe('Hoja 3');
    expect(nombreLibreDeHoja(['Hoja 1', 'hoja 3'], hoja, 3)).toBe('Hoja 4');
    expect(nombreLibreDeHoja(['Hoja 3', 'HOJA 4', 'Hoja 5'], hoja, 3)).toBe('Hoja 6');
    expect(nombreLibreDeHoja(['Datos', 'DATOS (2)'], (n) => `Datos (${n})`, 2)).toBe('Datos (3)');
  });
});

describe('opcionesDePestana', () => {
  it('mover depende del sitio y eliminar de que quede otra hoja', () => {
    expect(opcionesDePestana(libro(3), 0)).toEqual({
      renombrar: true,
      duplicar: true,
      color: true,
      izquierda: false,
      derecha: true,
      eliminar: true,
    });
    expect(opcionesDePestana(libro(3), 2).derecha).toBe(false);
    expect(opcionesDePestana(libro(3), 2).izquierda).toBe(true);
    expect(opcionesDePestana(libro(1), 0).eliminar).toBe(false);
  });

  it('con el libro protegido todo sale inerte, también el color', () => {
    const o = opcionesDePestana(libro(3, { protect: {} }), 1);
    expect(Object.values(o).some(Boolean)).toBe(false);
  });

  it('en solo lectura también sale todo inerte', () => {
    const o = opcionesDePestana(libro(3), 1, true);
    expect(Object.values(o).some(Boolean)).toBe(false);
  });
});

describe('accionDeTeclaEnPestana', () => {
  const k = (
    key: string,
    mods: Partial<Record<'shiftKey' | 'ctrlKey' | 'altKey' | 'metaKey', boolean>> = {},
  ) =>
    accionDeTeclaEnPestana({
      key,
      shiftKey: false,
      ctrlKey: false,
      altKey: false,
      metaKey: false,
      ...mods,
    });

  it('F2 renombra; Mayús+F10 y la tecla de menú abren el menú', () => {
    expect(k('F2')).toBe('renombrar');
    expect(k('F10', { shiftKey: true })).toBe('menu');
    expect(k('ContextMenu')).toBe('menu');
  });

  it('el resto, y las combinaciones con Ctrl/Alt/Meta, no son suyas', () => {
    expect(k('F10')).toBeNull();
    expect(k('F2', { ctrlKey: true })).toBeNull();
    expect(k('F2', { shiftKey: true })).toBeNull();
    expect(k('Enter')).toBeNull();
  });
});
