/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Serialización: ida y vuelta estable, versiones, migraciones y saneado de la
 * frontera de confianza (todo lo que llega de un documento ajeno).
 */
import { expect, test } from 'bun:test';
import {
  ESCENA_VERSION,
  LIMITES,
  agregarCapa,
  agregarNodo,
  agrupar,
  escenaVacia,
  leer,
  migrar,
  sanear,
  serializar,
} from './index.js';
import type { Escena, Migracion, Nodo } from './index.js';

function completa(): Escena {
  let e = escenaVacia({ ancho: 640, alto: 480 });
  e = { ...e, titulo: 'Flujo de alta', descripcion: 'Tres pasos', fondo: '#FFFFFF' };
  const nodos: Nodo[] = [
    {
      id: 'a',
      tipo: 'forma',
      forma: 'hexagon',
      x: 10.12345,
      y: 20,
      w: 120,
      h: 60,
      rot: 30,
      texto: 'Inicio',
      nombre: 'Inicio',
      estilo: { relleno: '#AABBCC', grosor: 2, discontinuo: true },
    },
    { id: 'b', tipo: 'forma', forma: 'cylinder', x: 300, y: 20, w: 80, h: 90 },
    {
      id: 't',
      tipo: 'texto',
      x: 0,
      y: 200,
      w: 200,
      h: 40,
      texto: 'Hola\nmundo',
      alinear: 'middle',
      negrita: true,
      tamTexto: 18,
    },
    {
      id: 'i',
      tipo: 'imagen',
      x: 0,
      y: 0,
      w: 10,
      h: 10,
      href: 'data:image/png;base64,AAAA',
      alt: 'logo',
    },
    { id: 'l', tipo: 'linea', x1: 0, y1: 0, x2: 50, y2: 50, flechaFin: true },
    {
      id: 'k',
      tipo: 'conector',
      trazado: 'elbow',
      desde: { nodo: 'a', lado: 'right' },
      hasta: { nodo: 'b' },
      flechaFin: true,
    },
    {
      id: 'p',
      tipo: 'trazo',
      puntos: [1, 2, 3, 4, 5.5, 6.25],
      estilo: { trazo: '#112233', grosor: 3 },
    },
  ];
  for (const n of nodos) e = agregarNodo(e, 'capa1', n);
  e = agregarCapa(e, { id: 'c2', nombre: 'Notas' });
  return agrupar(e, ['t', 'l'], 'g');
}

test('serializar es idempotente: leer(serializar(e)) vuelve a dar el mismo texto', () => {
  const t1 = serializar(completa());
  const r = leer(t1);
  expect(r.ok).toBe(true);
  if (!r.ok) return;
  expect(serializar(r.escena)).toBe(t1);
  expect(r.escena.v).toBe(ESCENA_VERSION);
});

test('el orden de las claves de entrada no cambia el texto', () => {
  const e = completa();
  const t = serializar(e);
  const barajado = JSON.parse(t) as Record<string, unknown>;
  const invertido = Object.fromEntries(Object.entries(barajado).reverse());
  const r = sanear(invertido);
  expect(r.ok && serializar(r.escena)).toBe(t);
});

test('redondea a 3 decimales y normaliza el color a minúsculas', () => {
  const r = leer(serializar(completa()));
  if (!r.ok) throw new Error('no se pudo leer');
  const a = r.escena.capas[0]!.nodos.find((n) => n.id === 'a')!;
  expect(a).toMatchObject({ x: 10.123, estilo: { relleno: '#aabbcc' } });
  expect(r.escena.fondo).toBe('#ffffff');
});

test('texto que no es JSON o no es una escena', () => {
  expect(leer('{no')).toEqual({ ok: false, motivo: 'json' });
  expect(leer('[]')).toEqual({ ok: false, motivo: 'forma' });
  expect(leer('{"capas":[]}')).toEqual({ ok: false, motivo: 'forma' }); // sin versión
  expect(leer('{"v":0,"capas":[]}')).toEqual({ ok: false, motivo: 'forma' });
  expect(leer('{"v":1.5,"capas":[]}')).toEqual({ ok: false, motivo: 'forma' });
});

test('una versión futura no se interpreta (se perdería lo que no entendemos)', () => {
  expect(leer(`{"v":${ESCENA_VERSION + 1},"capas":[]}`)).toEqual({
    ok: false,
    motivo: 'version-futura',
  });
});

test('las migraciones encadenan n → n+1 hasta la versión actual', () => {
  const m1: Migracion = (r) => ({ ...r, titulo: `${String(r['titulo'])}+1` });
  const m2: Migracion = (r) => ({ ...r, titulo: `${String(r['titulo'])}+2` });
  const ok = migrar({ v: 1, titulo: 'x', capas: [] }, { 1: m1, 2: m2 }, 3);
  expect(ok).toEqual({ ok: true, raw: { v: 3, titulo: 'x+1+2', capas: [] } });
  // Falta un eslabón: no se adivina.
  expect(migrar({ v: 1, capas: [] }, { 2: m2 }, 3)).toEqual({ ok: false, motivo: 'forma' });
  // Ya en la versión actual: no se toca.
  expect(migrar({ v: 3 }, {}, 3)).toEqual({ ok: true, raw: { v: 3 } });
});

test('saneado: tipos desconocidos, números y colores', () => {
  const r = sanear({
    v: 1,
    ancho: 1e12,
    alto: 'x',
    fondo: 'red',
    capas: [
      {
        id: 'c',
        nombre: '',
        nodos: [
          {
            id: 'ok',
            tipo: 'forma',
            forma: 'dragon',
            x: Infinity,
            y: NaN,
            w: -5,
            h: 1e12,
            estilo: { relleno: 'url(javascript:1)', grosor: 999, opacidad: 7 },
          },
          { id: 'raro', tipo: 'script' },
          'cadena',
          null,
        ],
      },
    ],
  });
  if (!r.ok) throw new Error('debería sanearse');
  const e = r.escena;
  expect(e.ancho).toBe(LIMITES.dimension);
  expect(e.alto).toBe(600);
  expect(e.fondo).toBeUndefined();
  expect(e.capas[0]!.nombre).toBe('Capa 1');
  expect(e.capas[0]!.nodos).toHaveLength(1);
  expect(e.capas[0]!.nodos[0]).toMatchObject({
    forma: 'rect',
    x: 0,
    y: 0,
    w: 1,
    h: LIMITES.dimension,
    estilo: { grosor: 64, opacidad: 1 },
  });
  expect(JSON.stringify(e)).not.toContain('javascript');
});

test('saneado: ids únicos y bien formados', () => {
  const r = sanear({
    v: 1,
    capas: [
      {
        id: 'c',
        nodos: [
          { id: 'a', tipo: 'trazo', puntos: [0, 0] },
          { id: 'a', tipo: 'trazo', puntos: [0, 0] },
          { id: '<img onerror=1>', tipo: 'trazo', puntos: [0, 0] },
        ],
      },
    ],
  });
  if (!r.ok) throw new Error('debería sanearse');
  const ids = r.escena.capas[0]!.nodos.map((n) => n.id);
  expect(new Set(ids).size).toBe(3);
  expect(ids.every((i) => /^[A-Za-z0-9_-]{1,64}$/.test(i))).toBe(true);
});

test('saneado: imágenes solo data:image, http(s) o ruta absoluta', () => {
  const img = (href: string) => ({
    id: 'i',
    tipo: 'imagen',
    x: 0,
    y: 0,
    w: 5,
    h: 5,
    href,
    alt: '',
  });
  const n = (href: string): number => {
    const r = sanear({ v: 1, capas: [{ id: 'c', nodos: [img(href)] }] });
    return r.ok ? r.escena.capas[0]!.nodos.length : -1;
  };
  expect(n('data:image/png;base64,iVBOR')).toBe(1);
  expect(n('https://sytue.eu/logo.png')).toBe(1);
  expect(n('/api/v1/drive/x')).toBe(1);
  expect(n('javascript:alert(1)')).toBe(0);
  expect(n('data:image/svg+xml;base64,PHN2Zz4=')).toBe(0);
  expect(n('//evil.example/x.png')).toBe(0);
  expect(n('data:text/html;base64,AAAA')).toBe(0);
});

test('saneado: un extremo anclado a un nodo inexistente pasa a punto libre', () => {
  const r = sanear({
    v: 1,
    capas: [
      {
        id: 'c',
        nodos: [
          { id: 'a', tipo: 'forma', forma: 'rect', x: 0, y: 0, w: 5, h: 5 },
          {
            id: 'k',
            tipo: 'conector',
            trazado: 'zigzag',
            desde: { nodo: 'a', lado: 'left' },
            hasta: { nodo: 'fantasma' },
          },
        ],
      },
    ],
  });
  if (!r.ok) throw new Error('debería sanearse');
  expect(r.escena.capas[0]!.nodos[1]).toMatchObject({
    trazado: 'straight',
    desde: { nodo: 'a', lado: 'left' },
    hasta: { x: 0, y: 0 },
  });
});

test('saneado: los conectores no viven dentro de grupos', () => {
  const r = sanear({
    v: 1,
    capas: [
      {
        id: 'c',
        nodos: [
          {
            id: 'g',
            tipo: 'grupo',
            x: 0,
            y: 0,
            w: 5,
            h: 5,
            hijos: [{ id: 'k', tipo: 'conector', desde: { x: 1, y: 1 }, hasta: { x: 2, y: 2 } }],
          },
        ],
      },
    ],
  });
  if (!r.ok) throw new Error('debería sanearse');
  expect(r.escena.capas[0]!.nodos[0]).toMatchObject({ tipo: 'grupo', hijos: [] });
});

test('límites: profundidad, nodos y puntos', () => {
  let anidado: Record<string, unknown> = { id: 'hoja', tipo: 'trazo', puntos: [0, 0] };
  for (let i = 0; i < LIMITES.profundidad + 3; i++) {
    anidado = { id: `g${i}`, tipo: 'grupo', x: 0, y: 0, w: 1, h: 1, hijos: [anidado] };
  }
  const r = sanear({ v: 1, capas: [{ id: 'c', nodos: [anidado] }] });
  expect(r.ok && JSON.stringify(r.escena).includes('hoja')).toBe(false);

  const muchos = Array.from({ length: LIMITES.nodos + 50 }, (_, i) => ({
    id: `n${i}`,
    tipo: 'linea',
  }));
  const r2 = sanear({ v: 1, capas: [{ id: 'c', nodos: muchos }] });
  expect(r2.ok && r2.escena.capas[0]!.nodos.length).toBe(LIMITES.nodos);

  const puntos = Array.from({ length: LIMITES.puntos * 2 + 100 }, (_, i) => i % 50);
  const r3 = sanear({ v: 1, capas: [{ id: 'c', nodos: [{ id: 't', tipo: 'trazo', puntos }] }] });
  const tr = r3.ok ? r3.escena.capas[0]!.nodos[0] : undefined;
  expect(tr && tr.tipo === 'trazo' && tr.puntos.length).toBe(LIMITES.puntos * 2);
});

test('una escena sin capas recibe una capa vacía', () => {
  const r = sanear({ v: 1 });
  expect(r.ok && r.escena.capas).toHaveLength(1);
});
