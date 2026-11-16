/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * ARRASTRAR A OTRO DÍA EN EL MES, montado. Solo la rejilla horaria arrastraba: en el
 * mes, donde se replanifica casi todo, una píldora solo se podía abrir. Se simulan los
 * `pointer*` sobre una rejilla de 700 px de ancho (100 px por día) y 100 px por semana.
 *
 * Vistos en ROJO sin el arrastre: soltar la píldora del martes en el jueves no llamaba
 * a `onMoveResize`, y no se resaltaba ninguna celda de destino.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { CalendarGrid, type GridEvent } from './CalendarGrid.js';
import { monthGrid } from './dates.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();

const CURSOR = new Date(2026, 10, 9);
const DIAS = monthGrid(CURSOR);
const SEMANAS = Math.ceil(DIAS.length / 7);
const ANCHO_DIA = 100;
const ALTO_SEMANA = 100;

// happy-dom no maqueta: la rejilla mide lo que diga este rectángulo.
let rectOriginal: typeof HTMLElement.prototype.getBoundingClientRect;
beforeAll(() => {
  rectOriginal = HTMLElement.prototype.getBoundingClientRect;
  HTMLElement.prototype.getBoundingClientRect = function () {
    return new DOMRect(0, 0, ANCHO_DIA * 7, ALTO_SEMANA * SEMANAS);
  };
});
afterEach(() => {
  render(null, document.body);
});
afterAll(async () => {
  HTMLElement.prototype.getBoundingClientRect = rectOriginal;
  if (registrado) await GlobalRegistrator.unregister();
});

/** Centro de la celda del día dado. */
function centro(dia: Date): { clientX: number; clientY: number } {
  const gi = DIAS.findIndex((d) => d.getTime() === dia.getTime());
  if (gi < 0) throw new Error(`el ${dia.toDateString()} no está en la rejilla`);
  return {
    clientX: (gi % 7) * ANCHO_DIA + ANCHO_DIA / 2,
    clientY: Math.floor(gi / 7) * ALTO_SEMANA + ALTO_SEMANA / 2,
  };
}

function ev(
  summary: string,
  start: string,
  end?: string,
  extra: Partial<GridEvent> = {},
): GridEvent {
  return {
    href: `/c/${summary}`,
    uid: summary,
    summary,
    start,
    ical: '',
    calendarUrl: '/dav/c/',
    ...(end ? { end } : {}),
    ...extra,
  };
}

interface Llamadas {
  mover: Array<{ summary: string; start: Date; end: Date }>;
  elegir: string[];
  crear: Date[];
}

async function montar(eventos: GridEvent[]): Promise<Llamadas> {
  const ll: Llamadas = { mover: [], elegir: [], crear: [] };
  await act(() => {
    render(
      <CalendarGrid
        locale="es"
        mode="month"
        cursor={CURSOR}
        events={eventos}
        onSelectEvent={(e) => ll.elegir.push(e.summary)}
        onCreateAt={(d) => ll.crear.push(d)}
        onCreateRange={() => {}}
        onMoveResize={(e, start, end) => {
          ll.mover.push({ summary: e.summary, start, end });
          return Promise.resolve();
        }}
      />,
      document.body,
    );
  });
  return ll;
}

function control(nombre: string): HTMLElement {
  const b = [...document.querySelectorAll<HTMLElement>('button')].find((x) =>
    (x.title || x.getAttribute('aria-label') || '').startsWith(nombre),
  );
  if (!b) throw new Error(`no hay control «${nombre}»`);
  return b;
}

function puntero(
  tipo: string,
  donde: { clientX: number; clientY: number },
  pointerType = 'mouse',
): PointerEvent {
  return new PointerEvent(tipo, {
    ...donde,
    pointerId: 1,
    pointerType,
    isPrimary: true,
    button: 0,
    bubbles: true,
    cancelable: true,
  });
}

/** Pulsa sobre `el` en `desde`, pasa por `pasos` y suelta en el último; luego el clic. */
async function arrastrar(
  el: HTMLElement,
  desde: { clientX: number; clientY: number },
  pasos: Array<{ clientX: number; clientY: number }>,
  pointerType = 'mouse',
): Promise<void> {
  await act(() => {
    el.dispatchEvent(puntero('pointerdown', desde, pointerType));
  });
  for (const p of pasos) {
    await act(() => {
      document.dispatchEvent(puntero('pointermove', p, pointerType));
    });
  }
  const fin = pasos.at(-1) ?? desde;
  await act(() => {
    document.dispatchEvent(puntero('pointerup', fin, pointerType));
  });
  // El navegador emite el `click` del gesto tras soltar.
  await act(() => {
    el.click();
  });
}

const MARTES = new Date(2026, 10, 10);
const MIERCOLES = new Date(2026, 10, 11);
const JUEVES = new Date(2026, 10, 12);

describe('arrastrar en el mes', () => {
  test('la píldora del martes soltada en el jueves se guarda dos días después, misma hora y duración', async () => {
    const ll = await montar([ev('Dentista', '20261110T090000', '20261110T103000')]);
    await arrastrar(control('Dentista'), centro(MARTES), [centro(MIERCOLES), centro(JUEVES)]);
    expect(ll.mover).toHaveLength(1);
    expect(ll.mover[0]!.start).toEqual(new Date(2026, 10, 12, 9, 0));
    expect(ll.mover[0]!.end).toEqual(new Date(2026, 10, 12, 10, 30));
    // El clic del gesto no abre la vista rápida ni «Crear» en el jueves.
    expect(ll.elegir).toEqual([]);
    expect(ll.crear).toEqual([]);
  });

  test('mientras se arrastra, la celda de destino se resalta y la píldora queda en tenue', async () => {
    await montar([ev('Dentista', '20261110T090000', '20261110T103000')]);
    const chip = control('Dentista');
    await act(() => {
      chip.dispatchEvent(puntero('pointerdown', centro(MARTES)));
    });
    await act(() => {
      document.dispatchEvent(puntero('pointermove', centro(JUEVES)));
    });
    const destino = document.querySelectorAll('[data-destino-arrastre]');
    expect(destino).toHaveLength(1);
    expect(control('Dentista').className).toContain('opacity-50');
    // Escape lo cancela sin guardar ni dejar rastro.
    await act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    });
    expect(document.querySelectorAll('[data-destino-arrastre]')).toHaveLength(0);
  });

  test('el evento soltado se pinta ya en su día nuevo mientras se guarda', async () => {
    await montar([ev('Dentista', '20261110T090000', '20261110T103000')]);
    await arrastrar(control('Dentista'), centro(MARTES), [centro(JUEVES)]);
    // La píldora vive ahora en la columna del jueves (la 4.ª de la semana).
    const chip = control('Dentista');
    const celdas = [...document.querySelectorAll('button[aria-label^="Crear"]')].map(
      (b) => b.parentElement,
    );
    const gi = celdas.findIndex((c) => c?.contains(chip));
    expect(DIAS[gi]).toEqual(JUEVES);
  });

  test('en una serie solo se pinta movida la ocurrencia arrastrada', async () => {
    // Las ocurrencias de una serie comparten href: moverlas por href movía todas.
    const serie = (start: string, end: string): GridEvent =>
      ev('Daily', start, end, { href: '/c/daily.ics' });
    await montar([
      serie('20261110T090000', '20261110T091500'),
      serie('20261111T090000', '20261111T091500'),
    ]);
    const martes = [...document.querySelectorAll<HTMLElement>('button[title="Daily"]')][0]!;
    await arrastrar(martes, centro(MARTES), [centro(JUEVES)]);
    const dias = [...document.querySelectorAll('button[title="Daily"]')].map((chip) => {
      const celdas = [...document.querySelectorAll('button[aria-label^="Crear"]')].map(
        (b) => b.parentElement,
      );
      return DIAS[celdas.findIndex((c) => c?.contains(chip))]!.getDate();
    });
    expect(dias.sort((a, b) => a - b)).toEqual([11, 12]);
  });

  test('en una serie, mientras se arrastra, solo queda en tenue la ocurrencia cogida', async () => {
    const serie = (start: string, end: string): GridEvent =>
      ev('Daily', start, end, { href: '/c/daily.ics' });
    await montar([
      serie('20261110T090000', '20261110T091500'),
      serie('20261111T090000', '20261111T091500'),
    ]);
    const [martes] = [...document.querySelectorAll<HTMLElement>('button[title="Daily"]')];
    await act(() => {
      martes!.dispatchEvent(puntero('pointerdown', centro(MARTES)));
    });
    await act(() => {
      document.dispatchEvent(puntero('pointermove', centro(JUEVES)));
    });
    const tenues = [...document.querySelectorAll<HTMLElement>('button[title="Daily"]')].map((b) =>
      b.className.includes('opacity-50'),
    );
    expect(tenues).toEqual([true, false]);
    await act(() => {
      document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
    });
  });

  test('sin arrastre real, un clic sigue abriendo la vista rápida', async () => {
    const ll = await montar([ev('Dentista', '20261110T090000', '20261110T103000')]);
    const m = centro(MARTES);
    // Un temblor de 2 px no es un arrastre.
    await arrastrar(control('Dentista'), m, [{ clientX: m.clientX + 2, clientY: m.clientY + 1 }]);
    expect(ll.mover).toEqual([]);
    expect(ll.elegir).toEqual(['Dentista']);
  });

  test('una suscripción no se mueve', async () => {
    const ll = await montar([
      ev('Festivo', '20261110T090000', '20261110T100000', { readOnly: true }),
    ]);
    await arrastrar(control('Festivo'), centro(MARTES), [centro(JUEVES)]);
    expect(ll.mover).toEqual([]);
    expect(document.querySelectorAll('[data-destino-arrastre]')).toHaveLength(0);
  });

  test('con el dedo no se arrastra: tocar sigue siendo abrir', async () => {
    const ll = await montar([ev('Dentista', '20261110T090000', '20261110T103000')]);
    await arrastrar(control('Dentista'), centro(MARTES), [centro(JUEVES)], 'touch');
    expect(ll.mover).toEqual([]);
  });

  test('una barra de varios días cogida por su segundo día se desplaza lo que se mueve', async () => {
    // Congreso de día completo del 10 al 12 (DTEND exclusivo el 13).
    const ll = await montar([ev('Congreso', '20261110', '20261113')]);
    await arrastrar(control('Congreso'), centro(MIERCOLES), [centro(JUEVES)]);
    expect(ll.mover).toHaveLength(1);
    expect(ll.mover[0]!.start).toEqual(new Date(2026, 10, 11));
    expect(ll.mover[0]!.end).toEqual(new Date(2026, 10, 14));
  });

  test('soltar en la misma celda no guarda nada', async () => {
    const ll = await montar([ev('Dentista', '20261110T090000', '20261110T103000')]);
    const m = centro(MARTES);
    await arrastrar(control('Dentista'), m, [{ clientX: m.clientX + 30, clientY: m.clientY }]);
    expect(ll.mover).toEqual([]);
    expect(ll.elegir).toEqual([]);
  });
});
