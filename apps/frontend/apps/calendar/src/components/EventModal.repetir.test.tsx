/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «REPETIR» MÁS ALLÁ DE DIARIA/SEMANAL/MENSUAL: cada año, cada día laborable y ciertos
 * días de la semana. Lo que importa es lo que viaja al guardar (la regla que pinta el
 * servicio) y que reabrir la serie enseñe lo que se eligió, no «Semanalmente» a secas.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeEach, describe, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { useT } from '../i18n/index.js';
import type { Calendar } from '../client/calendarApi.js';
import { EventModal, buildRecurrence, sameRecurrence, type EditingEvent } from './EventModal.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();

const calendario = { url: '/dav/cal/mio/', displayName: 'Mío', id: 'mio' } as Calendar;
// Miércoles 11 de noviembre de 2026, 9:00.
const inicio = new Date(2026, 10, 11, 9, 0);
const fin = new Date(2026, 10, 11, 10, 0);

let enviados: { url: string; body: Record<string, unknown> }[] = [];
const fetchOriginal = globalThis.fetch;

beforeEach(() => {
  enviados = [];
  globalThis.fetch = (async (url: string | URL | Request, init?: RequestInit) => {
    const body = typeof init?.body === 'string' ? JSON.parse(init.body) : {};
    enviados.push({ url: String(url), body });
    return new Response(JSON.stringify({ ok: true, href: '/x.ics', uid: 'x' }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }) as typeof fetch;
});
afterEach(() => {
  globalThis.fetch = fetchOriginal;
  render(null, document.body);
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

function Modal({ editing }: { editing: EditingEvent }) {
  const t = useT('es');
  return (
    <EventModal
      t={t}
      locale="es"
      open
      calendars={[calendario]}
      editing={editing}
      onClose={() => {}}
      onSaved={() => {}}
    />
  );
}

async function abrir(editing: Partial<EditingEvent> = {}): Promise<void> {
  await act(() => {
    render(
      <Modal
        editing={{
          href: null,
          summary: 'Pilates',
          start: inicio,
          end: fin,
          calendarUrl: calendario.url,
          ...editing,
        }}
      />,
      document.body,
    );
  });
}

function repetir(): HTMLSelectElement {
  const el = document.getElementById('ev-repeat');
  if (!el) throw new Error('sin «Repetir»');
  return el as HTMLSelectElement;
}

async function elegir(valor: string): Promise<void> {
  await act(() => {
    const sel = repetir();
    sel.value = valor;
    sel.dispatchEvent(new Event('change', { bubbles: true }));
  });
}

/** Los botones de los días, por su nombre accesible. */
function dias(): HTMLButtonElement[] {
  const grupo = document.querySelector('[role="group"][aria-labelledby="ev-repeat-days"]');
  return grupo ? [...grupo.querySelectorAll('button')] : [];
}

function encendidos(): string[] {
  return dias()
    .filter((b) => b.getAttribute('aria-pressed') === 'true')
    .map((b) => b.getAttribute('aria-label') ?? '');
}

async function guardar(): Promise<Record<string, unknown>> {
  const boton = [...document.querySelectorAll('button')].find(
    (b) => b.textContent?.trim() === 'Guardar',
  );
  if (!boton) throw new Error('sin «Guardar»');
  await act(async () => {
    boton.click();
    await new Promise((r) => setTimeout(r, 0));
  });
  const post = enviados.find((e) => e.url.endsWith('/api/event'));
  if (!post) throw new Error(`no se guardó: ${JSON.stringify(enviados)}`);
  return (post.body['eventInput'] ?? {}) as Record<string, unknown>;
}

describe('«Repetir» ofrece las repeticiones que faltaban', () => {
  test('el selector tiene «Anualmente» y «Cada día laborable»', async () => {
    await abrir();
    const valores = [...repetir().options].map((o) => o.value);
    expect(valores).toEqual(['', 'DAILY', 'WEEKLY', 'MONTHLY', 'YEARLY', 'WEEKDAYS']);
  });

  test('«Cada día laborable» guarda una semanal de lunes a viernes', async () => {
    await abrir();
    await elegir('WEEKDAYS');
    expect(dias()).toHaveLength(0); // no es una semanal con días a elegir
    const ev = await guardar();
    expect(ev['recurrence']).toEqual({ freq: 'WEEKLY', byday: ['MO', 'TU', 'WE', 'TH', 'FR'] });
  });

  test('«Anualmente» guarda una YEARLY', async () => {
    await abrir();
    await elegir('YEARLY');
    const ev = await guardar();
    expect(ev['recurrence']).toEqual({ freq: 'YEARLY' });
  });

  test('una semanal enciende el día del inicio y deja añadir otros', async () => {
    await abrir();
    await elegir('WEEKLY');
    expect(dias()).toHaveLength(7);
    expect(encendidos()).toEqual(['miércoles']);
    // Sin tocar los días, la semanal de siempre (sin BYDAY).
    await act(() => dias()[0]!.click()); // + lunes
    expect(encendidos()).toEqual(['lunes', 'miércoles']);
    const ev = await guardar();
    expect(ev['recurrence']).toEqual({ freq: 'WEEKLY', byday: ['MO', 'WE'] });
  });

  test('el último día encendido no se apaga', async () => {
    await abrir();
    await elegir('WEEKLY');
    await act(() => dias()[2]!.click()); // miércoles, el único
    expect(encendidos()).toEqual(['miércoles']);
    const ev = await guardar();
    expect(ev['recurrence']).toEqual({ freq: 'WEEKLY' });
  });
});

describe('reabrir la serie enseña lo elegido', () => {
  test('lunes a viernes vuelve como «Cada día laborable»', async () => {
    await abrir({
      href: '/x.ics',
      recurrence: { freq: 'WEEKLY', byday: ['MO', 'TU', 'WE', 'TH', 'FR'] },
    });
    expect(repetir().value).toBe('WEEKDAYS');
  });

  test('lunes y miércoles vuelve como semanal con esos dos días', async () => {
    await abrir({ href: '/x.ics', recurrence: { freq: 'WEEKLY', byday: ['MO', 'WE'] } });
    expect(repetir().value).toBe('WEEKLY');
    expect(encendidos()).toEqual(['lunes', 'miércoles']);
  });

  test('una anual vuelve como «Anualmente»', async () => {
    await abrir({ href: '/x.ics', recurrence: { freq: 'YEARLY' } });
    expect(repetir().value).toBe('YEARLY');
  });
});

describe('buildRecurrence', () => {
  test('cada día laborable lleva siempre los cinco días, sea cual sea el inicio', () => {
    expect(buildRecurrence('WEEKDAYS', 'count', '10', '', null, 'SU')).toEqual({
      freq: 'WEEKLY',
      byday: ['MO', 'TU', 'WE', 'TH', 'FR'],
      count: 10,
    });
  });
  test('los días se ordenan de lunes a domingo', () => {
    expect(buildRecurrence('WEEKLY', 'never', '', '', ['FR', 'MO'], 'MO')).toEqual({
      freq: 'WEEKLY',
      byday: ['MO', 'FR'],
    });
  });
  test('un BYDAY con solo el día del inicio es la misma semanal', () => {
    // «FREQ=WEEKLY;BYDAY=WE» de otro cliente, abierta un miércoles: buildRecurrence la
    // rehace sin BYDAY, y eso no es cambiar la regla (si no, se reescribía al tocar el
    // título).
    const abierta = { freq: 'WEEKLY', byday: ['WE'] } as const;
    const guardada = buildRecurrence('WEEKLY', 'never', '', '', ['WE'], 'WE');
    expect(guardada).toEqual({ freq: 'WEEKLY' });
    expect(sameRecurrence(guardada, { ...abierta, byday: [...abierta.byday] }, 'WE')).toBe(true);
    // Otro día sí cambia la regla.
    expect(sameRecurrence({ freq: 'WEEKLY' }, { freq: 'WEEKLY', byday: ['MO'] }, 'WE')).toBe(false);
  });
});
