/*
 * LO QUE DA FORMATO TIENE QUE SOBREVIVIR AL GUARDAR.
 *
 * El saneador del servicio solo deja en un `style` el color en `#rrggbb` y la
 * alineación. Con `styleWithCSS` encendido para todo, la negrita salía como
 * `<span style="font-weight: bold">` y desaparecía al guardar; y el color, que el
 * navegador escribe en `rgb()`, también. Aquí se mira lo que pide el editor al
 * navegador y lo que entrega a quien guarda.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import {
  boton,
  instalarEdicionDePrueba,
  montarEditor,
  pulsar,
  seleccionar,
  tick,
  type EdicionDePrueba,
  type EditorMontado,
} from './edicion-de-prueba.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

let edicion: EdicionDePrueba;
let editor: EditorMontado | null = null;
beforeEach(() => {
  edicion = instalarEdicionDePrueba();
});
afterEach(() => {
  editor?.desmontar();
  editor = null;
  edicion.quitar();
});

/** El `styleWithCSS` en vigor cuando se dio la orden `orden`. */
const cssAl = (orden: string) => edicion.ordenes.find(([o]) => o === orden)?.[2];

test('la negrita de la barra sale como etiqueta, no como estilo', async () => {
  editor = await montarEditor('<p>hola mundo</p>');
  seleccionar(editor.campo, 'hola');
  boton(editor.host, 'editor.bold').click();
  expect(cssAl('bold')).toBe(false);
  expect(editor.cambios.at(-1)).toContain('<b>hola</b>');
});

test('Ctrl+B, I y U van por el mismo camino que la barra', async () => {
  editor = await montarEditor('<p>hola mundo</p>');
  seleccionar(editor.campo, 'mundo');
  expect(pulsar(editor.campo, 'b', { ctrl: true })).toBe(true);
  expect(pulsar(editor.campo, 'i', { ctrl: true })).toBe(true);
  expect(pulsar(editor.campo, 'u', { ctrl: true })).toBe(true);
  expect(cssAl('bold')).toBe(false);
  expect(cssAl('italic')).toBe(false);
  expect(cssAl('underline')).toBe(false);
});

test('el color sale como estilo, y en hexadecimal', async () => {
  editor = await montarEditor('<p>hola mundo</p>');
  seleccionar(editor.campo, 'hola');
  boton(editor.host, 'editor.textColor').click();
  await tick();
  const muestra = [...document.querySelectorAll<HTMLElement>('[role="dialog"] button')].find(
    (b) => b.style.background,
  )!;
  muestra.click();
  expect(cssAl('foreColor')).toBe(true);
  const entregado = editor.cambios.at(-1)!;
  expect(entregado).toMatch(/style="color: #[0-9a-f]{6};"/);
  expect(entregado).not.toContain('rgb(');
});
