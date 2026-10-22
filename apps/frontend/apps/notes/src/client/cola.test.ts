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
    [...mem.entries()].filter(([key]) => key.startsWith(`${o}|${n}|`)).map(([, v]) => v),
}));

const cola = await import('./cola.js');

beforeEach(() => mem.clear());

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

test('se envía en orden, cada una UNA vez, y la cola queda vacía', async () => {
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
  expect(enviadas.map(([, i]) => i.content)).toEqual(['<p>uno</p>', '<p>dos</p>']);
  expect([...(tocados ?? [])]).toEqual(['nb1']);
  expect(await cola.pendientes('ada')).toEqual([]);
  await cola.vaciar('ada', e);
  expect(enviadas).toHaveLength(2);
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

test('un 5xx también se reintenta; un 4xx se aparta y no bloquea a las demás', async () => {
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
  expect(borrado.enviadas.map(([, i]) => i.content)).toEqual(['<p>dos</p>']);
  const quedan = await cola.pendientes('ada');
  expect(quedan.map((c) => [c.client_id, c.rechazada])).toEqual([['a', true]]);
  // Una rechazada no se vuelve a intentar.
  const otra = envio();
  await cola.vaciar('ada', otra.e);
  expect(otra.enviadas).toEqual([]);
  await cola.descartar('ada', 'a');
  expect(await cola.pendientes('ada')).toEqual([]);
});

test('una captura de un día sin cuaderno abre el día al enviarla', async () => {
  await cola.encolar('ada', {
    client_id: 'a',
    date: '2026-10-02',
    title: '2 de octubre',
    input: cola.conClientId(texto('uno'), 'a'),
    at: 1,
  });
  const { e, enviadas, dias } = envio();
  const tocados = await cola.vaciar('ada', e);
  expect(dias).toEqual(['2026-10-02:true']);
  expect(enviadas[0]![0]).toBe('nb-2026-10-02');
  expect(tocados?.has('dia:2026-10-02')).toBe(true);
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
