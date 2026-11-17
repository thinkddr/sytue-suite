/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Deslizar una fila con el dedo (Mail: derecha archiva, izquierda a la papelera):
 * la decisión de empezar (horizontal de verdad, no desplazar la lista), el tope del
 * movimiento y la fila de List que lo usa. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { List, ListItem } from '../components/List.js';
import { desplazamientoVisible, iniciarDeslizar } from './useDeslizarFila.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const AMBOS = { derecha: true, izquierda: true };

test('el gesto no empieza hasta que el dedo se mueve, y el vertical es de la lista', () => {
  expect(iniciarDeslizar(4, 3, AMBOS)).toBe('esperar');
  expect(iniciarDeslizar(14, 2, AMBOS)).toBe('arrastrar');
  expect(iniciarDeslizar(-14, 2, AMBOS)).toBe('arrastrar');
  // Vertical o diagonal dudosa: se desplaza la lista, la fila no hace nada.
  expect(iniciarDeslizar(2, 20, AMBOS)).toBe('soltar');
  expect(iniciarDeslizar(14, 12, AMBOS)).toBe('soltar');
  // Hacia un lado sin acción no se arrastra.
  expect(iniciarDeslizar(20, 0, { derecha: false, izquierda: true })).toBe('soltar');
});

test('la fila sigue al dedo hasta su ancho y no se mueve hacia un lado sin acción', () => {
  expect(desplazamientoVisible(40, 300, AMBOS)).toBe(40);
  expect(desplazamientoVisible(500, 300, AMBOS)).toBe(300);
  expect(desplazamientoVisible(-500, 300, AMBOS)).toBe(-300);
  expect(desplazamientoVisible(-40, 300, { derecha: true, izquierda: false })).toBe(0);
});

function puntero(el: Element, tipo: string, x: number, y: number, pointerType = 'touch'): void {
  const ev = new PointerEvent(tipo, {
    bubbles: true,
    cancelable: true,
    pointerId: 7,
    pointerType,
    isPrimary: true,
    clientX: x,
    clientY: y,
  });
  act(() => {
    el.dispatchEvent(ev);
  });
}

function montar() {
  const host = document.body.appendChild(document.createElement('div'));
  const hechas: string[] = [];
  let aperturas = 0;
  act(() =>
    render(
      <List label="Correo" selectable>
        <ListItem
          index={0}
          title="Boletín"
          onClick={() => aperturas++}
          deslizarDerecha={{
            etiqueta: 'Archivar',
            tono: 'success',
            alSoltar: () => hechas.push('archivar'),
          }}
          deslizarIzquierda={{
            etiqueta: 'Papelera',
            tono: 'danger',
            alSoltar: () => hechas.push('papelera'),
          }}
        />
      </List>,
      host,
    ),
  );
  const li = host.querySelector('li') as HTMLLIElement;
  return { host, li, hechas, aperturas: () => aperturas };
}

test('deslizar a la derecha descubre «Archivar» y archiva al soltar, sin abrir la fila', () => {
  const { host, li, hechas, aperturas } = montar();
  puntero(li, 'pointerdown', 20, 50);
  puntero(li, 'pointermove', 60, 52);
  expect(li.style.transform).toBe('translateX(40px)');
  const franja = host.querySelector('[data-deslizar="derecha"]');
  expect(franja?.textContent).toBe('Archivar');
  expect(franja?.getAttribute('aria-hidden')).toBe('true');
  puntero(li, 'pointermove', 120, 52);
  puntero(li, 'pointerup', 120, 52);
  expect(hechas).toEqual(['archivar']);
  expect(li.style.transform).toBe('');
  // El click que sigue al dedo no abre la conversación que se acaba de archivar.
  act(() => li.click());
  expect(aperturas()).toBe(0);
  // Y el siguiente toque normal sí la abre.
  act(() => li.click());
  expect(aperturas()).toBe(1);
  render(null, host);
});

test('deslizar a la izquierda manda a la papelera', () => {
  const { host, li, hechas } = montar();
  puntero(li, 'pointerdown', 200, 50);
  puntero(li, 'pointermove', 180, 50);
  expect(host.querySelector('[data-deslizar="izquierda"]')?.textContent).toBe('Papelera');
  puntero(li, 'pointermove', 100, 50);
  puntero(li, 'pointerup', 100, 50);
  expect(hechas).toEqual(['papelera']);
  render(null, host);
});

test('desplazar la lista en vertical, o arrastrar con el ratón, no hace nada', () => {
  const { host, li, hechas } = montar();
  puntero(li, 'pointerdown', 20, 50);
  puntero(li, 'pointermove', 24, 90);
  puntero(li, 'pointermove', 120, 95);
  puntero(li, 'pointerup', 120, 95);
  expect(li.style.transform).toBe('');
  puntero(li, 'pointerdown', 20, 50, 'mouse');
  puntero(li, 'pointermove', 200, 50, 'mouse');
  puntero(li, 'pointerup', 200, 50, 'mouse');
  expect(hechas).toEqual([]);
  render(null, host);
});

test('sin acciones de deslizar la fila no cambia', () => {
  const host = document.body.appendChild(document.createElement('div'));
  act(() =>
    render(
      <List label="x">
        <ListItem title="Fila" />
      </List>,
      host,
    ),
  );
  const li = host.querySelector('li') as HTMLLIElement;
  expect(li.className).not.toContain('touch-pan-y');
  puntero(li, 'pointerdown', 20, 50);
  puntero(li, 'pointermove', 200, 50);
  expect(li.style.transform).toBe('');
  render(null, host);
});
