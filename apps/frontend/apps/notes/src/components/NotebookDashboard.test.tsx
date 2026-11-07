// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Daniel Delgado Ramos
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { NoteInput, Notebook } from '@sytue/api-clients';
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
function montar(
  capture: () => Promise<boolean>,
  draft: (s: string) => void,
  focus: () => void,
  dock?: HTMLElement,
) {
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
      {...(dock ? { captureContainer: dock } : {})}
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
test('la captura del pie es un solo formulario y conserva borrador, submit y foco fuera del scroll', async () => {
  const dock = document.createElement('div');
  document.body.append(dock);
  try {
    let capturas = 0;
    const h = montar(
      async () => {
        capturas++;
        return true;
      },
      () => {},
      () => {},
      dock,
    );
    expect(h.querySelector('form')).toBeNull();
    expect(dock.querySelectorAll('form')).toHaveLength(1);
    expect(dock.querySelector('input')!.value).toBe('[ ] Preparar reunión');
    dock.querySelector('input')!.focus();
    dock
      .querySelector('form')!
      .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
    await tick();
    await tick();
    expect(capturas).toBe(1);
    expect(dock.querySelector('input')!.value).toBe('');
    expect(document.activeElement).toBe(dock.querySelector('input'));
  } finally {
    render(null, host!);
    dock.remove();
  }
});
test('puede seguir escribiendo mientras guarda sin perder el siguiente borrador', async () => {
  let finalizar: (ok: boolean) => void = () => {};
  const pendiente = new Promise<boolean>((resolve) => {
    finalizar = resolve;
  });
  const drafts: string[] = [];
  const h = montar(
    () => pendiente,
    (s) => drafts.push(s),
    () => {},
  );
  const input = h.querySelector('input')!;
  input.focus();
  h.querySelector('form')!.dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await tick();
  expect(input.disabled).toBe(false);
  input.value = 'Lo siguiente';
  input.dispatchEvent(new Event('input', { bubbles: true }));
  await tick();
  finalizar(true);
  await tick();
  await tick();
  expect(input.value).toBe('Lo siguiente');
  expect(drafts).toEqual(['Lo siguiente']);
});
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

/** Un `paste` con texto plano, como el del navegador. */
function pegarEn(el: HTMLElement, texto: string): Event {
  const ev = new Event('paste', { bubbles: true, cancelable: true });
  Object.defineProperty(ev, 'clipboardData', {
    value: { getData: (tipo: string) => (tipo === 'text/plain' ? texto : '') },
  });
  el.dispatchEvent(ev);
  return ev;
}

/** La captura con su borrador, y lo que llega al servicio. */
function montarCaptura(
  responder: (input: NoteInput, n: number) => Promise<boolean>,
  initialDraft = '',
  pegarRef?: { current: ((texto: string) => boolean) | null },
) {
  const enviadas: NoteInput[] = [];
  const drafts: string[] = [];
  host = document.createElement('div');
  document.body.append(host);
  render(
    <NotebookDashboard
      t={t}
      locale="es"
      notebook={{ id: 'nb', title: 'Ideas', layout: 'list' } as Notebook}
      notes={[]}
      disabled={false}
      enfoque="all"
      openingDay={false}
      initialDraft={initialDraft}
      onDraft={(s) => drafts.push(s)}
      onEnfoque={() => {}}
      hideFinished={false}
      onHideFinished={() => {}}
      onDay={() => {}}
      onCarry={() => {}}
      onCapture={(input) => {
        enviadas.push(input);
        return responder(input, enviadas.length);
      }}
      onTemplate={async () => false}
      {...(pegarRef ? { pegarRef } : {})}
    />,
    host,
  );
  return { input: host.querySelector('input')!, enviadas, drafts };
}

const hasta = async (ok: () => boolean) => {
  for (let i = 0; i < 50 && !ok(); i++) await tick();
};

test('pegar varias líneas crea una nota por línea, en orden y con la sintaxis de la captura', async () => {
  const { input, enviadas } = montarCaptura(async () => true);
  const ev = pegarEn(input, '- Comprar pan\n\n[x] Llamar a Ana\r\n---\nLa reunión es el jueves\n');
  expect(ev.defaultPrevented).toBe(true);
  await hasta(() => enviadas.length === 4 && input.value === '');
  expect(enviadas).toEqual([
    { kind: 'todo', title: 'Comprar pan', checked: false },
    { kind: 'todo', title: 'Llamar a Ana', checked: true },
    { kind: 'text', content: '<hr>' },
    { kind: 'text', content: '<p>La reunión es el jueves</p>' },
  ]);
  expect(input.value).toBe('');
});

test('pegar UNA línea la deja en el campo, como siempre', async () => {
  const { input, enviadas } = montarCaptura(async () => true);
  expect(pegarEn(input, 'Una sola idea\n').defaultPrevented).toBe(false);
  await tick();
  expect(enviadas).toEqual([]);
});

test('un fallo para en esa línea y lo que no ha salido se queda en la captura', async () => {
  const { input, enviadas, drafts } = montarCaptura(async (_i, n) => n !== 2);
  pegarEn(input, 'uno\ndos\ntres');
  await hasta(() => enviadas.length === 2 && input.value === 'dos tres');
  await tick();
  expect(enviadas).toHaveLength(2);
  // El campo es de una línea: enseña espacios, pero el borrador guarda los saltos, e
  // Intro sigue sacando una nota por línea.
  expect(input.value).toBe('dos tres');
  expect(drafts.at(-1)).toBe('dos\ntres');
  host!
    .querySelector('form')!
    .dispatchEvent(new Event('submit', { bubbles: true, cancelable: true }));
  await hasta(() => enviadas.length === 4);
  expect(enviadas.slice(2).map((i) => i.content)).toEqual(['<p>dos</p>', '<p>tres</p>']);
});

test('como mucho cincuenta de una vez: el resto se queda en la captura', async () => {
  const { input, enviadas } = montarCaptura(async () => true);
  pegarEn(input, Array.from({ length: 53 }, (_, i) => `l${i + 1}`).join('\n'));
  await hasta(() => enviadas.length === 50 && input.value === 'l51 l52 l53');
  expect(enviadas).toHaveLength(50);
  expect(input.value).toBe('l51 l52 l53');
});

test('lo pegado sobre el cuaderno sale por la captura sin mandar lo que estaba a medias', async () => {
  const pegarRef: { current: ((texto: string) => boolean) | null } = { current: null };
  const { input, enviadas } = montarCaptura(async () => true, 'a medias', pegarRef);
  await hasta(() => pegarRef.current !== null);
  expect(pegarRef.current?.('una sola')).toBe(false);
  expect(pegarRef.current?.('uno\ndos')).toBe(true);
  await hasta(() => enviadas.length === 2 && input.value === 'a medias');
  expect(enviadas.map((i) => i.content)).toEqual(['<p>uno</p>', '<p>dos</p>']);
  expect(input.value).toBe('a medias');
});
