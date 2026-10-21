import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { alternarPlegado, coincidenciasEn, liPlegable } from './en-nota.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

test('buscar en la nota encuentra sin acentos y en cada nodo de texto', () => {
  const raiz = document.createElement('div');
  raiz.innerHTML = '<p>Reunión de equipo</p><ul><li>otra <b>reunion</b></li></ul>';
  const r = coincidenciasEn(raiz, 'reunion');
  expect(r.map((x) => x.toString())).toEqual(['Reunión', 'reunion']);
  expect(coincidenciasEn(raiz, '  ')).toEqual([]);
});

test('solo se pliega un elemento con sublista, dentro o justo detrás', () => {
  const raiz = document.createElement('div');
  raiz.innerHTML =
    '<ul><li id="a">a<ul><li id="b">b</li></ul></li><li id="c">c</li><ul><li>d</li></ul><li id="e">e</li></ul>';
  expect(liPlegable(raiz.querySelector('#a')!.firstChild)?.id).toBe('a');
  expect(liPlegable(raiz.querySelector('#b')!.firstChild)).toBeNull();
  expect(liPlegable(raiz.querySelector('#c')!.firstChild)?.id).toBe('c');
  expect(liPlegable(raiz.querySelector('#e')!.firstChild)).toBeNull();
});

test('plegar y desplegar deja el HTML como estaba', () => {
  const html = '<ul><li>a</li><li>b<ul><li>c</li></ul></li></ul>';
  const plegado = alternarPlegado(html, 1);
  expect(plegado).toBe('<ul><li>a</li><li class="plegada">b<ul><li>c</li></ul></li></ul>');
  expect(alternarPlegado(plegado, 1)).toBe(html);
  expect(alternarPlegado(html, 9)).toBe(html);
});
