/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * LAS MUESTRAS DE COLOR DE UN CALENDARIO son la paleta de colores de usuario de la suite
 * (`PALETA_DE_USUARIO`), con el nombre de cada tono del catálogo común. Un calendario
 * guardado con un tono de la paleta de antes (los catorce tonos joya) se ve marcado en su
 * tono más cercano, y nada se reescribe mientras no se elija otro.
 *
 * Vistos en ROJO con las muestras de antes: catorce tonos con nombres propios de Calendar,
 * y el granate guardado (#a31f38) dejaba la paleta sin ningún tono marcado.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, expect, test } from 'bun:test';
import { render } from 'preact';
import { act } from 'preact/test-utils';
import { I18nProvider } from '@sytue/i18n/preact';
import { PALETA_DE_USUARIO } from '@sytue/config';
import { ColorSwatches } from './CalendarModal.js';

const registrado = typeof document === 'undefined';
if (registrado) GlobalRegistrator.register();
afterEach(() => {
  render(null, document.body);
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = ((k: string) => k) as never;

function montar(value: string, onChange: (v: string) => void, allowNone = false): HTMLElement[] {
  act(() => {
    render(
      <I18nProvider locale="es">
        <ColorSwatches t={t} value={value} onChange={onChange} allowNone={allowNone} />
      </I18nProvider>,
      document.body,
    );
  });
  return [...document.querySelectorAll<HTMLElement>('[role="radio"]')];
}

test('ofrece los doce tonos de la paleta de usuario, con su nombre común', () => {
  const radios = montar(PALETA_DE_USUARIO[0]!.valor, () => {});
  expect(radios).toHaveLength(12);
  expect(radios.map((r) => r.getAttribute('aria-label'))).toEqual([
    'Rojo',
    'Naranja',
    'Ocre',
    'Oliva',
    'Verde',
    'Verde azulado',
    'Petróleo',
    'Azul',
    'Índigo',
    'Morado',
    'Rosa',
    'Gris',
  ]);
  for (const [i, r] of radios.entries()) {
    expect(r.style.backgroundColor).not.toBe('');
    expect(r.getAttribute('aria-label')).not.toMatch(/^#/);
    expect(r.getAttribute('aria-checked')).toBe(String(i === 0));
  }
});

test('un color guardado de la paleta de antes se ve marcado en su tono, sin reescribirse', () => {
  let cambios = 0;
  const radios = montar('#a31f38', () => cambios++);
  const marcados = radios.filter((r) => r.getAttribute('aria-checked') === 'true');
  expect(marcados.map((r) => r.getAttribute('aria-label'))).toEqual(['Rojo']);
  // El tono marcado es también el que recibe el Tab.
  expect(marcados[0]!.tabIndex).toBe(0);
  expect(cambios).toBe(0);
});

test('con «color del calendario», el vacío marca esa muestra y no un tono', () => {
  const radios = montar('', () => {}, true);
  expect(radios).toHaveLength(13);
  expect(radios[0]!.getAttribute('aria-checked')).toBe('true');
  expect(radios.slice(1).every((r) => r.getAttribute('aria-checked') === 'false')).toBe(true);
});
