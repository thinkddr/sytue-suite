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
