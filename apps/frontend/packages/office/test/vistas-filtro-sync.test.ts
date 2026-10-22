// Checks de la sincronización de las vistas de filtro con el servidor y de la migración
// desde localStorage (sheets/vistasFiltroSync.ts). `fetch` y `storage` son falsos: lo que
// se comprueba es que NO se pierde nada (la copia local solo se borra con el servidor
// confirmado) y que el servidor recibe el estado unido.
import { expect, test } from 'bun:test';
import type { FilterViewsState } from '../src/editor/sheets/filtro';
import { mergeFilterViews } from '../src/editor/sheets/filtro';
import {
  cargarVistas,
  guardarVistas,
  type EntornoVistas,
} from '../src/editor/sheets/vistasFiltroSync';

const v = (id: string, name = id) => ({ id, name, excl: { '1': ['x'] } });
const est = (views: FilterViewsState['views'], active: Record<string, string> = {}) => ({
  views,
  active,
});

/** Un servidor de mentira con UNA fila por persona, un buzón local y la opción de caerse. */
function banco(inicial: FilterViewsState | null, local: FilterViewsState | null = null) {
  const b = {
    servidor: inicial,
    caido: false,
    posts: [] as unknown[],
    almacen: new Map<string, string>(local ? [['k', JSON.stringify(local)]] : []),
  };
  const io: EntornoVistas = {
    fetch: async (_u: string, init?: RequestInit) => {
      if (b.caido) throw new Error('sin red');
      if (init?.method === 'POST') {
        const { state } = JSON.parse(init.body as string) as { state: FilterViewsState };
        b.posts.push(state);
        b.servidor = state;
        return new Response(null, { status: 204 });
      }
      return Response.json({ state: b.servidor });
    },
    storage: {
      getItem: (k) => b.almacen.get(k) ?? null,
      setItem: (k, x) => void b.almacen.set(k, x),
      removeItem: (k) => void b.almacen.delete(k),
    },
  };
  return { b, io };
}

test('migración: lo que había en localStorage sube al servidor y solo entonces se borra', async () => {
  const local = est({ Hoja1: [v('a'), v('b')] }, { Hoja1: 'b' });
  const { b, io } = banco(null, local);
  const got = await cargarVistas(io, 'doc', 'k');
  expect(got).toEqual(local);
  expect(b.servidor).toEqual(local);
  expect(b.almacen.has('k')).toBe(false);
});

test('migración sin red: no se pierde nada, el buzón se queda y se sube en la siguiente carga', async () => {
  const local = est({ Hoja1: [v('a')] });
  const { b, io } = banco(null, local);
  b.caido = true;
  expect(await cargarVistas(io, 'doc', 'k')).toBeNull();
  expect(b.almacen.has('k')).toBe(true);
  b.caido = false;
  await cargarVistas(io, 'doc', 'k');
  expect(b.servidor).toEqual(local);
  expect(b.almacen.has('k')).toBe(false);
});

test('la subida falla (500): el buzón local se conserva', async () => {
  const local = est({ Hoja1: [v('a')] });
  const { b, io } = banco(est({}), local);
  const base = io.fetch;
  io.fetch = async (u: string, init?: RequestInit) =>
    init?.method === 'POST' ? new Response('x', { status: 500 }) : base(u, init);
  const got = await cargarVistas(io, 'doc', 'k');
  expect(got?.views['Hoja1']).toHaveLength(1); // se ve igualmente
  expect(b.almacen.has('k')).toBe(true);
});

test('servidor y local se unen id a id: el servidor manda en un conflicto', async () => {
  const servidor = est({ Hoja1: [v('a', 'servidor')] }, { Hoja1: 'a' });
  const local = est({ Hoja1: [v('a', 'local'), v('b')], Hoja2: [v('c')] }, { Hoja2: 'c' });
  const { b, io } = banco(servidor, local);
  const got = await cargarVistas(io, 'doc', 'k');
  expect(got?.views['Hoja1']?.map((x) => x.name)).toEqual(['servidor', 'b']);
  expect(got?.views['Hoja2']?.map((x) => x.id)).toEqual(['c']);
  expect(got?.active).toEqual({ Hoja1: 'a', Hoja2: 'c' });
  expect(b.servidor).toEqual(got as FilterViewsState);
});

test('sin buzón local no se sube nada al cargar', async () => {
  const servidor = est({ Hoja1: [v('a')] });
  const { b, io } = banco(servidor);
  expect(await cargarVistas(io, 'doc', 'k')).toEqual(servidor);
  expect(b.posts).toHaveLength(0);
});

test('guardar escribe primero en el buzón y lo vacía al confirmarse', async () => {
  const { b, io } = banco(null);
  const st = est({ Hoja1: [v('a')] });
  b.caido = true;
  expect(await guardarVistas(io, 'doc', 'k', st)).toBe(false);
  expect(JSON.parse(b.almacen.get('k') ?? 'null')).toEqual(st);
  b.caido = false;
  expect(await guardarVistas(io, 'doc', 'k', st)).toBe(true);
  expect(b.almacen.has('k')).toBe(false);
  expect(b.servidor).toEqual(st);
});

test('una respuesta del servidor con basura no tumba la carga', async () => {
  const { io } = banco(null);
  io.fetch = async () => Response.json({ state: { views: 'x', active: 3 } });
  expect(await cargarVistas(io, 'doc', 'k')).toEqual({ views: {}, active: {} });
});

test('la unión respeta el tope de 50 vistas por hoja', () => {
  const muchas = (p: string) => Array.from({ length: 40 }, (_, i) => v(`${p}${i}`));
  const m = mergeFilterViews(est({ H: muchas('s') }), est({ H: muchas('l') }));
  expect(m.views['H']).toHaveLength(50);
});

test('una vista activa que ya no existe no queda activa', () => {
  const m = mergeFilterViews(est({}, { H: 'zz' }), est({ H: [v('a')] }, { H: 'gone' }));
  expect(m.active).toEqual({});
});
