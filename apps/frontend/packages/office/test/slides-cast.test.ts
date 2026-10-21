/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de «Presentar con Cast» (slides/cast.ts): el título de una diapositiva, la
 * posición que se envía —y solo ella— y el avisador que no amontona peticiones.
 * Ejecutar: cd apps/frontend/packages/office && bun test
 */
import { expect, test } from 'bun:test';
import {
  MAX_TITULO,
  avisador,
  posicionDe,
  textoPlano,
  tituloDeDiapositiva,
  urlDeEmision,
  type PosicionCast,
} from '../src/editor/slides/cast';
import type { Slide } from '../src/editor/types';

const caja = (id: string, html: string, kind?: 'shape') => ({
  id,
  x: 0,
  y: 0,
  w: 10,
  h: 10,
  html,
  ...(kind ? { kind } : {}),
});
const diapo = (...cajas: ReturnType<typeof caja>[]): Slide => ({ id: 's', boxes: cajas });

test('textoPlano: sin etiquetas, con las entidades y los espacios en uno', () => {
  expect(textoPlano('<h1>Cuentas &amp; <b>pozo</b></h1><p>a&nbsp;b</p>')).toBe(
    'Cuentas & pozo a b',
  );
  expect(textoPlano('<p>uno<br>dos</p>')).toBe('uno dos');
});

test('tituloDeDiapositiva: el encabezado manda; si no, la primera línea de texto', () => {
  expect(
    tituloDeDiapositiva(diapo(caja('a', '<p>cuerpo</p>'), caja('b', '<h1>Presupuesto</h1>'))),
  ).toBe('Presupuesto');
  expect(tituloDeDiapositiva(diapo(caja('a', '<p>Primera línea</p><p>segunda</p>')))).toBe(
    'Primera línea',
  );
  // Una forma no es un título aunque lleve texto.
  expect(tituloDeDiapositiva(diapo(caja('a', '<h1>Forma</h1>', 'shape')))).toBe('');
  expect(tituloDeDiapositiva(diapo(caja('a', '<h1></h1>'), caja('b', '<p>Resto</p>')))).toBe(
    'Resto',
  );
  expect(tituloDeDiapositiva(diapo())).toBe('');
  expect(tituloDeDiapositiva(undefined)).toBe('');
  expect(
    Array.from(tituloDeDiapositiva(diapo(caja('a', `<h1>${'ñ'.repeat(500)}</h1>`)))).length,
  ).toBe(MAX_TITULO);
});

test('posicionDe: solo número, total, título y origen; nunca notas ni contenido', () => {
  const slides: Slide[] = [
    { ...diapo(caja('a', '<h1>Portada</h1><p>secreto</p>')), notes: 'decir que cuesta 40.000' },
    diapo(),
  ];
  const p = posicionDe(slides, 0, 'abc-1');
  expect(p).toEqual({ index: 0, total: 2, title: 'Portada', ref: 'docs:abc-1' });
  expect(JSON.stringify(p)).not.toContain('40.000');
  expect(JSON.stringify(p)).not.toContain('secreto');
  expect(posicionDe(slides, 2, 'x')).toBeNull();
  expect(posicionDe([], 0, 'x')).toBeNull();
});

test('urlDeEmision: la página de emitir de Cast con el directo', () => {
  expect(urlDeEmision('v-1')).toContain('/studio?live=v-1');
});

const pos = (index: number): PosicionCast => ({ index, total: 9, title: '', ref: '' });

test('avisador: de una en una y, si se pasa deprisa, solo la última', async () => {
  const enviadas: number[] = [];
  const sueltas: (() => void)[] = [];
  const enviar = (_u: string, p: PosicionCast): Promise<void> =>
    new Promise((ok) => {
      enviadas.push(p.index);
      sueltas.push(ok);
    });
  const avisar = avisador('v', () => undefined, enviar);
  avisar(pos(0));
  avisar(pos(1));
  avisar(pos(2));
  expect(enviadas).toEqual([0]); // la 1 y la 2 esperan
  sueltas.shift()?.();
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
  expect(enviadas).toEqual([0, 2]); // la 1 se saltó: la más reciente es la 2
});

test('avisador: un fallo se dice una vez por racha y no para los siguientes avisos', async () => {
  let fallos = 0;
  let intentos = 0;
  const enviar = (): Promise<void> => {
    intentos++;
    return intentos <= 2 ? Promise.reject(new Error('caído')) : Promise.resolve();
  };
  const avisar = avisador('v', () => fallos++, enviar);
  for (const i of [0, 1, 2, 3]) {
    avisar(pos(i));
    await new Promise((r) => setTimeout(r, 0));
  }
  expect(intentos).toBe(4);
  expect(fallos).toBe(1);
});
