/*
 * EL ASA ⋮⋮ DE CADA BLOQUE Y SU MENÚ: subir, bajar, duplicar, eliminar y «Convertir
 * en…», como en Confluence; con el teclado, Alt+Mayús+↑/↓ y Mayús+F10. El asa sigue al
 * bloque del cursor (con el dedo no hay «pasar por encima») y al del puntero.
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

const NOTA = '<h1>uno</h1><p>dos</p><p>tres</p>';
/** El editor con el cursor en «dos». */
const enDos = () =>
  montarEditor(NOTA, {}, (campo) => [campo.querySelectorAll('p')[0]!.firstChild!, 1]);
const asa = () => editor!.host.querySelector<HTMLElement>('.notes-asa');
const menu = () =>
  document.querySelector<HTMLElement>('[role="menu"][aria-label="editor.blockMenu"]');

test('el asa sale en el bloque del cursor y abre su menú', async () => {
  editor = await enDos();
  expect(asa()?.getAttribute('aria-label')).toBe('editor.blockMenu');
  expect(asa()?.getAttribute('draggable')).toBe('true');
  asa()!.click();
  await tick();
  const m = menu()!;
  expect(m).not.toBeNull();
  for (const e of ['editor.moveUp', 'editor.moveDown', 'editor.duplicate', 'editor.delete'])
    expect(m.textContent).toContain(e);
  expect(m.textContent).toContain('editor.convertTo');
});

test('subir, bajar, duplicar y eliminar desde el menú', async () => {
  editor = await enDos();
  const elegir = async (rotulo: string) => {
    asa()!.click();
    await tick();
    boton(menu()!, rotulo).click();
    await tick();
  };
  await elegir('editor.moveUp');
  expect(editor.campo.innerHTML).toBe('<p>dos</p><h1>uno</h1><p>tres</p>');
  await elegir('editor.moveDown');
  expect(editor.campo.innerHTML).toBe('<h1>uno</h1><p>dos</p><p>tres</p>');
  await elegir('editor.duplicate');
  expect(editor.campo.innerHTML).toBe('<h1>uno</h1><p>dos</p><p>dos</p><p>tres</p>');
  await elegir('editor.delete');
  expect(editor.campo.innerHTML).toBe('<h1>uno</h1><p>dos</p><p>tres</p>');
  expect(editor.cambios.at(-1)).toBe('<h1>uno</h1><p>dos</p><p>tres</p>');
});

test('«Convertir en…» cambia el bloque con la misma orden que la barra', async () => {
  editor = await enDos();
  asa()!.click();
  await tick();
  boton(menu()!, 'editor.heading2').click();
  await tick();
  expect(edicion.ordenes.some(([o, v]) => o === 'formatBlock' && v === '<h2>')).toBe(true);
  expect(editor.campo.innerHTML).toBe('<h1>uno</h1><h2>dos</h2><p>tres</p>');
  asa()!.click();
  await tick();
  boton(menu()!, 'editor.panelInfo').click();
  await tick();
  expect(editor.campo.innerHTML).toContain('<div data-panel="info"><h2>dos</h2></div>');
});

test('Alt+Mayús+↑/↓ mueve el bloque del cursor, y Mayús+F10 abre su menú', async () => {
  editor = await enDos();
  expect(pulsar(editor.campo, 'ArrowDown', { alt: true, shift: true })).toBe(true);
  expect(editor.campo.innerHTML).toBe('<h1>uno</h1><p>tres</p><p>dos</p>');
  pulsar(editor.campo, 'ArrowUp', { alt: true, shift: true });
  pulsar(editor.campo, 'ArrowUp', { alt: true, shift: true });
  expect(editor.campo.innerHTML).toBe('<p>dos</p><h1>uno</h1><p>tres</p>');
  expect(pulsar(editor.campo, 'F10', { shift: true })).toBe(true);
  await tick();
  expect(menu()).not.toBeNull();
});

/**
 * Un evento de arrastre con los dos nombres: el del navegador (`dragover`) y el que usa
 * Preact cuando el elemento no tiene `ondragover` (happy-dom no lo tiene): `DragOver`.
 */
function arrastre(el: Element, tipo: 'DragStart' | 'DragOver' | 'Drop', clientY = 0): boolean {
  let parado = false;
  for (const nombre of [tipo.toLowerCase(), tipo]) {
    const e = new MouseEvent(nombre, { bubbles: true, cancelable: true, clientY });
    el.dispatchEvent(e);
    parado ||= e.defaultPrevented;
  }
  return parado;
}

test('arrastrar el asa y soltar sobre otro bloque lo lleva allí', async () => {
  editor = await enDos();
  const tres = editor.campo.querySelectorAll('p')[1]!;
  tres.getBoundingClientRect = () => ({ top: 100, bottom: 120, height: 20 }) as DOMRect;
  arrastre(asa()!, 'DragStart');
  // Por debajo de la mitad de «tres»: cae detrás.
  expect(arrastre(tres, 'DragOver', 119)).toBe(true);
  await tick();
  expect(editor.host.querySelector('.notes-soltar')).not.toBeNull();
  expect(arrastre(tres, 'Drop')).toBe(true);
  await tick();
  expect(editor.campo.innerHTML).toBe('<h1>uno</h1><p>tres</p><p>dos</p>');
});
