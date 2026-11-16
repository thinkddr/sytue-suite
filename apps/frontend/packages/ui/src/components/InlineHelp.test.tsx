/**
 * InlineHelp — la caja de aviso común: se anuncia según el tono salvo que la app diga
 * otra cosa, y su titular puede ser un encabezado de verdad. Las apps se pasaron a
 * ella desde sus cajas `bg-*-bg` hechas a mano (`avisos-al-comun.test.ts`), y estas
 * dos opciones son lo que esas cajas hacían y la común no sabía hacer.
 * Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { InlineHelp } from './FieldHint.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

const hosts: HTMLElement[] = [];

afterEach(() => {
  for (const host of hosts.splice(0)) {
    render(null, host);
    host.remove();
  }
});

function pinta(vnode: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.append(host);
  hosts.push(host);
  render(vnode, host);
  return host.firstElementChild as HTMLElement;
}

test('el tono decide el anuncio: danger alerta, warning espera turno, el resto calla', () => {
  expect(pinta(<InlineHelp tone="danger">x</InlineHelp>).getAttribute('role')).toBe('alert');
  expect(pinta(<InlineHelp tone="warning">x</InlineHelp>).getAttribute('role')).toBe('status');
  for (const tone of ['neutral', 'info', 'success'] as const) {
    expect(pinta(<InlineHelp tone={tone}>x</InlineHelp>).hasAttribute('role')).toBe(false);
  }
});

test('live manda sobre el tono: off para la caja con controles, alert para el bloqueo', () => {
  const conCasilla = pinta(
    <InlineHelp tone="danger" live="off">
      <input type="checkbox" />
    </InlineHelp>,
  );
  expect(conCasilla.hasAttribute('role')).toBe(false);
  expect(
    pinta(
      <InlineHelp tone="warning" live="alert">
        x
      </InlineHelp>,
    ).getAttribute('role'),
  ).toBe('alert');
});

test('titleAs pone el titular como encabezado', () => {
  const caja = pinta(
    <InlineHelp tone="warning" title="Cópialo ahora" titleAs="h3">
      Solo se ve una vez.
    </InlineHelp>,
  );
  const h3 = caja.querySelector('h3');
  expect(h3?.textContent).toBe('Cópialo ahora');
  expect(h3?.className).toContain('text-warning-text');
  // Sin `titleAs`, sigue siendo el párrafo de siempre.
  expect(pinta(<InlineHelp title="Hola" />).querySelector('p')?.textContent).toBe('Hola');
});
