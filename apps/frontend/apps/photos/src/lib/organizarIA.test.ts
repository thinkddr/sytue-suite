/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * De qué fotos parte «Organizar con IA», qué crea con lo aceptado y cómo se
 * deshace. Los mismos casos que `organizar_test.go` de Fyne.
 */
import { expect, test } from 'bun:test';
import { TOPE_ORGANIZAR } from './agruparIA.js';
import {
  crearAlbumes,
  deshacerOrganizar,
  fotosSinAlbum,
  planDeAlbumes,
  POR_PAGINA_ORGANIZAR,
  type AlbumMio,
} from './organizarIA.js';

const foto = (id: string) => ({ id });
const album = (id: string, title: string, count = 2): AlbumMio => ({ id, title, count });

/** Una línea de tiempo de `n` fotos («f0», «f1»…) servida en páginas del tamaño dado. */
function linea(n: number, porPagina = POR_PAGINA_ORGANIZAR) {
  const todas = Array.from({ length: n }, (_, i) => foto(`f${i}`));
  const pedidas: (string | undefined)[] = [];
  return {
    pedidas,
    pagina: (cursor: string | undefined) => {
      pedidas.push(cursor);
      const desde = cursor ? Number(cursor) : 0;
      const hasta = Math.min(desde + porPagina, todas.length);
      return Promise.resolve({
        items: todas.slice(desde, hasta),
        ...(hasta < todas.length ? { nextCursor: String(hasta) } : {}),
      });
    },
  };
}

test('parte de las fotos que no están en ningún álbum de la persona', async () => {
  const l = linea(6);
  const pedidosDeAlbum: string[] = [];
  const r = await fotosSinAlbum({
    albumes: () => Promise.resolve([album('a1', 'Verano'), album('vacio', 'Nada', 0)]),
    fotosDelAlbum: (id) => {
      pedidosDeAlbum.push(id);
      return Promise.resolve([foto('f1'), foto('f4')]);
    },
    pagina: l.pagina,
  });
  expect(r.fotos.map((f) => f.id)).toEqual(['f0', 'f2', 'f3', 'f5']);
  expect(r.topado).toBe(false);
  // El álbum vacío no se pide: no hay nada que leer en él.
  expect(pedidosDeAlbum).toEqual(['a1']);
});

test('se para en el tope y dice que quedan', async () => {
  const l = linea(TOPE_ORGANIZAR + 50);
  const r = await fotosSinAlbum({
    albumes: () => Promise.resolve([]),
    fotosDelAlbum: () => Promise.resolve([]),
    pagina: l.pagina,
  });
  expect(r.fotos.length).toBe(TOPE_ORGANIZAR);
  expect(r.topado).toBe(true);
  // Con la primera página llena ya no se pide la segunda.
  expect(l.pedidas).toEqual([undefined]);
});

test('no recorre la biblioteca entera buscando las últimas sueltas', async () => {
  const l = linea(5000);
  const r = await fotosSinAlbum({
    albumes: () => Promise.resolve([album('todo', 'Todo', 5000)]),
    fotosDelAlbum: () => Promise.resolve(Array.from({ length: 4990 }, (_, i) => foto(`f${i}`))),
    pagina: l.pagina,
  });
  expect(r.fotos).toEqual([]);
  expect(r.topado).toBe(true);
  expect(l.pedidas.length).toBe(10);
});

test('si un álbum no se puede leer no se sigue', async () => {
  const l = linea(4);
  const fallo = new Error('HTTP 500');
  await expect(
    fotosSinAlbum({
      albumes: () => Promise.resolve([album('a1', 'Verano')]),
      fotosDelAlbum: () => Promise.reject(fallo),
      pagina: l.pagina,
    }),
  ).rejects.toBe(fallo);
  expect(l.pedidas).toEqual([]);
});

test('el plan va al álbum que ya se llama igual y junta los que se llaman igual', () => {
  const existentes = [album('v', 'Verano en Cádiz', 3), album('n', 'Navidad')];
  const plan = planDeAlbumes(
    [
      { titulo: '  VERANO EN CADIZ ', ids: ['1', '2'], incluido: true },
      { titulo: 'Montaña', ids: ['3', '4'], incluido: true },
      { titulo: 'montana', ids: ['4', '5'], incluido: true },
      { titulo: 'Navidad', ids: ['6', '7'], incluido: false },
      { titulo: '   ', ids: ['8', '9'], incluido: true },
    ],
    existentes,
  );
  expect(plan).toEqual([
    { titulo: 'VERANO EN CADIZ', existente: existentes[0]!, ids: ['1', '2'] },
    { titulo: 'Montaña', existente: null, ids: ['3', '4', '5'] },
  ]);
});

test('crear: los nuevos se crean y los que existen solo reciben las fotos', async () => {
  const llamadas: string[] = [];
  const { hechos, fallidos } = await crearAlbumes(
    [
      { titulo: 'Verano', existente: album('v', 'Verano', 3), ids: ['1', '2'] },
      { titulo: 'Montaña', existente: null, ids: ['3', '4'] },
      { titulo: 'Roto', existente: null, ids: ['5', '6'] },
    ],
    {
      crearAlbum: (t) => {
        llamadas.push(`crear ${t}`);
        return t === 'Roto' ? Promise.reject(new Error('x')) : Promise.resolve(album('m', t, 0));
      },
      anyadirAlAlbum: (id, ids) => {
        llamadas.push(`añadir ${id} ${ids.join(',')}`);
        return Promise.resolve({ added: ids.length });
      },
    },
  );
  expect(llamadas).toEqual(['añadir v 1,2', 'crear Montaña', 'añadir m 3,4', 'crear Roto']);
  expect(fallidos).toBe(1);
  expect(hechos).toEqual([
    { album: album('v', 'Verano', 5), nuevo: false, ids: ['1', '2'] },
    { album: album('m', 'Montaña', 2), nuevo: true, ids: ['3', '4'] },
  ]);
});

test('deshacer borra los nuevos y saca las fotos de los que ya existían', async () => {
  const llamadas: string[] = [];
  const { fallidas } = await deshacerOrganizar(
    [
      { album: album('v', 'Verano', 5), nuevo: false, ids: ['1', '2'] },
      { album: album('m', 'Montaña', 2), nuevo: true, ids: ['3', '4'] },
    ],
    {
      borrarAlbum: (id) => {
        llamadas.push(`borrar ${id}`);
        return Promise.resolve();
      },
      quitarDelAlbum: (id, f) => {
        llamadas.push(`quitar ${id} ${f}`);
        return f === '2' ? Promise.reject(new Error('x')) : Promise.resolve();
      },
    },
  );
  expect(llamadas.sort()).toEqual(['borrar m', 'quitar v 1', 'quitar v 2']);
  expect(fallidas).toBe(1);
});
