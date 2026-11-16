/*
 * LA TARJETA DE PERSONA (D-ROOM-04): al pulsar a alguien sale su nombre, su estado,
 * su correo del directorio y su hora local, con «Mensaje» y «Llamar». En la propia
 * no hay botones: uno no se escribe ni se llama a sí mismo.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { olvidarDirectorio } from '../lib/tarjetaPersona.js';
import { PersonCard, type TarjetaContexto } from './PersonCard.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = ((k: string, v?: Record<string, string>) =>
  v ? `${k}(${Object.values(v).join(',')})` : k) as never;

let host: HTMLElement | null = null;
const fetchOriginal = globalThis.fetch;
// Lo que contesta el Hub por /api/presence; cada test lo ajusta.
let presenciaDelHub: unknown[] = [];
beforeEach(() => {
  presenciaDelHub = [];
  globalThis.fetch = ((url: string) =>
    Promise.resolve(
      String(url).startsWith('/api/presence')
        ? Response.json({ presence: presenciaDelHub })
        : Response.json({
            people: [{ sub: 'ana', email: 'ana@acme.sytue.eu', title: 'Diseño' }],
          }),
    )) as unknown as typeof fetch;
});
afterEach(() => {
  if (host) {
    render(null, host);
    host.remove();
  }
  host = null;
  globalThis.fetch = fetchOriginal;
  olvidarDirectorio();
});

function ctx(llamadas: string[], selfSub = 'yo'): TarjetaContexto {
  return {
    presences: { ana: { subject: 'ana', state: 'busy', status: 'En una demo', tz: 'Asia/Tokyo' } },
    selfSub,
    locale: 'es',
    t,
    onMessage: (s) => llamadas.push(`msg:${s}`),
    onCall: (s) => llamadas.push(`call:${s}`),
  };
}

async function abrir(c: TarjetaContexto): Promise<HTMLElement> {
  host = document.createElement('div');
  document.body.append(host);
  await act(async () => {
    render(
      <PersonCard sub="ana" name="Ana Pérez" ctx={c}>
        Ana Pérez
      </PersonCard>,
      host!,
    );
  });
  const disparador = host.querySelector('button')!;
  expect(disparador.getAttribute('aria-label')).toBe('person.openProfile(Ana Pérez)');
  await act(async () => {
    disparador.click();
  });
  // El directorio llega por fetch: se deja que la promesa y su render terminen.
  for (let i = 0; i < 5; i++) await act(() => new Promise((r) => setTimeout(r, 0)));
  return document.querySelector('[role="dialog"]') as HTMLElement;
}

test('enseña nombre, estado, puesto, correo y hora local, y los botones actúan', async () => {
  const llamadas: string[] = [];
  const dialogo = await abrir(ctx(llamadas));
  expect(dialogo).not.toBeNull();
  const texto = dialogo.textContent ?? '';
  expect(texto).toContain('Ana Pérez');
  expect(texto).toContain('presence.dnd · En una demo');
  expect(texto).toContain('Diseño');
  expect(dialogo.querySelector('a[href="mailto:ana@acme.sytue.eu"]')).not.toBeNull();
  expect(texto).toMatch(/person\.localTime\(\d\d:\d\d\)/);

  const botones = [...dialogo.querySelectorAll('button')];
  const llamar = botones.find((b) => b.textContent?.includes('person.call'))!;
  await act(async () => llamar.click());
  expect(llamadas).toEqual(['call:ana']);
  // Al actuar, la tarjeta se cierra.
  expect(document.querySelector('[role="dialog"]')).toBeNull();
});

test('en la tarjeta propia no hay «Mensaje» ni «Llamar»', async () => {
  const dialogo = await abrir(ctx([], 'ana'));
  expect(dialogo.textContent).not.toContain('person.message');
  expect(dialogo.textContent).not.toContain('person.call');
});

test('el autor de un canal sin presencia sondeada la pide al abrir su tarjeta', async () => {
  // En un canal, ChatApp no sondea a los autores: sin la consulta al abrir saldría
  // «Sin conexión» y sin hora aunque esté conectada.
  presenciaDelHub = [{ subject: 'ana', state: 'online', tz: 'Europe/Madrid' }];
  const c = { ...ctx([]), presences: {} };
  const dialogo = await abrir(c);
  const texto = dialogo.textContent ?? '';
  expect(texto).toContain('presence.online');
  expect(texto).toMatch(/person\.localTime\(\d\d:\d\d\)/);
});
