// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (typeof document === 'undefined') GlobalRegistrator.register();
import { afterEach, expect, test } from 'bun:test';
import { render, type ComponentProps } from 'preact';
import type { Notebook, NotebookInput } from '@sytue/api-clients';
import type { NotesCrypto } from '../client/crypto.js';
import type { T } from '../i18n/index.js';
import { NotebookSettings } from './NotebookSettings.js';

const t = ((key: string) => key) as T;
const tick = () => new Promise((resolve) => setTimeout(resolve, 0));
const nada = () => {};
let host: HTMLElement | undefined;

afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
    host = undefined;
  }
});

const crypto: NotesCrypto = {
  v: 1,
  kdf: 'PBKDF2-SHA256',
  salt: 'MDEyMzQ1Njc4OWFiY2RlZg==',
  iterations: 210000,
  check: 'existing-notebook-check',
};

function notebook(encrypted = false): Notebook {
  return {
    id: 'shared-notebook',
    owner_sub: 'someone-else',
    parent_id: 'parent-notebook',
    title: 'Shared notebook',
    emoji: '📓',
    color: '#146a47',
    layout: 'tree',
    visibility: 'organization',
    indexable: !encrypted,
    encrypted,
    ...(encrypted ? { crypto } : {}),
    room_id: 'shared-room',
    position: 0,
    note_count: 0,
    created_at: '2026-10-26T09:00:00Z',
    updated_at: '2026-10-26T09:00:00Z',
  } as Notebook;
}

function mount(props: Omit<ComponentProps<typeof NotebookSettings>, 't' | 'onClose'>) {
  host = document.createElement('div');
  document.body.append(host);
  render(<NotebookSettings t={t} onClose={nada} {...props} />, host);
}

// El Modal real usa un portal: las consultas se hacen en el diálogo, no en host.
function dialog(): HTMLElement {
  const result = document.querySelector<HTMLElement>('[role="dialog"]');
  expect(result).not.toBeNull();
  return result!;
}

function footerButton(label: string): HTMLButtonElement | undefined {
  return [...dialog().querySelectorAll<HTMLButtonElement>('button')].find(
    (button) => button.textContent?.trim() === label,
  );
}

function optionButton(label: string): HTMLButtonElement {
  const result = [...dialog().querySelectorAll<HTMLButtonElement>('button')].find((button) =>
    [...button.querySelectorAll('span')].some((span) => span.textContent?.trim() === label),
  );
  expect(result).not.toBeUndefined();
  return result!;
}

function inputFor(label: string): HTMLInputElement {
  const fieldLabel = [...dialog().querySelectorAll<HTMLLabelElement>('label')].find(
    (element) => element.textContent?.trim() === label,
  );
  expect(fieldLabel).not.toBeUndefined();
  const result = document.getElementById(fieldLabel!.htmlFor) as HTMLInputElement | null;
  expect(result).not.toBeNull();
  return result!;
}

async function setChecked(input: HTMLInputElement, checked: boolean) {
  input.checked = checked;
  input.dispatchEvent(new Event('change', { bubbles: true }));
  await tick();
}

async function setTitle(value: string) {
  const input = inputFor('notebook.title');
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  await tick();
}

for (const encrypted of [false, true]) {
  test(`sin permiso conserva acceso (${encrypted ? 'cifrado' : 'claro'})`, async () => {
    const original = notebook(encrypted);
    const saved: NotebookInput[] = [];
    mount({
      notebook: original,
      puedeCambiarAcceso: false,
      // NotesApp no pasa onDelete a quien no es dueño.
      onSave: async (input) => {
        saved.push(input);
      },
    });

    expect(dialog().textContent).not.toContain('visibility.label');
    expect(dialog().textContent).not.toContain('crypto.title');
    expect(dialog().querySelector('[role="switch"]')).toBeNull();
    expect(footerButton('notebook.delete')).toBeUndefined();
    expect(inputFor('notebook.title').value).toBe(original.title);
    expect(optionButton('layout.tree').getAttribute('aria-pressed')).toBe('true');
    const index = inputFor('index.label');
    expect(index.disabled).toBe(encrypted);
    expect(index.checked).toBe(!encrypted);
    expect(footerButton('common.save')?.disabled).toBe(false);

    await setTitle('  Shared notebook renamed  ');
    optionButton('layout.columns').click();
    await tick();
    if (!encrypted) await setChecked(index, false);
    footerButton('common.save')!.click();
    await tick();

    expect(saved).toEqual([
      {
        parent_id: 'parent-notebook',
        title: 'Shared notebook renamed',
        emoji: original.emoji ?? '',
        color: original.color,
        layout: 'columns',
        visibility: original.visibility,
        indexable: false,
        encrypted: original.encrypted,
        crypto: encrypted ? crypto : null,
        room_id: 'shared-room',
      },
    ]);
  });
}

test('un cuaderno existente sin permiso explícito no ofrece cambiar el acceso', () => {
  mount({ notebook: notebook(), onSave: async () => {} });
  expect(dialog().textContent).not.toContain('visibility.label');
  expect(dialog().textContent).not.toContain('crypto.title');
  expect(inputFor('notebook.title').value).toBe('Shared notebook');
  expect(footerButton('common.save')?.disabled).toBe(false);
});

test('administrar conserva los controles de acceso y permite guardar sus cambios', async () => {
  const saved: NotebookInput[] = [];
  mount({
    notebook: notebook(true),
    puedeCambiarAcceso: true,
    onDelete: nada,
    onSave: async (input) => {
      saved.push(input);
    },
  });

  expect(dialog().textContent).toContain('visibility.label');
  expect(optionButton('visibility.organization').getAttribute('aria-pressed')).toBe('true');
  const encryption = inputFor('crypto.title');
  expect(encryption.getAttribute('role')).toBe('switch');
  expect(encryption.disabled).toBe(false);
  expect(encryption.checked).toBe(true);
  expect(footerButton('notebook.delete')).not.toBeUndefined();

  await setChecked(encryption, false);
  expect(dialog().textContent).toContain('crypto.disableWarning');
  expect(inputFor('index.label').disabled).toBe(false);
  optionButton('visibility.private').click();
  await tick();
  footerButton('common.save')!.click();
  await tick();

  expect(saved).toHaveLength(1);
  expect(saved[0]!.visibility).toBe('private');
  expect(saved[0]!.encrypted).toBe(false);
  expect(saved[0]!.crypto).toBeNull();
  expect(saved[0]!.indexable).toBe(false);
});

test('un cuaderno nuevo conserva los controles de acceso por defecto', async () => {
  const saved: NotebookInput[] = [];
  mount({
    notebook: null,
    parentId: 'parent-notebook',
    onSave: async (input) => {
      saved.push(input);
    },
  });

  expect(optionButton('visibility.personal').getAttribute('aria-pressed')).toBe('true');
  expect(inputFor('crypto.title').checked).toBe(false);
  expect(inputFor('crypto.title').disabled).toBe(false);
  expect(inputFor('index.label').disabled).toBe(false);
  expect(footerButton('notebook.delete')).toBeUndefined();
  expect(footerButton('common.save')?.disabled).toBe(true);

  await setTitle('New notebook');
  optionButton('visibility.organization').click();
  await tick();
  footerButton('common.save')!.click();
  await tick();

  expect(saved).toEqual([
    {
      parent_id: 'parent-notebook',
      title: 'New notebook',
      emoji: '',
      color: '#86198f',
      layout: 'tree',
      visibility: 'organization',
      indexable: true,
      encrypted: false,
      crypto: null,
      room_id: '',
    },
  ]);
});
