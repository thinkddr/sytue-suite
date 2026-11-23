/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La galería cifrada (ADR 0086) sobre el repositorio de Drive de verdad
 * (`PrivateDriveRepository`) y el almacén sintético de sus pruebas: índice, álbumes,
 * favoritas y ediciones solo como ficheros cifrados del Drive de la bóveda.
 */
import { expect, test } from 'bun:test';
import { readFileSync } from 'node:fs';
import { privateDriveWriteFixture } from '../../../../packages/offline/src/matrix/private-drive-fixture.js';
import { leerExif, fechaExif } from '../lib/exif.js';
import { destinoCifrado } from './driveCifrado.js';
import {
  buscarCifradas,
  CARPETA_GALERIA,
  decodificarAlbum,
  decodificarEntrada,
  GaleriaCifrada,
  type Procesar,
} from './galeriaCifrada.js';

const testdata = new URL('../../../../../nucleo/photos/galeriacifrada/testdata/', import.meta.url);

/** Un JPEG mínimo con APP1: orientación 6 y fecha de captura con desfase. */
function jpegConExif(fecha = '2026:07:14 20:32:05', desfase = '+02:00'): Uint8Array<ArrayBuffer> {
  const tiff = new Uint8Array(96);
  const v = new DataView(tiff.buffer);
  tiff.set([0x49, 0x49]);
  v.setUint16(2, 42, true);
  v.setUint32(4, 8, true);
  v.setUint16(8, 2, true);
  const campo = (off: number, tag: number, tipo: number, n: number, valor: number) => {
    v.setUint16(off, tag, true);
    v.setUint16(off + 2, tipo, true);
    v.setUint32(off + 4, n, true);
    v.setUint32(off + 8, valor, true);
  };
  campo(10, 0x0112, 3, 1, 6);
  campo(22, 0x8769, 4, 1, 38);
  v.setUint16(38, 2, true);
  campo(40, 0x9003, 2, 20, 68);
  campo(52, 0x9011, 2, 7, 88);
  tiff.set(new TextEncoder().encode(fecha + '\0'), 68);
  tiff.set(new TextEncoder().encode(desfase + '\0'), 88);
  const app1 = new Uint8Array(4 + 6 + tiff.length);
  app1.set([0xff, 0xe1, ((app1.length - 2) >> 8) & 0xff, (app1.length - 2) & 0xff]);
  app1.set(new TextEncoder().encode('Exif\0\0'), 4);
  app1.set(tiff, 10);
  const out = new Uint8Array(2 + app1.length + 2);
  out.set([0xff, 0xd8]);
  out.set(app1, 2);
  out.set([0xff, 0xd9], 2 + app1.length);
  return out;
}

const procesar: Procesar = (imagen) =>
  Promise.resolve({
    ancho: 3024,
    alto: 4032,
    miniatura: new Blob([`MINIATURA ${imagen.size}`], { type: 'image/jpeg' }),
  });

test('lee la fecha y la orientación del EXIF como el lector de Go', () => {
  expect(leerExif(jpegConExif())).toEqual({ tomada: '2026-07-14T18:32:05.000Z', orientacion: 6 });
  expect(leerExif(jpegConExif('2026:07:14 18:32:05', ''))).toEqual({
    tomada: '2026-07-14T18:32:05.000Z',
    orientacion: 6,
  });
  expect(fechaExif('0000:00:00 00:00:00')).toBeNull();
  expect(leerExif(new Uint8Array([0xff, 0xd8, 0xff, 0xe1, 0xff, 0xff]))).toEqual({
    tomada: null,
    orientacion: 0,
  });
  expect(leerExif(new TextEncoder().encode('no es un jpeg'))).toEqual({
    tomada: null,
    orientacion: 0,
  });
});

test('el formato es el mismo que escribe y lee el nativo', () => {
  const e = decodificarEntrada(readFileSync(new URL('entrada-v1.json', testdata), 'utf8'))!;
  expect(e.favorita).toBe(true);
  expect(e.tomada).toBe('2026-07-14T18:32:05.000Z');
  expect(e.ancho).toBe(4032);
  expect(e.albumes).toEqual(['a1b2c3d4-0000-4000-8000-000000000001']);
  const a = decodificarAlbum(readFileSync(new URL('album-v1.json', testdata), 'utf8'))!;
  expect(a.titulo).toBe('Verano');
  for (const malo of [
    '{"v":2,"tipo":"sytue.photos.cifrada.foto","foto":"a","tomada":"2026-01-01T00:00:00Z"}',
    '{"v":1,"tipo":"sytue.photos.cifrada.foto","foto":"../x","tomada":"2026-01-01T00:00:00Z"}',
    '{"v":1,"tipo":"sytue.photos.cifrada.foto","foto":"a","tomada":"ayer"}',
    'no es json',
  ])
    expect(decodificarEntrada(malo)).toBeNull();
});

test('subir indexa, y favoritas, álbumes y ediciones van solo a la bóveda', async () => {
  const f = await privateDriveWriteFixture(),
    repositorio = f.open();
  try {
    await repositorio.initialize(f.state);
    let n = 0;
    const galeria = new GaleriaCifrada(repositorio, {
      procesar,
      nuevoId: () => `album-${++n}`,
      ahora: () => new Date('2026-11-23T10:00:00Z'),
    });
    const destino = destinoCifrado(
      { repositorio, signal: new AbortController().signal },
      (fila, fichero) => galeria.indexar(fila, fichero),
    );
    const contenido = jpegConExif();
    await destino.subir(new File([contenido], 'IMG_0001.jpg', { type: 'image/jpeg' }));
    await destino.subir(new File(['SIN EXIF'], 'captura.jpg', { type: 'image/jpeg' }));

    let indice = await galeria.leer();
    expect(indice.fotos.map((x) => x.nombre)).toEqual(['captura.jpg', 'IMG_0001.jpg']);
    const [captura, foto] = indice.fotos as [
      (typeof indice.fotos)[number],
      (typeof indice.fotos)[number],
    ];
    expect(foto.conEntrada && !foto.pendiente).toBe(true);
    expect(foto.entrada.exif).toBe(true);
    expect(foto.entrada.tomada).toBe('2026-07-14T18:32:05.000Z');
    expect(captura.entrada.exif).toBe(false);
    expect(await (await galeria.miniatura(foto)).text()).toBe(`MINIATURA ${contenido.length}`);
    // La carpeta del índice está dentro de «Fotos» y no sale como foto.
    expect(indice.fotos.some((x) => x.nombre === CARPETA_GALERIA)).toBe(false);

    const album = await galeria.crearAlbum('  Verano ');
    await galeria.marcarFavorita(foto, true);
    await galeria.ponerEnAlbum(foto, album.id, true);
    const bloques = f.blobs.size;
    // Repetirlo no escribe nada.
    await galeria.marcarFavorita(foto, true);
    await galeria.ponerEnAlbum(foto, album.id, true);
    expect(f.blobs.size).toBe(bloques);

    indice = await new GaleriaCifrada(repositorio, { procesar }).leer();
    const vuelta = indice.fotos.find((x) => x.id === foto.id)!;
    expect(vuelta.entrada.favorita).toBe(true);
    expect(vuelta.entrada.albumes).toEqual([album.id]);
    expect(indice.albumes.map((a) => a.titulo)).toEqual(['Verano']);

    // Editar sube una versión nueva del mismo fichero y conserva fecha, favorita y álbum.
    await galeria.guardarEdicion(vuelta, new Blob(['EDITADA'], { type: 'image/jpeg' }));
    const versiones = await repositorio.versions(foto.id);
    expect(versiones.versions.length).toBe(2);
    indice = await galeria.leer();
    const editada = indice.fotos.find((x) => x.id === foto.id)!;
    expect(editada.pendiente).toBe(false);
    expect(editada.entrada.favorita).toBe(true);
    expect(editada.entrada.albumes).toEqual([album.id]);
    expect(editada.entrada.tomada).toBe('2026-07-14T18:32:05.000Z');
    expect(await (await galeria.original(editada)).text()).toBe('EDITADA');

    await galeria.ponerEnAlbum(editada, album.id, false);
    indice = await galeria.leer();
    expect(indice.fotos.find((x) => x.id === foto.id)!.entrada.albumes).toEqual([]);

    // Al servidor solo llega texto cifrado: ni nombres, ni fechas, ni el título.
    for (const cifrado of f.blobs.values()) {
      const t = new TextDecoder().decode(cifrado);
      expect(t).not.toContain('Verano');
      expect(t).not.toContain('2026-07-14');
      expect(t).not.toContain('MINIATURA');
    }
  } finally {
    await repositorio.close();
    await f.close();
  }
});

test('una foto sin entrada sale igual y se reconstruye desde el original', async () => {
  const f = await privateDriveWriteFixture(),
    repositorio = f.open();
  try {
    await repositorio.initialize(f.state);
    // Subida sin indexar (otra versión de la app, o un fallo a medias).
    const destino = destinoCifrado({ repositorio, signal: new AbortController().signal });
    await destino.subir(new File([jpegConExif()], 'IMG_0002.jpg', { type: 'image/jpeg' }));
    const galeria = new GaleriaCifrada(repositorio, { procesar });
    let [foto] = (await galeria.leer()).fotos;
    expect(foto!.conEntrada).toBe(false);
    expect(foto!.pendiente).toBe(true);
    expect(foto!.nombre).toBe('IMG_0002.jpg');
    // Marcar favorita sin entrada la crea.
    await galeria.marcarFavorita(foto!, true);
    await galeria.reconstruir((await galeria.leer()).fotos[0]!);
    [foto] = (await galeria.leer()).fotos;
    expect(foto!.conEntrada && !foto!.pendiente).toBe(true);
    expect(foto!.entrada.favorita).toBe(true);
    expect(foto!.entrada.exif).toBe(true);
  } finally {
    await repositorio.close();
    await f.close();
  }
});

test('busca por nombre y por fecha en el índice descifrado', () => {
  const base = {
    tipo: 'image/jpeg',
    bytes: 1,
    suma: '',
    conEntrada: true,
    pendiente: false,
  };
  const entrada = (foto: string, nombre: string, tomada: string) => ({
    v: 1 as const,
    tipo: 'sytue.photos.cifrada.foto' as const,
    foto,
    suma: '',
    nombre,
    tomada,
    exif: true,
    ancho: 0,
    alto: 0,
    favorita: false,
    albumes: [],
    miniatura: '',
    actualizada: '',
  });
  const fotos = [
    {
      ...base,
      id: 'a',
      nombre: 'Playa.jpg',
      entrada: entrada('a', 'Playa.jpg', '2026-07-14T10:00:00.000Z'),
    },
    {
      ...base,
      id: 'b',
      nombre: 'nieve.jpg',
      entrada: entrada('b', 'nieve.jpg', '2025-01-02T10:00:00.000Z'),
    },
  ];
  expect(buscarCifradas(fotos, 'playa').map((f) => f.id)).toEqual(['a']);
  expect(buscarCifradas(fotos, '2025').map((f) => f.id)).toEqual(['b']);
  expect(buscarCifradas(fotos, '14/07/2026').map((f) => f.id)).toEqual(['a']);
  expect(buscarCifradas(fotos, 'julio', () => 'julio de 2026').map((f) => f.id)).toEqual([
    'a',
    'b',
  ]);
  expect(buscarCifradas(fotos, '').length).toBe(2);
});
