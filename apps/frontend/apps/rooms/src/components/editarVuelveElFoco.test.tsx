/*
 * CERRAR UNA EDICIÓN ABIERTA CON ↑ DEVUELVE EL FOCO AL COMPOSITOR.
 *
 * Con el compositor vacío, ↑ abre la edición del último mensaje propio y le da el
 * foco. Al cerrarla —Intro para guardar o Esc para cancelar— el textarea se desmonta;
 * sin devolver el foco a quien lo tenía, cae al <body> y lo siguiente que se teclea no
 * va a ninguna parte (design.md §2.6, «retorno de foco al cerrar»). Una edición
 * reabierta desde «Editar» del menú del mensaje no lo devuelve: ahí no se pidió desde
 * el compositor. Gemelo en Fyne: TestCerrarLaEdicionDeFlechaVuelveAlCompositor
 * (apps/fyne/apps/rooms/sala_test.go).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { ChatMessage } from '../lib/chat.js';
import { MessageView } from './MessageView.js';

// Se registra solo aquí y se retira al acabar: el resto de la suite de Rooms prueba
// los BFF con el fetch/Request/Response de Bun, y los de happy-dom los rompían.
const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = ((k: string) => k) as never;

function msg(id: string, channelId: string): ChatMessage {
  return {
    id,
    channelId,
    author: { id: 'yo', name: 'Yo' },
    body: `texto de ${id}`,
    createdAt: '2026-11-05T10:00:00Z',
    mine: true,
  };
}

// happy-dom no tiene layout: sin un viewport con alto, la lista virtual no pinta
// ninguna fila (ver VirtualList.test.tsx). Se finge la geometría y se restaura.
const geometria = new Map<string, PropertyDescriptor | undefined>();
function fingirGeometria(): void {
  for (const [campo, px] of [
    ['clientHeight', 600],
    ['clientWidth', 800],
    ['offsetHeight', 600],
    ['offsetWidth', 800],
  ] as const) {
    geometria.set(campo, Object.getOwnPropertyDescriptor(HTMLElement.prototype, campo));
    Object.defineProperty(HTMLElement.prototype, campo, { configurable: true, get: () => px });
  }
}

let host: HTMLElement | null = null;
afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
  }
  host = null;
  for (const [campo, d] of geometria) {
    if (d) Object.defineProperty(HTMLElement.prototype, campo, d);
    else Reflect.deleteProperty(HTMLElement.prototype, campo);
  }
  geometria.clear();
});

async function pintar(
  messages: ChatMessage[],
  editRequest: { id: string; n: number } | null,
  onEdit?: (id: string, body: string) => void,
) {
  // act vacía los efectos (y el render que provocan) antes de mirar el DOM.
  await act(async () => {
    render(
      <MessageView
        messages={messages}
        loading={false}
        editRequest={editRequest}
        {...(onEdit ? { onEdit } : {})}
        locale="es"
        t={t}
      />,
      host!,
    );
  });
}

function cajaDeEdicion(): HTMLTextAreaElement | null {
  return host!.querySelector('textarea[aria-label="message.edit"]');
}

async function tecla(el: HTMLElement, key: string): Promise<void> {
  await act(async () => {
    el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
  });
}

for (const [tecla_, nombre] of [
  ['Enter', 'Intro guarda'],
  ['Escape', 'Esc cancela'],
] as const) {
  test(`${nombre} y el foco vuelve al compositor`, async () => {
    fingirGeometria();
    host = document.createElement('div');
    document.body.appendChild(host);
    const compositor = document.createElement('textarea');
    document.body.appendChild(compositor);
    try {
      const enA = [msg('a1', 'a')];
      await pintar(enA, null);
      compositor.focus();
      await pintar(enA, { id: 'a1', n: 1 });
      const caja = cajaDeEdicion();
      expect(document.activeElement).toBe(caja);
      await tecla(caja!, tecla_);
      expect(cajaDeEdicion()).toBeNull();
      expect(document.activeElement).toBe(compositor);
    } finally {
      compositor.remove();
    }
  });
}

test('reabierta desde «Editar» del menú, cerrarla no lleva el foco al compositor', async () => {
  fingirGeometria();
  host = document.createElement('div');
  document.body.appendChild(host);
  const compositor = document.createElement('textarea');
  document.body.appendChild(compositor);
  try {
    const enA = [msg('a1', 'a')];
    const onEdit = (): void => {};
    await pintar(enA, null, onEdit);
    compositor.focus();
    await pintar(enA, { id: 'a1', n: 1 }, onEdit); // abierta con ↑…
    expect(document.activeElement).toBe(cajaDeEdicion());
    const disparador = host!.querySelector<HTMLButtonElement>(
      'button[aria-label="message.moreActions"]',
    );
    await act(async () => disparador!.click());
    const editar = Array.from(document.querySelectorAll<HTMLElement>('[role="menuitem"]')).find(
      (el) => el.textContent?.includes('action.edit'),
    );
    await act(async () => editar!.click()); // …y reabierta desde el menú (`startEdit`)
    const caja = cajaDeEdicion();
    expect(caja).not.toBeNull();
    caja!.focus();
    await tecla(caja!, 'Escape');
    expect(cajaDeEdicion()).toBeNull();
    expect(document.activeElement).not.toBe(compositor);
  } finally {
    compositor.remove();
  }
});
