/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LA CELDA DEL MES, montada. Era un `<button>` con las píldoras (`role="button"`) y el
 * «+N más» dentro: un control dentro de otro (axe `nested-interactive`), y pulsar
 * «+N más» —un `<span title>`— creaba un evento en vez de enseñar los ocultos. Ahora
 * crear es un botón de fondo, el número del día abre la vista Día y «+N más» despliega
 * la lista entera del día.
 *
 * Vistos en ROJO con la celda de antes: hay interactivos anidados, «+N más» no es un
 * botón (ni llega el Tab) y su clic llamaba a `onCreateAt`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { CalendarGrid, type GridEvent } from './CalendarGrid.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterEach(() => {
  render(null, document.body);
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

function ev(summary: string, start: string, end?: string): GridEvent {
  return { href: `/c/${summary}`, uid: summary, summary, start, ical: '', ...(end ? { end } : {}) };
}

// Seis reuniones el 18 de noviembre (caben tres chips: el resto es «+3 más») y una
// barra de todo el día que también lo cruza.
const EVENTOS: GridEvent[] = [
  ...['A', 'B', 'C', 'D', 'E', 'F'].map((n, i) =>
    ev(`Reunión ${n}`, `2026111${8}T${String(9 + i).padStart(2, '0')}0000`),
  ),
  ev('Congreso', '20261117', '20261120'),
];

interface Llamadas {
  crear: Date[];
  abrirDia: Date[];
  elegir: string[];
}

async function montar(): Promise<Llamadas> {
  const ll: Llamadas = { crear: [], abrirDia: [], elegir: [] };
  await act(() => {
    render(
      <CalendarGrid
        locale="es"
        mode="month"
        cursor={new Date(2026, 10, 9)}
        events={EVENTOS}
        onSelectEvent={(e) => ll.elegir.push(e.summary)}
        onCreateAt={(d) => ll.crear.push(d)}
        onOpenDay={(d) => ll.abrirDia.push(d)}
        onCreateRange={() => {}}
        onMoveResize={() => Promise.resolve()}
      />,
      document.body,
    );
  });
  return ll;
}

const INTERACTIVO = 'button, a[href], input, select, textarea, [role="button"], [tabindex]';

function mas(): HTMLButtonElement {
  const b = [...document.querySelectorAll('button')].find((x) => /más/.test(x.textContent ?? ''));
  if (!b) throw new Error('no hay «+N más» como botón');
  return b;
}

describe('celda del mes', () => {
  test('ningún interactivo dentro de otro', async () => {
    await montar();
    const todos = [...document.querySelectorAll<HTMLElement>(INTERACTIVO)];
    expect(todos.length).toBeGreaterThan(42);
    const anidados = todos.filter((el) => el.parentElement?.closest(INTERACTIVO));
    expect(anidados.map((el) => el.outerHTML.slice(0, 80))).toEqual([]);
  });

  test('«+N más» es un botón alcanzable con Tab y no crea', async () => {
    const ll = await montar();
    const b = mas();
    expect(b.textContent).toContain('+3');
    expect(b.tabIndex).toBe(0);
    expect(b.getAttribute('aria-haspopup')).toBe('dialog');
    await act(() => b.click());
    expect(ll.crear).toEqual([]);
    expect(b.getAttribute('aria-expanded')).toBe('true');
  });

  test('despliega TODOS los eventos del día y cada uno abre su vista rápida', async () => {
    const ll = await montar();
    await act(() => mas().click());
    const panel = document.querySelector('[role="dialog"]')!;
    expect(panel).not.toBeNull();
    const filas = [...panel.querySelectorAll('li button')].map((b) => b.textContent ?? '');
    expect(filas).toHaveLength(7);
    // La barra de todo el día primero; luego las reuniones por hora.
    expect(filas[0]).toContain('Congreso');
    expect(filas.slice(1).map((f) => f.match(/Reunión \w/)?.[0])).toEqual(
      ['A', 'B', 'C', 'D', 'E', 'F'].map((n) => `Reunión ${n}`),
    );
    await act(async () => {
      (panel.querySelectorAll('li button')[5] as HTMLButtonElement).click();
      await new Promise((r) => setTimeout(r, 5));
    });
    expect(ll.elegir).toEqual(['Reunión E']);
    expect(ll.crear).toEqual([]);
    expect(document.querySelector('[role="dialog"]')).toBeNull();
  });

  test('el hueco vacío sigue creando y el número del día abre la vista Día', async () => {
    const ll = await montar();
    const crear = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Crear evento el 18 de noviembre"]',
    )!;
    expect(crear).not.toBeNull();
    await act(() => crear.click());
    expect(ll.crear.map((d) => d.getDate())).toEqual([18]);

    const dia = document.querySelector<HTMLButtonElement>(
      'button[aria-label="Ver el día 18 de noviembre"]',
    )!;
    expect(dia?.textContent).toBe('18');
    await act(() => dia.click());
    expect(ll.abrirDia.map((d) => d.getDate())).toEqual([18]);
    expect(ll.crear).toHaveLength(1);
  });
});
