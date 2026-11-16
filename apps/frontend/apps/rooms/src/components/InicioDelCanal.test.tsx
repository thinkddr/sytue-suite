/*
 * EL INICIO DEL CANAL ARRIBA DEL HISTORIAL (ROOM-13).
 *
 * «Este es el principio de #canal» solo salía con el canal vacío. Ahora sale también
 * al llegar al principio de un historial con mensajes, con quién creó el canal y
 * cuándo, su propósito y su tema, y las acciones solo para quien puede usarlas. En un
 * directo es «Conversación con Ana» con su presencia.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import type { Channel, ChatMessage } from '../lib/chat.js';
import { toChannel } from '../lib/chat.js';
import { MessageView } from './MessageView.js';
import { InicioDelCanal, lineaDeCreacion } from './InicioDelCanal.js';

// Se registra solo aquí y se retira al acabar: el resto de la suite de Rooms prueba
// los BFF con el fetch/Request/Response de Bun, y los de happy-dom los rompían.
const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

// Traductor de prueba que deja ver la clave y sus variables.
const t = ((k: string, v?: Record<string, unknown>) =>
  v ? `${k}(${Object.values(v).join('|')})` : k) as never;

const canal: Channel = {
  id: 'c1',
  name: 'general',
  kind: 'public',
  topic: 'Lanzamiento del viernes',
  purpose: 'Todo lo del equipo de producto',
  createdBy: 'ana',
  createdAt: '2026-05-03T10:00:00Z',
};

function msg(id: string): ChatMessage {
  return {
    id,
    channelId: 'c1',
    author: { id: 'beto', name: 'Beto' },
    body: `texto de ${id}`,
    createdAt: '2026-11-05T10:00:00Z',
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

async function pintar(vnode: preact.VNode): Promise<HTMLElement> {
  host ??= document.body.appendChild(document.createElement('div'));
  await act(async () => {
    render(vnode, host!);
  });
  return host;
}

function botones(el: HTMLElement): string[] {
  return [...el.querySelectorAll('button')].map((b) => b.textContent?.trim() ?? '');
}

test('el canal crudo del servicio trae quién lo creó y cuándo', () => {
  const c = toChannel({
    id: 'c1',
    name: 'general',
    kind: 'public',
    created_by: 'ana',
    created_at: '2026-05-03T10:00:00Z',
  } as never);
  expect(c.createdBy).toBe('ana');
  expect(c.createdAt).toBe('2026-05-03T10:00:00Z');
});

test('al llegar al principio de un historial con mensajes se pinta el bloque encima', async () => {
  fingirGeometria();
  const inicio = <InicioDelCanal canal={canal} yo="yo" nombreCreador="Ana" locale="es" t={t} />;
  const el = await pintar(
    <MessageView
      messages={[msg('m1'), msg('m2')]}
      loading={false}
      inicio={inicio}
      locale="es"
      t={t}
    />,
  );
  const bloque = el.querySelector('[data-inicio-canal]');
  expect(bloque).not.toBeNull();
  expect(bloque!.textContent).toContain('message.channelStart(general)');
  expect(bloque!.textContent).toContain('message.introCreatedBy(Ana|');
  expect(bloque!.textContent).toContain('Todo lo del equipo de producto');
  expect(bloque!.textContent).toContain('Lanzamiento del viernes');
  // Va ANTES del primer mensaje, dentro de su fila.
  const primero = [...el.querySelectorAll('p')].find((p) => p.textContent?.includes('texto de m1'));
  expect(primero).toBeDefined();
  expect(bloque!.compareDocumentPosition(primero!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  // Sin el sitio para escribir el primero: el canal ya tiene conversación.
  expect(bloque!.textContent).not.toContain('message.writeFirst');
});

test('mientras queda historial anterior no hay bloque', async () => {
  fingirGeometria();
  const el = await pintar(<MessageView messages={[msg('m1')]} loading={false} locale="es" t={t} />);
  expect(el.querySelector('[data-inicio-canal]')).toBeNull();
});

test('con el canal vacío el bloque sustituye al estado vacío', async () => {
  const inicio = (
    <InicioDelCanal canal={canal} yo="yo" nombreCreador="Ana" sinMensajes locale="es" t={t} />
  );
  const el = await pintar(
    <MessageView
      messages={[]}
      loading={false}
      channelName="general"
      inicio={inicio}
      locale="es"
      t={t}
    />,
  );
  const bloque = el.querySelector('[data-inicio-canal]');
  expect(bloque).not.toBeNull();
  expect(bloque!.textContent).toContain('message.writeFirst');
  // Un solo título del principio: el del bloque, no también el del EmptyState.
  expect(el.querySelectorAll('h2, h3').length).toBe(1);
});

test('las acciones solo salen para quien puede usarlas', async () => {
  const sin = await pintar(<InicioDelCanal canal={canal} yo="yo" locale="es" t={t} />);
  expect(botones(sin)).toEqual([]);

  let anadir = 0;
  let tema = 0;
  const con = await pintar(
    <InicioDelCanal
      canal={canal}
      yo="yo"
      onAnadirPersonas={() => anadir++}
      onEditarTema={() => tema++}
      locale="es"
      t={t}
    />,
  );
  expect(botones(con)).toEqual(['members.addPeople', 'channel.editTopic']);
  for (const b of con.querySelectorAll('button')) b.click();
  expect([anadir, tema]).toEqual([1, 1]);

  // Sin tema todavía, la acción es «Añadir un tema».
  const sinTema = await pintar(
    <InicioDelCanal
      canal={{ ...canal, topic: '' }}
      yo="yo"
      onEditarTema={() => undefined}
      locale="es"
      t={t}
    />,
  );
  expect(botones(sinTema)).toEqual(['channel.addTopic']);
});

test('quién creó el canal: tú, el nombre del roster o «alguien»', () => {
  expect(lineaDeCreacion(t, 'es', canal, 'ana')).toStartWith('message.introCreatedByYou(');
  expect(lineaDeCreacion(t, 'es', canal, 'yo', 'Ana')).toStartWith('message.introCreatedBy(Ana|');
  expect(lineaDeCreacion(t, 'es', canal, 'yo')).toStartWith(
    'message.introCreatedBy(message.introSomeone|',
  );
  // Sin fecha no se inventa nada.
  expect(lineaDeCreacion(t, 'es', { createdBy: 'ana' }, 'yo', 'Ana')).toBe('');
});

test('en un directo: «Conversación con Ana», su presencia y sin acciones de canal', async () => {
  const dm: Channel = { id: 'd1', name: 'Ana', kind: 'direct', peers: ['Ana'], peerSubs: ['ana'] };
  const el = await pintar(
    <InicioDelCanal
      canal={dm}
      yo="yo"
      presencia="En línea"
      onAnadirPersonas={() => undefined}
      onEditarTema={() => undefined}
      locale="es"
      t={t}
    />,
  );
  expect(el.querySelector('h2')!.textContent).toBe('message.introDm(Ana)');
  expect(el.textContent).toContain('En línea');
  expect(el.textContent).toContain('message.introDmStart(Ana)');
  expect(botones(el)).toEqual([]);
});
