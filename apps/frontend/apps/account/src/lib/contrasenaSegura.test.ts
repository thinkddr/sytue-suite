/*
 * SPDX-License-Identifier: Apache-2.0
 * SPDX-FileCopyrightText: (c) 2026 Daniel Delgado Ramos
 *
 * La contraseña sugerida cumple SIEMPRE la política que luego comprueba el
 * servidor —si no, el botón ofrecería una que el propio formulario rechaza— y no
 * arrastra el sesgo de módulo del generador.
 */
import { describe, expect, test } from 'bun:test';
import { ALFABETO, sugerirContrasena, type Aleatorio } from './contrasenaSegura.js';
import { requisitosPassword } from './requisitosPassword.js';

/** Una secuencia fija de valores de 32 bits, en bucle. */
function secuencia(valores: number[]): Aleatorio {
  let i = 0;
  return (destino) => {
    for (let k = 0; k < destino.length; k++) destino[k] = valores[i++ % valores.length]!;
    return destino;
  };
}

describe('sugerirContrasena', () => {
  test('cuatro grupos de cinco, con el alfabeto sin ambiguos', () => {
    const c = sugerirContrasena(null);
    expect(c).toMatch(/^[a-z2-9]{5}(-[a-z2-9]{5}){3}$/);
    for (const ch of c.replaceAll('-', '')) expect(ALFABETO).toContain(ch);
  });

  test('cumple la política más exigente: longitud y letra + dígito', () => {
    for (const min of [8, 12, 24, 30, 64]) {
      const politica = { min_length: min, require_complexity: true };
      for (let i = 0; i < 50; i++) {
        const c = sugerirContrasena(politica);
        const reqs = requisitosPassword(politica, c, 'la-actual');
        expect(reqs.every((r) => r.cumplido)).toBe(true);
      }
    }
  });

  test('aunque el azar solo dé letras, lleva un dígito (y al revés)', () => {
    // 0 es siempre el índice 0: «a» en el alfabeto y en las letras, «2» en los dígitos.
    const c = sugerirContrasena({ min_length: 8, require_complexity: true }, secuencia([0]));
    expect(c).toMatch(/[a-z]/);
    expect(c).toMatch(/[0-9]/);
  });

  test('descarta los valores de la cola en vez de doblar la probabilidad de los primeros', () => {
    // 2^32 - 1 está en la cola para n = 31 (2^32 mod 31 = 4): se descarta y vale el
    // siguiente. Sin el rechazo saldría (2^32 - 1) % 31 = 3, la «d» del alfabeto.
    const cola = 2 ** 32 - 1;
    const c = sugerirContrasena(null, secuencia([cola, 1]));
    expect(c[0]).toBe(ALFABETO[1]!);
  });
});
