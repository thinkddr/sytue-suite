/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, it } from 'bun:test';
import type { DiferenciaVisual } from './api';
import {
  SENSIBILIDADES,
  cajaEnPorcentaje,
  claveDeAviso,
  moverParada,
  paradasDe,
  porcentajeLegible,
  toleranciaDe,
} from './comparacionVisual.js';
import { en } from '../i18n/messages/en';

const pagina = (
  n: number,
  regiones: number,
  extra: Partial<DiferenciaVisual> = {},
): DiferenciaVisual => ({
  pagina: n,
  ancho: 595,
  alto: 842,
  escala: 1.5,
  pixeles_diferentes: regiones,
  pixeles_totales: 1000,
  porcentaje: 0.1,
  regiones: Array.from({ length: regiones }, (_, i) => ({ x: i, y: i, ancho: 1, alto: 1 })),
  ...extra,
});

describe('comparación visual de PDF', () => {
  it('cada sensibilidad y cada aviso tienen texto en el catálogo', () => {
    for (const s of SENSIBILIDADES) expect(s.clave in en).toBe(true);
    const avisos = [
      claveDeAviso(pagina(1, 0, { solo_en: 'a' })),
      claveDeAviso(pagina(1, 0, { solo_en: 'b' })),
      claveDeAviso(pagina(1, 0, { omitida: true, motivo: 'demasiado_grande' })),
      claveDeAviso(pagina(1, 0, { omitida: true, motivo: 'ilegible' })),
    ];
    for (const k of avisos) {
      expect(k).not.toBeNull();
      expect(k! in en).toBe(true);
    }
    expect(new Set(avisos).size).toBe(4);
    expect(claveDeAviso(pagina(1, 2))).toBeNull();
  });

  it('la sensibilidad por defecto es la del motor (48) y ninguna se sale de 1-255', () => {
    expect(toleranciaDe('normal')).toBe(48);
    for (const s of SENSIBILIDADES) {
      expect(s.tolerancia).toBeGreaterThanOrEqual(1);
      expect(s.tolerancia).toBeLessThanOrEqual(255);
    }
    // más sensible = menos tolerancia
    expect(toleranciaDe('alta')).toBeLessThan(toleranciaDe('normal'));
    expect(toleranciaDe('normal')).toBeLessThan(toleranciaDe('baja'));
  });

  it('las paradas van en orden de lectura y una página sin cajas cuenta una vez', () => {
    const p = paradasDe([pagina(1, 2), pagina(2, 0, { solo_en: 'b' }), pagina(4, 1)]);
    expect(p).toEqual([
      { pagina: 1, region: 0 },
      { pagina: 1, region: 1 },
      { pagina: 2, region: -1 },
      { pagina: 4, region: 0 },
    ]);
    expect(paradasDe([])).toEqual([]);
  });

  it('navegar da la vuelta en los dos extremos', () => {
    expect(moverParada(0, 3, 1)).toBe(1);
    expect(moverParada(2, 3, 1)).toBe(0);
    expect(moverParada(0, 3, -1)).toBe(2);
    expect(moverParada(0, 0, 1)).toBe(0);
    expect(moverParada(0, 1, -1)).toBe(0);
  });

  it('la caja se sitúa en porcentajes del lienzo y no se sale de él', () => {
    expect(cajaEnPorcentaje({ x: 100, y: 200, ancho: 50, alto: 100 }, 500, 1000)).toEqual({
      left: '20.000%',
      top: '20.000%',
      width: '10.000%',
      height: '10.000%',
    });
    const fuera = cajaEnPorcentaje({ x: -5, y: 2000, ancho: 9999, alto: 1 }, 500, 1000);
    expect(fuera.left).toBe('0.000%');
    expect(fuera.top).toBe('100.000%');
    expect(fuera.width).toBe('100.000%');
    // un lienzo sin medida no divide por cero
    expect(cajaEnPorcentaje({ x: 1, y: 1, ancho: 1, alto: 1 }, 0, 0).left).toBe('0%');
  });

  it('el porcentaje no dice «0 %» cuando algo difiere', () => {
    expect(porcentajeLegible(0, 'es')).toBe('0');
    expect(porcentajeLegible(0.00001, 'en')).toBe('0.001');
    expect(porcentajeLegible(0.0123, 'en')).toBe('0.012');
    expect(porcentajeLegible(12.345, 'en')).toBe('12.3');
  });
});
