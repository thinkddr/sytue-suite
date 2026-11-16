/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El orden de las tarjetas de álbum. Los casos son los mismos que comprueba Fyne
 * (`apps/fyne/apps/photos/orden_albumes_test.go`): las dos apps ordenan igual.
 */
import { afterEach, expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import type { Locale } from '@sytue/i18n';
import {
  CLAVE_ORDEN_ALBUMES,
  esOrdenAlbum,
  guardarOrdenAlbumes,
  leerOrdenAlbumes,
  ordenarAlbumes,
  type OrdenAlbum,
} from './ordenAlbumes.js';

const CASOS = new URL(
  '../../../../../fyne/apps/photos/testdata/orden-albumes.json',
  import.meta.url,
);

interface Casos {
  albums: { id: string; title: string; count: number; updatedAt: string }[];
  casos: { orden: OrdenAlbum; locale: Locale; ids: string[] }[];
}
const { albums, casos } = JSON.parse(readFileSync(CASOS, 'utf8')) as Casos;

for (const c of casos) {
  test(`ordena por «${c.orden}» como Fyne`, () => {
    expect(ordenarAlbumes(albums, c.orden, c.locale).map((a) => a.id)).toEqual(c.ids);
  });
}

test('no toca la lista de entrada', () => {
  const antes = albums.map((a) => a.id);
  ordenarAlbumes(albums, 'title', 'es');
  expect(albums.map((a) => a.id)).toEqual(antes);
});

test('solo reconoce los tres órdenes', () => {
  expect(esOrdenAlbum('count')).toBe(true);
  expect(esOrdenAlbum('size')).toBe(false);
  expect(esOrdenAlbum(null)).toBe(false);
});

const original = globalThis.localStorage;
afterEach(() => {
  Object.defineProperty(globalThis, 'localStorage', { value: original, configurable: true });
});

function almacen(inicial: Record<string, string> = {}): Record<string, string> {
  const datos = { ...inicial };
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem: (k: string) => datos[k] ?? null,
      setItem: (k: string, v: string) => {
        datos[k] = v;
      },
    },
  });
  return datos;
}

test('recuerda el orden elegido y descarta lo que no reconoce', () => {
  const datos = almacen();
  expect(leerOrdenAlbumes()).toBe('recent');
  guardarOrdenAlbumes('count');
  expect(datos[CLAVE_ORDEN_ALBUMES]).toBe('count');
  expect(leerOrdenAlbumes()).toBe('count');
  datos[CLAVE_ORDEN_ALBUMES] = 'tamaño';
  expect(leerOrdenAlbumes()).toBe('recent');
});

test('un almacenamiento bloqueado no rompe nada', () => {
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    value: {
      getItem: () => {
        throw new Error('bloqueado');
      },
      setItem: () => {
        throw new Error('bloqueado');
      },
    },
  });
  expect(leerOrdenAlbumes()).toBe('recent');
  expect(() => guardarOrdenAlbumes('title')).not.toThrow();
});
