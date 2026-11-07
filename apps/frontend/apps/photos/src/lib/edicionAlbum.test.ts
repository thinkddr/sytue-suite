/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import { esPortada, mandoPortada, tituloRenombrado } from './edicionAlbum.js';

test('renombrar guarda el título escrito, sin los espacios de los lados', () => {
  expect(tituloRenombrado('Verano', '  Verano en Getaria ')).toBe('Verano en Getaria');
});

test('cancelar, dejarlo vacío o no tocarlo no manda nada', () => {
  expect(tituloRenombrado('Verano', null)).toBeNull();
  expect(tituloRenombrado('Verano', '   ')).toBeNull();
  expect(tituloRenombrado('Verano', ' Verano ')).toBeNull();
});

test('la portada es la que dice el servicio, y sin álbum no hay ninguna', () => {
  expect(esPortada({ coverAssetId: 'f1' }, 'f1')).toBe(true);
  expect(esPortada({ coverAssetId: 'f1' }, 'f2')).toBe(false);
  expect(esPortada({ coverAssetId: null }, 'f1')).toBe(false);
  expect(esPortada(null, 'f1')).toBe(false);
});

test('el mando de portada no es un conmutador: en la que ya lo es lo dice y va apagado', () => {
  expect(mandoPortada({ coverAssetId: 'f1' }, 'f1')).toEqual({
    clave: 'album.isCover',
    apagado: true,
  });
  expect(mandoPortada({ coverAssetId: 'f1' }, 'f2')).toEqual({
    clave: 'album.setCover',
    apagado: false,
  });
  expect(mandoPortada(null, 'f1')).toEqual({ clave: 'album.setCover', apagado: false });
});
