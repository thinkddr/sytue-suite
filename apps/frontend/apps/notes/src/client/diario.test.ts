// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { expect, test } from 'bun:test';
import type { Note, Notebook } from '@sytue/api-clients';
import {
  atajoDiario,
  capturaRapida,
  cuadernosDelArbol,
  diaLocal,
  desplazarDia,
  enfocarNotas,
  expandirAtajos,
  expandirCaptura,
  resumenNota,
  semanaDe,
  terminada,
  vecesTrasladada,
} from './diario.js';
import { eventoDeDiaCompleto, eventoDelDia } from './agenda.js';

test('captura tareas solo por elección o marcador explícito y escapa HTML', () => {
  expect(capturaRapida(' - [X] Comprar pan ', 'text')).toEqual({
    kind: 'todo',
    title: 'Comprar pan',
    checked: true,
  });
  expect(capturaRapida('[ ] Revisar', 'text')).toEqual({
    kind: 'todo',
    title: 'Revisar',
    checked: false,
  });
  expect(capturaRapida('Llamar', 'todo')).toEqual({
    kind: 'todo',
    title: 'Llamar',
    checked: false,
  });
  expect(capturaRapida('<img src=x onerror=alert(1)>', 'text')?.content).toBe(
    '<p>&lt;img src=x onerror=alert(1)&gt;</p>',
  );
  expect(capturaRapida('https://sytue.eu', 'text')?.kind).toBe('link');
  expect(capturaRapida('  ', 'text')).toBeNull();
});

test('navega días civiles en los límites de mes, año, bisiesto y cambio horario', () => {
  expect(diaLocal(new Date(2026, 9, 1, 0, 5))).toBe('2026-10-01');
  expect(desplazarDia('2026-12-31', 1)).toBe('2027-01-01');
  expect(desplazarDia('2024-03-01', -1)).toBe('2024-02-29');
  expect(desplazarDia('2026-03-29', 1)).toBe('2026-03-30');
});

test('filtra sin perder los antepasados, respeta orden y termina ante ciclos', () => {
  const notes = [
    { id: 'group', kind: 'text' },
    { id: 'todo', kind: 'todo', group_id: 'group', checked: false },
    { id: 'done', kind: 'todo', checked: true },
    { id: 'pin', kind: 'text', pinned: true, group_id: 'loop' },
    { id: 'loop', kind: 'text', group_id: 'pin' },
  ].map((n) => ({
    notebook_id: 'nb',
    owner_sub: 'ada',
    x: 0,
    y: 0,
    col: 0,
    position: 0,
    width: 240,
    collapsed: false,
    checked: false,
    pinned: false,
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-01T00:00:00Z',
    ...n,
  })) as Note[];
  expect(enfocarNotas(notes, 'pending').map((n) => n.id)).toEqual(['group', 'todo']);
  expect(enfocarNotas(notes, 'done').map((n) => n.id)).toEqual(['done']);
  expect(enfocarNotas(notes, 'pinned').map((n) => n.id)).toEqual(['pin', 'loop']);
  expect(enfocarNotas(notes, 'all')).toEqual(notes);
});

test('la sintaxis de Twos: guion y casilla vacía son tareas, --- un separador', () => {
  expect(capturaRapida('- Comprar pan', 'text')).toEqual({
    kind: 'todo',
    title: 'Comprar pan',
    checked: false,
  });
  expect(capturaRapida('[] Revisar', 'text')).toEqual({
    kind: 'todo',
    title: 'Revisar',
    checked: false,
  });
  expect(capturaRapida('---', 'text')).toEqual({ kind: 'text', content: '<hr>' });
  // Sin espacio tras el guion, o con varias líneas, sigue siendo texto.
  expect(capturaRapida('-5 grados', 'text')?.kind).toBe('text');
  expect(capturaRapida('- uno\n- dos', 'text')?.kind).toBe('text');
  expect(capturaRapida('-\nComprar', 'text')?.kind).toBe('text');
  expect(capturaRapida('----', 'text')?.kind).toBe('text');
});

test('dd y tt se expanden solo como la palabra que se acaba de teclear', () => {
  const ahora = new Date(2026, 9, 2, 9, 5);
  expect(expandirAtajos('dd ', ahora)).toBe('2026-10-02 ');
  expect(expandirAtajos('Llamar tt ', ahora)).toBe('Llamar 09:05 ');
  expect(expandirAtajos('add ', ahora)).toBe('add ');
  expect(expandirAtajos('dd', ahora)).toBe('dd');
  expect(expandirAtajos('dd y más ', ahora)).toBe('dd y más ');
});

test('borrar no expande: solo un espacio recién tecleado', () => {
  const ahora = new Date(2026, 9, 2, 9, 5);
  expect(expandirCaptura('dd x', 'dd ', ahora)).toBe('dd ');
  expect(expandirCaptura('dd', 'dd ', ahora)).toBe('2026-10-02 ');
  expect(expandirCaptura('', 'dd ', ahora)).toBe('dd ');
});

test('los atajos del diario', () => {
  const k = (key: string, mod: Partial<KeyboardEvent> = {}, mac = true) =>
    atajoDiario(
      { key, metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, ...mod },
      mac,
    );
  expect(k('d', { metaKey: true })).toBe('today');
  // En macOS Ctrl+D es «borrar hacia delante»; fuera del Mac el principal es Ctrl.
  expect(k('D', { ctrlKey: true })).toBeNull();
  expect(k('D', { ctrlKey: true }, false)).toBe('today');
  expect(k('d', { metaKey: true }, false)).toBeNull();
  expect(k('ArrowLeft', { metaKey: true, shiftKey: true })).toBe('previous');
  expect(k('ArrowRight', { ctrlKey: true, shiftKey: true }, false)).toBe('next');
  expect(k('k', { metaKey: true })).toBe('capture');
  expect(k('d')).toBeNull();
  expect(k('ArrowLeft', { metaKey: true })).toBeNull();
  expect(k('d', { metaKey: true, altKey: true })).toBeNull();
});

test('el árbol deja fuera los diarios propios y conserva los que tienen hijos y los ajenos', () => {
  const nb = (id: string, extra: Partial<Notebook> = {}) =>
    ({ id, note_count: 0, owner_sub: 'yo', ...extra }) as Notebook;
  const ids = cuadernosDelArbol(
    [
      nb('a'),
      nb('d1', { daily_date: '2026-10-01', note_count: 2 }),
      nb('d2', { daily_date: '2026-10-02' }),
      nb('d3', { daily_date: '2026-10-03' }),
      nb('hijo', { parent_id: 'd2' }),
      nb('ajeno', { daily_date: '2026-10-01', owner_sub: 'otra' }),
    ],
    'yo',
  ).map((n) => n.id);
  expect(ids).toEqual(['a', 'd2', 'hijo', 'ajeno']);
});

test('las canceladas no son pendientes ni hechas, y ocultar terminadas conserva antepasados', () => {
  const notes = [
    { id: 'padre', kind: 'todo', checked: true },
    { id: 'hija', kind: 'todo', group_id: 'padre', checked: false },
    { id: 'cancelada', kind: 'todo', checked: false, cancelled_at: '2026-10-01T10:00:00Z' },
    { id: 'hecha', kind: 'todo', checked: true },
    { id: 'texto', kind: 'text' },
  ] as unknown as Note[];
  const ids = (ns: Note[]) => ns.map((n) => n.id);
  expect(ids(enfocarNotas(notes, 'pending'))).toEqual(['padre', 'hija']);
  expect(ids(enfocarNotas(notes, 'done'))).toEqual(['padre', 'hecha']);
  expect(ids(enfocarNotas(notes, 'all', true))).toEqual(['padre', 'hija', 'texto']);
  expect(terminada(notes[2]!)).toBe(true);
  expect(vecesTrasladada({ data: { carry_count: 3 } } as unknown as Note)).toBe(3);
  expect(vecesTrasladada({ data: { carry_count: '3' } } as unknown as Note)).toBe(0);
});

test('la semana va de lunes a domingo, también cruzando mes y año', () => {
  expect(semanaDe('2026-10-01')).toEqual([
    '2026-09-28',
    '2026-09-29',
    '2026-09-30',
    '2026-10-01',
    '2026-10-02',
    '2026-10-03',
    '2026-10-04',
  ]);
  expect(semanaDe('2026-09-28')[0]).toBe('2026-09-28');
  expect(semanaDe('2027-01-03')).toEqual([
    '2026-12-28',
    '2026-12-29',
    '2026-12-30',
    '2026-12-31',
    '2027-01-01',
    '2027-01-02',
    '2027-01-03',
  ]);
});

test('el resumen es el título o el texto del cuerpo, sin HTML y recortado', () => {
  expect(resumenNota({ title: ' Comprar pan ', content: '<p>x</p>' })).toBe('Comprar pan');
  expect(resumenNota({ content: '<h2>Plan</h2><p>Ir &amp; volver&nbsp;<b>hoy</b></p>' })).toBe(
    'Plan Ir & volver hoy',
  );
  expect(resumenNota({ content: '<p>&lt;script&gt;</p>' })).toBe('<script>');
  expect(resumenNota({ content: `<p>${'a'.repeat(200)}</p>` }, 10)).toBe('aaaaaaaaa…');
  expect(resumenNota({})).toBe('');
});

test('la agenda: los de día completo por su fecha UTC y el evento nace de día completo', () => {
  expect(eventoDelDia({ start: '2026-10-01T00:00:00Z', allDay: true }, '2026-10-01')).toBe(true);
  expect(eventoDelDia({ start: '2026-10-02T00:00:00Z', allDay: true }, '2026-10-01')).toBe(false);
  expect(eventoDelDia({ start: '2026-10-01T23:30:00+02:00' }, '2026-10-01')).toBe(true);
  expect(eventoDeDiaCompleto('2026-12-31', 'Fin')).toEqual({
    summary: 'Fin',
    start: '2026-12-31T00:00:00Z',
    end: '2027-01-01T00:00:00Z',
    allDay: true,
  });
  expect(eventoDeDiaCompleto('31-12-2026', 'Fin')).toBeNull();
  expect(eventoDeDiaCompleto('2026-12-31', '')).toBeNull();
});
