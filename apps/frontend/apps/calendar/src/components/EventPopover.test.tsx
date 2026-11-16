/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * «¿VAS A IR?» MARCA LA RESPUESTA QUE YA DISTE.
 *
 * Los tres botones salían iguales —«Sí» primario siempre— dijeras lo que dijeras: no
 * se sabía si la respuesta se había guardado ni cuál era. Ahora el de la respuesta dada
 * sale pulsado (`aria-pressed`), primario y con ✓; sin responder, ninguno.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { GuestStatus } from '../lib/ical.js';
import { EventPopover } from './EventPopover.js';
import type { GridEvent } from './CalendarGrid.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
const fetchOriginal = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = fetchOriginal;
  render(null, document.body);
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = ((k: string) => k) as never;

const evento: GridEvent = {
  uid: 'reunion-1@calendar.sytue.eu',
  summary: 'Seguimiento',
  start: '20261120T090000Z',
  end: '20261120T100000Z',
  ical:
    'BEGIN:VCALENDAR\r\nBEGIN:VEVENT\r\nUID:reunion-1@calendar.sytue.eu\r\n' +
    'ORGANIZER:mailto:grace@sytue.eu\r\n' +
    'ATTENDEE;PARTSTAT=TENTATIVE:mailto:ana@sytue.eu\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n',
} as GridEvent;

async function pintar(
  respuesta: GuestStatus | undefined,
  onResponder: (e: GridEvent, s: 'accepted' | 'tentative' | 'declined') => void = () => {},
): Promise<HTMLButtonElement[]> {
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
        onResponder={onResponder}
        respuesta={respuesta}
      />,
      document.body,
    );
  });
  // Un solo grupo se llama «¿vas a ir?»: la fila Editar/Duplicar/Eliminar no lo es,
  // y un lector de pantalla la anunciaría con el nombre de la de responder.
  const grupos = document.querySelectorAll('[role="group"][aria-label="popover.respond"]');
  expect(grupos).toHaveLength(1);
  const grupo = grupos[0];
  return [...(grupo?.querySelectorAll('button') ?? [])];
}

test('la respuesta dada sale pulsada y es la única primaria', async () => {
  const [si, quiza, no] = await pintar('tentative');
  expect(quiza?.getAttribute('aria-pressed')).toBe('true');
  expect(si?.getAttribute('aria-pressed')).toBe('false');
  expect(no?.getAttribute('aria-pressed')).toBe('false');
  // En @sytue/ui el secundario es el primario con borde: la elegida va sin él y
  // con la marca ✓, las otras con borde y sin marca.
  expect(quiza?.className).not.toContain('border-border-strong');
  expect(quiza?.querySelector('svg')).not.toBeNull();
  for (const otro of [si, no]) {
    expect(otro?.className).toContain('border-border-strong');
    expect(otro?.querySelector('svg')).toBeNull();
  }
});

test('sin responder, ningún botón sale marcado', async () => {
  for (const r of ['pending', undefined] as const) {
    const botones = await pintar(r);
    expect(botones).toHaveLength(3);
    for (const b of botones) {
      expect(b.getAttribute('aria-pressed')).toBe('false');
      expect(b.className).toContain('border-border-strong');
      expect(b.querySelector('svg')).toBeNull();
    }
  }
});

test('pulsar otra respuesta la manda', async () => {
  const pedidas: string[] = [];
  const [, , no] = await pintar('tentative', (_e, s) => pedidas.push(s));
  await act(async () => {
    no?.click();
  });
  expect(pedidas).toEqual(['declined']);
});
