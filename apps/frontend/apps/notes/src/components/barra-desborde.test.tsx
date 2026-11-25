/*
 * LA BARRA DEL EDITOR EN UNA FILA, Y LO QUE NO CABE EN «MÁS».
 *
 * La barra se envolvía en varias filas en el escritorio y se desplazaba de lado en el
 * teléfono. Ahora mide lo que cabe; lo demás va a «Más», empezando por lo menos usado.
 * happy-dom no maqueta: el ancho de la fila se pone a mano y las piezas miden su ancho
 * de reserva (un botón, 28 px).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { olvidarMedidas } from './BarraConMas.js';
import {
  boton,
  instalarEdicionDePrueba,
  montarEditor,
  tick,
  type EdicionDePrueba,
  type EditorMontado,
} from './edicion-de-prueba.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

let anchoDeFila = 0;
const original = Object.getOwnPropertyDescriptor(HTMLElement.prototype, 'clientWidth');
let edicion: EdicionDePrueba;
let editor: EditorMontado | null = null;

beforeEach(() => {
  olvidarMedidas();
  edicion = instalarEdicionDePrueba();
  Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
    configurable: true,
    get(this: HTMLElement) {
      return this.getAttribute('role') === 'toolbar' ? anchoDeFila : 0;
    },
  });
});
afterEach(() => {
  editor?.desmontar();
  editor = null;
  edicion.quitar();
  if (original) Object.defineProperty(HTMLElement.prototype, 'clientWidth', original);
});

const barra = () => editor!.host.querySelector<HTMLElement>('[role="toolbar"]')!;
const enBarra = () =>
  [...barra().querySelectorAll<HTMLElement>('button')].map((b) => b.getAttribute('aria-label'));
const mas = () => barra().querySelector<HTMLElement>('button[aria-label="editor.more"]');

test('con sitio de sobra no hay «Más» y está todo en la fila', async () => {
  anchoDeFila = 2000;
  editor = await montarEditor('<p>hola</p>');
  expect(mas()).toBeNull();
  expect(enBarra()).toContain('editor.ai');
  expect(enBarra()).toContain('editor.find');
});

test('estrecha, la fila no se envuelve ni se desplaza: lo que no cabe va a «Más»', async () => {
  anchoDeFila = 320;
  editor = await montarEditor('<p>hola</p>');
  expect(mas()).not.toBeNull();
  // Una fila, sin desplazamiento de lado: no lleva la marca del desbordamiento a propósito.
  expect(barra().hasAttribute('data-scroll-x')).toBe(false);
  expect(barra().className).toContain('flex-nowrap');
  // Lo más usado se queda; las herramientas se van primero.
  const fila = enBarra();
  expect(fila).toContain('editor.bold');
  expect(fila).toContain('editor.italic');
  expect(fila).toContain('editor.bulletList');
  expect(fila).not.toContain('editor.ai');
  expect(fila).not.toContain('editor.find');
});

test('«Más» ofrece lo escondido, con su atajo, y elegirlo actúa', async () => {
  anchoDeFila = 320;
  editor = await montarEditor('<p>hola</p>');
  mas()!.click();
  await tick();
  const menu = document.querySelector<HTMLElement>('[role="menu"]')!;
  expect(menu.getAttribute('aria-label')).toBe('editor.more');
  const textos = [...menu.querySelectorAll('[role="menuitem"], [role="menuitemcheckbox"]')].map(
    (i) => i.textContent,
  );
  expect(textos).toContain('editor.findCtrl+F');
  expect(textos).not.toContain('editor.boldCtrl+B');
  // Elegir «Buscar» abre la búsqueda de la nota, igual que su botón.
  boton(menu, 'editor.findCtrl+F').click();
  await tick(10);
  expect(editor.host.querySelector('[role="search"]')).not.toBeNull();
});

test('un control con estado lo dice también dentro de «Más»', async () => {
  anchoDeFila = 200;
  editor = await montarEditor('<p>hola</p>');
  mas()!.click();
  await tick();
  const tachado = [...document.querySelectorAll('[role="menu"] [role="menuitemcheckbox"]')].find(
    (i) => i.textContent?.startsWith('editor.strike'),
  );
  expect(tachado?.getAttribute('aria-checked')).toBe('false');
});

test('la herramienta escondida abre su panel colgado de «Más»', async () => {
  anchoDeFila = 320;
  editor = await montarEditor('<p>hola</p>');
  expect(enBarra()).not.toContain('editor.textColor');
  mas()!.click();
  await tick();
  boton(document.querySelector('[role="menu"]')!, 'editor.textColor').click();
  await tick(10);
  const panel = document.querySelector<HTMLElement>('[role="dialog"]');
  expect(panel?.getAttribute('aria-label')).toBe('editor.textColor');
  expect(panel?.textContent).toContain('editor.colorDefault');
});
