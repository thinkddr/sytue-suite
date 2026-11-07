/*
 * ↑ EN EL CANAL SIGUIENTE ABRE LA EDICIÓN A LA PRIMERA.
 *
 * MessageView sigue montado al pasar de un canal a otro y recuerda la última
 * petición de ↑ que atendió. ChatApp olvida la petición al cambiar de canal
 * (`editRequest` → null) y la siguiente vuelve a salir con n = 1; si MessageView
 * no olvidara también su cuenta, la descartaba por «ya atendida» y la tecla se
 * perdía (el compositor ya había llamado a preventDefault).
 *
 * Se reproduce la secuencia exacta de props que manda ChatApp: #a con su ↑,
 * cambio a #b (petición a null), ↑ en #b.
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

async function pintar(messages: ChatMessage[], editRequest: { id: string; n: number } | null) {
  // act vacía los efectos (y el render que provocan) antes de mirar el DOM.
  await act(async () => {
    render(
      <MessageView
        messages={messages}
        loading={false}
        editRequest={editRequest}
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

test('↑ en el canal siguiente abre la edición a la primera pulsación', async () => {
  fingirGeometria();
  host = document.createElement('div');
  document.body.appendChild(host);
  const enA = [msg('a1', 'a')];
  const enB = [msg('b1', 'b')];

  await pintar(enA, null);
  await pintar(enA, { id: 'a1', n: 1 });
  expect(cajaDeEdicion()?.value).toBe('texto de a1');

  // Cambio de canal: ChatApp pone la petición a null y llegan los mensajes de #b.
  await pintar(enB, null);
  expect(cajaDeEdicion()).toBeNull();

  // ↑ en #b: la cuenta de ChatApp vuelve a empezar.
  await pintar(enB, { id: 'b1', n: 1 });
  expect(cajaDeEdicion()?.value).toBe('texto de b1');
});
