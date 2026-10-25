// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { Note } from '@sytue/api-clients';
import type { ClaroPorNota } from './NoteCanvas.js';
import { NoteTree } from './NoteTree.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const hosts: HTMLElement[] = [];
const nothing = () => {};
afterEach(() => {
  for (const host of hosts.splice(0)) {
    render(null, host);
    host.remove();
  }
});

function note(overrides: Partial<Note>): Note {
  return {
    id: 'note',
    notebook_id: 'notebook',
    owner_sub: 'owner',
    kind: 'text',
    title: 'A note',
    x: 0,
    y: 0,
    col: 0,
    position: 0,
    width: 320,
    collapsed: false,
    checked: false,
    pinned: false,
    visibility: 'personal',
    indexable: false,
    created_at: '2026-10-20T12:00:00Z',
    updated_at: '2026-10-20T12:00:00Z',
    ...overrides,
  };
}

function host(): HTMLElement {
  const element = document.createElement('div');
  hosts.push(element);
  document.body.appendChild(element);
  return element;
}

function tree(element: HTMLElement, entry: Note, claro: ClaroPorNota = {}, bloqueado = false) {
  render(
    <NoteTree
      t={((key: string) => key) as never}
      locale="en"
      notes={[entry]}
      claro={claro}
      bloqueado={bloqueado}
      soloLectura
      seleccionada={null}
      onSelect={nothing}
      onNueva={nothing}
      onNuevaDentro={nothing}
      onSacar={nothing}
      onToggle={nothing}
      onRenombrar={nothing}
    />,
    element,
  );
}

test('decoded markup in a preview stays text rather than injected elements', () => {
  const element = host();
  tree(
    element,
    note({
      content: '&lt;img src=x onerror=alert(1)&gt; &lt;script&gt;alert(2)&lt;/script&gt;',
    }),
  );

  expect(element.textContent).toContain('<img src=x onerror=alert(1)> <script>alert(2)</script>');
  expect(element.querySelector('img, script, [onerror], [onload]')).toBeNull();
  expect(element.innerHTML).toContain('&lt;img');
});

test('locking removes decrypted previews even while their previous values remain in props', () => {
  const element = host();
  const entry = note({
    title: 'legacy plaintext title',
    content: '<p>legacy plaintext body</p>',
    content_cipher: 'encv1:opaque',
  });
  const clear = {
    [entry.id]: { title: 'private title', content: '<p>private preview</p>' },
  };
  tree(element, entry, clear);
  expect(element.textContent).toContain('private title');
  expect(element.textContent).toContain('private preview');
  expect(element.textContent).not.toContain('legacy plaintext');

  tree(element, entry, clear, true);
  expect(element.textContent).toContain('crypto.locked');
  expect(element.textContent).not.toContain('private title');
  expect(element.textContent).not.toContain('private preview');
  expect(element.textContent).not.toContain('legacy plaintext');
});
