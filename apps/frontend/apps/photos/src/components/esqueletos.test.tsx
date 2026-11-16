/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Los esqueletos de Fotos tienen la geometría de lo que va a llegar —las filas
 * justificadas de la galería y la rejilla 4:3 de los álbumes—, avisan una sola vez
 * de que se está cargando, y las pantallas los usan en vez de la rueda suelta.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { render, type ComponentChild } from 'preact';
import { act } from 'preact/test-utils';
import { I18nProvider } from '@sytue/i18n/preact';
import {
  EsqueletoAlbumes,
  EsqueletoDuplicadas,
  EsqueletoFilas,
  PROPORCIONES_ESQUELETO,
} from './Esqueletos.js';

let registrado = false;
beforeAll(() => {
  if (typeof document !== 'undefined') return;
  GlobalRegistrator.register();
  registrado = true;
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

let host: HTMLElement;
afterEach(() => {
  render(null, host);
  host.remove();
});
function montar(nodo: ComponentChild): HTMLElement {
  host = document.createElement('div');
  document.body.appendChild(host);
  act(() => render(nodo, host));
  return host;
}

test('las filas del esqueleto son las de la galería, con proporciones variadas', () => {
  const h = montar(<EsqueletoFilas cuantas={10} />);
  const filas = h.querySelector('ul.filas');
  expect(filas).not.toBeNull();
  const teselas = [...(filas?.querySelectorAll(':scope > li') ?? [])] as HTMLElement[];
  expect(teselas).toHaveLength(10);
  const rs = teselas.map((li) => Number(li.style.getPropertyValue('--r')));
  // Todas con su proporción (sin `--r`, `.filas` las haría cuadradas)…
  for (const r of rs) expect(r).toBeGreaterThan(0);
  // …y no todas iguales: un esqueleto cuadriculado se recompone al llegar las fotos.
  expect(new Set(rs).size).toBeGreaterThan(3);
  expect(rs.some((r) => r < 1)).toBe(true);
  expect(rs.some((r) => r > 1)).toBe(true);
  // Cada tesela es un bloque gris que la llena.
  expect(teselas.every((li) => li.querySelector('.sytue-skeleton') !== null)).toBe(true);
});

test('las proporciones caben en las que admite la galería', () => {
  for (const r of PROPORCIONES_ESQUELETO) {
    expect(r).toBeGreaterThanOrEqual(9 / 20);
    expect(r).toBeLessThanOrEqual(5 / 2);
  }
});

test('ocupado y con un único aviso «Cargando» para el lector de pantalla', () => {
  for (const crear of [
    () => <EsqueletoFilas mes cuantas={12} />,
    () => <EsqueletoAlbumes />,
    () => <EsqueletoAlbumes cuadradas />,
    () => <EsqueletoDuplicadas />,
  ]) {
    const h = montar(<I18nProvider locale="es">{crear()}</I18nProvider>);
    const raiz = h.firstElementChild;
    expect(raiz?.getAttribute('aria-busy')).toBe('true');
    const avisos = h.querySelectorAll('[role="status"]');
    expect(avisos).toHaveLength(1);
    expect(avisos[0]?.textContent).toBe('Cargando');
    // Lo gris no se anuncia: va todo dentro de algo oculto.
    for (const gris of h.querySelectorAll('.sytue-skeleton')) {
      expect(gris.closest('[aria-hidden="true"]')).not.toBeNull();
    }
    render(null, host);
    host.remove();
  }
  host = document.createElement('div');
});

test('con `mes` pinta la cabecera de un mes encima de las filas', () => {
  const sin = montar(<EsqueletoFilas cuantas={4} />);
  expect(sin.querySelectorAll('.sytue-skeleton')).toHaveLength(4);
  render(null, host);
  host.remove();
  const con = montar(<EsqueletoFilas mes cuantas={4} />);
  const cabecera = con.querySelector('[data-esqueleto] > div.py-2');
  expect(cabecera?.nextElementSibling?.matches('ul.filas')).toBe(true);
  expect(con.querySelectorAll('.sytue-skeleton')).toHaveLength(5);
});

test('las tarjetas de álbum son 4:3 como `TarjetaAlbum`; las compartidas, cuadradas', () => {
  const h = montar(<EsqueletoAlbumes cuantas={5} />);
  const tarjetas = h.querySelectorAll('ul.grid > li');
  expect(tarjetas).toHaveLength(5);
  expect(h.querySelectorAll('.aspect-\\[4\\/3\\]')).toHaveLength(5);
  expect(h.querySelector('ul.grid')?.classList.contains('gap-6')).toBe(true);
  render(null, host);
  host.remove();
  const c = montar(<EsqueletoAlbumes cuantas={3} cuadradas />);
  expect(c.querySelectorAll('.aspect-square')).toHaveLength(3);
  expect(c.querySelector('ul.grid')?.classList.contains('gap-3')).toBe(true);
});

test('las ocho pantallas cargan con esqueleto y no con la rueda suelta', () => {
  const fuente = (f: string): string => readFileSync(new URL(`./${f}`, import.meta.url), 'utf8');
  const usos: Record<string, string> = {
    'PhotosApp.tsx': 'EsqueletoFilas',
    'AlbumView.tsx': 'EsqueletoFilas',
    'Albums.tsx': 'EsqueletoAlbumes',
    'Compartidos.tsx': 'EsqueletoAlbumes',
    'Trash.tsx': 'EsqueletoFilas',
    'Similares.tsx': 'EsqueletoFilas',
    'Cerca.tsx': 'EsqueletoFilas',
    'Duplicadas.tsx': 'EsqueletoDuplicadas',
  };
  for (const [f, componente] of Object.entries(usos)) {
    const s = fuente(f);
    expect(s).toContain(`<${componente}`);
    // La galería conserva la rueda, pero solo para las páginas siguientes.
    if (f === 'PhotosApp.tsx') {
      expect(s).toContain('cargando && fotos.length > 0 ? <Spinner />');
      // Mientras se escribe la línea está vacía y aún no se ha pedido: el esqueleto
      // sigue, para que no haya un hueco en blanco entre tecla y tecla.
      expect(s).toContain('(cargando || pendiente) && fotos.length === 0 ? (');
    } else expect(s).not.toContain('<Spinner');
  }
});
