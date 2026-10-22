/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * Self-test de la conversión de coordenadas de Sytue PDF (pdf/api.ts, pura).
 *
 * Es la única aritmética del visor que, si falla, DESTRUYE algo: con la censura
 * el rectángulo que se marca en pantalla decide qué texto se borra del fichero
 * para siempre, y un signo cambiado lo borra media página más allá. Se comprueba
 * la ida y la vuelta en las cuatro rotaciones y en las cuatro esquinas.
 * Ejecutar: cd apps/frontend/packages/office && bun test
 * (Fuera de src/ a propósito: astro check no typechequea este fichero.)
 */
import { expect, test } from 'bun:test';
import { aPDF, aSVG, colorARGB, rectEnSVG, rgbAColor, type Pagina } from '../src/pdf/api';

/** Página con MediaBox desplazado: un origen en (0,0) esconde errores de offset. */
function pagina(rotacion: number): Pagina {
  return {
    numero: 1,
    ancho: rotacion % 180 === 0 ? 400 : 600,
    alto: rotacion % 180 === 0 ? 600 : 400,
    rotacion,
    caja: [20, 10, 420, 610],
    extracto: '',
  };
}

test('aPDF y aSVG son inversas exactas en las cuatro rotaciones', () => {
  for (const rot of [0, 90, 180, 270]) {
    const p = pagina(rot);
    for (const [sx, sy] of [
      [0, 0],
      [p.ancho, 0],
      [0, p.alto],
      [p.ancho, p.alto],
      [123.5, 47.25],
    ] as const) {
      const [px, py] = aPDF(p, sx, sy);
      const [vx, vy] = aSVG(p, px, py);
      expect([vx, vy]).toEqual([sx, sy]);
    }
  }
});

test('la esquina superior izquierda de la pantalla cae donde toca en el PDF', () => {
  // Sin rotar, arriba-izquierda en pantalla es arriba-izquierda del MediaBox, y
  // en el PDF eso es la Y MÁXIMA porque su eje va hacia arriba.
  expect(aPDF(pagina(0), 0, 0)).toEqual([20, 610]);
  // Girada 90° en el sentido del reloj, el borde izquierdo pasa a ser el de
  // arriba: la esquina de pantalla es la inferior izquierda del PDF.
  expect(aPDF(pagina(90), 0, 0)).toEqual([20, 10]);
});

test('rectEnSVG normaliza: un rectángulo dibujado al revés sigue teniendo área', () => {
  const p = pagina(0);
  const caja = rectEnSVG(p, [120, 500, 220, 560]);
  expect(caja).toEqual({ x: 100, y: 50, ancho: 100, alto: 60 });
  // Las esquinas cambiadas de sitio dan la MISMA caja: el motor normaliza igual,
  // y si aquí saliera negativa la zona censurada no se pintaría.
  expect(rectEnSVG(p, [220, 560, 120, 500])).toEqual(caja);
});

test('el color va y vuelve sin desviarse', () => {
  expect(rgbAColor(colorARGB('#f9b923'))).toBe('#f9b923');
  expect(colorARGB('#000000')).toEqual([0, 0, 0]);
  expect(colorARGB('#ffffff')).toEqual([1, 1, 1]);
});
