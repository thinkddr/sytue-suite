/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
/**
 * Tests de las flechas por listas y cuadrículas. Los casos de `vecinoEnRejilla` son los
 * mismos que los de `apps/fyne/ui/navegacionrejilla_test.go`: si cambia uno, cambia el otro.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { describe, expect, test } from 'bun:test';
import {
  moverFocoEnListaVirtual,
  moverFocoEnRejilla,
  vecinoEnLista,
  vecinoEnRejilla,
  type CajaRejilla,
} from './navegacionRejilla';

if (typeof document === 'undefined') GlobalRegistrator.register();

/** Una rejilla regular de `cols` columnas de 100 × 80 con 10 de hueco. */
const rejilla = (n: number, cols: number): CajaRejilla[] =>
  Array.from({ length: n }, (_, i) => ({
    x: (i % cols) * 110,
    y: Math.floor(i / cols) * 90,
    ancho: 100,
    alto: 80,
  }));

/**
 * Dos filas justificadas como las de Photos: arriba tres fotos (anchas, estrecha,
 * ancha) y abajo cuatro; luego la cabecera de un mes y una fila de dos.
 */
const justificadas: CajaRejilla[] = [
  { x: 0, y: 0, ancho: 200, alto: 100 },
  { x: 204, y: 0, ancho: 60, alto: 100 },
  { x: 268, y: 0, ancho: 232, alto: 100 },
  { x: 0, y: 104, ancho: 120, alto: 120 },
  { x: 124, y: 104, ancho: 120, alto: 120 },
  { x: 248, y: 104, ancho: 120, alto: 120 },
  { x: 372, y: 104, ancho: 128, alto: 120 },
  // 40 px de cabecera de mes en medio.
  { x: 0, y: 264, ancho: 300, alto: 150 },
  { x: 304, y: 264, ancho: 196, alto: 150 },
];

describe('vecinoEnLista', () => {
  test('↑ y ↓ de uno en uno, Inicio y Fin a los extremos, y nada fuera de ellos', () => {
    expect(vecinoEnLista(3, 10, 'ArrowDown')).toBe(4);
    expect(vecinoEnLista(3, 10, 'ArrowUp')).toBe(2);
    expect(vecinoEnLista(3, 10, 'Home')).toBe(0);
    expect(vecinoEnLista(3, 10, 'End')).toBe(9);
    expect(vecinoEnLista(9, 10, 'ArrowDown')).toBeNull();
    expect(vecinoEnLista(0, 10, 'ArrowUp')).toBeNull();
    expect(vecinoEnLista(0, 10, 'Home')).toBeNull();
    expect(vecinoEnLista(3, 10, 'ArrowRight')).toBeNull();
    expect(vecinoEnLista(3, 10, 'a')).toBeNull();
  });
});

describe('vecinoEnRejilla', () => {
  const cuatro = rejilla(10, 4); // filas de 4, 4 y 2

  test('← y → siguen el orden de lectura y pasan de fila', () => {
    expect(vecinoEnRejilla(cuatro, 3, 'ArrowRight')).toBe(4);
    expect(vecinoEnRejilla(cuatro, 4, 'ArrowLeft')).toBe(3);
    expect(vecinoEnRejilla(cuatro, 9, 'ArrowRight')).toBeNull();
    expect(vecinoEnRejilla(cuatro, 0, 'ArrowLeft')).toBeNull();
  });

  test('↑ y ↓ saltan una fila entera en la misma columna', () => {
    expect(vecinoEnRejilla(cuatro, 1, 'ArrowDown')).toBe(5);
    expect(vecinoEnRejilla(cuatro, 5, 'ArrowUp')).toBe(1);
    expect(vecinoEnRejilla(cuatro, 2, 'ArrowUp')).toBeNull();
  });

  test('↓ desde una columna que no existe en la última fila va a la más cercana', () => {
    expect(vecinoEnRejilla(cuatro, 7, 'ArrowDown')).toBe(9);
    expect(vecinoEnRejilla(cuatro, 9, 'ArrowDown')).toBeNull();
  });

  test('las columnas salen de la medida: con 2 por fila ↓ suma 2', () => {
    expect(vecinoEnRejilla(rejilla(6, 2), 1, 'ArrowDown')).toBe(3);
  });

  test('en filas justificadas ↑ y ↓ eligen la foto que cae debajo, y cruzan el mes', () => {
    // La estrecha del centro (204-264, centro 234) cae sobre la segunda de abajo (124-244).
    expect(vecinoEnRejilla(justificadas, 1, 'ArrowDown')).toBe(4);
    // La ancha de la derecha (centro 384) cae entre la tercera (centro 308) y la cuarta
    // (centro 436): gana la más cercana, la cuarta.
    expect(vecinoEnRejilla(justificadas, 2, 'ArrowDown')).toBe(6);
    expect(vecinoEnRejilla(justificadas, 4, 'ArrowUp')).toBe(1);
    // Por encima de la cabecera del mes.
    expect(vecinoEnRejilla(justificadas, 6, 'ArrowDown')).toBe(8);
    expect(vecinoEnRejilla(justificadas, 7, 'ArrowUp')).toBe(4);
  });

  test('Inicio y Fin van a la primera y a la última', () => {
    expect(vecinoEnRejilla(cuatro, 5, 'Home')).toBe(0);
    expect(vecinoEnRejilla(cuatro, 5, 'End')).toBe(9);
    expect(vecinoEnRejilla(cuatro, 5, 'Enter')).toBeNull();
    expect(vecinoEnRejilla([], 0, 'ArrowDown')).toBeNull();
  });
});

/** Botones con la caja de `cajas`: happy-dom no maqueta, así que la caja se impone. */
function botones(cajas: readonly CajaRejilla[]): HTMLButtonElement[] {
  document.body.innerHTML = '';
  return cajas.map((c) => {
    const b = document.createElement('button');
    b.getBoundingClientRect = () =>
      ({ left: c.x, top: c.y, width: c.ancho, height: c.alto }) as DOMRect;
    document.body.append(b);
    return b;
  });
}

function tecla(target: HTMLElement, key: string, init: KeyboardEventInit = {}): KeyboardEvent {
  const e = new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true, ...init });
  Object.defineProperty(e, 'target', { value: target });
  Object.defineProperty(e, 'currentTarget', { value: target });
  return e;
}

describe('moverFocoEnRejilla', () => {
  test('↓ enfoca la tesela de debajo y se queda la tecla', () => {
    const els = botones(rejilla(8, 4));
    els[1]!.focus();
    const e = tecla(els[1]!, 'ArrowDown');
    expect(moverFocoEnRejilla(e, els)).toBe(true);
    expect(document.activeElement).toBe(els[5]!);
    expect(e.defaultPrevented).toBe(true);
  });

  test('con Mayús, en el borde o desde un mando de dentro no hace nada', () => {
    const els = botones(rejilla(8, 4));
    expect(moverFocoEnRejilla(tecla(els[1]!, 'ArrowDown', { shiftKey: true }), els)).toBe(false);
    const borde = tecla(els[0]!, 'ArrowUp');
    expect(moverFocoEnRejilla(borde, els)).toBe(false);
    expect(borde.defaultPrevented).toBe(false);
    const dentro = document.createElement('input');
    expect(moverFocoEnRejilla(tecla(dentro, 'ArrowRight'), els)).toBe(false);
  });
});

describe('moverFocoEnListaVirtual', () => {
  test('lleva la fila a la vista y la enfoca cuando el virtualizador la pinta', async () => {
    const els = botones(rejilla(3, 1));
    const llevadas: number[] = [];
    // La fila 2 no está pintada todavía: aparece en el fotograma siguiente.
    let pintada = false;
    const e = tecla(els[0]!, 'End');
    const movido = moverFocoEnListaVirtual(
      e,
      0,
      50,
      (i) => {
        llevadas.push(i);
        requestAnimationFrame(() => (pintada = true));
      },
      (i) => (pintada && i === 49 ? els[2]! : null),
    );
    expect(movido).toBe(true);
    expect(llevadas).toEqual([49]);
    expect(e.defaultPrevented).toBe(true);
    await new Promise((r) => setTimeout(r, 100));
    expect(document.activeElement).toBe(els[2]!);
  });

  test('desde un mando de la fila (la casilla, el «⋯») las flechas no son suyas', () => {
    const els = botones(rejilla(2, 1));
    const e = new KeyboardEvent('keydown', { key: 'ArrowDown', cancelable: true });
    Object.defineProperty(e, 'target', { value: document.createElement('input') });
    Object.defineProperty(e, 'currentTarget', { value: els[0]! });
    expect(
      moverFocoEnListaVirtual(
        e,
        0,
        2,
        () => undefined,
        () => null,
      ),
    ).toBe(false);
  });
});
