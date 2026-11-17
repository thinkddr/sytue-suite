// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// ANCHO DE LECTURA DE LA HOJA (D-NOTA-04).
//
// La hoja de un árbol va a todo el ancho del panel —lo eligió el dueño—, pero en un
// monitor ancho las líneas pasan de 120 caracteres. Cada nota puede estrecharse a la
// columna de lectura desde su «…», como el «Full width» de Notion, y la elección se
// guarda en su `data` sin tocar lo demás que lleva (icono, adjuntos…). Solo en la hoja:
// una tarjeta del tablero o una línea del diario ya tienen su ancho.
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { Note } from '@sytue/api-clients';
import { CLAVE_ANCHO_LECTURA, NoteCard } from './NoteCard.js';

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

function montar(
  note: Partial<Note>,
  forma: { llenar?: boolean; diario?: boolean } = { llenar: true },
): unknown[] {
  const cambios: unknown[] = [];
  host = document.createElement('div');
  document.body.append(host);
  render(
    <NoteCard
      t={t}
      {...forma}
      seleccionada={false}
      note={
        {
          id: 'n',
          kind: 'text',
          title: 'Acta',
          content: '<p>Una línea larga</p>',
          checked: false,
          ...note,
        } as Note
      }
      onSelect={nada}
      onChange={(patch) => cambios.push(patch)}
      onDelete={nada}
      onDuplicate={nada}
      onTogglePin={nada}
      onShare={nada}
      onTags={nada}
      onBacklinks={nada}
    />,
    host,
  );
  return cambios;
}

const hoja = () => host!.querySelector('article')!;

/** Abre el «…» de la nota y devuelve sus entradas por su rótulo. */
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

test('por defecto la hoja va a todo el ancho y el menú ofrece estrecharla', async () => {
  const cambios = montar({ data: { icon: 'star' } });
  expect(hoja().classList.contains('notes-sheet')).toBe(true);
  expect(hoja().classList.contains('notes-sheet--lectura')).toBe(false);
  const entradas = await menu();
  expect(entradas.has('note.fullWidth')).toBe(false);
  entradas.get('note.readingWidth')!.click();
  // Lo que ya llevaba la nota (su icono) se queda.
  expect(cambios).toEqual([{ data: { icon: 'star', [CLAVE_ANCHO_LECTURA]: true } }]);
});

test('con el ancho de lectura, la hoja se estrecha y el menú la devuelve a todo el ancho', async () => {
  const cambios = montar({ data: { icon: 'star', [CLAVE_ANCHO_LECTURA]: true } });
  expect(hoja().classList.contains('notes-sheet--lectura')).toBe(true);
  const entradas = await menu();
  expect(entradas.has('note.readingWidth')).toBe(false);
  entradas.get('note.fullWidth')!.click();
  // Se quita la clave, no se deja a `false`: la nota vuelve a ser la de siempre.
  expect(cambios).toEqual([{ data: { icon: 'star' } }]);
});

test('fuera de la hoja (tarjeta o línea del diario) no hay ancho de lectura', async () => {
  for (const forma of [{ llenar: false }, { diario: true }]) {
    montar({ data: { [CLAVE_ANCHO_LECTURA]: true } }, forma);
    expect(hoja().classList.contains('notes-sheet--lectura')).toBe(false);
    const entradas = await menu();
    expect(entradas.has('note.readingWidth')).toBe(false);
    expect(entradas.has('note.fullWidth')).toBe(false);
    render(null, host!);
    host!.remove();
    host = null;
  }
});

test('la clase de lectura estrecha la hoja a la columna de 46rem y la centra', async () => {
  const css = await Bun.file(new URL('../styles/app.css', import.meta.url)).text();
  const i = css.indexOf('.notes-sheet--lectura {');
  expect(i).toBeGreaterThan(-1);
  const regla = css.slice(i, css.indexOf('}', i));
  expect(regla).toContain('mx-auto');
  expect(regla).toContain('max-w-[46rem]');
});
