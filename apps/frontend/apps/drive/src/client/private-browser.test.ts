/* SPDX-License-Identifier: Apache-2.0 */
import { expect, test, spyOn } from 'bun:test';
import type { SuiteMatrix } from '@sytue/offline/matrix';
import { PrivateDriveBrowser } from './private-browser.js';
import { privateDriveWriteFixture } from '../../../../packages/offline/src/matrix/private-drive-fixture.js';

async function fixture() {
  const f = await privateDriveWriteFixture(),
    repository = f.open();
  await repository.initialize(f.state);
  const lifetime = new AbortController(),
    calls: string[] = [];
  const data = new PrivateDriveBrowser(
    {
      client: { isClosed: false } as unknown as SuiteMatrix,
      storage: f.repository(),
      repository,
      signal: lifetime.signal,
      events: new EventTarget(),
      uploadFailed: false,
      resume: async () => {},
    },
    'https://drive.example/drive',
    Object.assign(
      async (url: Parameters<typeof fetch>[0]) => {
        calls.push(String(url));
        if (String(url) === 'https://drive.example/drive/api/quota')
          return Response.json({ usedBytes: 2, limitBytes: 10, availableBytes: 8 });
        throw new Error('unexpected plaintext content request');
      },
      { preconnect: () => {} },
    ),
  );
  const close = async () => {
    lifetime.abort();
    data.close();
    await repository.close();
    await f.close();
  };
  return { f, data, repository, calls, close };
}

test('private browser CRUD, literal percent names and authenticated bytes never use content HTTP', async () => {
  const f = await fixture();
  try {
    await f.data.api.createFolder('/100%');
    const a = await f.data.resolve('/100%');
    const upload = await f.data.api.upload(
      '/100%/private.txt',
      new File(['device private content'], 'private.txt', { type: 'text/plain' }),
    );
    const item = upload.resource!;
    await expect((await f.data.blob(item)).text()).resolves.toBe('device private content');
    await f.data.api.upload(
      '/100%/private.txt',
      new File(['replacement bytes'], 'private.txt', { type: 'text/plain' }),
    );
    expect((await f.data.versions(item.id!)).versions.map((v) => v.version)).toEqual([2, 1]);
    expect(await (await f.data.blob(item, undefined, undefined, 1)).text()).toBe(
      'device private content',
    );
    await f.data.restoreVersion(item.id!, 1);
    expect(await (await f.data.blob(item)).text()).toBe('device private content');
    await f.data.api.rename(item.path, 'renamed.txt');
    await f.data.api.createFolder('/Target');
    await f.data.api.move('/100%/renamed.txt', '/Target/renamed.txt');
    const moved = await f.data.byID('file', item.id!);
    expect(moved.path).toBe('/Target/renamed.txt');
    expect((await f.data.byID('folder', a.id!)).path).toBe('/100%');
    expect((await f.data.search('RENAME')).map((row) => row.id)).toEqual([item.id!]);
    await f.data.api.setFavorite(moved, true);
    await f.data.api.setFavorite(a, true);
    expect((await f.data.favorites()).map((row) => row.path)).toEqual([
      '/100%',
      '/Target/renamed.txt',
    ]);
    await f.data.api.setFavorite(a, false);
    expect((await f.data.api.favorites()).map((row) => row.id)).toEqual([item.id]);
    await f.data.api.remove(moved.path);
    expect((await f.data.api.listTrash()).map((row) => row.id)).toContain(item.id!);
    await expect(f.data.byID('file', item.id!)).rejects.toThrow('not_found');
    await f.data.api.restore(item.id!, 'file');
    expect((await f.data.resolve(moved.path)).id).toBe(item.id);
    expect((await f.data.api.listTrash()).map((row) => row.id)).not.toContain(item.id!);
    expect(f.calls).toHaveLength(0);
  } finally {
    await f.close();
  }
});

test('private browser pagination exposes folders only once', async () => {
  const f = await fixture();
  const children = spyOn(f.repository, 'children').mockImplementation(async (_id, cursor) => ({
    folders: [{ id: 'folder', name: 'Folder' }],
    files: [{ id: cursor ? 'two' : 'one', name: cursor ? 'Two' : 'One', size_bytes: 0 }],
    next_cursor: cursor ? '' : 'next',
  }));
  try {
    const first = await f.data.api.list('/'),
      second = await f.data.api.list('/', first.nextCursor);
    expect(first.items.filter((row) => row.isDirectory)).toHaveLength(1);
    expect(second.items.filter((row) => row.isDirectory)).toHaveLength(0);
    expect(second.items[0]?.name).toBe('Two');
    expect(f.calls).toHaveLength(0);
  } finally {
    children.mockRestore();
    await f.close();
  }
});

test('account quota stays separate while missing private operations and closed presenters cannot fall back', async () => {
  const f = await fixture();
  try {
    expect(await f.data.api.quota()).toEqual({ usedBytes: 2, limitBytes: 10, availableBytes: 8 });
    expect(await f.data.api.favorites()).toEqual([]);
    expect(() => f.data.api.zipHref({ folderIds: ['root'] })).toThrow('matrix_unsupported');
    await expect(f.data.resolve('/../private')).rejects.toThrow();
    f.data.close();
    await expect(f.data.api.list('/')).rejects.toThrow();
    await expect(f.data.api.restore('file', 'file')).rejects.toThrow();
    await expect(f.data.favorites()).rejects.toThrow();
    await expect(f.data.versions('file')).rejects.toThrow();
    await expect(f.data.restoreVersion('file', 1)).rejects.toThrow();
    expect(f.calls).toHaveLength(1);
  } finally {
    await f.close();
  }
});

test('view cancellation stops its private query without retiring the current account', async () => {
  const f = await fixture(),
    child = new AbortController();
  let entered: (() => void) | undefined;
  const began = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const children = spyOn(f.repository, 'children').mockImplementation(
    async (_id, _cursor, signal) => {
      entered?.();
      await new Promise<void>((_resolve, reject) => {
        signal!.addEventListener('abort', () => reject(signal!.reason), { once: true });
      });
      return { folders: [], files: [], next_cursor: '' };
    },
  );
  const pending = f.data.list('/', undefined, child.signal).catch((error: unknown) => error);
  try {
    await began;
    child.abort();
    expect(await pending).toBeInstanceOf(Error);
    children.mockRestore();
    expect((await f.data.api.list('/')).items.length).toBeGreaterThan(0);
    expect(f.calls).toHaveLength(0);
  } finally {
    child.abort();
    await pending;
    children.mockRestore();
    await f.close();
  }
});

test('an unnamed legacy root opens normally while unnamed children remain invalid', async () => {
  const f = await fixture();
  const root = spyOn(f.repository, 'root').mockResolvedValue({
    id: 'root',
    name: '',
    is_root: true,
  });
  const children = spyOn(f.repository, 'children');
  try {
    expect((await f.data.resolve('/')).name).toBe('/');
    expect((await f.data.list('/')).items.length).toBeGreaterThan(0);
    children.mockResolvedValue({
      folders: [{ id: 'child', name: '' }],
      files: [],
      next_cursor: '',
    });
    await expect(f.data.list('/')).rejects.toThrow('matrix_reference');
    expect(f.calls).toHaveLength(0);
  } finally {
    children.mockRestore();
    root.mockRestore();
    await f.close();
  }
});
