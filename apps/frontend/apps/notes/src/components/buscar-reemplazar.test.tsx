/*
 * BUSCAR Y REEMPLAZAR EN LA NOTA: Ctrl/⌘+F busca y Ctrl/⌘+H reemplaza; cuántas hay, la
 * siguiente y la anterior, «Coincidir mayúsculas», reemplazar una y todas. Buscar no
 * toca el HTML de la nota (las coincidencias se pintan aparte); reemplazar va con
 * `insertText`, que es texto y entra en la pila de deshacer.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { olvidarMedidas } from './BarraConMas.js';
import {
  boton,
  instalarEdicionDePrueba,
  montarEditor,
  pulsar,
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

const NOTA = '<p>Casa casa</p><p>una CASA y otra casa</p>';
const buscar = () => document.getElementById('notes-buscar-en-nota') as HTMLInputElement;
const reemplazo = () => document.getElementById('notes-reemplazar-en-nota') as HTMLInputElement;
const cuantas = () => editor!.host.querySelector('[role="search"] .sr-only')?.textContent;
/** La fila de reemplazar, con sus dos botones. */
const filaReemplazar = () => reemplazo().parentElement!;

async function escribirEn(campo: HTMLInputElement, valor: string) {
  campo.value = valor;
  campo.dispatchEvent(new Event('input', { bubbles: true }));
  await tick();
}

test('Ctrl+F abre la búsqueda con el foco en ella y cuenta sin tocar la nota', async () => {
  editor = await montarEditor(NOTA);
  expect(pulsar(editor.campo, 'f', { ctrl: true })).toBe(true);
  await tick(5);
  expect(document.activeElement).toBe(buscar());
  await escribirEn(buscar(), 'casa');
  // Sin acentos ni mayúsculas, como el resto de búsquedas de la suite.
  expect(cuantas()).toBe('editor.matches{"count":4}');
  expect(editor.host.querySelector('[role="search"] [aria-hidden="true"]')?.textContent).toBe(
    'find.count{"current":"1","total":"4"}',
  );
  // Siguiente y anterior, con Intro y Mayús+Intro.
  pulsar(buscar(), 'Enter');
  await tick();
  expect(editor.host.querySelector('[role="search"] [aria-hidden="true"]')?.textContent).toContain(
    '"current":"2"',
  );
  pulsar(buscar(), 'Enter', { shift: true });
  await tick();
  pulsar(buscar(), 'Enter', { shift: true });
  await tick();
  expect(editor.host.querySelector('[role="search"] [aria-hidden="true"]')?.textContent).toContain(
    '"current":"4"',
  );
  expect(editor.campo.innerHTML).toBe(NOTA);
});

test('«Coincidir mayúsculas» busca tal cual', async () => {
  editor = await montarEditor(NOTA);
  pulsar(editor.campo, 'f', { ctrl: true });
  await tick(5);
  await escribirEn(buscar(), 'casa');
  const mayus = boton(editor.host.querySelector('[role="search"]')!, 'editor.matchCase');
  expect(mayus.getAttribute('aria-pressed')).toBe('false');
  mayus.click();
  await tick();
  expect(mayus.getAttribute('aria-pressed')).toBe('true');
  expect(cuantas()).toBe('editor.matches{"count":2}');
  await escribirEn(buscar(), 'CASA');
  expect(cuantas()).toBe('editor.matches{"count":1}');
});

test('Ctrl+H abre reemplazar; se reemplaza la actual o todas', async () => {
  editor = await montarEditor(NOTA);
  pulsar(editor.campo, 'h', { ctrl: true });
  await tick(5);
  expect(reemplazo()).not.toBeNull();
  const abre = editor.host.querySelector('[aria-controls="notes-reemplazar-en-nota"]');
  expect(abre?.getAttribute('aria-expanded')).toBe('true');
  await escribirEn(buscar(), 'casa');
  await escribirEn(reemplazo(), 'piso');
  // La actual (la primera): una sola, con texto, no con HTML.
  boton(filaReemplazar(), 'editor.replace').click();
  await tick(5);
  expect(editor.campo.textContent).toBe('piso casauna CASA y otra casa');
  expect(edicion.ordenes.filter(([o]) => o === 'insertText').map(([, v]) => v)).toEqual(['piso']);
  expect(edicion.ordenes.some(([o]) => o === 'insertHTML')).toBe(false);
  expect(cuantas()).toBe('editor.matches{"count":3}');
  // Todas las que quedan.
  boton(filaReemplazar(), 'editor.replaceAll').click();
  await tick(5);
  expect(editor.campo.textContent).toBe('piso pisouna piso y otra piso');
  expect(cuantas()).toBe('editor.matches{"count":0}');
  expect(editor.cambios.at(-1)).toBe('<p>piso piso</p><p>una piso y otra piso</p>');
});

test('reemplazar todo respeta «Coincidir mayúsculas»', async () => {
  editor = await montarEditor(NOTA);
  pulsar(editor.campo, 'h', { ctrl: true });
  await tick(5);
  boton(editor.host.querySelector('[role="search"]')!, 'editor.matchCase').click();
  await tick();
  await escribirEn(buscar(), 'casa');
  await escribirEn(reemplazo(), 'piso');
  boton(filaReemplazar(), 'editor.replaceAll').click();
  await tick(5);
  expect(editor.campo.textContent).toBe('Casa pisouna CASA y otra piso');
});

test('Escape cierra la búsqueda y vuelve al texto', async () => {
  editor = await montarEditor(NOTA);
  pulsar(editor.campo, 'f', { ctrl: true });
  await tick(5);
  pulsar(buscar(), 'Escape');
  await tick();
  expect(editor.host.querySelector('[role="search"]')).toBeNull();
  expect(document.activeElement).toBe(editor.campo);
});
