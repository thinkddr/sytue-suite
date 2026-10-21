// Checks del roster de canal (Ola 2C): reconciliación WS, buscador y candidatos de
// «Añadir personas». Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { upsertMember, filterMembers, addableMembers } from './roster.js';
import type { ChannelMember } from './chat.js';

function m(sub: string, name: string, role?: 'admin' | 'member'): ChannelMember {
  return { sub, name, ...(role ? { role } : {}) };
}

const roster = [m('u1', 'Ana', 'admin'), m('u2', 'Bruno', 'member')];

test('upsert añade en el sitio que le toca por nombre', () => {
  expect(upsertMember(roster, m('u3', 'Aurora', 'member')).map((x) => x.name)).toEqual([
    'Ana',
    'Aurora',
    'Bruno',
  ]);
});

test('upsert de alguien que ya está reemplaza la fila (cambio de rol, no duplica)', () => {
  const next = upsertMember(roster, m('u2', 'Bruno', 'admin'));
  expect(next).toHaveLength(2);
  expect(next.find((x) => x.sub === 'u2')?.role).toBe('admin');
});

test('el orden ignora acentos y mayúsculas (como lower(name) del backend)', () => {
  const next = upsertMember([m('u1', 'Zoe'), m('u2', 'ana')], m('u3', 'Ángel'));
  expect(next.map((x) => x.name)).toEqual(['ana', 'Ángel', 'Zoe']);
});

test('el buscador filtra por nombre o por subject; vacío no filtra', () => {
  expect(filterMembers(roster, 'bru').map((x) => x.sub)).toEqual(['u2']);
  expect(filterMembers(roster, 'U1').map((x) => x.sub)).toEqual(['u1']);
  expect(filterMembers(roster, '  ')).toHaveLength(2);
  expect(filterMembers(roster, 'nadie')).toHaveLength(0);
});

test('candidatos: los del equipo que NO están ya en el canal', () => {
  const team = [m('u1', 'Ana', 'admin'), m('u2', 'Bruno'), m('u3', 'Carla'), m('u4', 'Diego')];
  expect(addableMembers(team, roster, '').map((x) => x.sub)).toEqual(['u3', 'u4']);
  expect(addableMembers(team, roster, 'car').map((x) => x.sub)).toEqual(['u3']);
  // Todo el equipo dentro → nada que añadir (el panel pinta su estado vacío).
  expect(addableMembers(team, team, '')).toHaveLength(0);
});
