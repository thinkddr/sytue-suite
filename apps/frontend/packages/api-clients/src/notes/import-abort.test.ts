// SPDX-License-Identifier: Apache-2.0
import { expect, test } from 'bun:test';
import { NotesClient } from './index.js';

for (const kind of ['preview', 'import'] as const) {
  test(`${kind} forwards the request signal and refuses an already aborted request`, async () => {
    let calls = 0;
    let start!: () => void;
    const started = new Promise<void>((resolve) => {
      start = resolve;
    });
    let received: AbortSignal | null | undefined;
    const fetchImpl = (async (_input: RequestInfo | URL, init?: RequestInit) => {
      calls++;
      received = init?.signal;
      start();
      const signal = init?.signal;
      return new Promise<Response>((_, reject) => {
        if (signal?.aborted) reject(signal.reason);
        else signal?.addEventListener('abort', () => reject(signal?.reason), { once: true });
      });
    }) as typeof fetch;
    const client = new NotesClient({
      baseUrl: 'https://notes.example.test',
      auth: { getAccessToken: async () => 'test-token' },
      fetchImpl,
    });
    const send = (signal: AbortSignal) =>
      kind === 'preview'
        ? client.previewNotebook('{}', undefined, signal)
        : client.importNotebook('{}', undefined, signal);
    const early = new AbortController();
    early.abort();
    await expect(send(early.signal)).rejects.toMatchObject({ name: 'AbortError' });
    expect(calls).toBe(0);

    const active = new AbortController();
    const pending = send(active.signal);
    await started;
    expect(received).toBe(active.signal);
    active.abort();
    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
    expect(calls).toBe(1);
  }, 2000);
}
