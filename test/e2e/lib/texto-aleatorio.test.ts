// SPDX-License-Identifier: Apache-2.0
import { expect, test } from 'bun:test';
import { textoAleatorio, type FuenteDeBytes } from './texto-aleatorio.js';

const ALFABETO = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';

/** Fuente determinista que recorre todos los bytes 0..255 por igual, en bucle. */
function todosLosBytes(): FuenteDeBytes {
  let siguiente = 0;
  return (n) => {
    const out = new Uint8Array(n);
    for (let i = 0; i < n; i++) out[i] = siguiente++ & 0xff;
    return out;
  };
}

function cuentas(texto: string): Map<string, number> {
  const m = new Map<string, number>();
  for (const c of texto) m.set(c, (m.get(c) ?? 0) + 1);
  return m;
}

// Regresión del sesgo de módulo: con bytes perfectamente uniformes, `byte % 62`
// daba 5 apariciones a A..H por cada 4 del resto. Con rechazo, todas iguales.
test('con bytes uniformes, cada símbolo aparece exactamente las mismas veces', () => {
  const texto = textoAleatorio(ALFABETO.length * 100, ALFABETO, todosLosBytes());
  const c = cuentas(texto);
  expect(c.size).toBe(ALFABETO.length);
  expect(new Set(c.values())).toEqual(new Set([100]));
});

test('respeta la longitud, solo usa el alfabeto y admite alfabetos extremos', () => {
  const texto = textoAleatorio(29_000, ALFABETO);
  expect(texto).toHaveLength(29_000);
  expect(/^[A-Za-z0-9]+$/.test(texto)).toBe(true);
  expect(textoAleatorio(0, ALFABETO)).toBe('');
  expect(textoAleatorio(5, 'x')).toBe('xxxxx');
  expect(textoAleatorio(300, 'ab', todosLosBytes())).toHaveLength(300);
});

// Cordura de la distribución real: chi-cuadrado con 61 grados de libertad. El umbral
// (130) queda muy por encima del percentil 99,99 (~110): no da falsos positivos, pero
// el sesgo de módulo (chi² ≈ 1300 con esta muestra) lo superaría con creces.
test('con la fuente criptográfica la distribución es plana', () => {
  const muestras = 200_000;
  const c = cuentas(textoAleatorio(muestras, ALFABETO));
  const esperado = muestras / ALFABETO.length;
  let chi2 = 0;
  for (const s of ALFABETO) chi2 += ((c.get(s) ?? 0) - esperado) ** 2 / esperado;
  expect(chi2).toBeLessThan(130);
});
