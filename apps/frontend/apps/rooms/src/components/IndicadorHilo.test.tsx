/*
 * EL HILO VISTO DESDE EL CANAL (D-ROOM-05).
 *
 * Bajo una raíz con respuestas van los avatares de quienes respondieron, «N
 * respuestas» y la última respuesta, en un solo botón que abre el hilo; y sobre una
 * respuesta mandada «también al canal», la línea que dice que es de un hilo.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { roomsT } from '../i18n/index.js';
import type { ChatMessage } from '../client/chatApi.js';
import { IndicadorHilo, RespondioEnHilo } from './IndicadorHilo.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = roomsT('es');

async function pintar(nodo: preact.ComponentChild): Promise<HTMLElement> {
  const host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(nodo, host);
  });
  return host;
}

function raiz(over: Partial<ChatMessage>): ChatMessage {
  return {
    id: 'r',
    channelId: 'c',
    author: { id: 'ana', name: 'Ana' },
    body: 'tema',
    createdAt: '2026-11-10T09:00:00Z',
    ...over,
  };
}

test('el indicador lleva avatares, la cuenta y la última respuesta, y abre el hilo', async () => {
  const abiertos: string[] = [];
  const m = raiz({
    replyCount: 4,
    lastReplyAt: '2026-11-10T09:55:00Z',
    threadParticipants: [
      { id: 'bea', name: 'Bea Gil' },
      { id: 'carl', name: 'Carl Ruiz' },
    ],
  });
  const host = await pintar(
    <IndicadorHilo
      m={m}
      onOpen={(x) => abiertos.push(x.id)}
      locale="es"
      t={t}
      now={Date.parse('2026-11-10T10:00:00Z')}
    />,
  );
  try {
    const boton = host.querySelector('button')!;
    expect(boton.textContent).toContain('4 respuestas');
    expect(boton.textContent).toContain('Última respuesta hace 5 min');
    const avatares = host.querySelector('[data-avatares-hilo]')!;
    // Decorativos: el nombre accesible del botón es la cuenta y la hora.
    expect(avatares.getAttribute('aria-hidden')).toBe('true');
    expect(avatares.children).toHaveLength(2);
    await act(async () => boton.click());
    expect(abiertos).toEqual(['r']);
  } finally {
    render(null, host);
    host.remove();
  }
});

test('sin respuestas no hay indicador', async () => {
  const host = await pintar(<IndicadorHilo m={raiz({})} locale="es" t={t} />);
  try {
    expect(host.innerHTML).toBe('');
  } finally {
    host.remove();
  }
});

test('una respuesta «también al canal» dice que es de un hilo y lleva a él', async () => {
  const abiertos: string[] = [];
  const respuesta = raiz({ id: 'x', parentId: 'r', alsoInChannel: true });
  const host = await pintar(
    <RespondioEnHilo m={respuesta} onOpen={(x) => abiertos.push(x.id)} t={t} />,
  );
  try {
    expect(host.textContent).toContain('Respondió en un hilo');
    const ver = host.querySelector('button')!;
    expect(ver.textContent).toBe('Ver el hilo');
    await act(async () => ver.click());
    expect(abiertos).toEqual(['x']);
  } finally {
    render(null, host);
    host.remove();
  }
  // Una respuesta normal (solo en el hilo) o una raíz no llevan la línea.
  const otra = await pintar(<RespondioEnHilo m={raiz({ parentId: 'r' })} t={t} />);
  expect(otra.innerHTML).toBe('');
  otra.remove();
});
