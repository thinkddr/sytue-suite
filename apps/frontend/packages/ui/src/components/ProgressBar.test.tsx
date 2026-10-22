/**
 * ProgressBar — su contrato: dice cuánto va hecho SIN alarmar, y cuando no se
 * sabe cuánto, no se lo inventa. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { expect, test } from 'bun:test';
import { render } from 'preact';
import type { VNode } from 'preact';
import { I18nProvider } from '@sytue/i18n/preact';
import { ProgressBar } from './ProgressBar.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

function barra(children: VNode): HTMLElement {
  const host = document.createElement('div');
  document.body.appendChild(host);
  render(<I18nProvider locale="es">{children}</I18nProvider>, host);
  return host.querySelector<HTMLElement>('[role="progressbar"]')!;
}

test('con valor: porcentaje entero en aria-valuenow y el texto del idioma', () => {
  const b = barra(<ProgressBar value={0.426} label="Subiendo clip.mp4" />);
  expect(b.getAttribute('aria-label')).toBe('Subiendo clip.mp4');
  expect(b.getAttribute('aria-valuenow')).toBe('43');
  expect(b.getAttribute('aria-valuetext')).toMatch(/43\s%/u);
  expect((b.firstElementChild as HTMLElement).style.width).toBe('43%');
});

test('casi hecha NO es una alarma: el relleno no cambia de color al 95 %', () => {
  const casi = barra(<ProgressBar value={0.97} label="x" />).firstElementChild!;
  expect(casi.className).toContain('bg-primary');
  expect(casi.className).not.toContain('bg-danger');
});

test('se acota a [0, 1]', () => {
  expect(barra(<ProgressBar value={1.7} label="x" />).getAttribute('aria-valuenow')).toBe('100');
  expect(barra(<ProgressBar value={-2} label="x" />).getAttribute('aria-valuenow')).toBe('0');
});

test('sin valor es indeterminada: no anuncia un número que no conoce', () => {
  const b = barra(<ProgressBar label="Procesando" />);
  expect(b.hasAttribute('aria-valuenow')).toBe(false);
  expect(b.firstElementChild!.className).toContain('sytue-progreso-indeterminado');
});

test('el tono de error se pide, no se deduce', () => {
  const fallo = barra(<ProgressBar value={0.3} label="x" tone="danger" />).firstElementChild!;
  expect(fallo.className).toContain('bg-danger');
});
