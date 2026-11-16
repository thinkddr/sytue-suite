/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Un fallo de Sytue Fotos se dice en el idioma de la persona, según su estado, y con
 * su salida. Antes las pantallas pintaban `String(e)`: «Error: Sytue Fotos respondió
 * 500» en castellano para todo el mundo, y sin «Reintentar».
 */
import { afterEach, expect, test } from 'bun:test';
import { readdirSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { translate } from '@sytue/i18n';
import {
  ErrorDeFotos,
  guardarReceta,
  indexar,
  lineaDeTiempo,
  restaurar,
} from '../client/photosApi.js';
import { subirFotos } from '../client/subida.js';
import { recetaVacia } from '../editor/receta.js';
import { estadoDeFallo, pantallaDeFallo, textoDeFallo } from './fallo.js';

const original = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = original;
});

function contesta(status: number, cuerpo: unknown = {}): void {
  globalThis.fetch = (async () => Response.json(cuerpo, { status })) as unknown as typeof fetch;
}

test('cada estado HTTP es un estado de la pantalla, como en Fyne', () => {
  expect(estadoDeFallo(new ErrorDeFotos(401))).toBe('sinSesion');
  expect(estadoDeFallo(new ErrorDeFotos(403))).toBe('sinPermiso');
  expect(estadoDeFallo(new ErrorDeFotos(500))).toBe('error');
  expect(estadoDeFallo(new ErrorDeFotos(507))).toBe('error');
  // `fetch` sin red rechaza con un TypeError: no contestó nadie.
  expect(estadoDeFallo(new TypeError('Failed to fetch'))).toBe('sinConexion');
  // Lo que contestó pero no se entiende (HTML en vez de JSON, una imagen rota) es
  // una avería, no la red: «sin conexión» mandaría a revisar una red que va.
  expect(estadoDeFallo(new SyntaxError('Unexpected token <'))).toBe('error');
  expect(estadoDeFallo(new DOMException('decode', 'InvalidStateError'))).toBe('error');
});

test('con el servicio contestando 500, la galería en alemán lo dice en alemán y nada en castellano', async () => {
  contesta(500);
  const e = await lineaDeTiempo().catch((x: unknown) => x);
  expect(e).toBeInstanceOf(ErrorDeFotos);
  expect((e as ErrorDeFotos).status).toBe(500);

  const texto = textoDeFallo('de', e);
  expect(texto).toBe(translate('de', 'error.connection'));
  expect(texto).not.toBe(translate('es', 'error.connection'));
  expect(texto).not.toContain('respondió');

  const p = pantallaDeFallo('de', e, 'Sytue Fotos');
  expect(p.titulo).toBe(translate('de', 'error.connection'));
  expect(p.salida).toBe('reintentar');
  expect(p.rotulo).toBe(translate('de', 'action.retry'));
});

test('un 401 ofrece entrar; un 403 explica el permiso y no ofrece reintentar', () => {
  const sesion = pantallaDeFallo('de', new ErrorDeFotos(401), 'Sytue Fotos');
  expect(sesion.salida).toBe('entrar');
  expect(sesion.rotulo).toBe(translate('de', 'account.signIn'));

  const permiso = pantallaDeFallo('de', new ErrorDeFotos(403), 'Sytue Fotos');
  expect(permiso.titulo).toBe(translate('de', 'forbidden.title'));
  expect(permiso.descripcion).toBe(translate('de', 'forbidden.description'));
  expect(permiso.salida).toBeNull();
});

test('sin red: «sin conexión» con su explicación y «Reintentar»', () => {
  const p = pantallaDeFallo('fr', new TypeError('Failed to fetch'), 'Sytue Photos');
  expect(p.titulo).toBe(translate('fr', 'offline.status'));
  expect(p.descripcion).toBe(translate('fr', 'offline.description'));
  expect(p.salida).toBe('reintentar');
  expect(textoDeFallo('fr', new TypeError('x'))).toBe(translate('fr', 'offline.description'));
});

test('un 507 en una acción dice que el Drive de la dueña está lleno', async () => {
  contesta(507);
  const e = await restaurar('f1').catch((x: unknown) => x);
  expect(textoDeFallo('it', e)).toBe(translate('it', 'sharedAlbum.quotaFull'));
});

test('guardar la receta, indexar y subir también llevan su estado', async () => {
  contesta(403);
  const receta = await guardarReceta('f1', recetaVacia()).catch((x: unknown) => x);
  expect(receta).toBeInstanceOf(ErrorDeFotos);
  expect((receta as ErrorDeFotos).status).toBe(403);

  contesta(502);
  const alta = await indexar('d1').catch((x: unknown) => x);
  expect(alta).toBeInstanceOf(ErrorDeFotos);
  expect((alta as ErrorDeFotos).status).toBe(502);

  // La subida sigue apuntando el nombre que falló, sin llevarse las demás.
  contesta(500);
  const fichero = new File([new Uint8Array([1])], 'playa.jpg', { type: 'image/jpeg' });
  const { fallos } = await subirFotos([fichero], () => {});
  expect(fallos).toEqual(['playa.jpg']);
});

test('ninguna pantalla pinta el error crudo ni el cliente lanza frases', () => {
  const raiz = join(import.meta.dir, '..');
  const crudos: string[] = [];
  for (const f of readdirSync(join(raiz, 'components')).filter((x) => x.endsWith('.tsx'))) {
    const fuente = readFileSync(join(raiz, 'components', f), 'utf8');
    // `String(e)` solo vale para mirar el código de la IA, nunca para enseñarlo.
    for (const m of fuente.matchAll(/(?:set\w*|avisar)\(\s*String\(\w+\)\s*\)/g)) {
      crudos.push(`${f}: ${m[0]}`);
    }
  }
  for (const f of ['photosApi.ts', 'subida.ts']) {
    const fuente = readFileSync(join(raiz, 'client', f), 'utf8');
    for (const m of fuente.matchAll(/throw new Error\(\s*['`][^'`]*\s[^'`]*['`]/g)) {
      crudos.push(`${f}: ${m[0]}`);
    }
  }
  expect(crudos).toEqual([]);
});
