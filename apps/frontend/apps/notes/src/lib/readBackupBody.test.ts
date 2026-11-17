// SPDX-License-Identifier: Apache-2.0
import { expect, test } from 'bun:test';
import { readBackupBody } from './readBackupBody.js';

test('rejects and cancels chunked requests as soon as their byte budget is exceeded', async () => {
  let cancelled = false;
  let index = 0;
  const body = new ReadableStream<Uint8Array>({
    pull(controller) {
      if (index++ === 0) controller.enqueue(new Uint8Array([65, 66]));
      else if (index === 2) controller.enqueue(new Uint8Array([67, 68]));
      else controller.enqueue(new Uint8Array([69]));
    },
    cancel() {
      cancelled = true;
    },
  });
  const request = new Request('https://example.test', {
    method: 'POST',
    body,
    duplex: 'half',
  } as RequestInit);
  await expect(readBackupBody(request, 3)).rejects.toThrow();
  expect(cancelled).toBe(true);
});

test('decodes split UTF-8 but rejects invalid sequences', async () => {
  const stream = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new Uint8Array([0xc3]));
      controller.enqueue(new Uint8Array([0xb1]));
      controller.close();
    },
  });
  const split = new Request('https://example.test', {
    method: 'POST',
    body: stream,
    duplex: 'half',
  } as RequestInit);
  const request = (bytes: number[]) =>
    new Request('https://example.test', { method: 'POST', body: new Uint8Array(bytes) });
  await expect(readBackupBody(split, 2)).resolves.toBe('ñ');
  await expect(readBackupBody(request([0xff]), 2)).rejects.toThrow();
});

test('decodes many tiny chunks and a character crossing an internal block boundary', async () => {
  const prefix = 64 * 1024 - 1;
  const tiny = 4096;
  let index = 0;
  const stream = new ReadableStream<Uint8Array>({
    pull(controller) {
      if (index === 0) controller.enqueue(new Uint8Array(prefix - tiny).fill(97));
      else if (index <= tiny) controller.enqueue(new Uint8Array([97]));
      else if (index === tiny + 1) controller.enqueue(new Uint8Array([0xc3]));
      else if (index === tiny + 2) controller.enqueue(new Uint8Array([0xb1]));
      else {
        controller.close();
        return;
      }
      index++;
    },
  });
  const request = new Request('https://example.test', {
    method: 'POST',
    body: stream,
    duplex: 'half',
  } as RequestInit);
  await expect(readBackupBody(request, prefix + 2)).resolves.toBe('a'.repeat(prefix) + 'ñ');
});

for (const fragmentSize of [0, 1]) {
  test(`accepts exactly the fragment limit with ${fragmentSize}-byte fragments`, async () => {
    let pulls = 0;
    const body = new ReadableStream<Uint8Array>({
      pull(controller) {
        if (pulls++ === 16 * 1024) {
          controller.close();
          return;
        }
        controller.enqueue(new Uint8Array(fragmentSize).fill(97));
      },
    });
    const request = new Request('https://example.test', {
      method: 'POST',
      body,
      duplex: 'half',
    } as RequestInit);
    await expect(readBackupBody(request)).resolves.toBe('a'.repeat(fragmentSize * 16 * 1024));
  });

  test(`rejects and cancels excessive ${fragmentSize}-byte fragments`, async () => {
    let pulls = 0;
    let cancelled = false;
    const body = new ReadableStream<Uint8Array>({
      pull(controller) {
        pulls++;
        controller.enqueue(new Uint8Array(fragmentSize));
      },
      cancel() {
        cancelled = true;
      },
    });
    const request = new Request('https://example.test', {
      method: 'POST',
      body,
      duplex: 'half',
    } as RequestInit);
    await expect(readBackupBody(request)).rejects.toThrow('too many backup fragments');
    expect(cancelled).toBe(true);
    expect(pulls).toBeLessThanOrEqual(16 * 1024 + 2);
  });
}

test('accepts exactly 32 MiB and rejects the next byte', async () => {
  const max = 32 * 1024 * 1024;
  const stream = (extra: boolean) => {
    let step = 0;
    return new ReadableStream<Uint8Array>({
      pull(controller) {
        if (step++ === 0) controller.enqueue(new Uint8Array(max).fill(97));
        else if (extra && step === 2) controller.enqueue(new Uint8Array([98]));
        else controller.close();
      },
    });
  };
  const request = (extra: boolean) =>
    new Request('https://example.test', {
      method: 'POST',
      body: stream(extra),
      duplex: 'half',
    } as RequestInit);
  const exact = await readBackupBody(request(false));
  expect(exact.length).toBe(max);
  expect(exact[0]).toBe('a');
  expect(exact[max - 1]).toBe('a');
  await expect(readBackupBody(request(true))).rejects.toThrow('backup too large');
});

test('rejects without waiting for a cancellation that never resolves', async () => {
  let cancelled = false;
  const body = new ReadableStream<Uint8Array>({
    start(controller) {
      controller.enqueue(new Uint8Array([65, 66]));
    },
    cancel() {
      cancelled = true;
      return new Promise<void>(() => {});
    },
  });
  const request = new Request('https://example.test', {
    method: 'POST',
    body,
    duplex: 'half',
  } as RequestInit);
  let timer: ReturnType<typeof setTimeout> | undefined;
  try {
    const result = await Promise.race([
      readBackupBody(request, 1).then(
        () => 'accepted',
        () => 'rejected',
      ),
      new Promise<string>((resolve) => {
        timer = setTimeout(() => resolve('timed out'), 1000);
      }),
    ]);
    expect(result).toBe('rejected');
    expect(cancelled).toBe(true);
  } finally {
    if (timer !== undefined) clearTimeout(timer);
  }
});

test('absolute deadline rejects a stalled read without waiting for cancellation', async () => {
  let cancelled = false;
  const body = new ReadableStream<Uint8Array>({
    pull() {
      return new Promise<void>(() => {});
    },
    cancel() {
      cancelled = true;
      return new Promise<void>(() => {});
    },
  });
  const request = new Request('https://example.test', {
    method: 'POST',
    body,
    duplex: 'half',
  } as RequestInit);
  await expect(readBackupBody(request, 1024, 20)).rejects.toThrow('deadline');
  expect(cancelled).toBe(true);
  expect(request.body?.locked).toBe(false);
}, 2000);

test('request abort rejects a stalled read and releases the reader', async () => {
  const controller = new AbortController();
  let cancelled = false;
  let entered!: () => void;
  const reading = new Promise<void>((resolve) => {
    entered = resolve;
  });
  const body = new ReadableStream<Uint8Array>({
    pull() {
      entered();
      return new Promise<void>(() => {});
    },
    cancel() {
      cancelled = true;
      return new Promise<void>(() => {});
    },
  });
  const request = new Request('https://example.test', {
    method: 'POST',
    body,
    signal: controller.signal,
    duplex: 'half',
  } as RequestInit);
  const pending = readBackupBody(request, 1024, 1000);
  await reading;
  controller.abort();
  await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
  expect(cancelled).toBe(true);
  expect(request.body?.locked).toBe(false);
}, 2000);
