/*
 * EL COMPOSITOR NO OFRECE LO QUE EL CIFRADO NO DEJA HACER.
 *
 * Programar, encuesta, tarea y pedir acuse los guarda o los cuenta el servicio, que con
 * el cifrado de extremo a extremo no ve el mensaje: se ofrecían y fallaban, o publicaban
 * una encuesta que nadie podía votar. Con cifrado siguen A LA VISTA pero apagados, con
 * una línea que dice por qué; las tarjetas y los acuses ya publicados se pintan en solo
 * lectura con `matrix.unsupported`. Sin cifrado, todo sigue igual. Gemelo en Fyne:
 * apps/fyne/apps/rooms/cifrado_test.go.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render, type JSX } from 'preact';
import { act } from 'preact/test-utils';
import { roomsT } from '../i18n/index.js';
import type { ChatMessage, MessageCard } from '../lib/chat.js';
import { CardView } from './CardView.js';
import { Composer } from './Composer.js';
import { MessageView } from './MessageView.js';

// Solo aquí, y se retira al acabar: el resto de la suite prueba los BFF con el
// fetch/Request/Response de Bun.
const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = roomsT('es');
const NOTA = t('matrix.notYetEncrypted');

let host: HTMLElement | null = null;
afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
  }
  host = null;
  document.body.innerHTML = '';
});

async function pintar(vista: JSX.Element): Promise<HTMLElement> {
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(vista, host!);
  });
  return host;
}

async function pulsar(el: HTMLElement): Promise<void> {
  await act(async () => {
    el.click();
  });
}

function compositor(cifrado: boolean): JSX.Element {
  return (
    <Composer
      channelName="general"
      channelId="c1"
      advanced
      cifrado={cifrado}
      onSend={() => {}}
      locale="es"
      t={t}
    />
  );
}

function boton(raiz: ParentNode, nombre: string): HTMLButtonElement {
  const b = [...raiz.querySelectorAll<HTMLButtonElement>('button')].find(
    (x) => x.getAttribute('aria-label') === nombre || x.textContent?.trim() === nombre,
  );
  if (!b) throw new Error(`sin botón «${nombre}»`);
  return b;
}

function itemDeMenu(nombre: string): HTMLButtonElement {
  const it = [...document.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')].find(
    (x) => x.textContent?.trim() === nombre,
  );
  if (!it) throw new Error(`sin ítem «${nombre}»`);
  return it;
}

function nota(): HTMLElement | null {
  return (
    [...document.querySelectorAll<HTMLElement>('p')].find((p) => p.textContent === NOTA) ?? null
  );
}

for (const cifrado of [true, false]) {
  const rama = cifrado ? 'con cifrado' : 'sin cifrado';

  test(`${rama}: encuesta y tarea del menú «+»`, async () => {
    const raiz = await pintar(compositor(cifrado));
    await pulsar(boton(raiz, t('composer.addToMessage')));
    for (const nombre of [t('card.newPoll'), t('card.newTask')]) {
      const it = itemDeMenu(nombre);
      expect(it.disabled).toBe(cifrado);
      if (cifrado)
        expect(document.getElementById(it.getAttribute('aria-describedby')!)).toBe(nota());
    }
    expect(nota() !== null).toBe(cifrado);
    // Lo demás del menú no cambia.
    expect(itemDeMenu(t('composer.uploadFile')).disabled).toBe(false);
  });

  test(`${rama}: pedir acuse, y la prioridad sigue elegible`, async () => {
    const raiz = await pintar(compositor(cifrado));
    await pulsar(boton(raiz, t('composer.priorityAndAck')));
    const casilla = document.querySelector<HTMLInputElement>('input[type="checkbox"]')!;
    expect(casilla.disabled).toBe(cifrado);
    expect(nota() !== null).toBe(cifrado);
    for (const radio of document.querySelectorAll<HTMLInputElement>('input[type="radio"]')) {
      expect(radio.disabled).toBe(false);
    }
  });

  test(`${rama}: «Enviar más tarde»`, async () => {
    const raiz = await pintar(compositor(cifrado));
    await pulsar(boton(raiz, t('composer.sendLater')));
    const dialogo = document.querySelector<HTMLElement>('[role="dialog"]')!;
    const atajos = [...dialogo.querySelectorAll<HTMLButtonElement>('button')];
    expect(atajos.length).toBeGreaterThanOrEqual(3);
    for (const b of atajos) expect(b.disabled).toBe(cifrado);
    expect(nota() !== null).toBe(cifrado);
  });
}

const ENCUESTA: MessageCard = {
  kind: 'poll',
  options: ['Sí', 'No'],
  votes: [{ value: '0', count: 1, mine: false }],
};

for (const cifrado of [true, false]) {
  test(`${cifrado ? 'con' : 'sin'} cifrado: la encuesta ${cifrado ? 'no' : 'sí'} se vota`, async () => {
    const votos: string[] = [];
    const raiz = await pintar(
      <CardView
        card={ENCUESTA}
        messageId="m1"
        onVote={(_, v) => votos.push(v)}
        cifrado={cifrado}
        locale="es"
        t={t}
      />,
    );
    const opciones = [...raiz.querySelectorAll<HTMLButtonElement>('button[aria-pressed]')];
    expect(opciones).toHaveLength(2);
    for (const o of opciones) expect(o.disabled).toBe(cifrado);
    await pulsar(opciones[1]!);
    expect(votos).toEqual(cifrado ? [] : ['1']);
    expect(raiz.textContent?.includes(t('matrix.unsupported'))).toBe(cifrado);
  });

  test(`${cifrado ? 'con' : 'sin'} cifrado: la tarea ${cifrado ? 'no' : 'sí'} se marca`, async () => {
    const raiz = await pintar(
      <CardView
        card={{ kind: 'task' }}
        messageId="m1"
        onVote={() => {}}
        cifrado={cifrado}
        locale="es"
        t={t}
      />,
    );
    expect(raiz.querySelector<HTMLButtonElement>('[role="checkbox"]')!.disabled).toBe(cifrado);
    expect(raiz.textContent?.includes(t('matrix.unsupported'))).toBe(cifrado);
  });
}

// happy-dom no tiene layout: sin un viewport con alto, la lista virtual no pinta
// ninguna fila (ver editarVuelveElFoco.test.tsx). Se finge y se restaura.
function conGeometria(): () => void {
  const previas = new Map<string, PropertyDescriptor | undefined>();
  for (const [campo, px] of [
    ['clientHeight', 600],
    ['clientWidth', 800],
    ['offsetHeight', 600],
    ['offsetWidth', 800],
  ] as const) {
    previas.set(campo, Object.getOwnPropertyDescriptor(HTMLElement.prototype, campo));
    Object.defineProperty(HTMLElement.prototype, campo, { configurable: true, get: () => px });
  }
  return () => {
    for (const [campo, d] of previas) {
      if (d) Object.defineProperty(HTMLElement.prototype, campo, d);
      else Reflect.deleteProperty(HTMLElement.prototype, campo);
    }
  };
}

for (const cifrado of [true, false]) {
  test(`${cifrado ? 'con' : 'sin'} cifrado: el acuse ${cifrado ? 'no' : 'sí'} se confirma`, async () => {
    const restaurar = conGeometria();
    try {
      const m: ChatMessage = {
        id: 'm1',
        channelId: 'c1',
        author: { id: 'otra', name: 'Ana' },
        body: 'Leedlo, por favor',
        createdAt: '2026-11-05T10:00:00Z',
        requiresAck: true,
        ackCount: 2,
      };
      const raiz = await pintar(
        <MessageView
          messages={[m]}
          loading={false}
          onAck={() => {}}
          cifrado={cifrado}
          locale="es"
          t={t}
        />,
      );
      const confirmar = [...raiz.querySelectorAll('button')].find(
        (b) => b.textContent?.trim() === t('ack.confirm'),
      );
      expect(confirmar === undefined).toBe(cifrado);
      expect(raiz.textContent?.includes(t('matrix.unsupported'))).toBe(cifrado);
      // El recuento de quien ya confirmó se sigue viendo.
      expect(raiz.textContent?.includes(t('ack.byCount', { count: 2 }))).toBe(true);
    } finally {
      restaurar();
    }
  });
}
