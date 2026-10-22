/**
 * Tests de ReadOnlyBar — el contrato de la franja de solo lectura: se anuncia como
 * estado, lleva el botón que la desbloquea y, enviada la petición, deja de ofrecerlo.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import { ReadOnlyBar } from './ReadOnlyBar.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

function montar(ui: preact.JSX.Element): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(ui, host);
  return host;
}

test('la franja ofrece la acción que la desbloquea, y la acción funciona', () => {
  let pulsado = 0;
  const host = montar(
    <ReadOnlyBar
      message="Solo lectura"
      action={{ label: 'Pedir acceso de edición', onClick: () => (pulsado += 1) }}
    />,
  );
  expect(host.querySelector('[role="status"]')?.textContent).toContain('Solo lectura');
  const btn = host.querySelector('button')!;
  expect(btn.textContent).toBe('Pedir acceso de edición');
  btn.click();
  expect(pulsado).toBe(1);
  render(null, host);
});

test('enviada la petición, el botón se cambia por la nota y no se puede volver a pedir', () => {
  const host = montar(
    <ReadOnlyBar
      message="Solo lectura"
      action={{ label: 'Pedir acceso de edición', onClick: () => undefined }}
      note="Petición enviada"
    />,
  );
  expect(host.querySelector('button')).toBeNull();
  expect(host.textContent).toContain('Petición enviada');
  render(null, host);
});

test('mientras se envía, el botón no admite un segundo clic', () => {
  const host = montar(
    <ReadOnlyBar
      message="x"
      action={{ label: 'Editar', onClick: () => undefined, loading: true }}
    />,
  );
  const btn = host.querySelector('button')!;
  expect(btn.disabled || btn.getAttribute('aria-busy') === 'true').toBe(true);
  render(null, host);
});
