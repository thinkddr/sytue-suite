/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Las flechas por la galería: van de foto en foto por su sitio en pantalla, y no
 * se meten en el corazón de favorita, que también es un botón de la tesela.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterAll, afterEach, beforeAll, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { render } from 'preact';
import { act } from 'preact/test-utils';
// Alias: `useT` no es un hook de verdad (no guarda estado), y aquí se llama fuera de un componente.
import { useT as traductor } from '../i18n/index.js';
import type { Foto } from '../client/photosApi.js';
import { Miniatura, alTeclearEnGaleria } from './PhotosApp.js';

// El DOM falso solo mientras corre ESTE fichero (ver miniatura-seleccion.test.tsx).
let registrado = false;
beforeAll(() => {
  if (typeof document !== 'undefined') return;
  GlobalRegistrator.register();
  registrado = true;
});
afterAll(async () => {
  if (registrado) await GlobalRegistrator.unregister();
});

const t = traductor('es');
const foto = (i: number) =>
  ({
    id: `f${i}`,
    name: `foto${i}.jpg`,
    width: 400,
    height: 300,
    favorite: false,
    takenAtFromExif: true,
  }) as unknown as Foto;

let host: HTMLElement;
afterEach(() => {
  render(null, host);
  host.remove();
});

/** Seis fotos en dos filas de tres, de 100 × 80; happy-dom no maqueta y la caja se impone. */
function montar(): HTMLElement[] {
  host = document.createElement('div');
  document.body.appendChild(host);
  act(() =>
    render(
      // oxlint-disable-next-line jsx-a11y/no-noninteractive-element-interactions -- como en la galería
      <ul class="filas" onKeyDown={alTeclearEnGaleria}>
        {[0, 1, 2, 3, 4, 5].map((i) => (
          <Miniatura key={i} foto={foto(i)} t={t} onOpen={() => {}} onFavorite={() => {}} />
        ))}
      </ul>,
      host,
    ),
  );
  const teselas = [...host.querySelectorAll<HTMLElement>('[data-foto-tesela]')];
  teselas.forEach((b, i) => {
    b.getBoundingClientRect = () =>
      ({ left: (i % 3) * 104, top: Math.floor(i / 3) * 84, width: 100, height: 80 }) as DOMRect;
  });
  return teselas;
}

const pulsar = (el: HTMLElement, key: string) =>
  act(() => {
    el.dispatchEvent(new KeyboardEvent('keydown', { key, bubbles: true, cancelable: true }));
  });

test('→ pasa a la siguiente foto y ↓ a la de debajo', () => {
  const fotos = montar();
  fotos[0]!.focus();
  pulsar(fotos[0]!, 'ArrowRight');
  expect(document.activeElement).toBe(fotos[1]!);
  pulsar(fotos[1]!, 'ArrowDown');
  expect(document.activeElement).toBe(fotos[4]!);
  pulsar(fotos[4]!, 'End');
  expect(document.activeElement).toBe(fotos[5]!);
  pulsar(fotos[5]!, 'Home');
  expect(document.activeElement).toBe(fotos[0]!);
});

test('las flechas desde el corazón no mueven el foco: no es una tesela', () => {
  const fotos = montar();
  const corazon = host.querySelector<HTMLElement>('button[aria-pressed]')!;
  corazon.focus();
  pulsar(corazon, 'ArrowRight');
  expect(document.activeElement).toBe(corazon);
  expect(fotos.length).toBe(6);
});

test('la galería, los álbumes, las parecidas y «cerca» escuchan las flechas', () => {
  const fuente = (f: string) => readFileSync(new URL(f, import.meta.url), 'utf8');
  expect(fuente('./PhotosApp.tsx')).toContain('<div onKeyDown={alTeclearEnGaleria}>');
  for (const f of ['./AlbumView.tsx', './Similares.tsx', './Cerca.tsx']) {
    expect(fuente(f)).toContain('<ul class="filas" onKeyDown={alTeclearEnGaleria}>');
  }
});
