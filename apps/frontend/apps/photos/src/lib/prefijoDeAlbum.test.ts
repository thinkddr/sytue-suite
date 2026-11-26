/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El ZIP de «Descargar el álbum» se llama como el álbum, sin lo que un sistema de
 * ficheros rechaza: un «Viaje 2026/Italia» daría una carpeta en vez de un fichero.
 */
import { expect, test } from 'bun:test';
import { nombreDeZip, prefijoDeAlbum } from './descargaZip.js';

test('el título del álbum, sin barras ni dos puntos, y el respaldo si no queda nada', () => {
  expect(prefijoDeAlbum('Viaje 2026/Italia', 'Fotos')).toBe('Viaje 2026 Italia');
  expect(prefijoDeAlbum('  Boda: «Ana»  ', 'Fotos')).toBe('Boda «Ana»');
  expect(prefijoDeAlbum('/// ', 'Fotos')).toBe('Fotos');
  expect(prefijoDeAlbum('a'.repeat(200), 'Fotos')).toHaveLength(80);
  expect(nombreDeZip(prefijoDeAlbum('Verano', 'Fotos'), new Date(2026, 6, 4))).toBe(
    'Verano 2026-07-04',
  );
});
