/**
 * Tests de Popover — garantías de accesibilidad del panel contextual:
 *  1) cerrado por defecto; el disparador anuncia haspopup=dialog y aria-expanded;
 *  2) al abrir hay role=dialog con nombre accesible y el foco entra al panel;
 *  3) Escape cierra y DEVUELVE el foco al disparador (cierre por teclado);
 *  4) un clic fuera cierra el panel;
 *  5) el render-prop recibe `close` para cerrarse desde dentro.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { Popover } from './Popover.js';

// Idempotente: otro fichero de test puede haber registrado ya happy-dom.
if (typeof document === 'undefined') GlobalRegistrator.register();

// Preact vacía las actualizaciones de estado en un microtask; un macro-tick
// garantiza que el re-render y los efectos hayan corrido antes de aseverar.
const flush = (): Promise<void> => new Promise((r) => setTimeout(r, 0));

const mount = (ui: preact.ComponentChild) => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(ui, host);
  return {
    host,
    cleanup: () => {
      render(null, host);
      host.remove();
    },
  };
};

test('cerrado por defecto; el disparador anuncia haspopup y aria-expanded', () => {
  const { host, cleanup } = mount(
    <Popover label="Compartir" trigger={(t) => <button {...t}>Abrir</button>}>
      <button id="dentro">Acción</button>
    </Popover>,
  );
  const trigger = host.querySelector('button') as HTMLButtonElement;
  expect(trigger.getAttribute('aria-haspopup')).toBe('dialog');
  expect(trigger.getAttribute('aria-expanded')).toBe('false');
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  cleanup();
});

test('abierto (controlado): role=dialog con nombre accesible y foco dentro del panel', () => {
  const { host, cleanup } = mount(
    <Popover open label="Compartir" trigger={(t) => <button {...t}>Abrir</button>}>
      <button id="dentro">Acción</button>
    </Popover>,
  );
  const dialog = document.querySelector('[role="dialog"]');
  expect(dialog?.getAttribute('aria-label')).toBe('Compartir');
  // EL PANEL VA AL PORTAL (body), como el menú de Dropdown: fuera del árbol que
  // lo recorta —una barra con overflow-x en móvil— y libre de contextos de
  // apilamiento ajenos.
  expect(dialog?.parentElement).toBe(document.body);
  expect(
    (host.querySelector('button[aria-expanded]') as HTMLElement).getAttribute('aria-expanded'),
  ).toBe('true');
  // El foco inicial entra al primer focusable del panel.
  expect((document.activeElement as HTMLElement).id).toBe('dentro');
  cleanup();
});

test('Escape cierra y devuelve el foco al disparador', async () => {
  const { host, cleanup } = mount(
    <Popover label="Compartir" trigger={(t) => <button {...t}>Abrir</button>}>
      <button id="dentro">Acción</button>
    </Popover>,
  );
  const trigger = host.querySelector('button') as HTMLButtonElement;
  trigger.click();
  await flush();
  expect(document.querySelector('[role="dialog"]')).not.toBeNull();

  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await flush();
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  expect(document.activeElement).toBe(trigger); // foco devuelto (cierre por teclado)
  cleanup();
});

test('un clic fuera cierra el panel', async () => {
  const outside = document.createElement('button');
  document.body.appendChild(outside);
  const { host, cleanup } = mount(
    <Popover label="Compartir" trigger={(t) => <button {...t}>Abrir</button>}>
      <button id="dentro">Acción</button>
    </Popover>,
  );
  (host.querySelector('button') as HTMLButtonElement).click();
  await flush();
  expect(document.querySelector('[role="dialog"]')).not.toBeNull();

  outside.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
  await flush();
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  outside.remove();
  cleanup();
});

test('el render-prop recibe `close` para cerrarse desde dentro', async () => {
  const { host, cleanup } = mount(
    <Popover label="Compartir" trigger={(t) => <button {...t}>Abrir</button>}>
      {(close) => (
        <button id="cerrar" onClick={close}>
          Cerrar
        </button>
      )}
    </Popover>,
  );
  (host.querySelector('button') as HTMLButtonElement).click();
  await flush();
  (document.getElementById('cerrar') as HTMLButtonElement).click();
  await flush();
  expect(document.querySelector('[role="dialog"]')).toBeNull();
  cleanup();
});
