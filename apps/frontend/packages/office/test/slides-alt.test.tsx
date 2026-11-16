/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Texto alternativo de las imágenes de diapositiva (DIAP-06): la lógica pura
 * (textoAlt.ts), el `alt` que pinta el render compartido de escenario, miniaturas y
 * presentación (renderBox), el nombre del marco (boxLabel) y el campo de la mini-barra
 * (CampoAlt). Antes toda imagen salía con `alt=""` y el modelo no tenía dónde guardarlo.
 * Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
if (typeof document === 'undefined') GlobalRegistrator.register();

import { expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { docsT } from '../src/i18n';
import { CampoAlt } from '../src/editor/slides/CampoAlt';
import { boxLabel, renderBox } from '../src/editor/slides/SlidesCanvas';
import { MAX_ALT_CHARS, altDe, conAlt } from '../src/editor/slides/textoAlt';
import type { SlideBox } from '../src/editor/types';

const t = docsT('es');
const imagen = (extra: Partial<SlideBox> = {}): SlideBox => ({
  id: 'i',
  x: 10,
  y: 10,
  w: 40,
  h: 30,
  kind: 'image',
  html: '',
  src: 'data:image/png;base64,AA',
  ...extra,
});

test('altDe: una línea, recortada, con techo; lo que no es texto no cuenta', () => {
  expect(altDe(imagen({ alt: '  Logo\n de  la\tempresa ' }))).toBe('Logo de la empresa');
  expect(altDe(imagen())).toBe('');
  expect(altDe({ alt: 42 as unknown as string })).toBe('');
  expect(altDe({ alt: 'x'.repeat(MAX_ALT_CHARS + 50) })).toHaveLength(MAX_ALT_CHARS);
});

test('conAlt: guarda el texto limpio y, vacío, quita el campo (JSON limpio)', () => {
  expect(conAlt(imagen(), ' Gato dormido ').alt).toBe('Gato dormido');
  const sin = conAlt(imagen({ alt: 'Gato' }), '   ');
  expect('alt' in sin).toBe(false);
  expect(sin.src).toBe('data:image/png;base64,AA'); // el resto de la caja, intacto
});

test('renderBox pinta el alt de la imagen (escenario, miniaturas y presentación)', () => {
  const host = document.createElement('div');
  render(renderBox(t, imagen({ alt: 'Mapa de oficinas' }), 540, undefined, true), host);
  expect(host.querySelector('img')?.getAttribute('alt')).toBe('Mapa de oficinas');
  render(renderBox(t, imagen(), 540), host);
  expect(host.querySelector('img')?.getAttribute('alt')).toBe(''); // sin describir = decorativa
});

test('boxLabel nombra la imagen por su alt (recortado) y, sin él, por su tipo', () => {
  expect(boxLabel(t, imagen({ alt: 'Mapa' }))).toBe('Mapa');
  expect(boxLabel(t, imagen({ alt: 'a'.repeat(40) }))).toBe(`${'a'.repeat(29)}…`);
  expect(boxLabel(t, imagen())).toBe(t('slides.box.image'));
});

test('CampoAlt: aplica al escribir sin guardar y guarda una vez al confirmar con Intro', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const llamadas: [string, string, boolean][] = [];
  act(() => {
    render(
      <CampoAlt box={imagen({ alt: 'Antes' })} onAlt={(id, a, p) => llamadas.push([id, a, p])} />,
      host,
    );
  });
  const campo = host.querySelector('input')!;
  expect(campo.value).toBe('Antes');
  expect(campo.getAttribute('aria-label')).toBe(docsT('en')('image.alt.label')); // idioma por defecto del contexto
  act(() => {
    campo.value = 'Gráfico de barras';
    campo.dispatchEvent(new Event('input', { bubbles: true }));
  });
  act(() => {
    campo.focus();
    campo.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }));
  });
  // Un segundo blur sin escribir nada no vuelve a guardar.
  act(() => {
    campo.focus();
    campo.blur();
  });
  expect(llamadas).toEqual([
    ['i', 'Gráfico de barras', false],
    ['i', 'Gráfico de barras', true],
  ]);
  render(null, host);
  host.remove();
});

test('CampoAlt refleja un cambio de fuera (deshacer, otra imagen) si no se está escribiendo', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  act(() => render(<CampoAlt box={imagen({ alt: 'Uno' })} onAlt={() => {}} />, host));
  act(() => render(<CampoAlt box={imagen({ id: 'otra', alt: 'Dos' })} onAlt={() => {}} />, host));
  expect(host.querySelector('input')!.value).toBe('Dos');
  render(null, host);
  host.remove();
});

test('CampoAlt: pinchar otra imagen con el campo a medias guarda en la primera, no en la nueva', () => {
  // La selección cambia en el `pointerdown` y el escenario se repinta antes del `blur`.
  const host = document.createElement('div');
  document.body.appendChild(host);
  const llamadas: [string, string, boolean][] = [];
  const onAlt = (id: string, a: string, p: boolean): void => {
    llamadas.push([id, a, p]);
  };
  act(() => render(<CampoAlt box={imagen({ alt: 'Uno' })} onAlt={onAlt} />, host));
  const campo = host.querySelector('input')!;
  act(() => {
    campo.focus();
    campo.value = 'Mapa';
    campo.dispatchEvent(new Event('input', { bubbles: true }));
  });
  act(() => render(<CampoAlt box={imagen({ id: 'otra', alt: 'Dos' })} onAlt={onAlt} />, host));
  expect(campo.value).toBe('Dos');
  act(() => campo.blur()); // el blur tardío ya no guarda nada
  expect(llamadas).toEqual([
    ['i', 'Mapa', false],
    ['i', 'Mapa', true],
  ]);
  render(null, host);
  host.remove();
});

test('CampoAlt: quitar la mini-barra con el campo a medias guarda lo escrito', () => {
  const host = document.createElement('div');
  document.body.appendChild(host);
  const llamadas: [string, string, boolean][] = [];
  act(() =>
    render(<CampoAlt box={imagen()} onAlt={(id, a, p) => llamadas.push([id, a, p])} />, host),
  );
  const campo = host.querySelector('input')!;
  act(() => {
    campo.value = 'Logo';
    campo.dispatchEvent(new Event('input', { bubbles: true }));
  });
  act(() => render(null, host));
  expect(llamadas).toEqual([
    ['i', 'Logo', false],
    ['i', 'Logo', true],
  ]);
  host.remove();
});
