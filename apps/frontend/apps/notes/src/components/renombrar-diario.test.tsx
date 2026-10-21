// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
//
// En el diario, un texto sin título va en línea (sin cabecera). «Renombrar» del
// menú tiene que sacar el campo igualmente, o esa nota no puede tener título nunca.
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { Note } from '@sytue/api-clients';
import { NoteCard } from './NoteCard.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const t = ((k: string) => k) as never;
const tick = () => new Promise((resolve) => setTimeout(resolve, 0));
const nada = () => {};

test('renombrar un texto sin título del diario saca el campo', async () => {
  const host = document.createElement('div');
  document.body.append(host);
  render(
    <NoteCard
      t={t}
      diario
      seleccionada={false}
      note={{ id: 'n', kind: 'text', title: '', content: '<p>hola</p>' } as Note}
      onSelect={nada}
      onChange={nada}
      onDelete={nada}
      onDuplicate={nada}
      onTogglePin={nada}
      onShare={nada}
      onTags={nada}
      onBacklinks={nada}
    />,
    host,
  );
  expect(host.querySelector('input[aria-label="note.rename"]')).toBeNull();
  host.querySelector<HTMLButtonElement>('button[aria-label="common.more"]')!.click();
  await tick();
  const item = [...document.querySelectorAll<HTMLElement>('[role="menuitem"]')].find(
    (e) => e.textContent === 'note.rename',
  );
  item!.click();
  await tick();
  expect(host.querySelector('input[aria-label="note.rename"]')).not.toBeNull();
  render(null, host);
  host.remove();
});
