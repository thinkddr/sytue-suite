/**
 * El árbol decide qué se VE y dónde. Un error aquí no es una excepción: es una
 * nota que desaparece del panel, o un navegador colgado en un bucle.
 */
import { expect, test } from 'bun:test';
import type { Note } from '@sytue/api-clients';
import {
  colocarNota,
  conSangria,
  construirArbol,
  descendientes,
  reponerFamilia,
} from '../client/arbol.js';

function nota(id: string, extra: Partial<Note> = {}): Note {
  return {
    id,
    notebook_id: 'nb',
    owner_sub: 'yo',
    kind: 'text',
    x: 0,
    y: 0,
    col: 0,
    position: 0,
    width: 280,
    collapsed: false,
    checked: false,
    pinned: false,
    created_at: '2026-09-08T00:00:00Z',
    updated_at: '2026-09-08T00:00:00Z',
    ...extra,
  } as Note;
}

test('reponer una familia tras un 409 conserva capturas y ediciones posteriores', () => {
  const anteriores = [
    nota('padre', { content: 'antes' }),
    nota('hija', { group_id: 'padre' }),
    nota('otra'),
  ];
  const actuales = [
    nota('padre', { content: 'editado después' }),
    nota('otra', { content: 'otra edición' }),
    nota('captura', { content: 'recién guardada' }),
  ];
  const finales = reponerFamilia(actuales, anteriores, new Set(['padre', 'hija']));
  expect(finales.map((n) => n.id)).toEqual(['padre', 'otra', 'captura', 'hija']);
  expect(finales[0]!.content).toBe('editado después');
  expect(finales[0]).toBe(actuales[0]);
  expect(finales[1]!.content).toBe('otra edición');
  expect(finales[2]!.content).toBe('recién guardada');
});

test('reordenar conserva padres, fijadas y notas ocultas en el plan de posiciones', () => {
  const notas = [
    nota('a'),
    nota('oculta', { position: 1 }),
    nota('b', { position: 2 }),
    nota('hija', { group_id: 'a', position: 0 }),
    nota('fijada', { pinned: true, position: 0 }),
  ];
  const cambios = colocarNota(notas, 'b', 0, 'a', true);
  const porId = new Map(cambios.map((n) => [n.id, n]));
  const finales = notas.map((n) => porId.get(n.id) ?? n);
  expect(
    finales
      .filter((n) => !n.group_id && !n.pinned)
      .sort((a, b) => a.position - b.position)
      .map((n) => n.id),
  ).toEqual(['b', 'a', 'oculta']);
  expect(porId.has('hija')).toBe(false);
  expect(porId.has('fijada')).toBe(false);
  expect(colocarNota(notas, 'b', 0, 'hija', true)).toEqual([]);
  expect(colocarNota(notas, 'b', 0, 'fijada', true)).toEqual([]);
});

test('anida por group_id y ordena hermanos por posición', () => {
  const arbol = construirArbol([
    nota('b', { position: 2 }),
    nota('a', { position: 1 }),
    nota('a1', { group_id: 'a', position: 1 }),
    nota('a0', { group_id: 'a', position: 0 }),
  ]);
  expect(arbol.map((n) => n.note.id)).toEqual(['a', 'b']);
  expect(arbol[0]!.hijos.map((n) => n.note.id)).toEqual(['a0', 'a1']);
  expect(descendientes(arbol[0]!)).toEqual(['a', 'a0', 'a1']);
});

test('una nota cuyo padre no está a la vista sube a la raíz', () => {
  // El padre puede estar filtrado o en la papelera: la hija no desaparece.
  const arbol = construirArbol([nota('huerfana', { group_id: 'no-existe' })]);
  expect(arbol.map((n) => n.note.id)).toEqual(['huerfana']);
});

test('un ciclo en los datos no cuelga y deja las notas a la vista', () => {
  const arbol = construirArbol([
    nota('a', { group_id: 'b' }),
    nota('b', { group_id: 'a' }),
    nota('yo-misma', { group_id: 'yo-misma' }),
  ]);
  expect(arbol.map((n) => n.note.id).sort()).toEqual(['a', 'b', 'yo-misma']);
  expect(arbol.every((n) => n.hijos.length === 0)).toBe(true);
});

test('a igual posición manda la antigüedad', () => {
  const arbol = construirArbol([
    nota('nueva', { created_at: '2026-09-08T10:00:00Z' }),
    nota('vieja', { created_at: '2026-09-01T10:00:00Z' }),
  ]);
  expect(arbol.map((n) => n.note.id)).toEqual(['vieja', 'nueva']);
});

test('la lista con sangría respeta el orden que llega y cuelga cada sublista de su nota', () => {
  const filas = conSangria([
    nota('hijo-b', { group_id: 'b' }),
    nota('b'),
    nota('a'),
    nota('nieto', { group_id: 'hijo-b' }),
    nota('huerfana', { group_id: 'no-esta' }),
  ]);
  expect(filas.map((f) => `${f.note.id}:${f.nivel}`)).toEqual([
    'b:0',
    'hijo-b:1',
    'nieto:2',
    'a:0',
    'huerfana:0',
  ]);
});

test('la lista con sangría no pierde ni repite notas con un ciclo en los datos', () => {
  const filas = conSangria([nota('a', { group_id: 'b' }), nota('b', { group_id: 'a' })]);
  expect(filas.map((f) => f.note.id).sort()).toEqual(['a', 'b']);
});
