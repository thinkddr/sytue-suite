import { expect, test } from 'bun:test';
import { importFreeGif, uploadAttachment, type FreeGif } from './chatApi.js';

const gif: FreeGif = {
  id: 123,
  title: 'Dance',
  source: 'https://commons.wikimedia.org/w/index.php?curid=123',
  license: 'CC0 1.0',
  width: 4,
  height: 4,
  size: 50,
};

test('cancelar la descarga no sube ni registra el GIF', async () => {
  const previous = globalThis.fetch;
  const controller = new AbortController();
  const calls: string[] = [];
  globalThis.fetch = (async (url: RequestInfo | URL, init?: RequestInit) => {
    calls.push(String(url));
    expect(init?.signal).toBe(controller.signal);
    if (String(url) === '/api/emoji') return Response.json({ emoji: [] });
    controller.abort();
    // Aunque la respuesta llegue tarde, no debe empezar la subida siguiente.
    return new Response('gif');
  }) as unknown as typeof fetch;
  try {
    await expect(importFreeGif(gif, controller.signal)).rejects.toThrow();
    expect(calls).toEqual(['/api/emoji', '/api/gifs?id=123']);
  } finally {
    globalThis.fetch = previous;
  }
});

test('el catálogo puede reutilizar un GIF existente sin enviar bytes al uploader en claro', async () => {
  const previousFetch = globalThis.fetch;
  const previousXhr = globalThis.XMLHttpRequest;
  const names: string[] = [];
  const deleted: string[] = [];
  let listed = 0;
  const uploadedId = 'bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb';
  class Upload {
    status = 201;
    response = {
      file: { fileId: uploadedId, name: 'gif', contentType: 'image/gif', sizeBytes: 3 },
    };
    upload = {};
    onload: (() => void) | null = null;
    onabort: (() => void) | null = null;
    open(_method: string, url: string): void {
      names.push(new URL(url, 'https://rooms.test').searchParams.get('name') ?? '');
    }
    setRequestHeader(): void {}
    send(): void {
      this.onload?.();
    }
    abort(): void {
      this.onabort?.();
    }
  }
  globalThis.XMLHttpRequest = Upload as unknown as typeof XMLHttpRequest;
  globalThis.fetch = (async (url: RequestInfo | URL, init?: RequestInit) => {
    const path = String(url);
    if (init?.method === 'DELETE') {
      deleted.push(path);
      return new Response(null, { status: 204 });
    }
    if (path.startsWith('/api/gifs')) return new Response('gif');
    if (init?.method === 'POST') return new Response(null, { status: 409 });
    return Response.json({
      emoji: ++listed === 1 ? [] : [{ name: 'commons-123', fileId: 'winner' }],
    });
  }) as unknown as typeof fetch;
  try {
    const result = await importFreeGif(gif);
    expect(result.fileId).toBe('winner');
    expect(names).toEqual([]);
    expect(deleted).toEqual([]);
  } finally {
    globalThis.fetch = previousFetch;
    globalThis.XMLHttpRequest = previousXhr;
  }
});

test('una subida ya cancelada nunca envía bytes', async () => {
  const previousXhr = globalThis.XMLHttpRequest;
  let sent = false;
  class Upload {
    upload = {};
    open(): void {}
    setRequestHeader(): void {}
    send(): void {
      sent = true;
    }
  }
  globalThis.XMLHttpRequest = Upload as unknown as typeof XMLHttpRequest;
  const controller = new AbortController();
  controller.abort();
  try {
    await expect(
      uploadAttachment(new File(['gif'], 'gif.gif'), { signal: controller.signal }),
    ).rejects.toThrow('aborted');
    expect(sent).toBe(false);
  } finally {
    globalThis.XMLHttpRequest = previousXhr;
  }
});
