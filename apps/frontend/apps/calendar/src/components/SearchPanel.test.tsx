/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «CARGAR MÁS» PIDE LA PÁGINA SIGUIENTE DE LA MISMA CONSULTA.
 *
 * La paginación es keyset: el cursor solo tiene sentido con el tramo [from, to) con
 * el que se pidió la primera página. Con «Próximos», recalcular el tramo con la hora
 * de ahora al pulsar «Cargar más» movía el `from` bajo el mismo cursor, y un evento
 * que empezaba entre una página y otra podía saltarse o salir dos veces.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, setSystemTime, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { SearchPanel } from './SearchPanel.js';

// Solo aquí, y se retira al acabar: el resto de la suite corre sin DOM.
const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
const fetchOriginal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchOriginal;
  setSystemTime();
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = ((k: string) => k) as never;
const resultado = (id: string) => ({
  id,
  calendar_id: 'c1',
  uid: `${id}@sytue.eu`,
  summary: `Comité ${id}`,
  dtstart: '2026-11-20T09:00:00Z',
  dtend: '2026-11-20T10:00:00Z',
  all_day: false,
});

async function hasta(cond: () => boolean): Promise<void> {
  for (let i = 0; i < 100 && !cond(); i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 20));
    });
  }
  expect(cond()).toBe(true);
}

test('«Cargar más» con «Próximos» reutiliza el tramo de la primera página', async () => {
  const pedidas: URLSearchParams[] = [];
  globalThis.fetch = (async (url: string) => {
    const q = new URL(url, 'http://x').searchParams;
    pedidas.push(q);
    const pagina = q.get('cursor')
      ? { events: [resultado('e2')], next_cursor: '' }
      : { events: [resultado('e1')], next_cursor: 'c-1' };
    return new Response(JSON.stringify(pagina), { status: 200 });
  }) as unknown as typeof fetch;

  setSystemTime(new Date('2026-11-06T10:00:00Z'));
  const host = document.createElement('div');
  document.body.appendChild(host);
  try {
    await act(async () => {
      render(
        <SearchPanel
          t={t}
          locale="es"
          open
          onClose={() => {}}
          calendars={[]}
          onOpenResult={() => {}}
        />,
        host,
      );
    });
    const periodo = document.querySelector<HTMLSelectElement>('#cal-search-period')!;
    await act(async () => {
      periodo.value = 'upcoming';
      periodo.dispatchEvent(new Event('change', { bubbles: true }));
    });
    const campo = document.querySelector<HTMLInputElement>('#cal-search')!;
    await act(async () => {
      campo.value = 'comité';
      campo.dispatchEvent(new Event('input', { bubbles: true }));
    });
    await hasta(() => pedidas.length === 1 && document.body.textContent!.includes('Comité e1'));
    expect(pedidas[0]!.get('from')).toBe('2026-11-06T10:00:00.000Z');

    // Pasa el tiempo entre una página y otra.
    setSystemTime(new Date('2026-11-06T10:05:00Z'));
    const mas = [...document.querySelectorAll('button')].find(
      (b) => b.textContent?.trim() === 'search.loadMore',
    );
    expect(mas).toBeDefined();
    await act(async () => {
      mas!.click();
    });
    await hasta(() => pedidas.length === 2);
    expect(pedidas[1]!.get('cursor')).toBe('c-1');
    expect(pedidas[1]!.get('from')).toBe('2026-11-06T10:00:00.000Z');
    expect(pedidas[1]!.get('q')).toBe('comité');
  } finally {
    render(null, host);
    host.remove();
  }
});
