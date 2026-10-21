import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (typeof document === 'undefined') GlobalRegistrator.register();
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { SytueUser } from '@sytue/auth';
import type { Note, Notebook, NoteCipherMigration } from '@sytue/api-clients';
import { NotesApp } from './NotesApp.js';
import { nuevosParametros, desempaquetar } from '../client/crypto.js';

const savedFetch = globalThis.fetch;
let host: HTMLElement | undefined;
const browser = window as unknown as { happyDOM: { setURL: (url: string) => void } };
const originalURL = window.location.href;
afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
    host = undefined;
  }
  globalThis.fetch = savedFetch;
  browser.happyDOM.setURL(originalURL);
});
async function until(check: () => boolean) {
  for (let i = 0; i < 300; i++) {
    if (check()) return;
    await new Promise((r) => setTimeout(r, 10));
  }
  throw new Error('UI did not reach expected state');
}

test('actual Notes screen encrypts migration and edits, then clears plaintext when identity changes', async () => {
  browser.happyDOM.setURL('https://notes.invalid/notes');
  const { key, params } = await nuevosParametros('long password for test');
  const nb = {
    id: 'nb',
    owner_sub: 'owner',
    title: 'Encrypted notebook',
    color: '',
    layout: 'list',
    visibility: 'personal',
    indexable: false,
    encrypted: true,
    crypto: params,
    position: 0,
    note_count: 1,
    created_at: '2026-10-20T12:00:00Z',
    updated_at: '2026-10-20T12:00:00Z',
  } as Notebook;
  let row = {
    id: 'n',
    notebook_id: 'nb',
    owner_sub: 'owner',
    kind: 'text',
    title: 'historic private',
    content: '<p>historic body</p>',
    data: {
      attachments: [
        {
          id: 'a',
          name: 'private.pdf',
          size: 4,
          type: 'application/pdf',
          dataUrl: 'data:application/pdf;base64,c2VjcmV0',
        },
      ],
    },
    data_json:
      '{"attachments":[{"id":"a","name":"private.pdf","size":4,"type":"application/pdf","dataUrl":"data:application/pdf;base64,c2VjcmV0"}]}',
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
    updated_at: '2026-10-20T12:00:00.123456Z',
  } as Note;
  const writes: { method: string; body: Record<string, unknown> }[] = [];
  let releaseCreate: () => void = () => {};
  let createFinished = false;
  let oldBackend = false;
  let rejectedCreates = 0;
  let rejectedEdits = 0;
  let uncertainEdit: { kind: 'server' | 'network' | 'conflict'; calls: number } | undefined;
  let legacyWrites = 0;
  let listedNotes = 0;
  const pauseCreate = new Promise<void>((resolve) => {
    releaseCreate = resolve;
  });
  globalThis.fetch = (async (input, init) => {
    const path = String(input);
    const method = init?.method ?? 'GET';
    if (
      (method === 'POST' && path.endsWith('/notebooks/nb/notes')) ||
      (method === 'PUT' && path.endsWith('/api/notes/n'))
    ) {
      legacyWrites++;
      return Response.json({ ...row, id: 'unsafe-legacy-write' });
    }
    if (path.includes('/encrypted-content')) {
      const body = JSON.parse(String(init?.body)) as NoteCipherMigration;
      writes.push({ method, body: body as unknown as Record<string, unknown> });
      expect(body.expected_crypto).toEqual(params);
      expect(body.expected_updated_at).toBe(row.updated_at);
      row = {
        ...row,
        title: '',
        content: '',
        content_cipher: body.content_cipher,
        data: body.data ?? {},
        updated_at: '2026-10-20T12:00:01Z',
      };
      delete row.data_json;
      return Response.json(row);
    }
    if (path.endsWith('/api/notes/n/encrypted') && method === 'PUT') {
      if (oldBackend) {
        rejectedEdits++;
        return new Response('Not found', { status: 404 });
      }
      const body = JSON.parse(String(init?.body)) as Record<string, unknown>;
      if (uncertainEdit) {
        uncertainEdit.calls++;
        if (uncertainEdit.kind === 'network') {
          // Simulate a committed write whose acknowledgement never reaches the browser.
          row = { ...row, ...body } as Note;
          throw new TypeError('simulated lost acknowledgement');
        }
        return Response.json(
          { error: uncertainEdit.kind === 'conflict' ? 'note_changed' : 'upstream_error' },
          { status: uncertainEdit.kind === 'conflict' ? 409 : 500 },
        );
      }
      writes.push({ method, body });
      row = { ...row, ...body } as Note;
      return Response.json(row);
    }
    if (path.endsWith('/notebooks/nb/encrypted-notes') && method === 'POST') {
      if (oldBackend) {
        rejectedCreates++;
        return new Response('Not found', { status: 404 });
      }
      const body = JSON.parse(String(init?.body)) as Record<string, unknown>;
      writes.push({ method, body });
      await pauseCreate;
      createFinished = true;
      return Response.json({ ...row, ...body, id: 'n2' });
    }
    if (path.includes('/notebooks/nb/notes')) {
      listedNotes++;
      return Response.json({ notes: [row] });
    }
    if (path.includes('/api/notebooks')) return Response.json({ notebooks: [nb] });
    if (path.includes('/api/tags')) return Response.json({ tags: [] });
    if (path.includes('/api/reminders')) return Response.json({ reminders: [] });
    return Response.json({ notifications: [], unread: 0, accessToken: '', access_token: '' });
  }) as typeof fetch;
  host = document.createElement('div');
  document.body.append(host);
  const user = {
    id: 'owner',
    tenant: 'test',
    email: 'owner@example.invalid',
    name: 'Owner',
    roles: [],
  } as unknown as SytueUser;
  render(
    <NotesApp
      user={user}
      hubBaseUrl="https://hub.invalid"
      publicBase="https://notes.invalid/notes"
      locale="en"
      notebookInicial="nb"
    />,
    host,
  );
  await until(() =>
    [...document.querySelectorAll('button')].some(
      (b) => b.textContent?.trim() === 'Unlock notebook',
    ),
  );
  // The locked view must stay closed once the legacy plaintext rows have ARRIVED, not only
  // before the list request resolves (that race hid a sidebar leak locally).
  await until(() => listedNotes > 0);
  await new Promise((r) => setTimeout(r, 50));
  expect(host.textContent).not.toContain('historic private');
  expect(host.textContent).not.toContain('private.pdf');
  (
    [...document.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === 'Unlock notebook',
    ) as HTMLButtonElement
  ).click();
  await until(() => Boolean(document.querySelector('input[type="password"]')));
  const pass = document.querySelector('input[type="password"]') as HTMLInputElement;
  pass.value = 'long password for test';
  pass.dispatchEvent(new Event('input', { bubbles: true }));
  await new Promise((r) => setTimeout(r, 0));
  document
    .querySelector('#cuaderno-clave')!
    .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await until(() => writes.length === 1 && Boolean(host?.textContent?.includes('private.pdf')));
  const body = writes[0]!.body;
  expect(JSON.stringify(body)).not.toContain('private.pdf');
  expect(JSON.stringify(body)).not.toContain('historic private');
  expect(body.cipher_version).toBe(2);
  const clear = await desempaquetar(key, String(body.content_cipher));
  expect(clear?.data?.attachments).toHaveLength(1);
  expect(clear?.title).toBe('historic private');
  await until(() =>
    [...(host?.querySelectorAll('[data-note-id="n"] button') ?? [])].some(
      (b) => b.textContent?.trim() === 'historic private',
    ),
  );
  const titleButton = [...host.querySelectorAll('[data-note-id="n"] button')].find(
    (b) => b.textContent?.trim() === 'historic private',
  ) as HTMLButtonElement;
  titleButton.click();
  await until(() => Boolean(host?.querySelector('[data-note-id="n"] input:not([type="file"])')));
  const titleInput = host.querySelector(
    '[data-note-id="n"] input:not([type="file"])',
  ) as HTMLInputElement;
  titleInput.value = 'changed private title';
  titleInput.dispatchEvent(new Event('input', { bubbles: true }));
  await new Promise((r) => setTimeout(r, 0));
  titleInput.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
  await until(() => writes.length === 2);
  const edited = writes[1]!.body;
  expect(edited.expected_crypto).toEqual(params);
  expect(edited.cipher_version).toBe(2);
  expect(JSON.stringify(edited)).not.toContain('private.pdf');
  expect(JSON.stringify(edited)).not.toContain('changed private title');
  const reopened = await desempaquetar(key, String(edited.content_cipher));
  expect(reopened?.title).toBe('changed private title');
  expect(reopened?.data?.attachments).toHaveLength(1);
  const newButton = host.querySelector('button[aria-label="New note"]') as HTMLButtonElement;
  expect(newButton).not.toBeNull();
  const editTitle = async (current: string, next: string) => {
    const button = [...host!.querySelectorAll('[data-note-id="n"] button')].find(
      (b) => b.textContent?.trim() === current,
    ) as HTMLButtonElement;
    expect(button).not.toBeUndefined();
    button.click();
    await until(() => Boolean(host?.querySelector('[data-note-id="n"] input:not([type="file"])')));
    const input = host!.querySelector(
      '[data-note-id="n"] input:not([type="file"])',
    ) as HTMLInputElement;
    input.value = next;
    input.dispatchEvent(new Event('input', { bubbles: true }));
    await new Promise((r) => setTimeout(r, 0));
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
  };
  const retryButton = () =>
    [...document.querySelectorAll('[role="alert"] button')].find(
      (b) => b.textContent?.trim() === 'Retry',
    ) as HTMLButtonElement;
  oldBackend = true;
  await editTitle('changed private title', 'unsaved private draft');
  await until(
    () => rejectedEdits === 1 && Boolean(document.body.textContent?.includes('Could not save.')),
  );
  expect(host.textContent).toContain('unsaved private draft');
  expect(host.textContent).toContain('private.pdf');
  expect((await desempaquetar(key, row.content_cipher!))?.title).toBe('changed private title');
  expect(writes).toHaveLength(2);
  expect(legacyWrites).toBe(0);
  newButton.click();
  await until(
    () => rejectedCreates === 1 && Boolean(document.body.textContent?.includes('Could not save.')),
  );
  expect(writes).toHaveLength(2);
  expect(legacyWrites).toBe(0);
  expect(host.querySelector('[data-note-id="unsafe-legacy-write"]')).toBeNull();
  oldBackend = false;
  const retry = retryButton();
  expect(retry).not.toBeUndefined();
  retry.click();
  retry.click();
  await until(() => writes.length === 3);
  const recovered = await desempaquetar(key, row.content_cipher!);
  expect(recovered?.title).toBe('unsaved private draft');
  expect(recovered?.data?.attachments).toHaveLength(1);
  let currentDraft = 'unsaved private draft';
  for (const kind of ['server', 'network', 'conflict'] as const) {
    for (const alert of document.querySelectorAll('[role="alert"]'))
      (alert.querySelector('button[aria-label]') as HTMLButtonElement).click();
    await until(() => document.querySelectorAll('[role="alert"]').length === 0);
    uncertainEdit = { kind, calls: 0 };
    const nextDraft = `private ${kind} draft`;
    await editTitle(currentDraft, nextDraft);
    await until(
      () =>
        uncertainEdit?.calls === 1 &&
        Boolean(document.body.textContent?.includes('Could not save.')),
    );
    expect(retryButton()).toBeUndefined();
    expect(host.textContent).toContain(nextDraft);
    expect(host.textContent).toContain('private.pdf');
    expect(uncertainEdit.calls).toBe(1);
    expect(writes).toHaveLength(3);
    if (kind === 'network')
      expect((await desempaquetar(key, row.content_cipher!))?.title).toBe(nextDraft);
    currentDraft = nextDraft;
  }
  uncertainEdit = undefined;
  oldBackend = true;
  await editTitle(currentDraft, 'draft discarded on lock');
  await until(() => rejectedEdits === 2 && Boolean(retryButton()));
  const staleRetry = retryButton();
  oldBackend = false;
  newButton.click();
  await until(() => writes.length === 4);
  const created = writes[3]!.body;
  expect(created.cipher_version).toBe(2);
  expect(created.expected_crypto).toEqual(params);
  expect(created.title).toBe('');
  expect(Object.keys(created.data as Record<string, unknown>)).toEqual(['client_id']);
  expect(typeof (created.data as Record<string, unknown>).client_id).toBe('string');
  const lockButton = [...host.querySelectorAll('button')].find((b) =>
    b.textContent?.trim().startsWith('Lock'),
  ) as HTMLButtonElement;
  expect(lockButton).not.toBeUndefined();
  lockButton.click();
  await until(() => !host?.textContent?.includes('private.pdf'));
  staleRetry.click();
  expect(writes).toHaveLength(4);
  releaseCreate();
  await until(() => createFinished);
  await new Promise((r) => setTimeout(r, 20));
  expect(host.querySelector('[data-note-id="n2"]')).toBeNull();
  // Replacing the identity remounts the view and cannot expose old clear state.
  render(
    <NotesApp
      user={{ ...user, id: 'other' }}
      hubBaseUrl="https://hub.invalid"
      publicBase="https://notes.invalid/notes"
      locale="en"
    />,
    host,
  );
  expect(host.textContent).not.toContain('historic private');
  expect(host.textContent).not.toContain('private.pdf');
  expect(document.body.textContent).not.toContain('draft discarded on lock');
  expect(legacyWrites).toBe(0);
}, 15000);
