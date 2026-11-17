/**
 * BulkActionBar — su contrato: UNA parada de tabulación, y Escape suelta la
 * selección salvo que haya algo delante que ya escuche el teclado.
 *
 * Lo segundo es lo que hacía que Escape no se pudiera usar: con un menú abierto
 * dentro de la barra —el de etiquetas de Mail—, una sola pulsación cerraba el menú
 * Y ADEMÁS deshacía la selección, que es perder el sitio dos veces.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { BulkActionBar } from './BulkActionBar.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

const hosts: HTMLElement[] = [];

afterEach(() => {
  for (const host of hosts.splice(0)) {
    render(null, host);
    host.remove();
  }
});

function mount(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  hosts.push(host);
  render(<I18nProvider locale="es">{children}</I18nProvider>, host);
  return host;
}

function escape(): void {
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
}

/**
 * El oyente de Escape lo pone un `useEffect`, y Preact los ejecuta «tras el
 * pintado»: con `requestAnimationFrame` si lo hay y, si no, con un `setTimeout`
 * de 100 ms. En happy-dom no hay pintado, así que manda el segundo camino.
 */
const tick = (): Promise<void> => new Promise((r) => setTimeout(r, 150));

function barra(vaciar: () => void, count = 3): VNode {
  return (
    <BulkActionBar count={count} countLabel={`${count} seleccionados`} onClear={vaciar}>
      <button type="button">Archivar</button>
      <button type="button">Borrar</button>
    </BulkActionBar>
  );
}

test('sin nada marcado la barra no existe', () => {
  expect(mount(barra(() => {}, 0)).innerHTML).toBe('');
});

test('toda la barra es una sola parada de tabulación', () => {
  const host = mount(barra(() => {}));
  // Dos acciones + el aspa de vaciar; solo la primera es alcanzable con Tab.
  const tabs = Array.from(host.querySelectorAll('button')).map((b) => b.tabIndex);
  expect(tabs).toEqual([0, -1, -1]);
});

/*
 * En el teléfono solo se deslizan las ACCIONES: el recuento y el aspa quedan fuera del
 * grupo que se desplaza. Visto en ROJO en el muestrario a 320 px: con la fila entera
 * deslizándose, «Eliminar» salía cortado y el aspa quedaba fuera de la pantalla.
 * El reparto real (medido en Chromium a 320/767/768/1280) está en el muestrario; aquí
 * se fija la estructura que lo hace posible.
 */
test('en el teléfono se deslizan las acciones, no el recuento ni el aspa', () => {
  const host = mount(barra(() => {}));
  const barraEl = host.querySelector('[role="toolbar"]') as HTMLElement;
  const grupo = barraEl.querySelector('[data-scroll-x]') as HTMLElement;
  expect(grupo).not.toBeNull();
  // Solo este grupo se marca como desplazamiento deliberado (detector-desbordamiento).
  expect(barraEl.hasAttribute('data-scroll-x')).toBe(false);
  expect(grupo.className).toContain('max-md:overflow-x-auto');
  // En escritorio el grupo no existe como caja: la barra es la de siempre.
  expect(grupo.className).toContain('md:contents');
  expect([...grupo.querySelectorAll('button')].map((b) => b.textContent)).toEqual([
    'Archivar',
    'Borrar',
  ]);
  const aspa = barraEl.querySelector('button[aria-label]') as HTMLElement;
  expect(grupo.contains(aspa)).toBe(false);
  expect(grupo.contains(barraEl.querySelector('[role="status"]'))).toBe(false);
  // Y sigue siendo UNA parada de tabulación con el aspa la última.
  expect([...barraEl.querySelectorAll('button')].at(-1)).toBe(aspa);
});

test('en el teléfono el recuento es el número, y la frase sigue para el lector', () => {
  const host = mount(barra(() => {}, 12));
  const vivo = host.querySelector('[aria-live="polite"]') as HTMLElement;
  // La región viva es SOLO la frase: el número que se ve va fuera de ella.
  expect(vivo.textContent).toBe('12 seleccionados');
  expect(vivo.className).toContain('max-md:sr-only');
  const numero = vivo.nextElementSibling as HTMLElement;
  expect(numero.textContent).toBe('12');
  expect(numero.getAttribute('aria-hidden')).toBe('true');
  expect(numero.className).toContain('md:hidden');
});

test('el recuento se anuncia sin robar el foco', () => {
  const vivo = mount(barra(() => {})).querySelector('[aria-live="polite"]')!;
  expect(vivo.textContent).toBe('3 seleccionados');
});

test('Escape suelta la selección', async () => {
  let vaciadas = 0;
  mount(barra(() => (vaciadas += 1)));
  await tick();

  escape();

  expect(vaciadas).toBe(1);
});

test('con un menú abierto, Escape es del menú', async () => {
  let vaciadas = 0;
  const host = mount(barra(() => (vaciadas += 1)));
  await tick();
  // Un Dropdown de @sytue/ui pinta `role="menu"` mientras está desplegado.
  const menu = document.createElement('div');
  menu.setAttribute('role', 'menu');
  host.appendChild(menu);

  escape();

  expect(vaciadas).toBe(0);
});
