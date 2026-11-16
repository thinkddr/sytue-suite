import { expect, test } from 'bun:test';
import { filterCommands } from './filtrarComandos.js';

const cmds = [
  { label: 'Salas de reunión', group: 'Secciones' },
  { label: 'Crear grupo', group: 'Acciones', keywords: 'nuevo equipo' },
];

test('pliega acentos y mayúsculas, y exige todas las palabras', () => {
  expect(filterCommands(cmds, 'REUNION').map((c) => c.label)).toEqual(['Salas de reunión']);
  expect(filterCommands(cmds, 'nuevo acciones').map((c) => c.label)).toEqual(['Crear grupo']);
  expect(filterCommands(cmds, 'nuevo secciones')).toEqual([]);
  expect(filterCommands(cmds, '  ')).toEqual(cmds);
});
