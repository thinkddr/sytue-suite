// SPDX-License-Identifier: Apache-2.0
import { expect, test } from 'bun:test';
import type { APIContext } from 'astro';
import { createNotebookImportHandler } from '../../../lib/notebookImport.js';

test('the import endpoint forwards only bounded, clear backups', async () => {
  let calls = 0;
  const POST = createNotebookImportHandler({
    conCliente: async () => ({
      client: {
        previewNotebook: async () => {
          calls++;
          return {};
        },
        importNotebook: async () => {
          calls++;
          return {};
        },
      },
    }),
    json: (body: unknown, status = 200) => Response.json(body, { status }),
    fallo: () => Response.json({ error: 'upstream_error' }, { status: 502 }),
  });
  const url = new URL('https://example.test/api/notebooks/import?preview=1');
  const invoke = (body: ReadableStream<Uint8Array>, target = url) =>
    POST({
      url: target,
      request: new Request(target, { method: 'POST', body, duplex: 'half' } as RequestInit),
    } as APIContext);
  const streamText = (parts: string[]) =>
    new ReadableStream<Uint8Array>({
      start(controller) {
        for (const part of parts) controller.enqueue(new TextEncoder().encode(part));
        controller.close();
      },
    });

  const clear = await invoke(streamText(['{"notebook":', '{"title":"clear"},"notas":[]}']));
  expect(clear.status).toBe(200);
  expect(calls).toBe(1);
  const encrypted = await invoke(
    streamText(['{"notebook":{"encrypted":true,', '"title":"secret"},"notas":[]}']),
  );
  expect(encrypted.status).toBe(409);
  expect(calls).toBe(1);

  let cancelled = false;
  let index = 0;
  const oversized = new ReadableStream<Uint8Array>({
    pull(controller) {
      controller.enqueue(new Uint8Array(index++ < 2 ? 16 * 1024 * 1024 : 1));
    },
    cancel() {
      cancelled = true;
    },
  });
  const rejected = await invoke(oversized);
  expect(rejected.status).toBe(400);
  expect(cancelled).toBe(true);
  expect(calls).toBe(1);

  for (const fragmentSize of [0, 1]) {
    let fragmentCancelled = false;
    const fragmented = new ReadableStream<Uint8Array>({
      pull(controller) {
        controller.enqueue(new Uint8Array(fragmentSize));
      },
      cancel() {
        fragmentCancelled = true;
      },
    });
    const response = await invoke(fragmented);
    expect(response.status).toBe(400);
    expect(fragmentCancelled).toBe(true);
    expect(calls).toBe(1);
  }

  let blockedCancelCalled = false;
  let chunks = 0;
  const blockedCancel = new ReadableStream<Uint8Array>({
    pull(controller) {
      controller.enqueue(new Uint8Array(chunks++ < 2 ? 16 * 1024 * 1024 : 1));
    },
    cancel() {
      blockedCancelCalled = true;
      return new Promise<void>(() => {});
    },
  });
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    const response = await Promise.race([
      invoke(blockedCancel),
      new Promise<null>((resolve) => {
        timer = setTimeout(() => resolve(null), 5000);
      }),
    ]);
    expect(response?.status).toBe(400);
    expect(blockedCancelCalled).toBe(true);
    expect(calls).toBe(1);
  } finally {
    if (timer !== undefined) clearTimeout(timer);
  }

  // The same BFF route without preview must block before creating a notebook.
  const importUrl = new URL('https://example.test/api/notebooks/import');
  const encryptedImport = await invoke(
    streamText(['{"notebook":{"encrypted":true},"notas":[]}']),
    importUrl,
  );
  expect(encryptedImport.status).toBe(409);
  expect(calls).toBe(1);
  const fragmentedImport = new ReadableStream<Uint8Array>({
    pull(controller) {
      controller.enqueue(new Uint8Array(1));
    },
  });
  expect((await invoke(fragmentedImport, importUrl)).status).toBe(400);
  expect(calls).toBe(1);
  const clearImport = await invoke(
    streamText(['{"notebook":{"title":"clear"},"notas":[]}']),
    importUrl,
  );
  expect(clearImport.status).toBe(201);
  expect(calls).toBe(2);
}, 20000);

test('authentication fails before reading or forwarding the backup', async () => {
  const handler = createNotebookImportHandler({
    conCliente: async () => Response.json({ error: 'unauthenticated' }, { status: 401 }),
    json: (body: unknown, status = 200) => Response.json(body, { status }),
    fallo: () => Response.json({ error: 'upstream_error' }, { status: 502 }),
  });
  const url = new URL('https://example.test/api/notebooks/import');
  const request = new Request(url, { method: 'POST', body: 'private backup' });
  const response = await handler({ url, request } as APIContext);
  expect(response.status).toBe(401);
  expect(request.bodyUsed).toBe(false);
});

test('an aborted stalled request is never forwarded to the backend', async () => {
  let calls = 0;
  let entered!: () => void;
  const reading = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const handler = createNotebookImportHandler({
    conCliente: async () => ({
      client: {
        previewNotebook: async () => {
          calls++;
          return {};
        },
        importNotebook: async () => {
          calls++;
          return {};
        },
      },
    }),
    json: (body: unknown, status = 200) => Response.json(body, { status }),
    fallo: () => Response.json({ error: 'upstream_error' }, { status: 502 }),
  });
  const controller = new AbortController();
  const body = new ReadableStream<Uint8Array>({
    pull() {
      entered();
      return new Promise<void>(() => {});
    },
    cancel() {
      return new Promise<void>(() => {});
    },
  });
  const url = new URL('https://example.test/api/notebooks/import');
  const request = new Request(url, {
    method: 'POST',
    body,
    signal: controller.signal,
    duplex: 'half',
  } as RequestInit);
  const pending = handler({ url, request } as APIContext);
  await reading;
  controller.abort();
  expect((await pending).status).toBe(400);
  expect(calls).toBe(0);
  expect(request.body?.locked).toBe(false);
}, 2000);

test('a request aborted after the body is read is not forwarded', async () => {
  let calls = 0;
  const handler = createNotebookImportHandler({
    conCliente: async () => ({
      client: {
        previewNotebook: async () => {
          calls++;
          return {};
        },
        importNotebook: async () => {
          calls++;
          return {};
        },
      },
    }),
    json: (body: unknown, status = 200) => Response.json(body, { status }),
    fallo: () => Response.json({ error: 'upstream_error' }, { status: 502 }),
  });
  const controller = new AbortController();
  const url = new URL('https://example.test/api/notebooks/import');
  const request = new Request(url, {
    method: 'POST',
    body: '{"notebook":{"title":"clear"},"notas":[]}',
    signal: controller.signal,
  });
  const actualSignal = request.signal;
  let observations = 0;
  Object.defineProperty(request, 'signal', {
    get() {
      if (++observations === 2) controller.abort();
      return actualSignal;
    },
  });
  const response = await handler({ url, request } as APIContext);
  expect(response.status).toBe(400);
  expect(calls).toBe(0);
});

test('an abort in flight reaches the import client', async () => {
  const controller = new AbortController();
  let started!: () => void;
  const invoked = new Promise<void>((resolve) => {
    started = resolve;
  });
  let received: AbortSignal | undefined;
  const send = async (_body: Uint8Array, _name?: string, signal?: AbortSignal) => {
    received = signal;
    started();
    return new Promise<never>((_, reject) => {
      if (signal?.aborted) reject(signal.reason);
      else signal?.addEventListener('abort', () => reject(signal?.reason), { once: true });
    });
  };
  const handler = createNotebookImportHandler({
    conCliente: async () => ({ client: { previewNotebook: send, importNotebook: send } }),
    json: (body: unknown, status = 200) => Response.json(body, { status }),
    fallo: () => Response.json({ error: 'upstream_error' }, { status: 502 }),
  });
  const url = new URL('https://example.test/api/notebooks/import');
  const request = new Request(url, {
    method: 'POST',
    body: '{"notebook":{"title":"clear"},"notas":[]}',
    signal: controller.signal,
  });
  const pending = handler({ url, request } as APIContext);
  await invoked;
  expect(received).toBe(request.signal);
  controller.abort();
  const response = await pending;
  expect(response.status).toBe(502);
  expect(await response.json()).toEqual({ error: 'import_result_unknown' });
}, 2000);

test('a backend commit followed by abort is reported as uncertain, even if it resolves', async () => {
  const controller = new AbortController();
  let started!: () => void;
  const invoked = new Promise<void>((resolve) => {
    started = resolve;
  });
  let finish!: (value: unknown) => void;
  let calls = 0;
  let committed = false;
  const send = async () => {
    calls++;
    committed = true;
    started();
    return new Promise<unknown>((resolve) => {
      finish = resolve;
    });
  };
  const handler = createNotebookImportHandler({
    conCliente: async () => ({ client: { previewNotebook: send, importNotebook: send } }),
    json: (body: unknown, status = 200) => Response.json(body, { status }),
    fallo: () => Response.json({ error: 'upstream_error' }, { status: 502 }),
  });
  const url = new URL('https://example.test/api/notebooks/import');
  const request = new Request(url, {
    method: 'POST',
    body: '{"notebook":{"title":"clear"},"notas":[]}',
    signal: controller.signal,
  });
  const pending = handler({ url, request } as APIContext);
  await invoked;
  expect(committed).toBe(true);
  controller.abort();
  finish({ notebook: { title: 'clear' }, importadas: 1, descartadas: 0 });
  const response = await pending;
  expect(response.status).toBe(502);
  expect(await response.json()).toEqual({ error: 'import_result_unknown' });
  expect(calls).toBe(1);
}, 2000);

for (const failure of ['connection lost', 'server 500 after partial commit']) {
  test(`import reports an uncertain result after ${failure}`, async () => {
    let calls = 0;
    let committed = false;
    let translated = 0;
    const send = async () => {
      calls++;
      committed = true;
      throw Object.assign(new Error(failure), { status: 500 });
    };
    const handler = createNotebookImportHandler({
      conCliente: async () => ({ client: { previewNotebook: send, importNotebook: send } }),
      json: (body: unknown, status = 200) => Response.json(body, { status }),
      fallo: () => {
        translated++;
        return Response.json({ error: 'upstream_error' }, { status: 500 });
      },
    });
    const url = new URL('https://example.test/api/notebooks/import');
    const request = new Request(url, {
      method: 'POST',
      body: '{"notebook":{"title":"clear"},"notas":[]}',
    });
    const response = await handler({ url, request } as APIContext);
    expect(committed).toBe(true);
    expect(calls).toBe(1);
    expect(translated).toBe(0);
    expect(response.status).toBe(502);
    expect(await response.json()).toEqual({ error: 'import_result_unknown' });
  });
}

test('preview failures keep their normal error translation', async () => {
  const send = async () => {
    throw new Error('unavailable');
  };
  const handler = createNotebookImportHandler({
    conCliente: async () => ({ client: { previewNotebook: send, importNotebook: send } }),
    json: (body: unknown, status = 200) => Response.json(body, { status }),
    fallo: () => Response.json({ error: 'upstream_error' }, { status: 503 }),
  });
  const url = new URL('https://example.test/api/notebooks/import?preview=1');
  const request = new Request(url, {
    method: 'POST',
    body: '{"notebook":{"title":"clear"},"notas":[]}',
  });
  const response = await handler({ url, request } as APIContext);
  expect(response.status).toBe(503);
  expect(await response.json()).toEqual({ error: 'upstream_error' });
});

// Un ZIP de Markdown es binario y no lleva cifrado: el BFF lo reenvía en bytes,
// intacto y con su nombre, sin pasarlo por la guarda del `.onex`.
test('a Markdown ZIP reaches the service byte for byte with its name', async () => {
  const zip = new Uint8Array([0x50, 0x4b, 0x03, 0x04, 0xff, 0x00, 0x9c]);
  let seen: { body: Uint8Array; name?: string | undefined } | undefined;
  const send = async (body: Uint8Array, name?: string) => {
    seen = { body, name };
    return {};
  };
  const handler = createNotebookImportHandler({
    conCliente: async () => ({ client: { previewNotebook: send, importNotebook: send } }),
    json: (body: unknown, status = 200) => Response.json(body, { status }),
    fallo: () => Response.json({ error: 'upstream_error' }, { status: 502 }),
  });
  const url = new URL('https://example.test/api/notebooks/import?preview=1&name=B%C3%B3veda.zip');
  const response = await handler({
    url,
    request: new Request(url, { method: 'POST', body: zip }),
  } as APIContext);
  expect(response.status).toBe(200);
  expect(seen?.name).toBe('Bóveda.zip');
  expect([...(seen?.body ?? [])]).toEqual([...zip]);
});
