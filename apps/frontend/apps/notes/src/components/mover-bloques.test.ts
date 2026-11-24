/** Subir, bajar, duplicar, quitar y soltar un bloque de primer nivel de la nota. */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, beforeAll, expect, test } from 'bun:test';
import {
  bloqueDe,
  duplicarBloque,
  enPanel,
  llevarBloque,
  moverBloque,
  quitarBloque,
} from './mover-bloques.js';

let registrado = false;
beforeAll(() => {
  if (typeof document === 'undefined') {
    GlobalRegistrator.register();
    registrado = true;
  }
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

function campo(): HTMLElement {
  const c = document.createElement('div');
  c.innerHTML = '<h1>a</h1><p>b <b>c</b></p><ul><li>d</li></ul>';
  return c;
}

test('el bloque de primer nivel se encuentra desde lo más hondo', () => {
  const c = campo();
  expect(bloqueDe(c.querySelector('b')!.firstChild, c)?.tagName).toBe('P');
  expect(bloqueDe(c.querySelector('li')!, c)?.tagName).toBe('UL');
  expect(bloqueDe(document.body, c)).toBeNull();
});

test('subir y bajar un puesto, y en los extremos nada', () => {
  const c = campo();
  const p = c.querySelector('p')!;
  expect(moverBloque(p, -1)).toBe(true);
  expect(c.innerHTML).toBe('<p>b <b>c</b></p><h1>a</h1><ul><li>d</li></ul>');
  expect(moverBloque(p, -1)).toBe(false);
  expect(moverBloque(c.querySelector('ul')!, 1)).toBe(false);
  moverBloque(p, 1);
  moverBloque(p, 1);
  expect(c.innerHTML).toBe('<h1>a</h1><ul><li>d</li></ul><p>b <b>c</b></p>');
});

test('duplicar, quitar (dejando dónde escribir) y envolver en panel', () => {
  const c = campo();
  duplicarBloque(c.querySelector('h1')!);
  expect(c.innerHTML.startsWith('<h1>a</h1><h1>a</h1>')).toBe(true);
  // Una copia: la colección de hijos está viva y se vacía al quitar.
  for (const b of Array.from(c.children)) quitarBloque(b as HTMLElement, c);
  expect(c.innerHTML).toBe('<p><br></p>');
  enPanel(c.querySelector('p')!, 'aviso');
  expect(c.innerHTML).toBe('<div data-panel="aviso"><p><br></p></div>');
});

test('soltar un bloque delante o detrás de otro, no sobre sí mismo', () => {
  const c = campo();
  const [h1, p, ul] = [...c.children] as HTMLElement[];
  expect(llevarBloque(ul!, h1!, true)).toBe(true);
  expect(c.innerHTML).toBe('<ul><li>d</li></ul><h1>a</h1><p>b <b>c</b></p>');
  expect(llevarBloque(p!, p!, false)).toBe(false);
});
