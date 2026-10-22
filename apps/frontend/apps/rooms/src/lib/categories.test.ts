// Checks de la agrupación del sidebar por categorías (Ola 11). Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { groupChannels, DEFAULT_FAVORITES, DEFAULT_CHANNELS, DEFAULT_DMS } from './categories.js';
import type { Channel, Category } from './chat.js';

function ch(p: Partial<Channel> & { id: string }): Channel {
  return { name: p.id, kind: 'public', ...p };
}

test('sin categorías personalizadas: reparte por favorito / kind', () => {
  const channels = [
    ch({ id: 'a', kind: 'public' }),
    ch({ id: 'b', kind: 'public', favorite: true }),
    ch({ id: 'c', kind: 'direct' }),
  ];
  const groups = groupChannels(channels, []);
  expect(groups.map((g) => g.key)).toEqual([DEFAULT_FAVORITES, DEFAULT_CHANNELS, DEFAULT_DMS]);
  expect(groups[0]!.channels.map((c) => c.id)).toEqual(['b']);
  expect(groups[1]!.channels.map((c) => c.id)).toEqual(['a']);
  expect(groups[2]!.channels.map((c) => c.id)).toEqual(['c']);
});

test('Favoritos se oculta cuando no hay favoritos; Canales/DMs siempre presentes', () => {
  const groups = groupChannels([ch({ id: 'a' })], []);
  expect(groups.map((g) => g.key)).toEqual([DEFAULT_CHANNELS, DEFAULT_DMS]);
});

test('una categoría personalizada gana sobre la sección por defecto según kind', () => {
  const cats: Category[] = [{ id: 'cat1', name: 'Proyectos', sortOrder: 0, collapsed: false }];
  const channels = [
    ch({ id: 'a', kind: 'public', categoryId: 'cat1' }),
    ch({ id: 'b', kind: 'direct', categoryId: 'cat1' }), // un DM movido a la categoría
    ch({ id: 'c', kind: 'public' }),
  ];
  const groups = groupChannels(channels, cats);
  const proyectos = groups.find((g) => g.key === 'cat1')!;
  expect(proyectos.channels.map((c) => c.id).sort()).toEqual(['a', 'b']);
  expect(groups.find((g) => g.key === DEFAULT_CHANNELS)!.channels.map((c) => c.id)).toEqual(['c']);
  // La categoría personalizada va entre Canales y Mensajes directos.
  expect(groups.map((g) => g.key)).toEqual([DEFAULT_CHANNELS, 'cat1', DEFAULT_DMS]);
});

test('categoryId de una categoría borrada recae a la sección por defecto', () => {
  const channels = [ch({ id: 'a', kind: 'public', categoryId: 'zombie' })];
  const groups = groupChannels(channels, []);
  expect(groups.find((g) => g.key === DEFAULT_CHANNELS)!.channels.map((c) => c.id)).toEqual(['a']);
});

test('el no-leído agregado excluye los canales silenciados; ordena por sortOrder', () => {
  const cats: Category[] = [{ id: 'cat1', name: 'C', sortOrder: 0, collapsed: false }];
  const channels = [
    ch({ id: 'a', categoryId: 'cat1', unread: 3, sortOrder: 2 }),
    ch({ id: 'b', categoryId: 'cat1', unread: 5, muted: true, sortOrder: 1 }),
  ];
  const g = groupChannels(channels, cats).find((x) => x.key === 'cat1')!;
  expect(g.unread).toBe(3); // el silenciado (5) no suma
  expect(g.channels.map((c) => c.id)).toEqual(['b', 'a']); // sortOrder 1 antes que 2
});

test('el no-leído agregado respeta notify_level: none no suma, mentions solo @menciones', () => {
  const channels = [
    ch({ id: 'a', unread: 4 }), // 'all' implícito → suma 4
    ch({ id: 'b', unread: 9, mentions: 2, notifyLevel: 'mentions' }), // solo menciones → 2
    ch({ id: 'c', unread: 7, notifyLevel: 'none', muted: true }), // none → 0
  ];
  const g = groupChannels(channels, []).find((x) => x.key === DEFAULT_CHANNELS)!;
  expect(g.unread).toBe(6); // 4 + 2 (menciones de b) + 0
});

test('las categorías personalizadas se ordenan por sortOrder y luego por nombre', () => {
  const cats: Category[] = [
    { id: 'c2', name: 'Beta', sortOrder: 1, collapsed: false },
    { id: 'c1', name: 'Alfa', sortOrder: 0, collapsed: false },
  ];
  const groups = groupChannels([], cats);
  expect(groups.map((g) => g.key)).toEqual([DEFAULT_CHANNELS, 'c1', 'c2', DEFAULT_DMS]);
});
