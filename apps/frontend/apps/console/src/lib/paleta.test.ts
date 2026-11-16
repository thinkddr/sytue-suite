/**
 * La paleta ⌘K de Sytue Admin: qué ofrece a quién y que el filtrado (el de
 * @sytue/ui, compartido con Sytue Cloud) pliega acentos. El teclado y el foco los
 * prueba PaletaDeComandos.test.tsx en @sytue/ui.
 */
import { afterEach, expect, test } from 'bun:test';
import { filterCommands } from '@sytue/ui';
import { NAV } from '../components/ConsoleShell.js';
import { en } from '../i18n/messages/en.js';
import { es } from '../i18n/messages/es.js';
import { allowedSections } from './sections.js';
import { accionPedida, comandosDeConsola, type Textos } from './paleta.js';

const textos = (m: Record<string, unknown>): Textos => ({
  grupoSecciones: 'Secciones',
  grupoAcciones: 'Acciones',
  grupoPersonas: 'Personas',
  accion: (a) => ({
    label: String(m[`palette.action.${a}`]),
    keywords: String(m[`palette.action.${a}.keywords`]),
  }),
});
const nav = NAV.map((n) => ({ ...n, label: String(es[n.labelKey]) }));

test('un admin delegado solo ve sus secciones y las acciones de ellas', () => {
  const allowed = allowedSections(['custom'], ['groups.manage']);
  const ids = comandosDeConsola(nav, allowed, textos(es)).map((c) => c.id);
  expect(ids).toEqual(['seccion:groups', 'accion:crear-grupo']);
});

test('owner/admin (allowed ausente) ve todo el menú y las cuatro acciones', () => {
  const cmds = comandosDeConsola(nav, undefined, textos(es));
  expect(cmds.filter((c) => c.id.startsWith('seccion:'))).toHaveLength(NAV.length);
  expect(cmds.filter((c) => c.id.startsWith('accion:')).map((c) => c.href)).toEqual([
    '/users?accion=invitar',
    '/groups?accion=crear-grupo',
    '/apps?accion=conectar-app',
    '/sso?accion=conectar-sso',
  ]);
});

test('personas solo si puede ver usuarios, y llevan al directorio buscando su correo', () => {
  const p = [{ id: 'u1', fullName: 'Ana Pérez', email: 'ana+x@acme.eu' }];
  expect(
    comandosDeConsola(nav, ['groups'], textos(es), p).some((c) => c.group === 'Personas'),
  ).toBe(false);
  const ana = comandosDeConsola(nav, ['users'], textos(es), p).find((c) => c.id === 'persona:u1');
  expect(ana?.href).toBe('/users?q=ana%2Bx%40acme.eu');
});

test('«reunion» encuentra la acción con acento y «invite» la de inglés', () => {
  const filas = [
    ...comandosDeConsola(nav, undefined, textos(es)),
    { id: 'x', group: 'Secciones', label: 'Salas de reunión', icon: 'grid' as const, href: '/x' },
  ];
  expect(filterCommands(filas, 'reunion').map((c) => c.label)).toEqual(['Salas de reunión']);
  expect(filterCommands(filas, 'AUDITORIA').map((c) => c.id)).toEqual(['seccion:audit']);
  expect(filterCommands(filas, 'grupo nuevo').map((c) => c.id)).toEqual(['accion:crear-grupo']);
  const ingles = comandosDeConsola(nav, undefined, textos(en));
  expect(filterCommands(ingles, 'invite').map((c) => c.id)).toEqual(['accion:invitar']);
});

const g = globalThis as { window?: unknown };
afterEach(() => {
  delete g.window;
});

test('accionPedida abre una vez y limpia la marca de la barra', () => {
  let barra = '';
  g.window = {
    location: { href: 'https://admin.example/admin/acme/groups?accion=crear-grupo&x=1#h' },
    history: { replaceState: (_s: unknown, _t: string, u: string) => (barra = u) },
  };
  expect(accionPedida('invitar')).toBe(false);
  expect(accionPedida('crear-grupo')).toBe(true);
  expect(barra).toBe('/admin/acme/groups?x=1#h');
});
