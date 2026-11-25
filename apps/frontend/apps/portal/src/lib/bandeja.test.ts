/**
 * La bandeja: qué clic se deja al navegador, qué aviso se puede silenciar desde él y
 * el corte por día (el mismo que la actividad). Las huellas de Descargas, aparte, en
 * `descargas.test.ts`.
 */
import { describe, expect, test } from 'bun:test';
import { clicAparte, silenciableDesdeElAviso } from './bandeja.js';
import { agruparPorDia, dayLabelKey } from './activity.js';

const clic = { ctrlKey: false, metaKey: false, shiftKey: false, button: 0 };

describe('clicAparte', () => {
  test('el clic normal navega aquí; Ctrl, ⌘, Mayús y la rueda abren aparte', () => {
    expect(clicAparte(clic)).toBe(false);
    expect(clicAparte({ ...clic, ctrlKey: true })).toBe(true);
    expect(clicAparte({ ...clic, metaKey: true })).toBe(true);
    expect(clicAparte({ ...clic, shiftKey: true })).toBe(true);
    expect(clicAparte({ ...clic, button: 1 })).toBe(true);
  });
});

describe('silenciableDesdeElAviso', () => {
  test('un aviso corriente se puede silenciar por su tipo', () => {
    expect(silenciableDesdeElAviso({ source: 'drive', type: 'drive.shared' })).toBe(true);
  });
  test('los de Sytue ID y los que piden actuar, no', () => {
    expect(silenciableDesdeElAviso({ source: 'id', type: 'security.signin.new' })).toBe(false);
    expect(silenciableDesdeElAviso({ source: 'drive', type: '' })).toBe(false);
  });
});

describe('agruparPorDia', () => {
  test('corta por día local y deja al final los que no traen fecha', () => {
    const hoy = new Date(2026, 10, 25, 12, 0);
    const ayer = new Date(2026, 10, 24, 9, 0);
    const avisos = [
      { id: 'a', created_at: hoy.toISOString() },
      { id: 'x', created_at: 'no es una fecha' },
      { id: 'b', created_at: new Date(2026, 10, 25, 8, 0).toISOString() },
      { id: 'c', created_at: ayer.toISOString() },
    ];
    const grupos = agruparPorDia(avisos, (n) => n.created_at);
    expect(grupos.map((g) => g.items.map((n) => n.id))).toEqual([['a', 'b'], ['c'], ['x']]);
    expect(grupos.map((g) => dayLabelKey(g.day, hoy))).toEqual(['today', 'yesterday', 'date']);
  });
});
