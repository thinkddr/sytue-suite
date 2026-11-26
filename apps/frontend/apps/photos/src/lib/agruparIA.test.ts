/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La instrucción, el contexto y el lector de «Organizar con IA» dicen lo MISMO que
 * los de las apps nativas (`apps/nucleo/api/photos_agrupar.go`): las dos pruebas
 * leen los mismos casos. Si una cambia sin la otra, el modelo lee una cosa en la
 * web y otra en Fyne, y eso no da ningún error: solo álbumes distintos.
 */
import { expect, test } from 'bun:test';
import {
  contextoAgrupar,
  instruccionAgrupar,
  leerAgrupacion,
  type AlbumPropuesto,
  type FotoParaAgrupar,
} from './agruparIA.js';

interface Casos {
  instrucciones: { idioma: string; texto: string }[];
  contextos: { fotos: FotoParaAgrupar[]; contexto: string; ids: string[] }[];
  lector: { caso: string; texto: string; ids: string[]; albumes: AlbumPropuesto[] | null }[];
}

const CASOS = new URL('../../../../../nucleo/api/testdata/agrupar.json', import.meta.url);
const casos = (await Bun.file(CASOS).json()) as Casos;

test('hay casos de los tres, para no pasar en verde sin leer nada', () => {
  expect(casos.instrucciones.length).toBeGreaterThanOrEqual(3);
  expect(casos.contextos.length).toBeGreaterThanOrEqual(1);
  expect(casos.lector.length).toBeGreaterThanOrEqual(10);
});

test('la instrucción es la de Go, con el idioma nombrado', () => {
  for (const c of casos.instrucciones) {
    expect([c.idioma, instruccionAgrupar(c.idioma)]).toEqual([c.idioma, c.texto]);
  }
});

test('el contexto es el de Go: números cortos, sin las fotos sin nada escrito', () => {
  for (const c of casos.contextos) {
    expect(contextoAgrupar(c.fotos)).toEqual({ contexto: c.contexto, ids: c.ids });
  }
});

test('el lector es el de Go: ids de la entrada, sin repetir, títulos que valen y topes', () => {
  for (const c of casos.lector) {
    expect([c.caso, leerAgrupacion(c.texto, c.ids)]).toEqual([c.caso, c.albumes]);
  }
});
