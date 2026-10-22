// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { EventNotes } from '../../../calendar/src/components/EventNotes.js';

if (typeof document === 'undefined') GlobalRegistrator.register();
const originalFetch = globalThis.fetch;
const t = ((key: string) => key) as never;
const hosts: HTMLElement[] = [];
afterEach(() => {
  for (const h of hosts) {
    render(null, h);
    h.remove();
  }
  hosts.length = 0;
  globalThis.fetch = originalFetch;
  delete (window as { __sytueBase?: string }).__sytueBase;
});
const tick = () => new Promise((resolve) => setTimeout(resolve, 60));

test('consulta el prefijo de Calendar y vuelve a la nota sin repetir /notes', async () => {
  (window as { __sytueBase?: string }).__sytueBase = '/calendar/acme';
  const requested: string[] = [];
  globalThis.fetch = (async (url: string | URL | Request) => {
    requested.push(String(url));
    return Response.json({
      notes: [{ id: 'n 1', notebook_id: 'cuaderno', title: 'Preparar reunión', encrypted: false }],
      notesUrl: 'https://notes.sytue.eu/notes/acme/',
    });
  }) as typeof fetch;
  const h = document.createElement('div');
  document.body.append(h);
  hosts.push(h);
  render(<EventNotes t={t} eventUid="evento+uno@calendar" summary="Reunión" />, h);
  await tick();
  await tick();
  expect(requested).toEqual(['/calendar/acme/api/notes?event=evento%2Buno%40calendar']);
  const links = [...h.querySelectorAll('a')].map((a) => a.href);
  expect(links).toEqual([
    'https://notes.sytue.eu/notes/acme/?notebook=cuaderno&note=n%201',
    'https://notes.sytue.eu/notes/acme/?link=calendar_event:evento%2Buno%40calendar&label=Reuni%C3%B3n',
  ]);
});
