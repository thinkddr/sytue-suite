/**
 * El buscador del TELÉFONO ocupa la fila mientras se escribe en él.
 *
 * Desplegada, la cara de la suite reparte 320 px entre el ✕, la marca, el candado, la
 * campana y la cuenta, y al buscador le quedaban 50 (93 a 390): medido en el banco de
 * la barra. Con el foco dentro, el `<header>` lleva `data-buscando` y la marca y las
 * acciones, la variante que las esconde por debajo de 768. happy-dom no aplica la hoja,
 * así que aquí se guarda el contrato (el atributo y quién lleva la clase); el ancho, en
 * el banco (`test/e2e/barra-unica.mjs`).
 *
 * VISTO EN ROJO con `marcarBusqueda` vaciada (no se pone el atributo) y con el Tab sin
 * quitarlo antes de mover el foco (la campana y la cuenta se saltaban).
 *
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { TopBar } from './TopBar.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const APARTABLE = 'max-md:in-data-[buscando]:hidden';

function montar(): { header: HTMLElement; host: HTMLElement } {
  const host = document.createElement('div');
  document.body.appendChild(host);
  act(() => {
    render(
      <TopBar
        rail
        brand={<button type="button">marca</button>}
        center={<input type="search" aria-label="Búsqueda global" />}
        actions={<button type="button">campana</button>}
        movil={{ titulo: 'Mail', entrada: <span>AG</span>, alDestinos: () => undefined }}
      />,
      host,
    );
  });
  return { header: host.querySelector('header')!, host };
}

test('desplegada, el foco en el buscador aparta la marca y las acciones; al salir vuelven', () => {
  const { header, host } = montar();
  const entrada = header.querySelector<HTMLButtonElement>('button[aria-expanded="false"]')!;
  act(() => entrada.click());
  const marca = [...header.querySelectorAll('button')].find((b) => b.textContent === 'marca')!;
  const campana = [...header.querySelectorAll('button')].find((b) => b.textContent === 'campana')!;
  // Los dos contenedores llevan la variante que los esconde con el foco en el buscador.
  expect(marca.parentElement!.parentElement!.className).toContain(APARTABLE);
  expect(campana.parentElement!.className).toContain(APARTABLE);

  const campo = header.querySelector('input')!;
  act(() => campo.focus());
  expect(header.hasAttribute('data-buscando')).toBe(true);

  // Tab: el atributo se quita ANTES de que el navegador busque la siguiente parada.
  campo.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', bubbles: true }));
  expect(header.hasAttribute('data-buscando')).toBe(false);

  // (happy-dom no mueve el foco con el Tab: se le quita a mano para volver a entrar.)
  act(() => campo.blur());
  act(() => campo.focus());
  expect(header.hasAttribute('data-buscando')).toBe(true);
  act(() => campana.focus());
  expect(header.hasAttribute('data-buscando')).toBe(false);
  render(null, host);
  host.remove();
});

test('plegada o en ancho nadie se aparta: la variante solo va en la cara de la suite', () => {
  const { header, host } = montar();
  expect(header.innerHTML).not.toContain(APARTABLE);
  render(null, host);
  host.remove();
});

test('«Atrás» del teléfono pliega la suite desplegada antes de abandonar la página', () => {
  // Con el dedo y por debajo de 768: `useMobileBack` arma una entrada temporal en el
  // historial y el `popstate` del «atrás» la recoge. Antes la cara de la suite no se
  // apuntaba y «atrás» se iba de la página con ella abierta. VISTO EN ROJO sin el
  // `useMobileBack` de TopBar.tsx.
  const antes = window.matchMedia;
  window.matchMedia = ((media: string) => ({
    matches: true,
    media,
    addEventListener: () => undefined,
    removeEventListener: () => undefined,
  })) as unknown as typeof window.matchMedia;
  try {
    const { header, host } = montar();
    const entrada = header.querySelector<HTMLButtonElement>('button[aria-expanded="false"]')!;
    act(() => entrada.click());
    expect(header.innerHTML).toContain(APARTABLE);
    const estado = window.history.state as Record<string, unknown> | null;
    expect(estado?.['__sytueMobileBack']).toBeTruthy();
    act(() => {
      window.dispatchEvent(new Event('popstate'));
    });
    expect(header.innerHTML).not.toContain(APARTABLE);
    render(null, host);
    host.remove();
  } finally {
    window.matchMedia = antes;
  }
});
