/*
 * MARKDOWN AL ESCRIBIR, EN EL EDITOR: lo de principio de línea (`# `, `- `, `[] `…) y lo
 * de en medio (`**x**`, `*x*`, `_x_`, `` `x` ``, `~~x~~`), y un Ctrl+Z JUSTO después
 * que devuelve lo tecleado tal cual. Dentro de un bloque de código, nada.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { olvidarMedidas } from './BarraConMas.js';
import {
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

test('«# » al principio de la línea es un título, y Ctrl+Z devuelve «# »', async () => {
  editor = await montarEditor('<p>#</p>');
  await teclear(editor.campo, ' ');
  expect(editor.campo.querySelector('h1')).not.toBeNull();
  expect(editor.campo.textContent).toBe('');
  expect(pulsar(editor.campo, 'z', { ctrl: true })).toBe(true);
  await tick();
  expect(editor.campo.querySelector('h1')).toBeNull();
  expect(editor.campo.textContent).toBe('# ');
});

test('«- » es una lista y «[] » una tarea; deshacer la tarea devuelve «[] »', async () => {
  editor = await montarEditor('<p>-</p>');
  await teclear(editor.campo, ' ');
  expect(editor.campo.querySelector('ul > li')).not.toBeNull();
  editor.desmontar();
  editor = await montarEditor('<p>[]</p>');
  await teclear(editor.campo, ' ');
  expect(editor.campo.querySelector('.task-row input[type="checkbox"]')).not.toBeNull();
  pulsar(editor.campo, 'z', { ctrl: true });
  await tick();
  expect(editor.campo.querySelector('.task-row')).toBeNull();
  expect(editor.campo.textContent).toBe('[] ');
});

test('las marcas en la línea convierten al cerrarse, y un Ctrl+Z devuelve las marcas', async () => {
  const casos: [string, string][] = [
    ['**sí**', '<b>sí</b>'],
    ['*vale*', '<i>vale</i>'],
    ['_así_', '<i>así</i>'],
    ['`npm i`', '<code>npm i</code>'],
    ['~~no~~', '<s>no</s>'],
  ];
  for (const [tecleado, html] of casos) {
    editor = await montarEditor('<p>dijo </p>');
    await teclear(editor.campo, tecleado);
    expect(editor.campo.innerHTML).toContain(html);
    // Al guardar no viaja el espacio de ancho cero que deja seguir fuera del formato.
    expect(editor.cambios.at(-1)).not.toContain('​');
    pulsar(editor.campo, 'z', { ctrl: true });
    await tick();
    expect(editor.campo.textContent).toBe(`dijo ${tecleado}`);
    editor.desmontar();
    editor = null;
  }
});

test('el Ctrl+Z solo deshace el atajo si es lo último: después, es el de siempre', async () => {
  editor = await montarEditor('<p>dijo </p>');
  await teclear(editor.campo, '**sí**');
  await teclear(editor.campo, 'x');
  const antes = edicion.ordenes.filter(([o]) => o === 'undo').length;
  // Ya no es lo último: el Ctrl+Z sigue su curso (lo atiende el navegador).
  expect(pulsar(editor.campo, 'z', { ctrl: true })).toBe(false);
  expect(edicion.ordenes.filter(([o]) => o === 'undo').length).toBe(antes);
});

test('dentro de un bloque de código las marcas se quedan en letras', async () => {
  editor = await montarEditor('<pre>x </pre>');
  await teclear(editor.campo, '`a` **b** ');
  expect(edicion.ordenes.some(([o]) => o === 'insertHTML' || o === 'formatBlock')).toBe(false);
  expect(editor.campo.textContent).toBe('x `a` **b** ');
});

test('donde no hay marca de verdad, no convierte nada', async () => {
  editor = await montarEditor('<p>a </p>');
  await teclear(editor.campo, '2*3* snake_case_ ');
  expect(edicion.ordenes.some(([o]) => o === 'insertHTML')).toBe(false);
});
