import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (typeof document === 'undefined') GlobalRegistrator.register();
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { SytueUser } from '@sytue/auth';
import type { Notebook } from '@sytue/api-clients';
import { NotesApp } from './NotesApp.js';

const originalFetch = globalThis.fetch;
const browser = window as unknown as { happyDOM: { setURL: (url: string) => void } };
const originalURL = window.location.href;
let host: HTMLElement | undefined;

afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
    host = undefined;
  }
  globalThis.fetch = originalFetch;
  browser.happyDOM.setURL(originalURL);
});

async function until(check: () => boolean) {
  for (let i = 0; i < 300; i++) {
    if (check()) return;
    await new Promise((resolve) => setTimeout(resolve, 10));
  }
  throw new Error('import UI did not reach expected state');
}

test('lost import acknowledgement preserves the file and requires an explicit review', async () => {
  browser.happyDOM.setURL('https://notes.invalid/notes');
  const notebook = {
    id: 'nb',
    owner_sub: 'owner',
    title: 'Imported copy',
    color: '',
    layout: 'list',
    visibility: 'personal',
    indexable: false,
    encrypted: false,
    position: 0,
    note_count: 0,
    created_at: '2026-10-20T12:00:00Z',
    updated_at: '2026-10-20T12:00:00Z',
  } as Notebook;
  const backup = '{"formato":"sytue.notepads","version":1,"notebook":{"title":"copy"},"notas":[]}';
  const writes: string[] = [];
  let committed = false;
  globalThis.fetch = (async (input, init) => {
    const path = String(input);
    if (path.includes('/api/notebooks/import?preview=1')) {
      return Response.json({ notebook, notas: [], descartadas: 0 });
    }
    if (path.includes('/api/notebooks/import?name=') && init?.method === 'POST') {
      writes.push(new TextDecoder().decode(init.body as Uint8Array));
      if (writes.length === 1) {
        committed = true; // the server may have created the notebook before the response was lost
        throw new TypeError('lost acknowledgement');
      }
      return Response.json({ notebook, importadas: 0, descartadas: 0 });
    }
    if (/\/api\/notebooks\/[^/]+\/notes/.test(path)) return Response.json({ notes: [] });
    if (path.includes('/api/notebooks'))
      return Response.json({ notebooks: committed ? [notebook] : [] });
    if (path.includes('/api/tags')) return Response.json({ tags: [] });
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
    />,
    host,
  );
  await until(() => Boolean(host?.querySelector('input[accept*=".onex"]')));
  const picker = host.querySelector<HTMLInputElement>('input[accept*=".onex"]')!;
  Object.defineProperty(picker, 'files', {
    configurable: true,
    value: [new File([backup], 'backup.onex')],
  });
  picker.dispatchEvent(new Event('change', { bubbles: true }));
  await until(() => Boolean(document.querySelector('[role="dialog"]')));
  const findButton = (text: string) =>
    [...document.querySelectorAll('button')].find((button) => button.textContent?.trim() === text);
  const confirm = findButton('Import')!;
  confirm.click();
  confirm.click();
  await until(() => Boolean(host?.textContent?.includes('may have completed partly or fully')));
  expect(writes).toEqual([backup]);
  expect(picker.disabled).toBe(true);
  await until(() => Boolean(host?.textContent?.includes('Imported copy')));
  await new Promise((resolve) => setTimeout(resolve, 50));
  expect(writes).toHaveLength(1);
  findButton('I checked; reopen backup')!.click();
  await until(() => Boolean(document.querySelector('[role="dialog"]')));
  expect(writes).toHaveLength(1);
  const dialog = document.querySelector('[role="dialog"]')!;
  const cancel = [...dialog.querySelectorAll('button')].find(
    (button) => button.textContent?.trim() === 'Cancel',
  );
  expect(cancel).toBeDefined();
  cancel!.click();
  await until(() => !document.querySelector('[role="dialog"]'));
  expect(writes).toHaveLength(1);
  findButton('I checked; reopen backup')!.click();
  await until(() => Boolean(document.querySelector('[role="dialog"]')));
  expect(writes).toHaveLength(1);
  findButton('Import')!.click();
  await until(() => writes.length === 2);
  expect(writes).toEqual([backup, backup]);
}, 15000);

test('a delayed preview cannot replace the reviewed uncertain backup', async () => {
  browser.happyDOM.setURL('https://notes.invalid/notes');
  const backupA = '{"formato":"sytue.notepads","version":1,"notebook":{"title":"A"},"notas":[]}';
  const backupB = '{"formato":"sytue.notepads","version":1,"notebook":{"title":"B"},"notas":[]}';
  const notebook = { id: 'nb', title: 'Backup B' } as Notebook;
  const writes: string[] = [];
  let requestedA = false;
  let releaseA!: () => void;
  const pendingA = new Promise<void>((resolve) => {
    releaseA = resolve;
  });
  const channel = new MessageChannel();
  const responseAProcessed = new Promise<void>((resolve) => {
    channel.port1.onmessage = () => resolve();
  });
  globalThis.fetch = (async (input, init) => {
    const path = String(input);
    if (path.includes('/api/notebooks/import?preview=1')) {
      if (new TextDecoder().decode(init?.body as Uint8Array) === backupA) {
        requestedA = true;
        await pendingA;
        const response = Response.json({
          notebook: { ...notebook, title: 'Backup A' },
          notas: [],
          descartadas: 0,
        });
        const readJSON = response.json.bind(response);
        Object.defineProperty(response, 'json', {
          value: async () => {
            const result = await readJSON();
            // A task runs after the promise continuations that consume this response.
            channel.port2.postMessage(null);
            return result;
          },
        });
        return response;
      }
      return Response.json({ notebook, notas: [], descartadas: 0 });
    }
    if (path.includes('/api/notebooks/import?name=') && init?.method === 'POST') {
      writes.push(new TextDecoder().decode(init.body as Uint8Array));
      if (writes.length === 1) throw new TypeError('lost acknowledgement');
      return Response.json({ notebook, importadas: 0, descartadas: 0 });
    }
    if (/\/api\/notebooks\/[^/]+\/notes/.test(path)) return Response.json({ notes: [] });
    if (path.includes('/api/notebooks')) return Response.json({ notebooks: [] });
    if (path.includes('/api/tags')) return Response.json({ tags: [] });
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
    />,
    host,
  );
  try {
    await until(() => Boolean(host?.querySelector('input[accept*=".onex"]')));
    const picker = host.querySelector<HTMLInputElement>('input[accept*=".onex"]')!;
    const choose = (backup: string) => {
      Object.defineProperty(picker, 'files', {
        configurable: true,
        value: [new File([backup], 'backup.onex')],
      });
      picker.dispatchEvent(new Event('change', { bubbles: true }));
    };
    const button = (text: string) =>
      [...document.querySelectorAll('button')].find((item) => item.textContent?.trim() === text)!;
    choose(backupA);
    await until(() => requestedA);
    choose(backupB);
    await until(() =>
      Boolean(document.querySelector('[role="dialog"]')?.textContent?.includes('Backup B')),
    );
    button('Import').click();
    await until(() => Boolean(host?.textContent?.includes('may have completed partly or fully')));
    expect(writes).toEqual([backupB]);
    button('I checked; reopen backup').click();
    await until(() =>
      Boolean(document.querySelector('[role="dialog"]')?.textContent?.includes('Backup B')),
    );
    expect(picker.disabled).toBe(true);
    await act(async () => {
      releaseA();
      await responseAProcessed;
    });
    expect(document.querySelector('[role="dialog"]')?.textContent).toContain('Backup B');
    expect(writes).toEqual([backupB]);
    button('Import').click();
    await until(() => writes.length === 2);
    expect(writes).toEqual([backupB, backupB]);
  } finally {
    releaseA();
    channel.port1.close();
    channel.port2.close();
  }
}, 15000);
