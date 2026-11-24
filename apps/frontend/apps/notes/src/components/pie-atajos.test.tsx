/*
 * EL PIE DEL EDITOR (palabras, caracteres y lectura, con las cuentas de «Información»),
 * LA HOJA DE ATAJOS (Ctrl/⌘+/) Y EL ANCHO DE LA HOJA desde la barra.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { olvidarMedidas } from './BarraConMas.js';
import {
  boton,
  instalarEdicionDePrueba,
  montarEditor,
  pulsar,
  teclear,
  tick,
  type EdicionDePrueba,
  type EditorMontado,
} from './edicion-de-prueba.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

let edicion: EdicionDePrueba;
let editor: EditorMontado | null = null;
beforeEach(() => {
  olvidarMedidas();
  edicion = instalarEdicionDePrueba();
});
afterEach(() => {
  editor?.desmontar();
  editor = null;
  edicion.quitar();
});

const pie = () =>
  [...editor!.host.querySelectorAll('.tabular-nums.select-none > span')].map((s) => s.textContent);

test('el pie cuenta palabras, caracteres y minutos de lectura, y sigue a lo escrito', async () => {
  editor = await montarEditor('<p>uno dos <b>tres</b></p>');
  expect(pie()).toEqual([
    'editor.words{"count":3}',
    'editor.chars{"count":12}',
    'editor.readingTime{"minutes":1}',
  ]);
  await teclear(editor.campo, ' cuatro');
  await tick(300);
  expect(pie()[0]).toBe('editor.words{"count":4}');
});

test('una nota vacía son cero palabras y cero minutos', async () => {
  editor = await montarEditor('');
  expect(pie()).toEqual([
    'editor.words{"count":0}',
    'editor.chars{"count":0}',
    'editor.readingTime{"minutes":0}',
  ]);
});

test('Ctrl+/ abre la hoja de atajos del editor, y Escape la cierra', async () => {
  editor = await montarEditor('<p>hola</p>');
  expect(pulsar(editor.campo, '/', { ctrl: true })).toBe(true);
  await tick();
  const hoja = document.querySelector<HTMLElement>('[role="dialog"]')!;
  expect(hoja.textContent).toContain('editor.shortcuts');
  for (const texto of ['Ctrl+F', 'Ctrl+H', 'Ctrl+B', 'editor.shortcutsMarkdown', '**x**'])
    expect(hoja.textContent).toContain(texto);
  pulsar(hoja, 'Escape');
  await tick(10);
  expect(document.querySelector('[role="dialog"]')).toBeNull();
});

test('en la hoja del árbol, «Ancho completo» dice su estado y cambia el ancho', async () => {
  let veces = 0;
  editor = await montarEditor('<p>hola</p>', { anchoCompleto: true, onAncho: () => veces++ });
  const ancho = boton(editor.host, 'editor.fullWidth');
  expect(ancho.getAttribute('aria-pressed')).toBe('true');
  ancho.click();
  expect(veces).toBe(1);
  editor.desmontar();
  // Fuera de la hoja (una tarjeta), no se ofrece.
  editor = await montarEditor('<p>hola</p>');
  expect(() => boton(editor!.host, 'editor.fullWidth')).toThrow();
});
