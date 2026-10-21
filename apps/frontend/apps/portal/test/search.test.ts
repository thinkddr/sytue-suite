// Self-check de groupBySource: agrupa por producto preservando el orden global
// de aparición (el Hub rankea; el primer producto que aparece encabeza).
// Ejecutar: cd apps/frontend/apps/portal && bun test
// (Fuera de src/ a propósito: astro check no typechequea este fichero.)
import { expect, test } from 'bun:test';
import { groupBySource } from '../src/lib/search.js';
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
