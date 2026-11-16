/**
 * «PRIMEROS PASOS» EN LA PORTADA.
 *
 * Se vigila:
 * - que cada paso lleva a donde se arregla: la sección de Mi cuenta por su ancla,
 *   o la página del portal;
 * - que sin pasos no hay tarjeta (la cuenta ya lista no ve nada);
 * - que «Ocultar» la quita y se recuerda por persona y organización;
 * - que «instalar» se da por hecho al ir a las descargas, o abierta como app;
 * - que el guion de antes de pintar llega a lo mismo que el componente, para que
 *   el HTML del SSR no enseñe una tarjeta que al hidratar se va;
 * - que si el SSR no trajo los pasos se piden al BFF.
 *
 * Vistos en ROJO sin la tarjeta (no existía) y con el guion sin aplicar lo oculto.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { appDomains } from '@sytue/config';
import { primerosPasos } from '@sytue/shell';
import { en } from '../i18n/messages/en.js';
import type { PasoPendiente } from '../lib/primeros-pasos.js';

// El prefijo del portal, como en el build (`base: '/one'`).
process.env.BASE_URL ??= '/one/';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register({ url: 'https://one.sytue.test/' });

const { PrimerosPasos } = await import('./PrimerosPasos.js');
const { clavePrimerosPasos, guionAntesDePintar } = await import('../lib/primeros-pasos.js');

const CLAVE = clavePrimerosPasos('u-1', 'acme');
const fetchOriginal = globalThis.fetch;

// Los de una cuenta recién creada, con el mismo cálculo que hace el servidor.
const todos: PasoPendiente[] = primerosPasos({
  me: { mfa_enabled: false, has_avatar: false },
  passkeys: [],
  contrasenasApp: [],
  instalada: false,
});

beforeEach(() => localStorage.clear());

afterEach(() => {
  act(() => render(null, document.body));
  document.body.innerHTML = '';
  globalThis.fetch = fetchOriginal;
});

afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

function montar(pasos: PasoPendiente[] | undefined, clave = CLAVE): void {
  act(() => {
    render(<PrimerosPasos locale="en" pasos={pasos} clave={clave} org={null} />, document.body);
  });
}

const tarjeta = () => document.getElementById('primeros-pasos');
const enlaces = () =>
  Object.fromEntries(
    [...document.querySelectorAll<HTMLLIElement>('li[data-paso]')].map((li) => [
      li.dataset.paso,
      li.querySelector('a')!.getAttribute('href'),
    ]),
  );

test('cada paso lleva a donde se arregla', () => {
  montar(todos);
  expect(tarjeta()?.getAttribute('aria-labelledby')).toBe('primeros-pasos-titulo');
  expect(document.getElementById('primeros-pasos-titulo')?.textContent).toBe(
    en['firstSteps.title'],
  );
  const hrefs = enlaces();
  expect(hrefs.dos_pasos).toBe(`${appDomains.id}/account#seguridad`);
  expect(hrefs.recuperacion).toBe(`${appDomains.id}/account#recuperacion`);
  expect(hrefs.foto).toBe(`${appDomains.id}/account#perfil`);
  expect(hrefs.dispositivos).toBe('/one/dispositivos');
  expect(hrefs.instalar).toBe('/one/descargas');
  expect(tarjeta()!.textContent).toContain(en['firstSteps.mfa.title']);
  expect(tarjeta()!.textContent).toContain(en['ob.next.devices.title']);
});

test('una cuenta ya lista no ve la tarjeta', () => {
  montar([]);
  expect(tarjeta()).toBeNull();
});

test('«Ocultar» la quita y se recuerda por persona y organización', () => {
  montar(todos);
  const boton = [...document.querySelectorAll('button')].find(
    (b) => b.textContent === en['firstSteps.hide'],
  )!;
  expect(boton.getAttribute('aria-label')).toBe(en['firstSteps.hideAria']);
  act(() => boton.click());
  expect(tarjeta()).toBeNull();

  act(() => render(null, document.body));
  montar(todos);
  expect(tarjeta()).toBeNull();
  // Otra organización de la misma persona no hereda el «Ocultar».
  act(() => render(null, document.body));
  montar(todos, clavePrimerosPasos('u-1', 'otra'));
  expect(tarjeta()).not.toBeNull();
});

test('ir a las descargas da la instalación por hecha; si es lo único, no queda tarjeta', () => {
  const soloInstalar = todos.filter((p) => p.clave === 'instalar');
  montar(soloInstalar);
  const enlace = document.querySelector<HTMLAnchorElement>('li[data-paso="instalar"] a')!;
  enlace.addEventListener('click', (e) => e.preventDefault());
  act(() => enlace.click());
  act(() => render(null, document.body));
  montar(soloInstalar);
  expect(tarjeta()).toBeNull();
  act(() => render(null, document.body));
  montar(todos);
  expect(Object.keys(enlaces())).toEqual(['dos_pasos', 'recuperacion', 'foto', 'dispositivos']);
});

test('el guion de antes de pintar esconde lo mismo que el componente', () => {
  const ssr = (pasos: PasoPendiente[]) => {
    document.body.innerHTML = `<section id="primeros-pasos"><ul>${pasos
      .map((p) => `<li data-paso="${p.clave}"></li>`)
      .join('')}</ul></section>`;
    // eslint-disable-next-line no-new-func -- es el guion en línea tal cual sale en la página
    new Function(guionAntesDePintar(CLAVE))();
    return document.getElementById('primeros-pasos')!;
  };
  expect(ssr(todos).hidden).toBe(false);

  localStorage.setItem(CLAVE, JSON.stringify({ instalada: true }));
  const sinInstalar = ssr(todos);
  expect(sinInstalar.hidden).toBe(false);
  expect(sinInstalar.querySelector<HTMLElement>('[data-paso="instalar"]')!.hidden).toBe(true);
  expect(ssr(todos.filter((p) => p.clave === 'instalar')).hidden).toBe(true);

  localStorage.setItem(CLAVE, JSON.stringify({ oculto: true }));
  expect(ssr(todos).hidden).toBe(true);
  // Lo guardado para otra persona no toca esta tarjeta.
  localStorage.clear();
  localStorage.setItem(clavePrimerosPasos('u-2', 'acme'), JSON.stringify({ oculto: true }));
  expect(ssr(todos).hidden).toBe(false);
});

test('la clave no puede cerrar el guion en mitad del HTML', () => {
  expect(guionAntesDePintar(clavePrimerosPasos('</script><b>', 'x'))).not.toContain('</script>');
});

test('sin los pasos del SSR se piden al BFF y la tarjeta entra si queda algo', async () => {
  const pedidas: string[] = [];
  globalThis.fetch = (async (url: string) => {
    pedidas.push(String(url));
    return Response.json({ pasos: todos.slice(0, 1) });
  }) as typeof fetch;
  montar(undefined);
  expect(tarjeta()).toBeNull();
  await act(async () => {
    await new Promise((r) => setTimeout(r, 0));
  });
  expect(pedidas.some((u) => u.endsWith('/api/primeros-pasos'))).toBe(true);
  expect(Object.keys(enlaces())).toEqual(['dos_pasos']);
});
