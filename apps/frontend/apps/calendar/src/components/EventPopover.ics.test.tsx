/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «DESCARGAR .ICS» DESDE LA VISTA RÁPIDA DEL EVENTO.
 *
 * `icsDescarga.ts` existía con sus tests y nadie lo llamaba: llevarse un evento a otro
 * calendario o mandárselo a alguien de fuera no tenía puerta. Ahora la vista rápida
 * ofrece «Descargar .ics» —también en los eventos de solo lectura— y guarda el `ical`
 * del evento como fichero; si el evento no trae `ical`, el botón no sale.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { EventPopover } from './EventPopover.js';
import type { GridEvent } from './CalendarGrid.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
const fetchOriginal = globalThis.fetch;
const crearOriginal = URL.createObjectURL;
const revocarOriginal = URL.revokeObjectURL;
const clickOriginal = HTMLAnchorElement.prototype.click;
afterEach(() => {
  globalThis.fetch = fetchOriginal;
  URL.createObjectURL = crearOriginal;
  URL.revokeObjectURL = revocarOriginal;
  HTMLAnchorElement.prototype.click = clickOriginal;
  render(null, document.body);
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = ((k: string) => k) as never;

// Solo el VEVENT, como lo entregan algunos servidores: lo guardado debe ir envuelto.
const VEVENT =
  'BEGIN:VEVENT\r\nUID:viaje-1@calendar.sytue.eu\r\nSUMMARY:Viaje a Lyon\r\n' +
  'DTSTART:20261120T090000Z\r\nDTEND:20261120T100000Z\r\nEND:VEVENT';

const base: GridEvent = {
  uid: 'viaje-1@calendar.sytue.eu',
  summary: 'Viaje a Lyon',
  start: '20261120T090000Z',
  end: '20261120T100000Z',
  ical: VEVENT,
} as GridEvent;

async function pintar(evento: GridEvent): Promise<void> {
  // Las notas vinculadas piden a Notes; aquí no hay Notes detrás.
  globalThis.fetch = (() =>
    Promise.resolve(new Response('{}', { status: 404 }))) as unknown as typeof fetch;
  await act(async () => {
    render(
      <EventPopover
        t={t}
        locale={'es' as never}
        event={evento}
        anchor={new DOMRect(10, 10, 100, 20)}
        accentColor="var(--color-accent)"
        onClose={() => {}}
        onEdit={() => {}}
        onDelete={() => {}}
        onDuplicate={() => {}}
      />,
      document.body,
    );
  });
}

function botonIcs(): HTMLButtonElement | undefined {
  return [...document.querySelectorAll<HTMLButtonElement>('button')].find(
    (b) => b.textContent?.trim() === 'popover.downloadIcs',
  );
}

test('descarga el evento como .ics con el asunto por nombre', async () => {
  const blobs: Blob[] = [];
  const pulsados: { href: string; download: string }[] = [];
  URL.createObjectURL = ((b: Blob) => {
    blobs.push(b);
    return 'blob:evento';
  }) as typeof URL.createObjectURL;
  URL.revokeObjectURL = () => {};
  HTMLAnchorElement.prototype.click = function (this: HTMLAnchorElement) {
    pulsados.push({ href: this.href, download: this.download });
  };

  await pintar(base);
  const boton = botonIcs();
  expect(boton).toBeDefined();
  await act(async () => {
    boton?.click();
  });

  expect(pulsados).toEqual([{ href: 'blob:evento', download: 'Viaje a Lyon.ics' }]);
  expect(blobs).toHaveLength(1);
  expect(blobs[0]?.type).toContain('text/calendar');
  const texto = await blobs[0]!.text();
  expect(texto.startsWith('BEGIN:VCALENDAR')).toBe(true);
  expect(texto).toContain('SUMMARY:Viaje a Lyon');
  expect(texto.trimEnd().endsWith('END:VCALENDAR')).toBe(true);
});

test('también se ofrece en un evento de solo lectura', async () => {
  await pintar({ ...base, readOnly: true });
  expect(document.body.textContent).toContain('popover.readOnly');
  expect(botonIcs()).toBeDefined();
});

test('sin ical no hay botón', async () => {
  await pintar({ ...base, ical: '' });
  expect(botonIcs()).toBeUndefined();
  await pintar({ ...base, ical: '', readOnly: true });
  expect(botonIcs()).toBeUndefined();
});
