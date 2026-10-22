// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { Notebook } from '@sytue/api-clients';
import { NotebookDashboard } from './NotebookDashboard.js';
if (typeof document === 'undefined') GlobalRegistrator.register();
const t = ((key: string) => key) as never;
let host: HTMLElement | null = null;
afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
    host = null;
  }
});
const tick = () => new Promise((resolve) => setTimeout(resolve, 0));
function montar(capture: () => Promise<boolean>, draft: (s: string) => void, focus: () => void) {
  host = document.createElement('div');
  document.body.append(host);
  render(
    <NotebookDashboard
      t={t}
      locale="es"
      notebook={{ id: 'nb', title: 'Ideas', layout: 'list' } as Notebook}
      notes={[]}
      disabled={false}
      enfoque="pending"
      openingDay={false}
      initialDraft="[ ] Preparar reunión"
      onDraft={draft}
      onEnfoque={focus}
      hideFinished={false}
      onHideFinished={() => {}}
      onDay={() => {}}
      onCarry={() => {}}
      onCapture={capture}
      onTemplate={async () => false}
    />,
    host,
  );
  return host;
}
test('un fallo conserva la captura; el éxito borra el borrador y vuelve a Todas', async () => {
  let success = false;
  const drafts: string[] = [];
  let focuses = 0;
  const h = montar(
    async () => success,
    (s) => drafts.push(s),
    () => focuses++,
  );
  h.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await tick();
  await tick();
  expect(h.querySelector('input')!.value).toBe('[ ] Preparar reunión');
  expect(drafts).toEqual([]);
  expect(focuses).toBe(0);
  success = true;
  h.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await tick();
  await tick();
  expect(h.querySelector('input')!.value).toBe('');
  expect(drafts).toEqual(['']);
  expect(focuses).toBe(1);
});
test('terminar una captura después de cambiar de cuaderno no cambia el filtro nuevo', async () => {
  let finish: (v: boolean) => void = () => {};
  const pending = new Promise<boolean>((resolve) => (finish = resolve));
  const drafts: string[] = [];
  let focuses = 0;
  const h = montar(
    () => pending,
    (s) => drafts.push(s),
    () => focuses++,
  );
  h.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await tick();
  render(null, h);
  finish(true);
  await tick();
  expect(drafts).toEqual(['']);
  expect(focuses).toBe(0);
});

test('un día pasado con pendientes ofrece trasladarlas; uno sin pendientes, no', () => {
  // El botón de Sytue Calendar de un diario compone su URL con la base de la app.
  process.env['BASE_URL'] ??= '/';
  const pintar = (notes: unknown[], date: string) => {
    let carries = 0;
    const h = document.createElement('div');
    document.body.append(h);
    render(
      <NotebookDashboard
        t={t}
        locale="es"
        notebook={{ id: 'd', title: 'Día', layout: 'list', daily_date: date } as Notebook}
        notes={notes as never}
        disabled={false}
        enfoque="all"
        openingDay={false}
        initialDraft=""
        onDraft={() => {}}
        onEnfoque={() => {}}
        hideFinished={false}
        onHideFinished={() => {}}
        onDay={() => {}}
        onCarry={() => carries++}
        onCapture={async () => false}
        onTemplate={async () => false}
      />,
      h,
    );
    const boton = [...h.querySelectorAll('button')].find(
      (b) => b.textContent === 'daily.carryToToday',
    );
    boton?.click();
    render(null, h);
    h.remove();
    return { hay: Boolean(boton), carries };
  };
  const pendiente = { id: 'a', kind: 'todo', checked: false };
  const cancelada = { id: 'b', kind: 'todo', checked: false, cancelled_at: '2020-01-02T00:00:00Z' };
  expect(pintar([pendiente], '2020-01-01')).toEqual({ hay: true, carries: 1 });
  expect(pintar([cancelada], '2020-01-01').hay).toBe(false);
  expect(pintar([pendiente], '2999-01-01').hay).toBe(false);
});
