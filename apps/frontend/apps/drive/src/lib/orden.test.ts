/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * El orden de una carpeta de Drive: carpetas delante, el campo elegido y desempate por
 * nombre; y lo que se recuerda entre recargas.
 */
import { GlobalRegistrator } from '@happy-dom/global-registrator';
import { afterEach, beforeEach, expect, test } from 'bun:test';
import { partirEnTeselas } from './teselas';
import {
  guardarOrden,
  guardarVista,
  leerOrden,
  leerVista,
  ordenarRecursos,
  siguienteOrden,
  esOrdenDelServicio,
  ordenDeLaPeticion,
  parametrosDeOrden,
  ORDEN_DEL_SERVICIO,
  type CampoOrden,
  type Orden,
} from './orden';
import type { Locale } from '@sytue/i18n';
import { carpetaEnCache, urlDeListado } from './ubicacion';

if (typeof document === 'undefined') GlobalRegistrator.register();

beforeEach(() => localStorage.clear());
// Y al salir: el orden guardado no debe colarse en los tests de otros ficheros del mismo proceso.
afterEach(() => localStorage.clear());

const f = (name: string, size: number, modifiedAt: string, isDirectory = false) => ({
  name,
  size,
  modifiedAt,
  isDirectory,
});
const items = [
  f('informe 10.pdf', 300, '2026-01-03T10:00:00Z'),
  f('Zeta', 0, '2026-01-01T10:00:00Z', true),
  f('informe 2.pdf', 100, '2026-01-05T10:00:00Z'),
  f('alfa', 0, '2026-01-09T10:00:00Z', true),
  f('árbol.txt', 100, '2026-01-05T10:00:00Z'),
];
const nombres = (xs: { name: string }[]) => xs.map((x) => x.name);

test('por nombre: carpetas delante y orden natural, sin distinguir tildes ni mayúsculas', () => {
  expect(nombres(ordenarRecursos(items, { campo: 'name', sentido: 'asc' }, 'es'))).toEqual([
    'alfa',
    'Zeta',
    'árbol.txt',
    'informe 2.pdf',
    'informe 10.pdf',
  ]);
});

// Los MISMOS casos que `drive.Ordenar` de la app nativa: si uno ordena distinto, la
// misma carpeta se ve en otro orden en la web y en el escritorio.
const CASOS = new URL('../../../../../fyne/apps/drive/testdata/orden.json', import.meta.url);

test('ordena igual que la app nativa (testdata/orden.json)', async () => {
  const casos = (await Bun.file(CASOS).json()) as {
    elementos: { name: string; isDirectory: boolean; size: number; modifiedAt: string }[];
    casos: { nota: string; orden: Orden; locale: Locale; esperado: string[] }[];
    siguiente: { actual: Orden; campo: CampoOrden; esperado: Orden }[];
  };
  expect(casos.casos.length).toBeGreaterThan(5);
  for (const c of casos.casos) {
    expect([c.nota, nombres(ordenarRecursos(casos.elementos, c.orden, c.locale))]).toEqual([
      c.nota,
      c.esperado,
    ]);
  }
  for (const s of casos.siguiente) expect(siguienteOrden(s.actual, s.campo)).toEqual(s.esperado);
});

test('«Modificado» va de lo más reciente a lo más antiguo y la segunda pulsación invierte', () => {
  const o = siguienteOrden(ORDEN_DEL_SERVICIO, 'modified');
  expect(o).toEqual({ campo: 'modified', sentido: 'desc' });
  // Empate de fecha entre «árbol» e «informe 2»: decide el nombre.
  expect(nombres(ordenarRecursos(items, o, 'es'))).toEqual([
    'alfa',
    'Zeta',
    'árbol.txt',
    'informe 2.pdf',
    'informe 10.pdf',
  ]);
  const o2 = siguienteOrden(o, 'modified');
  expect(o2.sentido).toBe('asc');
  expect(nombres(ordenarRecursos(items, o2, 'es'))).toEqual([
    'Zeta',
    'alfa',
    'informe 10.pdf',
    'árbol.txt',
    'informe 2.pdf',
  ]);
});

test('por tamaño, lo más grande primero; las carpetas siguen delante', () => {
  const o = siguienteOrden(ORDEN_DEL_SERVICIO, 'size');
  expect(nombres(ordenarRecursos(items, o, 'es')).slice(2)).toEqual([
    'informe 10.pdf',
    'árbol.txt',
    'informe 2.pdf',
  ]);
});

test('la selección por rango sigue el orden visible: enOrden sale de la lista ordenada', () => {
  const visibles = ordenarRecursos(items, { campo: 'size', sentido: 'desc' }, 'es');
  expect(nombres([...partirEnTeselas(visibles, true).enOrden])).toEqual(nombres(visibles));
});

test('solo el nombre ascendente es el orden del servicio', () => {
  expect(esOrdenDelServicio({ campo: 'name', sentido: 'asc' })).toBe(true);
  expect(esOrdenDelServicio({ campo: 'name', sentido: 'desc' })).toBe(false);
  expect(esOrdenDelServicio({ campo: 'size', sentido: 'asc' })).toBe(false);
});

test('el orden y la vista sobreviven a una recarga; un valor corrupto vuelve al de siempre', () => {
  expect(leerOrden()).toEqual(ORDEN_DEL_SERVICIO);
  expect(leerVista()).toBe('list');
  guardarOrden({ campo: 'modified', sentido: 'desc' });
  guardarVista('grid');
  expect(leerOrden()).toEqual({ campo: 'modified', sentido: 'desc' });
  expect(leerVista()).toBe('grid');
  localStorage.setItem('sytue.drive.orden', 'peso:arriba');
  expect(leerOrden()).toEqual(ORDEN_DEL_SERVICIO);
});

test('el servicio recibe el orden con sus nombres; el de siempre no lleva parámetros', () => {
  expect(parametrosDeOrden(ORDEN_DEL_SERVICIO)).toBeNull();
  expect(parametrosDeOrden({ campo: 'modified', sentido: 'desc' })).toEqual({
    orden: 'modificado',
    sentido: 'desc',
  });
  expect(parametrosDeOrden({ campo: 'size', sentido: 'asc' })).toEqual({
    orden: 'tamano',
    sentido: 'asc',
  });
  expect(parametrosDeOrden({ campo: 'name', sentido: 'desc' })).toEqual({
    orden: 'nombre',
    sentido: 'desc',
  });
});

test('la página siguiente se pide en el mismo orden que la primera', () => {
  const o = { campo: 'modified', sentido: 'desc' } as const;
  expect(urlDeListado('/Actas', undefined, o)).toBe(
    '/api/files?path=%2FActas&limit=200&orden=modificado&sentido=desc',
  );
  expect(urlDeListado('/Actas', 'c1', o)).toBe(
    '/api/files?path=%2FActas&limit=200&orden=modificado&sentido=desc&cursor=c1',
  );
  // El orden de siempre conserva la URL de antes: la de la caché sin conexión.
  expect(urlDeListado('/Actas', undefined, ORDEN_DEL_SERVICIO)).toBe(urlDeListado('/Actas'));
});

test('el BFF solo reenvía órdenes y sentidos conocidos', () => {
  expect(ordenDeLaPeticion(new URLSearchParams('orden=tamano&sentido=desc'))).toEqual({
    orden: 'tamano',
    sentido: 'desc',
  });
  expect(ordenDeLaPeticion(new URLSearchParams('orden=peso&sentido=arriba'))).toEqual({});
  expect(ordenDeLaPeticion(new URLSearchParams(''))).toEqual({});
});

test('sin red, «Mostrar en su carpeta» busca la carpeta en el orden que se recuerda', async () => {
  const base = 'https://acme.sytue.eu/drive/';
  const o = { campo: 'size', sentido: 'desc' } as const;
  const guardada = new URL(urlDeListado('/Actas', undefined, o), base).toString();
  const almacen = {
    match: async (req: RequestInfo | URL) =>
      String(req) === guardada ? new Response('{}') : undefined,
  } as Pick<CacheStorage, 'match'>;
  // Con el orden de siempre, la guardada por tamaño no es la que se pediría al llegar.
  expect(await carpetaEnCache('/Actas', almacen, base)).toBe(false);
  guardarOrden(o);
  expect(await carpetaEnCache('/Actas', almacen, base)).toBe(true);
});
