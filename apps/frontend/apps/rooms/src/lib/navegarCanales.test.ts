// Alt+↑/↓ y Alt+Mayús+↑/↓ entre canales (ROOM-16). Ejecutar: `bun test`.
import { expect, test } from 'bun:test';
import { DEFAULT_CHANNELS, DEFAULT_FAVORITES, groupChannels } from './categories.js';
import type { Category, Channel } from './chat.js';
import {
  canalVecino,
  ordenVisible,
  pasoDeNavegacion,
  type TeclaNavegar,
} from './navegarCanales.js';

function ch(p: Partial<Channel> & { id: string }): Channel {
  return { name: p.id, kind: 'public', ...p };
}

const cats: Category[] = [{ id: 'cat', name: 'Proyectos', sortOrder: 0, collapsed: false }];
// Por nombre dentro de cada sección: el orden de entrada da igual.
const canales = [
  ch({ id: 'dm-ana', kind: 'direct', unread: 2 }),
  ch({ id: 'general' }),
  ch({ id: 'diseño', categoryId: 'cat', unread: 1, muted: true }),
  ch({ id: 'avisos', unread: 4 }),
  ch({ id: 'equipo', favorite: true }),
  ch({ id: 'obra', categoryId: 'cat', mentions: 1, unread: 3, notifyLevel: 'mentions' }),
];
const grupos = groupChannels(canales, cats);
const abierto = (): boolean => false;
const ids = (l: readonly Channel[]): string[] => l.map((c) => c.id);

test('el orden visible va sección a sección, con los favoritos primero', () => {
  expect(ids(ordenVisible(grupos, abierto))).toEqual([
    'equipo',
    'avisos',
    'general',
    'diseño',
    'obra',
    'dm-ana',
  ]);
});

test('las secciones plegadas no entran en el orden', () => {
  const plegadas = new Set([DEFAULT_FAVORITES, 'cat']);
  expect(ids(ordenVisible(grupos, (g) => plegadas.has(g.key)))).toEqual([
    'avisos',
    'general',
    'dm-ana',
  ]);
  expect(ids(ordenVisible(grupos, (g) => g.key === DEFAULT_CHANNELS))).toEqual([
    'equipo',
    'diseño',
    'obra',
    'dm-ana',
  ]);
});

const orden = ordenVisible(grupos, abierto);

test.each([
  // [abierto, paso, solo pendientes, destino]
  ['avisos', 1, false, 'general'],
  ['avisos', -1, false, 'equipo'],
  ['equipo', -1, false, null], // a secas no da la vuelta
  ['dm-ana', 1, false, null],
  [null, 1, false, 'equipo'], // sin canal abierto: el primero al bajar…
  [null, -1, false, 'dm-ana'], // …y el último al subir
  ['oculto', 1, false, 'equipo'], // el abierto está en una sección plegada
  // Con Mayús: el siguiente con algo pendiente; «diseño» está silenciado y no cuenta.
  ['equipo', 1, true, 'avisos'],
  ['avisos', 1, true, 'obra'],
  ['obra', 1, true, 'dm-ana'],
  ['dm-ana', 1, true, 'avisos'], // da la vuelta
  ['avisos', -1, true, 'dm-ana'], // da la vuelta hacia arriba
  ['general', -1, true, 'avisos'],
  [null, -1, true, 'dm-ana'],
] as const)('desde %p con paso %p (pendientes: %p) va a %p', (desde, paso, solo, destino) => {
  expect(canalVecino(orden, desde, paso, solo)?.id ?? null).toBe(destino);
});

test('con Mayús, el abierto no cuenta aunque tenga pendientes', () => {
  const solo = [ch({ id: 'a', unread: 5 }), ch({ id: 'b' })];
  expect(canalVecino(solo, 'a', 1, true)).toBeNull();
  expect(canalVecino([], null, 1)).toBeNull();
});

function tecla(p: Partial<TeclaNavegar>): TeclaNavegar {
  return {
    key: 'ArrowDown',
    altKey: true,
    shiftKey: false,
    ctrlKey: false,
    metaKey: false,
    target: null,
    ...p,
  };
}

test('Alt+flecha mueve de canal; con Mayús, a los pendientes', () => {
  expect(pasoDeNavegacion(tecla({}))).toEqual({ paso: 1, soloPendientes: false });
  expect(pasoDeNavegacion(tecla({ key: 'ArrowUp', shiftKey: true }))).toEqual({
    paso: -1,
    soloPendientes: true,
  });
  expect(pasoDeNavegacion(tecla({ altKey: false }))).toBeNull();
  expect(pasoDeNavegacion(tecla({ ctrlKey: true }))).toBeNull();
  expect(pasoDeNavegacion(tecla({ metaKey: true }))).toBeNull();
  expect(pasoDeNavegacion(tecla({ key: 'ArrowLeft' }))).toBeNull();
});

test('en la caja solo actúa si está vacía', () => {
  const caja = (value: string) => ({ tagName: 'TEXTAREA', value }) as unknown as EventTarget;
  expect(pasoDeNavegacion(tecla({ target: caja('') }))).not.toBeNull();
  expect(pasoDeNavegacion(tecla({ target: caja('hola') }))).toBeNull();
  expect(pasoDeNavegacion(tecla({ target: caja(' ') }))).toBeNull();
  const editable = (textContent: string) =>
    ({ isContentEditable: true, textContent }) as unknown as EventTarget;
  expect(pasoDeNavegacion(tecla({ target: editable('') }))).not.toBeNull();
  expect(pasoDeNavegacion(tecla({ target: editable('texto') }))).toBeNull();
  // Un desplegable es suyo aunque su valor sea '': Alt+↓ abre sus opciones.
  const desplegable = { tagName: 'SELECT', value: '' } as unknown as EventTarget;
  expect(pasoDeNavegacion(tecla({ target: desplegable }))).toBeNull();
  // Un botón o una fila del carril no son campos: ahí siempre vale.
  const boton = { tagName: 'BUTTON' } as unknown as EventTarget;
  expect(pasoDeNavegacion(tecla({ target: boton }))).not.toBeNull();
});
