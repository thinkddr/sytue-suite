/*
 * EN EL HILO SE REACCIONA, SE EDITA Y SE BORRA COMO EN EL CANAL.
 *
 * El panel de hilo pintaba solo cuerpo, adjuntos y tarjeta: no había chips de
 * reacción, ni barra de reacciones rápidas, ni el menú «…» (editar, borrar, copiar el
 * enlace, guardar…), y una respuesta que no se pudo enviar se quedaba en rojo sin
 * «Reintentar». Estas pruebas montan el panel con happy-dom y recorren cada acción
 * como lo haría la persona: pulsando los mismos controles que en el canal.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { ChatMessage } from '../lib/chat.js';
import { ThreadPanel, type AccionesDelHilo } from './ThreadPanel.js';

// Se registra solo aquí y se retira al acabar: el resto de la suite de Rooms prueba
// los BFF con el fetch/Request/Response de Bun, y los de happy-dom los rompían.
const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

// Traductor identidad con los parámetros a la vista: así se distingue
// «reaction.with 👍» de «reaction.with ❤️».
const t = ((k: string, p?: Record<string, unknown>) =>
  p ? `${k} ${Object.values(p).join(' ')}` : k) as never;

const raiz: ChatMessage = {
  id: 'raiz',
  channelId: 'c1',
  author: { id: 'ana', name: 'Ana' },
  body: 'pregunta',
  createdAt: '2026-11-05T10:00:00Z',
  mine: false,
  replyCount: 2,
};

function respuesta(id: string, extra: Partial<ChatMessage> = {}): ChatMessage {
  return {
    id,
    channelId: 'c1',
    parentId: 'raiz',
    author: { id: 'yo', name: 'Yo' },
    body: `texto de ${id}`,
    createdAt: '2026-11-05T10:01:00Z',
    mine: true,
    ...extra,
  };
}

let host: HTMLElement | null = null;
afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
  }
  host = null;
  document.body.innerHTML = '';
});

async function pintar(replies: ChatMessage[], acciones: AccionesDelHilo): Promise<HTMLElement> {
  host = document.createElement('div');
  document.body.appendChild(host);
  await act(async () => {
    render(
      <ThreadPanel
        root={raiz}
        replies={replies}
        loading={false}
        onClose={() => undefined}
        onSendReply={() => undefined}
        ownerId="yo"
        locale="es"
        t={t}
        {...acciones}
      />,
      host!,
    );
  });
  return host;
}

/** Fila (contenedor con la barra de acciones) del mensaje cuyo cuerpo es `texto`. */
function filaDe(texto: string): HTMLElement {
  const p = Array.from(host!.querySelectorAll('p')).find((el) => el.textContent?.includes(texto));
  const fila = p?.closest<HTMLElement>('.group');
  if (!fila) throw new Error(`sin fila para «${texto}»`);
  return fila;
}

async function pulsar(el: Element | null | undefined): Promise<void> {
  if (!el) throw new Error('control ausente');
  await act(async () => {
    (el as HTMLElement).click();
  });
}

/** Abre el menú «…» de una fila y devuelve las etiquetas de sus entradas. */
async function abrirMenu(fila: HTMLElement): Promise<HTMLElement[]> {
  await pulsar(fila.querySelector('[aria-label="message.moreActions"]'));
  return Array.from(document.querySelectorAll<HTMLElement>('[role="menuitem"]'));
}

test('los chips de reacción de una respuesta se ven y alternan la reacción', async () => {
  const reacciones: [string, string][] = [];
  await pintar(
    [respuesta('r1', { mine: false, reactions: [{ emoji: '🎉', count: 2, mine: false }] })],
    { onReact: (id, emoji) => reacciones.push([id, emoji]) },
  );
  const chip = filaDe('texto de r1').querySelector('[aria-pressed="false"]');
  expect(chip?.textContent).toContain('2');
  await pulsar(chip);
  expect(reacciones).toEqual([['r1', '🎉']]);
});

test('la barra de la respuesta ofrece las reacciones rápidas del canal', async () => {
  const reacciones: [string, string][] = [];
  await pintar([respuesta('r1')], { onReact: (id, emoji) => reacciones.push([id, emoji]) });
  await pulsar(filaDe('texto de r1').querySelector('[aria-label="reaction.with 👍"]'));
  expect(reacciones).toEqual([['r1', '👍']]);
});

test('una respuesta propia se edita desde el menú «…» e Intro guarda', async () => {
  const ediciones: [string, string][] = [];
  await pintar([respuesta('r1')], {
    onEdit: (id, body) => ediciones.push([id, body]),
    onDelete: () => undefined,
  });
  const entradas = await abrirMenu(filaDe('texto de r1'));
  await pulsar(entradas.find((e) => e.textContent === 'action.edit'));

  const caja = host!.querySelector<HTMLTextAreaElement>('textarea[aria-label="message.edit"]');
  expect(caja?.value).toBe('texto de r1');
  expect(document.activeElement).toBe(caja);
  await act(async () => {
    caja!.value = 'corregido';
    caja!.dispatchEvent(new Event('input', { bubbles: true }));
  });
  await act(async () => {
    caja!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
  });
  expect(ediciones).toEqual([['r1', 'corregido']]);
  expect(host!.querySelector('textarea[aria-label="message.edit"]')).toBeNull();
});

test('cancelar la edición con Esc devuelve el foco al «…» de la respuesta', async () => {
  await pintar([respuesta('r1')], { onEdit: () => undefined });
  const entradas = await abrirMenu(filaDe('texto de r1'));
  await pulsar(entradas.find((e) => e.textContent === 'action.edit'));
  const caja = host!.querySelector<HTMLTextAreaElement>('textarea[aria-label="message.edit"]');
  await act(async () => {
    caja!.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  });
  expect(host!.querySelector('textarea[aria-label="message.edit"]')).toBeNull();
  expect(document.activeElement).toBe(
    filaDe('texto de r1').querySelector('[aria-label="message.moreActions"]'),
  );
});

test('borrar una respuesta propia pide confirmación con el diálogo de la casa', async () => {
  const borrados: string[] = [];
  await pintar([respuesta('r1')], { onDelete: (id) => borrados.push(id) });
  const entradas = await abrirMenu(filaDe('texto de r1'));
  await pulsar(entradas.find((e) => e.textContent === 'action.delete'));
  // Nada se borra hasta confirmar.
  expect(borrados).toEqual([]);
  expect(document.body.textContent).toContain('message.confirmDelete');
  const dialogo = document.querySelector('[role="dialog"], [role="alertdialog"], dialog');
  const confirmar = Array.from(dialogo?.querySelectorAll('button') ?? []).find(
    (b) => b.textContent === 'action.delete',
  );
  await pulsar(confirmar);
  expect(borrados).toEqual(['r1']);
});

test('el menú de una respuesta ajena no ofrece editar ni borrar, sí copiar el enlace', async () => {
  await pintar([respuesta('r1', { mine: false, author: { id: 'luis', name: 'Luis' } })], {
    onEdit: () => undefined,
    onDelete: () => undefined,
    onToggleSave: () => undefined,
  });
  const etiquetas = (await abrirMenu(filaDe('texto de r1'))).map((e) => e.textContent);
  expect(etiquetas).toContain('message.copyLink');
  expect(etiquetas).toContain('action.save');
  expect(etiquetas).not.toContain('action.edit');
  expect(etiquetas).not.toContain('action.delete');
});

test('una respuesta fallida ofrece «Reintentar» y no la barra de acciones', async () => {
  const reintentos: string[] = [];
  const fallida = respuesta('r1', { failed: true });
  await pintar([fallida], {
    onReact: () => undefined,
    onResend: (m) => reintentos.push(m.id),
  });
  const fila = filaDe('texto de r1');
  expect(fila.querySelector('[aria-label="message.moreActions"]')).toBeNull();
  const reintentar = Array.from(fila.querySelectorAll('button')).find(
    (b) => b.textContent === 'action.retry',
  );
  await pulsar(reintentar);
  expect(reintentos).toEqual(['r1']);
});
