/**
 * Self-check de la lógica de la paleta ⌘K: el filtrado por tokens (el índice activo
 * lo prueba PaletaDeComandos.test.tsx en @sytue/ui). Sin DOM ni framework. Vive fuera de src/ para no entrar en el
 * `astro check`/eslint/prettier de la app. Ejecutar: `bun test` en cloud-dashboard.
 */
import { expect, test } from 'bun:test';
import { filterCommands } from '../src/lib/palette.js';

type Row = { label: string; keywords?: string; group: string };

const cmds: Row[] = [
  { label: 'Resumen', group: 'Navegación', keywords: 'overview' },
  { label: 'Despliegues', group: 'Navegación', keywords: 'deployments' },
  { label: 'Crear proyecto', group: 'Acciones', keywords: 'nuevo repositorio git' },
  { label: 'tienda-web', group: 'Proyectos', keywords: 'org/tienda' },
];

test('consulta vacía o en blanco devuelve todos, en orden', () => {
  expect(filterCommands(cmds, '')).toEqual(cmds);
  expect(filterCommands(cmds, '   ')).toEqual(cmds);
});

test('filtra por subcadena en label, sin distinguir mayúsculas', () => {
  expect(filterCommands(cmds, 'RESUM').map((c) => c.label)).toEqual(['Resumen']);
});

test('coincide por keywords ocultas y por nombre de grupo', () => {
  expect(filterCommands(cmds, 'git').map((c) => c.label)).toEqual(['Crear proyecto']);
  expect(filterCommands(cmds, 'acciones').map((c) => c.label)).toEqual(['Crear proyecto']);
});

test('multi-token es AND: todas las palabras deben aparecer', () => {
  expect(filterCommands(cmds, 'crear git').map((c) => c.label)).toEqual(['Crear proyecto']);
  expect(filterCommands(cmds, 'crear despliegues')).toEqual([]);
});

test('sin coincidencias devuelve lista vacía', () => {
  expect(filterCommands(cmds, 'zzz')).toEqual([]);
});
