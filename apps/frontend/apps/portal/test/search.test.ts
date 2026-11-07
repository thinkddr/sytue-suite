// Self-check de groupBySource: agrupa por producto preservando el orden global
// de aparición (el Hub rankea; el primer producto que aparece encabeza).
// Ejecutar: cd apps/frontend/apps/portal && bun test
// (Fuera de src/ a propósito: astro check no typechequea este fichero.)
import { expect, test } from 'bun:test';
import {
  cargaSinNovedad,
  enlaceCargarMas,
  groupBySource,
  hayMas,
  MAX_PAGINAS,
  paginasHastaElFallo,
  paginasPedidas,
  POR_PAGINA,
  totalAMostrar,
  unirPaginas,
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

test('unirPaginas pega en orden, sin repetir, y la última dice si quedan', () => {
  const u = unirPaginas([
    pagina(['a', 'b'], 5, true, ['drive']),
    pagina(['b', 'c', 'd'], 5, false, ['drive', 'rooms']),
  ]);
  expect(u.results.map((x) => x.id)).toEqual(['a', 'b', 'c', 'd']);
  expect(u.total).toBe(5);
  expect(u.hasMore).toBe(false);
  expect(u.fallidos.map((f) => f.source)).toEqual(['drive', 'rooms']);
});

test('hayMas: lo dice el Hub, y nunca pasado el tope de páginas', () => {
  expect(hayMas(pagina(['a'], 1, false), 1)).toBe(false);
  expect(hayMas(pagina(['a'], 1, true), 1)).toBe(true);
  // Sin `has_more` pero con un total mayor que lo PEDIDO: también quedan.
  expect(hayMas(pagina(['a'], 40, false), 1)).toBe(true);
  expect(hayMas(pagina(['a'], 400, true), MAX_PAGINAS)).toBe(false);
});

test('hayMas: los repetidos quitados no dejan el botón puesto para siempre', () => {
  // Dos páginas pedidas (40), un repetido entre ellas: se enseñan 39 de 40 y el Hub
  // dice que no quedan. Comparar el total con lo enseñado daba «hay más» sin fin.
  const ids = Array.from({ length: 2 * POR_PAGINA }, (_, i) => `x${i}`);
  const u = unirPaginas([
    pagina(ids.slice(0, POR_PAGINA), 2 * POR_PAGINA, true),
    pagina([ids[POR_PAGINA - 1]!, ...ids.slice(POR_PAGINA + 1)], 2 * POR_PAGINA, false),
  ]);
  expect(u.results.length).toBe(2 * POR_PAGINA - 1);
  expect(hayMas(u, 2)).toBe(false);
});

test('paginasHastaElFallo: una página que falla no tira lo de antes', () => {
  const ok = (ids: string[]) => ({ status: 'fulfilled' as const, value: pagina(ids, 9, true) });
  const ko = { status: 'rejected' as const, reason: new Error('timeout') };
  expect(paginasHastaElFallo([ko, ok(['b'])])).toBeNull();
  const corta = paginasHastaElFallo([ok(['a']), ko, ok(['c'])]);
  expect(corta?.paginas.map((p) => p.results[0]!.id)).toEqual(['a']);
  expect(corta?.cortada).toBe(true);
  expect(paginasHastaElFallo([ok(['a']), ok(['b'])])?.cortada).toBe(false);
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
  const ids = Array.from({ length: 2 * POR_PAGINA }, (_, i) => `x${i}`);
  const u = unirPaginas([
    pagina(ids.slice(0, POR_PAGINA), 2 * POR_PAGINA, true),
    pagina([ids[POR_PAGINA - 1]!, ...ids.slice(POR_PAGINA + 1)], 2 * POR_PAGINA, false),
  ]);
  expect(totalAMostrar(u, 2)).toBe(2 * POR_PAGINA - 1);
  // Quedan en el Hub: su total sigue siendo la verdad, también pasado el tope.
  expect(totalAMostrar(pagina(['a'], 143, true), 1)).toBe(143);
  expect(totalAMostrar(pagina(['a'], 400, true), MAX_PAGINAS)).toBe(400);
});

// Visto en ROJO a mano: sin la comparación con lo de antes, una página vacía con
// `has_more` (o toda repetidos) recargaba la pantalla igual y sin decir nada.
test('cargaSinNovedad: una página que falla o que no trae nada nuevo se dice', () => {
  const p1 = pagina(['a', 'b'], 9, true);
  expect(cargaSinNovedad({ paginas: [p1], cortada: false })).toBe(false);
  expect(cargaSinNovedad({ paginas: [p1], cortada: true })).toBe(true);
  expect(cargaSinNovedad({ paginas: [p1, pagina(['c'], 9, true)], cortada: false })).toBe(false);
  expect(cargaSinNovedad({ paginas: [p1, pagina([], 9, true)], cortada: false })).toBe(true);
  expect(cargaSinNovedad({ paginas: [p1, pagina(['b'], 9, true)], cortada: false })).toBe(true);
});
