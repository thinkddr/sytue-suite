/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */

import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { Note, Notebook } from '@sytue/api-clients';
import { NoteCanvas } from './NoteCanvas.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
let host: HTMLDivElement | null = null;
afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
    host = null;
  }
});
const noop = () => {};
const notes: Note[] = [0, 1].map((i) => ({
  id: `n${i}`,
  notebook_id: 'nb',
  owner_sub: 'u',
  collapsed: false,
  checked: false,
  pinned: false,
  created_at: '2026-10-04T10:00:00Z',
  updated_at: '2026-10-04T10:00:00Z',
  group_id: 'grupo',
  kind: 'text',
  title: `Nota ${i}`,
  content: '',
  x: 24,
  y: 24,
  col: i,
  position: i,
  width: 300,
  states: [],
  links: [],
}));
function mount(layout: Notebook['layout'], moves: unknown[], readOnly = false) {
  host = document.createElement('div');
  document.body.append(host);
  render(
    <NoteCanvas
      t={((key: string) => key) as never}
      notebook={{ id: 'nb', layout } as Notebook}
      notes={notes}
      cargando={false}
      claro={{}}
      bloqueado={false}
      soloLectura={readOnly}
      seleccionada={null}
      onSelect={noop}
      onChange={noop}
      onMove={(id, pos) => moves.push({ id, ...pos })}
      onDelete={noop}
      onDuplicate={noop}
      onTogglePin={noop}
      onShare={noop}
      onTags={noop}
      onBacklinks={noop}
    />,
    host,
  );
  const grip = host.querySelector<HTMLButtonElement>('button[title="history.move"]');
  if (grip) {
    grip.setPointerCapture = noop;
    grip.hasPointerCapture = () => false;
  }
  return grip;
}
function pointer(grip: HTMLElement, type: string, x: number, y: number) {
  grip.dispatchEvent(
    new PointerEvent(type, {
      bubbles: true,
      cancelable: true,
      pointerId: 1,
      button: 0,
      clientX: x,
      clientY: y,
    }),
  );
}
test('libre guarda una sola posición al soltar, sin modificar la columna', () => {
  const moves: unknown[] = [];
  const grip = mount('free', moves)!;
  pointer(grip, 'pointerdown', 30, 30);
  pointer(grip, 'pointermove', 130, 160);
  expect(moves).toEqual([]);
  pointer(grip, 'pointerup', 130, 160);
  expect(moves).toEqual([{ id: 'n0', x: 124, y: 154, col: 0, position: 0, group_id: 'grupo' }]);
});
test('cancelar o pulsar el asa sin arrastrar no escribe ni mueve la nota', () => {
  const moves: unknown[] = [];
  const grip = mount('free', moves)!;
  pointer(grip, 'pointerdown', 30, 30);
  pointer(grip, 'pointermove', 130, 160);
  pointer(grip, 'pointercancel', 130, 160);
  pointer(grip, 'pointerdown', 30, 30);
  pointer(grip, 'pointerup', 30, 30);
  expect(moves).toEqual([]);
});
test('lista y columnas también ofrecen el asa; un cuaderno de lectura no', () => {
  for (const layout of ['list', 'columns'] as const) {
    expect(mount(layout, [])).not.toBeNull();
    render(null, host!);
    host!.remove();
    host = null;
  }
  expect(mount('free', [], true)).toBeNull();
});
