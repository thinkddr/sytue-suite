// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// En el diario, la raya de una cosa es su casilla por marcar: pulsarla la hace tarea.
// Y el menú de la línea convierte en los dos sentidos. Solo donde se puede editar la
// nota entera: una compartida conmigo o una de solo lectura no cambian de tipo.
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { Note } from '@sytue/api-clients';
import { NoteCard } from './NoteCard.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((k: string) => k) as never;
const tick = () => new Promise((resolve) => setTimeout(resolve, 0));
const nada = () => {};
let host: HTMLElement | null = null;
afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
    host = null;
  }
});

function montar(note: Partial<Note>, extra: { soloLectura?: boolean; sinMenu?: boolean } = {}) {
  const cambios: unknown[] = [];
  host = document.createElement('div');
  document.body.append(host);
  render(
    <NoteCard
      t={t}
      diario
      seleccionada={false}
      note={{ id: 'n', title: '', content: '', checked: false, ...note } as Note}
      onSelect={nada}
      onChange={(patch) => cambios.push(patch)}
      onDelete={nada}
      onDuplicate={nada}
      onTogglePin={nada}
      onShare={nada}
      onTags={nada}
      onBacklinks={nada}
      {...extra}
    />,
    host,
  );
  return cambios;
}

const raya = () => host!.querySelector<HTMLButtonElement>('.notes-stream-glyph button');

/** Abre el «…» de la línea y devuelve sus entradas por su rótulo. */
async function menu(): Promise<Map<string, HTMLElement>> {
  host!.querySelector<HTMLButtonElement>('button[aria-label="common.more"]')!.click();
  await tick();
  return new Map(
    [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')].map((e) => [
      e.textContent ?? '',
      e,
    ]),
  );
}

function desmontar() {
  render(null, host!);
  host!.remove();
}

test('pulsar la raya de un texto lo hace tarea con su primera línea de título', () => {
  const cambios = montar({ kind: 'text', content: '<p>Llamar a Ana<br>al móvil</p>' });
  const boton = raya();
  expect(boton?.getAttribute('aria-label')).toBe('note.toTodo: Llamar a Ana al móvil');
  expect(boton?.classList.contains('sytue-focus-ring')).toBe(true);
  boton!.click();
  expect(cambios).toEqual([{ kind: 'todo', title: 'Llamar a Ana', content: '<p>al móvil</p>' }]);
});

test('el menú convierte un texto en tarea y una tarea en texto', async () => {
  const aTarea = montar({ kind: 'text', content: '<p>Comprar pan</p>' });
  (await menu()).get('note.toTodo')!.click();
  expect(aTarea).toEqual([{ kind: 'todo', title: 'Comprar pan', content: '' }]);
  desmontar();
  const aTexto = montar({ kind: 'todo', title: 'Comprar pan', checked: true });
  const entradas = await menu();
  expect(entradas.has('note.toTodo')).toBe(false);
  entradas.get('note.toText')!.click();
  expect(aTexto).toEqual([
    { kind: 'text', title: '', content: '<p>Comprar pan</p>', checked: false, cancelled: false },
  ]);
});

test('ni la raya ni el menú convierten lo que no se puede editar entero', async () => {
  montar({ kind: 'text', content: '<p>Ajena</p>' }, { soloLectura: true });
  expect(raya()).toBeNull();
  expect(host!.querySelector('.notes-stream-bullet')).not.toBeNull();
  desmontar();
  // Compartida conmigo con edición: su texto sí, su tipo no (el servicio lo ignora).
  montar({ kind: 'text', content: '<p>Compartida</p>' }, { sinMenu: true });
  expect(raya()).toBeNull();
  desmontar();
  // Un separador no es una cosa apuntada.
  montar({ kind: 'text', content: '<hr>' });
  expect(raya()).toBeNull();
  expect((await menu()).has('note.toTodo')).toBe(false);
});
