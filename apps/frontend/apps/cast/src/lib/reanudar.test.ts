/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { expect, test } from 'bun:test';
import {
  CADUCIDAD_MS,
  MAX_ENTRADAS,
  anotar,
  desde,
  guardar,
  leer,
  merece,
  recordar,
  visto,
  type Almacen,
  type Punto,
} from './reanudar.js';

const DIA = 24 * 60 * 60 * 1000;

function enMemoria(): Almacen & { datos: Map<string, string> } {
  const datos = new Map<string, string>();
  return {
    datos,
    getItem: (k) => datos.get(k) ?? null,
    setItem: (k, v) => void datos.set(k, v),
  };
}

test('solo se recuerda entre los 10 s y el 95 %', () => {
  expect(merece(9.9, 600)).toBe(false);
  expect(merece(10, 600)).toBe(true);
  expect(merece(569, 600)).toBe(true);
  expect(merece(570, 600)).toBe(false);
  // Sin duración conocida no hay porcentaje que valga.
  expect(merece(30, 0)).toBe(false);
  expect(merece(30, Number.NaN)).toBe(false);
});

test('anotar guarda la posición y la olvida al acabar o al volver al principio', () => {
  let p = anotar({}, 'a', 120, 600, 1_000);
  expect(desde(p, 'a', 2_000)).toBe(120);
  expect(visto(p, 'a', 2_000)).toBeCloseTo(0.2);
  p = anotar(p, 'a', 590, 600, 3_000);
  expect(desde(p, 'a', 4_000)).toBeNull();
  p = anotar(anotar(p, 'a', 120, 600, 5_000), 'a', 3, 600, 6_000);
  expect(p.a).toBeUndefined();
});

test('el recuerdo caduca a los 90 días', () => {
  const p = anotar({}, 'a', 120, 600, 0);
  expect(desde(p, 'a', CADUCIDAD_MS - 1)).toBe(120);
  expect(desde(p, 'a', CADUCIDAD_MS)).toBeNull();
  expect(visto(p, 'a', CADUCIDAD_MS)).toBeNull();
  // Y al anotar otro se poda.
  expect(anotar(p, 'b', 60, 600, CADUCIDAD_MS).a).toBeUndefined();
});

test('se recuerdan como mucho 200 vídeos: los más recientes', () => {
  let p = {};
  for (let i = 0; i <= MAX_ENTRADAS; i++) p = anotar(p, `v${i}`, 60, 600, i * 1000);
  expect(Object.keys(p)).toHaveLength(MAX_ENTRADAS);
  expect(desde(p, 'v0', MAX_ENTRADAS * 1000)).toBeNull();
  expect(desde(p, `v${MAX_ENTRADAS}`, MAX_ENTRADAS * 1000)).toBe(60);
});

test('cada persona tiene lo suyo, y sin persona no se recuerda nada', () => {
  const a = enMemoria();
  recordar('ana', 'v', 120, 600, a);
  expect(desde(leer('ana', a), 'v', Date.now())).toBe(120);
  expect(leer('luis', a)).toEqual({});
  recordar(null, 'v', 300, 600, a);
  expect(leer(null, a)).toEqual({});
  expect(a.datos.size).toBe(1);
});

test('un almacén roto o con basura no rompe nada', () => {
  const roto: Almacen = {
    getItem: () => {
      throw new Error('SecurityError');
    },
    setItem: () => {
      throw new Error('QuotaExceededError');
    },
  };
  expect(leer('ana', roto)).toEqual({});
  expect(() => guardar('ana', { v: { s: 20, t: 60, en: Date.now() } }, roto)).not.toThrow();
  const a = enMemoria();
  a.setItem('sytue.cast.reanudar:ana', '{no es json');
  expect(leer('ana', a)).toEqual({});
  a.setItem('sytue.cast.reanudar:ana', JSON.stringify({ v: { s: 'x' }, w: null }));
  expect(leer('ana', a)).toEqual({});
  const bueno: Punto = { s: 30, t: 600, en: Date.now() - DIA };
  a.setItem('sytue.cast.reanudar:ana', JSON.stringify({ v: bueno }));
  expect(desde(leer('ana', a), 'v', Date.now())).toBe(30);
});
