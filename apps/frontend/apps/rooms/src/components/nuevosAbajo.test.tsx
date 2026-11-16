/*
 * «↓ N MENSAJES NUEVOS» CUANDO SE LEE EL HISTORIAL MÁS ARRIBA.
 *
 * La lista solo sigue al fondo si ya estaba abajo; quien se ha subido a leer no se
 * enteraba de lo que llegaba. Se prueba la píldora flotante: cuenta lo ajeno llegado
 * con el scroll arriba, no cuenta lo propio, pulsarla baja y la retira, y llegar abajo
 * a mano también la retira.
 *
 * happy-dom no tiene layout: «estar abajo» se finge sobre el handle real de la lista
 * (MessageView pregunta por él, igual que con el cap de ChatApp).
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import type { VirtualListHandle } from '@sytue/ui';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { createRef, render } from 'preact';
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

const t = ((k: string, v?: { count?: number }) =>
  v?.count === undefined ? k : `${k}:${v.count}`) as never;

function msg(id: string, mine = false): ChatMessage {
  return {
    id,
    channelId: 'c1',
    author: mine ? { id: 'yo', name: 'Yo' } : { id: 'ana', name: 'Ana' },
    body: `texto de ${id}`,
    createdAt: '2026-11-05T10:00:00Z',
    mine,
  };
}

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

const handle = createRef<VirtualListHandle>();
let abajo = true;
let bajadas = 0;

async function pintar(messages: ChatMessage[]): Promise<void> {
  await act(async () => {
    render(
      <MessageView messages={messages} loading={false} handleRef={handle} locale="es" t={t} />,
      host!,
    );
  });
  // El handle es el mismo objeto mientras viva la lista: basta con fingirlo una vez.
  const h = handle.current!;
  h.isAtBottom = () => abajo;
  h.scrollToBottom = () => {
    bajadas++;
    abajo = true;
  };
}

function pildora(): HTMLButtonElement | null {
  return (
    [...host!.querySelectorAll('button')].find((b) =>
      b.textContent?.includes('message.newBelow'),
    ) ?? null
  );
}

async function montar(): Promise<ChatMessage[]> {
  fingirGeometria();
  host = document.createElement('div');
  document.body.appendChild(host);
  abajo = true;
  bajadas = 0;
  const base = [msg('a'), msg('b')];
  await pintar(base);
  return base;
}

test('con el scroll arriba, dos mensajes ajenos dan «2 mensajes nuevos»; los propios no', async () => {
  const base = await montar();
  expect(pildora()).toBeNull();
  abajo = false;
  await pintar([...base, msg('c')]);
  await pintar([...base, msg('c'), msg('d', true)]);
  await pintar([...base, msg('c'), msg('d', true), msg('e')]);
  expect(pildora()?.textContent).toContain('message.newBelow:2');
  expect(bajadas).toBe(0);
});

test('pulsarla baja al final y la retira', async () => {
  const base = await montar();
  abajo = false;
  await pintar([...base, msg('c')]);
  await act(async () => pildora()!.click());
  expect(bajadas).toBe(1);
  expect(pildora()).toBeNull();
});

test('bajar a mano hasta el final también la retira', async () => {
  const base = await montar();
  abajo = false;
  await pintar([...base, msg('c')]);
  expect(pildora()).not.toBeNull();
  abajo = true;
  const lista = host!.querySelector('[aria-label="message.listAria"]')!;
  await act(async () => {
    lista.dispatchEvent(new Event('scroll'));
  });
  expect(pildora()).toBeNull();
});

test('estando abajo no hay píldora: la lista sigue al fondo', async () => {
  const base = await montar();
  await pintar([...base, msg('c')]);
  expect(pildora()).toBeNull();
  expect(bajadas).toBe(1);
});

test('otra lista (otro canal) reinicia la cuenta', async () => {
  const base = await montar();
  abajo = false;
  await pintar([...base, msg('c')]);
  expect(pildora()).not.toBeNull();
  await pintar([msg('x'), msg('y')]);
  expect(pildora()).toBeNull();
});
