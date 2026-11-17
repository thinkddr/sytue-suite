// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { beforeEach, expect, mock, test } from 'bun:test';
import type { Note, NoteInput, Notebook } from '@sytue/api-clients';

// IndexedDB no existe en bun: el almacén de @sytue/offline, en memoria y con la
// misma forma (dueño + espacio + id).
const mem = new Map<string, { value: unknown; updatedAt: number }>();
const k = (o: string, n: string, id: string) => `${o}|${n}|${id}`;
// Con el resto del módulo real: el mock vale para todo el proceso de bun y otros
// tests (encrypted-workflow, vía @sytue/shell) importan isNativeShell de aquí.
const real = await import('@sytue/offline');
mock.module('@sytue/offline', () => ({
  ...real,
  readOfflineValue: async (o: string, n: string, id: string) => mem.get(k(o, n, id))?.value ?? null,
  writeOfflineValue: async (o: string, n: string, id: string, value: unknown) => {
    mem.set(k(o, n, id), { value, updatedAt: Date.now() });
  },
  deleteOfflineValue: async (o: string, n: string, id: string) => {
    mem.delete(k(o, n, id));
  },
  mutateOfflineValue: async (o: string, n: string, id: string, f: (p: unknown) => unknown) => {
    const next = f(mem.get(k(o, n, id))?.value ?? null);
    if (next === null) mem.delete(k(o, n, id));
    else mem.set(k(o, n, id), { value: next, updatedAt: Date.now() });
  },
  listOfflineValues: async (o: string, n: string) =>
    [...mem.entries()]
      .filter(([key]) => key.startsWith(`${o}|${n}|`))
      .map(([key, value]) => ({ ...value, id: key.slice(`${o}|${n}|`.length) })),
}));

const cola = await import('./cola.js');

beforeEach(() => mem.clear());

test('legacy encrypted capture with clear links remains rejected without any network call', async () => {
  const input: NoteInput = {
    content_cipher: 'encv1:synthetic',
    cipher_version: 2,
    links: [{ target_kind: 'docs_document', target_id: 'secret-id', label: 'private' }],
  };
  await cola.encolar('ada', { client_id: 'unsafe', date: '2026-10-02', input, at: 1 });
  const { e, enviadas, dias } = envio();
  await cola.vaciar('ada', e);
  const pending = await cola.pendientes('ada');
  expect(enviadas).toEqual([]);
  expect(dias).toEqual([]);
  expect(pending).toHaveLength(1);
  expect(pending[0]?.rechazada).toBe(true);
  expect(pending[0]?.input).toEqual(input);
});

test('unsafe legacy shapes remain in the journal without daily or note requests', async () => {
  const inputs: unknown[] = [
    {
      cipher_version: 2,
      content_cipher: 'encv1:old',
      Links: [{ target_kind: 'note', target_id: 'secret' }],
    },
    {
      cipher_version: 2,
      content_cipher: 'encv1:old',
      links: [],
      Links: [{ target_kind: 'note', target_id: 'secret' }],
    },
    { Cipher_Version: 2, content_cipher: 'encv1:old', links: [] },
    { cipher_version: 2, content_cipher: 'encv1:old', linKs: [{ target_id: 'secret' }] },
    { cipher_version: 2, content_cipher: 'encv1:old', linkſ: [{ target_id: 'secret' }] },
    { cipher_version: 2, content_cipher: 'encv1:old', links: { label: 'secret' } },
    { cipher_version: 2, content_cipher: 'encv1:old', links: [null] },
    { cipher_version: 2, content_cipher: 'encv1:old', data: { attachment: 'secret' } },
    { cipher_version: 2, content_cipher: 'encv1:old', expected_crypto: { secret: 'private' } },
    { cipher_version: 2, content_cipher: 'encv1:old', title: 'secret' },
    { cipher_version: 2, content_cipher: 'encv1:old', unknown: 'secret' },
    { cipher_version: '2', content_cipher: 'encv1:old' },
    { cipher_version: 2, content_cipher: 'encv1:old', checked: 'yes' },
    { content: '\ud800' },
    { data: { attachment: { title: '\ud800' } } },
    { data: { attachment: { id: Number.MAX_SAFE_INTEGER + 1 } } },
    { cipher_version: 2, content_cipher: 'encv1:old', data: { client_id: '\udc00' } },
    null,
  ];
  for (const [index, input] of inputs.entries()) {
    await cola.encolar('ada', {
      client_id: String(index),
      date: '2026-10-02',
      input: input as NoteInput,
      at: index,
    });
  }
  const { e, enviadas, dias } = envio();
  await cola.vaciar('ada', e);
  const pending = await cola.pendientes('ada');
  expect(enviadas).toEqual([]);
  expect(dias).toEqual([]);
  expect(pending.map((entry) => entry.rechazada)).toEqual(inputs.map(() => true));
  expect(pending.map((entry): unknown => entry.input)).toEqual(inputs);
  for (const [index, input] of inputs.entries()) {
    expect(mem.get(k('ada', 'notes.capturas', String(index)))?.value).toEqual({
      client_id: String(index),
      date: '2026-10-02',
      input,
      at: index,
    });
  }
});

test('capture record, capture ID and payload ID must match without rewriting the source', async () => {
  const captures = [
    { client_id: 'other', input: { data: { client_id: 'x' } }, at: 1 },
    { client_id: 'x', input: { data: { client_id: 'other' } }, at: 1 },
    { client_id: 'x', input: { data: {} }, at: 1 },
    { client_id: 'x', input: { data: { client_id: 'x', Client_ID: 'other' } }, at: 1 },
  ];
  for (const capture of captures) {
    mem.set(k('ada', 'notes.capturas', 'x'), { value: capture, updatedAt: 1 });
    const { e, enviadas, dias } = envio();
    await cola.vaciar('ada', e);
    expect(enviadas).toEqual([]);
    expect(dias).toEqual([]);
    expect((await cola.pendientes('ada'))[0]?.rechazada).toBe(true);
    expect(mem.get(k('ada', 'notes.capturas', 'x'))?.value).toEqual(capture);
  }
});

const nota = (id: string): Note => ({ id }) as Note;
const texto = (s: string): NoteInput => ({ kind: 'text', content: `<p>${s}</p>` });

function envio(fallos: Record<string, unknown> = {}) {
  const enviadas: [string, NoteInput][] = [];
  const dias: string[] = [];
  return {
    enviadas,
    dias,
    e: {
      hoy: '2026-10-02',
      createNote: async (nb: string, input: NoteInput) => {
        const f = fallos[String(input.data?.client_id)];
        if (f) throw f;
        enviadas.push([nb, input]);
        return nota(`n-${enviadas.length}`);
      },
      dailyNotebook: async (date: string, _t: string, carry: boolean) => {
        dias.push(`${date}:${carry}`);
        return { id: `nb-${date}` } as Notebook;
      },
    },
  };
}

test('el replay pausado conserva capturas y no invoca la red', async () => {
  await cola.encolar('ada', {
    client_id: 'b',
    notebook_id: 'nb1',
    input: cola.conClientId(texto('dos'), 'b'),
    at: 2,
  });
  await cola.encolar('ada', {
    client_id: 'a',
    notebook_id: 'nb1',
    input: cola.conClientId(texto('uno'), 'a'),
    at: 1,
  });
  const { e, enviadas } = envio();
  const tocados = await cola.vaciar('ada', e);
  expect(enviadas).toEqual([]);
  expect([...(tocados ?? [])]).toEqual([]);
  expect((await cola.pendientes('ada')).map((c) => c.client_id)).toEqual(['a', 'b']);
  await cola.vaciar('ada', e);
  expect(enviadas).toEqual([]);
});

test('un fallo de red para la cola y no pierde nada', async () => {
  await cola.encolar('ada', {
    client_id: 'a',
    notebook_id: 'nb1',
    input: cola.conClientId(texto('uno'), 'a'),
    at: 1,
  });
  await cola.encolar('ada', {
    client_id: 'b',
    notebook_id: 'nb1',
    input: cola.conClientId(texto('dos'), 'b'),
    at: 2,
  });
  const { e, enviadas } = envio({ a: new TypeError('Failed to fetch') });
  await cola.vaciar('ada', e);
  expect(enviadas).toEqual([]);
  expect((await cola.pendientes('ada')).map((c) => c.client_id)).toEqual(['a', 'b']);
});

test('el replay pausado conserva capturas aunque el servicio rechazara alguna', async () => {
  await cola.encolar('ada', {
    client_id: 'a',
    notebook_id: 'nb1',
    input: cola.conClientId(texto('uno'), 'a'),
    at: 1,
  });
  await cola.encolar('ada', {
    client_id: 'b',
    notebook_id: 'nb1',
    input: cola.conClientId(texto('dos'), 'b'),
    at: 2,
  });
  const caido = envio({ a: { status: 503 } });
  await cola.vaciar('ada', caido.e);
  expect(caido.enviadas).toEqual([]);
  const borrado = envio({ a: { status: 404 } });
  await cola.vaciar('ada', borrado.e);
  expect(borrado.enviadas).toEqual([]);
  const quedan = await cola.pendientes('ada');
  expect(quedan.map((c) => [c.client_id, c.rechazada])).toEqual([
    ['a', undefined],
    ['b', undefined],
  ]);
  const otra = envio();
  await cola.vaciar('ada', otra.e);
  expect(otra.enviadas).toEqual([]);
  await cola.descartar('ada', 'a');
  expect((await cola.pendientes('ada')).map((c) => c.client_id)).toEqual(['b']);
});

test('una captura de día no crea cuaderno mientras el replay está pausado', async () => {
  await cola.encolar('ada', {
    client_id: 'a',
    date: '2026-10-02',
    title: '2 de octubre',
    input: cola.conClientId(texto('uno'), 'a'),
    at: 1,
  });
  const { e, enviadas, dias } = envio();
  const tocados = await cola.vaciar('ada', e);
  expect(dias).toEqual([]);
  expect(enviadas).toEqual([]);
  expect(tocados?.size).toBe(0);
  expect((await cola.pendientes('ada')).map((c) => c.client_id)).toEqual(['a']);
});

test('la cola es de cada persona', async () => {
  await cola.encolar('ada', {
    client_id: 'a',
    notebook_id: 'nb1',
    input: cola.conClientId(texto('uno'), 'a'),
    at: 1,
  });
  expect(await cola.pendientes('bob')).toEqual([]);
});

test('la misma persona en otra organización no ve ni envía lo capturado en la primera', async () => {
  const a = cola.claveAlmacen('ada', 'org-a');
  const b = cola.claveAlmacen('ada', 'org-b');
  expect(a).not.toBe(b);
  await cola.encolar(a, { client_id: 'x', date: '2026-10-02', input: texto('día'), at: 1 });
  await cola.guardarCuadernos(a, [{ id: 'nb-a' } as Notebook]);
  const { e, enviadas, dias } = envio();
  await cola.vaciar(b, e);
  expect(enviadas).toEqual([]);
  expect(dias).toEqual([]);
  expect(await cola.pendientes(b)).toEqual([]);
  expect(await cola.leerCuadernos(b)).toBeNull();
  expect((await cola.pendientes(a)).map((c) => c.client_id)).toEqual(['x']);
});

test('conClientId conserva lo que ya traía data', () => {
  const i = cola.conClientId({ kind: 'link', data: { url: 'https://x.eu' } }, 'c1');
  expect(i.data).toEqual({ url: 'https://x.eu', client_id: 'c1' });
});

test('el día sin red: el conocido si lo hay; si no, uno provisional', () => {
  const conocido = { id: 'd1', daily_date: '2026-10-02', owner_sub: 'ada' } as Notebook;
  expect(cola.diaSinConexion([conocido], 'ada', '2026-10-02', 'x').id).toBe('d1');
  const nuevo = cola.diaSinConexion([conocido], 'ada', '2026-10-03', 'x');
  expect(nuevo.id).toBe('dia:2026-10-03');
  expect(nuevo.encrypted).toBe(false);
});

test('la caché de lectura guarda y devuelve cuadernos y notas por persona', async () => {
  await cola.guardarCuadernos('ada', [{ id: 'nb1' } as Notebook]);
  await cola.guardarNotas('ada', 'nb1', [nota('n1')]);
  expect((await cola.leerCuadernos('ada'))?.map((n) => n.id)).toEqual(['nb1']);
  expect((await cola.leerNotas('ada', 'nb1'))?.map((n) => n.id)).toEqual(['n1']);
  expect(await cola.leerNotas('bob', 'nb1')).toBeNull();
});
