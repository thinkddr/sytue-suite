/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * EL MES A 1280, medido el 2026-11-24 (roadmap, «Rendimiento y estética de la web»):
 *
 *  · Las barras de varios días medían 18 px y las fichas con hora 24 (WCAG 2.5.8): ahora
 *    las barras también, en el mes y en la franja «todo el día» de la semana.
 *  · Las fichas se cortaban a los diez caracteres («08:00 Reunión s…»): la única ficha de
 *    una celda sin barras sigue en una segunda línea.
 *  · Y cuántas fichas caben sale del ALTO medido de la semana, no solo de contar carriles:
 *    con la barra a 24, la segunda ficha de un día con barra perdía el borde de abajo, y
 *    ya antes la cuarta de un día lleno no se veía sin «+N más» que lo dijera.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { CalendarGrid, EventChip, fichasQueCaben, type GridEvent } from './CalendarGrid.js';

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

async function montar(mode: 'month' | 'week', events: GridEvent[]): Promise<void> {
  await act(() => {
    render(
      <CalendarGrid
        locale="es"
        mode={mode}
        cursor={new Date(2026, 10, 18)}
        events={events}
        onSelectEvent={() => {}}
        onCreateAt={() => {}}
        onOpenDay={() => {}}
        onCreateRange={() => {}}
        onMoveResize={() => Promise.resolve()}
      />,
      document.body,
    );
  });
}

const barra = (titulo: string): HTMLElement =>
  document.querySelector<HTMLElement>(`button[title="${titulo}"]`)!;

describe('barras de varios días', () => {
  test('en el mes miden 24 px, como las fichas', async () => {
    await montar('month', [ev('Congreso', '20261117', '20261120')]);
    expect(barra('Congreso').style.height).toBe('24px');
  });

  test('en la franja «todo el día» de la semana, también', async () => {
    await montar('week', [ev('Congreso', '20261117', '20261120')]);
    expect(barra('Congreso').style.height).toBe('24px');
  });

  test('dos barras apiladas no se tocan: 2 px entre una y otra', async () => {
    await montar('month', [
      ev('Congreso', '20261117', '20261120'),
      ev('Feria', '20261117', '20261119'),
    ]);
    const tops = ['Congreso', 'Feria']
      .map((n) => parseFloat(barra(n).style.top))
      .sort((a, b) => a - b);
    expect(tops[1]! - tops[0]!).toBe(26);
  });
});

describe('la ficha sola de su celda', () => {
  test('va en dos líneas, con la hora y el título en el mismo párrafo', async () => {
    await montar('month', [ev('Reunión semanal del equipo', '20261103T080000')]);
    const ficha = barra('Reunión semanal del equipo');
    const parrafo = ficha.querySelector<HTMLElement>('.line-clamp-2, [class*="line-clamp-2"]')!;
    expect(parrafo).not.toBeNull();
    expect(parrafo.textContent).toBe('08:00Reunión semanal del equipo');
  });

  test('con una barra en la semana, o con otra ficha al lado, en una', async () => {
    await montar('month', [
      ev('Congreso', '20261117', '20261120'),
      ev('Revisión', '20261116T090000'),
      ev('Dentista', '20261110T090000'),
      ev('Cena', '20261110T180000'),
    ]);
    for (const n of ['Revisión', 'Dentista', 'Cena'])
      expect(barra(n).querySelector('[class*="line-clamp-2"]')).toBeNull();
  });

  test('a menos de 640 px de alto de ventana se queda en una (no cabría)', () => {
    const arbol = EventChip({
      locale: 'es',
      event: ev('Reunión semanal del equipo', '20261103T080000') as never,
      date: new Date(2026, 10, 3, 8),
      color: '#1e50a8',
      dosLineas: true,
      onClick: () => {},
    }) as unknown as { props: { children: unknown[] } };
    const clases = JSON.stringify(arbol);
    expect(clases).toContain('truncate');
    expect(clases).toContain('[@media(min-height:40rem)]:line-clamp-2');
  });
});

describe('cuántas fichas caben', () => {
  // A 1280×800 una semana del mes mide 107,8 px.
  const SEMANA = 107.8;

  test('sin medir (primer pintado, DOM sin alturas), manda el presupuesto de carriles', () => {
    expect(fichasQueCaben(0, 0, 5)).toBeNull();
  });

  test('sin barras caben dos; con tres, dos y «+1 más»', () => {
    expect(fichasQueCaben(SEMANA, 0, 1)).toBe(1);
    expect(fichasQueCaben(SEMANA, 0, 2)).toBe(2);
    // Tres no caben: se deja sitio a «+N más» (16 px) bajo las dos que sí.
    expect(fichasQueCaben(SEMANA, 0, 3)).toBe(2);
  });

  test('con una fila de barras, una ficha entera y «+1 más», no dos cortadas', () => {
    expect(fichasQueCaben(SEMANA, 24, 1)).toBe(1);
    expect(fichasQueCaben(SEMANA, 24, 2)).toBe(1);
  });

  test('con barras escondidas, «+N más» siempre tiene su sitio', () => {
    expect(fichasQueCaben(SEMANA, 0, 2, true)).toBe(2);
    expect(fichasQueCaben(SEMANA, 24, 1, true)).toBe(1);
    expect(fichasQueCaben(SEMANA, 48, 1, true)).toBe(0);
  });

  test('en una ventana alta caben más', () => {
    // 1920×1080: unas 155 px por semana.
    expect(fichasQueCaben(155, 0, 4)).toBe(4);
    expect(fichasQueCaben(155, 0, 6)).toBe(3);
  });

  test('nunca menos de cero', () => {
    expect(fichasQueCaben(40, 50, 3)).toBe(0);
  });
});
