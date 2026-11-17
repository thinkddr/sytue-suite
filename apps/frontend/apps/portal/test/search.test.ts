// Self-check de groupBySource: agrupa por producto preservando el orden global
// de aparición (el Hub rankea; el primer producto que aparece encabeza).
// Ejecutar: cd apps/frontend/apps/portal && bun test
// (Fuera de src/ a propósito: astro check no typechequea este fichero.)
import { expect, test } from 'bun:test';
import {
  buscarPaginas,
  cargaSinNovedad,
  enlaceCargarMas,
  groupBySource,
  hayMas,
  MAX_PAGINAS,
  paginasPedidas,
  POR_PAGINA,
  totalAMostrar,
} from '../src/lib/search.js';
import { rutaConOrg } from '@sytue/config';
import type { SearchResult } from '@sytue/api-clients';

const r = (source: string, id: string): SearchResult => ({
  source: source as SearchResult['source'],
  id,
  title: id,
  url: `/${id}`,
});

test('agrupa por producto y preserva el orden de aparición', () => {
  const groups = groupBySource([r('mail', 'a'), r('drive', 'b'), r('mail', 'c'), r('docs', 'd')]);
  expect(groups.map((g) => g.key)).toEqual(['mail', 'drive', 'docs']);
  expect(groups[0]!.items.map((i) => i.id)).toEqual(['a', 'c']);
  expect(groups[1]!.items.map((i) => i.id)).toEqual(['b']);
});

test('lista vacía → sin grupos', () => {
  expect(groupBySource([])).toEqual([]);
});

// «Cargar más»: la página es SSR, así que «más» es la misma URL con una página más.
const pagina = (ids: string[], total: number, hasMore: boolean, fallidos: string[] = []) => ({
  results: ids.map((id) => r('mail', id)),
  total,
  hasMore,
  fallidos: fallidos.map((s) => ({ source: s as SearchResult['source'], error: 'timeout' })),
});

test('paginasPedidas lee ?paginas= como dato no confiable', () => {
  expect(paginasPedidas(null)).toBe(1);
  expect(paginasPedidas('')).toBe(1);
  expect(paginasPedidas('0')).toBe(1);
  expect(paginasPedidas('-3')).toBe(1);
  expect(paginasPedidas('abc')).toBe(1);
  expect(paginasPedidas('3')).toBe(3);
  expect(paginasPedidas('999')).toBe(MAX_PAGINAS);
});

// Un Hub falso que cuenta las peticiones: cada una es una búsqueda federada a todos
// los orígenes, y antes «Cargar más» lanzaba una por página enseñada.
function hubFalso(fallaSi: (limit: number) => boolean = () => false) {
  const llamadas: { limit: number; offset: number }[] = [];
  return {
    llamadas,
    search: async (_q: string, _s: undefined, page: { limit: number; offset: number }) => {
      llamadas.push(page);
      if (fallaSi(page.limit)) throw new Error('hub caído');
      const ids = Array.from({ length: Math.min(page.limit, 250) }, (_, i) => `x${i}`);
      return pagina(ids, 250, page.limit < 250, ['drive']);
    },
  };
}

// Visto en ROJO a mano: con una petición por página, `?paginas=10` hacía diez.
test('buscarPaginas: las diez páginas en UNA petición al Hub', async () => {
  const hub = hubFalso();
  const b = await buscarPaginas(hub, 'informe', MAX_PAGINAS);
  expect(hub.llamadas).toEqual([{ limit: MAX_PAGINAS * POR_PAGINA, offset: 0 }]);
  expect(b.res.results.length).toBe(MAX_PAGINAS * POR_PAGINA);
  expect(b.paginas).toBe(MAX_PAGINAS);
  expect(b.cortada).toBe(false);
  // El aviso de orígenes caídos viaja con la respuesta.
  expect(b.res.fallidos.map((f) => f.source)).toEqual(['drive']);
  expect(cargaSinNovedad(b)).toBe(false);
});

test('buscarPaginas: un «Cargar más» que falla no tira lo de antes', async () => {
  const hub = hubFalso((limit) => limit > 2 * POR_PAGINA);
  const b = await buscarPaginas(hub, 'informe', 3);
  expect(hub.llamadas.map((l) => l.limit)).toEqual([3 * POR_PAGINA, 2 * POR_PAGINA]);
  expect(b.paginas).toBe(2);
  expect(b.res.results.length).toBe(2 * POR_PAGINA);
  expect(b.cortada).toBe(true);
  expect(cargaSinNovedad(b)).toBe(true);
  // Si falla la primera página, la búsqueda entera ha fallado.
  const caido = hubFalso(() => true);
  await expect(buscarPaginas(caido, 'informe', 1)).rejects.toThrow('hub caído');
  expect(caido.llamadas.length).toBe(1);
  // Si tampoco salen las que ya se veían, es la búsqueda entera la que ha fallado.
  const sinHub = hubFalso(() => true);
  await expect(buscarPaginas(sinHub, 'informe', 3)).rejects.toThrow('hub caído');
  expect(sinHub.llamadas.map((l) => l.limit)).toEqual([3 * POR_PAGINA, 2 * POR_PAGINA]);
});

test('hayMas: lo dice el Hub, y nunca pasado el tope de páginas', () => {
  expect(hayMas(pagina(['a'], 1, false), 1)).toBe(false);
  expect(hayMas(pagina(['a'], 1, true), 1)).toBe(true);
  // Sin `has_more` pero con un total mayor que lo PEDIDO: también quedan.
  expect(hayMas(pagina(['a'], 40, false), 1)).toBe(true);
  expect(hayMas(pagina(['a'], 400, true), MAX_PAGINAS)).toBe(false);
});

test('hayMas: lo enseñado por debajo de lo pedido no deja el botón puesto para siempre', () => {
  // Dos páginas pedidas (40): el Hub manda 39 de un total de 40 y dice que no quedan.
  // Comparar el total con lo enseñado daba «hay más» sin fin.
  const ids = Array.from({ length: 2 * POR_PAGINA - 1 }, (_, i) => `x${i}`);
  expect(hayMas(pagina(ids, 2 * POR_PAGINA, false), 2)).toBe(false);
});

test('enlaceCargarMas lleva el prefijo de la app y la org, y aterriza en el primero nuevo', () => {
  // Con `base: '/one'` un `/buscar` a pelo es el 404 de Traefik: el enlace sale de `r()`.
  const ruta = (p: string) => rutaConOrg('/one', 'acme', p);
  expect(enlaceCargarMas(ruta, 'informe & co', 1, 20)).toBe(
    '/one/acme/buscar?q=informe+%26+co&paginas=2#r-20',
  );
});

// Visto en ROJO a mano: devolviendo siempre `p.total`, el «39 de 40» se queda en pantalla
// sin botón para traer el 40, que no existe.
test('totalAMostrar: sin nada más en el Hub, lo enseñado es el total', () => {
  const ids = Array.from({ length: 2 * POR_PAGINA - 1 }, (_, i) => `x${i}`);
  expect(totalAMostrar(pagina(ids, 2 * POR_PAGINA, false), 2)).toBe(2 * POR_PAGINA - 1);
  // Quedan en el Hub: su total sigue siendo la verdad, también pasado el tope.
  expect(totalAMostrar(pagina(['a'], 143, true), 1)).toBe(143);
  expect(totalAMostrar(pagina(['a'], 400, true), MAX_PAGINAS)).toBe(400);
});

// Visto en ROJO a mano: sin la comparación con lo de antes, una página vacía con
// `has_more` recargaba la pantalla igual y sin decir nada.
test('cargaSinNovedad: una petición que falla o que no trae nada nuevo se dice', () => {
  const ids = (n: number) => Array.from({ length: n }, (_, i) => `x${i}`);
  const b = (n: number, paginas: number, cortada = false) => ({
    res: pagina(ids(n), 99, true),
    paginas,
    cortada,
  });
  expect(cargaSinNovedad(b(POR_PAGINA, 1))).toBe(false);
  expect(cargaSinNovedad(b(POR_PAGINA, 1, true))).toBe(true);
  expect(cargaSinNovedad(b(POR_PAGINA + 1, 2))).toBe(false);
  expect(cargaSinNovedad(b(POR_PAGINA, 2))).toBe(true);
});
