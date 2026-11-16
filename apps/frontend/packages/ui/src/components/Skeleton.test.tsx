/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Test de SkeletonRows: las filas de carga miden lo que medirá la fila de verdad y la carga
 * se anuncia UNA vez (docs/design.md §3.18). Espejo de `TestLasFilasEsqueleto*` en
 * `apps/fyne/ui`. Ejecutar: `bun test`.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { I18nProvider } from '@sytue/i18n/preact';
import { expect, test } from 'bun:test';
import type { ComponentChildren } from 'preact';
import { render } from 'preact';
import { SkeletonRows, altoFilaEsqueleto } from './Skeleton.js';

if (typeof document === 'undefined') GlobalRegistrator.register();

function monta(children: ComponentChildren) {
  const host = document.body.appendChild(document.createElement('div'));
  render(<I18nProvider locale="es">{children}</I18nProvider>, host);
  return host;
}

// VISTO EN ROJO: con un alto fijo de 2.25rem para todas las filas, la tesela de Drive
// (36 px) desborda la fila y la lista salta al llegar los datos.
test('cada fila mide py-2 más lo más alto de sus hijos, como ListItem', () => {
  expect(altoFilaEsqueleto('none', 1)).toBe('calc(1rem + 1.375rem)');
  expect(altoFilaEsqueleto('none', 2)).toBe('calc(1rem + 2.5rem)');
  expect(altoFilaEsqueleto('avatar', 1)).toBe('calc(1rem + max(32px, 1.375rem))');
  expect(altoFilaEsqueleto('tile', 2)).toBe('calc(1rem + max(36px, 2.5rem))');

  const host = monta(<SkeletonRows rows={4} lead="tile" lines={2} />);
  const filas = host.querySelectorAll<HTMLLIElement>('[data-skeleton-row]');
  expect(filas.length).toBe(4);
  for (const f of filas) {
    expect(f.style.height).toBe(altoFilaEsqueleto('tile', 2));
    // La tesela y las dos líneas.
    expect(f.querySelectorAll('.sytue-skeleton').length).toBe(3);
  }
  render(null, host);
});

// VISTO EN ROJO: con un `role="status"` por fila, el lector dice «Cargando» seis veces.
test('la carga se anuncia una sola vez y los bloques no llegan al lector', () => {
  const host = monta(<SkeletonRows rows={6} lead="avatar" meta />);
  const region = host.firstElementChild as HTMLElement;
  expect(region.getAttribute('aria-busy')).toBe('true');
  const estados = host.querySelectorAll('[role="status"]');
  expect(estados.length).toBe(1);
  expect(estados[0]!.textContent).toBe('Cargando');
  expect(host.querySelector('ul')!.getAttribute('aria-hidden')).toBe('true');
  // Avatar redondo, título y la raya de meta.
  const fila = host.querySelector('[data-skeleton-row]')!;
  expect(fila.querySelectorAll('.sytue-skeleton').length).toBe(3);
  expect(fila.querySelector('.rounded-full')).not.toBeNull();
  render(null, host);
});

test('el rótulo propio sustituye al de serie y las columnas llevan sus clases', () => {
  const host = monta(
    <SkeletonRows
      rows={2}
      label="Cargando archivos"
      meta={[
        { width: '44px', class: 'hidden sm:block' },
        { width: '76px', class: 'hidden w-24 md:block' },
      ]}
    />,
  );
  expect(host.querySelector('[role="status"]')!.textContent).toBe('Cargando archivos');
  const fila = host.querySelector('[data-skeleton-row]')!;
  expect(fila.querySelectorAll('.hidden.sm\\:block').length).toBe(1);
  expect(fila.querySelectorAll('.md\\:block').length).toBe(1);
  render(null, host);
});

test('los anchos del título varían por fila', () => {
  const host = monta(<SkeletonRows rows={5} />);
  const anchos = new Set(
    Array.from(host.querySelectorAll<HTMLElement>('[data-skeleton-row] .sytue-skeleton')).map(
      (b) => b.style.width,
    ),
  );
  expect(anchos.size).toBeGreaterThan(3);
  render(null, host);
});
