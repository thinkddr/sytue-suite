/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 */
import { describe, expect, test } from 'bun:test';
import { esperarALaPuerta, INTENTOS_PUERTA, PAUSA_PUERTA_MS } from './puerta.js';

class Cerrada extends Error {}
const sinEsperar = (): Promise<void> => Promise.resolve();

describe('esperarALaPuerta', () => {
  test('entra en cuanto el anfitrión abre, sin pedir nada más', async () => {
    let veces = 0;
    const pausas: number[] = [];
    const r = await esperarALaPuerta({
      entrar: () => {
        veces++;
        return veces < 3 ? Promise.reject(new Cerrada()) : Promise.resolve('dentro');
      },
      cerrada: (e) => e instanceof Cerrada,
      esperar: (ms) => {
        pausas.push(ms);
        return Promise.resolve();
      },
    });
    expect(r).toBe('dentro');
    expect(veces).toBe(3);
    // Antes de cada intento, la misma pausa que Fyne.
    expect(pausas).toEqual([PAUSA_PUERTA_MS, PAUSA_PUERTA_MS, PAUSA_PUERTA_MS]);
  });

  test('se rinde tras los intentos de Fyne sin lanzar', async () => {
    let veces = 0;
    const r = await esperarALaPuerta({
      entrar: () => {
        veces++;
        return Promise.reject(new Cerrada());
      },
      cerrada: (e) => e instanceof Cerrada,
      esperar: sinEsperar,
    });
    expect(r).toBeNull();
    expect(veces).toBe(INTENTOS_PUERTA);
  });

  test('otro error corta la espera y llega a quien llamó', async () => {
    let veces = 0;
    const espera = esperarALaPuerta({
      entrar: () => {
        veces++;
        return Promise.reject(new Error('la reunión terminó'));
      },
      cerrada: (e) => e instanceof Cerrada,
      esperar: sinEsperar,
    });
    expect(espera).rejects.toThrow('la reunión terminó');
    await espera.catch(() => undefined);
    expect(veces).toBe(1);
  });

  test('cancelar antes del siguiente intento no vuelve a pedir', async () => {
    let veces = 0;
    let cancelado = false;
    const r = await esperarALaPuerta({
      entrar: () => {
        veces++;
        cancelado = true; // p. ej., quien espera cierra la vista
        return Promise.reject(new Cerrada());
      },
      cerrada: (e) => e instanceof Cerrada,
      esperar: sinEsperar,
      cancelado: () => cancelado,
    });
    expect(r).toBeNull();
    expect(veces).toBe(1);
  });
});
